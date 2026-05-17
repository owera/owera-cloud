# Support SLA (T20.2)

> **Audience: customer-success operator + on-call.** The formal commitments behind [`docs/support.md`](../../docs/support.md). Customer-facing summary lives in support.md; the rolling-metric math, escalation triggers, and the tracking pipeline live here.

**Owner:** SOE workstream (customer-success operator), accountable to the TL for breach-credit issuance.
**Reviewed by:** TL + Rodrigo, monthly during the V0 → GA window. Quarterly after GA.
**Effective:** 2026-05-17 (V0). Renegotiated per-customer on the enterprise tier via the MSA.

## 1. Scope

This runbook covers **first-response SLAs** on support tickets opened through any of the channels named in `docs/support.md` ("How to reach us"). It does **not** cover:

- **Uptime SLAs** — measured per-SKU and governed by the customer's order form. The credit table in `docs/support.md` is the customer-visible version; the operator-plane ledger is the source of truth for whether a breach occurred.
- **SKU latency SLAs** — the `<2 min` for `triage-watch` and `<15 min` for `campaign-swarm` are SKU-level commitments enforced at job-submit time by the catalog; SLA breaches surface as P1 support tickets per §3.
- **Incident-response cadence** — governed by [`incident-comms-templates.md`](incident-comms-templates.md) (the T1/T2/T3/T4 update interval rules).

When a P0 ticket *is* an active incident, the incident-response cadence (T1 ≤5 min ack, then ≤30 min update intervals) layers on top of — and is faster than — the P0 first-response target below.

## 2. First-response targets

"First response" = a human reads the ticket, classifies the severity, and posts an acknowledgement that names the next step and the next update time. Auto-acknowledgement emails do not count.

| Severity | Business hours (08:00–18:00 BRT, Mon–Fri) | After hours / weekends / BR holidays |
|---|---|---|
| **P0 — Critical** | ≤30 min | ≤2 h (pager-driven) |
| **P1 — High** | ≤2 h | ≤8 h |
| **P2 — Normal** | ≤24 h business hours | n/a — queued to next business day |
| **P3 — Low** | ≤72 h business hours | n/a — queued to next business day |

Severity definitions live in `docs/support.md` so customers and the operator read from the same dictionary. When operator and customer disagree on severity, the operator's assessment is recorded on the ticket with a one-line reason, and the customer is told why; the customer can re-escalate via the emergency channel if they disagree.

**Clock-starts-when.** The first-response clock starts at the **timestamp on the inbound channel** — when the in-product chat row is created, when the inbound email lands in the support inbox, when the GitHub issue is opened. For tickets opened outside business hours but flagged P2/P3, the clock pauses until the start of the next business day and resumes from there.

## 3. Rolling 30-day metric

We commit to **≥90% of tickets meeting their first-response target across a rolling 30-day window**, measured per-severity and in aggregate. The metric is computed at the end of each business day from the ticket pipeline (§5) and published internally on Monday hand-offs (per [`on-call-runbook.md`](on-call-runbook.md) §1).

| Metric | Target | Action when below target |
|---|---|---|
| P0 ≤target | ≥95% rolling 30-day | TL + on-call lead review on the next Monday hand-off; root-cause the misses; one structural fix landed before the next Monday. |
| P1 ≤target | ≥90% rolling 30-day | Same review, lower urgency. |
| P2 ≤target | ≥90% rolling 30-day | Reviewed monthly; staffing or process change considered. |
| P3 ≤target | ≥85% rolling 30-day | Reviewed monthly. |
| All-tickets ≤target | ≥90% rolling 30-day | Aggregate health signal; published in GA-gate G3 evidence per [`compliance/policies/ga-gate.md`](../policies/ga-gate.md). |

Below-target months trigger a written retrospective: what was missed, why, structural remediation, owner, due date. Stored in `compliance/sla-retros/YYYY-MM.md` (TODO: directory created with the first retro).

The metric **excludes**:

- Tickets the customer closed before we got to them (self-resolved).
- Tickets misclassified by the inbound channel and re-routed within 30 min (e.g., a sales question into the support channel).
- Tickets opened during a planned maintenance window announced ≥48 h in advance, when the ticket is about the announced-and-degraded surface.

The metric **includes** every other ticket, even when our miss was driven by a customer not being reachable for clarification — in that case the response clock keeps running, because "first response" is about us, not about back-and-forth time.

## 4. Escalation triggers

Tickets that age past these thresholds escalate **automatically** (the pipeline pages on the threshold; operator action does not unblock the clock):

| Severity | Unack threshold | Escalates to | Action |
|---|---|---|---|
| **P0** | 15 min unack (any time of day) | Primary on-call via PagerDuty | Page fires per [`on-call-runbook.md`](on-call-runbook.md) §3. If primary doesn't ack within 10 min of page, secondary; then SRE lead; then CTO. |
| **P0** | 30 min unack (any time) | Primary + secondary in parallel | Belt-and-braces — the customer's first-response target is about to breach. |
| **P1** | 1 h unack (business hours) | Primary on-call via PagerDuty | Same escalation tree; lower-urgency page text. |
| **P1** | 4 h unack (after hours) | Primary on-call via PagerDuty | Same. |
| **P2** | 4 business hours unack | Operator queue — re-prioritise | No page; surfaces in the daily ticket-stand-up. |
| **P3** | 2 business days unack | Operator queue — re-prioritise | Same. |

The on-call runbook ([`on-call-runbook.md`](on-call-runbook.md)) is the source of truth for **who carries which pager** and the parallel escalation tree once a P0/P1 has been escalated; this runbook tells you **when** to escalate.

**Manual escalation.** Operator can escalate at any time without waiting for the threshold — when a customer's tone suggests the issue is bigger than their reported severity, when a P2 is repeating across multiple tenants, or when a P0 looks like an active incident (in which case the incident-response runbook takes over and a status-page banner goes up before the next ticket update).

## 5. Tracking pipeline

V0 tracking is deliberately minimal — we ship the discipline, then layer on tooling as ticket volume justifies it.

| Step | Today (V0) | Targeted (V1+) |
|---|---|---|
| Ticket source-of-truth | The support inbox (`hello@owera.com`) + in-product chat row + GitHub issues with the `customer_issue` label. Each event copied into one structured log line. | A real ticketing system (Linear, Plain, or similar) with channel ingestion. |
| Structured log | JSONL appended to `~/.hermes/logs/support.jsonl` on the gateway (today: TBD path while the pipeline is being built; the SOE workstream is the owner). One line per ticket event (open, classify, first-response, close), including `ticket_id`, `tenant_id`, `severity`, `channel`, `opened_at`, `first_response_at`, `closed_at`, `breached`. | Database-backed; same fields, queryable. |
| Daily roll-up | Manual `jq`-driven script (TODO: `scripts/support-sla-rollup.sh` — author at first month-end if not before). Publishes today's first-response count + breaches by severity to `#support-metrics`. | Automated; runs as a launchd agent on the gateway alongside the existing fleet jobs. |
| Monday hand-off | Surfaces in the on-call hand-off note (per [`on-call-runbook.md`](on-call-runbook.md) §6) when the rolling metric is approaching or below target. | Same — Monday hand-off is the human checkpoint. |
| Customer-visible reporting | None during V0 — the metric is internal. Customers see their own ticket history in the dashboard. | A per-tenant "your support history" page on the dashboard. |

The log fields shape (V0):

```json
{
  "ts": "2026-05-17T14:22:03Z",
  "ticket_id": "tkt_01HXXX...",
  "tenant_id": "ten_xyz...",
  "channel": "dashboard|email|github|emergency",
  "severity": "P0|P1|P2|P3",
  "event": "opened|classified|first_response|closed",
  "opened_at": "2026-05-17T14:00:00Z",
  "first_response_at": "2026-05-17T14:22:03Z",
  "closed_at": null,
  "first_response_seconds": 1323,
  "target_seconds": 1800,
  "breached": false,
  "business_hours": true
}
```

Bash-3.2 compatible, JSON-Lines, one event per row — same shape as the existing `~/.hermes/logs/*.jsonl` files the fleet runbooks already consume. The roll-up script reuses the `jq` patterns from `fleet-health-snapshot.sh`.

## 6. Outage-grade communication

Standard support replies (in-product chat, email back-and-forth) are operator's-judgement-of-the-moment. When a P0 or P1 is an **active incident** affecting multiple tenants, the operator switches to the templated cadence in [`incident-comms-templates.md`](incident-comms-templates.md):

| Template | When |
|---|---|
| T1 — Initial acknowledgement | Within 5 min of paged — *replaces* the standard P0 first-response window for incident-scope tickets. |
| T2 — Confirmed degradation | Within 30 min of T1. |
| T3 — Root cause identified | When known. |
| T4 — Recovery confirmed | When verified. |
| T5 — SLA breach notification | After resolution, per-customer, for SKU-SLA breaches identified during the incident. |

For a ticket that turns out to be one tenant's view of a broader incident, the standard SLA targets in §2 still apply to the per-tenant acknowledgement, but the operator should link the customer to the status-page incident rather than write a bespoke triage thread.

## 7. Enterprise overrides

The targets in §2 are the V0 default. Enterprise-tier customers can negotiate tighter SLAs as part of the MSA — typical asks:

- 15-min P0 first response 24×7 (rather than 30-min business / 2-h after-hours).
- Per-tenant named-operator routing (vs. shared queue).
- 99.99% uptime per SKU instead of 99.9% (with renegotiated credit table).

When an MSA names a tighter SLA, the ticket pipeline reads the override from the tenant record (TODO: `monthly_cap_cents`-style admin endpoint for SLA overrides — design pending; today, overrides are flagged manually on the customer's onboarding-history file and respected by the operator).

## 8. Related

- [`docs/support.md`](../../docs/support.md) — customer-facing summary
- [`on-call-runbook.md`](on-call-runbook.md) — pager rotation + escalation tree
- [`incident-comms-templates.md`](incident-comms-templates.md) — outage-grade communication templates
- [`incident-response.md`](incident-response.md) — operator playbook for active incidents
- [`onboarding-playbook.md`](onboarding-playbook.md) — Day-7 review surfaces support-channel expectations to new customers
- [`../policies/ga-gate.md`](../policies/ga-gate.md) — G3 (customer-success operations) uses the rolling-30-day metric as evidence

## 9. Change log

| Date | Author | Change |
|---|---|---|
| 2026-05-17 | TL (Wave 9 — T20.2) | Initial draft. Four-tier P0/P1/P2/P3 table, rolling-30-day ≥90% metric, escalation triggers, JSONL tracking shape, cross-links to on-call + incident-comms runbooks. |
