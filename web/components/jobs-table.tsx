"use client";

import * as React from "react";
import Link from "next/link";
import { Table, THead, TBody, TR, TH, TD } from "./ui/table";
import { JobStatusBadge } from "./job-status-badge";
import type { Job } from "@/lib/types";
import { duration, formatCents, relativeTime } from "@/lib/format";

type SortKey = "submittedAt" | "costCents" | "state" | "skuSlug";
type SortDir = "asc" | "desc";

export interface JobsTableProps {
  jobs: Job[];
  /** When true, renders a more compact "recent jobs" view. */
  compact?: boolean;
}

export function JobsTable({ jobs, compact = false }: JobsTableProps) {
  const [sortKey, setSortKey] = React.useState<SortKey>("submittedAt");
  const [sortDir, setSortDir] = React.useState<SortDir>("desc");

  const sorted = React.useMemo(() => {
    const arr = [...jobs];
    arr.sort((a, b) => {
      const av = a[sortKey];
      const bv = b[sortKey];
      let cmp = 0;
      if (typeof av === "number" && typeof bv === "number") cmp = av - bv;
      else cmp = String(av).localeCompare(String(bv));
      return sortDir === "asc" ? cmp : -cmp;
    });
    return arr;
  }, [jobs, sortKey, sortDir]);

  function toggle(k: SortKey) {
    if (k === sortKey) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(k);
      setSortDir("desc");
    }
  }

  function header(k: SortKey, label: string) {
    const arrow = sortKey === k ? (sortDir === "asc" ? "↑" : "↓") : "";
    return (
      <TH>
        <button
          type="button"
          onClick={() => toggle(k)}
          className="inline-flex items-center gap-1 hover:text-[var(--color-foreground)] transition-colors"
        >
          {label} <span className="text-[10px]">{arrow}</span>
        </button>
      </TH>
    );
  }

  return (
    <Table>
      <THead>
        <TR>
          <TH>JOB ID</TH>
          {header("skuSlug", "SKU")}
          {header("state", "STATE")}
          {header("submittedAt", "SUBMITTED")}
          {!compact && <TH>DURATION</TH>}
          {header("costCents", "COST")}
          {!compact && <TH>NODES</TH>}
        </TR>
      </THead>
      <TBody>
        {sorted.length === 0 && (
          <TR>
            <TD
              colSpan={compact ? 5 : 7}
              className="text-center text-[var(--color-muted-foreground)] py-6"
            >
              No jobs yet.
            </TD>
          </TR>
        )}
        {sorted.map((job) => (
          <TR key={job.id}>
            <TD>
              <Link
                href={`/jobs/${job.id}`}
                className="text-[var(--color-primary)] hover:underline"
              >
                {job.id}
              </Link>
            </TD>
            <TD>{job.skuSlug}</TD>
            <TD>
              <JobStatusBadge state={job.state} />
            </TD>
            <TD title={job.submittedAt}>{relativeTime(job.submittedAt)}</TD>
            {!compact && (
              <TD>
                {job.startedAt
                  ? duration(job.startedAt, job.finishedAt)
                  : "—"}
              </TD>
            )}
            <TD>{formatCents(job.costCents)}</TD>
            {!compact && (
              <TD className="text-[var(--color-muted-foreground)]">
                {job.assignedNodes.join(", ") || "—"}
              </TD>
            )}
          </TR>
        ))}
      </TBody>
    </Table>
  );
}
