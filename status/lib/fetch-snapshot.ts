// Consumer for the operator-plane HealthSnapshot RPC payload.
//
// The status page deliberately does NOT call api.owera.ai/internal/status
// (which would couple "is the API up" to "can the status page render").
// Instead, an operator-plane cron POSTs the snapshot JSON to a public
// object store (Cloudflare R2 / S3); we read it from there.
//
// The shape mirrors `internal/rpc/healthsnapshot.go` in owera-fleet. Field
// names are snake_case to match the Go JSON tags.

export type GatewayHealth = {
  ok: boolean;
  hermes_version: string;
  uptime_seconds: number;
};

export type WorkerHealth = {
  node: string;
  ok: boolean;
  last_heartbeat_age_seconds: number;
  hermes_version: string;
};

export type SKUConformance = {
  sku: string;
  p50_latency_ms: number;
  p95_latency_ms: number;
  error_rate_pct: number;
  sla_target_ms: number;
};

export type HealthSnapshot = {
  ts: string;
  gateway: GatewayHealth;
  workers: WorkerHealth[];
  sku_conformance: SKUConformance[];
};

export type FetchResult =
  | { ok: true; snapshot: HealthSnapshot; fetchedAt: number; stale: boolean }
  | { ok: false; error: string; fetchedAt: number };

// A snapshot older than this is treated as stale — the operator plane writes
// it every 30s, so anything past 5x that window means the writer is dead.
export const STALE_SNAPSHOT_AGE_MS = 150_000;

// Hard cap on fetch time. The status page polls every 30s; a slow snapshot
// store should not block the UI.
const FETCH_TIMEOUT_MS = 8_000;

const FORCED_INCIDENT: HealthSnapshot = {
  ts: new Date(0).toISOString(),
  gateway: {
    ok: false,
    hermes_version: "",
    uptime_seconds: 0,
  },
  workers: [],
  sku_conformance: [],
};

export async function fetchSnapshot(url: string): Promise<FetchResult> {
  if (process.env.NEXT_PUBLIC_FORCE_INCIDENT === "1") {
    return {
      ok: true,
      snapshot: { ...FORCED_INCIDENT, ts: new Date().toISOString() },
      fetchedAt: Date.now(),
      stale: false,
    };
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
  try {
    const res = await fetch(url, {
      cache: "no-store",
      signal: controller.signal,
      headers: { accept: "application/json" },
    });
    if (!res.ok) {
      return {
        ok: false,
        error: `snapshot fetch returned ${res.status}`,
        fetchedAt: Date.now(),
      };
    }
    const snap = (await res.json()) as HealthSnapshot;
    if (!snap.ts || !snap.gateway || !Array.isArray(snap.workers)) {
      return {
        ok: false,
        error: "snapshot payload missing required fields",
        fetchedAt: Date.now(),
      };
    }
    const snapAge = Date.now() - new Date(snap.ts).getTime();
    return {
      ok: true,
      snapshot: snap,
      fetchedAt: Date.now(),
      stale: snapAge > STALE_SNAPSHOT_AGE_MS,
    };
  } catch (err) {
    return {
      ok: false,
      error: err instanceof Error ? err.message : "unknown fetch error",
      fetchedAt: Date.now(),
    };
  } finally {
    clearTimeout(timer);
  }
}
