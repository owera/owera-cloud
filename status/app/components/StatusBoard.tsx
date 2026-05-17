"use client";

import { useEffect, useState } from "react";
import type { Component, ComponentGroup } from "@/lib/components";
import {
  deriveComponents,
  deriveOverall,
  type DerivedComponent,
} from "@/lib/derive-status";
import type { FetchResult, HealthSnapshot } from "../../lib/fetch-snapshot";

// Poll cadence. The acceptance criterion is "forced incident reflects in
// <60s"; 30s gives us two attempts inside that window. Operator-plane
// snapshots refresh every 30s, so polling faster would just hit a stale
// cache.
const POLL_INTERVAL_MS = 30_000;

type Props = {
  components: Component[];
  groups: ComponentGroup[];
  initial: FetchResult;
};

export function StatusBoard({ components, groups, initial }: Props) {
  const [result, setResult] = useState<FetchResult>(initial);

  useEffect(() => {
    let cancelled = false;
    async function tick() {
      try {
        const res = await fetch("/api/status", { cache: "no-store" });
        if (!res.ok) return;
        const data = (await res.json()) as FetchResult;
        if (!cancelled) setResult(data);
      } catch {
        // swallow — keep last good state on transient network errors
      }
    }
    const id = window.setInterval(tick, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  const snapshot: HealthSnapshot | null = result.ok ? result.snapshot : null;
  const stale = result.ok ? result.stale : true;
  const error = result.ok ? null : result.error;
  const derived = deriveComponents(components, snapshot, error);
  const overall = deriveOverall(derived);
  const byId = new Map<string, DerivedComponent>();
  for (const d of derived) byId.set(d.id, d);

  const lastUpdated = snapshot ? new Date(snapshot.ts) : null;

  return (
    <>
      <section className={`overall ${overall}`}>
        <span className={`dot ${overall}`} aria-hidden />
        <div>
          <h1>{overallHeadline(overall)}</h1>
          <p>
            {snapshot
              ? `Snapshot ${formatRelative(lastUpdated!)}.`
              : `No snapshot data available${error ? ` (${error})` : ""}.`}
          </p>
          {stale && snapshot ? (
            <p className="stale">
              Data may be stale; the snapshot writer has not refreshed
              recently.
            </p>
          ) : null}
        </div>
      </section>

      {groups.map((g) => {
        const members = g.members
          .map((id) => byId.get(id))
          .filter((c): c is DerivedComponent => Boolean(c));
        if (members.length === 0) return null;
        return (
          <div key={g.id} className="group">
            <h2>{g.name}</h2>
            {members.map((m) => (
              <div className="component" key={m.id}>
                <span className={`dot ${m.status}`} aria-hidden />
                <div>
                  <div className="name">{m.name}</div>
                  <div className="detail">{m.detail}</div>
                </div>
                <span className="badge">{m.status}</span>
              </div>
            ))}
          </div>
        );
      })}
    </>
  );
}

function overallHeadline(s: DerivedComponent["status"]): string {
  switch (s) {
    case "operational":
      return "All systems operational";
    case "degraded":
      return "Some systems degraded";
    case "down":
      return "Active incident in progress";
    default:
      return "Status unknown";
  }
}

function formatRelative(d: Date): string {
  const delta = Date.now() - d.getTime();
  if (delta < 0) return "just now";
  const s = Math.floor(delta / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  return `${h}h ago`;
}
