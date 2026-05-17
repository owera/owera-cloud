import type { Component } from "@/lib/components";
import type {
  HealthSnapshot,
  WorkerHealth,
} from "../../lib/fetch-snapshot";

export type ComponentStatus = "operational" | "degraded" | "down" | "unknown";

export type DerivedComponent = {
  id: string;
  name: string;
  description: string;
  status: ComponentStatus;
  detail: string;
};

// Threshold for declaring the worker fleet degraded vs down. Pulled from
// the `hermes-workers` entry in components.yaml (`healthy_threshold_pct`)
// at runtime, with a defensive default.
const DEFAULT_WORKER_THRESHOLD_PCT = 67;

function workerSummary(workers: WorkerHealth[]): { healthy: number; total: number } {
  return {
    healthy: workers.filter((w) => w.ok).length,
    total: workers.length,
  };
}

export function deriveOverall(components: DerivedComponent[]): ComponentStatus {
  if (components.some((c) => c.status === "down")) return "down";
  if (components.some((c) => c.status === "degraded")) return "degraded";
  if (components.every((c) => c.status === "operational")) return "operational";
  return "unknown";
}

// Map the operator-plane snapshot onto the component list declared in
// components.yaml. Components that the snapshot doesn't speak to (web,
// status page itself, tunnel) fall through to "operational" — they're
// observed from the cloud edge, not the operator plane, and a separate
// probe will populate them in a future ticket. For T19.4 the acceptance
// criterion is "kill gateway -> reflected in <60s", which exercises the
// operator-plane-gateway + hermes-workers components specifically.
export function deriveComponents(
  components: Component[],
  snapshot: HealthSnapshot | null,
  fetchError: string | null,
): DerivedComponent[] {
  return components.map((c) => {
    if (c.id === "operator-plane-gateway") {
      if (!snapshot) {
        return {
          id: c.id,
          name: c.name,
          description: c.description,
          status: fetchError ? "down" : "unknown",
          detail: fetchError ?? "No snapshot data yet.",
        };
      }
      return {
        id: c.id,
        name: c.name,
        description: c.description,
        status: snapshot.gateway.ok ? "operational" : "down",
        detail: snapshot.gateway.ok
          ? `Hermes ${snapshot.gateway.hermes_version}, uptime ${formatUptime(
              snapshot.gateway.uptime_seconds,
            )}.`
          : "Gateway not reporting a healthy Hermes version.",
      };
    }

    if (c.id === "hermes-workers") {
      if (!snapshot) {
        return {
          id: c.id,
          name: c.name,
          description: c.description,
          status: fetchError ? "down" : "unknown",
          detail: fetchError ?? "No snapshot data yet.",
        };
      }
      const { healthy, total } = workerSummary(snapshot.workers);
      const threshold =
        c.probe.healthy_threshold_pct ?? DEFAULT_WORKER_THRESHOLD_PCT;
      if (total === 0) {
        return {
          id: c.id,
          name: c.name,
          description: c.description,
          status: "down",
          detail: "No worker nodes are reporting heartbeats.",
        };
      }
      const pct = (healthy / total) * 100;
      const status: ComponentStatus =
        pct >= threshold ? (pct === 100 ? "operational" : "degraded") : "down";
      return {
        id: c.id,
        name: c.name,
        description: c.description,
        status,
        detail: `${healthy}/${total} workers healthy (threshold ${threshold}%).`,
      };
    }

    if (c.id === "public-api" || c.id === "customer-dashboard" || c.id === "public-status") {
      // No edge probe in this slice. A future Wave-9 ticket adds a
      // separate uptime-robot probe that publishes its own snapshot;
      // until then we report "operational" so the page is not
      // permanently red for components nobody is probing yet.
      return {
        id: c.id,
        name: c.name,
        description: c.description,
        status: "operational",
        detail: "Edge probe pending (Wave 9).",
      };
    }

    return {
      id: c.id,
      name: c.name,
      description: c.description,
      status: "unknown",
      detail: "No probe wired.",
    };
  });
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}
