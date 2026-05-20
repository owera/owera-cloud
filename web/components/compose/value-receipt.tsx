"use client";

import * as React from "react";
import { formatCents } from "@/lib/format";
import type { Job, JobLedgerEntry } from "@/lib/types";

interface ValueReceiptProps {
  job: Job;
  ledger: ReadonlyArray<JobLedgerEntry>;
  /** The blueprint id, if this job was hired from the catalog. */
  jobBlueprintId?: string;
  /** What unit is this job charged by (e.g. "per qualified lead"). */
  billingUnit?: string;
}

interface ResultItem {
  id: string;
  label: string;
  detail?: string;
  ts: string;
  accepted: "yes" | "no" | "pending";
}

/**
 * Value receipt — what the agent claims it produced + the operator's
 * accept/reject per item. This is the BILLING ARTIFACT: we only charge
 * for items the operator accepted (or didn't reject within the policy
 * window). It is also the training signal.
 *
 * For now the receipt is local-only — accept/reject is stored in
 * component state so the QA flow demonstrates the loop. Persisting it
 * is a backend follow-up.
 */
export function ValueReceipt({
  job,
  ledger,
  jobBlueprintId,
  billingUnit,
}: ValueReceiptProps) {
  // Project ledger entries of kind "output" into result items. If the
  // agent didn't produce explicit outputs (most fixture jobs don't), fall
  // back to billing entries as proxy items so the UI is always populated.
  const items = React.useMemo<ResultItem[]>(() => {
    const outputs = ledger.filter((e) => e.kind === "output");
    if (outputs.length > 0) {
      return outputs.map((e) => ({
        id: e.id,
        label: e.message,
        ts: e.ts,
        accepted: "pending",
      }));
    }
    const billing = ledger.filter((e) => e.kind === "billing");
    return billing.map((e) => ({
      id: e.id,
      label: e.message,
      ts: e.ts,
      accepted: "pending",
    }));
  }, [ledger]);

  const [decisions, setDecisions] = React.useState<Record<string, "yes" | "no">>({});
  const finalized = job.state === "succeeded" || job.state === "failed" || job.state === "cancelled";

  const accepted = items.filter((i) => decisions[i.id] === "yes").length;
  const rejected = items.filter((i) => decisions[i.id] === "no").length;
  const pending = items.length - accepted - rejected;

  return (
    <div className="border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.2)] p-5 flex flex-col gap-4">
      <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2">
        <span className="readout-label">Value receipt</span>
        <span className="readout-label">
          {finalized ? "FINAL" : "PROVISIONAL"}
        </span>
      </header>

      {/* Summary line. */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-sm">
        <Stat label="Spent" value={formatCents(job.costCents)} />
        <Stat label="Items" value={String(items.length)} />
        <Stat
          label="Accepted"
          value={String(accepted)}
          tone={accepted > 0 ? "good" : undefined}
        />
        <Stat
          label="Rejected"
          value={String(rejected)}
          tone={rejected > 0 ? "bad" : undefined}
        />
      </div>

      {/* Item list with accept/reject. */}
      <div className="flex flex-col gap-2 mt-2">
        {items.length === 0 && (
          <p className="text-sm text-[var(--color-ink-dim)] italic" style={{ fontFamily: "var(--font-display)" }}>
            No deliverables yet. When the agent produces items, they&apos;ll
            appear here for you to accept or reject.
          </p>
        )}
        {items.map((item) => {
          const decision = decisions[item.id];
          return (
            <div
              key={item.id}
              className="flex items-start gap-3 px-3 py-2 rounded-sm border border-[var(--color-rule)] bg-[rgba(0,0,0,0.15)]"
            >
              <span
                className={`w-2 h-2 mt-2 rounded-full ${
                  decision === "yes"
                    ? "bg-[var(--color-state-succeeded)]"
                    : decision === "no"
                      ? "bg-[var(--color-state-failed)]"
                      : "bg-[var(--color-ink-dim)]"
                }`}
                aria-hidden
              />
              <div className="flex-1 min-w-0">
                <p className="text-sm text-[var(--color-ink)] leading-snug">
                  {item.label}
                </p>
                {item.detail && (
                  <p className="text-xs text-[var(--color-ink-dim)] mt-0.5">
                    {item.detail}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-1.5 shrink-0">
                <button
                  type="button"
                  onClick={() =>
                    setDecisions((d) => ({ ...d, [item.id]: "yes" }))
                  }
                  className={[
                    "px-2 h-7 rounded-sm text-[10px] font-mono uppercase tracking-wider border transition-colors",
                    decision === "yes"
                      ? "bg-[var(--color-state-succeeded)]/15 border-[var(--color-state-succeeded)] text-[var(--color-state-succeeded)]"
                      : "bg-transparent border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-state-succeeded)] hover:border-[var(--color-state-succeeded)]/50",
                  ].join(" ")}
                  aria-pressed={decision === "yes"}
                >
                  Accept
                </button>
                <button
                  type="button"
                  onClick={() =>
                    setDecisions((d) => ({ ...d, [item.id]: "no" }))
                  }
                  className={[
                    "px-2 h-7 rounded-sm text-[10px] font-mono uppercase tracking-wider border transition-colors",
                    decision === "no"
                      ? "bg-[var(--color-state-failed)]/15 border-[var(--color-state-failed)] text-[var(--color-state-failed)]"
                      : "bg-transparent border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-state-failed)] hover:border-[var(--color-state-failed)]/50",
                  ].join(" ")}
                  aria-pressed={decision === "no"}
                >
                  Reject
                </button>
              </div>
            </div>
          );
        })}
      </div>

      {/* Billing line. */}
      <footer className="border-t border-[var(--color-rule)] pt-3 flex items-baseline justify-between text-sm">
        <span
          className="italic text-[var(--color-ink-dim)]"
          style={{ fontFamily: "var(--font-display)" }}
        >
          {billingUnit
            ? `You're charged ${billingUnit} — only accepted items count.`
            : "Owera charges only for items you accept."}
          {jobBlueprintId && (
            <span className="text-[var(--color-ink-dim)] ml-2">
              · Hired from <code className="font-mono">{jobBlueprintId}</code>
            </span>
          )}
        </span>
        <span className="readout-label">
          {pending > 0
            ? `${pending} pending`
            : finalized
              ? `${accepted} BILLABLE`
              : "ALL DECIDED"}
        </span>
      </footer>
    </div>
  );
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: "good" | "bad";
}) {
  return (
    <div className="flex flex-col gap-1">
      <span className="readout-label">{label}</span>
      <span
        className="readout-numeric text-lg"
        style={{
          color:
            tone === "good"
              ? "var(--color-state-succeeded)"
              : tone === "bad"
                ? "var(--color-state-failed)"
                : "var(--color-ink)",
        }}
      >
        {value}
      </span>
    </div>
  );
}
