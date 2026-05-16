# Incident response policy

**Scope:** any unplanned event that degrades availability, integrity, or confidentiality of the Owera Agentic platform, or that reasonably could.

**Operational runbook:** [`../runbooks/incident-response.md`](../runbooks/incident-response.md). This document is the policy framing; the runbook is the step-by-step under pressure.

## 1. Severity classification

| Severity | Definition | Examples | Initial response SLA |
|----------|------------|----------|----------------------|
| **Sev1** | Customer-facing outage, data integrity loss, security breach, or LGPD/GDPR-reportable event | api.owera.ai down >5min; customer data exfiltration suspected; tunnel credentials leaked | 15 min, 24×7 |
| **Sev2** | Significant degradation, partial outage, or near-miss security event | Operator plane down (cloud plane still serving cached responses); break-glass without approval; high-severity dependency CVE | 1 h business, 2 h after-hours |
| **Sev3** | Localized issue with workaround, non-customer-facing | One worker unhealthy; staging broken; non-urgent CVE | Next business day |

Severity is set by the **incident commander** within 15 min of declaration and can be re-leveled as facts develop. Re-leveling is logged.

## 2. Declaration

Any Owera employee or contractor can declare an incident. Declaration:

1. Post `#incident-declare` in Slack with: brief description, suspected severity, affected systems.
2. PagerDuty triggers the incident commander rotation.
3. Within 15 min, the IC starts the incident timeline in `#incident-<n>` (auto-created channel).

External notifications (customer-affecting Sev1/Sev2): the IC delegates to comms lead and customer success per the runbook.

## 3. Roles during an incident

| Role | Responsibility | Filled by |
|------|----------------|-----------|
| Incident commander (IC) | Owns decisions, runs the timeline, calls re-levels | Primary on-call SRE; rotates |
| Technical lead | Diagnoses + drives mitigation | Engineer with the deepest context on the affected system |
| Comms lead | External customer comms, status page, regulator notifications | Customer success or CEO depending on scope |
| Scribe | Maintains the incident channel timeline | Any available engineer not in IC/TL/comms |

For LGPD/GDPR-reportable events, the CISO joins as **data protection lead** within the first hour.

## 4. Post-mortem requirement

Every Sev1 and every Sev2 with customer impact gets a **blameless post-mortem** within 5 business days. Required sections:

1. Summary (one paragraph; the elevator pitch).
2. Timeline (UTC; every meaningful event with attribution).
3. What went wrong (technical root cause; no individual blame).
4. What went well (genuinely — what saved us time).
5. Action items (each with owner, deadline, tracking issue).
6. Lessons (knowledge to fold into runbooks / training).

Post-mortems are stored in `/Users/claw3/owera-cloud/compliance/runbooks/post-mortems/<YYYY-MM-DD>-<short-slug>.md` (directory created on first incident) and reviewed in the next monthly engineering review.

## 5. Communications

| Audience | Channel | When | Owner |
|----------|---------|------|-------|
| Internal team | Slack `#incident-<n>` | Continuously throughout the incident | Scribe |
| Customers (live status) | status.owera.ai | Within 30 min of Sev1 declaration; updated every 30 min | Comms lead |
| Affected customers (email) | SES, template `incident-update` | Within 1h of Sev1 if customer data potentially exposed | Customer success |
| Regulators (LGPD) | ANPD via the official portal | Within 72h of confirming a reportable incident (treating "reasonable time" as ≤72h, mirroring GDPR Art. 33) | CISO |
| Regulators (GDPR, if applicable) | Relevant lead DPA | Within 72h | CISO + DPO |
| Stripe | Stripe dashboard incident form | If payments are affected, within 24h | CFO |

Customer-affecting communication MUST be approved by the IC before posting. Regulator notifications MUST be approved by the CISO and reviewed by Owera Software Ltda's external counsel.

## 6. Evidence preservation

For security-class incidents, preserve evidence before remediation:

- Snapshot affected logs (read-only copy to an isolated bucket).
- Snapshot the audit-log WORM store as of declaration time.
- Preserve any compromised credentials in a sealed envelope (do NOT delete; rotate but retain the old value for forensics, in 1Password under a `compromised-credentials/` folder with retention 1 year).

## 7. Tabletop exercises

The incident response procedure is rehearsed twice per year (Q2, Q4) via tabletop exercises. Tabletops use fictional scenarios drawn from a rotating list:

- Production credential leak via accidental git push
- Stripe webhook secret exfiltration
- Customer reporting unauthorized agent action
- ANPD reporter requests evidence of a past event
- Mac mini gateway physical theft

Tabletop outcomes drive policy + runbook updates.

## Ownership

| Role | Responsibility |
|------|----------------|
| CISO | Accountable for the policy; chairs tabletops |
| SRE Lead | Maintains the on-call rotation and pager hygiene |
| CEO | Public communications on Sev1 incidents |

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
