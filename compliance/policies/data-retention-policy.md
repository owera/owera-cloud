# Data retention policy

**Scope:** every category of data Owera collects, processes, or stores on behalf of customers, personnel, or system operations.

## 1. Categories and retention

| Category | Retention | Storage | Notes |
|----------|-----------|---------|-------|
| **Customer account metadata** (email, name, billing address, tenant id) | Lifetime of account + 90 days | Postgres via Clerk/WorkOS, mirrored to api SQLite cache | Hard-deleted on account closure + 90d grace |
| **Customer payloads** (agent job inputs and outputs) | 30 days from job completion, OR until tenant-configured TTL (whichever is shorter) | Operator plane gateway filesystem (encrypted) | Tenant can request immediate deletion; honored within 15 working days (LGPD Art. 18 §VI) |
| **Customer billing records** (invoices, transactions) | 5 years | Stripe + api SQLite cache | Brazilian fiscal law (Receita Federal) requires 5y minimum |
| **Audit logs** (auth events, privilege escalations, payload access) | 7 years | api WORM store (immutable, hash-chained) | SOC 2 + LGPD evidentiary need; never deletable per-tenant request |
| **Application logs** (api / web / operator plane operational logs) | 30 days hot, 365 days cold | Fly / Vercel / operator-plane SFTP backup | Scrubbed of PII at ingest where feasible |
| **Backup snapshots** (operator plane state via restic) | 30 daily, 12 monthly, 7 annual | restic SFTP to salmonpoke | Honors GDPR/LGPD right-to-erasure within the 30-day window |
| **Incident records** (timelines, post-mortems) | 7 years | `compliance/runbooks/post-mortems/` in git | Retained for SOC 2 evidence |
| **HR records** (offboarding, training attestations) | 5 years post-departure | HR SaaS | Brazilian labor law requirement |
| **Marketing / consent records** | Until consent withdrawn, then 90 days for proof-of-withdrawal | CRM | LGPD Art. 8 §5 (consent revocability) |

## 2. Deletion (LGPD Art. 18 §VI — right to deletion)

Operational procedure: [`../runbooks/customer-data-deletion.md`](../runbooks/customer-data-deletion.md). Programmatic surface: `DELETE /v1/tenants/me/data` (see `api/internal/erasure/`); self-service in dashboard piggybacks the same endpoint.

A customer's right to deletion is honored within **15 working days** of a verified request — the LGPD Art. 18 §V / Art. 19 §1 ceiling, stricter than GDPR Art. 12's 30 calendar days; one SLA satisfies both regimes. The audit-log WORM store is **exempt** from deletion — audit logs containing references to the deleted tenant remain, but PII fields within audit records are tokenized (the original PII is destroyed; the tokens remain for hash-chain integrity). LGPD Art. 16 explicitly preserves this carve-out for legal-obligation retention.

Deletion is **logged** to the audit-log WORM store as a non-deletable record showing: tenant id, request date, fields scrubbed, fields preserved-under-legal-basis, operator who executed.

## 3. Audit-log WORM specifics

The audit log is hash-chained: every entry includes the SHA-256 of the previous entry, so any tampering with history is detectable. Retention is 7 years. Storage is:

- Hot: api SQLite cache (sqlcipher encrypted at rest)
- Cold: monthly export to S3 (or equivalent) with object-lock in compliance mode

Restore of the audit log from cold storage is verifiable by hash-chain validation; the auditor is given a script that walks the chain end-to-end.

## 4. Treatment of minors (LGPD Art. 14)

Owera does not knowingly accept signups from individuals under 18 years old. The signup flow includes an age-gate (date of birth or attestation). LGPD Art. 14 §1 requires parental consent for children under 12; we set the bar higher (18) and decline rather than collect parental consent, because the product is a B2B developer tool.

If we later discover an account belongs to a minor:

1. Immediately suspend the account.
2. Notify the parent / guardian if discoverable.
3. Delete the account within 7 days unless verified parental consent is obtained and the use case is appropriate (e.g. educational institution context).
4. Log the event in the audit-log WORM store.

## 5. Backups vs. deletion

Backups age out per the schedule above. A customer deletion request **does not** trigger a forced re-keying of restic backups — the next ~30 days of daily snapshots will contain the customer's encrypted data, which then ages out naturally. The customer-facing 15-working-day API SLA covers the application-layer purge; the residual backup window is disclosed in the deletion-acknowledgement email and in the privacy notice.

For an "urgent erasure" request (e.g. court order), the runbook covers re-keying restic to force-evict the data within 7 days.

## 6. Export for portability (LGPD Art. 18 §V, §VIII; GDPR Art. 20)

Operational procedure: [`../runbooks/customer-data-export.md`](../runbooks/customer-data-export.md).

Exports are delivered in machine-readable JSON (one file per category) plus a human-readable PDF summary. Delivered within 15 days of request via a single-use signed download link (1Password-shared or email-delivered with the customer's primary authentication factor).

## Ownership

| Role | Responsibility |
|------|----------------|
| CISO | Accountable for the policy; signs off on legal-basis retention exemptions |
| SRE Lead | Operational enforcement of retention schedules (deletion crons, restic lifecycle) |
| CFO | Billing retention (Stripe + Receita Federal) |
| DPO (when appointed) | Customer-facing rights-request handling |

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
