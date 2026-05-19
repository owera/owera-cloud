// JSON Schema for the agent-programmable compose surface.
//
// Served at /api/compose/schema as application/schema+json. The same shape is
// validated server-side on POST /api/compose. Hand-rolled to avoid a new dep —
// keep this file the single source of schema truth.

import { ARCHETYPES } from "./archetypes";
import { COMPLEXITY_LEVELS } from "./levels";

const ARCHETYPE_IDS = ARCHETYPES.map((a) => a.id);

export const COMPOSE_JSON_SCHEMA = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  $id: "https://app.owera.ai/api/compose/schema",
  title: "Owera Compose Request",
  description:
    "Hire an Owera Agentic job. The same shape is rendered by the composer UI and accepted by the API — humans and agents exercise an identical code path.",
  type: "object",
  required: ["level", "sku", "prompt"],
  properties: {
    archetype: {
      enum: [...ARCHETYPE_IDS],
      description:
        "Job archetype. Optional — defaults to 'custom' for open prompts. The composer UI uses the archetype to render a structured form.",
    },
    inputs: {
      type: "object",
      additionalProperties: true,
      description:
        "Structured archetype inputs (keyed by field id). Shape varies per archetype; see /docs/concepts/anatomy-of-a-job.",
    },
    level: {
      enum: [...COMPLEXITY_LEVELS],
      description:
        "Quality dial. Maps 1:1 to the slider in the composer's Details step.",
    },
    sku: {
      type: "string",
      pattern: "^[a-z0-9-]+(@v[0-9]+)?$",
      description: "Catalog SKU slug, optionally pinned with @vN.",
    },
    prompt: {
      type: "string",
      minLength: 1,
      maxLength: 8000,
      description:
        "Final natural-language task the agent sees. The composer synthesizes this from archetype inputs; agents may pass it directly.",
    },
    tools: {
      type: "array",
      items: { type: "string" },
      uniqueItems: true,
    },
    budget: {
      type: "object",
      properties: {
        max_cents: { type: "integer", minimum: 0 },
        max_latency_ms: { type: "integer", minimum: 0 },
      },
      additionalProperties: false,
    },
    schedule: {
      type: "object",
      required: ["kind"],
      properties: {
        kind: { enum: ["once", "daily", "weekly", "cron"] },
        time: {
          type: "string",
          pattern: "^([0-1][0-9]|2[0-3]):[0-5][0-9]$",
          description: "Local HH:mm — used by daily and weekly.",
        },
        weekdays: {
          type: "array",
          items: { type: "integer", minimum: 0, maximum: 6 },
        },
        cron: {
          type: "string",
          description: "Five-field cron expression. Required when kind=cron.",
        },
        timezone: {
          type: "string",
          description: "IANA timezone, e.g. 'America/Sao_Paulo'.",
        },
      },
      additionalProperties: false,
    },
    delivery: {
      type: "object",
      required: ["kind"],
      properties: {
        kind: { enum: ["dashboard", "email", "slack", "webhook"] },
        target: {
          type: "string",
          description:
            "Address / channel / URL — required for email, slack, webhook.",
        },
      },
      additionalProperties: false,
    },
    name: { type: "string", maxLength: 120 },
    save_as_template: { type: "boolean" },
    idempotency_key: { type: "string" },
  },
  additionalProperties: false,
} as const;

export interface ValidationIssue {
  path: string;
  message: string;
}

export function validateComposeJson(
  body: unknown,
):
  | { ok: true; value: import("./state").ComposeJson }
  | { ok: false; issues: ValidationIssue[] } {
  const issues: ValidationIssue[] = [];
  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    return {
      ok: false,
      issues: [{ path: "$", message: "body must be a JSON object" }],
    };
  }
  const b = body as Record<string, unknown>;

  if (b.archetype !== undefined) {
    if (typeof b.archetype !== "string" || !ARCHETYPE_IDS.includes(b.archetype as never)) {
      issues.push({
        path: "archetype",
        message: `must be one of ${ARCHETYPE_IDS.join(", ")}`,
      });
    }
  }
  if (b.inputs !== undefined && (typeof b.inputs !== "object" || b.inputs === null || Array.isArray(b.inputs))) {
    issues.push({ path: "inputs", message: "must be an object" });
  }
  if (
    typeof b.level !== "string" ||
    !(COMPLEXITY_LEVELS as readonly string[]).includes(b.level)
  ) {
    issues.push({
      path: "level",
      message: `must be one of ${COMPLEXITY_LEVELS.join(", ")}`,
    });
  }
  if (typeof b.sku !== "string" || !/^[a-z0-9-]+(@v[0-9]+)?$/.test(b.sku)) {
    issues.push({ path: "sku", message: "must match ^[a-z0-9-]+(@v[0-9]+)?$" });
  }
  if (
    typeof b.prompt !== "string" ||
    b.prompt.length < 1 ||
    b.prompt.length > 8000
  ) {
    issues.push({ path: "prompt", message: "string, 1..8000 chars" });
  }
  if (b.tools !== undefined) {
    if (!Array.isArray(b.tools) || !b.tools.every((t) => typeof t === "string")) {
      issues.push({ path: "tools", message: "array of strings" });
    } else if (new Set(b.tools).size !== b.tools.length) {
      issues.push({ path: "tools", message: "must be unique" });
    }
  }
  if (b.budget !== undefined) {
    if (typeof b.budget !== "object" || b.budget === null) {
      issues.push({ path: "budget", message: "must be an object" });
    } else {
      const bg = b.budget as Record<string, unknown>;
      for (const k of ["max_cents", "max_latency_ms"]) {
        if (
          bg[k] !== undefined &&
          (!Number.isInteger(bg[k]) || (bg[k] as number) < 0)
        ) {
          issues.push({ path: `budget.${k}`, message: "non-negative integer" });
        }
      }
      for (const k of Object.keys(bg)) {
        if (!["max_cents", "max_latency_ms"].includes(k)) {
          issues.push({ path: `budget.${k}`, message: "unknown property" });
        }
      }
    }
  }
  if (b.schedule !== undefined) {
    if (typeof b.schedule !== "object" || b.schedule === null) {
      issues.push({ path: "schedule", message: "must be an object" });
    } else {
      const s = b.schedule as Record<string, unknown>;
      if (!["once", "daily", "weekly", "cron"].includes(s.kind as string)) {
        issues.push({
          path: "schedule.kind",
          message: "must be one of once, daily, weekly, cron",
        });
      }
      if (s.time !== undefined) {
        if (typeof s.time !== "string" || !/^([0-1][0-9]|2[0-3]):[0-5][0-9]$/.test(s.time)) {
          issues.push({ path: "schedule.time", message: "HH:mm" });
        }
      }
      if (s.weekdays !== undefined) {
        if (
          !Array.isArray(s.weekdays) ||
          !s.weekdays.every((d) => Number.isInteger(d) && (d as number) >= 0 && (d as number) <= 6)
        ) {
          issues.push({
            path: "schedule.weekdays",
            message: "array of integers 0..6",
          });
        }
      }
      if (s.cron !== undefined && typeof s.cron !== "string") {
        issues.push({ path: "schedule.cron", message: "string" });
      }
      if (s.timezone !== undefined && typeof s.timezone !== "string") {
        issues.push({ path: "schedule.timezone", message: "string" });
      }
      for (const k of Object.keys(s)) {
        if (!["kind", "time", "weekdays", "cron", "timezone"].includes(k)) {
          issues.push({ path: `schedule.${k}`, message: "unknown property" });
        }
      }
    }
  }
  if (b.delivery !== undefined) {
    if (typeof b.delivery !== "object" || b.delivery === null) {
      issues.push({ path: "delivery", message: "must be an object" });
    } else {
      const d = b.delivery as Record<string, unknown>;
      if (!["dashboard", "email", "slack", "webhook"].includes(d.kind as string)) {
        issues.push({
          path: "delivery.kind",
          message: "must be one of dashboard, email, slack, webhook",
        });
      }
      if (d.target !== undefined && typeof d.target !== "string") {
        issues.push({ path: "delivery.target", message: "string" });
      }
      if ((d.kind === "email" || d.kind === "slack" || d.kind === "webhook") && !d.target) {
        issues.push({
          path: "delivery.target",
          message: `required when delivery.kind="${d.kind as string}"`,
        });
      }
      for (const k of Object.keys(d)) {
        if (!["kind", "target"].includes(k)) {
          issues.push({ path: `delivery.${k}`, message: "unknown property" });
        }
      }
    }
  }
  if (b.name !== undefined && (typeof b.name !== "string" || b.name.length > 120)) {
    issues.push({ path: "name", message: "string, ≤120 chars" });
  }
  if (b.save_as_template !== undefined && typeof b.save_as_template !== "boolean") {
    issues.push({ path: "save_as_template", message: "boolean" });
  }
  if (b.idempotency_key !== undefined && typeof b.idempotency_key !== "string") {
    issues.push({ path: "idempotency_key", message: "string" });
  }

  const allowed = new Set([
    "archetype",
    "inputs",
    "level",
    "sku",
    "prompt",
    "tools",
    "budget",
    "schedule",
    "delivery",
    "name",
    "save_as_template",
    "idempotency_key",
  ]);
  for (const k of Object.keys(b)) {
    if (!allowed.has(k)) {
      issues.push({ path: k, message: "unknown property" });
    }
  }

  if (issues.length) return { ok: false, issues };
  return { ok: true, value: body as import("./state").ComposeJson };
}
