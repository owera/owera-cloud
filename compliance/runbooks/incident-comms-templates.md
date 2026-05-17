# Incident comms templates (T20.3)

> **Audience: customer-success operator on an active incident.** These templates are starting points, not scripts — adapt to the situation. The principles below come first; the templates follow.

**Owner:** SOE workstream
**Used by:** PagerDuty drill (T19.6), real incidents (any severity), SLA-breach notifications.
**Review cadence:** After every drill or real incident; templates updated based on what worked.

## Principles (read every time)

1. **Speed beats prose.** Acknowledge fast, refine later. The first message is "we see it, here's what we know" — even if "what we know" is "investigating." Empty acknowledgement is better than silence.
2. **One source of truth.** Status page is the canonical record; email + Slack reference it. Don't fork details across channels.
3. **No blame, no speculation.** Until root cause is confirmed, say "investigating" — not "looks like X." Misattributed cause in a public message is harder to retract than a delayed update.
4. **Action over apology.** "We've isolated the issue and restored service in 12 min" lands harder than three paragraphs of "we're so sorry."
5. **Honesty about scope.** If you don't know how many customers are affected, say "affected scope still being confirmed." Don't undersell.
6. **Brazilian operator, global customers.** Default to English; offer pt-BR for Brazilian customers if it accelerates resolution. Time stamps in UTC + local (BRT for Brazilian customers, customer's local for others when known).

## Template catalogue

### T1 — Initial acknowledgement (within 5 min of paged)

**Channel:** status page (banner) + email to affected customers.

**Subject:** `[Owera Agentic] Investigating — <one-sentence symptom>`

```
We are investigating an issue affecting <SKU(s) or "the API"> starting
approximately <UTC time>.

Current impact: <one sentence — e.g., "job submissions are returning 5xx for
a subset of tenants" or "scope is still being confirmed">.

We are working the issue now. The next update will follow within 15 minutes
even if our position has not changed.

Status: https://status.owera.ai

— Owera Agentic operations team
```

### T2 — Confirmed degradation (within 30 min of T1)

**Channel:** status page (incident open) + email update to affected customers.

**Subject:** `[Owera Agentic] Degraded — <symptom> — investigating`

```
Update <UTC time>:

Confirmed: <what we know — symptom, affected SKUs, approximate % of jobs/
customers impacted>.
Status: Investigating root cause.
Workaround: <if any — e.g., "retries with idempotency-key are succeeding";
otherwise omit this line>.
Next update: <UTC time, no more than 30 min out>.

Status page: https://status.owera.ai/incidents/<id>

We will continue updating every 30 minutes until resolution.

— Owera Agentic operations team
```

### T3 — Root cause identified (mid-incident)

**Channel:** status page update + email.

```
Update <UTC time>:

Root cause identified: <brief, factual — e.g., "a Cloudflare tunnel
endpoint became unreachable from one region; failover did not trigger
automatically">.
Mitigation in progress: <what's being done, e.g., "manually failing over
to the secondary endpoint">.
Estimated time to recovery: <best honest estimate>.
Next update: <UTC time, ≤30 min out>.

Status page: https://status.owera.ai/incidents/<id>

— Owera Agentic operations team
```

### T4 — Recovery confirmed

**Channel:** status page (incident resolved) + email.

**Subject:** `[Owera Agentic] Resolved — <one-sentence symptom>`

```
Resolved <UTC time>.

Service restored at <UTC time>. End-to-end recovery validated against
<the synthetic check / canary / verification scenario you used>.

Total customer-impact window: <UTC start> to <UTC end> (<duration>).
Approximate impact: <jobs failed / customers affected / dollars at risk if
quantifiable>.

A post-mortem will be published within 5 business days at
https://status.owera.ai/incidents/<id> with timeline, root cause, and
remediation plan.

Thank you for your patience.

— Owera Agentic operations team
```

### T5 — SLA breach notification (one-off)

**Channel:** direct email to the affected customer's primary contact + the dashboard support inbox row.

**Subject:** `[Owera Agentic] SLA breach on <SKU> on <date> — credit issued`

```
Hi <name>,

The <SKU> SLA committed in your MSA (Section <X>) was not met on
<UTC date> for the following jobs:

  <job_id_1>  submitted <ts>  promised by <ts>  actually completed <ts>
  <job_id_2>  submitted <ts>  promised by <ts>  actually completed <ts>
  ...

Root cause: <brief, factual>.
Customer-side action required: <none / specific request>.
Credit: <amount, applied to next invoice / refunded to card>. Reference
       Stripe credit memo <id>.

We have <opened ticket / scheduled work / shipped patch> to prevent
recurrence. Tracking: <link to issue or PR>.

Reply to this email with any questions, or open a support ticket from
your dashboard.

— Rodrigo, Owera Software Ltda
```

### T6 — Post-mortem (within 5 business days)

**Channel:** public, on status page; linked in resolved-incident email.

Format: see [`post-mortem-template.md`](post-mortem-template.md) (TODO: extract from prior incidents into a template). Minimum sections:

1. **Summary** (one paragraph)
2. **Timeline** (UTC, minute-by-minute)
3. **Customer impact** (jobs, tenants, dollars)
4. **Root cause**
5. **What went well**
6. **What didn't go well**
7. **Remediation items** (each owned + dated)

### T7 — Drill (no real impact)

**Channel:** internal Slack + dashboard support inbox (silent — no customer email).

```
[DRILL] <date> — <scenario name>

This is a planned incident-response drill per compliance/runbooks/
incident-response.md. No customer impact. Stage: <T1 / T2 / T3 / T4>.

The on-call team is exercising:
- <what's being tested — e.g., "Cloudflare tunnel failover">
- <expected observable behaviour>

If you see this on a real customer surface (status page banner, customer
email), STOP THE DRILL and treat it as a real incident.

End of drill expected by <UTC time>.

— Owera Agentic operations team (drill)
```

### T8 — Security-issue customer notification (LGPD/GDPR triggered)

**Channel:** direct email to every affected customer; status page footnote (no detail) for the broader community.

**Subject:** `[Owera Agentic] Security notice — <date>`

```
Dear <Customer>,

On <UTC date>, Owera Software Ltda identified <factual one-sentence
description of the issue>.

Affected scope:
- Data classes potentially exposed: <list>
- Customers potentially affected: <count or "all customers using
  <feature> in <window>">
- Mitigation completed: <date>

What we did:
1. <action>
2. <action>
3. <action>

What you should do:
- <e.g., rotate API keys created before <date> via the dashboard>
- <e.g., review your audit log via /v1/audit for the period <range>>

LGPD obligations: We have reported this incident to the Autoridade
Nacional de Proteção de Dados (ANPD) as required under Art. 48 of LGPD
13.709/2018. If you are an EU customer, we will provide the equivalent
GDPR Article 33 / 34 notice via separate communication.

Contact:
- General questions: hello@owera.com
- Security questions: security@owera.com
- DPO (LGPD): dpo@owera.com

We are deeply sorry for the disruption. The post-mortem will be
published at https://status.owera.ai once remediation is verified.

— Rodrigo Recio, Owera Software Ltda
```

⚠ **Send T8 only after legal sign-off.** Notification content is binding once sent. Coordinate with counsel (and the DPO if separate from Rodrigo) before transmission.

## Cadence rules

| Stage | Update interval |
|---|---|
| T1 → T2 | ≤30 min |
| T2 → T3 | ≤30 min |
| T3 → T4 | ≤30 min (or shorter if change of status) |
| T4 → T6 | 5 business days |
| Drill (T7) | Per drill plan; no customer-channel comms |
| Security (T8) | Within ANPD window (72 h where feasible per Art. 48) |

## Related

- [`incident-response.md`](incident-response.md) — operator playbook (what to do, not what to say)
- [`on-call-runbook.md`](on-call-runbook.md) — paging, escalation, who runs which step
- [`../policies/incident-response-policy.md`](../policies/incident-response-policy.md) — formal policy
- [`../policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md) — Art. 48 notification rules
- [`../policies/ga-gate.md`](../policies/ga-gate.md) — G4 measures drill cadence

## Change log

| Date | Author | Change |
|---|---|---|
| 2026-05-17 | TL (Wave 9 prep) | Initial draft for T20.3 closeout |
