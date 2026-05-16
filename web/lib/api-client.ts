// Single thin fetch wrapper. All API calls in the app go through this module —
// nothing else should reference NEXT_PUBLIC_API_URL or hardcode an absolute URL.

import { getApiToken } from "./auth";
import type {
  ApiError,
  ApiKey,
  Job,
  JobLedgerEntry,
  SKU,
  UsageMeter,
} from "./types";

const DEFAULT_BASE = "http://localhost:8080";

function baseUrl(): string {
  return process.env.NEXT_PUBLIC_API_URL ?? DEFAULT_BASE;
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
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const token = opts.token ?? (await getApiToken());
  const headers: Record<string, string> = {
    accept: "application/json",
  };
  if (token) headers.authorization = `Bearer ${token}`;
  if (opts.body !== undefined) headers["content-type"] = "application/json";

  const res = await fetch(`${baseUrl()}${path}`, {
    method: opts.method ?? "GET",
    headers,
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    signal: opts.signal,
    // Always go to the network — the upstream API owns caching.
    cache: "no-store",
  });

  if (!res.ok) {
    let body: ApiError;
    try {
      body = (await res.json()) as ApiError;
    } catch {
      body = { code: "unknown", message: res.statusText };
    }
    throw new ApiClientError(res.status, body);
  }

  // 204 No Content
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/* ----------------------------- public surface --------------------------- */

export const api = {
  /** GET /v1/jobs?limit=&state= */
  listJobs(params: { limit?: number; state?: string } = {}): Promise<Job[]> {
    const qs = new URLSearchParams();
    if (params.limit) qs.set("limit", String(params.limit));
    if (params.state) qs.set("state", params.state);
    const suffix = qs.toString() ? `?${qs.toString()}` : "";
    return request<Job[]>(`/v1/jobs${suffix}`);
  },
  /** GET /v1/jobs/:id */
  getJob(id: string): Promise<Job> {
    return request<Job>(`/v1/jobs/${encodeURIComponent(id)}`);
  },
  /** GET /v1/jobs/:id/ledger */
  getJobLedger(id: string): Promise<JobLedgerEntry[]> {
    return request<JobLedgerEntry[]>(
      `/v1/jobs/${encodeURIComponent(id)}/ledger`,
    );
  },
  /** GET /v1/skus */
  listSkus(): Promise<SKU[]> {
    return request<SKU[]>("/v1/skus");
  },
  /** GET /v1/usage/current */
  getCurrentUsage(): Promise<UsageMeter> {
    return request<UsageMeter>("/v1/usage/current");
  },
  /** GET /v1/api-keys */
  listApiKeys(): Promise<ApiKey[]> {
    return request<ApiKey[]>("/v1/api-keys");
  },
  /** POST /v1/api-keys */
  createApiKey(input: { name: string; scopes: ApiKey["scopes"] }): Promise<
    ApiKey & { secret: string }
  > {
    return request<ApiKey & { secret: string }>("/v1/api-keys", {
      method: "POST",
      body: input,
    });
  },
  /** DELETE /v1/api-keys/:id */
  revokeApiKey(id: string): Promise<void> {
    return request<void>(`/v1/api-keys/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
  },
};

export type Api = typeof api;
