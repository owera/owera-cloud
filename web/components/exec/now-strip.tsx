"use client";

import * as React from "react";
import type { RunState, ControlState } from "@/lib/exec/run-state";
import { formatLatency } from "@/lib/compose/estimate";

interface NowStripProps {
  run: RunState;
  onPause?: () => void;
  onResume?: () => void;
  onEditNext?: () => void;
  onSkip?: () => void;
  onStop?: () => void;
}

/**
 * The Now strip — a persistent band at the top of the run page that shows
 * what the agent is doing RIGHT NOW and gives the operator four controls:
 * Pause / Edit next / Skip / Stop.
 *
 * This is the brake pedal. It's always one click from any view. Pause is
 * non-destructive (the agent commits state at every step boundary); Stop
 * is destructive and confirms.
 */
export function NowStrip({
  run,
  onPause,
  onResume,
  onEditNext,
  onSkip,
  onStop,
}: NowStripProps) {
  const [confirmStop, setConfirmStop] = React.useState(false);
  const isRunning = run.control === "running";
  const isPaused = run.control === "paused";
  const isStopped = run.control === "stopped";

  // Heartbeat dot pulse — tiny live signal so the operator can see the
  // agent breathing without parsing logs.
  const breathing = isRunning;

  return (
    <div
      className={[
        "sticky top-0 z-30 -mx-6 px-6 py-3",
        "border-b border-[var(--color-rule)]",
        "bg-[rgba(10,10,11,0.92)] backdrop-blur-md",
        "flex items-center gap-4",
      ].join(" ")}
      data-control={run.control}
    >
      {/* Status pill */}
      <div className="flex items-center gap-2 shrink-0">
        <span
          className={[
            "inline-block size-2 rounded-full",
            isRunning
              ? "bg-[var(--color-state-running)]"
              : isPaused
                ? "bg-[var(--color-state-queued)]"
                : "bg-[var(--color-ink-dim)]",
            breathing ? "now-pulse" : "",
          ].join(" ")}
          aria-hidden
        />
        <span className="readout-label">
          {isRunning ? "RUNNING" : isPaused ? "PAUSED" : "STOPPED"}
        </span>
      </div>

      {/* Current activity */}
      <div className="flex-1 min-w-0 flex items-center gap-3">
        <span
          className="italic text-base text-[var(--color-ink)] truncate"
          style={{ fontFamily: "var(--font-display)" }}
          title={run.currentActivity}
        >
          {run.currentActivity}
        </span>
        {run.etaMs !== undefined && isRunning && (
          <span className="readout-label text-[var(--color-ink-dim)] shrink-0">
            ETA · {formatLatency(run.etaMs)}
          </span>
        )}
        {run.confidence !== undefined && isRunning && (
          <ConfidenceDot value={run.confidence} />
        )}
      </div>

      {/* Brake pedal */}
      <div className="flex items-center gap-1.5 shrink-0">
        {isRunning && (
          <PedalButton onClick={onPause} title="Pause — safe, the agent commits state and waits">
            <span aria-hidden>❙❙</span> Pause
          </PedalButton>
        )}
        {isPaused && (
          <PedalButton
            tone="primary"
            onClick={onResume}
            title="Resume from where the agent paused"
          >
            <span aria-hidden>▸</span> Resume
          </PedalButton>
        )}
        <PedalButton
          onClick={onEditNext}
          disabled={!isPaused}
          title={isPaused ? "Edit the next step before resuming" : "Pause first to edit"}
        >
          Edit next
        </PedalButton>
        <PedalButton
          onClick={onSkip}
          disabled={isStopped}
          title="Skip the current step — agent uses its default"
        >
          Skip
        </PedalButton>
        {!confirmStop ? (
          <PedalButton
            tone="danger"
            onClick={() => setConfirmStop(true)}
            disabled={isStopped}
            title="Stop the run — destructive"
          >
            Stop
          </PedalButton>
        ) : (
          <span className="flex items-center gap-1.5">
            <PedalButton tone="danger" onClick={onStop}>
              Confirm stop?
            </PedalButton>
            <PedalButton onClick={() => setConfirmStop(false)}>
              Cancel
            </PedalButton>
          </span>
        )}
      </div>

      {/* Keyframes are inline so the component is self-contained. */}
      <style>{`
        .now-pulse {
          animation: now-pulse 1.2s ease-in-out infinite;
        }
        @keyframes now-pulse {
          0%, 100% { box-shadow: 0 0 0 0 rgba(245, 158, 11, 0.5); }
          50%      { box-shadow: 0 0 0 5px rgba(245, 158, 11, 0); }
        }
        @media (prefers-reduced-motion: reduce) {
          .now-pulse { animation: none; }
        }
      `}</style>
    </div>
  );
}

function ConfidenceDot({ value }: { value: number }) {
  const pct = Math.round(value * 100);
  const tone =
    value >= 0.75
      ? "var(--color-state-succeeded)"
      : value >= 0.5
        ? "var(--color-state-running)"
        : "var(--color-state-failed)";
  return (
    <span
      className="readout-label flex items-center gap-1"
      title={`Agent confidence: ${pct}%`}
    >
      <span
        className="inline-block size-1.5 rounded-full"
        style={{ background: tone }}
        aria-hidden
      />
      {pct}% conf
    </span>
  );
}

function PedalButton({
  children,
  onClick,
  disabled,
  title,
  tone,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  title?: string;
  tone?: "primary" | "danger";
}) {
  const base =
    "h-8 px-3 rounded-sm border text-xs font-mono uppercase tracking-wider transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]";
  const palette =
    tone === "primary"
      ? "bg-[var(--color-stop-fill)]/15 border-[var(--color-stop-fill)] text-[var(--color-stop-fill)] hover:bg-[var(--color-stop-fill)]/25"
      : tone === "danger"
        ? "bg-transparent border-[var(--color-state-failed)]/60 text-[var(--color-state-failed)] hover:bg-[var(--color-state-failed)]/10"
        : "bg-transparent border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-ink)] hover:border-[var(--color-stop-fill)]/40";
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={[
        base,
        palette,
        disabled ? "opacity-40 cursor-not-allowed" : "",
      ].join(" ")}
    >
      {children}
    </button>
  );
}

// Re-export the type for the parent to manipulate.
export type { ControlState };
