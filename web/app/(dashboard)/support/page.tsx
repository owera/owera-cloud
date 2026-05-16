import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

export const metadata = { title: "Support" };

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

export default function SupportPage() {
  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <h1 className="font-mono text-xl font-semibold tracking-tight">
          SUPPORT
        </h1>
        <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
          TICKET INBOX — stubbed
        </span>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>DOCS</CardTitle>
        </CardHeader>
        <CardBody>
          <ul className="grid grid-cols-2 gap-3">
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

      <Card>
        <CardHeader>
          <CardTitle>TICKETS</CardTitle>
        </CardHeader>
        <CardBody className="space-y-4">
          <p className="text-sm text-[var(--color-muted-foreground)] leading-relaxed">
            Ticket inbox is not wired yet. The api/ agent will expose{" "}
            <code className="text-[var(--color-foreground)]">
              GET /v1/support/tickets
            </code>{" "}
            and{" "}
            <code className="text-[var(--color-foreground)]">
              POST /v1/support/tickets
            </code>{" "}
            against the chosen helpdesk provider.
          </p>
          <Button variant="primary" disabled>
            Open a ticket
          </Button>
        </CardBody>
      </Card>
    </div>
  );
}
