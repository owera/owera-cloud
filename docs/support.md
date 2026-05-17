# Support

> **Audience: customers.** How to reach us, what we respond to and how fast, and what we do when things break. For the formal response-time table and how we measure ourselves against it, see [`compliance/runbooks/support-sla.md`](../compliance/runbooks/support-sla.md).

The Owera Agentic API is live at `https://owera-agentic-api.fly.dev` (the canonical hostname `api.owera.ai` resolves here once DNS is cut over). The customer dashboard is at [`app.owera.ai`](https://app.owera.ai) and the public status page at [`status.owera.ai`](https://status.owera.ai).

## How to reach us

Pick the lowest channel on this list that fits your need — it's both the fastest path and the one with the most context preserved.

| Channel | Address | Use for |
|---|---|---|
| **In-product chat** — recommended | Dashboard → **Support** (pre-fills your tenant + recent jobs) | Anything routine: questions, bug reports, SLA concerns. Pre-attaches your tenant context so we don't ping-pong on identification. |
| Email — general | `hello@owera.com` | Sales, billing, account changes, pricing questions, SKU requests, anything not covered below. |
| Email — security | `security@owera.com` | Vulnerability reports. See [`SECURITY.md`](../SECURITY.md). Do not file public issues for security findings. |
| Emergency — P0 outage | Reply `URGENT` to any prior thread, or page via the dashboard's **Report incident** banner (visible when status page shows degraded). | Production outage on your tenant when the dashboard chat path isn't responsive. Triggers the on-call pager out-of-hours. |
| GitHub issues | [Issues tracker](https://github.com/owera/owera-cloud/issues) | Public bug reports, feature requests. Paying customers should prefer the in-product chat — it gets routed to a human; GitHub issues do not page anyone. |
| Status page | [`status.owera.ai`](https://status.owera.ai) | Real-time platform health. Subscribe to incident notifications via the status page itself; you'll be emailed when an incident affects an SKU your tenant uses. |

**Escalation path.** Start with in-product chat (or email if you can't reach the dashboard). If the issue is a production outage and you haven't heard back inside the P0 first-response window (see SLA below), escalate via the emergency channel — this pages the on-call directly. We deliberately do not publish a phone number; the dashboard banner + emergency-email path is the authoritative pager trigger so we keep one audit-logged source of truth.

## Business hours

| Window | Hours (BRT, UTC−3) | What happens |
|---|---|---|
| Business hours | **08:00–18:00 BRT, Monday–Friday** (Macapá, Brazil) | Human on the support queue; P2/P3 worked in priority order. |
| After hours, weekends, BR holidays | Outside the window above | P0/P1 page the on-call ([`on-call-runbook.md`](../compliance/runbooks/on-call-runbook.md)); P2 and lower queue for next business day. |

Brazilian public holidays (federal + Amapá state) are honoured. The on-call schedule is the source of truth for "is anyone awake right now"; the support inbox is checked first thing on the next business morning.

## Severity definitions

| Severity | Definition | Examples |
|---|---|---|
| **P0 — Critical** | Production outage. API `/v1/jobs` returns 5xx for >5 min, billing materially incorrect (>10% off the operator-plane ledger), security breach in progress, or all dashboard sign-ins failing. | API down, dashboard inaccessible, jobs silently dropping, Stripe webhook signature mismatch storm. |
| **P1 — High** | Significant degradation. SKU SLA breaches for one or more tenants, a major surface broken for all customers on a tier, or sustained elevated error rate. | `triage-watch` first-response SLA missed at scale, dashboard showing stale data, status page wrong. |
| **P2 — Normal** | Functional bug or feature gap affecting one tenant or a non-critical workflow. | SKU returning wrong outputs for a niche input, dashboard glitch, API quirk, account-management issue that isn't blocking. |
| **P3 — Low** | Cosmetic, documentation, or polish issue. | Typo in docs, slightly off pricing display, broken doc link, UX paper-cut. |

Severity is what *we* assess on receipt. If you submit a P0 ticket and it turns out to be a P2, we downgrade and tell you why; if you submit a P3 and it's actually a P0, we upgrade and page immediately.

## Response SLAs

The full table — first-response targets by severity, the rolling-30-day ≥90% metric, escalation triggers, and how we track it — lives in [`compliance/runbooks/support-sla.md`](../compliance/runbooks/support-sla.md).

The headline numbers (response = "a human acknowledges, triages, and tells you what's happening next"):

| Severity | First response (business hours) | First response (after hours) |
|---|---|---|
| P0 | **≤30 min** | **≤2 h** (pager-driven) |
| P1 | ≤2 h | ≤8 h |
| P2 | ≤24 h business hours | n/a — queued to next business day |
| P3 | ≤72 h business hours | n/a — queued to next business day |

These are response targets — resolution depends on the issue. Tighter contractual SLAs (e.g., 15-min P0 24×7) are available on enterprise tier; email `hello@owera.com` for a quote.

## Incident response

When the platform is degraded:

1. **`status.owera.ai`** is updated within 5 minutes of confirmed degradation. Severity, scope, ETA-if-known.
2. **API responses** include an `X-Owera-Incident: <id>` header pointing to the incident on the status page.
3. **Email notifications** go out to subscribed addresses on the status page when an incident is opened, updated, or resolved.
4. **Post-mortem** is published within 5 business days for P0/P1 incidents. Public on the status page; redacted of customer specifics.

For the cadence and wording of customer-facing comms during an incident, see [`compliance/runbooks/incident-comms-templates.md`](../compliance/runbooks/incident-comms-templates.md). Outage-grade communication (T1–T4 templates) is governed by that runbook; the standard support channels above are for the everyday case.

## SLA credits

Service credits for SLA breaches are spelled out in your engagement letter or order form. The standard structure for V0:

| Monthly uptime | Credit |
|---|---|
| ≥99.9% | None |
| 99.0% – 99.9% | 10% of monthly fee |
| 95.0% – 99.0% | 25% of monthly fee |
| <95.0% | 50% of monthly fee |

Credits are applied to the next invoice automatically when the post-mortem confirms the breach. You don't need to file a claim — the SLA-breach notification (template T5 in [`incident-comms-templates.md`](../compliance/runbooks/incident-comms-templates.md)) names the Stripe credit memo ID.

Uptime is measured per-SKU (a `triage-watch` outage doesn't credit `campaign-swarm`-only customers) and excludes scheduled maintenance windows announced ≥48 h in advance.

## What we don't support

- **Self-service migrations** between tenants (e.g., moving jobs from tenant A to tenant B). Email us; we'll handle it manually.
- **Custom SKU development** as a contracted engagement. We accept SKU proposals (see [`CONTRIBUTING.md`](../CONTRIBUTING.md)); we don't take "build me a custom agent" work as part of the standard plan.
- **Operator-plane debugging** from customer support. If the fleet has a problem, that's our internal escalation — you don't need to file a fleet issue, and we don't expose fleet internals on the support channel.
- **Pre-sales POCs without an MSA.** During V0 (private beta), we only onboard customers under signed MSA. Sales conversations go through `hello@owera.com`.

## Related

- [`compliance/runbooks/support-sla.md`](../compliance/runbooks/support-sla.md) — formal SLA targets, rolling metric, escalation triggers, tracking
- [`compliance/runbooks/on-call-runbook.md`](../compliance/runbooks/on-call-runbook.md) — who carries the pager, escalation tree
- [`compliance/runbooks/incident-comms-templates.md`](../compliance/runbooks/incident-comms-templates.md) — what customers hear during an outage
- [`onboarding.md`](onboarding.md) — customer-facing first-job walkthrough
- [`api.md`](api.md) — endpoint reference (most "is X supposed to work?" questions resolve here)
- [`pricing.md`](pricing.md) — SKU pricing modes, cost-cap math
