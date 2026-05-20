"use client";

import * as React from "react";
import Link from "next/link";
import { FUNCTIONS, type FunctionId } from "@/lib/compose/functions";
import {
  JOB_CATALOG,
  jobsByFunction,
  type JobBlueprint,
} from "@/lib/compose/catalog";
import { formatCents } from "@/lib/format";

interface JobLibraryProps {
  /** Initial function filter (from `?fn=` or null = "all"). */
  initialFunction?: FunctionId | null;
}

/**
 * /compose front door v3 — the job library.
 *
 * Left rail: 10 business functions + counts. Main: job cards filtered by
 * the selected function. Each card is a hireable outcome with a clear
 * monthly price and a one-click "Hire" action that deep-links into the
 * job-card composer with the blueprint pre-loaded.
 */
export function JobLibrary({ initialFunction }: JobLibraryProps) {
  const [selected, setSelected] = React.useState<FunctionId | "killer" | "all">(
    initialFunction ?? "killer",
  );
  const [search, setSearch] = React.useState("");

  const visible = React.useMemo<JobBlueprint[]>(() => {
    let list = JOB_CATALOG.slice();
    if (selected === "killer") list = list.filter((j) => j.killer);
    else if (selected !== "all") list = jobsByFunction(selected);
    if (search) {
      const q = search.toLowerCase();
      list = list.filter(
        (j) =>
          j.name.toLowerCase().includes(q) ||
          j.tagline.toLowerCase().includes(q),
      );
    }
    return list;
  }, [selected, search]);

  return (
    <div className="grid grid-cols-1 lg:grid-cols-[14rem_1fr] gap-8">
      {/* Function rail. */}
      <aside className="flex flex-col gap-3">
        <span className="readout-label">Browse by function</span>
        <nav className="flex flex-col gap-0.5">
          <RailButton
            active={selected === "killer"}
            onClick={() => setSelected("killer")}
            glyph="★"
            label="Most hired"
            count={JOB_CATALOG.filter((j) => j.killer).length}
            tone="primary"
          />
          <RailButton
            active={selected === "all"}
            onClick={() => setSelected("all")}
            glyph="∞"
            label="All jobs"
            count={JOB_CATALOG.length}
          />
          <div className="h-px bg-[var(--color-rule)] my-1.5" />
          {FUNCTIONS.map((f) => (
            <RailButton
              key={f.id}
              active={selected === f.id}
              onClick={() => setSelected(f.id)}
              glyph={f.glyph}
              label={f.label}
              count={jobsByFunction(f.id).length}
            />
          ))}
        </nav>

        <div className="border-t border-[var(--color-rule)] pt-3 mt-3 flex flex-col gap-1">
          <span className="readout-label">For agents</span>
          <Link
            href="/api/compose/schema"
            className="text-xs text-[var(--color-ink-dim)] hover:text-[var(--color-ink)]"
          >
            /api/compose/schema
          </Link>
          <Link
            href="/llms.txt"
            className="text-xs text-[var(--color-ink-dim)] hover:text-[var(--color-ink)]"
          >
            /llms.txt
          </Link>
        </div>
      </aside>

      {/* Job grid. */}
      <section className="flex flex-col gap-5">
        <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-3">
          <div className="flex flex-col gap-1">
            <span className="readout-label">
              {sectionLabel(selected)} ·{" "}
              <span className="text-[var(--color-ink-dim)]">
                {visible.length} JOB(S)
              </span>
            </span>
            <h2
              className="text-[1.6rem] leading-tight"
              style={{ fontFamily: "var(--font-display)" }}
            >
              <em>Hire</em> an agent that does this job, every week.
            </h2>
          </div>
          <input
            type="search"
            placeholder="Search 100 jobs…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-9 w-56 rounded-sm border bg-[rgba(0,0,0,0.25)] border-[var(--color-rule)] px-3 text-sm font-mono text-[var(--color-ink)] focus:border-[var(--color-stop-fill)] focus:outline-none placeholder:text-[var(--color-ink-dim)]/70"
          />
        </header>

        {visible.length === 0 && (
          <div className="border border-dashed border-[var(--color-rule)] rounded-sm py-12 text-center text-[var(--color-ink-dim)]">
            No jobs match — try a different filter or search.
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
          {visible.map((job) => (
            <JobCard key={job.id} job={job} />
          ))}
        </div>
      </section>
    </div>
  );
}

function sectionLabel(s: FunctionId | "killer" | "all"): string {
  if (s === "killer") return "MOST HIRED";
  if (s === "all") return "ALL JOBS";
  return FUNCTIONS.find((f) => f.id === s)!.label.toUpperCase();
}

function RailButton({
  active,
  onClick,
  glyph,
  label,
  count,
  tone,
}: {
  active: boolean;
  onClick: () => void;
  glyph: string;
  label: string;
  count: number;
  tone?: "primary";
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        "flex items-center justify-between text-left px-2 py-1.5 rounded-sm",
        "transition-colors",
        "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--color-ring)]",
        active
          ? "bg-[var(--color-stop-fill)]/10 text-[var(--color-ink)]"
          : "text-[var(--color-ink-dim)] hover:text-[var(--color-ink)] hover:bg-[var(--color-muted)]/30",
      ].join(" ")}
      aria-pressed={active}
    >
      <span className="flex items-center gap-2">
        <span
          className={[
            "w-4 text-center text-sm",
            tone === "primary" || active
              ? "text-[var(--color-stop-fill)]"
              : "text-[var(--color-ink-dim)]",
          ].join(" ")}
          aria-hidden
        >
          {glyph}
        </span>
        <span className="font-mono text-xs uppercase tracking-wider">
          {label}
        </span>
      </span>
      <span className="font-mono text-[10px] text-[var(--color-ink-dim)]">
        {count}
      </span>
    </button>
  );
}

function JobCard({ job }: { job: JobBlueprint }) {
  const href = `/compose/build?job=${encodeURIComponent(job.id)}`;
  return (
    <Link
      href={href}
      className="group relative border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.2)] px-4 py-4 flex flex-col gap-2 hover:border-[var(--color-stop-fill)]/50 transition-colors min-h-[210px]"
    >
      <div className="flex items-start justify-between">
        <span className="readout-label">
          {job.function.toUpperCase()}
        </span>
        {job.killer && (
          <span
            className="text-[var(--color-stop-fill)] text-xs"
            title="Most hired"
            aria-label="Most hired"
          >
            ★
          </span>
        )}
      </div>
      <span className="font-mono text-sm uppercase tracking-wider text-[var(--color-ink)] mt-1 leading-snug">
        {job.name}
      </span>
      <span
        className="italic text-sm text-[var(--color-ink-dim)] leading-snug flex-1"
        style={{ fontFamily: "var(--font-display)" }}
      >
        {job.tagline}
      </span>
      <div className="mt-2 pt-2 border-t border-[var(--color-rule)] flex items-baseline justify-between">
        <span className="readout-numeric text-base text-[var(--color-ink)]">
          {formatCents(job.priceMonthly)}
          <span className="text-[var(--color-ink-dim)] text-xs"> /mo</span>
        </span>
        <span className="text-[var(--color-stop-fill)] text-xs group-hover:underline">
          Hire →
        </span>
      </div>
      <span className="readout-label text-[9px] text-[var(--color-ink-dim)]/80">
        {job.billingUnit}
      </span>
    </Link>
  );
}
