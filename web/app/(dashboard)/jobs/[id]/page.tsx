import * as React from "react";
import Link from "next/link";
import { notFound } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { JobStatusBadge } from "@/components/job-status-badge";
import { JobTimeline } from "@/components/job-timeline";
import { api, ApiClientError } from "@/lib/api-client";
import { duration, formatCents, shortTimestamp } from "@/lib/format";
import type { Job, JobLedgerEntry } from "@/lib/types";

export const dynamic = "force-dynamic";

// Tiny fixture mirroring what the api/ agent will eventually serve.
const FIXTURE_JOB: Job = {
  id: "job_demo",
  tenantId: "tnt_mock_0001",
  skuSlug: "agentic.research",
  state: "running",
  inputSummary: "Demo job — replace with live data once the API is wired.",
  submittedAt: "2026-05-16T12:00:00Z",
  startedAt: "2026-05-16T12:00:12Z",
  finishedAt: null,
  costCents: 540,
  assignedNodes: ["claw1.local"],
  error: null,
};

const FIXTURE_LEDGER: JobLedgerEntry[] = [
  {
    id: "lg_1",
    jobId: "job_demo",
    ts: "2026-05-16T12:00:00Z",
    kind: "state_change",
    message: "submitted",
    data: null,
  },
  {
    id: "lg_2",
    jobId: "job_demo",
    ts: "2026-05-16T12:00:08Z",
    kind: "state_change",
    message: "queued → running on claw1.local",
    data: null,
  },
  {
    id: "lg_3",
    jobId: "job_demo",
    ts: "2026-05-16T12:01:21Z",
    kind: "tool_call",
    message: "web.search(\"Inside Out 3 reviews\")",
    data: { results: 12 },
  },
  {
    id: "lg_4",
    jobId: "job_demo",
    ts: "2026-05-16T12:03:42Z",
    kind: "billing",
    message: "+ $5.40 (research/minutes)",
    data: { cents: 540 },
  },
];

async function safeFetch(id: string): Promise<{
  job: Job;
  ledger: JobLedgerEntry[];
  live: boolean;
}> {
  try {
    const [job, ledger] = await Promise.all([
      api.getJob(id),
      api.getJobLedger(id),
    ]);
    return { job, ledger, live: true };
  } catch (err) {
    if (err instanceof ApiClientError && err.status === 404) notFound();
    return { job: { ...FIXTURE_JOB, id }, ledger: FIXTURE_LEDGER, live: false };
  }
}

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function JobDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { job, ledger, live } = await safeFetch(id);

  const rerunHref = `/compose?sku=${encodeURIComponent(job.skuSlug)}&prompt=${encodeURIComponent(job.inputSummary)}`;
  // Tool calls in the ledger are the "what the agent did" — pull them out
  // for a dedicated Plan panel so a user can scan the work at a glance.
  const planSteps = ledger.filter(
    (e) => e.kind === "tool_call" || e.kind === "state_change",
  );

  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <div>
          <Link
            href="/jobs"
            className="text-xs text-[var(--color-muted-foreground)] hover:text-[var(--color-foreground)]"
          >
            ← back to jobs
          </Link>
          <h1 className="mt-1 font-mono text-xl font-semibold tracking-tight">
            {job.id}
          </h1>
        </div>
        <div className="flex items-center gap-2">
          <JobStatusBadge state={job.state} />
          {!live && (
            <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
              FIXTURE DATA
            </span>
          )}
        </div>
      </header>

      {/* Re-hire actions. The single most important muscle: turn one run into
         many. */}
      <section className="border border-[var(--color-rule)] rounded-sm bg-[rgba(0,0,0,0.2)] px-5 py-4 flex flex-wrap items-center gap-3">
        <div className="flex flex-col gap-1 flex-1 min-w-[200px]">
          <span className="readout-label">Re-hire this job</span>
          <span className="text-xs text-[var(--color-ink-dim)]">
            Run again with the same inputs, change something before re-running,
            or schedule it to repeat.
          </span>
        </div>
        <Button asChild variant="primary" size="sm">
          <Link href={rerunHref}>Run again →</Link>
        </Button>
        <Button asChild variant="secondary" size="sm">
          <Link href={`${rerunHref}&edit=1`}>Edit & re-hire</Link>
        </Button>
        <Button asChild variant="ghost" size="sm">
          <Link href={`${rerunHref}&schedule=1`}>Schedule…</Link>
        </Button>
      </section>

      <section className="grid grid-cols-4 gap-3">
        <Meta label="SKU" value={job.skuSlug} />
        <Meta label="SUBMITTED" value={shortTimestamp(job.submittedAt)} />
        <Meta
          label="DURATION"
          value={job.startedAt ? duration(job.startedAt, job.finishedAt) : "—"}
        />
        <Meta label="COST" value={formatCents(job.costCents)} />
      </section>

      <Card>
        <CardHeader>
          <CardTitle>INPUT SUMMARY</CardTitle>
        </CardHeader>
        <CardBody>
          <pre className="text-sm font-mono whitespace-pre-wrap leading-relaxed">
            {job.inputSummary}
          </pre>
        </CardBody>
      </Card>

      {job.error && (
        <Card className="border-[var(--color-state-failed)]/40">
          <CardHeader>
            <CardTitle className="text-[var(--color-state-failed)]">
              ERROR
            </CardTitle>
          </CardHeader>
          <CardBody>
            <pre className="text-sm font-mono whitespace-pre-wrap text-[var(--color-state-failed)]">
              {job.error}
            </pre>
          </CardBody>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>PLAN · WHAT THE AGENT IS DOING</CardTitle>
        </CardHeader>
        <CardBody>
          {planSteps.length === 0 ? (
            <p className="text-sm text-[var(--color-muted-foreground)]">
              Plan steps will appear as the agent works. Each tool call and
              state transition is recorded.
            </p>
          ) : (
            <ol className="flex flex-col gap-2">
              {planSteps.map((step, i) => (
                <li
                  key={step.id}
                  className="flex items-start gap-3 text-sm font-mono"
                >
                  <span className="text-[var(--color-muted-foreground)] w-8 shrink-0">
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <span
                    className={[
                      "px-1.5 rounded text-[10px] uppercase tracking-wider",
                      step.kind === "tool_call"
                        ? "bg-[var(--color-primary)]/15 text-[var(--color-primary)]"
                        : "bg-[var(--color-muted)] text-[var(--color-muted-foreground)]",
                    ].join(" ")}
                  >
                    {step.kind === "tool_call" ? "TOOL" : "STATE"}
                  </span>
                  <span className="flex-1 text-[var(--color-foreground)]">
                    {step.message}
                  </span>
                  <span className="text-[10px] text-[var(--color-muted-foreground)]">
                    {shortTimestamp(step.ts).split(" ").slice(-2).join(" ")}
                  </span>
                </li>
              ))}
            </ol>
          )}
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>LEDGER</CardTitle>
        </CardHeader>
        <CardBody>
          <JobTimeline jobId={job.id} initial={ledger} live={live} />
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>OUTPUTS</CardTitle>
        </CardHeader>
        <CardBody className="text-sm text-[var(--color-muted-foreground)]">
          Outputs become available once the job reaches{" "}
          <span className="font-mono">succeeded</span>. The OpenAPI contract
          will expose a signed-URL download list per job.
        </CardBody>
      </Card>
    </div>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardBody>
        <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
          {label}
        </div>
        <div className="mt-1 font-mono text-sm">{value}</div>
      </CardBody>
    </Card>
  );
}
