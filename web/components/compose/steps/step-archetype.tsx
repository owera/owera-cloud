"use client";

import * as React from "react";
import { ARCHETYPES, type ArchetypeId } from "@/lib/compose/archetypes";

interface StepArchetypeProps {
  value: ArchetypeId;
  onChange: (next: ArchetypeId) => void;
}

/**
 * Step 1 — choose an outcome. Six cards (research / triage / brief / build /
 * watch / custom). Picking one seeds the rest of the state.
 */
export function StepArchetype({ value, onChange }: StepArchetypeProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {ARCHETYPES.map((a) => {
        const isActive = a.id === value;
        return (
          <button
            key={a.id}
            type="button"
            onClick={() => onChange(a.id)}
            className={[
              "group relative text-left",
              "border bg-[rgba(0,0,0,0.25)]",
              "rounded-sm px-5 py-5 min-h-[180px] flex flex-col gap-2",
              "transition-all duration-200",
              "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]",
              isActive
                ? "border-[var(--color-stop-fill)] bg-[rgba(91,141,239,0.06)]"
                : "border-[var(--color-rule)] hover:border-[var(--color-stop-fill)]/40",
            ].join(" ")}
            aria-pressed={isActive}
          >
            <div className="flex items-start justify-between">
              <span
                className="text-[2rem] leading-none font-light"
                style={{
                  color: isActive
                    ? "var(--color-stop-fill)"
                    : "var(--color-ink-dim)",
                  fontFamily: "var(--font-display)",
                }}
                aria-hidden
              >
                {a.glyph}
              </span>
              <span className="readout-label">
                {a.id.toUpperCase()}
              </span>
            </div>
            <div className="mt-auto flex flex-col gap-1">
              <span className="font-mono text-sm uppercase tracking-wider text-[var(--color-ink)]">
                {a.name}
              </span>
              <span
                className="italic text-[var(--color-ink-dim)] text-sm leading-snug"
                style={{ fontFamily: "var(--font-display)" }}
              >
                {a.tagline}
              </span>
              <span className="text-xs text-[var(--color-ink-dim)] leading-snug mt-1">
                {a.description}
              </span>
            </div>
            {/* Active indicator hairline. */}
            {isActive && (
              <span
                className="absolute left-0 top-0 bottom-0 w-px bg-[var(--color-stop-fill)]"
                aria-hidden
              />
            )}
          </button>
        );
      })}
    </div>
  );
}
