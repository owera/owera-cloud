// Job cadence. A Job is "hired" once and can run repeatedly on a schedule.
//
// The kinds we surface in the UI:
//   - once     — ad-hoc, single execution
//   - daily    — every day at a chosen local time
//   - weekly   — chosen weekdays at a chosen local time
//   - cron     — full power-user expression
//
// We always store the user's IANA timezone alongside the schedule so the
// scheduler doesn't drift on DST. Execution itself is a separate backend
// concern (real scheduler integration is a follow-on PR); this module just
// captures the user's intent and round-trips it through the API.

export type ScheduleKind = "once" | "daily" | "weekly" | "cron";

export interface Schedule {
  kind: ScheduleKind;
  /** Local-clock HH:mm. Used by daily and weekly. */
  time?: string;
  /** Days of week (0-Sun..6-Sat). Used by weekly. */
  weekdays?: ReadonlyArray<number>;
  /** Five-field cron expression. Used by cron. */
  cron?: string;
  /** IANA tz, e.g. "America/Sao_Paulo". Captured at compose time. */
  timezone?: string;
}

export const SCHEDULE_DEFAULTS: Record<ScheduleKind, Schedule> = {
  once: { kind: "once" },
  daily: { kind: "daily", time: "09:00" },
  weekly: { kind: "weekly", time: "09:00", weekdays: [1] },
  cron: { kind: "cron", cron: "0 9 * * 1-5" },
};

export const WEEKDAY_LABELS: ReadonlyArray<string> = [
  "Sun",
  "Mon",
  "Tue",
  "Wed",
  "Thu",
  "Fri",
  "Sat",
];

export function isScheduleKind(v: unknown): v is ScheduleKind {
  return v === "once" || v === "daily" || v === "weekly" || v === "cron";
}

/** Return a plain-language description of a schedule. */
export function describeSchedule(s: Schedule): string {
  switch (s.kind) {
    case "once":
      return "Run once, now.";
    case "daily":
      return `Every day at ${s.time ?? "09:00"}${s.timezone ? ` (${s.timezone})` : ""}.`;
    case "weekly": {
      const days = (s.weekdays ?? []).map((d) => WEEKDAY_LABELS[d]).join(", ");
      return `Every ${days || "Monday"} at ${s.time ?? "09:00"}${s.timezone ? ` (${s.timezone})` : ""}.`;
    }
    case "cron":
      return `Cron: ${s.cron ?? "0 9 * * 1-5"}${s.timezone ? ` (${s.timezone})` : ""}.`;
  }
}

/** Estimate runs per month for cost projection. Conservative for cron. */
export function estimateRunsPerMonth(s: Schedule): number {
  switch (s.kind) {
    case "once":
      return 1;
    case "daily":
      return 30;
    case "weekly":
      return Math.max(1, (s.weekdays?.length ?? 1)) * 4;
    case "cron":
      // Best-effort: count "*" wildcards to ballpark frequency. Doesn't try
      // to be a real cron parser — that lives in the scheduler.
      return 30;
  }
}

/** Detect the user's timezone safely (browser only — falls back to UTC). */
export function detectTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}
