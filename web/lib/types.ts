// SYNC: api/openapi.yaml — regenerate when contract changes
//
// These are hand-written shapes mirroring the OpenAPI spec the api/ agent is
// drafting in parallel. Once api/openapi.yaml stabilises, swap this file for
// a generated client (see .gitignore: web/lib/api/generated/). Until then,
// keep field names + enum values in lock-step with api/openapi.yaml.

export type JobState =
  | "submitted"
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled";

export const JOB_STATES: readonly JobState[] = [
  "submitted",
  "queued",
  "running",
  "succeeded",
  "failed",
  "cancelled",
] as const;

export interface Tenant {
  id: string;
  name: string;
  /** Stripe customer id (cus_...) — null until billing is provisioned. */
  stripeCustomerId: string | null;
  createdAt: string; // ISO-8601
}

export interface User {
  id: string;
  email: string;
  name: string;
  tenantId: string;
  role: "owner" | "admin" | "member";
}

/** A managed-service SKU exposed in the public catalog. */
export interface SKU {
  id: string;
  /** Human slug e.g. "agentic.research" */
  slug: string;
  name: string;
  description: string;
  /** Unit price in USD cents per job (or per million tokens, see `unit`). */
  unitPriceCents: number;
  unit: "job" | "minute" | "mtokens";
  category: "research" | "etl" | "ops" | "build" | "custom";
}

export interface Job {
  id: string;
  tenantId: string;
  skuSlug: string;
  state: JobState;
  /** Free-form job prompt or structured input payload reference. */
  inputSummary: string;
  /** ISO-8601 timestamps. */
  submittedAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  /** Cost so far, in USD cents. */
  costCents: number;
  /** Workers/agents that touched this job (host names). */
  assignedNodes: string[];
  /** Optional terminal-state error message. */
  error: string | null;
}

/** Single ledger entry on a job timeline. */
export interface JobLedgerEntry {
  id: string;
  jobId: string;
  ts: string; // ISO-8601
  kind: "state_change" | "log" | "tool_call" | "output" | "billing";
  message: string;
  /** Free-form structured payload — opaque to the UI. */
  data: Record<string, unknown> | null;
}

export interface ApiKey {
  id: string;
  /** Display label only — never the secret. */
  name: string;
  /** Last 4 chars of the secret, for identification. */
  lastFour: string;
  createdAt: string;
  lastUsedAt: string | null;
  /** Scopes the key is permitted to call. */
  scopes: Array<"jobs.read" | "jobs.write" | "billing.read">;
  revokedAt: string | null;
}

/** Aggregated usage for the current billing period. */
export interface UsageMeter {
  periodStart: string; // ISO-8601
  periodEnd: string;
  totalCostCents: number;
  jobsCount: number;
  /** Per-SKU breakdown — keys are SKU slugs. */
  bySku: Record<string, { jobs: number; costCents: number }>;
}

/** Standard error envelope returned by the API. */
export interface ApiError {
  code: string;
  message: string;
  /** Optional correlation id for support tickets. */
  requestId?: string;
}
