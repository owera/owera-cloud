// Run state — what an executing job looks like to the cockpit UI.
//
// The backend doesn't yet expose this shape. The adapter below
// (`buildRunStateFromLedger`) projects today's JobLedger into a richer
// RunState so the cockpit components have something to render against
// fixture data immediately. When the API surfaces this natively, we keep
// the same shape and swap the adapter for a direct fetch.

import type { Job, JobLedgerEntry } from "@/lib/types";

export type StepStatus =
  | "planned"     // not yet started
  | "running"     // currently active
  | "done"        // completed successfully
  | "blocked"     // waiting on a decision
  | "skipped"     // skipped (by operator or by agent policy)
  | "edited"      // user edited the step while paused
  | "failed";

export interface StepIO {
  /** Human-readable input summary. */
  input?: string;
  /** Human-readable output summary, when done. */
  output?: string;
  /** Cost in USD cents accrued by this step. */
  costCents?: number;
  /** Duration in ms (only set when done). */
  durationMs?: number;
}

export interface RunStep {
  id: string;
  /** 1-based human ordinal — what the cockpit prints. */
  ordinal: number;
  /** Short imperative label. */
  label: string;
  /** Long-form description; can be edited while paused. */
  detail?: string;
  /** Which tool/connector this step uses. Editable. */
  tool?: string;
  /** Max cents this step is allowed to spend. Editable. */
  maxCents?: number;
  status: StepStatus;
  /** ISO when the step transitioned to its current status. */
  ts: string;
  io: StepIO;
  /** True if this step's source ledger entry was an explicit milestone
   *  (tool call or state change) rather than a logged line. */
  milestone?: boolean;
}

export type DecisionKind =
  | "blocking-question"  // agent is stuck and needs a yes/no/answer
  | "approval-draft"     // agent prepared an artifact awaiting send
  | "non-blocking-flag"; // agent made a judgment call, FYI

export interface RunDecision {
  id: string;
  kind: DecisionKind;
  /** Short prompt for the operator. */
  title: string;
  /** Long-form context. */
  detail?: string;
  /** ISO when the decision was raised. */
  ts: string;
  /** For blocking questions: the default action if the user doesn't respond. */
  defaultAction?: string;
  /** For blocking questions: ISO when the default fires. */
  defaultAt?: string;
  /** For approval-draft: a preview snippet of the artifact. */
  preview?: string;
}

export type ControlState = "running" | "paused" | "stopped";

export interface RunState {
  /** The job this run belongs to. */
  jobId: string;
  /** What the agent is doing RIGHT NOW, one sentence. */
  currentActivity: string;
  /** ETA for current step in ms, if known. */
  etaMs?: number;
  /** 0..1 confidence the agent has on its current step. */
  confidence?: number;
  /** Top-level control state — drives the brake-pedal UI. */
  control: ControlState;
  /** Last ts the agent emitted any signal. */
  lastHeartbeatTs: string;
  /** Steps in order, planned + executed. */
  steps: ReadonlyArray<RunStep>;
  /** Items the human is asked to decide on. */
  decisions: ReadonlyArray<RunDecision>;
}

/* ---------------- adapter from JobLedger → RunState ---------------- */

/**
 * Build a richer run state from the current ledger shape. This is a
 * **best-effort** projection so the cockpit can render against today's
 * fixtures; the backend will own this shape natively in a later PR.
 *
 * Heuristics:
 *   - `state_change` entries become milestone steps (system bookkeeping).
 *   - `tool_call` entries become done steps with a humanized label.
 *   - `output` entries do not become steps (they belong on the receipt).
 *   - `log` entries refresh `currentActivity` (we keep the latest).
 *   - If the job state is `running`, the last step is promoted to `running`
 *     and we synthesize a couple of `planned` follow-ups so the operator
 *     sees forward motion (this is fixture-only — the backend will know
 *     the real plan).
 */
export function buildRunStateFromLedger(
  job: Job,
  ledger: ReadonlyArray<JobLedgerEntry>,
): RunState {
  const steps: RunStep[] = [];
  let ordinal = 0;
  let latestLog: JobLedgerEntry | undefined;
  for (const e of ledger) {
    if (e.kind === "log") {
      latestLog = e;
      continue;
    }
    if (e.kind === "output" || e.kind === "billing") continue;
    ordinal += 1;
    const label =
      e.kind === "state_change" ? humanizeState(e.message) : humanizeTool(e.message);
    steps.push({
      id: e.id,
      ordinal,
      label,
      detail: e.message,
      tool: e.kind === "tool_call" ? toolFromMessage(e.message) : undefined,
      status: "done",
      ts: e.ts,
      io: {
        input: e.kind === "tool_call" ? extractCallArgs(e.message) : undefined,
        output:
          e.data && typeof e.data === "object" && "results" in e.data
            ? `${(e.data as Record<string, unknown>).results} results`
            : undefined,
      },
      milestone: true,
    });
  }

  const isRunning = job.state === "running" || job.state === "queued";
  if (isRunning && steps.length > 0) {
    // Promote the last completed step to "running" to express "this is happening now."
    const last = steps[steps.length - 1]!;
    last.status = "running";
    // Synthesize two upcoming planned steps so the operator sees
    // what's next. Real backend will provide the actual plan.
    steps.push(
      synthPlanned(ordinal + 1, "Enrich results with company + role context", "research"),
      synthPlanned(ordinal + 2, "Draft per-target output for human review", "draft"),
    );
  }

  // Fixture decisions — only when running, only on a fixture job. These let
  // the cockpit demonstrate the decision queue against today's data.
  const decisions: RunDecision[] = isRunning
    ? [
        {
          id: "dec-fixture-block-1",
          kind: "blocking-question",
          title: "Send the 5 outbound emails I drafted?",
          detail:
            "I have 5 first-touch drafts ready for prospects you've never reached. I'll proceed with default after the timer if you don't respond.",
          ts: job.startedAt ?? job.submittedAt,
          defaultAction: "Hold and notify in the morning brief",
          defaultAt: addHours(new Date(job.submittedAt), 2).toISOString(),
          preview:
            "Subject: A tactical thought after seeing your Series A — happy to share what we learned…",
        },
        {
          id: "dec-fixture-flag-1",
          kind: "non-blocking-flag",
          title: "Skipped 4 prospects below 50-employee ICP floor",
          detail:
            "All 4 are sub-50 and have <$1M ARR. I noted them in the 'review later' tab so you can override the ICP rule if you want.",
          ts: job.startedAt ?? job.submittedAt,
        },
        {
          id: "dec-fixture-approval-1",
          kind: "approval-draft",
          title: "Weekly summary memo ready for review",
          detail: "Drafted the Monday pipeline standup memo from this week's HubSpot activity.",
          ts: job.startedAt ?? job.submittedAt,
          preview:
            "Pipeline moved $48k in Slack week-over-week. 3 deals slipped to next month — Acme, Brookfield, and Halton. Suggest a save call on Acme today.",
        },
      ]
    : [];

  return {
    jobId: job.id,
    currentActivity:
      latestLog?.message ??
      (isRunning ? steps.find((s) => s.status === "running")?.label ?? "Working…" : "Idle"),
    etaMs: isRunning ? 14 * 60 * 1000 : undefined,
    confidence: isRunning ? 0.78 : undefined,
    control: job.state === "cancelled" ? "stopped" : isRunning ? "running" : "stopped",
    lastHeartbeatTs:
      ledger[ledger.length - 1]?.ts ?? job.startedAt ?? job.submittedAt,
    steps,
    decisions,
  };
}

function humanizeState(message: string): string {
  if (message.includes("submitted")) return "Job submitted";
  if (message.includes("queued") && message.includes("running"))
    return "Worker picked up the job";
  if (message.includes("succeeded")) return "Finished successfully";
  if (message.includes("failed")) return "Stopped — error encountered";
  if (message.includes("cancelled")) return "Cancelled";
  return `State: ${message}`;
}

function humanizeTool(message: string): string {
  if (message.startsWith("web.search")) return "Searched the web";
  if (message.startsWith("hubspot.")) return "Read from HubSpot";
  if (message.startsWith("stripe.")) return "Read from Stripe";
  if (message.startsWith("apollo.")) return "Read from Apollo";
  if (message.startsWith("gmail.")) return "Checked inbox";
  if (message.startsWith("calendar.")) return "Checked the calendar";
  if (message.startsWith("slack.")) return "Posted to Slack";
  if (message.startsWith("github.")) return "Read from GitHub";
  const dot = message.indexOf(".");
  const paren = message.indexOf("(");
  if (dot > 0 && paren > dot) {
    return `Used ${message.slice(0, dot)}: ${message.slice(dot + 1, paren)}`;
  }
  return message;
}

function toolFromMessage(message: string): string | undefined {
  const dot = message.indexOf(".");
  return dot > 0 ? message.slice(0, dot) : undefined;
}

function extractCallArgs(message: string): string | undefined {
  const open = message.indexOf("(");
  const close = message.lastIndexOf(")");
  if (open >= 0 && close > open) return message.slice(open + 1, close);
  return undefined;
}

function addHours(d: Date, hours: number): Date {
  const out = new Date(d.getTime());
  out.setHours(out.getHours() + hours);
  return out;
}

function synthPlanned(ordinal: number, label: string, tool: string): RunStep {
  return {
    id: `synth-${ordinal}`,
    ordinal,
    label,
    detail: label,
    tool,
    status: "planned",
    ts: new Date().toISOString(),
    io: {},
    milestone: false,
  };
}
