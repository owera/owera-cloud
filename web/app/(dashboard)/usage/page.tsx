"use client";

import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { getApiToken } from "@/lib/auth";

// Current-period usage meter (T16.4 / shared with WS-17 T17.4).
//
// WS-16 owns the data shape and fetch; WS-17 owns the visual chrome and
// can replace the body of <UsageTable/> below without touching this
// page's data layer. The contract: each row is { sku, units } as returned
// from GET /v1/usage.

interface UsageMeters {
  period: string;
  meters: Record<string, number>;
}

async function fetchUsage(): Promise<UsageMeters> {
  const apiBase =
    process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
  const token = await getApiToken();
  const res = await fetch(`${apiBase}/v1/usage`, {
    headers: {
      accept: "application/json",
      ...(token ? { authorization: `Bearer ${token}` } : {}),
    },
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}`);
  }
  return (await res.json()) as UsageMeters;
}

export default function UsagePage(): React.ReactElement {
  const [data, setData] = React.useState<UsageMeters | null>(null);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    fetchUsage()
      .then(setData)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "unknown error");
      });
  }, []);

  const rows = data ? Object.entries(data.meters) : [];

  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <h1 className="font-mono text-xl font-semibold tracking-tight">
          USAGE
        </h1>
        <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-muted-foreground)]">
          period: {data?.period ?? "—"}
        </span>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>METERED USAGE</CardTitle>
        </CardHeader>
        <CardBody>
          {error ? (
            <p className="text-sm text-[var(--color-state-failed)]">
              {error}
            </p>
          ) : rows.length === 0 ? (
            <p className="text-sm text-[var(--color-muted-foreground)]">
              No usage recorded this period.
            </p>
          ) : (
            <UsageTable rows={rows} />
          )}
        </CardBody>
      </Card>
    </div>
  );
}

function UsageTable({
  rows,
}: {
  rows: Array<[string, number]>;
}): React.ReactElement {
  return (
    <table className="w-full text-sm font-mono">
      <thead>
        <tr className="text-left text-[10px] uppercase tracking-wide text-[var(--color-muted-foreground)]">
          <th className="pb-2">SKU</th>
          <th className="pb-2 text-right">Units</th>
        </tr>
      </thead>
      <tbody>
        {rows.map(([sku, units]) => (
          <tr
            key={sku}
            className="border-t border-[var(--color-border)]"
          >
            <td className="py-2">{sku}</td>
            <td className="py-2 text-right">
              {units.toLocaleString()}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
