import * as React from "react";
import Link from "next/link";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { JobsTable } from "@/components/jobs-table";
import { TemplateCard } from "@/components/compose/template-card";
import { STARTER_TEMPLATES } from "@/lib/compose/templates";
import { api } from "@/lib/api-client";
import { JOB_STATES, type Job, type JobState } from "@/lib/types";

export const metadata = { title: "Jobs" };
export const dynamic = "force-dynamic";

const FIXTURE: Job[] = [
  {
    id: "job_01HXR8M2",
    tenantId: "tnt_mock_0001",
    skuSlug: "agentic.research",
    state: "running",
    inputSummary: "Competitive teardown: Pixar's Inside Out 3 marketing",
    submittedAt: "2026-05-16T12:14:00Z",
    startedAt: "2026-05-16T12:14:08Z",
    finishedAt: null,
    costCents: 1240,
    assignedNodes: ["claw1.local"],
    error: null,
  },
  {
    id: "job_01HXR7TQ",
    tenantId: "tnt_mock_0001",
    skuSlug: "agentic.etl",
    state: "succeeded",
    inputSummary: "Normalise CSV exports from /drop and upsert into warehouse",
    submittedAt: "2026-05-16T11:02:00Z",
    startedAt: "2026-05-16T11:02:05Z",
    finishedAt: "2026-05-16T11:08:42Z",
    costCents: 320,
    assignedNodes: ["claw1.local"],
    error: null,
  },
  {
    id: "job_01HXR0F1",
    tenantId: "tnt_mock_0001",
    skuSlug: "agentic.build",
    state: "failed",
    inputSummary: "Swift Package CI build for tag v0.3.1",
    submittedAt: "2026-05-16T09:48:00Z",
    startedAt: "2026-05-16T09:48:10Z",
    finishedAt: "2026-05-16T09:51:33Z",
    costCents: 180,
    assignedNodes: ["claw1.local"],
    error: "xcodebuild: error: No signing certificate found",
  },
  {
    id: "job_01HXQF21",
    tenantId: "tnt_mock_0001",
    skuSlug: "agentic.ops",
    state: "queued",
    inputSummary: "Rotate vault keys across staging fleet",
    submittedAt: "2026-05-16T08:11:00Z",
    startedAt: null,
    finishedAt: null,
    costCents: 0,
    assignedNodes: [],
    error: null,
  },
  {
    id: "job_01HXQ8N0",
    tenantId: "tnt_mock_0001",
    skuSlug: "agentic.research",
    state: "cancelled",
    inputSummary: "Backfill literature review (legacy)",
    submittedAt: "2026-05-15T22:30:00Z",
    startedAt: "2026-05-15T22:30:11Z",
    finishedAt: "2026-05-15T22:31:02Z",
    costCents: 40,
    assignedNodes: ["claw1.local"],
    error: null,
  },
];

interface PageProps {
  searchParams?: Promise<{ state?: string }>;
}

async function safeList(state: string | undefined) {
  try {
    return {
      jobs: await api.listJobs({ limit: 100, state }),
      live: true,
    };
  } catch {
    const filtered = state
      ? FIXTURE.filter((j) => j.state === state)
      : FIXTURE;
    return { jobs: filtered, live: false };
  }
}

function isJobState(v: string | undefined): v is JobState {
  return !!v && (JOB_STATES as readonly string[]).includes(v);
}

export default async function JobsPage({ searchParams }: PageProps) {
  const params = (await searchParams) ?? {};
  const state = isJobState(params.state) ? params.state : undefined;
  const { jobs, live } = await safeList(state);

  return (
    <div className="space-y-8">
      {/* Hero / hire CTA. */}
      <section className="flex items-center justify-between gap-4 border-b border-[var(--color-rule)] pb-5">
        <div className="flex flex-col gap-1">
          <span className="readout-label">YOUR JOBS</span>
          <h1 className="font-mono text-xl tracking-tight">
            Run, re-run, or schedule.
          </h1>
          <p className="text-sm text-[var(--color-ink-dim)]">
            Hire a new job from a template, or re-run anything in the list.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {!live && (
            <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
              FIXTURE DATA
            </span>
          )}
          <Button asChild variant="primary" className="h-10 px-5">
            <Link href="/compose">Compose new →</Link>
          </Button>
        </div>
      </section>

      {/* Templates / re-hire surface. */}
      <section className="flex flex-col gap-3">
        <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2">
          <span className="readout-label">Templates · start here</span>
          <span className="readout-label">RE-HIRE WITH ONE CLICK</span>
        </header>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {STARTER_TEMPLATES.map((t) => (
            <TemplateCard key={t.id} seed={t} hint="Open in composer →" />
          ))}
        </div>
      </section>

      {/* Filters + table. */}
      <section className="flex flex-col gap-3">
        <header className="flex items-baseline justify-between border-b border-[var(--color-rule)] pb-2">
          <span className="readout-label">Recent runs</span>
          <span className="readout-label">{jobs.length} JOB(S)</span>
        </header>
        <div className="flex items-center gap-2 text-xs font-mono">
          <FilterLink current={state} target={undefined}>
            ALL
          </FilterLink>
          {JOB_STATES.map((s) => (
            <FilterLink key={s} current={state} target={s}>
              {s.toUpperCase()}
            </FilterLink>
          ))}
        </div>
        <Card>
          <JobsTable jobs={jobs} />
        </Card>
      </section>
    </div>
  );
}

function FilterLink({
  current,
  target,
  children,
}: {
  current: JobState | undefined;
  target: JobState | undefined;
  children: React.ReactNode;
}) {
  const active = current === target;
  const href = target ? `/jobs?state=${target}` : "/jobs";
  return (
    <Link
      href={href}
      className={[
        "rounded border px-2 py-1 transition-colors",
        active
          ? "border-[var(--color-primary)] text-[var(--color-primary)]"
          : "border-[var(--color-border)] text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]",
      ].join(" ")}
    >
      {children}
    </Link>
  );
}
