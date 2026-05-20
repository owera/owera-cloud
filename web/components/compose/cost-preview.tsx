"use client";

import * as React from "react";
import type { CostEstimate } from "@/lib/compose/estimate";
import { formatLatency } from "@/lib/compose/estimate";
import { formatCents } from "@/lib/format";

interface CostPreviewProps {
  estimate: CostEstimate;
  className?: string;
}

const TIER_LABEL: Record<CostEstimate["tier"], string> = {
  free: "FREE",
  pro: "PRO",
  team: "TEAM",
  enterprise: "ENTERPRISE",
};

/**
 * Oscilloscope-style instrument readout. Three large numeric cells with
 * column dividers, hairline rules, tabular numerics, and a subtle reveal
 * on value change so the meter feels alive without being noisy.
 */
export function CostPreview({ estimate, className }: CostPreviewProps) {
  return (
    <div
      className={[
        "relative grid grid-cols-3 border-y border-[var(--color-rule)]",
        "divide-x divide-[var(--color-rule)] bg-[rgba(255,255,255,0.01)]",
        className ?? "",
      ].join(" ")}
    >
      <Cell
        label="Estimated cost"
        value={
          <>
            {formatCents(estimate.centsLow)}
            <span className="text-[var(--color-ink-dim)] mx-1">–</span>
            {formatCents(estimate.centsHigh)}
          </>
        }
        key1={estimate.centsLow}
        key2={estimate.centsHigh}
      />
      <Cell
        label="Typical · p95 latency"
        value={
          <>
            {formatLatency(estimate.p50ms)}
            <span className="text-[var(--color-ink-dim)] mx-1">·</span>
            {formatLatency(estimate.p95ms)}
          </>
        }
        key1={estimate.p50ms}
        key2={estimate.p95ms}
      />
      <Cell
        label="Tier"
        value={TIER_LABEL[estimate.tier]}
        key1={estimate.tier as unknown as number}
        key2={0}
      />
    </div>
  );
}

interface CellProps {
  label: string;
  value: React.ReactNode;
  key1: number;
  key2: number;
}

function Cell({ label, value, key1, key2 }: CellProps) {
  // Pulse the value on update — keyed on the actual numbers so it only
  // fires when something changes. Brief, opacity-only, no layout shift.
  const k = `${key1}-${key2}`;
  return (
    <div className="flex flex-col gap-3 px-5 py-4 relative">
      <span className="readout-label">{label}</span>
      <span
        key={k}
        className="readout-numeric text-[1.4rem] sm:text-[1.55rem] leading-none text-[var(--color-ink)]"
        style={{
          animation: "compose-rise 380ms cubic-bezier(0.2, 0.8, 0.2, 1)",
        }}
      >
        {value}
      </span>
      {/* Hairline tick at the bottom — instrument-panel detail. */}
      <span className="absolute bottom-1 left-5 right-5 h-px bg-[var(--color-rule)] opacity-50" />
    </div>
  );
}
