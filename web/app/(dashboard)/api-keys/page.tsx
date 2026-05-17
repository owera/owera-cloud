import * as React from "react";
import { Card, CardBody } from "@/components/ui/card";
import { ApiKeysManager } from "@/components/api-keys-manager";
import { api, ApiClientError } from "@/lib/api-client";
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
      </header>

      <Card>
        <ApiKeysManager initial={keys} live={live} />
      </Card>

      <Card>
        <CardBody className="text-xs text-[var(--color-muted-foreground)]">
          Keys are scoped credentials. Secrets are shown exactly once — at
          creation time — and never stored client-side. Revocation invalidates
          the key immediately at the API edge.
        </CardBody>
      </Card>
    </div>
  );
}
