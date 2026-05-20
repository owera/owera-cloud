"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { ComplexitySlider } from "@/components/ui/slider";
import {
  type Trigger,
  type TriggerKind,
  type Source,
  type Action,
  type ActionVerb,
  type Output,
  type OutputKind,
  type Approval,
  type ApprovalKind,
  type Memory,
  type MemoryKind,
  describeTrigger,
  describeSources,
  describeAction,
  describeOutput,
  describeApproval,
  ACTION_LABELS,
  APPROVAL_LABELS,
  MEMORY_LABELS,
  OUTPUT_LABELS,
  TRIGGER_LABELS,
} from "@/lib/compose/primitives";
import { type JobBlueprint } from "@/lib/compose/catalog";
import { getFunction } from "@/lib/compose/functions";
import { type ComposeState, toJson } from "@/lib/compose/state";
import {
  estimate,
  formatLatency,
  type CostEstimate,
} from "@/lib/compose/estimate";
import { complexityLevelLabel, type ComplexityLevel } from "@/lib/compose/levels";
import { formatCents } from "@/lib/format";
import type { SKU } from "@/lib/types";

interface JobCardProps {
  blueprint: JobBlueprint;
  initialState: ComposeState;
  skus: ReadonlyArray<SKU>;
  plan: "anonymous" | "free" | "paid";
}

/** The five slots are derived from the blueprint and then edited in place.
 *  When the user clicks Hire, we POST the same /api/compose JSON shape that
 *  v1/v2 used — the slot model is additive metadata on top. */
interface JobCardSlots {
  trigger: Trigger;
  sources: Source[];
  action: Action;
  output: Output;
  approval: Approval;
  memory: Memory;
}

/**
 * The job-card composer — v3's central UI.
 *
 * A single editable card with five slots (WHEN / READ / DO / DELIVER / CONFIRM)
 * laid out like a contract. Each line opens an inline editor on click. The
 * footer shows three cost columns (per-run / per-month projected / per-accepted
 * result) and a Quality dial expand-on-demand below.
 */
export function JobCardComposer({
  blueprint,
  initialState,
  skus,
  plan,
}: JobCardProps) {
  const router = useRouter();
  const fn = getFunction(blueprint.function);

  const [state, setState] = React.useState<ComposeState>(initialState);
  const [slots, setSlots] = React.useState<JobCardSlots>({
    trigger: blueprint.trigger,
    sources: [...blueprint.sources],
    action: blueprint.action,
    output: blueprint.output,
    approval: blueprint.approval,
    memory: blueprint.memory,
  });
  const [editing, setEditing] = React.useState<keyof JobCardSlots | null>(null);
  const [showAdvanced, setShowAdvanced] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  const est: CostEstimate = React.useMemo(
    () => estimate(state, skus),
    [state, skus],
  );

  const runsPerMonth = estimateRunsPerMonth(slots.trigger);
  const monthlyLow = est.centsLow * runsPerMonth;
  const monthlyHigh = est.centsHigh * runsPerMonth;
  const perAccepted = Math.round((est.centsLow + est.centsHigh) / 2);

  function setLevel(next: ComplexityLevel) {
    setState((prev) => ({ ...prev, level: next }));
  }

  async function onHire() {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      // Encode the slot edits into inputs.* so the agent sees them, while
      // keeping the v2 wire shape intact.
      const idemKey =
        state.idempotencyKey ??
        `compose-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const stateWithSlots: ComposeState = {
        ...state,
        idempotencyKey: idemKey,
        inputs: {
          ...state.inputs,
          owera_job_id: blueprint.id,
          owera_trigger: JSON.stringify(slots.trigger),
          owera_sources: JSON.stringify(slots.sources),
          owera_action_verb: slots.action.verb,
          owera_action_description: slots.action.description,
          owera_output: JSON.stringify(slots.output),
          owera_approval: JSON.stringify(slots.approval),
          owera_memory: JSON.stringify(slots.memory),
          owera_price_monthly_cents: String(blueprint.priceMonthly),
          owera_billing_unit: blueprint.billingUnit,
        },
      };
      const body = toJson(stateWithSlots);
      const res = await fetch("/api/compose", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const errBody = (await res.json().catch(() => null)) as
          | { code?: string; message?: string }
          | null;
        setError(
          `${errBody?.code ?? `http_${res.status}`}: ${errBody?.message ?? res.statusText}`,
        );
        setBusy(false);
        return;
      }
      const json = (await res.json()) as { job_id: string; shareUrl?: string };
      router.push(
        json.shareUrl ??
          `/jobs/${encodeURIComponent(json.job_id)}?from=compose&job=${blueprint.id}`,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setBusy(false);
    }
  }

  return (
    <div className="relative isolate">
      <div
        aria-hidden
        className="absolute inset-0 -z-10 overflow-hidden rounded-sm"
      >
        <div className="absolute inset-0 compose-grid" />
        <div className="absolute inset-0 compose-grain pointer-events-none" />
        <div
          className="absolute inset-0"
          style={{
            background:
              "radial-gradient(60% 60% at 30% 20%, rgba(91,141,239,0.06), transparent 60%)",
          }}
        />
      </div>

      <div className="relative max-w-4xl mx-auto px-4 sm:px-8 py-8 sm:py-12">
        <span className="compose-bracket tl" aria-hidden />
        <span className="compose-bracket tr" aria-hidden />
        <span className="compose-bracket bl" aria-hidden />
        <span className="compose-bracket br" aria-hidden />

        {/* Breadcrumb / kicker. */}
        <div className="flex items-center justify-between mb-6">
          <Link
            href={`/compose?fn=${blueprint.function}`}
            className="readout-label text-[var(--color-ink-dim)] hover:text-[var(--color-ink)]"
          >
            ← {fn.label} library
          </Link>
          {blueprint.killer && (
            <span className="readout-label text-[var(--color-stop-fill)]">
              ★ MOST HIRED
            </span>
          )}
        </div>

        {/* Title. */}
        <header className="mb-8 flex flex-col gap-2">
          <span className="readout-label">HIRING</span>
          <h1
            className="text-[2rem] sm:text-[2.6rem] leading-tight"
            style={{ fontFamily: "var(--font-display)" }}
          >
            {blueprint.name}.
          </h1>
          <p className="italic text-base text-[var(--color-ink-dim)]" style={{ fontFamily: "var(--font-display)" }}>
            {blueprint.tagline}
          </p>
        </header>

        {/* The five slots — the contract card. */}
        <div className="border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.25)] overflow-hidden">
          <SlotRow
            label="WHEN"
            value={describeTrigger(slots.trigger)}
            editing={editing === "trigger"}
            onEdit={() => setEditing(editing === "trigger" ? null : "trigger")}
            kicker="Trigger"
          >
            <TriggerEditor
              value={slots.trigger}
              onChange={(t) => setSlots((s) => ({ ...s, trigger: t }))}
            />
          </SlotRow>

          <SlotRow
            label="READ"
            value={describeSources(slots.sources)}
            editing={editing === "sources"}
            onEdit={() => setEditing(editing === "sources" ? null : "sources")}
            kicker="Sources"
          >
            <SourcesEditor
              value={slots.sources}
              onChange={(srcs) => setSlots((s) => ({ ...s, sources: srcs }))}
            />
          </SlotRow>

          <SlotRow
            label="DO"
            value={describeAction(slots.action)}
            editing={editing === "action"}
            onEdit={() => setEditing(editing === "action" ? null : "action")}
            kicker="Action"
          >
            <ActionEditor
              value={slots.action}
              onChange={(a) => setSlots((s) => ({ ...s, action: a }))}
            />
          </SlotRow>

          <SlotRow
            label="DELIVER"
            value={describeOutput(slots.output)}
            editing={editing === "output"}
            onEdit={() => setEditing(editing === "output" ? null : "output")}
            kicker="Output"
          >
            <OutputEditor
              value={slots.output}
              onChange={(o) => setSlots((s) => ({ ...s, output: o }))}
            />
          </SlotRow>

          <SlotRow
            label="CONFIRM"
            value={describeApproval(slots.approval)}
            editing={editing === "approval"}
            onEdit={() => setEditing(editing === "approval" ? null : "approval")}
            kicker="Approval"
            last
          >
            <ApprovalEditor
              value={slots.approval}
              onChange={(a) => setSlots((s) => ({ ...s, approval: a }))}
            />
          </SlotRow>
        </div>

        {/* Advanced toggle. */}
        <div className="mt-3 flex items-center gap-3">
          <button
            type="button"
            onClick={() => setShowAdvanced((s) => !s)}
            className="text-xs font-mono uppercase tracking-wider text-[var(--color-ink-dim)] hover:text-[var(--color-ink)]"
          >
            {showAdvanced ? "− Hide" : "+ Show"} memory · budget · SLA · quality dial
          </button>
          <span className="readout-label text-[var(--color-ink-dim)]/70">
            {MEMORY_LABELS[slots.memory.kind]} · SLA: {blueprint.sla}
          </span>
        </div>

        {showAdvanced && (
          <div className="mt-4 border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.18)] p-5 flex flex-col gap-6">
            {/* Memory */}
            <div className="flex flex-col gap-2">
              <span className="readout-label">Memory</span>
              <select
                value={slots.memory.kind}
                onChange={(e) =>
                  setSlots((s) => ({
                    ...s,
                    memory: { ...s.memory, kind: e.target.value as MemoryKind },
                  }))
                }
                className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
              >
                {(["stateless", "per-target", "per-job", "org-persistent"] as MemoryKind[]).map((k) => (
                  <option key={k} value={k}>
                    {MEMORY_LABELS[k]}
                  </option>
                ))}
              </select>
              {slots.memory.description && (
                <span className="text-xs text-[var(--color-ink-dim)] italic" style={{ fontFamily: "var(--font-display)" }}>
                  {slots.memory.description}
                </span>
              )}
            </div>

            {/* Quality dial */}
            <div className="flex flex-col gap-3">
              <div className="flex items-baseline justify-between">
                <span className="readout-label">Quality dial</span>
                <span className="readout-label text-[var(--color-ink)]">
                  {complexityLevelLabel(state.level)}
                </span>
              </div>
              <ComplexitySlider value={state.level} onChange={setLevel} />
            </div>
          </div>
        )}

        {/* Cost panel — the upsell story. */}
        <section className="mt-6 grid grid-cols-1 sm:grid-cols-3 border border-[var(--color-rule)] rounded-sm divide-y sm:divide-y-0 sm:divide-x divide-[var(--color-rule)] bg-[rgba(255,255,255,0.01)]">
          <CostCell label="Per run">
            {formatCents(est.centsLow)}
            <span className="text-[var(--color-ink-dim)] mx-1">–</span>
            {formatCents(est.centsHigh)}
          </CostCell>
          <CostCell label={`Per month · ~${runsPerMonth} runs`}>
            {formatCents(monthlyLow)}
            <span className="text-[var(--color-ink-dim)] mx-1">–</span>
            {formatCents(monthlyHigh)}
          </CostCell>
          <CostCell label={`Per ${blueprint.billingUnit.split(" ").slice(-1)[0]}`}>
            ≈ {formatCents(perAccepted)}
          </CostCell>
        </section>

        <p
          className="mt-2 italic text-xs text-[var(--color-ink-dim)]"
          style={{ fontFamily: "var(--font-display)" }}
        >
          Suggested subscription · {formatCents(blueprint.priceMonthly)}/mo, charged{" "}
          {blueprint.billingUnit}. {formatLatency(est.p50ms)} typical
          latency, p95 {formatLatency(est.p95ms)}.
        </p>

        {/* Action footer. */}
        <footer className="mt-8 pt-6 border-t border-[var(--color-rule)] flex flex-wrap items-center gap-4">
          <Button
            type="button"
            variant="primary"
            disabled={busy || plan === "anonymous"}
            onClick={onHire}
            className="h-11 px-6 text-sm tracking-wider font-medium"
          >
            {busy ? "Hiring…" : `Hire — ${formatCents(blueprint.priceMonthly)}/mo →`}
          </Button>

          <Button asChild variant="secondary" className="h-11 px-4">
            <Link href={`/compose?fn=${blueprint.function}`}>
              ← Browse other {fn.label.toLowerCase()} jobs
            </Link>
          </Button>

          <span className="readout-label flex-1 text-right">
            POST{" "}
            <a
              href={`/api/compose?job=${blueprint.id}`}
              target="_blank"
              rel="noreferrer"
              className="underline decoration-[var(--color-stop-fill)] decoration-1 underline-offset-4 hover:text-[var(--color-ink)]"
            >
              /api/compose?job={blueprint.id}
            </a>
          </span>

          {error && (
            <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--color-state-failed)] w-full">
              {error}
            </span>
          )}
        </footer>

        {plan === "anonymous" && (
          <p className="mt-4 text-xs text-[var(--color-ink-dim)]">
            Sign in to hire this job. Anonymous browsing is welcome.
          </p>
        )}

        <div className="mt-12 pt-4 border-t border-[var(--color-rule)] flex items-center justify-between">
          <span className="readout-label">OWERA · AGENTIC · NO. 01</span>
          <span className="readout-label">
            {blueprint.id.toUpperCase()}
          </span>
        </div>
      </div>
    </div>
  );
}

/* -------------------------- Slot row ---------------------------- */

function SlotRow({
  label,
  value,
  kicker,
  editing,
  onEdit,
  children,
  last,
}: {
  label: string;
  value: string;
  kicker: string;
  editing: boolean;
  onEdit: () => void;
  children: React.ReactNode;
  last?: boolean;
}) {
  return (
    <div
      className={`px-5 sm:px-6 py-4 ${last ? "" : "border-b border-[var(--color-rule)]"}`}
    >
      <button
        type="button"
        onClick={onEdit}
        className="w-full text-left flex items-start gap-4 group focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)] rounded-sm"
      >
        <span className="readout-label w-20 shrink-0 mt-1">{label}</span>
        <span className="flex-1 font-mono text-base text-[var(--color-ink)] leading-snug">
          {value}
        </span>
        <span className="readout-label text-[var(--color-ink-dim)] group-hover:text-[var(--color-stop-fill)] transition-colors">
          {editing ? "− " + kicker : "Edit"}
        </span>
      </button>
      {editing && (
        <div className="mt-3 pl-0 sm:pl-24">{children}</div>
      )}
    </div>
  );
}

/* -------------------------- Editors ---------------------------- */

function TriggerEditor({ value, onChange }: { value: Trigger; onChange: (t: Trigger) => void }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-2">
        {(["schedule", "event", "inbox", "manual", "threshold"] as TriggerKind[]).map((k) => (
          <ChipButton
            key={k}
            active={value.kind === k}
            onClick={() => onChange({ ...value, kind: k })}
          >
            {TRIGGER_LABELS[k]}
          </ChipButton>
        ))}
      </div>
      {value.kind === "schedule" && (
        <input
          type="text"
          value={value.cadence ?? ""}
          onChange={(e) => onChange({ ...value, cadence: e.target.value })}
          placeholder='e.g. "Every Monday at 7:00 AM"'
          className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
        />
      )}
      {(value.kind === "event" || value.kind === "inbox") && (
        <input
          type="text"
          value={value.signal ?? ""}
          onChange={(e) => onChange({ ...value, signal: e.target.value })}
          placeholder={
            value.kind === "event"
              ? 'e.g. "hubspot.form.submitted"'
              : 'e.g. "accounting@ — new PDF attachment"'
          }
          className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
        />
      )}
      {value.kind === "threshold" && (
        <input
          type="text"
          value={value.predicate ?? ""}
          onChange={(e) => onChange({ ...value, predicate: e.target.value })}
          placeholder='e.g. "ticket queue > 50"'
          className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
        />
      )}
    </div>
  );
}

function SourcesEditor({ value, onChange }: { value: Source[]; onChange: (s: Source[]) => void }) {
  return (
    <div className="flex flex-col gap-2">
      <span className="text-xs text-[var(--color-ink-dim)]">
        Showing the connectors the blueprint defaults to. Remove unused or add
        more after hire from the settings panel — full connector picker comes
        in the run page.
      </span>
      <div className="flex flex-wrap gap-2">
        {value.map((s, i) => (
          <span
            key={`${s.connector}-${i}`}
            className="inline-flex items-center gap-1.5 px-2 h-7 rounded-sm border border-[var(--color-rule)] bg-[rgba(0,0,0,0.2)] text-xs font-mono"
          >
            <span className="text-[var(--color-ink-dim)] uppercase tracking-wider">
              {s.category}
            </span>
            <span className="text-[var(--color-ink)]">
              {s.label ?? s.connector}
            </span>
            <button
              type="button"
              onClick={() => onChange(value.filter((_, j) => j !== i))}
              className="text-[var(--color-ink-dim)] hover:text-[var(--color-state-failed)]"
              aria-label="Remove source"
            >
              ×
            </button>
          </span>
        ))}
      </div>
    </div>
  );
}

function ActionEditor({ value, onChange }: { value: Action; onChange: (a: Action) => void }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-1.5">
        {(Object.keys(ACTION_LABELS) as ActionVerb[]).map((v) => (
          <ChipButton
            key={v}
            active={value.verb === v}
            onClick={() => onChange({ ...value, verb: v })}
          >
            {ACTION_LABELS[v]}
          </ChipButton>
        ))}
      </div>
      <textarea
        rows={2}
        value={value.description}
        onChange={(e) => onChange({ ...value, description: e.target.value })}
        className="rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 py-2 text-sm font-sans text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none resize-y"
      />
    </div>
  );
}

function OutputEditor({ value, onChange }: { value: Output; onChange: (o: Output) => void }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
        {(Object.keys(OUTPUT_LABELS) as OutputKind[]).map((k) => (
          <ChipButton
            key={k}
            active={value.kind === k}
            onClick={() => onChange({ ...value, kind: k })}
          >
            {OUTPUT_LABELS[k]}
          </ChipButton>
        ))}
      </div>
      <input
        type="text"
        value={value.target ?? ""}
        onChange={(e) => onChange({ ...value, target: e.target.value })}
        placeholder={
          value.kind === "slack"
            ? "#channel-name"
            : value.kind === "email-draft"
              ? "your@email.com"
              : value.kind === "table"
                ? "Google Sheet name or URL"
                : "destination (optional)"
        }
        className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
      />
    </div>
  );
}

function ApprovalEditor({ value, onChange }: { value: Approval; onChange: (a: Approval) => void }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        {(Object.keys(APPROVAL_LABELS) as ApprovalKind[]).map((k) => (
          <ChipButton
            key={k}
            active={value.kind === k}
            onClick={() => onChange({ ...value, kind: k })}
            className="justify-start text-left"
          >
            {APPROVAL_LABELS[k]}
          </ChipButton>
        ))}
      </div>
      <p
        className="italic text-xs text-[var(--color-ink-dim)] leading-snug"
        style={{ fontFamily: "var(--font-display)" }}
      >
        Higher autonomy = lower friction, higher trust risk. Start with
        Draft for sensitive outbound actions; widen as you trust the agent.
      </p>
    </div>
  );
}

function ChipButton({
  active,
  onClick,
  children,
  className,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        "px-3 h-8 rounded-sm border text-xs font-mono uppercase tracking-wider",
        "transition-colors",
        active
          ? "bg-[var(--color-stop-fill)]/15 border-[var(--color-stop-fill)] text-[var(--color-stop-fill)]"
          : "bg-transparent border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-ink)] hover:border-[var(--color-stop-fill)]/40",
        className ?? "",
      ].join(" ")}
    >
      {children}
    </button>
  );
}

function CostCell({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2 px-5 py-4">
      <span className="readout-label">{label}</span>
      <span className="readout-numeric text-[1.25rem] sm:text-[1.4rem] text-[var(--color-ink)]">
        {children}
      </span>
    </div>
  );
}

/* ------------------------------- helpers ------------------------------- */

function estimateRunsPerMonth(t: Trigger): number {
  if (t.kind !== "schedule") {
    // Conservative ballpark for event/inbox/threshold/manual.
    if (t.kind === "manual") return 1;
    return 20;
  }
  const c = (t.cadence ?? "").toLowerCase();
  if (c.includes("hour")) return 24 * 30;
  if (c.includes("daily") || c.includes("every day")) return 30;
  if (c.includes("weekly") || c.includes("every ")) return 4;
  if (c.includes("monthly") || c.includes("each month")) return 1;
  if (c.includes("quarter")) return 1 / 3;
  return 4;
}
