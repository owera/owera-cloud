"use client";

import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { getApiToken } from "@/lib/auth";

// Stripe Customer Portal embed (T16.2).
//
// Mints a short-lived portal session via POST /v1/billing/portal and
// redirects the browser to Stripe's hosted UI. From there the customer
// can update payment method, view invoices, and cancel subscriptions —
// Stripe owns the surface; we own only the bounce. Return URL is the
// /billing page itself so a closed-portal returns the customer to the
// same place.

interface PortalResponse {
  url: string;
}

interface PortalErrorBody {
  error?: string;
  detail?: string;
}

async function openPortal(): Promise<void> {
  const apiBase =
    process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
  const token = await getApiToken();
  const res = await fetch(`${apiBase}/v1/billing/portal`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      ...(token ? { authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({
      return_url: window.location.href,
    }),
  });
  if (!res.ok) {
    let body: PortalErrorBody = {};
    try {
      body = (await res.json()) as PortalErrorBody;
    } catch {
      // ignore parse failure; surface status alone
    }
    const detail = body.detail ?? body.error ?? `HTTP ${res.status}`;
    throw new Error(detail);
  }
  const json = (await res.json()) as PortalResponse;
  window.location.assign(json.url);
}

export default function BillingPage(): React.ReactElement {
  const [loading, setLoading] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  async function handleClick(): Promise<void> {
    setLoading(true);
    setError(null);
    try {
      await openPortal();
    } catch (err) {
      setError(err instanceof Error ? err.message : "unknown error");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <h1 className="font-mono text-xl font-semibold tracking-tight">
          BILLING
        </h1>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>CUSTOMER PORTAL</CardTitle>
        </CardHeader>
        <CardBody className="space-y-4">
          <p className="text-sm text-[var(--color-muted-foreground)] leading-relaxed">
            Manage your payment method, download invoices, change billing
            contacts, and cancel subscriptions via the Stripe Customer
            Portal. We mint a short-lived session and bounce you over.
          </p>
          <div className="flex items-center gap-3">
            <Button
              variant="primary"
              disabled={loading}
              onClick={() => {
                void handleClick();
              }}
            >
              {loading ? "Opening…" : "Open Stripe portal"}
            </Button>
            {error ? (
              <span className="text-xs text-[var(--color-state-failed)]">
                {error}
              </span>
            ) : null}
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>PLAN</CardTitle>
        </CardHeader>
        <CardBody>
          <dl className="grid grid-cols-2 gap-y-3 font-mono text-sm">
            <dt className="text-[var(--color-muted-foreground)]">Plan</dt>
            <dd>Pay-as-you-go</dd>
            <dt className="text-[var(--color-muted-foreground)]">Currency</dt>
            <dd>USD</dd>
            <dt className="text-[var(--color-muted-foreground)]">Billed via</dt>
            <dd>Stripe</dd>
          </dl>
        </CardBody>
      </Card>
    </div>
  );
}
