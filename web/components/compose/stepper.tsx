"use client";

import * as React from "react";

interface StepperProps {
  steps: ReadonlyArray<{ key: string; label: string }>;
  current: number;
  onJump?: (i: number) => void;
}

/**
 * Linear stepper for the composer. Each step renders as an index + label;
 * a hairline rail underneath shows progress. Past steps are click-to-jump.
 */
export function Stepper({ steps, current, onJump }: StepperProps) {
  return (
    <div className="relative" aria-label="Composer progress">
      <div className="grid grid-cols-4 gap-0 relative z-10">
        {steps.map((s, i) => {
          const isActive = i === current;
          const isPast = i < current;
          const clickable = isPast && onJump;
          return (
            <button
              key={s.key}
              type="button"
              onClick={() => clickable && onJump?.(i)}
              disabled={!clickable}
              className={[
                "flex items-baseline gap-2 px-1 py-2 text-left",
                "transition-colors",
                clickable
                  ? "cursor-pointer hover:text-[var(--color-ink)]"
                  : "cursor-default",
                "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]",
              ].join(" ")}
              aria-current={isActive ? "step" : undefined}
            >
              <span
                className="readout-label"
                style={{
                  color: isActive
                    ? "var(--color-stop-fill)"
                    : isPast
                      ? "var(--color-ink)"
                      : "var(--color-stop-label-muted)",
                }}
              >
                {String(i + 1).padStart(2, "0")}
              </span>
              <span
                className={[
                  "text-sm font-mono uppercase tracking-wider",
                  isActive
                    ? "text-[var(--color-ink)]"
                    : isPast
                      ? "text-[var(--color-ink-dim)]"
                      : "text-[var(--color-stop-label-muted)]",
                ].join(" ")}
              >
                {s.label}
              </span>
            </button>
          );
        })}
      </div>

      {/* Rail. */}
      <div className="mt-2 grid grid-cols-4 gap-0">
        {steps.map((_, i) => {
          const filled = i <= current;
          return (
            <span
              key={i}
              className="h-px transition-colors duration-300"
              style={{
                background: filled
                  ? "var(--color-stop-fill)"
                  : "var(--color-rule)",
              }}
            />
          );
        })}
      </div>
    </div>
  );
}
