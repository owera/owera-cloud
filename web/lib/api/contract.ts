// Bridge between the generated OpenAPI schema (snake_case) and the
// camelCase UI types in lib/types.ts.
//
// WS-14 owns api/openapi.yaml; we pin a snapshot at web/openapi.snapshot.yaml
// and run ./scripts/generate-api-client.sh to refresh lib/api/generated.ts.
// Everything in the UI consumes the camelCase shapes from lib/types.ts —
// adapters live here so divergence between the wire format and the UI is
// localised to a single file.

import type { components } from "./generated";
import type {
  Job,
  JobLedgerEntry,
  JobState,
  UsageMeter,
  ApiKey,
} from "../types";

export type WireJob = components["schemas"]["Job"];
export type WireJobList = components["schemas"]["JobList"];
export type WireJobCreate = components["schemas"]["JobCreate"];
export type WireJobCreated = components["schemas"]["JobCreated"];
export type WireSKU = components["schemas"]["SKU"];
export type WireUsage = components["schemas"]["Usage"];
export type WireError = components["schemas"]["Error"];

const KNOWN_STATES: ReadonlyArray<JobState> = [
  "submitted",
  "queued",
  "running",
  "succeeded",
  "failed",
  "cancelled",
];

function asState(s: string): JobState {
  return (KNOWN_STATES as readonly string[]).includes(s)
    ? (s as JobState)
    : "submitted";
}

export function adaptJob(w: WireJob): Job {
  const outputs = w.outputs ?? null;
  const inputSummary =
    typeof outputs?.input_summary === "string"
      ? outputs.input_summary
      : `(${w.sku})`;
  const cost = typeof outputs?.cost_cents === "number" ? outputs.cost_cents : 0;
  const nodes = Array.isArray(outputs?.assigned_nodes)
    ? (outputs.assigned_nodes as string[])
    : [];
  return {
    id: w.id,
    tenantId:
      typeof outputs?.tenant_id === "string" ? outputs.tenant_id : "self",
    skuSlug: w.sku,
    state: asState(w.status),
    inputSummary,
    submittedAt: w.submitted_at,
    startedAt:
      typeof outputs?.started_at === "string" ? outputs.started_at : null,
    finishedAt:
      w.status === "succeeded" || w.status === "failed" || w.status === "cancelled"
        ? w.updated_at
        : null,
    costCents: cost,
    assignedNodes: nodes,
    error: w.error ?? null,
  };
}

export function adaptUsage(w: WireUsage): UsageMeter {
  const meters = w.meters ?? {};
  let total = 0;
  let jobs = 0;
  const bySku: Record<string, { jobs: number; costCents: number }> = {};
  for (const [k, v] of Object.entries(meters)) {
    if (typeof v !== "number") continue;
    if (k === "total_cost_cents") {
      total = v;
    } else if (k === "total_jobs") {
      jobs = v;
    } else if (k.endsWith(":cost_cents")) {
      const sku = k.slice(0, -":cost_cents".length);
      bySku[sku] = { jobs: bySku[sku]?.jobs ?? 0, costCents: v };
    } else if (k.endsWith(":jobs")) {
      const sku = k.slice(0, -":jobs".length);
      bySku[sku] = { jobs: v, costCents: bySku[sku]?.costCents ?? 0 };
    }
  }
  const now = new Date();
  const periodStart = new Date(now.getFullYear(), now.getMonth(), 1).toISOString();
  const periodEnd = new Date(now.getFullYear(), now.getMonth() + 1, 0).toISOString();
  return {
    periodStart,
    periodEnd,
    totalCostCents: total,
    jobsCount: jobs,
    bySku,
  };
}

// API-key shapes are not in the WS-14 OpenAPI yet — we shape them from a
// minimal expected envelope. WS-15 owns the upstream contract.
interface WireApiKey {
  id: string;
  name: string;
  last_four: string;
  created_at: string;
  last_used_at: string | null;
  scopes: ApiKey["scopes"];
  revoked_at: string | null;
}

export function adaptApiKey(w: WireApiKey): ApiKey {
  return {
    id: w.id,
    name: w.name,
    lastFour: w.last_four,
    createdAt: w.created_at,
    lastUsedAt: w.last_used_at,
    scopes: w.scopes,
    revokedAt: w.revoked_at,
  };
}

// Job ledger entries are not in the OpenAPI yet either; this matches the
// shape we expect WS-14 to expose at /v1/jobs/{id}/events (SSE) and
// /v1/jobs/{id}/ledger (REST).
interface WireLedgerEntry {
  id: string;
  job_id: string;
  ts: string;
  kind: JobLedgerEntry["kind"];
  message: string;
  data?: Record<string, unknown> | null;
}

export function adaptLedgerEntry(w: WireLedgerEntry): JobLedgerEntry {
  return {
    id: w.id,
    jobId: w.job_id,
    ts: w.ts,
    kind: w.kind,
    message: w.message,
    data: w.data ?? null,
  };
}
