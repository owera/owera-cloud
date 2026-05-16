# Support

> **Audience: customers.** How to reach us, what we respond to and how fast, and what we do when things break.

## How to reach us

| Channel | Address | Use for |
|---|---|---|
| Email — general | `hello@owera.com` | Sales, billing, account changes, pricing questions, SKU requests, anything not covered below. |
| Email — security | `security@owera.com` | Vulnerability reports. See [`SECURITY.md`](../SECURITY.md). Do not file public issues for security findings. |
| GitHub issues | [Issues tracker](https://github.com/owera/owera-cloud/issues) | Public bug reports, feature requests. Paying customers should prefer the `customer_issue` template, which auto-routes to support. |
| Status page | [`status.owera.ai`](https://status.owera.ai) | Real-time platform health. Subscribe to incident notifications via the status page itself. |
| Dashboard | [`app.owera.ai`](https://app.owera.ai) → Support | In-product support form, scoped to your tenant. Recommended path for paying customers — pre-fills your tenant context. |

Human support hours: **Brazil business hours, 08:00-18:00 BRT (UTC−3), Monday-Friday.** Outside those hours, urgent (severity 1) issues are paged on-call; everything else is queued for next business day.

## Severity definitions

| Severity | Definition | Examples |
|---|---|---|
| **S1 — Critical** | Production outage. API returns 5xx for >5 min, or billing materially incorrect (>10% off ledger), or security breach in progress. | API down, dashboard inaccessible, jobs silently dropping. |
| **S2 — High** | Significant degradation. SLA breach on one or more SKUs, or a major feature broken for all customers on a tier. | Job submissions timing out, dashboard showing stale data, status page incorrect. |
| **S3 — Normal** | Functional bug or feature gap affecting one customer or a non-critical workflow. | An SKU returning wrong outputs for a niche input, a dashboard glitch, an API quirk. |
| **S4 — Low** | Cosmetic or documentation issue. | Typo in docs, slightly-off pricing display, a 404 link. |

## Response SLAs

Response = "a human acknowledges, triages, and tells you what's happening next." Resolution time depends on the issue.

| Severity | Response time | Resolution target |
|---|---|---|
| S1 | **30 min during business hours; 2 h outside.** Pager-driven. | Best-effort, continuous engagement until resolved or downgraded. |
| S2 | 4 business hours | 2 business days |
| S3 | 1 business day | Next minor release or release-cadence-bounded |
| S4 | 3 business days | Next release window |

These are response targets for the **private beta + early-GA** window. Tighter contractual SLAs (e.g. 15-min S1 response 24/7) are available on enterprise tier — email `hello@owera.com` for a quote.

> **Operator note.** The 30-min / 2-h S1 numbers are judgment calls for V0 — Rodrigo is the on-call, and a Brazil-hours pager rotation hasn't been set up yet. Revisit when on-call rotation lands (per `owera-fleet` Phase 4 / T19.6).

## Incident response

When the platform is degraded:

1. **`status.owera.ai`** is updated within 5 minutes of confirmed degradation. Severity, scope, ETA-if-known.
2. **API responses** include an `X-Owera-Incident: <id>` header pointing to the incident on the status page.
3. **Email notifications** go out to subscribed addresses on the status page when an incident is opened, updated, or resolved.
4. **Post-mortem** is published within 5 business days for S1/S2 incidents. Public on the status page; redacted of customer specifics.

## SLA credits

Service credits for SLA breaches are spelled out in your engagement letter or order form. The standard structure:

| Monthly uptime | Credit |
|---|---|
| ≥99.9% | None |
| 99.0% - 99.9% | 10% of monthly fee |
| 95.0% - 99.0% | 25% of monthly fee |
| <95.0% | 50% of monthly fee |

Credits are applied to the next invoice automatically when the post-mortem confirms the breach. You don't need to file a claim.

Uptime is measured per-SKU (a `triage-watch` outage doesn't credit `campaign-swarm` customers) and excludes scheduled maintenance windows announced ≥48 h in advance.

## What we don't support

- **Self-service migrations** between tenants (e.g. moving jobs from tenant A to tenant B). Email us; we'll handle it manually.
- **Custom SKU development** as a contracted engagement. We accept SKU proposals (see [`CONTRIBUTING.md`](../CONTRIBUTING.md)); we don't take "build me a custom agent" work as part of the standard plan.
- **Operator-plane debugging** from customer support. If the fleet has a problem, that's our internal escalation — you don't need to file a fleet issue, and we don't expose fleet internals on the support channel.
