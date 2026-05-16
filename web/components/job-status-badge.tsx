import * as React from "react";
import { Badge } from "./ui/badge";
import type { JobState } from "@/lib/types";

const TONES: Record<JobState, string> = {
  submitted: "var(--color-state-submitted)",
  queued: "var(--color-state-queued)",
  running: "var(--color-state-running)",
  succeeded: "var(--color-state-succeeded)",
  failed: "var(--color-state-failed)",
  cancelled: "var(--color-state-cancelled)",
};

// CSS vars can't reach Badge's inline rgba math — resolve to literal hex here.
const LITERAL: Record<JobState, string> = {
  submitted: "#64748b",
  queued: "#3b82f6",
  running: "#f59e0b",
  succeeded: "#10b981",
  failed: "#ef4444",
  cancelled: "#71717a",
};

export interface JobStatusBadgeProps {
  state: JobState;
  className?: string;
}

export function JobStatusBadge({ state, className }: JobStatusBadgeProps) {
  // We pass the literal hex so the alpha-tinted background works.
  // The CSS variable map is kept for parity with the rest of the design system.
  void TONES;
  return (
    <Badge tone={LITERAL[state]} className={className}>
      {state}
    </Badge>
  );
}
