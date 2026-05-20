import * as React from "react";
import type { JobLedgerEntry } from "@/lib/types";
import { shortTimestamp } from "@/lib/format";

interface ShippingTrackerProps {
  /** The ledger entries we project into named milestones. */
  ledger: ReadonlyArray<JobLedgerEntry>;
  /** Whether the job is still running. */
  running: boolean;
}

interface Milestone {
  id: string;
  ts: string;
  label: string;
  status: "done" | "current" | "pending";
}

/**
 * Names the milestones for a deep agent in a way a non-technical operator
 * can actually read at a glance. We project state_change + the first
 * tool_call of a phase into milestones, hide the rest. The current
 * activity line surfaces the most recent log message.
 */
export function ShippingTracker({ ledger, running }: ShippingTrackerProps) {
  const milestones: Milestone[] = [];
  for (const e of ledger) {
    if (e.kind === "state_change") {
      milestones.push({
        id: e.id,
        ts: e.ts,
        label: humanizeState(e.message),
        status: "done",
      });
    } else if (e.kind === "tool_call") {
      // Promote the first tool_call after a state_change into a milestone.
      const last = milestones[milestones.length - 1];
      if (last && last.label.startsWith("State")) continue;
      milestones.push({
        id: e.id,
        ts: e.ts,
        label: humanizeTool(e.message),
        status: "done",
      });
    }
  }
  // If still running, mark the last milestone as current.
  if (running && milestones.length > 0) {
    milestones[milestones.length - 1]!.status = "current";
  }
  const latestLog = ledger.findLast?.((e) => e.kind === "log") ?? null;
  const currentActivity = latestLog
    ? latestLog.message
    : running
      ? "Working…"
      : null;

  return (
    <div className="flex flex-col gap-4">
      <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2">
        <span className="readout-label">SHIPMENT</span>
        <span className="readout-label">
          {milestones.length} MILESTONE{milestones.length === 1 ? "" : "S"}
        </span>
      </header>

      <ol className="flex flex-col gap-2">
        {milestones.length === 0 && (
          <li className="text-sm text-[var(--color-ink-dim)] italic" style={{ fontFamily: "var(--font-display)" }}>
            Awaiting the agent&apos;s first heartbeat…
          </li>
        )}
        {milestones.map((m) => (
          <li
            key={m.id}
            className="flex items-start gap-3 text-sm font-mono"
          >
            <span
              className="w-4 mt-1 text-base leading-none"
              aria-hidden
              style={{
                color:
                  m.status === "done"
                    ? "var(--color-stop-fill)"
                    : m.status === "current"
                      ? "var(--color-state-running)"
                      : "var(--color-ink-dim)",
              }}
            >
              {m.status === "done" ? "✓" : m.status === "current" ? "○" : "·"}
            </span>
            <span className="text-[var(--color-ink-dim)] w-16 shrink-0 text-xs mt-1">
              {shortTimestamp(m.ts).split(" ").slice(-2).join(" ")}
            </span>
            <span className="flex-1 text-[var(--color-ink)] leading-snug">
              {m.label}
            </span>
          </li>
        ))}
      </ol>

      {currentActivity && (
        <div className="mt-2 border-t border-[var(--color-rule)] pt-3 flex items-start gap-3 text-sm">
          <span className="readout-label">Now</span>
          <span
            className="italic text-[var(--color-ink-dim)] leading-snug flex-1"
            style={{ fontFamily: "var(--font-display)" }}
          >
            {currentActivity}
          </span>
        </div>
      )}
    </div>
  );
}

function humanizeState(message: string): string {
  // "queued → running on claw1.local" → "Started running"
  if (message.includes("submitted")) return "Job submitted";
  if (message.includes("queued") && message.includes("running")) return "Worker picked up the job";
  if (message.includes("succeeded")) return "Finished successfully";
  if (message.includes("failed")) return "Stopped — error encountered";
  if (message.includes("cancelled")) return "Cancelled";
  return `State: ${message}`;
}

function humanizeTool(message: string): string {
  // "web.search(...)" → "Searched the web"
  if (message.startsWith("web.search")) return "Searched the web";
  if (message.startsWith("hubspot.")) return "Read from HubSpot";
  if (message.startsWith("stripe.")) return "Read from Stripe";
  if (message.startsWith("apollo.")) return "Read from Apollo";
  if (message.startsWith("gmail.")) return "Checked inbox";
  if (message.startsWith("calendar.")) return "Checked the calendar";
  if (message.startsWith("slack.")) return "Posted to Slack";
  if (message.startsWith("github.")) return "Read from GitHub";
  // Fallback: take the verb portion before "(".
  const dot = message.indexOf(".");
  const paren = message.indexOf("(");
  if (dot > 0 && paren > dot) {
    return `Used ${message.slice(0, dot)}: ${message.slice(dot + 1, paren)}`;
  }
  return message;
}
