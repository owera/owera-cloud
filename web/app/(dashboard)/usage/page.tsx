import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, THead, TBody, TR, TH, TD } from "@/components/ui/table";
import { api, ApiClientError } from "@/lib/api-client";
import { formatCents, formatCount, shortTimestamp } from "@/lib/format";
import type { UsageMeter } from "@/lib/types";

export const metadata = { title: "Usage" };
export const dynamic = "force-dynamic";

const FIXTURE: UsageMeter = {
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

async function safeFetch(): Promise<{ usage: UsageMeter; live: boolean }> {
  try {
    return { usage: await api.getCurrentUsage(), live: true };
  } catch (err) {
    if (err instanceof ApiClientError || err instanceof Error) {
      return { usage: FIXTURE, live: false };
    }
    throw err;
  }
}

export default async function UsagePage() {
  const { usage, live } = await safeFetch();
  const rows = Object.entries(usage.bySku).sort((a, b) =>
    b[1].costCents - a[1].costCents,
  );
  const maxCost = Math.max(1, ...rows.map(([, r]) => r.costCents));

  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <div>
          <h1 className="font-mono text-xl font-semibold tracking-tight">
            USAGE
          </h1>
          <p className="text-xs text-[var(--color-muted-foreground)] mt-1">
            Billing period {shortTimestamp(usage.periodStart)} →{" "}
            {shortTimestamp(usage.periodEnd)}
          </p>
        </div>
        {!live && (
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

      <Card>
        <CardHeader>
          <CardTitle>BY SKU</CardTitle>
        </CardHeader>
        <Table>
          <THead>
            <TR>
              <TH>SKU</TH>
              <TH className="text-right">JOBS</TH>
              <TH className="text-right">COST</TH>
              <TH className="w-1/3">SHARE</TH>
            </TR>
          </THead>
          <TBody>
            {rows.map(([slug, r]) => (
              <TR key={slug}>
                <TD>{slug}</TD>
                <TD className="text-right">{formatCount(r.jobs)}</TD>
                <TD className="text-right">{formatCents(r.costCents)}</TD>
                <TD>
                  <Bar value={r.costCents} max={maxCost} />
                </TD>
              </TR>
            ))}
            {rows.length === 0 && (
              <TR>
                <TD
                  colSpan={4}
                  className="text-center text-[var(--color-muted-foreground)] py-6"
                >
                  No usage yet this period.
                </TD>
              </TR>
            )}
          </TBody>
        </Table>
      </Card>

      <Card>
        <CardBody className="text-xs text-[var(--color-muted-foreground)] leading-relaxed">
          Usage meters reflect the signed ledger emitted by the operator plane.
          If a meter disagrees with a Stripe invoice, the ledger wins —{" "}
          <span className="font-mono">/billing</span> opens the Stripe portal
          where the source-of-truth invoice lives.
        </CardBody>
      </Card>
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

function Bar({ value, max }: { value: number; max: number }) {
  const pct = Math.round((value / max) * 100);
  return (
    <div className="h-2 w-full rounded bg-[var(--color-muted)] overflow-hidden">
      <div
        className="h-full bg-[var(--color-primary)]"
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}
