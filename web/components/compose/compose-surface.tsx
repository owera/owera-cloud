"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { ComplexitySlider } from "@/components/ui/slider";
import {
  complexityLevelCaption,
  type ComplexityLevel,
} from "@/lib/compose/levels";
import { Button } from "@/components/ui/button";
import { CostPreview } from "./cost-preview";
import { UpsellGate } from "./upsell-gate";
import {
  defaultsForStop,
  levelRequiresAuth,
  levelRequiresPaidPlan,
  toJson,
  toSearchParams,
  type ComposeState,
} from "@/lib/compose/state";
import { estimate, type CostEstimate } from "@/lib/compose/estimate";
import type { SKU } from "@/lib/types";

interface ComposeSurfaceProps {
  initialState: ComposeState;
  skus: ReadonlyArray<SKU>;
  /** "anonymous" | "free" | "paid" — controls which gates fire. */
  plan: "anonymous" | "free" | "paid";
}

export function ComposeSurface({
  initialState,
  skus,
  plan,
}: ComposeSurfaceProps) {
  const router = useRouter();
  const [state, setState] = React.useState<ComposeState>(initialState);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const gateAt: ComplexityLevel | null = React.useMemo(() => {
    if (plan === "paid") return null;
    if (plan === "free") return "expert";
    return "advanced";
  }, [plan]);

  const isGated = React.useMemo(() => {
    if (plan === "paid") return false;
    if (plan === "free") return levelRequiresPaidPlan(state.level);
    return levelRequiresAuth(state.level);
  }, [plan, state.level]);

  // Push URL on every state change (URL is the source of truth).
  React.useEffect(() => {
    const qs = toSearchParams(state).toString();
    router.replace(`/compose${qs ? `?${qs}` : ""}`, { scroll: false });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state]);

  const est: CostEstimate = React.useMemo(
    () => estimate(state, skus),
    [state, skus],
  );

  function setLevel(level: ComplexityLevel) {
    // Snap to that stop's defaults (preserve prompt + sku if user already set them).
    setState((prev) => {
      const seeded = defaultsForStop(level);
      return {
        ...seeded,
        prompt: prev.prompt,
        sku: prev.sku || seeded.sku,
      };
    });
  }

  function patch(p: Partial<ComposeState>) {
    setState((prev) => ({ ...prev, ...p }));
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (busy) return;
    if (isGated) return;
    if (!state.prompt.trim()) {
      setError("Prompt is required.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const idemKey =
        state.idempotencyKey ??
        `compose-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const body = toJson({ ...state, idempotencyKey: idemKey });
      // Both humans and agents go through /api/compose — same code path,
      // same validation, same tier gate. The client never imports api-client
      // directly (keeps the auth/Clerk graph out of the client bundle).
      const res = await fetch("/api/compose", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const errBody = (await res.json().catch(() => null)) as
          | { code?: string; message?: string }
          | null;
        setError(
          `${errBody?.code ?? `http_${res.status}`}: ${errBody?.message ?? res.statusText}`,
        );
        setBusy(false);
        return;
      }
      const json = (await res.json()) as { job_id: string; shareUrl?: string };
      router.push(
        json.shareUrl ??
          `/jobs/${encodeURIComponent(json.job_id)}?from=compose&level=${state.level}`,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-6 max-w-3xl">
      <header className="flex flex-col gap-1">
        <div className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
          NEW JOB · COMPOSE
        </div>
        <h1 className="text-2xl font-medium tracking-tight">
          As simple or as powerful as you need.
        </h1>
        <p className="text-sm text-[var(--color-muted-foreground)]">
          Drag the slider to dial complexity. Cost and latency update live. The same
          surface drives our API — every state has a URL and a JSON config.
        </p>
      </header>

      <section className="border border-[var(--color-border)] rounded-md bg-[var(--color-card)] px-4 py-4">
        <div className="flex items-baseline justify-between mb-2">
          <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
            Complexity
          </span>
          <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
            {complexityLevelCaption(state.level)}
          </span>
        </div>
        <ComplexitySlider
          value={state.level}
          onChange={setLevel}
          gateAt={gateAt}
        />
      </section>

      <CostPreview estimate={est} />

      {isGated && (
        <UpsellGate
          level={state.level}
          unlocks={state.level}
          variant={plan === "anonymous" ? "signin" : "checkout"}
        />
      )}

      <div className="flex flex-col gap-2">
        <label className="flex flex-col gap-1 text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
          Prompt
          <textarea
            value={state.prompt}
            onChange={(e) => patch({ prompt: e.target.value })}
            placeholder='e.g. "Summarise top 5 AI safety papers from the last week with citations."'
            rows={4}
            required
            maxLength={8000}
            className="rounded-md border bg-[var(--color-input)] border-[var(--color-border)] px-3 py-2 text-sm font-sans text-[var(--color-foreground)] normal-case tracking-normal focus:outline-none focus:ring-1 focus:ring-[var(--color-ring)] resize-y"
          />
        </label>

        {(state.level === "advanced" ||
          state.level === "expert" ||
          state.level === "custom") && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-2">
            <label className="flex flex-col gap-1 text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
              SKU
              <select
                value={state.sku}
                onChange={(e) => patch({ sku: e.target.value })}
                className="h-9 rounded-md border bg-[var(--color-input)] border-[var(--color-border)] px-2 text-sm font-mono text-[var(--color-foreground)]"
              >
                {skus.length === 0 ? (
                  <option value={state.sku}>{state.sku}</option>
                ) : (
                  skus.map((s) => (
                    <option key={s.id} value={s.slug}>
                      {s.slug}
                    </option>
                  ))
                )}
              </select>
            </label>
            <label className="flex flex-col gap-1 text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
              Max budget (cents)
              <input
                type="number"
                min={0}
                value={state.budget.maxCents ?? ""}
                onChange={(e) =>
                  patch({
                    budget: {
                      ...state.budget,
                      maxCents: e.target.value
                        ? Math.max(0, Number(e.target.value))
                        : undefined,
                    },
                  })
                }
                placeholder="auto"
                className="h-9 rounded-md border bg-[var(--color-input)] border-[var(--color-border)] px-2 text-sm font-mono text-[var(--color-foreground)] focus:outline-none focus:ring-1 focus:ring-[var(--color-ring)]"
              />
            </label>
          </div>
        )}

        {(state.level === "expert" || state.level === "custom") && (
          <div className="flex flex-wrap gap-1 mt-1">
            <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)] w-full mb-1">
              Tools
            </span>
            {["web", "code", "browser", "files"].map((t) => {
              const on = state.tools.includes(t);
              return (
                <button
                  key={t}
                  type="button"
                  onClick={() =>
                    patch({
                      tools: on
                        ? state.tools.filter((x) => x !== t)
                        : [...state.tools, t],
                    })
                  }
                  className={
                    "px-2 h-7 rounded border text-xs font-mono " +
                    (on
                      ? "bg-[var(--color-primary)]/15 border-[var(--color-primary)] text-[var(--color-primary)]"
                      : "bg-[var(--color-muted)] border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]")
                  }
                >
                  {t}
                </button>
              );
            })}
          </div>
        )}
      </div>

      <div className="flex items-center gap-3 pt-2 border-t border-[var(--color-border)]">
        <Button
          type="submit"
          variant="primary"
          disabled={busy || isGated || !state.prompt.trim()}
        >
          {busy ? "Submitting…" : "Run job"}
        </Button>
        <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-muted-foreground)]">
          Or POST this state to{" "}
          <a
            href={`/api/compose?${toSearchParams(state).toString()}`}
            className="underline hover:text-[var(--color-foreground)]"
            target="_blank"
            rel="noreferrer"
          >
            /api/compose
          </a>
        </span>
        {error && (
          <span className="text-[10px] font-mono uppercase tracking-wide text-[var(--color-state-failed)] ml-auto">
            {error}
          </span>
        )}
      </div>
    </form>
  );
}
