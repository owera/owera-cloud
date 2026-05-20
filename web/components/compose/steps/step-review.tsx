"use client";

import * as React from "react";
import {
  resolveJobName,
  resolvePrompt,
  type ComposeState,
} from "@/lib/compose/state";
import { getArchetype } from "@/lib/compose/archetypes";
import { describeSchedule, estimateRunsPerMonth } from "@/lib/compose/schedule";
import { describeDelivery } from "@/lib/compose/delivery";
import { estimate } from "@/lib/compose/estimate";
import { formatCents } from "@/lib/format";
import type { SKU } from "@/lib/types";

interface StepReviewProps {
  state: ComposeState;
  setState: React.Dispatch<React.SetStateAction<ComposeState>>;
  skus: ReadonlyArray<SKU>;
}

/**
 * Step 4 — plain-language confirmation of what's about to be hired.
 *
 * No new knobs except an editable name and a "save as template" toggle.
 * Everything the user is about to do is restated in prose so they can
 * spot mistakes before paying.
 */
export function StepReview({ state, setState, skus }: StepReviewProps) {
  const arche = getArchetype(state.archetype);
  const est = estimate(state, skus);
  const prompt = resolvePrompt(state);
  const synthName = resolveJobName(state);
  const runs = estimateRunsPerMonth(state.schedule);
  const monthlyLow = est.centsLow * runs;
  const monthlyHigh = est.centsHigh * runs;
  const recurring = state.schedule.kind !== "once";

  return (
    <div className="flex flex-col gap-8">
      {/* Editable name. */}
      <section className="flex flex-col gap-2">
        <label className="readout-label">Job name</label>
        <input
          type="text"
          maxLength={120}
          value={state.name ?? synthName}
          onChange={(e) =>
            setState((prev) => ({ ...prev, name: e.target.value }))
          }
          className="h-11 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-base font-sans text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
        />
        <span className="text-xs text-[var(--color-ink-dim)]">
          Shown in your Jobs list. You can re-hire this job from there.
        </span>
      </section>

      {/* Plain-language summary. */}
      <section className="border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.25)] p-5 flex flex-col gap-3">
        <span className="readout-label">Summary</span>
        <p
          className="italic text-lg leading-snug text-[var(--color-ink)]"
          style={{ fontFamily: "var(--font-display)" }}
        >
          “{arche.name}”{" "}
          <span className="text-[var(--color-ink-dim)]">—</span>{" "}
          {describeSchedule(state.schedule).toLowerCase().replace(/\.$/, "")}.{" "}
          {describeDelivery(state.delivery)
            .toLowerCase()
            .replace(/\.$/, "")}{" "}
          <span className="text-[var(--color-ink-dim)]">·</span>{" "}
          Quality dial:{" "}
          <span className="text-[var(--color-stop-fill)]">{state.level}</span>.
        </p>
        <div className="border-t border-[var(--color-rule)] pt-3 mt-1">
          <span className="readout-label block mb-2">The agent will see</span>
          <pre className="text-xs font-mono whitespace-pre-wrap text-[var(--color-ink-dim)] leading-relaxed max-h-40 overflow-auto">
            {prompt || "(empty — go back and add details)"}
          </pre>
        </div>
      </section>

      {/* Cost projection. */}
      <section className="border border-[var(--color-rule)] rounded-sm p-5">
        <span className="readout-label">Cost</span>
        <div className="mt-3 flex flex-col gap-1">
          <div className="flex items-baseline justify-between">
            <span className="text-sm text-[var(--color-ink-dim)]">
              Per run
            </span>
            <span className="readout-numeric text-base text-[var(--color-ink)]">
              {formatCents(est.centsLow)} – {formatCents(est.centsHigh)}
            </span>
          </div>
          {recurring && (
            <div className="flex items-baseline justify-between">
              <span className="text-sm text-[var(--color-ink-dim)]">
                Projected per month ({runs} runs)
              </span>
              <span className="readout-numeric text-base text-[var(--color-ink)]">
                {formatCents(monthlyLow)} – {formatCents(monthlyHigh)}
              </span>
            </div>
          )}
          <div className="flex items-baseline justify-between mt-1 pt-2 border-t border-[var(--color-rule)]">
            <span className="text-sm text-[var(--color-ink-dim)]">Tier</span>
            <span className="readout-numeric text-base text-[var(--color-stop-fill)]">
              {est.tier.toUpperCase()}
            </span>
          </div>
        </div>
      </section>

      {/* Save as template. */}
      <section className="border border-[var(--color-rule)] rounded-sm p-5 flex items-start gap-3">
        <label className="inline-flex items-start gap-3 cursor-pointer select-none flex-1">
          <input
            type="checkbox"
            className="sr-only"
            checked={state.saveAsTemplate}
            onChange={(e) =>
              setState((prev) => ({
                ...prev,
                saveAsTemplate: e.target.checked,
              }))
            }
          />
          <span
            className={[
              "mt-0.5 relative inline-block w-10 h-5 rounded-full transition-colors shrink-0",
              state.saveAsTemplate
                ? "bg-[var(--color-stop-fill)]"
                : "bg-[var(--color-rule)]",
            ].join(" ")}
          >
            <span
              className={[
                "absolute top-0.5 size-4 rounded-full bg-[var(--color-ink)] transition-transform",
                state.saveAsTemplate ? "translate-x-5" : "translate-x-0.5",
              ].join(" ")}
            />
          </span>
          <span className="flex flex-col gap-1">
            <span className="readout-label">Save as template</span>
            <span className="text-sm text-[var(--color-ink-dim)] leading-snug">
              Appears in your Jobs list so you can re-hire it with one click,
              or change the schedule later.
            </span>
          </span>
        </label>
      </section>
    </div>
  );
}
