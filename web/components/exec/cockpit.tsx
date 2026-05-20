"use client";

import * as React from "react";
import { NowStrip } from "./now-strip";
import { DecisionQueue } from "./decision-queue";
import { StepsStream } from "./steps-stream";
import type { RunState, RunStep, ControlState } from "@/lib/exec/run-state";

interface CockpitProps {
  initial: RunState;
}

/**
 * The Cockpit — execution view for a hired job.
 *
 * Composes three components in priority order:
 *
 *   1. NowStrip       — persistent brake pedal (pause / edit / skip / stop)
 *   2. DecisionQueue  — what needs you (blocking questions, approvals, flags)
 *   3. StepsStream    — collapsed transcript with edit-while-paused
 *
 * State is owned here so the user can pause and edit upcoming steps; the
 * backend will own this state in the real wiring. We keep optimistic
 * updates client-side so the demo flow is fluid against fixture data.
 */
export function Cockpit({ initial }: CockpitProps) {
  const [run, setRun] = React.useState<RunState>(initial);

  function setControl(next: ControlState) {
    setRun((prev) => ({
      ...prev,
      control: next,
      currentActivity:
        next === "paused"
          ? `Paused at "${prev.steps.find((s) => s.status === "running")?.label ?? "current step"}"`
          : next === "running"
            ? prev.steps.find((s) => s.status === "running")?.label ?? "Working…"
            : "Stopped",
    }));
  }

  function decide(id: string, choice: "accept" | "edit" | "reject") {
    setRun((prev) => ({
      ...prev,
      decisions: prev.decisions.map((d) =>
        d.id === id
          ? { ...d, defaultAction: choice + "ed", defaultAt: undefined }
          : d,
      ),
    }));
  }

  function patchStep(id: string, patch: Partial<RunStep>) {
    setRun((prev) => ({
      ...prev,
      steps: prev.steps.map((s) => (s.id === id ? { ...s, ...patch } : s)),
    }));
  }

  function editNext() {
    // Find the first planned step and surface "edit it" by scrolling and
    // letting StepsStream's row toggle into edit. The simplest move: force
    // the stream open so the user can see + click Edit on the planned row.
    const next = run.steps.find((s) => s.status === "planned");
    if (!next) return;
    // Best-effort scroll into view.
    requestAnimationFrame(() => {
      document
        .querySelector(`[data-step-id="${next.id}"]`)
        ?.scrollIntoView({ behavior: "smooth", block: "center" });
    });
  }

  function skip() {
    setRun((prev) => {
      const idx = prev.steps.findIndex((s) => s.status === "running" || s.status === "blocked");
      if (idx < 0) return prev;
      const steps = prev.steps.slice();
      steps[idx] = { ...steps[idx]!, status: "skipped" };
      // Promote the next planned step to running, if any.
      const nextIdx = steps.findIndex((s, i) => i > idx && s.status === "planned");
      if (nextIdx >= 0) steps[nextIdx] = { ...steps[nextIdx]!, status: "running" };
      return {
        ...prev,
        steps,
        currentActivity:
          nextIdx >= 0 ? steps[nextIdx]!.label : "Skipped step — nothing planned next",
      };
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <NowStrip
        run={run}
        onPause={() => setControl("paused")}
        onResume={() => setControl("running")}
        onEditNext={editNext}
        onSkip={skip}
        onStop={() => setControl("stopped")}
      />

      <DecisionQueue decisions={run.decisions} onDecide={decide} />

      <StepsStream
        steps={run.steps}
        control={run.control}
        onStepEdit={patchStep}
        defaultExpanded={run.control === "paused"}
      />
    </div>
  );
}
