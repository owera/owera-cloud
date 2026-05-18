# Trust Service Category — Processing Integrity

> **Scope.** Processing Integrity addresses whether system processing is complete, valid, accurate, timely, and authorized. Owera elects this TSC because the system makes specific processing-integrity claims to customers:
> - Every state-changing API call is captured in a tamper-evident audit log.
> - Every operator-plane job-step is captured in a signed JSONL ledger.
> - Billing is sourced from the audit log + signed ledger, not from in-memory counters.
> - Per-tenant cost caps are enforced before work begins; 402 + Retry-After is the contract when a cap is hit.

## Control mapping

| PI # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| PI1.1 | Defines processing requirements (inputs, processing rules, outputs). | OpenAPI / JSON-RPC contracts in `api/` + catalog-one-PR contract enforces per-SKU spec under `docs/`. | [`api/internal/server/`](../../api/internal/server/), `docs/pricing.md`, [`.github/workflows/catalog-one-pr-contract.yml`](../../.github/workflows/catalog-one-pr-contract.yml) | SRE / CSE | implemented |
| PI1.2 | Implements input controls (authentication, validation). | Clerk JWT verification on dashboard ingress; argon2id-verified API keys on programmatic ingress; per-tenant rate limits; per-tenant cost cap pre-check. | [`api/internal/auth/clerk.go`](../../api/internal/auth/clerk.go), [`api/internal/identity/identity.go`](../../api/internal/identity/identity.go), [`api/internal/billing/costcap.go`](../../api/internal/billing/costcap.go) → cross-link [`evidence/CC6.1-logical-access.md`](./evidence/CC6.1-logical-access.md) | IDE / CSE | implemented |
| PI1.3 | Implements processing controls (integrity of in-flight processing). | Job dispatcher writes audit row on accept, on dispatch, and on terminal state; ledger captures every job step with a minisign signature + prev-hash chain. | [`api/internal/dispatcher/`](../../api/internal/dispatcher/), [`api/internal/jobs/`](../../api/internal/jobs/), [`api/internal/audit/audit.go`](../../api/internal/audit/audit.go), [`owera-fleet/internal/ledger/ledger.go`](../../../owera-fleet/internal/ledger/ledger.go) → [`evidence/CC2.1-audit-log.md`](./evidence/CC2.1-audit-log.md) | CSE / SRE | implemented |
| PI1.4 | Implements output controls (completeness, accuracy of results). | Job result envelope is hash-stamped; the same hash is captured in the audit log and in the per-task ledger entry, so the result the customer received can be cross-checked. | [`api/internal/jobs/`](../../api/internal/jobs/), [`owera-fleet/internal/ledger/ledger.go`](../../../owera-fleet/internal/ledger/ledger.go) → [`evidence/CC2.1-audit-log.md`](./evidence/CC2.1-audit-log.md) | CSE / SRE | implemented |
| PI1.5 | Stores inputs/outputs/state with appropriate integrity (tamper detection). | WORM via S3 Object Lock in Governance mode for the audit log cold tier; SQLite triggers prevent in-application overwrite of an audit row; minisign signature + previous-hash chain detects ledger tampering. | [`api/internal/audit/audit.go`](../../api/internal/audit/audit.go) (header comment §"Cold: S3 with Object Lock"), [`api/internal/audit/worm.go`](../../api/internal/audit/worm.go) (`ErrWORMLocked` semantics), [`owera-fleet/internal/ledger/ledger.go`](../../../owera-fleet/internal/ledger/ledger.go) (Ed25519 signing key on disk per task) | CSE / PE | implemented |
| PI1.6 (Owera-defined) | Per-tenant cost caps prevent over-spend regardless of usage pattern. | `costcap.Check()` runs before work is dispatched; cap exceeded → returns a typed error → HTTP middleware translates to 402 with `Retry-After: <seconds until UTC month roll-over>`. | [`api/internal/billing/costcap.go`](../../api/internal/billing/costcap.go) (see L66–L103 cost-cap-exceeds-returns-402 contract), [`api/internal/billing/billing_test.go`](../../api/internal/billing/billing_test.go) `TestCostCap_Exceeds_Returns402` | CSE | implemented |
| PI1.7 (Owera-defined) | Billing is derived from the audit log + signed ledger; no in-memory counters trusted for invoicing. | Stripe usage records are submitted only after the ledger entry for the chargeable event lands and is signed. | [`api/internal/billing/`](../../api/internal/billing/), [`api/internal/audit/audit.go`](../../api/internal/audit/audit.go) | CSE | documented-only |

## Common-Criteria controls invoked by Processing Integrity

| CC # | Description | Why it matters for Processing Integrity |
|------|-------------|-----------------------------------------|
| CC2.1 | Generates quality information. | The WORM audit log + signed ledger are the canonical sources of processing evidence. |
| CC7.1 | Detects config changes. | Audit log + git history together cover "did anyone change processing rules between input and output?" |
| CC7.2 | Anomaly monitoring. | Sentry on the API + heartbeat watchdog on the operator plane detect processing anomalies. |
| CC8.1 | Change management. | Processing rules are change-managed via PR → CI gates → deploy. |

## Known gaps (summary)

See [`known-gaps.md`](./known-gaps.md). Processing-Integrity-specific gaps:

- **PI1.5 production WORM wiring.** Today [`api/internal/audit/worm.go`](../../api/internal/audit/worm.go) exposes both `MockWORMStreamer` (in-process) and `S3WORMStreamer` (production). The auditor will want a sample request → S3 Object Lock retention header → bucket-policy denial of overwrite on a test key. The bucket exists in dev; production-tenant rollout is pending. Owner: CSE.
- **PI1.7 ledger → Stripe usage-record lineage.** The lineage is implemented in code but the auditor-facing trace doc (a "follow one charge from API call → audit row → ledger entry → Stripe usage record → invoice line item") does not yet exist. Owner: CSE.

## Cross-references

- **Security** — see [`tsc-security.md`](./tsc-security.md) for CC1–CC9.
- **Confidentiality** — see [`tsc-confidentiality.md`](./tsc-confidentiality.md) for the encryption-at-rest controls that protect the ledger / audit log artifacts.
- **Evidence collection** — see [`evidence-collection-runbook.md`](./evidence-collection-runbook.md) for the SQLite queries to dump the audit log and the `ledger` CLI invocation to verify a task's chain.
