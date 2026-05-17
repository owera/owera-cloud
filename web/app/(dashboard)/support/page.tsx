import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { SupportInbox } from "@/components/support-inbox";
import { api, ApiClientError } from "@/lib/api-client";
import type { Ticket } from "@/lib/api-client";

export const metadata = { title: "Support" };
export const dynamic = "force-dynamic";

interface DocLink {
  title: string;
  href: string;
  description: string;
}

const DOCS: ReadonlyArray<DocLink> = [
  {
    title: "Quickstart",
    href: "https://docs.owera.ai/quickstart",
    description: "Submit your first job in 60 seconds.",
  },
  {
    title: "SKU catalogue",
    href: "https://docs.owera.ai/skus",
    description: "Research, ETL, ops, build — what's available and pricing.",
  },
  {
    title: "API reference",
    href: "https://docs.owera.ai/api",
    description: "Generated from the api/openapi.yaml contract.",
  },
  {
    title: "Status page",
    href: "https://status.owera.ai",
    description: "Live SLA + incident history.",
  },
];

const FIXTURE: Ticket[] = [
  {
    id: "tkt_demo_1",
    subject: "job_01HXR0F1 — xcodebuild signing failed",
    state: "open",
    createdAt: "2026-05-16T09:55:00Z",
    updatedAt: "2026-05-16T10:11:00Z",
    lastAuthor: "customer",
  },
  {
    id: "tkt_demo_2",
    subject: "How do I rotate API keys without downtime?",
    state: "resolved",
    createdAt: "2026-05-12T14:02:00Z",
    updatedAt: "2026-05-12T16:40:00Z",
    lastAuthor: "support",
  },
];

async function safeList(): Promise<{ tickets: Ticket[]; live: boolean }> {
  try {
    return { tickets: await api.listTickets(), live: true };
  } catch (err) {
    if (err instanceof ApiClientError || err instanceof Error) {
      return { tickets: FIXTURE, live: false };
    }
    throw err;
  }
}

export default async function SupportPage() {
  const { tickets, live } = await safeList();

  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <h1 className="font-mono text-xl font-semibold tracking-tight">
          SUPPORT
        </h1>
        {!live && (
          <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
            FIXTURE DATA — API not reachable
          </span>
        )}
      </header>

      <SupportInbox initial={tickets} live={live} />

      <Card>
        <CardHeader>
          <CardTitle>DOCS</CardTitle>
        </CardHeader>
        <CardBody>
          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {DOCS.map((d) => (
              <li
                key={d.href}
                className="rounded border border-[var(--color-border)] bg-[var(--color-muted)]/40 p-3"
              >
                <a
                  href={d.href}
                  className="font-mono text-sm text-[var(--color-foreground)] hover:text-[var(--color-primary)] transition-colors"
                  target="_blank"
                  rel="noreferrer"
                >
                  {d.title} ↗
                </a>
                <p className="mt-1 text-xs text-[var(--color-muted-foreground)]">
                  {d.description}
                </p>
              </li>
            ))}
          </ul>
        </CardBody>
      </Card>
    </div>
  );
}
