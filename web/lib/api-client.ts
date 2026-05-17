// Single thin fetch wrapper. All API calls in the app go through this module —
// nothing else should reference NEXT_PUBLIC_API_URL or hardcode an absolute URL.
//
// Routing model: both server and browser callers go through the in-app
// /api/proxy/* route, which attaches the upstream bearer token server-side.
// The client never sees raw API keys.

import { getApiToken } from "./auth";
import {
  adaptApiKey,
  adaptJob,
  adaptLedgerEntry,
  adaptUsage,
  type WireJob,
  type WireJobCreate,
  type WireJobCreated,
  type WireJobList,
  type WireSKU,
  type WireUsage,
} from "./api/contract";
import type {
  ApiError,
  ApiKey,
  Job,
  JobLedgerEntry,
  SKU,
  UsageMeter,
} from "./types";

const DEFAULT_BASE = "http://localhost:8080";

function isServer(): boolean {
  return typeof window === "undefined";
}

function baseUrl(): string {
  // Server components/route handlers hit the upstream API directly so they
  // can attach the bearer token server-side. The browser must go through the
  // /api/proxy/* edge to avoid exposing the token client-side.
  if (isServer()) return process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_BASE;
  return "/api/proxy";
}

export class ApiClientError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | undefined;
  constructor(status: number, body: ApiError) {
    super(body.message);
    this.status = status;
    this.code = body.code;
    this.requestId = body.requestId;
  }
}

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  /** Override the default token (mostly for tests). */
  token?: string | null;
  /** Forwarded to fetch(); use sparingly. */
  signal?: AbortSignal;
  /** Forwarded as Idempotency-Key for POST. */
  idempotencyKey?: string;
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {
    accept: "application/json",
  };
  // Token is attached server-side; browser fetches via /api/proxy where the
  // proxy route injects it. Server-side callers attach it here.
  if (isServer()) {
    const token = opts.token ?? (await getApiToken());
    if (token) headers.authorization = `Bearer ${token}`;
  }
  if (opts.body !== undefined) headers["content-type"] = "application/json";
  if (opts.idempotencyKey) headers["idempotency-key"] = opts.idempotencyKey;

  const res = await fetch(`${baseUrl()}${path}`, {
    method: opts.method ?? "GET",
    headers,
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    signal: opts.signal,
    // The upstream API owns caching.
    cache: "no-store",
    credentials: isServer() ? undefined : "same-origin",
  });

  if (!res.ok) {
    let body: ApiError;
    try {
      const raw = (await res.json()) as { error?: string; detail?: string; code?: string; message?: string; requestId?: string };
      body = {
        code: raw.code ?? raw.error ?? "unknown",
        message: raw.message ?? raw.detail ?? raw.error ?? res.statusText,
        requestId: raw.requestId,
      };
    } catch {
      body = { code: "unknown", message: res.statusText };
    }
    throw new ApiClientError(res.status, body);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/* ----------------------------- public surface --------------------------- */

interface ListJobsParams {
  limit?: number;
  state?: string;
  cursor?: string;
}

function adaptSku(w: WireSKU): SKU {
  const slug = w.name;
  const base = w.pricing?.base_cents ?? 0;
  const meter = w.billing_meter ?? "job";
  const unit: SKU["unit"] =
    meter === "minute" ? "minute" : meter === "mtokens" ? "mtokens" : "job";
  const category = (w.category ?? "custom") as SKU["category"];
  return {
    id: `${slug}@${w.version}`,
    slug,
    name: slug,
    description: w.sla?.description ?? "",
    unitPriceCents: base,
    unit,
    category,
  };
}

export const api = {
  /** GET /v1/jobs?limit=&status= */
  async listJobs(params: ListJobsParams = {}): Promise<Job[]> {
    const qs = new URLSearchParams();
    if (params.limit) qs.set("limit", String(params.limit));
    if (params.state) qs.set("status", params.state);
    if (params.cursor) qs.set("cursor", params.cursor);
    const suffix = qs.toString() ? `?${qs.toString()}` : "";
    const wire = await request<WireJobList>(`/v1/jobs${suffix}`);
    return (wire.jobs ?? []).map(adaptJob);
  },
  /** GET /v1/jobs/:id */
  async getJob(id: string): Promise<Job> {
    const wire = await request<WireJob>(`/v1/jobs/${encodeURIComponent(id)}`);
    return adaptJob(wire);
  },
  /** GET /v1/jobs/:id/ledger */
  async getJobLedger(id: string): Promise<JobLedgerEntry[]> {
    const wire = await request<{ entries: Parameters<typeof adaptLedgerEntry>[0][] }>(
      `/v1/jobs/${encodeURIComponent(id)}/ledger`,
    );
    return (wire.entries ?? []).map(adaptLedgerEntry);
  },
  /** POST /v1/jobs */
  async submitJob(input: {
    sku: string;
    inputs: Record<string, unknown>;
    idempotencyKey?: string;
  }): Promise<{ jobId: string; status: string }> {
    const body: WireJobCreate = {
      sku: input.sku,
      inputs: input.inputs,
      idempotency_key: input.idempotencyKey ?? null,
    };
    const wire = await request<WireJobCreated>("/v1/jobs", {
      method: "POST",
      body,
      idempotencyKey: input.idempotencyKey,
    });
    return { jobId: wire.job_id, status: wire.status };
  },
  /** POST /v1/jobs/:id/cancel */
  cancelJob(id: string): Promise<{ status: string }> {
    return request<{ status: string }>(
      `/v1/jobs/${encodeURIComponent(id)}/cancel`,
      { method: "POST" },
    );
  },
  /** GET /v1/skus */
  async listSkus(): Promise<SKU[]> {
    const wire = await request<{ skus?: WireSKU[] }>("/v1/skus");
    return (wire.skus ?? []).map(adaptSku);
  },
  /** GET /v1/usage?period=current */
  async getCurrentUsage(): Promise<UsageMeter> {
    const wire = await request<WireUsage>("/v1/usage?period=current");
    return adaptUsage(wire);
  },
  /** GET /v1/api-keys (WS-15 contract) */
  async listApiKeys(): Promise<ApiKey[]> {
    const wire = await request<{ keys?: Parameters<typeof adaptApiKey>[0][] }>(
      "/v1/api-keys",
    );
    return (wire.keys ?? []).map(adaptApiKey);
  },
  /** POST /v1/api-keys (WS-15 contract) — returns the plaintext secret once. */
  async createApiKey(input: {
    name: string;
    scopes: ApiKey["scopes"];
  }): Promise<ApiKey & { secret: string }> {
    const wire = await request<
      Parameters<typeof adaptApiKey>[0] & { secret: string }
    >("/v1/api-keys", {
      method: "POST",
      body: { name: input.name, scopes: input.scopes },
    });
    return { ...adaptApiKey(wire), secret: wire.secret };
  },
  /** DELETE /v1/api-keys/:id */
  revokeApiKey(id: string): Promise<void> {
    return request<void>(`/v1/api-keys/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
  },
  /** POST /v1/billing/portal (WS-16) — server-side preferred. */
  openBillingPortal(): Promise<{ url: string }> {
    return request<{ url: string }>("/v1/billing/portal", { method: "POST" });
  },
  /** GET /v1/support/tickets */
  listTickets(): Promise<Ticket[]> {
    return request<{ tickets: Ticket[] }>("/v1/support/tickets").then(
      (r) => r.tickets ?? [],
    );
  },
  /** GET /v1/support/tickets/:id */
  getTicket(id: string): Promise<TicketDetail> {
    return request<TicketDetail>(
      `/v1/support/tickets/${encodeURIComponent(id)}`,
    );
  },
  /** POST /v1/support/tickets */
  createTicket(input: {
    subject: string;
    body: string;
  }): Promise<Ticket> {
    return request<Ticket>("/v1/support/tickets", {
      method: "POST",
      body: input,
    });
  },
  /** POST /v1/support/tickets/:id/messages */
  postTicketMessage(
    id: string,
    body: string,
  ): Promise<{ id: string; ts: string }> {
    return request<{ id: string; ts: string }>(
      `/v1/support/tickets/${encodeURIComponent(id)}/messages`,
      { method: "POST", body: { body } },
    );
  },
};

export type Api = typeof api;

/* ------------------------------ support types --------------------------- */
// Lightweight ticket shapes. WS-15/WS-18 may move these into the OpenAPI
// contract; until then we own them locally.

export type TicketState = "open" | "pending" | "resolved" | "closed";

export interface Ticket {
  id: string;
  subject: string;
  state: TicketState;
  createdAt: string;
  updatedAt: string;
  /** Author display label of the latest message — "you" or "support". */
  lastAuthor: "customer" | "support";
}

export interface TicketMessage {
  id: string;
  ts: string;
  author: "customer" | "support";
  body: string;
}

export interface TicketDetail extends Ticket {
  messages: TicketMessage[];
}
