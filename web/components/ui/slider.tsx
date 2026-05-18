"use client";

import * as React from "react";
import * as RadixSlider from "@radix-ui/react-slider";
import { cn } from "./cn";
import {
  COMPLEXITY_LEVELS,
  STOP_META,
  type ComplexityLevel,
} from "@/lib/compose/levels";

export type { ComplexityLevel } from "@/lib/compose/levels";

const LEVEL_INDEX: Record<ComplexityLevel, number> = {
  simple: 0,
  standard: 1,
  advanced: 2,
  expert: 3,
  custom: 4,
};

export interface ComplexitySliderProps {
  value: ComplexityLevel;
  onChange: (next: ComplexityLevel) => void;
  /** First stop the user is NOT allowed to cross (server-enforced too). */
  gateAt?: ComplexityLevel | null;
  /** Disable interaction. */
  disabled?: boolean;
  className?: string;
}

/**
 * Discrete 5-stop complexity slider. The hero control of /compose.
 *
 * Keyboard: arrows step, Home/End jump, 1–5 jump to stop.
 * A11y: aria-valuetext reports the human stop label.
 */
export function ComplexitySlider({
  value,
  onChange,
  gateAt = null,
  disabled,
  className,
}: ComplexitySliderProps) {
  const idx = LEVEL_INDEX[value];
  const gateIdx = gateAt ? LEVEL_INDEX[gateAt] : COMPLEXITY_LEVELS.length;

  const apply = React.useCallback(
    (next: number) => {
      const clamped = Math.max(
        0,
        Math.min(COMPLEXITY_LEVELS.length - 1, next),
      );
      const nextLevel = COMPLEXITY_LEVELS[clamped];
      if (nextLevel && nextLevel !== value) onChange(nextLevel);
    },
    [onChange, value],
  );

  const onKey = React.useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (disabled) return;
      if (e.key >= "1" && e.key <= "5") {
        e.preventDefault();
        apply(Number(e.key) - 1);
        return;
      }
      // Radix handles arrows/Home/End for us, no override needed.
    },
    [apply, disabled],
  );

  return (
    <div
      className={cn("relative select-none", className)}
      onKeyDown={onKey}
      data-stop={value}
    >
      <RadixSlider.Root
        value={[idx]}
        min={0}
        max={COMPLEXITY_LEVELS.length - 1}
        step={1}
        disabled={disabled}
        onValueChange={(vals) => {
          const next = vals[0] ?? 0;
          apply(next);
        }}
        className={cn(
          "relative flex w-full touch-none items-center",
          "h-10",
        )}
        aria-label="Complexity level"
      >
        <RadixSlider.Track
          className={cn(
            "relative h-1 w-full grow rounded-full",
            "bg-[var(--color-stop-track)]",
          )}
        >
          <RadixSlider.Range
            className={cn(
              "absolute h-full rounded-full",
              "bg-[var(--color-stop-fill)]",
              "transition-[width] duration-150 ease-out",
            )}
          />
        </RadixSlider.Track>

        {COMPLEXITY_LEVELS.map((level, i) => {
          const isActive = i <= idx;
          const isGated = i >= gateIdx;
          const left = (i / (COMPLEXITY_LEVELS.length - 1)) * 100;
          return (
            <button
              key={level}
              type="button"
              tabIndex={-1}
              aria-hidden
              disabled={disabled}
              onClick={() => !disabled && apply(i)}
              className={cn(
                "absolute -translate-x-1/2 size-3 rounded-full border",
                "transition-colors duration-150",
                isActive
                  ? "bg-[var(--color-stop-tick-active)] border-[var(--color-stop-tick-active)]"
                  : "bg-[var(--color-stop-track)] border-[var(--color-stop-tick)]",
                isGated && "ring-1 ring-[var(--color-state-failed)]/40",
                "hover:scale-110",
              )}
              style={{ left: `${left}%` }}
            />
          );
        })}

        <RadixSlider.Thumb
          aria-valuetext={STOP_META[value].label}
          className={cn(
            "block size-5 rounded-full",
            "bg-[var(--color-stop-thumb)]",
            "shadow-[0_0_0_1px_var(--color-border)]",
            "focus-visible:outline-none",
            "focus-visible:ring-2 focus-visible:ring-[var(--color-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-background)]",
            "transition-transform duration-150 ease-out",
            "hover:scale-105",
            disabled && "opacity-50 cursor-not-allowed",
          )}
        />
      </RadixSlider.Root>

      <div className="mt-3 grid grid-cols-5 gap-1">
        {COMPLEXITY_LEVELS.map((level, i) => {
          const isActive = i === idx;
          const isPast = i < idx;
          const isGated = i >= gateIdx;
          return (
            <button
              key={level}
              type="button"
              disabled={disabled}
              onClick={() => !disabled && apply(i)}
              className={cn(
                "flex flex-col items-center text-center px-1 py-1",
                "transition-colors rounded",
                "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]",
                isActive && "bg-[var(--color-muted)]",
              )}
            >
              <span
                className={cn(
                  "font-mono text-[10px] uppercase tracking-wide",
                  isActive
                    ? "text-[var(--color-stop-label-active)]"
                    : isPast
                      ? "text-[var(--color-foreground)]"
                      : "text-[var(--color-stop-label-muted)]",
                  isGated && "text-[var(--color-state-failed)]/70",
                )}
              >
                {STOP_META[level].label}
              </span>
              <span
                className={cn(
                  "mt-0.5 text-[10px] leading-tight",
                  isActive
                    ? "text-[var(--color-muted-foreground)]"
                    : "text-[var(--color-stop-label-muted)]",
                  "hidden sm:block",
                )}
              >
                {STOP_META[level].caption}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

