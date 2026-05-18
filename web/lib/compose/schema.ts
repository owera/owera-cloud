// JSON Schema for the agent-programmable compose surface.
//
// Served at /api/compose/schema as application/schema+json. The same shape is
// validated server-side on POST /api/compose. Hand-rolled to avoid a new dep —
// keep this file the single source of schema truth.

import { COMPLEXITY_LEVELS } from "./levels";

export const COMPOSE_JSON_SCHEMA = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  $id: "https://app.owera.ai/api/compose/schema",
  title: "Owera Compose Request",
  description:
    "Submit a job to Owera Agentic via the same surface humans use by dragging the complexity slider.",
  type: "object",
  required: ["level", "sku", "prompt"],
  properties: {
    level: {
      enum: [...COMPLEXITY_LEVELS],
      description:
        "Complexity tier. Maps 1:1 to the slider stop. A 'level' alone is enough to materialize a valid job — defaults fill the rest server-side.",
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
      description: "Free-form natural-language instruction to the agent.",
    },
    tools: {
      type: "array",
      items: { type: "string" },
      uniqueItems: true,
      description: "Tools allowlist. Ignored at level=simple.",
    },
    budget: {
      type: "object",
      properties: {
        max_cents: { type: "integer", minimum: 0 },
        max_latency_ms: { type: "integer", minimum: 0 },
      },
      additionalProperties: false,
    },
    idempotency_key: { type: "string" },
  },
  additionalProperties: false,
} as const;

export interface ValidationIssue {
  path: string;
  message: string;
}

/** Lightweight runtime validator — enforces the same rules the schema declares. */
export function validateComposeJson(
  body: unknown,
): { ok: true; value: import("./state").ComposeJson } | { ok: false; issues: ValidationIssue[] } {
  const issues: ValidationIssue[] = [];
  if (typeof body !== "object" || body === null || Array.isArray(body)) {
    return { ok: false, issues: [{ path: "$", message: "body must be a JSON object" }] };
  }
  const b = body as Record<string, unknown>;

  if (typeof b.level !== "string" || !(COMPLEXITY_LEVELS as readonly string[]).includes(b.level)) {
    issues.push({ path: "level", message: `must be one of ${COMPLEXITY_LEVELS.join(", ")}` });
  }
  if (typeof b.sku !== "string" || !/^[a-z0-9-]+(@v[0-9]+)?$/.test(b.sku)) {
    issues.push({ path: "sku", message: "must match ^[a-z0-9-]+(@v[0-9]+)?$" });
  }
  if (typeof b.prompt !== "string" || b.prompt.length < 1 || b.prompt.length > 8000) {
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
        if (bg[k] !== undefined && (!Number.isInteger(bg[k]) || (bg[k] as number) < 0)) {
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
  if (b.idempotency_key !== undefined && typeof b.idempotency_key !== "string") {
    issues.push({ path: "idempotency_key", message: "must be a string" });
  }
  for (const k of Object.keys(b)) {
    if (
      !["level", "sku", "prompt", "tools", "budget", "idempotency_key"].includes(
        k,
      )
    ) {
      issues.push({ path: k, message: "unknown property" });
    }
  }

  if (issues.length) return { ok: false, issues };
  return { ok: true, value: body as import("./state").ComposeJson };
}
