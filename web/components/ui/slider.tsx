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
 * The hero control of /compose. Treated as a precision instrument, not a
 * generic Radix slider — vernier-tick rail, index labels per stop, variable
 * font-weight transitions on the active label, soft aura on the thumb.
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

  // Build vernier-style tick marks between the major stops. 12 minor ticks
  // between each major stop = 48 minor + 5 major = the precision-instrument
  // feel. Pure decoration — the slider snaps to majors only.
  const minorTicks = React.useMemo(() => {
    const ticks: number[] = [];
    const segments = COMPLEXITY_LEVELS.length - 1;
    const minorsPerSeg = 12;
    for (let s = 0; s < segments; s++) {
      for (let m = 1; m < minorsPerSeg; m++) {
        ticks.push(((s + m / minorsPerSeg) / segments) * 100);
      }
    }
    return ticks;
  }, []);

  return (
    <div
      className={cn("relative select-none", className)}
      onKeyDown={onKey}
      data-stop={value}
    >
      {/* Vernier tick rail — sits ABOVE the slider track. Decorative. */}
      <div
        className="relative h-3 w-full mb-2 pointer-events-none"
        aria-hidden
      >
        {minorTicks.map((left, i) => (
          <span
            key={`m${i}`}
            className="absolute top-0 w-px h-1.5 bg-[var(--color-stop-tick)]"
            style={{ left: `${left}%` }}
          />
        ))}
        {COMPLEXITY_LEVELS.map((_, i) => {
          const left = (i / (COMPLEXITY_LEVELS.length - 1)) * 100;
          const past = i <= idx;
          return (
            <span
              key={`M${i}`}
              className={cn(
                "absolute top-0 w-px h-3 transition-colors duration-200",
                past
                  ? "bg-[var(--color-stop-fill)]"
                  : "bg-[var(--color-stop-tick)]",
              )}
              style={{ left: `${left}%` }}
            />
          );
        })}
      </div>

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
        className="relative flex w-full touch-none items-center h-8"
        aria-label="Complexity level"
      >
        <RadixSlider.Track className="relative h-px w-full grow bg-[var(--color-stop-track)] compose-track-draw">
          <RadixSlider.Range className="absolute h-full bg-[var(--color-stop-fill)] transition-[width] duration-200 ease-out" />
        </RadixSlider.Track>

        {COMPLEXITY_LEVELS.map((level, i) => {
          const isActive = i <= idx;
          const isCurrent = i === idx;
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
                "absolute -translate-x-1/2 rounded-full",
                "transition-all duration-200 ease-out",
                isCurrent
                  ? "size-2.5 bg-[var(--color-stop-fill)] ring-1 ring-[var(--color-stop-fill)]"
                  : isActive
                    ? "size-1.5 bg-[var(--color-stop-fill)]"
                    : "size-1.5 bg-[var(--color-stop-tick)]",
                isGated && "ring-1 ring-[var(--color-state-failed)]/50",
                !disabled && "hover:scale-125",
              )}
              style={{ left: `${left}%` }}
            />
          );
        })}

        <RadixSlider.Thumb
          aria-valuetext={STOP_META[value].label}
          className={cn(
            "block size-7 rounded-full bg-[var(--color-stop-thumb)]",
            "compose-thumb-aura",
            "focus-visible:outline-none",
            "focus-visible:ring-2 focus-visible:ring-[var(--color-ring)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-background)]",
            "transition-transform duration-200 ease-out",
            "hover:scale-105 active:scale-95",
            disabled && "opacity-50 cursor-not-allowed",
          )}
        />
      </RadixSlider.Root>

      {/* Index labels — instrument-panel feel (INST. 01 / 02 / 03…). */}
      <div className="mt-1 grid grid-cols-5 pointer-events-none">
        {COMPLEXITY_LEVELS.map((_, i) => (
          <span
            key={`idx${i}`}
            className="readout-label text-center"
            style={{ fontSize: 9, opacity: i <= idx ? 0.85 : 0.35 }}
          >
            {String(i + 1).padStart(2, "0")}
          </span>
        ))}
      </div>

      {/* Stop labels — variable font weight transitions on active. */}
      <div className="mt-5 grid grid-cols-5 gap-1">
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
                "flex flex-col items-center text-center px-1 py-1 rounded",
                "transition-colors",
                "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]",
              )}
            >
              <span
                className="stop-label text-[10px]"
                data-active={isActive}
                data-past={isPast}
                style={{
                  color: isGated
                    ? "rgba(239, 68, 68, 0.7)"
                    : undefined,
                }}
              >
                {STOP_META[level].label}
              </span>
              <span
                className={cn(
                  "mt-1 text-[10px] leading-tight font-sans",
                  isActive
                    ? "text-[var(--color-ink-dim)]"
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
