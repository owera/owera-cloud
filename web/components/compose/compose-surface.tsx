"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { ComplexitySlider } from "@/components/ui/slider";
import {
  complexityLevelCaption,
  complexityLevelLabel,
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
import { COMPLEXITY_LEVELS } from "@/lib/compose/levels";
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
    setState((prev) => {
      const seeded = defaultsForStop(level);
      return {
        ...prev,
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
    <form onSubmit={onSubmit} className="relative isolate">
      {/* Background composition: drifting dot grid + grain + vignette. */}
      <div
        aria-hidden
        className="absolute inset-0 -z-10 overflow-hidden rounded-sm"
      >
        <div className="absolute inset-0 compose-grid" />
        <div className="absolute inset-0 compose-grain pointer-events-none" />
        <div
          className="absolute inset-0"
          style={{
            background:
              "radial-gradient(60% 60% at 30% 20%, rgba(91,141,239,0.06), transparent 60%)",
          }}
        />
      </div>

      <div className="relative max-w-4xl mx-auto px-4 sm:px-8 py-8 sm:py-12">
        {/* Viewfinder corner brackets framing the hero canvas. */}
        <span className="compose-bracket tl" aria-hidden />
        <span className="compose-bracket tr" aria-hidden />
        <span className="compose-bracket bl" aria-hidden />
        <span className="compose-bracket br" aria-hidden />

        {/* Editorial header — small mono kicker over an oversized serif line. */}
        <header className="flex flex-col gap-4 mb-12">
          <div
            className="flex items-center justify-between compose-rise"
            style={{ ["--rise-delay" as string]: "0ms" }}
          >
            <span className="readout-label">NEW JOB · COMPOSE</span>
            <span className="readout-label">
              INSTR.{" "}
              {String(COMPLEXITY_LEVELS.indexOf(state.level) + 1).padStart(
                2,
                "0",
              )}{" "}
              / 05
            </span>
          </div>
          <h1
            className="compose-display text-[2.6rem] sm:text-[3.8rem] compose-rise"
            style={{ ["--rise-delay" as string]: "120ms" }}
          >
            As <em>simple</em>
            <br />
            or as <em>powerful</em>
            <br />
            as you need.
          </h1>
          <p
            className="text-sm text-[var(--color-ink-dim)] max-w-md compose-rise"
            style={{ ["--rise-delay" as string]: "260ms" }}
          >
            Dial complexity with one control. Cost and latency update live. The
            same surface drives our API — every state has a URL and a JSON
            config, so an agent can call it the same way you do.
          </p>
        </header>

        {/* The slider section — the hero element. */}
        <section
          className="relative mb-10 compose-rise"
          style={{ ["--rise-delay" as string]: "400ms" }}
        >
          <div className="flex items-baseline justify-between mb-6 pb-2 border-b border-[var(--color-rule)]">
            <span className="readout-label">Complexity</span>
            <span
              className="readout-label transition-colors duration-300"
              style={{ color: "var(--color-ink)" }}
            >
              {complexityLevelLabel(state.level)} ·{" "}
              <span className="text-[var(--color-ink-dim)]">
                {complexityLevelCaption(state.level).toUpperCase()}
              </span>
            </span>
          </div>

          <ComplexitySlider
            value={state.level}
            onChange={setLevel}
            gateAt={gateAt}
          />
        </section>

        {/* Instrument readout. */}
        <section
          className="mb-10 compose-rise"
          style={{ ["--rise-delay" as string]: "560ms" }}
        >
          <CostPreview estimate={est} />
        </section>

        {isGated && (
          <div
            className="mb-8 compose-rise"
            style={{ ["--rise-delay" as string]: "640ms" }}
          >
            <UpsellGate
              level={state.level}
              unlocks={state.level}
              variant={plan === "anonymous" ? "signin" : "checkout"}
            />
          </div>
        )}

        {/* Prompt + progressive disclosure. */}
        <section
          className="flex flex-col gap-3 compose-rise"
          style={{ ["--rise-delay" as string]: "720ms" }}
        >
          <label className="flex flex-col gap-2">
            <span className="readout-label">Prompt</span>
            <textarea
              value={state.prompt}
              onChange={(e) => patch({ prompt: e.target.value })}
              placeholder='e.g. "Summarise top 5 AI safety papers from the last week with citations."'
              rows={4}
              required
              maxLength={8000}
              className={[
                "rounded-sm border bg-[rgba(0,0,0,0.25)]",
                "border-[var(--color-rule)] focus:border-[var(--color-stop-fill)]",
                "px-4 py-3 text-base font-sans text-[var(--color-ink)]",
                "placeholder:text-[var(--color-ink-dim)]/70 placeholder:italic",
                "focus:outline-none focus:ring-1 focus:ring-[var(--color-stop-fill)]/30",
                "resize-y transition-colors",
              ].join(" ")}
            />
          </label>

          {(state.level === "advanced" ||
            state.level === "expert" ||
            state.level === "custom") && (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-2">
              <label className="flex flex-col gap-2">
                <span className="readout-label">SKU</span>
                <select
                  value={state.sku}
                  onChange={(e) => patch({ sku: e.target.value })}
                  className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none focus:ring-1 focus:ring-[var(--color-stop-fill)]/30"
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
              <label className="flex flex-col gap-2">
                <span className="readout-label">Max budget (cents)</span>
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
                  className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none focus:ring-1 focus:ring-[var(--color-stop-fill)]/30"
                />
              </label>
            </div>
          )}

          {(state.level === "expert" || state.level === "custom") && (
            <div className="mt-2">
              <span className="readout-label mb-2 inline-block">Tools</span>
              <div className="flex flex-wrap gap-1.5">
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
                      className={[
                        "px-3 h-8 rounded-sm border text-xs font-mono uppercase tracking-wider",
                        "transition-colors",
                        on
                          ? "bg-[var(--color-stop-fill)]/15 border-[var(--color-stop-fill)] text-[var(--color-stop-fill)]"
                          : "bg-transparent border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-ink)] hover:border-[var(--color-stop-fill)]/40",
                      ].join(" ")}
                    >
                      {t}
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </section>

        {/* Action row. */}
        <footer
          className="mt-10 pt-6 border-t border-[var(--color-rule)] flex items-center gap-4 compose-rise"
          style={{ ["--rise-delay" as string]: "880ms" }}
        >
          <Button
            type="submit"
            variant="primary"
            disabled={busy || isGated || !state.prompt.trim()}
            className="h-11 px-6 text-sm tracking-wider font-medium"
          >
            {busy ? "Submitting…" : "Run job →"}
          </Button>

          <span className="readout-label flex-1">
            OR POST THIS STATE TO{" "}
            <a
              href={`/api/compose?${toSearchParams(state).toString()}`}
              className="underline decoration-[var(--color-stop-fill)] decoration-1 underline-offset-4 hover:text-[var(--color-ink)]"
              target="_blank"
              rel="noreferrer"
            >
              /api/compose
            </a>
          </span>

          {error && (
            <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--color-state-failed)]">
              {error}
            </span>
          )}
        </footer>

        {/* Footer signature — instrument plate. */}
        <div
          className="mt-12 pt-4 border-t border-[var(--color-rule)] flex items-center justify-between compose-rise"
          style={{ ["--rise-delay" as string]: "1040ms" }}
        >
          <span className="readout-label">OWERA · AGENTIC · NO. 01</span>
          <span className="readout-label">PRECISION INSTRUMENT</span>
        </div>
      </div>
    </form>
  );
}
