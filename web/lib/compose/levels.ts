// Complexity-level constants and pure helpers.
//
// Lives outside `components/ui/slider.tsx` (which is `"use client"`) so that
// RSC pages and edge route handlers can import the constants and labels
// without webpack treating them as client-only references.

export type ComplexityLevel =
  | "simple"
  | "standard"
  | "advanced"
  | "expert"
  | "custom";

export const COMPLEXITY_LEVELS: ReadonlyArray<ComplexityLevel> = [
  "simple",
  "standard",
  "advanced",
  "expert",
  "custom",
] as const;

interface StopMeta {
  label: string;
  caption: string;
}

export const STOP_META: Record<ComplexityLevel, StopMeta> = {
  simple: { label: "SIMPLE", caption: "One prompt, one click" },
  standard: { label: "STANDARD", caption: "Tools + output shape" },
  advanced: { label: "ADVANCED", caption: "Model, depth, budget" },
  expert: { label: "EXPERT", caption: "Multi-agent orchestration" },
  custom: { label: "CUSTOM", caption: "Untie everything" },
};

export function complexityLevelLabel(level: ComplexityLevel): string {
  return STOP_META[level].label;
}

export function complexityLevelCaption(level: ComplexityLevel): string {
  return STOP_META[level].caption;
}

export function isComplexityLevel(v: unknown): v is ComplexityLevel {
  return (
    typeof v === "string" &&
    (COMPLEXITY_LEVELS as readonly string[]).includes(v)
  );
}
