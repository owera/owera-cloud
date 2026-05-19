"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Stepper } from "./stepper";
import { StepArchetype } from "./steps/step-archetype";
import { StepDetails } from "./steps/step-details";
import { StepCadence } from "./steps/step-cadence";
import { StepReview } from "./steps/step-review";
import {
  defaultsForArchetype,
  resolvePrompt,
  toJson,
  toSearchParams,
  type ComposeState,
} from "@/lib/compose/state";
import { getArchetype, type ArchetypeId } from "@/lib/compose/archetypes";
import type { SKU } from "@/lib/types";

interface ComposerProps {
  initialState: ComposeState;
  skus: ReadonlyArray<SKU>;
  plan: "anonymous" | "free" | "paid";
}

const STEPS = [
  { key: "outcome", label: "Outcome" },
  { key: "details", label: "Details" },
  { key: "cadence", label: "Cadence" },
  { key: "review", label: "Review" },
] as const;

/**
 * The /compose front door, v2 — a four-step composer that turns a vague
 * intent into a hireable Job.
 *
 * - Step 1 picks an archetype (outcome).
 * - Step 2 fills archetype-specific fields and the Quality dial.
 * - Step 3 sets cadence (once / daily / weekly / cron) and delivery.
 * - Step 4 reviews in plain language, names the job, and hires it.
 *
 * All state lives in one ComposeState. The slider-relevant subset round-trips
 * to the URL (so deep-links from docs still land on the right preset); the
 * archetype inputs and schedule are form-only until submit. The submitted
 * payload is the same JSON shape /api/compose validates and an agent could
 * POST directly.
 */
export function Composer({ initialState, skus, plan }: ComposerProps) {
  const router = useRouter();
  const [state, setState] = React.useState<ComposeState>(initialState);
  const [step, setStep] = React.useState(0);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  // Reflect the slider-relevant subset back to the URL so deep-links + share
  // continue to work even mid-composition.
  React.useEffect(() => {
    const qs = toSearchParams(state).toString();
    router.replace(`/compose${qs ? `?${qs}` : ""}`, { scroll: false });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    state.archetype,
    state.level,
    state.sku,
    state.prompt,
    state.tools,
    state.budget.maxCents,
    state.budget.maxLatencyMs,
  ]);

  function setArchetype(id: ArchetypeId) {
    setState((prev) => {
      // If user switches archetype, reseed inputs + recipe but keep any
      // already-typed prompt as a fallback so they don't lose their work.
      const next = defaultsForArchetype(id);
      return { ...next, prompt: prev.prompt };
    });
  }

  const canAdvance = canAdvanceFromStep(state, step);

  async function onHire() {
    if (busy) return;
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
    <div className="relative isolate">
      {/* Background composition (kept from v1). */}
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
        <span className="compose-bracket tl" aria-hidden />
        <span className="compose-bracket tr" aria-hidden />
        <span className="compose-bracket bl" aria-hidden />
        <span className="compose-bracket br" aria-hidden />

        {/* Header */}
        <header className="flex flex-col gap-4 mb-10">
          <div className="flex items-center justify-between">
            <span className="readout-label">NEW JOB · COMPOSE</span>
            <span className="readout-label">
              {getArchetype(state.archetype).name.toUpperCase()}
            </span>
          </div>
          <h1 className="compose-display text-[2rem] sm:text-[2.8rem]">
            Hire an <em>agent</em>.
            <br />
            <span className="text-[var(--color-ink-dim)]">In four steps.</span>
          </h1>
        </header>

        {/* Stepper */}
        <div className="mb-10">
          <Stepper
            steps={STEPS}
            current={step}
            onJump={(i) => setStep(i)}
          />
        </div>

        {/* Step body */}
        <div className="min-h-[400px]">
          {step === 0 && (
            <StepArchetype value={state.archetype} onChange={setArchetype} />
          )}
          {step === 1 && (
            <StepDetails
              state={state}
              setState={setState}
              skus={skus}
              plan={plan}
            />
          )}
          {step === 2 && <StepCadence state={state} setState={setState} />}
          {step === 3 && (
            <StepReview state={state} setState={setState} skus={skus} />
          )}
        </div>

        {/* Navigation footer */}
        <footer className="mt-10 pt-6 border-t border-[var(--color-rule)] flex items-center gap-4">
          <Button
            type="button"
            variant="ghost"
            disabled={step === 0 || busy}
            onClick={() => setStep((s) => Math.max(0, s - 1))}
          >
            ← Back
          </Button>

          <span className="readout-label flex-1">
            STEP {String(step + 1).padStart(2, "0")} / 04 ·{" "}
            {STEPS[step]?.label?.toUpperCase()}
          </span>

          {error && (
            <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--color-state-failed)]">
              {error}
            </span>
          )}

          {step < STEPS.length - 1 ? (
            <Button
              type="button"
              variant="primary"
              disabled={!canAdvance}
              onClick={() => setStep((s) => Math.min(STEPS.length - 1, s + 1))}
              className="h-11 px-6 text-sm tracking-wider font-medium"
            >
              Next →
            </Button>
          ) : (
            <Button
              type="button"
              variant="primary"
              disabled={busy || !canAdvance}
              onClick={onHire}
              className="h-11 px-6 text-sm tracking-wider font-medium"
            >
              {busy ? "Hiring…" : "Hire & run →"}
            </Button>
          )}
        </footer>

        {/* Instrument plate */}
        <div className="mt-12 pt-4 border-t border-[var(--color-rule)] flex items-center justify-between">
          <span className="readout-label">OWERA · AGENTIC · NO. 01</span>
          <span className="readout-label">
            POST{" "}
            <a
              href="/api/compose"
              className="underline decoration-[var(--color-stop-fill)] decoration-1 underline-offset-4 hover:text-[var(--color-ink)]"
            >
              /api/compose
            </a>{" "}
            FOR THE SAME SURFACE
          </span>
        </div>
      </div>
    </div>
  );
}

/** Gate the Next button on each step. */
function canAdvanceFromStep(state: ComposeState, step: number): boolean {
  if (step === 0) {
    return !!state.archetype;
  }
  if (step === 1) {
    const arche = getArchetype(state.archetype);
    for (const f of arche.fields) {
      if (!f.required) continue;
      const v = state.inputs[f.key];
      if (v === undefined || v === "" || (Array.isArray(v) && v.length === 0)) {
        return false;
      }
    }
    return true;
  }
  if (step === 2) {
    // Schedule + delivery: if delivery is non-dashboard, target is required.
    if (state.delivery.kind !== "dashboard" && !state.delivery.target?.trim()) {
      return false;
    }
    if (state.schedule.kind === "weekly") {
      if (!state.schedule.weekdays || state.schedule.weekdays.length === 0) {
        return false;
      }
    }
    if (state.schedule.kind === "cron") {
      if (!state.schedule.cron?.trim()) return false;
    }
    return true;
  }
  if (step === 3) {
    // Final: must have a synthesised prompt or explicit one.
    return resolvePrompt(state).length > 0;
  }
  return true;
}
