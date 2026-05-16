# Incident response runbook

Step-by-step for a Sev1. The policy framing is in [`../policies/incident-response-policy.md`](../policies/incident-response-policy.md); this runbook is what you execute under pressure.

## 0. Trigger

You are reading this because:

- You were paged by PagerDuty for a Sev1, OR
- You declared an incident based on customer reports / your own observation, OR
- Someone in `#incident-declare` asked you to take IC.

If none of the above and you're just reading: thanks. Re-read it every quarter so the muscle memory is there.

## 1. First five minutes — declare and assemble

```
[ ] Acknowledge the page in PagerDuty.
[ ] Post in #incident-declare with: short description, suspected severity, affected systems.
[ ] PagerDuty auto-creates #incident-<n> Slack channel; move there.
[ ] Confirm or set severity per ../policies/incident-response-policy.md §1.
[ ] Assign roles in the new channel:
    - IC: <you, by default>
    - Technical lead: <who has the most context>
    - Comms lead: <customer-success on-call OR CEO if scope is large>
    - Scribe: <any available engineer>
[ ] If Sev1 with potential LGPD/GDPR exposure: page the CISO directly.
[ ] Start the timeline. Every meaningful event gets a UTC timestamp + one-line description.
```

## 2. First fifteen minutes — communicate

```
[ ] Comms lead posts a holding update to status.owera.ai:
    "We are investigating reports of <X>. Updates in 30 minutes."
[ ] If customer data is potentially involved, IC notifies CISO + external counsel
    (the latter via the CFO's contact).
[ ] If payments are affected, IC notifies CFO to engage Stripe support.
[ ] Tech lead starts diagnosis. IC stays out of debugging — IC's job is decisions.
```

Status page template (live):
```
[INVESTIGATING] We are investigating reports of <symptom>. Affected: <api / web / both>.
Started: <UTC timestamp>. Updates every 30 minutes.
```

## 3. Diagnose and mitigate

```
[ ] Tech lead works the hypothesis tree. Common starting points:
    - Recent deploy?           → fly releases / vercel deployments
    - Dependency outage?       → status pages for Fly, Vercel, Cloudflare, Stripe, Clerk
    - Operator plane?          → fleetctl status (owera-fleet)
    - Tunnel?                  → launchctl print system/ai.owera.cloudflared
    - Audit-log integrity?     → run hash-chain validator (see ../policies/data-retention-policy.md §3)
[ ] Rollback first if a recent change is plausible. Per ../policies/change-management-policy.md §5.
[ ] IC re-levels severity as facts develop. Re-levels are logged in the timeline.
[ ] Comms lead posts status updates every 30 minutes, even if "no change".
```

## 4. Resolve

```
[ ] Confirm customer-facing symptom is gone. Verify with the same probe that triggered the page.
[ ] Comms lead posts "[MONITORING]" to status page. 30-minute monitoring window.
[ ] At end of monitoring window, post "[RESOLVED]" with one-sentence root cause.
[ ] IC closes the PagerDuty incident.
```

## 5. Customer communications (Sev1, data potentially affected)

Within **1 hour** of confirming customer data was potentially exposed, send the following via SES (template `incident-update`):

```
Subject: Owera Agentic security incident — what we know so far

We are writing to inform you of a security incident affecting <systems> from
<start UTC> to <end UTC>. Based on our current investigation:

- What happened: <one-paragraph factual summary>
- What data may have been affected: <specific categories>
- What we have done: <containment + remediation steps>
- What you should do: <concrete actions, if any>

We will send an updated communication within 24 hours with our final findings.
For questions, reply to this email or contact security@owera.ai.

— The Owera team
```

CISO + external counsel review every customer-facing breach communication before send.

## 6. Regulator notifications

### LGPD — ANPD (Brazil)

LGPD Art. 48 requires notification "within a reasonable time". Owera treats this as **≤72 hours** from confirming a reportable incident (mirroring GDPR Art. 33 for consistency).

```
[ ] CISO drafts the ANPD notification using the official template at
    <https://www.gov.br/anpd/pt-br>.
[ ] External Brazilian counsel reviews.
[ ] CISO submits via the ANPD portal.
[ ] Submission receipt stored in 1Password under Owera/Compliance/ANPD-notifications/.
[ ] Timeline updated with submission timestamp.
```

Reportable threshold: incidents that "may cause relevant risk or damage to data subjects". When in doubt, notify — the cost of over-reporting is administrative; the cost of under-reporting is regulatory action.

### GDPR — lead DPA (EU, conditional)

Only applicable once Owera serves EU customers. The DPO appointed at that time owns this step. Notification within 72 hours per GDPR Art. 33.

### Stripe — payment incidents

If payments are affected: CFO files via the Stripe dashboard incident form within 24 hours. Stripe may require additional information; respond promptly.

## 7. Evidence preservation

Before remediation alters logs:

```
[ ] Snapshot affected service logs to an isolated read-only bucket
    (s3://owera-incident-evidence/<incident-id>/).
[ ] Snapshot audit-log WORM store as of declaration time.
[ ] Preserve any compromised credentials in 1Password under
    compromised-credentials/<incident-id>/ for forensic retention (1 year minimum).
[ ] Capture command-line and console output the responder used to diagnose.
```

## 8. Post-mortem

Within **5 business days**, the IC publishes a blameless post-mortem at:

```
/Users/claw3/owera-cloud/compliance/runbooks/post-mortems/<YYYY-MM-DD>-<short-slug>.md
```

Required sections per [`../policies/incident-response-policy.md`](../policies/incident-response-policy.md) §4. The post-mortem is reviewed in the next monthly engineering review; action items are tracked to completion.

## 9. Severity-specific addenda

### Sev1: Suspected credential leak (e.g. accidental git push)

```
[ ] Within 15 min: rotate the credential. (See ../../infra/secrets-manifest.md per credential.)
[ ] Within 30 min: revoke the leaked credential at the provider (Stripe, Cloudflare, etc.).
[ ] Within 1h: search GitHub + Wayback + any mirrors for the leaked value.
[ ] Within 4h: review logs for any unauthorized use of the credential.
[ ] If unauthorized use found: this becomes a data-breach incident — escalate accordingly.
```

### Sev1: Audit-log hash chain mismatch

```
[ ] Within 5 min: freeze writes to the audit-log store (set the api into degraded mode that buffers writes locally).
[ ] Within 15 min: page CISO + external counsel.
[ ] Within 30 min: snapshot current state for forensic analysis.
[ ] Within 1h: determine whether the mismatch is corruption or tampering.
[ ] If tampering: this is a data-breach incident; full LGPD/GDPR process applies.
```

### Sev1: Mac mini gateway physical access (theft, tampering)

```
[ ] Within 15 min: revoke gateway SSH access from all workers (set workers' authorized_keys to empty).
[ ] Within 30 min: rotate all gateway-resident credentials (tunnel creds, restic password, ed25519 keys).
[ ] Within 1h: confirm FileVault was enabled; if it was, customer data exposure risk is low.
[ ] Within 4h: bring up a replacement gateway from backups (see ../../infra/disaster-recovery.md).
[ ] File a police report per Brazilian procedure.
```

## Cross-references

- [`../policies/incident-response-policy.md`](../policies/incident-response-policy.md) — policy framing
- [`../policies/access-control-policy.md`](../policies/access-control-policy.md) — break-glass procedure
- [`../policies/data-retention-policy.md`](../policies/data-retention-policy.md) — audit-log integrity
- [`on-call-runbook.md`](./on-call-runbook.md) — rotation and escalation
- [`../../infra/secrets-manifest.md`](../../infra/secrets-manifest.md) — credential rotation
- [`../../infra/disaster-recovery.md`](../../infra/disaster-recovery.md) — restore procedures

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
