import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { JobsTable } from "@/components/jobs-table";
import { api, ApiClientError } from "@/lib/api-client";
import { formatCents, formatCount, shortTimestamp } from "@/lib/format";
import type { Job, UsageMeter } from "@/lib/types";

export const metadata = { title: "Overview" };
export const dynamic = "force-dynamic";

// Until the api/ agent's service is reachable in dev, we fall back to a small
// fixture so the dashboard renders. Remove this when the API is live.
const FIXTURE_USAGE: UsageMeter = {
  periodStart: "2026-05-01T00:00:00Z",
  periodEnd: "2026-05-31T23:59:59Z",
  totalCostCents: 482300,
  jobsCount: 1284,
  bySku: {
    "agentic.research": { jobs: 412, costCents: 198400 },
    "agentic.etl": { jobs: 522, costCents: 142700 },
    "agentic.build": { jobs: 350, costCents: 141200 },
  },
};

const FIXTURE_JOBS: Job[] = [
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
];

async function safeListJobs(): Promise<{ jobs: Job[]; live: boolean }> {
  try {
    const jobs = await api.listJobs({ limit: 5 });
    return { jobs, live: true };
  } catch (err) {
    if (err instanceof ApiClientError || err instanceof Error) {
      return { jobs: FIXTURE_JOBS, live: false };
    }
    throw err;
  }
}

async function safeUsage(): Promise<{ usage: UsageMeter; live: boolean }> {
  try {
    const usage = await api.getCurrentUsage();
    return { usage, live: true };
  } catch (err) {
    if (err instanceof ApiClientError || err instanceof Error) {
      return { usage: FIXTURE_USAGE, live: false };
    }
    throw err;
  }
}

export default async function OverviewPage() {
  const [{ jobs, live: jobsLive }, { usage, live: usageLive }] =
    await Promise.all([safeListJobs(), safeUsage()]);

  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <div>
          <h1 className="font-mono text-xl font-semibold tracking-tight">
            OVERVIEW
          </h1>
          <p className="text-xs text-[var(--color-muted-foreground)] mt-1">
            Billing period {shortTimestamp(usage.periodStart)} →{" "}
            {shortTimestamp(usage.periodEnd)}
          </p>
        </div>
        {(!jobsLive || !usageLive) && (
          <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
            FIXTURE DATA — API not reachable
          </span>
        )}
      </header>

      <section className="grid grid-cols-3 gap-3">
        <Stat label="JOBS THIS PERIOD" value={formatCount(usage.jobsCount)} />
        <Stat label="COST TO DATE" value={formatCents(usage.totalCostCents)} />
        <Stat
          label="ACTIVE SKUS"
          value={String(Object.keys(usage.bySku).length)}
        />
      </section>

      <section>
        <Card>
          <CardHeader>
            <CardTitle>RECENT JOBS</CardTitle>
          </CardHeader>
          <div className="border-t border-[var(--color-border)]">
            <JobsTable jobs={jobs} compact />
          </div>
        </Card>
      </section>

      <section>
        <Card>
          <CardHeader>
            <CardTitle>USAGE BY SKU</CardTitle>
          </CardHeader>
          <CardBody>
            <ul className="divide-y divide-[var(--color-border)]">
              {Object.entries(usage.bySku).map(([slug, row]) => (
                <li
                  key={slug}
                  className="flex items-center justify-between py-2 font-mono text-sm"
                >
                  <span>{slug}</span>
                  <span className="text-[var(--color-muted-foreground)]">
                    {formatCount(row.jobs)} jobs
                  </span>
                  <span>{formatCents(row.costCents)}</span>
                </li>
              ))}
            </ul>
          </CardBody>
        </Card>
      </section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardBody>
        <div className="text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
          {label}
        </div>
        <div className="mt-1 font-mono text-2xl">{value}</div>
      </CardBody>
    </Card>
  );
}
