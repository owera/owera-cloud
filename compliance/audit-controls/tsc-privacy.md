# Trust Service Category — Privacy

> **Scope.** Privacy covers personal information collected, used, retained, disclosed, and disposed of by Owera. The auditor will read management's system description for the privacy commitments. As of 2026-05-17 those are:
> - Owera Software Ltda is the **controller** for Brazilian data subjects under LGPD (Lei 13.709/2018); the DPO is named in [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md).
> - For EU data subjects (first EU customer onboarding), Owera is the **controller** under GDPR. The right-to-erasure pipeline serves both regimes from one code path.
> - Personal data classes: account holders (Clerk-managed), billing principals (Stripe-managed), job payloads that *may* contain personal data on Brazilian / EU residents (customer-managed; Owera treats as personal if the customer flags it as such in the workspace settings).

> **Cross-regime mapping.** The SOC 2 Privacy criteria (P1–P8) map almost 1:1 onto LGPD articles 6, 9, 18 and GDPR articles 5, 13, 15, 17, 32. Where a single control satisfies both regimes, the row below cites both; where they diverge, the row picks the stricter of the two (typically LGPD's 15-working-day erasure window).

## Control mapping

| P # | Description | Owera control | Evidence path | Owner workstream | Status |
|-----|-------------|---------------|---------------|-------------------|--------|
| P1.1 | Provides notice to data subjects about its privacy practices. | Public privacy policy published at `owera.ai/privacy` (English + Portuguese); LGPD-specific notice section. | [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md), `web/app/privacy/page.tsx` (web tier) | CISO | documented-only |
| P2.1 | Obtains consent when collecting personal information; respects withdrawal. | Clerk-managed sign-up captures consent at account creation; consent revocation triggers the right-to-erasure pipeline. | [`api/internal/auth/clerk.go`](../../api/internal/auth/clerk.go), [`api/internal/erasure/erasure.go`](../../api/internal/erasure/erasure.go) | IDE / CSE | implemented |
| P3.1 | Collects personal information consistent with the stated purpose. | LGPD policy enumerates purposes (service delivery, billing, security); the audit log captures every persistence event, enabling after-the-fact purpose-limitation review. | [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md), [`api/internal/audit/`](../../api/internal/audit/) | CISO | documented-only |
| P3.2 | Collects from authorized sources only. | All ingestion is via authenticated API calls (Clerk JWT or API key). No third-party ingestion paths. | [`api/internal/auth/`](../../api/internal/auth/), [`api/internal/identity/`](../../api/internal/identity/) | IDE | implemented |
| P4.1 | Uses personal information only for stated purposes. | Tenant-scoped SQL queries; access reviews per [`compliance/policies/access-control-policy.md`](../policies/access-control-policy.md) §5; audit-log review after each access. | [`compliance/policies/access-control-policy.md`](../policies/access-control-policy.md), [`api/internal/audit/`](../../api/internal/audit/) | CSE / IDE | documented-only |
| P4.2 | Retains personal information for no longer than necessary. | Data-retention policy with per-class windows; erasure pipeline + nightly purger sweep records past retention. | [`compliance/policies/data-retention-policy.md`](../policies/data-retention-policy.md), [`api/internal/erasure/purger.go`](../../api/internal/erasure/purger.go) | CSE | implemented |
| P4.3 | Disposes of personal information securely. | LGPD/GDPR right-to-erasure pipeline. SLA = 15 working days (LGPD Art. 18 §V — stricter than GDPR's 30 calendar days, so we honor LGPD). | [`api/internal/erasure/erasure.go`](../../api/internal/erasure/erasure.go) (see header comment §"LGPD Art. 18 §V; GDPR Art. 12"), [`api/internal/erasure/worker.go`](../../api/internal/erasure/worker.go), [`compliance/runbooks/customer-data-deletion.md`](../runbooks/customer-data-deletion.md) | CSE | implemented |
| P5.1 | Grants data subjects access to their personal information. | Customer-data export runbook; self-serve export endpoint is on the backlog. | [`compliance/runbooks/customer-data-export.md`](../runbooks/customer-data-export.md) | CSE | documented-only |
| P5.2 | Grants data subjects correction/deletion of their personal information. | Right-to-erasure pipeline (cited under P4.3); correction via dashboard account-settings flow (Clerk-driven). | [`api/internal/erasure/`](../../api/internal/erasure/), Clerk dashboard | IDE / CSE | implemented |
| P6.1 | Discloses personal information to third parties only with consent or per agreement. | Sub-processor list in vendor-management-policy; DPA + MSA negotiated per [`compliance/runbooks/onboarding-playbook.md`](../runbooks/onboarding-playbook.md). | [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md), [`compliance/runbooks/onboarding-playbook.md`](../runbooks/onboarding-playbook.md) | CISO | documented-only |
| P6.2 | Holds third parties to equivalent privacy commitments. | Vendor SOC 2 reports collected annually; DPA template references LGPD + GDPR. | [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md) | CISO | TBD |
| P7.1 | Maintains data quality (accuracy, completeness, currency). | Tenants manage their own account data via Clerk; Owera does not enrich. | [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md) | CSE | documented-only |
| P8.1 | Monitors compliance and addresses complaints. | DPO contact published in privacy policy; complaints flow into the incident-response runbook (Sev classification handles regulator notifications). | [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md), [`compliance/runbooks/incident-response.md`](../runbooks/incident-response.md) §6 | CISO | documented-only |

## Common-Criteria controls invoked by Privacy

| CC # | Description | Why it matters for Privacy |
|------|-------------|----------------------------|
| CC2.3 | External-party communications. | Regulator notifications (ANPD for LGPD, EU supervisory authorities for GDPR) flow through this runbook. |
| CC6.1 | Logical access controls. | Personal data access is gated by the same argon2id + Clerk controls that protect non-personal data. |
| CC6.5 | Asset disposal. | The right-to-erasure pipeline IS the asset-disposal control for personal data. |
| CC6.7 | Data movement. | Restricts personal data movement to authorized exports. |

## Known gaps (summary)

See [`known-gaps.md`](./known-gaps.md). Privacy-specific gaps:

- **P1.1 published privacy policy.** [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md) exists; the bilingual web-tier publication at `owera.ai/privacy` is on the GA gate checklist. Owner: CISO.
- **P5.1 self-serve export.** Same TODO surfaced under Confidentiality §C1.7 — the [`compliance/runbooks/customer-data-export.md`](../runbooks/customer-data-export.md) still says "the SRE-on-duty exports each tenant's data manually." Owner: CSE.
- **P6.2 vendor DPA + SOC 2 collection.** [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md) lists every critical/important vendor with `[ ] TODO` — collect each one's SOC 2 + sign each DPA before SOC 2 audit window. Owner: CISO.
- **DPO designation.** [`compliance/policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md) must name the Data Protection Officer explicitly. Today: Rodrigo (interim DPO). Formalize before audit window. Owner: CEO.

## Cross-references

- **Security** — see [`tsc-security.md`](./tsc-security.md) for CC1–CC9.
- **Confidentiality** — see [`tsc-confidentiality.md`](./tsc-confidentiality.md) for the encryption + access controls that protect personal data at rest and in transit.
- **Evidence collection** — see [`evidence-collection-runbook.md`](./evidence-collection-runbook.md) for the procedure to demonstrate an end-to-end erasure (request → audit row → purger run → row absence) at audit time.
