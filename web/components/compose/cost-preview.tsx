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

export function CostPreview({ estimate, className }: CostPreviewProps) {
  return (
    <div
      className={
        "grid grid-cols-3 gap-3 text-[10px] font-mono uppercase tracking-wide " +
        (className ?? "")
      }
    >
      <Cell label="Estimated cost">
        <span className="text-[var(--color-foreground)] text-sm normal-case tracking-normal">
          {formatCents(estimate.centsLow)}
          <span className="text-[var(--color-muted-foreground)]"> – </span>
          {formatCents(estimate.centsHigh)}
        </span>
      </Cell>
      <Cell label="Typical latency">
        <span className="text-[var(--color-foreground)] text-sm normal-case tracking-normal">
          {formatLatency(estimate.p50ms)}
          <span className="text-[var(--color-muted-foreground)]">
            {" · p95 "}
          </span>
          {formatLatency(estimate.p95ms)}
        </span>
      </Cell>
      <Cell label="Tier">
        <span className="text-[var(--color-foreground)] text-sm">
          {TIER_LABEL[estimate.tier]}
        </span>
      </Cell>
    </div>
  );
}

function Cell({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5 border border-[var(--color-border)] rounded-md bg-[var(--color-card)] px-3 py-2">
      <span className="text-[var(--color-muted-foreground)]">{label}</span>
      {children}
    </div>
  );
}
