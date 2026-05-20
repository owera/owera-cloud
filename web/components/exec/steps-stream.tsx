"use client";

import * as React from "react";
import type { RunStep, ControlState } from "@/lib/exec/run-state";
import { shortTimestamp } from "@/lib/format";
import { formatCents } from "@/lib/format";

interface StepsStreamProps {
  steps: ReadonlyArray<RunStep>;
  control: ControlState;
  /** Called when an upcoming step is edited while paused. */
  onStepEdit?: (id: string, patch: Partial<RunStep>) => void;
  /** Whether the stream is expanded by default. */
  defaultExpanded?: boolean;
}

/**
 * The steps stream — collapsed transcript of what's happened and what's
 * planned. Each step has a status, duration, and an inputs/outputs
 * disclosure. When the run is paused, upcoming steps (status=planned)
 * are editable in place: change the detail prompt, swap the tool, raise
 * the budget. Resume picks up from there.
 *
 * Past steps remain read-only — the agent's history is the audit trail.
 */
export function StepsStream({
  steps,
  control,
  onStepEdit,
  defaultExpanded = false,
}: StepsStreamProps) {
  const [expanded, setExpanded] = React.useState(defaultExpanded);

  // Auto-expand when the run pauses so the operator sees the upcoming
  // planned steps + Edit affordance without an extra click. Only fires
  // on the running→paused transition, not on every render.
  const prevControl = React.useRef(control);
  React.useEffect(() => {
    if (prevControl.current !== "paused" && control === "paused") {
      setExpanded(true);
    }
    prevControl.current = control;
  }, [control]);

  const counts = React.useMemo(() => countByStatus(steps), [steps]);

  return (
    <section className="border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.18)]">
      <header className="flex items-center justify-between border-b border-[var(--color-rule)] px-5 py-3">
        <button
          type="button"
          onClick={() => setExpanded((s) => !s)}
          className="readout-label hover:text-[var(--color-ink)]"
        >
          {expanded ? "− Steps" : "+ Steps"}
        </button>
        <span className="readout-label text-[var(--color-ink-dim)]">
          {counts.done} DONE · {counts.running} RUNNING · {counts.planned} PLANNED
          {counts.blocked > 0 && ` · ${counts.blocked} BLOCKED`}
          {counts.failed > 0 && ` · ${counts.failed} FAILED`}
        </span>
      </header>

      {expanded && (
        <ol className="flex flex-col divide-y divide-[var(--color-rule)]">
          {steps.map((step) => (
            <StepRow
              key={step.id}
              step={step}
              editable={control === "paused" && step.status === "planned"}
              onEdit={onStepEdit}
            />
          ))}
        </ol>
      )}
    </section>
  );
}

function StepRow({
  step,
  editable,
  onEdit,
}: {
  step: RunStep;
  editable: boolean;
  onEdit?: (id: string, patch: Partial<RunStep>) => void;
}) {
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState(false);
  const [draftDetail, setDraftDetail] = React.useState(step.detail ?? "");
  const [draftTool, setDraftTool] = React.useState(step.tool ?? "");
  const [draftMax, setDraftMax] = React.useState<string>(
    step.maxCents !== undefined ? String(step.maxCents) : "",
  );

  function commit() {
    const patch: Partial<RunStep> = {
      detail: draftDetail || undefined,
      tool: draftTool || undefined,
      maxCents: draftMax ? Math.max(0, Number(draftMax)) : undefined,
      status: "edited",
    };
    onEdit?.(step.id, patch);
    setEditing(false);
  }

  const tone = toneFor(step.status);
  const showDisclosure = step.io.input || step.io.output || step.io.costCents !== undefined;

  return (
    <li
      className={`px-5 py-3 flex flex-col gap-2 ${tone.bg}`}
      data-status={step.status}
    >
      <div className="flex items-start gap-3">
        <span
          className={`readout-label w-8 shrink-0 mt-1 ${tone.label}`}
          aria-hidden
        >
          {String(step.ordinal).padStart(2, "0")}
        </span>
        <span
          className={`text-base font-mono leading-snug mt-0.5 w-4 shrink-0 ${tone.glyph}`}
          aria-hidden
          title={step.status}
        >
          {glyphFor(step.status)}
        </span>
        <div className="flex-1 min-w-0">
          <p className="text-sm text-[var(--color-ink)] leading-snug">
            {step.label}
          </p>
          {step.detail && step.detail !== step.label && !editing && (
            <p className="mt-1 text-xs text-[var(--color-ink-dim)] leading-snug">
              {step.detail}
            </p>
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {step.io.durationMs !== undefined && (
            <span className="readout-label text-[var(--color-ink-dim)]">
              {Math.round(step.io.durationMs / 100) / 10}s
            </span>
          )}
          <span className="readout-label text-[var(--color-ink-dim)]">
            {shortTimestamp(step.ts).split(" ").slice(-2).join(" ")}
          </span>
          {editable && !editing && (
            <button
              type="button"
              onClick={() => setEditing(true)}
              className="readout-label text-[var(--color-stop-fill)] hover:underline"
            >
              Edit
            </button>
          )}
          {showDisclosure && !editing && (
            <button
              type="button"
              onClick={() => setOpen((s) => !s)}
              className="readout-label text-[var(--color-ink-dim)] hover:text-[var(--color-ink)]"
            >
              {open ? "−" : "+"} I/O
            </button>
          )}
        </div>
      </div>

      {/* Inline editor — only visible while paused on upcoming steps. */}
      {editing && (
        <div className="ml-12 mt-1 flex flex-col gap-2 border-l border-[var(--color-stop-fill)] pl-4">
          <label className="flex flex-col gap-1">
            <span className="readout-label">Instructions for this step</span>
            <textarea
              rows={2}
              value={draftDetail}
              onChange={(e) => setDraftDetail(e.target.value)}
              className="rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 py-2 text-sm font-sans text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none resize-y"
            />
          </label>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <label className="flex flex-col gap-1">
              <span className="readout-label">Tool</span>
              <input
                type="text"
                value={draftTool}
                onChange={(e) => setDraftTool(e.target.value)}
                placeholder="research / draft / web / hubspot…"
                className="h-9 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="readout-label">Max cents for this step</span>
              <input
                type="number"
                min={0}
                value={draftMax}
                onChange={(e) => setDraftMax(e.target.value)}
                placeholder="auto"
                className="h-9 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
              />
            </label>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={commit}
              className="h-8 px-3 rounded-sm border border-[var(--color-stop-fill)] bg-[var(--color-stop-fill)]/15 text-[var(--color-stop-fill)] text-xs font-mono uppercase tracking-wider hover:bg-[var(--color-stop-fill)]/25"
            >
              Save · resume from here
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="h-8 px-3 rounded-sm border border-[var(--color-rule)] text-[var(--color-ink-dim)] text-xs font-mono uppercase tracking-wider hover:text-[var(--color-ink)]"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* I/O disclosure */}
      {open && !editing && showDisclosure && (
        <div className="ml-12 mt-1 flex flex-col gap-2 border-l border-[var(--color-rule)] pl-4">
          {step.io.input && (
            <div>
              <span className="readout-label">Input</span>
              <p className="mt-1 text-xs font-mono text-[var(--color-ink-dim)] leading-snug whitespace-pre-wrap">
                {step.io.input}
              </p>
            </div>
          )}
          {step.io.output && (
            <div>
              <span className="readout-label">Output</span>
              <p className="mt-1 text-xs font-mono text-[var(--color-ink)] leading-snug whitespace-pre-wrap">
                {step.io.output}
              </p>
            </div>
          )}
          {step.io.costCents !== undefined && (
            <div className="flex items-baseline gap-2">
              <span className="readout-label">Spent</span>
              <span className="readout-numeric text-sm text-[var(--color-ink)]">
                {formatCents(step.io.costCents)}
              </span>
            </div>
          )}
        </div>
      )}
    </li>
  );
}

function glyphFor(status: RunStep["status"]): string {
  switch (status) {
    case "done":
      return "✓";
    case "running":
      return "○";
    case "planned":
      return "·";
    case "blocked":
      return "‖";
    case "skipped":
      return "↷";
    case "edited":
      return "✎";
    case "failed":
      return "✗";
  }
}

function toneFor(
  status: RunStep["status"],
): { label: string; glyph: string; bg: string } {
  switch (status) {
    case "done":
      return {
        label: "text-[var(--color-stop-fill)]",
        glyph: "text-[var(--color-stop-fill)]",
        bg: "",
      };
    case "running":
      return {
        label: "text-[var(--color-state-running)]",
        glyph: "text-[var(--color-state-running)]",
        bg: "bg-[rgba(245,158,11,0.04)]",
      };
    case "planned":
      return {
        label: "text-[var(--color-ink-dim)]",
        glyph: "text-[var(--color-ink-dim)]",
        bg: "",
      };
    case "blocked":
      return {
        label: "text-[var(--color-state-queued)]",
        glyph: "text-[var(--color-state-queued)]",
        bg: "bg-[rgba(59,130,246,0.04)]",
      };
    case "skipped":
      return {
        label: "text-[var(--color-ink-dim)]",
        glyph: "text-[var(--color-ink-dim)]",
        bg: "",
      };
    case "edited":
      return {
        label: "text-[var(--color-stop-fill)]",
        glyph: "text-[var(--color-stop-fill)]",
        bg: "bg-[rgba(91,141,239,0.05)]",
      };
    case "failed":
      return {
        label: "text-[var(--color-state-failed)]",
        glyph: "text-[var(--color-state-failed)]",
        bg: "bg-[rgba(239,68,68,0.04)]",
      };
  }
}

function countByStatus(
  steps: ReadonlyArray<RunStep>,
): Record<RunStep["status"], number> {
  const out: Record<RunStep["status"], number> = {
    done: 0,
    running: 0,
    planned: 0,
    blocked: 0,
    skipped: 0,
    edited: 0,
    failed: 0,
  };
  for (const s of steps) out[s.status] += 1;
  return out;
}
