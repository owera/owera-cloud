# Known gaps

> **Purpose.** Consolidated list of every control whose status is `not-started` (TBD) or `in-progress` (documented-only). Each row becomes a Wave 11+ engineering ticket. The CSE reconciles this list quarterly; the SOC 2 audit window cannot open until every row here is either closed or has a documented management response.

> **How the list is built.** Every row below mirrors a row in [`soc2-cc.yaml`](./soc2-cc.yaml) whose `status` is not `ready`. The status mapping is:
>
> | YAML `status` | Auditor vocabulary | Treatment |
> |---------------|--------------------|-----------|
> | `not-started` | `TBD` | Must become `in-progress` or `ready` before audit-window open. |
> | `in-progress` | `documented-only` | Acceptable for Type 1 if remediation owner + target date are documented in the management response. |
> | `ready` | `implemented` | Excluded from this list. |

> **Target closure date.** SOC 2 Type 1 audit window opens approximately **Q2 2027** (12 months from V1 GA, per the master plan). All `TBD` items must close at least 60 days before that. All `documented-only` items either close or have an explicit management response.

## TBD (highest priority — currently `not-started`)

| CC / TSC | Description | Owner | Target | Notes |
|----------|-------------|-------|--------|-------|
| CC1.2 | Board independence and oversight (board minutes). | CEO | Q4 2026 | Pre-GA: founders + advisors only. Formalize before SOC 2 audit window — first quarterly minutes Q4 2026. |
| CC3.2 | Annual risk assessment doc. | CISO | Q3 2026 | First formal risk assessment due Q3 2026 ahead of SOC 2 onboarding. Lives at 1Password `Owera/Compliance/risk-assessment-2026.md`. |
| CC3.3 | Fraud risk evaluation. | CISO | Q3 2026 | Sub-section of the CC3.2 risk assessment. |
| A1.3 | DR-drill execution evidence. | SRE | Q3 2026 | [`infra/disaster-recovery.md`](../../infra/disaster-recovery.md) lists every restore target as `[ ] Q3/Q4 2026 (target)`. First drill produces a check-in under `compliance/audit-controls/evidence/A1.3-drill-screenshots/`. |
| P6.2 | Vendor DPA + SOC 2 collection. | CISO | Q3 2026 | [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md) has `[ ] TODO` for every critical/important vendor (Stripe, Cloudflare, Fly, Vercel, 1Password, GitHub, Google, Slack, PagerDuty, Sentry, Anthropic). Collect each one's SOC 2 + sign each DPA. |

## documented-only (medium priority — currently `in-progress`)

### Security

| CC | Description | Owner | Remediation | Target |
|----|-------------|-------|-------------|--------|
| CC1.4 | Hiring and training competence. | CEO | Onboarding checklist + per-employee training-completion attestations exported from HR SaaS. | Q3 2026 |
| CC1.5 | Accountability for internal-control responsibilities. | CISO | Disciplinary policy is referenced but not yet linked to a specific signed acceptable-use acknowledgement per employee. | Q4 2026 |
| CC2.1 | Audit-log production wiring. | CSE | `MockWORMStreamer` exercised in tests; `S3WORMStreamer` exists but production-tenant rollout is pending. | Q4 2026 (post-V1 GA) |
| CC4.1 | Quarterly access reviews. | CISO | Policy exists; first quarterly review artifact will land under `compliance/audit-controls/reviews/`. | Q3 2026 (first review) |
| CC6.2 | New-user registration. | IDE | Clerk-driven; need to attach an onboarding-checklist excerpt to each Clerk org-creation event. | Q4 2026 |
| CC6.3 | Role-based authorization (audit-log query). | IDE / CSE | The `access-grant` / `access-revoke` audit actions are emitted by code; need an admin-tool to bulk-query for a quarterly review. | Q4 2026 |
| CC6.4 | Physical access (gateway in home office today). | PE | Photos exist in 1Password; consider co-location post-Series-A. Document compensating controls in the meantime. | Post-Series-A |
| CC6.8 | Malware defense (third-party Mac software). | SRE / CSE | gitleaks + CodeQL run in CI; need an explicit "approved software" list for the gateway Mac. | Q4 2026 |
| CC7.2 | Anomaly monitoring (operator-plane heartbeat). | SRE / PE | Heartbeat watchdog runs; ntfy alerts work. Add a 90-day rolling alert log so the auditor can see "this is what we caught and how fast." | Q3 2026 |

### Availability

| Control | Description | Owner | Remediation | Target |
|---------|-------------|-------|-------------|--------|
| A1.1 | Capacity-management quarterly review. | SRE | First quarterly capacity-review artifact under `compliance/audit-controls/reviews/`. | Q3 2026 |
| A1.6 | SLA roll-up automation. | CISO | `scripts/support-sla-rollup.sh` per [`compliance/runbooks/support-sla.md`](../runbooks/support-sla.md) — author + install as a gateway LaunchAgent. | Q4 2026 |

### Confidentiality

| Control | Description | Owner | Remediation | Target |
|---------|-------------|-------|-------------|--------|
| C1.7 | Self-serve customer-data export endpoint. | CSE | Today the runbook says "the SRE-on-duty exports each tenant's data manually." Build the endpoint; expose under `DELETE /v1/tenants/me/data`'s sibling `GET /v1/tenants/me/data-export`. | Q4 2026 |

### Processing Integrity

| Control | Description | Owner | Remediation | Target |
|---------|-------------|-------|-------------|--------|
| PI1.5 | Production WORM bucket wiring. | CSE | `MockWORMStreamer` covers tests; switch dev → S3 with Object Lock in Governance mode; verify retention header set per PUT. | Q4 2026 |
| PI1.7 | End-to-end charge lineage doc. | CSE | Author "follow one charge from API call → audit row → ledger entry → Stripe usage record → invoice line item" as an auditor-facing trace doc. | Q1 2027 |

### Privacy

| Control | Description | Owner | Remediation | Target |
|---------|-------------|-------|-------------|--------|
| P1.1 | Bilingual published privacy notice. | CISO | [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md) exists; publish at `owera.ai/privacy` in English + Portuguese. | GA gate (V1) |
| P3.1 | Purpose-limitation review cadence. | CISO | LGPD policy enumerates purposes; the auditor will want a quarterly review confirming actual processing matched stated purposes. | Q3 2026 |
| P4.1 | Personal-data access reviews. | CSE / IDE | Same quarterly access-review cadence as CC4.1; need a check-mark per tenant in the review artifact. | Q3 2026 |
| P5.1 | Self-serve data export. | CSE | Same as C1.7. | Q4 2026 |
| P6.1 | Sub-processor list disclosure. | CISO | List is in [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md); needs a public version on `owera.ai/sub-processors`. | GA gate (V1) |
| P7.1 | Data-quality assurances. | CSE | Account data is Clerk-managed; document the no-enrichment posture explicitly. | Q3 2026 |
| P8.1 | DPO designation + complaint-handling SLA. | CEO | [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md) must name the DPO explicitly; today Rodrigo is interim DPO. Formalize before audit window. | Q4 2026 |

## Aggregate counts

| Category | TBD | documented-only | Total open |
|----------|-----|------------------|-------------|
| Security (CC1–CC9) | 3 | 9 | 12 |
| Availability | 1 | 2 | 3 |
| Confidentiality | 0 | 1 | 1 |
| Processing Integrity | 0 | 2 | 2 |
| Privacy | 1 | 6 | 7 |
| **Totals** | **5** | **20** | **25** |

The Security-row counts include the rows already tracked in [`soc2-cc.yaml`](./soc2-cc.yaml) (coverage: 21 ready / 9 in-progress / 3 not-started — three of which double-count under Privacy and Availability because those TSCs invoke the same CC). The TSC-specific overlay above is what gets surfaced in the audit response.

## Pre-audit close-out workflow

For each row in this file:

1. CSE confirms remediation is complete in code or in process.
2. CSE moves the YAML `status` to `ready` and the TSC-doc **Status** column to `implemented`.
3. CSE removes the row from this file in the same PR.
4. PR review: CISO sign-off required.

When this file has zero rows, SOC 2 Type 1 audit window can open.

## Refresh cadence

- **Quarterly** — CISO reviews the list; any row whose target date has slipped is escalated to the CEO.
- **Per-PR** — when a control flips to `ready`, the row leaves this file in the same PR.
- **Post-incident** — any incident that exercised a `documented-only` control triggers re-assessment (might flip back to `not-started` if the incident exposed a gap).
