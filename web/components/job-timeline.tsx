"use client";

import * as React from "react";
import type { JobLedgerEntry } from "@/lib/types";
import { shortTimestamp } from "@/lib/format";

interface JobTimelineProps {
  jobId: string;
  initial: JobLedgerEntry[];
  /** When false, do not open SSE (useful for static/fixture mode). */
  live: boolean;
}

interface SseFrame {
  id?: string;
  job_id?: string;
  ts?: string;
  kind?: JobLedgerEntry["kind"];
  message?: string;
  data?: Record<string, unknown> | null;
}

function frameToEntry(f: SseFrame, fallbackId: string): JobLedgerEntry | null {
  if (!f.ts || !f.kind || !f.message) return null;
  return {
    id: f.id ?? fallbackId,
    jobId: f.job_id ?? "",
    ts: f.ts,
    kind: f.kind,
    message: f.message,
    data: f.data ?? null,
  };
}

const KIND_TONE: Record<JobLedgerEntry["kind"], string> = {
  state_change: "var(--color-state-running)",
  log: "var(--color-muted-foreground)",
  tool_call: "var(--color-primary)",
  output: "var(--color-state-succeeded)",
  billing: "var(--color-state-queued)",
};

export function JobTimeline({ jobId, initial, live }: JobTimelineProps) {
  const [entries, setEntries] = React.useState<JobLedgerEntry[]>(initial);
  const [streamState, setStreamState] = React.useState<
    "idle" | "connecting" | "open" | "closed" | "error"
  >(live ? "connecting" : "idle");

  React.useEffect(() => {
    if (!live) return;
    const url = `/api/proxy/v1/jobs/${encodeURIComponent(jobId)}/events`;
    const es = new EventSource(url);
    let counter = 0;
    setStreamState("connecting");
    es.onopen = () => setStreamState("open");
    es.onerror = () => {
      // EventSource auto-reconnects; surface the state but don't tear down.
      setStreamState("error");
    };
    es.onmessage = (ev) => {
      try {
        const frame = JSON.parse(ev.data) as SseFrame;
        const entry = frameToEntry(frame, `sse_${++counter}`);
        if (!entry) return;
        setEntries((prev) => {
          if (prev.some((p) => p.id === entry.id)) return prev;
          return [...prev, entry];
        });
      } catch {
        // Drop malformed frames silently; the upstream should always emit JSON.
      }
    };
    return () => {
      es.close();
      setStreamState("closed");
    };
  }, [jobId, live]);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
          {entries.length} {entries.length === 1 ? "event" : "events"}
        </div>
        <StreamPill state={streamState} />
      </div>

      <ol className="space-y-2 font-mono text-xs">
        {entries.length === 0 && (
          <li className="text-[var(--color-muted-foreground)] py-2 text-center">
            Awaiting events…
          </li>
        )}
        {entries.map((e) => (
          <li
            key={e.id}
            className="grid grid-cols-[10rem_8rem_1fr] gap-2 items-baseline"
          >
            <span className="text-[var(--color-muted-foreground)]">
              {shortTimestamp(e.ts)}
            </span>
            <span
              className="uppercase"
              style={{ color: KIND_TONE[e.kind] }}
            >
              {e.kind}
            </span>
            <span className="break-words">{e.message}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}

function StreamPill({
  state,
}: {
  state: "idle" | "connecting" | "open" | "closed" | "error";
}) {
  const map: Record<typeof state, { label: string; tone: string }> = {
    idle: { label: "STATIC", tone: "var(--color-muted-foreground)" },
    connecting: { label: "CONNECTING", tone: "var(--color-state-queued)" },
    open: { label: "LIVE", tone: "var(--color-state-succeeded)" },
    closed: { label: "CLOSED", tone: "var(--color-muted-foreground)" },
    error: { label: "RETRYING", tone: "var(--color-state-failed)" },
  };
  const { label, tone } = map[state];
  return (
    <span
      className="inline-flex items-center gap-1.5 rounded border px-1.5 py-0.5 text-[10px] uppercase tracking-wide font-mono"
      style={{ color: tone, borderColor: `${tone}66` }}
    >
      <span
        className="h-1.5 w-1.5 rounded-full"
        style={{ backgroundColor: tone }}
      />
      {label}
    </span>
  );
}
