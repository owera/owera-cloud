// Compose state machine — v2.
//
// A Job has five concerns:
//   - outcome (archetype)         — what kind of work
//   - recipe  (sku/inputs/level)  — the underlying agent + quality dial
//   - cadence (schedule)          — once or recurring
//   - delivery                    — where output goes
//   - identity (name / template)  — re-hire ergonomics
//
// The URL is still the source of truth for the slider mini-app, but more
// elaborate state lives in JSON only (URLs would get unwieldy for archetype
// inputs). One parser still runs on the server route handler AND the client
// surface — same shape, same validation, same code path.

import {
  type ArchetypeId,
  isArchetypeId,
  archetypeInitialInputs,
  getArchetype,
} from "./archetypes";
import { type JobBlueprint, getJobBlueprint } from "./catalog";
import {
  type Delivery,
  type DeliveryKind,
  isDeliveryKind,
  DELIVERY_DEFAULTS,
} from "./delivery";
import { isComplexityLevel, type ComplexityLevel } from "./levels";
import {
  type Schedule,
  type ScheduleKind,
  isScheduleKind,
  SCHEDULE_DEFAULTS,
} from "./schedule";

export { isComplexityLevel };

export const DEFAULT_LEVEL: ComplexityLevel = "simple";

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

/** v1 inputs payload — keyed string/array/bool from the archetype field set. */
export type ArchetypeInputs = Record<string, string | string[] | boolean>;

export interface ComposeState {
  /** v3: the catalog job id this composition was seeded from (if any). */
  jobId?: string;
  /** Outcome — the kind of job we are composing. */
  archetype: ArchetypeId;
  /** Structured inputs for the chosen archetype (per field key). */
  inputs: ArchetypeInputs;

  /** Recipe — underlying SKU + Quality dial + tools + budget. */
  level: ComplexityLevel;
  sku: string;
  tools: string[];
  budget: ComposeBudget;

  /** Cadence — when to run. */
  schedule: Schedule;
  /** Delivery — where output lands. */
  delivery: Delivery;

  /** Identity — short name; if absent we synthesize one from the inputs. */
  name?: string;
  /** Save this composition as a template for later re-hire. */
  saveAsTemplate: boolean;

  /** Legacy free-form prompt (kept for the custom archetype + URL share). */
  prompt: string;

  idempotencyKey?: string;
}

/** Build the canonical "empty" state for a given archetype. */
export function defaultsForArchetype(id: ArchetypeId): ComposeState {
  const a = getArchetype(id);
  const recipe = defaultsForStop(a.defaultLevel);
  return {
    archetype: id,
    inputs: archetypeInitialInputs(id),
    level: a.defaultLevel,
    sku: a.defaultSku,
    tools: recipe.tools,
    budget: recipe.budget,
    schedule:
      a.suggestedCadence === "daily"
        ? SCHEDULE_DEFAULTS.daily
        : a.suggestedCadence === "weekly"
          ? SCHEDULE_DEFAULTS.weekly
          : SCHEDULE_DEFAULTS.once,
    delivery: DELIVERY_DEFAULTS.dashboard,
    saveAsTemplate: a.suggestedCadence !== "once",
    prompt: "",
  };
}

/** Build the canonical "empty" recipe for a given Quality stop. */
export function defaultsForStop(level: ComplexityLevel): {
  level: ComplexityLevel;
  sku: string;
  tools: string[];
  budget: ComposeBudget;
  prompt: string;
} {
  const base = {
    level,
    sku: DEFAULT_SKU_BY_LEVEL[level],
    tools: [] as string[],
    budget: {} as ComposeBudget,
    prompt: "",
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

/** Resolve the primary natural-language prompt the agent should see. */
export function resolvePrompt(state: ComposeState): string {
  // Explicit prompt always wins (custom archetype path).
  if (state.prompt && state.prompt.trim().length > 0) return state.prompt;
  // Otherwise prefer the archetype's `isPrimaryPrompt` field, then build
  // a structured string from the remaining inputs so the underlying agent
  // gets a coherent task description.
  const arche = getArchetype(state.archetype);
  const primaryField = arche.fields.find((f) => f.isPrimaryPrompt);
  const primary = primaryField
    ? String(state.inputs[primaryField.key] ?? "")
    : "";
  const lines: string[] = [];
  if (primary) lines.push(primary.trim());
  for (const f of arche.fields) {
    if (f.isPrimaryPrompt) continue;
    const v = state.inputs[f.key];
    if (v === undefined || v === "" || (Array.isArray(v) && v.length === 0)) {
      continue;
    }
    const display = Array.isArray(v) ? v.join(", ") : String(v);
    lines.push(`${f.label}: ${display}`);
  }
  return lines.join("\n\n").trim();
}

/** Pick the v2 archetype that best matches a catalog blueprint's action verb.
 *  v2 archetypes are kept as the "shape" layer that drives the wizard fields;
 *  the catalog is the "outcome" layer the user actually browses. */
function blueprintToArchetype(b: JobBlueprint): ArchetypeId {
  switch (b.action.verb) {
    case "research":
    case "cluster":
    case "summarize":
      return "research";
    case "classify":
    case "monitor":
      // Watch and triage diverge on whether output goes to a person or a queue.
      if (b.output.kind === "alert" || b.output.kind === "slack") return "watch";
      return "triage";
    case "draft":
      // Briefs go out on cadence; everything else is custom/research.
      if (b.trigger.kind === "schedule") return "brief";
      return "research";
    case "send":
    case "coordinate":
    case "update":
    case "reconcile":
    case "escalate":
      return "custom";
  }
}

/** Synthesize a short job name from inputs when the user hasn't supplied one. */
export function resolveJobName(state: ComposeState): string {
  if (state.name && state.name.trim()) return state.name.trim();
  const arche = getArchetype(state.archetype);
  const primaryField = arche.fields.find((f) => f.isPrimaryPrompt);
  const primary = primaryField
    ? String(state.inputs[primaryField.key] ?? "").trim()
    : state.prompt.trim();
  if (primary) {
    const short = primary.split(/[.!?\n]/)[0]?.slice(0, 60) ?? primary.slice(0, 60);
    return `${arche.name} — ${short}`;
  }
  return arche.name;
}

/* ---------------------------- URL <-> state ---------------------------- */

export function parseFromSearchParams(
  params: URLSearchParams | Record<string, string | string[] | undefined>,
): ComposeState {
  const get = (key: string): string | undefined => {
    if (params instanceof URLSearchParams) return params.get(key) ?? undefined;
    const raw = params[key];
    if (Array.isArray(raw)) return raw[0];
    return raw;
  };

  // v3: prefer the catalog blueprint if `?job=<id>` is present. The
  // blueprint owns trigger/sources/action/output/approval/memory defaults.
  // The wizard fields stay populated via the archetype that maps to the
  // job's underlying recipe, so step 2 still has structured inputs to show.
  const jobIdParam = get("job");
  if (jobIdParam) {
    const blueprint = getJobBlueprint(jobIdParam);
    if (blueprint) {
      const archetype: ArchetypeId = blueprintToArchetype(blueprint);
      const seeded = defaultsForArchetype(archetype);
      seeded.jobId = blueprint.id;
      // Seed the primary-prompt field with the blueprint's hero prompt so
      // step 2 has real text immediately.
      const archeDef = getArchetype(archetype);
      const primaryField = archeDef.fields.find((f) => f.isPrimaryPrompt);
      if (primaryField) seeded.inputs[primaryField.key] = blueprint.heroPrompt;
      seeded.prompt = blueprint.heroPrompt;
      seeded.name = blueprint.name;
      // Apply level overrides from URL if present.
      const rawLevel = get("level");
      if (isComplexityLevel(rawLevel)) seeded.level = rawLevel;
      return seeded;
    }
  }

  const rawArche = get("archetype");
  const archetype: ArchetypeId = isArchetypeId(rawArche) ? rawArche : "custom";
  const seeded = defaultsForArchetype(archetype);

  const rawLevel = get("level");
  if (isComplexityLevel(rawLevel)) seeded.level = rawLevel;

  const recipe = defaultsForStop(seeded.level);
  seeded.tools = recipe.tools;
  seeded.budget = recipe.budget;

  const sku = get("sku")?.trim();
  if (sku) seeded.sku = sku;

  const prompt = get("prompt") ?? "";
  if (prompt) seeded.prompt = prompt;

  const rawTools = get("tools");
  if (rawTools !== undefined) {
    seeded.tools = rawTools
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
  }

  const maxCentsRaw = get("max_cents");
  if (maxCentsRaw && Number.isFinite(Number(maxCentsRaw))) {
    seeded.budget.maxCents = Math.max(0, Math.floor(Number(maxCentsRaw)));
  }
  const maxLatencyRaw = get("max_latency_ms");
  if (maxLatencyRaw && Number.isFinite(Number(maxLatencyRaw))) {
    seeded.budget.maxLatencyMs = Math.max(0, Math.floor(Number(maxLatencyRaw)));
  }

  const idempotencyKey = get("idempotency_key")?.trim() || undefined;
  if (idempotencyKey) seeded.idempotencyKey = idempotencyKey;

  return seeded;
}

/** Emit URL search params — only the slider-relevant subset round-trips here.
 *
 * Larger state (archetype inputs, schedule, delivery) stays in form state and
 * gets posted as JSON. We keep URLs human-shareable instead of stuffing
 * everything into them. */
export function toSearchParams(state: ComposeState): URLSearchParams {
  const out = new URLSearchParams();
  if (state.archetype !== "custom") out.set("archetype", state.archetype);
  out.set("level", state.level);
  const recipe = defaultsForStop(state.level);
  if (state.sku && state.sku !== recipe.sku) out.set("sku", state.sku);
  if (state.prompt) out.set("prompt", state.prompt);
  const toolsSorted = [...state.tools].sort();
  const recipeToolsSorted = [...recipe.tools].sort();
  if (toolsSorted.join(",") !== recipeToolsSorted.join(",")) {
    if (toolsSorted.length) out.set("tools", toolsSorted.join(","));
    else out.set("tools", "");
  }
  if (
    state.budget.maxCents !== undefined &&
    state.budget.maxCents !== recipe.budget.maxCents
  ) {
    out.set("max_cents", String(state.budget.maxCents));
  }
  if (
    state.budget.maxLatencyMs !== undefined &&
    state.budget.maxLatencyMs !== recipe.budget.maxLatencyMs
  ) {
    out.set("max_latency_ms", String(state.budget.maxLatencyMs));
  }
  if (state.idempotencyKey) out.set("idempotency_key", state.idempotencyKey);
  return out;
}

/* ------------------------- JSON wire shape ---------------------------- */

export interface ComposeJson {
  archetype?: ArchetypeId;
  inputs?: ArchetypeInputs;
  level: ComplexityLevel;
  sku: string;
  prompt: string;
  tools?: string[];
  budget?: { max_cents?: number; max_latency_ms?: number };
  schedule?: {
    kind: ScheduleKind;
    time?: string;
    weekdays?: number[];
    cron?: string;
    timezone?: string;
  };
  delivery?: { kind: DeliveryKind; target?: string };
  name?: string;
  save_as_template?: boolean;
  idempotency_key?: string;
}

export function toJson(state: ComposeState): ComposeJson {
  const out: ComposeJson = {
    archetype: state.archetype,
    inputs: state.inputs,
    level: state.level,
    sku: state.sku,
    prompt: resolvePrompt(state),
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
  if (state.schedule.kind !== "once") {
    out.schedule = {
      kind: state.schedule.kind,
      ...(state.schedule.time !== undefined ? { time: state.schedule.time } : {}),
      ...(state.schedule.weekdays
        ? { weekdays: [...state.schedule.weekdays] }
        : {}),
      ...(state.schedule.cron !== undefined ? { cron: state.schedule.cron } : {}),
      ...(state.schedule.timezone
        ? { timezone: state.schedule.timezone }
        : {}),
    };
  }
  if (state.delivery.kind !== "dashboard") {
    out.delivery = {
      kind: state.delivery.kind,
      ...(state.delivery.target ? { target: state.delivery.target } : {}),
    };
  }
  if (state.name) out.name = state.name;
  if (state.saveAsTemplate) out.save_as_template = true;
  if (state.idempotencyKey) out.idempotency_key = state.idempotencyKey;
  return out;
}

export function fromJson(json: ComposeJson): ComposeState {
  const archetype: ArchetypeId = isArchetypeId(json.archetype)
    ? json.archetype
    : "custom";
  const seeded = defaultsForArchetype(archetype);
  if (isComplexityLevel(json.level)) seeded.level = json.level;
  const recipe = defaultsForStop(seeded.level);
  seeded.tools = recipe.tools;
  seeded.budget = recipe.budget;
  if (json.sku?.trim()) seeded.sku = json.sku.trim();
  seeded.prompt = json.prompt ?? "";
  if (json.inputs && typeof json.inputs === "object") {
    seeded.inputs = { ...seeded.inputs, ...json.inputs };
  }
  if (Array.isArray(json.tools)) seeded.tools = json.tools.filter(Boolean);
  if (json.budget?.max_cents !== undefined)
    seeded.budget.maxCents = json.budget.max_cents;
  if (json.budget?.max_latency_ms !== undefined)
    seeded.budget.maxLatencyMs = json.budget.max_latency_ms;
  if (json.schedule && isScheduleKind(json.schedule.kind)) {
    seeded.schedule = {
      kind: json.schedule.kind,
      time: json.schedule.time,
      weekdays: json.schedule.weekdays,
      cron: json.schedule.cron,
      timezone: json.schedule.timezone,
    };
  }
  if (json.delivery && isDeliveryKind(json.delivery.kind)) {
    seeded.delivery = {
      kind: json.delivery.kind,
      target: json.delivery.target,
    };
  }
  if (typeof json.name === "string") seeded.name = json.name;
  if (typeof json.save_as_template === "boolean")
    seeded.saveAsTemplate = json.save_as_template;
  if (json.idempotency_key) seeded.idempotencyKey = json.idempotency_key;
  return seeded;
}

/* ------------------ Adapter: state -> JobCreate body ------------------ */

export function composeStateToJobCreate(state: ComposeState): {
  sku: string;
  inputs: Record<string, unknown>;
  idempotencyKey?: string;
} {
  const prompt = resolvePrompt(state);
  const inputs: Record<string, unknown> = {
    prompt,
    owera_level: state.level,
    owera_archetype: state.archetype,
  };
  // Surface structured inputs so the upstream agent can use them
  // without parsing the rendered prompt string.
  if (state.archetype !== "custom") {
    inputs.owera_archetype_inputs = state.inputs;
  }
  if (state.tools.length) inputs.tools = state.tools;
  if (state.budget.maxCents !== undefined)
    inputs.max_cents = state.budget.maxCents;
  if (state.budget.maxLatencyMs !== undefined)
    inputs.max_latency_ms = state.budget.maxLatencyMs;
  if (state.schedule.kind !== "once") {
    inputs.owera_schedule = {
      kind: state.schedule.kind,
      time: state.schedule.time,
      weekdays: state.schedule.weekdays,
      cron: state.schedule.cron,
      timezone: state.schedule.timezone,
    };
  }
  if (state.delivery.kind !== "dashboard") {
    inputs.owera_delivery = {
      kind: state.delivery.kind,
      target: state.delivery.target,
    };
  }
  const name = resolveJobName(state);
  if (name) inputs.owera_name = name;
  if (state.saveAsTemplate) inputs.owera_save_as_template = true;
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
