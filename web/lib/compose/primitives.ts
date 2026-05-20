// Compose primitives — the five slots that characterize 80% of jobs, plus
// three advanced ones.
//
//   Trigger  · when does this agent run?
//   Source   · what does it read?
//   Action   · what does it do?
//   Output   · where does the result land?
//   Approval · how much autonomy does the human grant?
//
// Advanced (collapsed by default in the UI):
//   Memory   · what does it remember across runs?
//   Budget   · cost & latency caps
//   Escalate · how does it ask for help?
//
// Research note: the 100-use-case survey for SMBs (see git history of this
// branch) found these eight to be the only primitives that recur across
// every domain. The composition UI must expose all five essentials; the
// advanced three are surfaced as defaults the user can override.

export type TriggerKind =
  | "schedule"
  | "event"
  | "inbox"
  | "manual"
  | "threshold";

export interface Trigger {
  kind: TriggerKind;
  /** Schedule kind: a single human-readable cadence string for now. */
  cadence?: string;
  /** Event source key for event/inbox/threshold (e.g. "stripe.invoice.payment_failed"). */
  signal?: string;
  /** Threshold predicate, free-text for now (e.g. "ticket queue > 50"). */
  predicate?: string;
}

/** Categorical source types — the integrations layer. */
export type SourceCategory =
  | "crm"
  | "inbox"
  | "chat"
  | "finance"
  | "people"
  | "productivity"
  | "telemetry"
  | "web"
  | "calendar"
  | "files";

export interface Source {
  category: SourceCategory;
  /** Specific integration slug. Resolved against the connector library. */
  connector: string;
  /** Optional human label. */
  label?: string;
}

export type ActionVerb =
  | "research"
  | "draft"
  | "classify"
  | "cluster"
  | "reconcile"
  | "monitor"
  | "summarize"
  | "coordinate"
  | "update"
  | "send"
  | "escalate";

export interface Action {
  verb: ActionVerb;
  /** One-line natural-language description of what the action does. */
  description: string;
}

export type OutputKind =
  | "memo"
  | "table"
  | "slack"
  | "email-draft"
  | "ticket"
  | "alert"
  | "dashboard"
  | "auto-send";

export interface Output {
  kind: OutputKind;
  /** Destination label. For slack: "#sales-hot". For email-draft: the inbox. */
  target?: string;
}

export type ApprovalKind =
  | "autonomous"
  | "draft-for-approval"
  | "always-ask"
  | "confidence-gated";

export interface Approval {
  kind: ApprovalKind;
  /** Confidence threshold for confidence-gated (0..1). */
  threshold?: number;
}

export type MemoryKind =
  | "stateless"
  | "per-target"
  | "per-job"
  | "org-persistent";

export interface Memory {
  kind: MemoryKind;
  /** Human description ("remembers facts about each prospect"). */
  description?: string;
}

export interface JobBudget {
  /** Max USD cents per run. */
  maxCentsPerRun?: number;
  /** Max wall-clock latency per run in seconds. */
  maxLatencySec?: number;
  /** Retry policy in plain language. */
  retryPolicy?: string;
}

export type EscalationKind = "slack" | "email" | "ticket" | "sms" | "queue";

export interface Escalation {
  kind: EscalationKind;
  /** Channel or role to ping (e.g. "@rodrigo" or "#ops"). */
  target?: string;
  /** Default action if a blocking question goes unanswered. */
  defaultAction?: string;
  /** Hours to wait before applying defaultAction. */
  defaultTimeoutHours?: number;
}

/* ----------------------- Display helpers ----------------------- */

export const TRIGGER_LABELS: Record<TriggerKind, string> = {
  schedule: "Schedule",
  event: "Event",
  inbox: "Inbox watch",
  manual: "Manual",
  threshold: "Threshold",
};

export const ACTION_LABELS: Record<ActionVerb, string> = {
  research: "Research",
  draft: "Draft",
  classify: "Classify",
  cluster: "Cluster",
  reconcile: "Reconcile",
  monitor: "Monitor",
  summarize: "Summarize",
  coordinate: "Coordinate",
  update: "Update",
  send: "Send",
  escalate: "Escalate",
};

export const OUTPUT_LABELS: Record<OutputKind, string> = {
  memo: "Memo",
  table: "Spreadsheet rows",
  slack: "Slack message",
  "email-draft": "Email draft",
  ticket: "Ticket / CRM update",
  alert: "Alert",
  dashboard: "Dashboard entry",
  "auto-send": "Auto-send (no approval)",
};

export const APPROVAL_LABELS: Record<ApprovalKind, string> = {
  autonomous: "Autonomous — act and report",
  "draft-for-approval": "Draft for my approval",
  "always-ask": "Always ask before acting",
  "confidence-gated": "Auto above confidence, draft below",
};

export const MEMORY_LABELS: Record<MemoryKind, string> = {
  stateless: "Stateless — every run independent",
  "per-target": "Per-target — remembers each subject",
  "per-job": "Per-job — knows what changed since last run",
  "org-persistent": "Org-persistent — knows our voice & playbook",
};

export const SOURCE_CONNECTORS: Record<
  SourceCategory,
  ReadonlyArray<{ slug: string; label: string }>
> = {
  crm: [
    { slug: "hubspot", label: "HubSpot" },
    { slug: "pipedrive", label: "Pipedrive" },
    { slug: "salesforce", label: "Salesforce" },
    { slug: "attio", label: "Attio" },
    { slug: "close", label: "Close" },
  ],
  inbox: [
    { slug: "gmail", label: "Gmail" },
    { slug: "outlook", label: "Outlook" },
    { slug: "intercom", label: "Intercom" },
    { slug: "front", label: "Front" },
    { slug: "helpscout", label: "Help Scout" },
    { slug: "zendesk", label: "Zendesk" },
  ],
  chat: [
    { slug: "slack", label: "Slack" },
    { slug: "discord", label: "Discord" },
    { slug: "teams", label: "Microsoft Teams" },
  ],
  finance: [
    { slug: "stripe", label: "Stripe" },
    { slug: "quickbooks", label: "QuickBooks" },
    { slug: "ramp", label: "Ramp" },
    { slug: "brex", label: "Brex" },
    { slug: "mercury", label: "Mercury" },
    { slug: "carta", label: "Carta" },
  ],
  people: [
    { slug: "greenhouse", label: "Greenhouse" },
    { slug: "ashby", label: "Ashby" },
    { slug: "lever", label: "Lever" },
    { slug: "bamboohr", label: "BambooHR" },
    { slug: "rippling", label: "Rippling" },
    { slug: "gusto", label: "Gusto" },
  ],
  productivity: [
    { slug: "gdrive", label: "Google Drive" },
    { slug: "gdocs", label: "Google Docs" },
    { slug: "gsheets", label: "Google Sheets" },
    { slug: "notion", label: "Notion" },
    { slug: "linear", label: "Linear" },
    { slug: "asana", label: "Asana" },
    { slug: "airtable", label: "Airtable" },
    { slug: "jira", label: "Jira" },
    { slug: "github", label: "GitHub" },
  ],
  telemetry: [
    { slug: "mixpanel", label: "Mixpanel" },
    { slug: "amplitude", label: "Amplitude" },
    { slug: "posthog", label: "PostHog" },
    { slug: "segment", label: "Segment" },
    { slug: "ga4", label: "Google Analytics 4" },
  ],
  web: [
    { slug: "web", label: "Web search" },
    { slug: "url", label: "Specific URL / page" },
    { slug: "rss", label: "RSS feed" },
    { slug: "apollo", label: "Apollo" },
    { slug: "linkedin", label: "LinkedIn" },
    { slug: "crunchbase", label: "Crunchbase" },
  ],
  calendar: [
    { slug: "gcal", label: "Google Calendar" },
    { slug: "calendly", label: "Calendly" },
    { slug: "calcom", label: "Cal.com" },
  ],
  files: [
    { slug: "pdf", label: "PDF" },
    { slug: "xlsx", label: "Spreadsheet" },
    { slug: "csv", label: "CSV" },
    { slug: "image", label: "Image" },
    { slug: "loom", label: "Loom recording" },
  ],
};

/** A connector slug looked up across all categories — returns a friendly label. */
export function connectorLabel(slug: string): string {
  for (const list of Object.values(SOURCE_CONNECTORS)) {
    const hit = list.find((c) => c.slug === slug);
    if (hit) return hit.label;
  }
  return slug;
}

/** Plain-language describer for a Trigger. */
export function describeTrigger(t: Trigger): string {
  switch (t.kind) {
    case "schedule":
      return t.cadence ? `On schedule: ${t.cadence}` : "On a schedule";
    case "event":
      return t.signal ? `When event fires: ${t.signal}` : "On an event";
    case "inbox":
      return t.signal ? `When ${t.signal} matches` : "On an inbox match";
    case "threshold":
      return t.predicate
        ? `When ${t.predicate}`
        : "When a threshold is crossed";
    case "manual":
      return "When I click Run";
  }
}

export function describeAction(a: Action): string {
  return a.description || ACTION_LABELS[a.verb];
}

export function describeOutput(o: Output): string {
  if (o.kind === "slack" && o.target) return `Post to Slack ${o.target}`;
  if (o.kind === "email-draft" && o.target) return `Draft email in ${o.target}`;
  if (o.kind === "table" && o.target) return `Append rows to ${o.target}`;
  if (o.target) return `${OUTPUT_LABELS[o.kind]} — ${o.target}`;
  return OUTPUT_LABELS[o.kind];
}

export function describeApproval(a: Approval): string {
  if (a.kind === "confidence-gated" && a.threshold !== undefined) {
    return `Auto above ${Math.round(a.threshold * 100)}% confidence, otherwise draft`;
  }
  return APPROVAL_LABELS[a.kind];
}

export function describeSources(sources: ReadonlyArray<Source>): string {
  if (sources.length === 0) return "(no sources yet)";
  const labels = sources.map((s) => s.label ?? connectorLabel(s.connector));
  if (labels.length === 1) return labels[0]!;
  if (labels.length === 2) return labels.join(" + ");
  return `${labels.slice(0, -1).join(", ")} + ${labels[labels.length - 1]!}`;
}
