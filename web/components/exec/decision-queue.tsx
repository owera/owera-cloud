"use client";

import * as React from "react";
import type { RunDecision, DecisionKind } from "@/lib/exec/run-state";

interface DecisionQueueProps {
  decisions: ReadonlyArray<RunDecision>;
  /** Called when the operator decides on an item. */
  onDecide?: (id: string, choice: "accept" | "edit" | "reject") => void;
}

/**
 * The Decision queue — a structured "needs you" inbox.
 *
 * Three kinds of items can land here:
 *   - blocking-question  : agent is paused waiting on you (with a default + countdown)
 *   - approval-draft     : agent has an artifact ready; one click to send
 *   - non-blocking-flag  : FYI — agent made a judgment call you should know about
 *
 * Empty state explicitly says "nothing needs you, the agent is working" —
 * silence here means trust, which is the point.
 */
export function DecisionQueue({ decisions, onDecide }: DecisionQueueProps) {
  // Blocking questions sort to the top because they're holding the run.
  const sorted = React.useMemo(() => {
    const order: Record<DecisionKind, number> = {
      "blocking-question": 0,
      "approval-draft": 1,
      "non-blocking-flag": 2,
    };
    return [...decisions].sort((a, b) => order[a.kind] - order[b.kind]);
  }, [decisions]);

  const blockingCount = sorted.filter((d) => d.kind === "blocking-question").length;

  return (
    <section className="border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.2)]">
      <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] px-5 py-3">
        <span className="readout-label">Decisions · needs you</span>
        <span className="readout-label">
          {blockingCount > 0 ? (
            <span className="text-[var(--color-state-running)]">
              {blockingCount} BLOCKING
            </span>
          ) : (
            <span className="text-[var(--color-ink-dim)]">
              {sorted.length} ITEM{sorted.length === 1 ? "" : "S"}
            </span>
          )}
        </span>
      </header>

      {sorted.length === 0 ? (
        <div className="px-5 py-6 flex items-center gap-3">
          <span
            className="size-1.5 rounded-full bg-[var(--color-state-succeeded)]"
            aria-hidden
          />
          <span
            className="italic text-sm text-[var(--color-ink-dim)]"
            style={{ fontFamily: "var(--font-display)" }}
          >
            Nothing needs you. The agent is working.
          </span>
        </div>
      ) : (
        <ul className="flex flex-col divide-y divide-[var(--color-rule)]">
          {sorted.map((d) => (
            <DecisionRow key={d.id} decision={d} onDecide={onDecide} />
          ))}
        </ul>
      )}
    </section>
  );
}

function DecisionRow({
  decision,
  onDecide,
}: {
  decision: RunDecision;
  onDecide?: (id: string, choice: "accept" | "edit" | "reject") => void;
}) {
  const [expanded, setExpanded] = React.useState(decision.kind === "blocking-question");
  const [decided, setDecided] = React.useState<"accept" | "edit" | "reject" | null>(
    null,
  );
  const countdown = useCountdown(decision.defaultAt ?? null);

  function decide(choice: "accept" | "edit" | "reject") {
    setDecided(choice);
    onDecide?.(decision.id, choice);
  }

  const tone = toneFor(decision.kind);
  const verb = verbFor(decision.kind);

  return (
    <li
      className={[
        "px-5 py-4 flex flex-col gap-2",
        decided ? "opacity-50" : "",
      ].join(" ")}
    >
      <div className="flex items-start gap-3">
        {/* Kind dot */}
        <span
          className={`mt-1.5 size-2 rounded-full ${tone.dot} shrink-0`}
          aria-hidden
        />
        {/* Title + meta */}
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-2 flex-wrap">
            <span className={`readout-label ${tone.label}`}>{verb}</span>
            {countdown && (
              <span className="readout-label text-[var(--color-state-running)]">
                AUTO-DEFAULT IN {countdown}
              </span>
            )}
          </div>
          <p className="mt-1 text-sm text-[var(--color-ink)] leading-snug">
            {decision.title}
          </p>
        </div>
        {/* Action buttons */}
        {!decided ? (
          <ActionBar
            kind={decision.kind}
            onAccept={() => decide("accept")}
            onEdit={() => decide("edit")}
            onReject={() => decide("reject")}
          />
        ) : (
          <span className="readout-label text-[var(--color-ink-dim)]">
            {decided.toUpperCase()}
          </span>
        )}
      </div>

      {/* Expand for detail/preview */}
      {(decision.detail || decision.preview || decision.defaultAction) && (
        <div className="ml-5 flex flex-col gap-2">
          <button
            type="button"
            onClick={() => setExpanded((s) => !s)}
            className="self-start text-[10px] font-mono uppercase tracking-wider text-[var(--color-ink-dim)] hover:text-[var(--color-ink)]"
          >
            {expanded ? "− Less" : "+ More"}
          </button>
          {expanded && (
            <div className="flex flex-col gap-2 border-l border-[var(--color-rule)] pl-4">
              {decision.detail && (
                <p className="text-sm text-[var(--color-ink-dim)] leading-snug">
                  {decision.detail}
                </p>
              )}
              {decision.preview && (
                <div className="text-sm font-mono bg-[rgba(0,0,0,0.25)] border border-[var(--color-rule)] rounded-sm px-3 py-2 text-[var(--color-ink)]">
                  {decision.preview}
                </div>
              )}
              {decision.defaultAction && (
                <p
                  className="italic text-xs text-[var(--color-ink-dim)]"
                  style={{ fontFamily: "var(--font-display)" }}
                >
                  Default if you don&apos;t respond: {decision.defaultAction}.
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </li>
  );
}

function ActionBar({
  kind,
  onAccept,
  onEdit,
  onReject,
}: {
  kind: DecisionKind;
  onAccept: () => void;
  onEdit: () => void;
  onReject: () => void;
}) {
  return (
    <div className="flex items-center gap-1.5 shrink-0">
      <RowButton tone="success" onClick={onAccept}>
        {kind === "approval-draft" ? "Send" : kind === "blocking-question" ? "Yes" : "OK"}
      </RowButton>
      <RowButton onClick={onEdit}>Edit</RowButton>
      <RowButton tone="danger" onClick={onReject}>
        {kind === "approval-draft" ? "Discard" : kind === "blocking-question" ? "No" : "Hide"}
      </RowButton>
    </div>
  );
}

function RowButton({
  children,
  onClick,
  tone,
}: {
  children: React.ReactNode;
  onClick: () => void;
  tone?: "success" | "danger";
}) {
  const palette =
    tone === "success"
      ? "border-[var(--color-state-succeeded)]/50 text-[var(--color-state-succeeded)] hover:bg-[var(--color-state-succeeded)]/10"
      : tone === "danger"
        ? "border-[var(--color-state-failed)]/50 text-[var(--color-state-failed)] hover:bg-[var(--color-state-failed)]/10"
        : "border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-ink)]";
  return (
    <button
      type="button"
      onClick={onClick}
      className={`h-7 px-2.5 rounded-sm border text-[10px] font-mono uppercase tracking-wider bg-transparent transition-colors ${palette}`}
    >
      {children}
    </button>
  );
}

function toneFor(kind: DecisionKind): { dot: string; label: string } {
  switch (kind) {
    case "blocking-question":
      return {
        dot: "bg-[var(--color-state-running)]",
        label: "text-[var(--color-state-running)]",
      };
    case "approval-draft":
      return {
        dot: "bg-[var(--color-stop-fill)]",
        label: "text-[var(--color-stop-fill)]",
      };
    case "non-blocking-flag":
      return {
        dot: "bg-[var(--color-ink-dim)]",
        label: "text-[var(--color-ink-dim)]",
      };
  }
}

function verbFor(kind: DecisionKind): string {
  switch (kind) {
    case "blocking-question":
      return "BLOCKING QUESTION";
    case "approval-draft":
      return "READY FOR APPROVAL";
    case "non-blocking-flag":
      return "FYI · JUDGMENT CALL";
  }
}

function useCountdown(targetIso: string | null): string | null {
  const [now, setNow] = React.useState(() => Date.now());
  React.useEffect(() => {
    if (!targetIso) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [targetIso]);
  if (!targetIso) return null;
  const ms = new Date(targetIso).getTime() - now;
  if (ms <= 0) return "0s";
  const totalSec = Math.floor(ms / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s.toString().padStart(2, "0")}s`;
  return `${s}s`;
}
