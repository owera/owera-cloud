// Compose state machine.
//
// The URL is the source of truth. ComposeState round-trips between:
//   - URLSearchParams (?level=...&prompt=...&sku=...&tools=...&max_cents=...)
//   - JSON config (POST body to /api/compose)
//   - the JobCreate body consumed by api.submitJob
//
// One parser runs on the server route handler AND the client surface — so the
// human dragging the slider and the agent POSTing JSON exercise an identical
// code path. This is the whole point: no second source of truth, ever.

import { isComplexityLevel, type ComplexityLevel } from "./levels";
// Re-export so existing callers that import from state still work.
export { isComplexityLevel };

export const DEFAULT_LEVEL: ComplexityLevel = "simple";

/** Default SKU each stop seeds with when the user hasn't picked one. */
export const DEFAULT_SKU_BY_LEVEL: Record<ComplexityLevel, string> = {
  simple: "triage-watch",
  standard: "triage-watch",
  advanced: "campaign-swarm",
  expert: "campaign-swarm",
  custom: "campaign-swarm",
};

export interface ComposeBudget {
  maxCents?: number;
  maxLatencyMs?: number;
}

export interface ComposeState {
  level: ComplexityLevel;
  sku: string;
  prompt: string;
  /** Tools toggled on. Empty for simple. */
  tools: string[];
  budget: ComposeBudget;
  /** Optional client-supplied idempotency key. */
  idempotencyKey?: string;
}

/** Build the canonical "empty" state for a given stop. */
export function defaultsForStop(level: ComplexityLevel): ComposeState {
  const base: ComposeState = {
    level,
    sku: DEFAULT_SKU_BY_LEVEL[level],
    prompt: "",
    tools: [],
    budget: {},
  };
  switch (level) {
    case "simple":
      return base;
    case "standard":
      return { ...base, tools: ["web"] };
    case "advanced":
      return {
        ...base,
        tools: ["web", "code"],
        budget: { maxCents: 50, maxLatencyMs: 90_000 },
      };
    case "expert":
      return {
        ...base,
        tools: ["web", "code", "browser", "files"],
        budget: { maxCents: 200, maxLatencyMs: 300_000 },
      };
    case "custom":
      return {
        ...base,
        tools: ["web", "code", "browser", "files"],
        budget: { maxCents: 500, maxLatencyMs: 600_000 },
      };
  }
}

/** Parse URL search params into a ComposeState. Invalid values fall back to defaults. */
export function parseFromSearchParams(
  params: URLSearchParams | Record<string, string | string[] | undefined>,
): ComposeState {
  const get = (key: string): string | undefined => {
    if (params instanceof URLSearchParams) return params.get(key) ?? undefined;
    const raw = params[key];
    if (Array.isArray(raw)) return raw[0];
    return raw;
  };

  const rawLevel = get("level");
  const level: ComplexityLevel = isComplexityLevel(rawLevel)
    ? rawLevel
    : DEFAULT_LEVEL;

  const seeded = defaultsForStop(level);

  const sku = get("sku")?.trim() || seeded.sku;
  const prompt = get("prompt") ?? "";

  const rawTools = get("tools");
  const tools = rawTools
    ? rawTools
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean)
    : seeded.tools;

  const maxCentsRaw = get("max_cents");
  const maxLatencyRaw = get("max_latency_ms");
  const budget: ComposeBudget = {};
  if (maxCentsRaw && Number.isFinite(Number(maxCentsRaw))) {
    budget.maxCents = Math.max(0, Math.floor(Number(maxCentsRaw)));
  } else if (seeded.budget.maxCents !== undefined) {
    budget.maxCents = seeded.budget.maxCents;
  }
  if (maxLatencyRaw && Number.isFinite(Number(maxLatencyRaw))) {
    budget.maxLatencyMs = Math.max(0, Math.floor(Number(maxLatencyRaw)));
  } else if (seeded.budget.maxLatencyMs !== undefined) {
    budget.maxLatencyMs = seeded.budget.maxLatencyMs;
  }

  const idempotencyKey = get("idempotency_key")?.trim() || undefined;

  return { level, sku, prompt, tools, budget, idempotencyKey };
}

/** Emit URL search params. Empty/default fields are omitted to keep URLs short. */
export function toSearchParams(state: ComposeState): URLSearchParams {
  const out = new URLSearchParams();
  out.set("level", state.level);
  const seeded = defaultsForStop(state.level);

  if (state.sku && state.sku !== seeded.sku) out.set("sku", state.sku);
  if (state.prompt) out.set("prompt", state.prompt);

  const toolsSorted = [...state.tools].sort();
  const seededToolsSorted = [...seeded.tools].sort();
  if (toolsSorted.join(",") !== seededToolsSorted.join(",")) {
    if (toolsSorted.length) out.set("tools", toolsSorted.join(","));
    else out.set("tools", "");
  }
  if (
    state.budget.maxCents !== undefined &&
    state.budget.maxCents !== seeded.budget.maxCents
  ) {
    out.set("max_cents", String(state.budget.maxCents));
  }
  if (
    state.budget.maxLatencyMs !== undefined &&
    state.budget.maxLatencyMs !== seeded.budget.maxLatencyMs
  ) {
    out.set("max_latency_ms", String(state.budget.maxLatencyMs));
  }
  if (state.idempotencyKey) out.set("idempotency_key", state.idempotencyKey);
  return out;
}

/** Wire JSON shape exposed at /api/compose and /api/compose/schema. */
export interface ComposeJson {
  level: ComplexityLevel;
  sku: string;
  prompt: string;
  tools?: string[];
  budget?: { max_cents?: number; max_latency_ms?: number };
  idempotency_key?: string;
}

export function toJson(state: ComposeState): ComposeJson {
  const out: ComposeJson = {
    level: state.level,
    sku: state.sku,
    prompt: state.prompt,
  };
  if (state.tools.length) out.tools = [...state.tools].sort();
  if (
    state.budget.maxCents !== undefined ||
    state.budget.maxLatencyMs !== undefined
  ) {
    out.budget = {};
    if (state.budget.maxCents !== undefined)
      out.budget.max_cents = state.budget.maxCents;
    if (state.budget.maxLatencyMs !== undefined)
      out.budget.max_latency_ms = state.budget.maxLatencyMs;
  }
  if (state.idempotencyKey) out.idempotency_key = state.idempotencyKey;
  return out;
}

/** Round-trip from posted JSON back to ComposeState (with defaults). */
export function fromJson(json: ComposeJson): ComposeState {
  const level: ComplexityLevel = isComplexityLevel(json.level)
    ? json.level
    : DEFAULT_LEVEL;
  const seeded = defaultsForStop(level);
  return {
    level,
    sku: json.sku?.trim() || seeded.sku,
    prompt: json.prompt ?? "",
    tools: Array.isArray(json.tools) ? json.tools.filter(Boolean) : seeded.tools,
    budget: {
      maxCents: json.budget?.max_cents ?? seeded.budget.maxCents,
      maxLatencyMs: json.budget?.max_latency_ms ?? seeded.budget.maxLatencyMs,
    },
    idempotencyKey: json.idempotency_key,
  };
}

/** Adapter: ComposeState → JobCreate body for api.submitJob. */
export function composeStateToJobCreate(state: ComposeState): {
  sku: string;
  inputs: Record<string, unknown>;
  idempotencyKey?: string;
} {
  const inputs: Record<string, unknown> = {
    prompt: state.prompt,
    owera_level: state.level,
  };
  if (state.tools.length) inputs.tools = state.tools;
  if (state.budget.maxCents !== undefined)
    inputs.max_cents = state.budget.maxCents;
  if (state.budget.maxLatencyMs !== undefined)
    inputs.max_latency_ms = state.budget.maxLatencyMs;
  return {
    sku: state.sku,
    inputs,
    idempotencyKey: state.idempotencyKey,
  };
}

/** Tier-gate predicate (server-enforced too — never trust the URL alone). */
export function levelRequiresAuth(level: ComplexityLevel): boolean {
  return level === "advanced" || level === "expert" || level === "custom";
}

export function levelRequiresPaidPlan(level: ComplexityLevel): boolean {
  return level === "expert" || level === "custom";
}
