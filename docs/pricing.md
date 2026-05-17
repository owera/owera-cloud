# Pricing

> **Audience: customers.** Concrete pricing numbers (USD per call, monthly retainer amounts) are set per-customer during onboarding. This document covers the **shape** of the pricing — what's metered, what triggers overages, where the cost caps live. For a quote, email `hello@owera.com`.

## Billing model

Two pricing modes, depending on the SKU:

| Mode | What it means | Used by |
|---|---|---|
| **Subscription + overage** | Flat monthly fee covers a baseline volume; overage billed per unit above the baseline. | Recurring SKUs: `triage-watch`, `code-audit`, plus V2+ recurring SKUs. |
| **Per-job fixed** | One-shot price per job, no monthly commitment. | One-shot SKUs: `campaign-swarm`, `research-brief`, plus V2+ one-shot SKUs. |

Billing cycle is **calendar month**, denominated in **USD**. Invoices issue on the 1st for the prior month; payment due net-15. BRL pricing is available for Brazilian customers on request and is added natively as Brazilian customer volume justifies a parallel Stripe Brazil setup.

## Cost caps

Every tenant has a **monthly cost cap** set during onboarding. The cap is enforced at job-submission time:

- If a job's estimated cost would push the tenant above the cap for the current period, `POST /v1/jobs` returns `402 Payment Required` with a `detail` explaining the cap and the remaining headroom.
- Per-job cost estimates derive from the SKU's pricing schema; estimates are conservative (assume the worst-case input size).
- The cap is raisable from the dashboard for self-serve tiers or by emailing `hello@owera.com` for managed tiers.
- The cap protects you from runaway agents; it is **not** a credit limit — you still pay for work that completes below the cap.

## SKU catalogue

The catalogue ramps in tiers. V0 ships at private beta; V1 ships at GA. V2-V4 are planned and not yet exposed.

### V0 (private beta)

| SKU | Mode | What you get | SLA |
|---|---|---|---|
| `triage-watch` | Subscription + overage per ticket above tier | Continuous Zendesk/helpdesk triage; categorization, draft replies, escalation to your pager when human-needed. | <2 min ticket response. |
| `campaign-swarm` | Per-campaign tiered (S/M/L by # channels + posts) | Coordinated multi-channel launch — Twitter, LinkedIn, email — from a single brief. | ≤4-12 h depending on tier. |

#### V0 reference prices (USD, billed via Stripe)

These are the V0 reference prices the billing pipeline emits against. Per-customer pricing on managed contracts may vary; the **default** Stripe price slot is the one in this table.

| SKU | Stripe price ref | Unit | Amount (USD cents) | Recurring? |
|---|---|---|---|---|
| `triage-watch` (base subscription) | `triage-watch:base` | per month | 49,900 ($499) | monthly |
| `triage-watch` (per-ticket metered overage) | `triage-watch:ticket` | per ticket processed | 200 ($2.00) | metered (sum, monthly) |
| `campaign-swarm` (S) | `campaign-swarm:S` | per campaign | 49,900 ($499) | per-campaign |
| `campaign-swarm` (M) | `campaign-swarm:M` | per campaign | 99,900 ($999) | per-campaign |
| `campaign-swarm` (L) | `campaign-swarm:L` | per campaign | 199,900 ($1,999) | per-campaign |

The concrete Stripe product/price IDs (test mode + production) are recorded in `api/internal/billing/stripe_ids.go` and never appear in this document.

### V1 (general availability)

Adds:

| SKU | Mode | What you get | SLA |
|---|---|---|---|
| `research-brief` | Per-brief fixed (S/M/L by depth) | Deep web research with citations on a topic. Used for competitive intel, market sizing, due diligence. | ≤24 h delivery. |
| `code-audit` | Subscription per repo + overage on findings | Recurring code-quality + security audit on a target repo. Opens issues + PRs nightly. | Daily run, ≤2 h completion. |

### V2 (90 days post-GA — planned)

Adds: `dep-upgrade`, `inbox-triage`, `monitor-watch`, `content-batch`.

### V3 (180 days post-GA — planned, after SOC 2 readiness)

Adds: `xcode-ci`, `app-build`, `docs-author`, `incident-postmortem`.

### V4 (12 months+ — demand-driven)

Adds: `test-author`, `migration-pilot`, `lead-enrich`, `etl-flow`.

The full forward-looking SKU table (with descriptions and margin levers) lives in the canonical plan at `owera-fleet/knowing-all-you-now-calm-leaf.md`. SKU schemas are versioned (`triage-watch@v1`); breaking changes ship as parallel `@v2` SKUs without retiring `@v1`.

## What's metered

Per-tenant, per-billing-period, queryable via `GET /v1/usage`:

| Meter | Unit | Used by |
|---|---|---|
| Job count | Number of completed jobs (by SKU) | All SKUs. |
| Compute seconds | CPU-seconds on worker Macs | Long-running SKUs (`research-brief`, `migration-pilot`, future `xcode-ci`). |
| Tickets processed | Count of helpdesk tickets evaluated | `triage-watch`, future `inbox-triage`. |
| Findings opened | Issues + PRs filed | `code-audit`. |
| Posts published | Outbound posts across channels | `campaign-swarm`, future `content-batch`. |

Meter readings come from the operator-plane signed ledger, not from the API surface. If your dashboard says "47 tickets processed" and our ledger says 47, those numbers are bit-identical — they came from the same source.

## Subscriptions, cancellations, refunds

- **Subscriptions** are month-to-month with no minimum term. Cancel anytime from the dashboard or by emailing `hello@owera.com`. Cancellation takes effect at the end of the current billing period.
- **One-shot jobs** are billed on completion. Cancelled jobs (cancelled before `running` state) are not charged. Cancelled jobs that already entered `running` state are charged pro-rated based on compute consumed.
- **Refunds** are issued on a case-by-case basis for SLA breaches, demonstrable platform faults, or billing errors. Contact `hello@owera.com` with your tenant ID and the relevant invoice number.

## Tax

Owera Software Ltda is a Brazilian merchant of record. Brazilian customers see ISS (municipal services tax) and may see PIS/COFINS depending on classification. International customers see USD-denominated invoices with no Brazilian tax withholding; your own jurisdiction's import-of-services tax (VAT, GST, etc.) is your responsibility.

For the formal tax treatment in your jurisdiction, consult your accountant and reference the engagement letter.
