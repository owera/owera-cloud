import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Table, THead, TBody, TR, TH, TD } from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { api, ApiClientError } from "@/lib/api-client";
import { relativeTime, shortTimestamp } from "@/lib/format";
import type { ApiKey } from "@/lib/types";

export const metadata = { title: "API keys" };
export const dynamic = "force-dynamic";

const FIXTURE: ApiKey[] = [
  {
    id: "key_01HXR8M2",
    name: "dev laptop",
    lastFour: "9f2c",
    createdAt: "2026-05-01T10:23:00Z",
    lastUsedAt: "2026-05-16T11:08:42Z",
    scopes: ["jobs.read", "jobs.write"],
    revokedAt: null,
  },
  {
    id: "key_01HXQ8M2",
    name: "ci-runner",
    lastFour: "01af",
    createdAt: "2026-04-12T15:00:00Z",
    lastUsedAt: "2026-05-16T03:00:14Z",
    scopes: ["jobs.read", "jobs.write", "billing.read"],
    revokedAt: null,
  },
];

async function safeList(): Promise<{ keys: ApiKey[]; live: boolean }> {
  try {
    return { keys: await api.listApiKeys(), live: true };
  } catch (err) {
    if (err instanceof ApiClientError || err instanceof Error) {
      return { keys: FIXTURE, live: false };
    }
    throw err;
  }
}

export default async function ApiKeysPage() {
  const { keys, live } = await safeList();

  return (
    <div className="space-y-4">
      <header className="flex items-baseline justify-between">
        <h1 className="font-mono text-xl font-semibold tracking-tight">
          API KEYS
        </h1>
        <div className="flex items-center gap-3">
          {!live && (
            <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
              FIXTURE DATA
            </span>
          )}
          {/* The actual create flow lives in a follow-on PR — needs a modal +
              one-time-secret reveal UI. */}
          <Button variant="primary" disabled>
            New key
          </Button>
        </div>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>ACTIVE KEYS</CardTitle>
        </CardHeader>
        <Table>
          <THead>
            <TR>
              <TH>NAME</TH>
              <TH>ID</TH>
              <TH>SECRET</TH>
              <TH>SCOPES</TH>
              <TH>CREATED</TH>
              <TH>LAST USED</TH>
              <TH>ACTION</TH>
            </TR>
          </THead>
          <TBody>
            {keys.map((k) => (
              <TR key={k.id}>
                <TD>{k.name}</TD>
                <TD className="text-[var(--color-muted-foreground)]">{k.id}</TD>
                <TD>sk_…{k.lastFour}</TD>
                <TD className="flex gap-1">
                  {k.scopes.map((s) => (
                    <Badge key={s}>{s}</Badge>
                  ))}
                </TD>
                <TD title={k.createdAt}>{shortTimestamp(k.createdAt)}</TD>
                <TD title={k.lastUsedAt ?? "never"}>
                  {k.lastUsedAt ? relativeTime(k.lastUsedAt) : "never"}
                </TD>
                <TD>
                  <Button size="sm" variant="danger" disabled>
                    Revoke
                  </Button>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Card>

      <Card>
        <CardBody className="text-xs text-[var(--color-muted-foreground)]">
          Keys are scoped credentials. Secrets are shown exactly once — at
          creation time — and never stored client-side. The api/ agent will
          expose <code>POST /v1/api-keys</code> and{" "}
          <code>DELETE /v1/api-keys/:id</code>.
        </CardBody>
      </Card>
    </div>
  );
}
