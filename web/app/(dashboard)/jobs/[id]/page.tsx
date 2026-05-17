import * as React from "react";
import Link from "next/link";
import { notFound } from "next/navigation";
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
