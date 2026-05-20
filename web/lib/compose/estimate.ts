// Pure cost/latency estimator. Same function runs on server (POST /api/compose
// response) and client (live preview as the slider moves) so humans and agents
// see identical numbers.

import type { SKU } from "@/lib/types";
import type { ComposeState, ComposeBudget } from "./state";
import type { ComplexityLevel } from "./levels";

/** Subset of state that the estimator actually needs. Lets callers pass
 *  partial fragments (cost-example doc widget) without minting full states. */
export type EstimateInput = Pick<ComposeState, "level" | "sku" | "tools"> & {
  budget: ComposeBudget;
};

/** Multiplier per stop, applied to the SKU base price. Tuned for trust over hype. */
const COST_FACTOR: Record<ComplexityLevel, number> = {
  simple: 0.4,
  standard: 1.0,
  advanced: 2.5,
  expert: 6.0,
  custom: 10.0,
};

/** Tool count amplifies cost slightly past 2 tools. */
const TOOL_FACTOR_PAST_TWO = 0.15;

const LATENCY_BY_LEVEL: Record<ComplexityLevel, { p50: number; p95: number }> = {
  simple: { p50: 4_000, p95: 12_000 },
  standard: { p50: 12_000, p95: 30_000 },
  advanced: { p50: 30_000, p95: 90_000 },
  expert: { p50: 90_000, p95: 240_000 },
  custom: { p50: 120_000, p95: 480_000 },
};

const TIER_BY_LEVEL: Record<ComplexityLevel, "free" | "pro" | "team" | "enterprise"> = {
  simple: "free",
  standard: "free",
  advanced: "pro",
  expert: "team",
  custom: "enterprise",
};

export interface CostEstimate {
  /** Low end of the predicted cost in USD cents. */
  centsLow: number;
  /** High end of the predicted cost in USD cents. */
  centsHigh: number;
  /** Latency band, milliseconds. */
  p50ms: number;
  p95ms: number;
  /** Pricing tier this stop maps to. */
  tier: "free" | "pro" | "team" | "enterprise";
}

export function estimate(
  state: EstimateInput,
  skus: ReadonlyArray<SKU> | null = null,
): CostEstimate {
  const base = lookupSkuBaseCents(state.sku, skus);
  const factor = COST_FACTOR[state.level];
  const toolBonus =
    state.tools.length > 2 ? (state.tools.length - 2) * TOOL_FACTOR_PAST_TWO : 0;
  const center = base * factor * (1 + toolBonus);

  // Range is ±35% — wide enough that real billed cents almost always lands
  // inside, narrow enough that the preview is still informative.
  const low = Math.max(1, Math.round(center * 0.65));
  const high = Math.max(low + 1, Math.round(center * 1.35));

  const latency = LATENCY_BY_LEVEL[state.level];

  // Hard cap by user-set budget if provided.
  const capped =
    state.budget.maxCents !== undefined
      ? Math.min(high, state.budget.maxCents)
      : high;
  const cappedLow = Math.min(low, capped);

  return {
    centsLow: cappedLow,
    centsHigh: capped,
    p50ms: latency.p50,
    p95ms: latency.p95,
    tier: TIER_BY_LEVEL[state.level],
  };
}

function lookupSkuBaseCents(
  sku: string,
  skus: ReadonlyArray<SKU> | null,
): number {
  if (skus) {
    const slug = sku.split("@")[0];
    const found = skus.find((s) => s.slug === slug || s.id === sku);
    if (found && found.unitPriceCents > 0) return found.unitPriceCents;
  }
  // Conservative fallback when the SKU catalog hasn't loaded yet.
  return 5;
}

/** Format milliseconds as a short human label: "4s", "1m 30s", "5m". */
export function formatLatency(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  const rem = sec % 60;
  if (rem === 0) return `${min}m`;
  return `${min}m ${rem}s`;
}
