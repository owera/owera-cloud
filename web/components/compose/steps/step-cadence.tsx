"use client";

import * as React from "react";
import type { ComposeState } from "@/lib/compose/state";
import {
  SCHEDULE_DEFAULTS,
  WEEKDAY_LABELS,
  type ScheduleKind,
  describeSchedule,
  detectTimezone,
} from "@/lib/compose/schedule";
import {
  DELIVERY_DEFAULTS,
  type DeliveryKind,
  describeDelivery,
} from "@/lib/compose/delivery";

interface StepCadenceProps {
  state: ComposeState;
  setState: React.Dispatch<React.SetStateAction<ComposeState>>;
}

/**
 * Step 3 — when does this job run, and where does the output go?
 * Schedule (once / daily / weekly / cron) + delivery (dashboard / email /
 * slack / webhook). Both are stored in state; execution wiring is a
 * separate backend concern.
 */
export function StepCadence({ state, setState }: StepCadenceProps) {
  // Capture user's tz once for new schedules so the scheduler doesn't drift.
  React.useEffect(() => {
    if (state.schedule.kind !== "once" && !state.schedule.timezone) {
      setState((prev) => ({
        ...prev,
        schedule: { ...prev.schedule, timezone: detectTimezone() },
      }));
    }
  }, [state.schedule.kind, state.schedule.timezone, setState]);

  function setScheduleKind(kind: ScheduleKind) {
    setState((prev) => ({
      ...prev,
      schedule: {
        ...SCHEDULE_DEFAULTS[kind],
        timezone: prev.schedule.timezone ?? detectTimezone(),
      },
    }));
  }

  function setDeliveryKind(kind: DeliveryKind) {
    setState((prev) => ({
      ...prev,
      delivery: { ...DELIVERY_DEFAULTS[kind] },
    }));
  }

  return (
    <div className="flex flex-col gap-10">
      {/* Cadence */}
      <section className="flex flex-col gap-4">
        <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2">
          <span className="readout-label">Cadence</span>
          <span className="readout-label">03 · WHEN TO RUN</span>
        </header>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
          {(
            [
              { k: "once", label: "Run once", caption: "Ad-hoc" },
              { k: "daily", label: "Daily", caption: "Every day" },
              { k: "weekly", label: "Weekly", caption: "Pick weekdays" },
              { k: "cron", label: "Cron", caption: "Power user" },
            ] as ReadonlyArray<{
              k: ScheduleKind;
              label: string;
              caption: string;
            }>
          ).map((opt) => {
            const isActive = state.schedule.kind === opt.k;
            return (
              <button
                key={opt.k}
                type="button"
                onClick={() => setScheduleKind(opt.k)}
                className={[
                  "border rounded-sm px-4 py-3 text-left flex flex-col gap-1",
                  "transition-colors",
                  "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]",
                  isActive
                    ? "border-[var(--color-stop-fill)] bg-[rgba(91,141,239,0.06)]"
                    : "border-[var(--color-rule)] hover:border-[var(--color-stop-fill)]/40",
                ].join(" ")}
                aria-pressed={isActive}
              >
                <span className="font-mono text-sm uppercase tracking-wider text-[var(--color-ink)]">
                  {opt.label}
                </span>
                <span className="text-xs text-[var(--color-ink-dim)]">
                  {opt.caption}
                </span>
              </button>
            );
          })}
        </div>

        {/* Schedule details that depend on the kind. */}
        {state.schedule.kind === "daily" && (
          <div className="flex items-center gap-3 mt-2">
            <span className="readout-label">At</span>
            <input
              type="time"
              value={state.schedule.time ?? "09:00"}
              onChange={(e) =>
                setState((prev) => ({
                  ...prev,
                  schedule: { ...prev.schedule, time: e.target.value },
                }))
              }
              className="h-9 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-2 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
            />
            <span className="text-xs text-[var(--color-ink-dim)]">
              {state.schedule.timezone ?? "your local time"}
            </span>
          </div>
        )}

        {state.schedule.kind === "weekly" && (
          <div className="flex flex-col gap-3 mt-2">
            <div className="flex items-center gap-3">
              <span className="readout-label">At</span>
              <input
                type="time"
                value={state.schedule.time ?? "09:00"}
                onChange={(e) =>
                  setState((prev) => ({
                    ...prev,
                    schedule: { ...prev.schedule, time: e.target.value },
                  }))
                }
                className="h-9 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-2 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
              />
              <span className="text-xs text-[var(--color-ink-dim)]">
                {state.schedule.timezone ?? "your local time"}
              </span>
            </div>
            <div>
              <span className="readout-label block mb-2">On</span>
              <div className="flex flex-wrap gap-1">
                {WEEKDAY_LABELS.map((label, i) => {
                  const on = (state.schedule.weekdays ?? []).includes(i);
                  return (
                    <button
                      key={i}
                      type="button"
                      onClick={() =>
                        setState((prev) => {
                          const cur = prev.schedule.weekdays ?? [];
                          const next = cur.includes(i)
                            ? cur.filter((d) => d !== i)
                            : [...cur, i].sort((a, b) => a - b);
                          return {
                            ...prev,
                            schedule: { ...prev.schedule, weekdays: next },
                          };
                        })
                      }
                      className={[
                        "h-8 w-12 rounded-sm border text-xs font-mono uppercase tracking-wider",
                        on
                          ? "bg-[var(--color-stop-fill)]/15 border-[var(--color-stop-fill)] text-[var(--color-stop-fill)]"
                          : "bg-transparent border-[var(--color-rule)] text-[var(--color-ink-dim)] hover:text-[var(--color-ink)] hover:border-[var(--color-stop-fill)]/40",
                      ].join(" ")}
                    >
                      {label}
                    </button>
                  );
                })}
              </div>
            </div>
          </div>
        )}

        {state.schedule.kind === "cron" && (
          <div className="flex flex-col gap-2 mt-2">
            <label className="readout-label">Expression</label>
            <input
              type="text"
              spellCheck={false}
              value={state.schedule.cron ?? "0 9 * * 1-5"}
              onChange={(e) =>
                setState((prev) => ({
                  ...prev,
                  schedule: { ...prev.schedule, cron: e.target.value },
                }))
              }
              className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
              placeholder="0 9 * * 1-5"
            />
            <span className="text-xs text-[var(--color-ink-dim)]">
              Five-field cron. Default runs at 09:00 every weekday in{" "}
              {state.schedule.timezone ?? "your local time"}.
            </span>
          </div>
        )}

        <p
          className="italic text-sm text-[var(--color-ink-dim)] mt-1"
          style={{ fontFamily: "var(--font-display)" }}
        >
          {describeSchedule(state.schedule)}
        </p>
      </section>

      {/* Delivery */}
      <section className="flex flex-col gap-4">
        <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2">
          <span className="readout-label">Delivery</span>
          <span className="readout-label">WHERE RESULTS LAND</span>
        </header>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
          {(
            [
              { k: "dashboard", label: "Dashboard", caption: "Default · always works" },
              { k: "email", label: "Email", caption: "To an address" },
              { k: "slack", label: "Slack", caption: "To a channel" },
              { k: "webhook", label: "Webhook", caption: "POST to a URL" },
            ] as ReadonlyArray<{
              k: DeliveryKind;
              label: string;
              caption: string;
            }>
          ).map((opt) => {
            const isActive = state.delivery.kind === opt.k;
            return (
              <button
                key={opt.k}
                type="button"
                onClick={() => setDeliveryKind(opt.k)}
                className={[
                  "border rounded-sm px-4 py-3 text-left flex flex-col gap-1",
                  "transition-colors",
                  "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]",
                  isActive
                    ? "border-[var(--color-stop-fill)] bg-[rgba(91,141,239,0.06)]"
                    : "border-[var(--color-rule)] hover:border-[var(--color-stop-fill)]/40",
                ].join(" ")}
                aria-pressed={isActive}
              >
                <span className="font-mono text-sm uppercase tracking-wider text-[var(--color-ink)]">
                  {opt.label}
                </span>
                <span className="text-xs text-[var(--color-ink-dim)]">
                  {opt.caption}
                </span>
              </button>
            );
          })}
        </div>

        {state.delivery.kind !== "dashboard" && (
          <div className="flex flex-col gap-2 mt-2">
            <label className="readout-label">
              {state.delivery.kind === "email"
                ? "Email address"
                : state.delivery.kind === "slack"
                  ? "Slack channel"
                  : "Webhook URL"}
            </label>
            <input
              type={
                state.delivery.kind === "webhook"
                  ? "url"
                  : state.delivery.kind === "email"
                    ? "email"
                    : "text"
              }
              value={state.delivery.target ?? ""}
              onChange={(e) =>
                setState((prev) => ({
                  ...prev,
                  delivery: { ...prev.delivery, target: e.target.value },
                }))
              }
              placeholder={
                state.delivery.kind === "email"
                  ? "alerts@yourco.com"
                  : state.delivery.kind === "slack"
                    ? "#owera-alerts"
                    : "https://hooks.yourco.com/owera"
              }
              className="h-10 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none"
            />
          </div>
        )}

        <p
          className="italic text-sm text-[var(--color-ink-dim)] mt-1"
          style={{ fontFamily: "var(--font-display)" }}
        >
          {describeDelivery(state.delivery)}
        </p>
      </section>
    </div>
  );
}
