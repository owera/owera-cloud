import * as React from "react";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

export const metadata = { title: "Billing" };

// Stripe Customer Portal embed (stub).
//
// In production, the api/ agent will issue a short-lived portal session URL via
// POST /v1/billing/portal. We stub that as a button that links nowhere yet.
export default function BillingPage() {
  return (
    <div className="space-y-6">
      <header className="flex items-baseline justify-between">
        <h1 className="font-mono text-xl font-semibold tracking-tight">
          BILLING
        </h1>
        <span className="text-[10px] uppercase tracking-wide font-mono text-[var(--color-state-running)]">
          STUB — Stripe wiring pending
        </span>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>CUSTOMER PORTAL</CardTitle>
        </CardHeader>
        <CardBody className="space-y-4">
          <p className="text-sm text-[var(--color-muted-foreground)] leading-relaxed">
            Manage your payment method, download invoices, and update billing
            contacts via the Stripe Customer Portal. The button below will mint
            a short-lived portal session once the API exposes{" "}
            <code className="text-[var(--color-foreground)]">
              POST /v1/billing/portal
            </code>
            .
          </p>
          <div className="flex items-center gap-3">
            <Button variant="primary" disabled>
              Open Stripe portal
            </Button>
            <span className="text-xs text-[var(--color-muted-foreground)]">
              Not yet available in this build.
            </span>
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
