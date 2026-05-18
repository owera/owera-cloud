# audit-controls/

The evidence layer. Maps compliance frameworks → controls → evidence locations the auditor can pull. Machine-readable so we can lint coverage during CI and surface gaps in the next compliance review.

## Files

| File | Framework | Purpose |
|------|-----------|---------|
| [`soc2-cc.yaml`](./soc2-cc.yaml) | SOC 2 Common Criteria (CC1.1 – CC9.2) | One entry per Common Criterion with description, evidence type, evidence location, current status. Machine-readable; the source of truth. |
| [`tsc-security.md`](./tsc-security.md) | SOC 2 Trust Service Category — Security | Auditor-facing view of CC1–CC9 grouped by category, with Owera control + concrete evidence path per row. |
| [`tsc-availability.md`](./tsc-availability.md) | SOC 2 Trust Service Category — Availability | A-series criteria (uptime, capacity, DR). In-scope because SLA is part of the product. |
| [`tsc-confidentiality.md`](./tsc-confidentiality.md) | SOC 2 Trust Service Category — Confidentiality | C-series criteria. In-scope for customer payloads and secrets. |
| [`tsc-processing-integrity.md`](./tsc-processing-integrity.md) | SOC 2 Trust Service Category — Processing Integrity | PI-series criteria. In-scope because of the signed ledger, audit log, and per-tenant cost cap. |
| [`tsc-privacy.md`](./tsc-privacy.md) | SOC 2 Trust Service Category — Privacy | P-series criteria. LGPD/GDPR overlap; in-scope because Owera handles personal data. |
| [`evidence-collection-runbook.md`](./evidence-collection-runbook.md) | All TSCs | Step-by-step procedures the auditor (or the on-call CSE escorting the auditor) follows to actually pull each evidence type at audit time. |
| [`known-gaps.md`](./known-gaps.md) | All TSCs | Consolidated list of every control whose status is `not-started` or `in-progress`. Each row becomes a Wave 11+ engineering ticket. |
| [`evidence/`](./evidence/) | All TSCs | One file per Common Criterion. The auditor reads these on the audit pull; each one names the evidence source and the pre-audit screenshot checklist. |

LGPD and GDPR mapping files are deferred until SOC 2 Type 1 readiness — the Common Criteria framework covers ~80% of the LGPD operational controls, and the residual gap is small enough to hold in the policy documents themselves until we engage a dedicated DPO. When we onboard the first EU customer, a `gdpr-art.yaml` joins this directory. Privacy-specific controls that overlap LGPD/GDPR are tracked today in [`tsc-privacy.md`](./tsc-privacy.md).

## How an auditor reads this directory

1. Start at this README to understand the layout.
2. Open the TSC doc for the category in scope (Security is mandatory; the others are in scope per Owera's system description).
3. For each row, read the **Evidence path** column — that is either a markdown file under [`evidence/`](./evidence/) or a direct path under `compliance/policies/`, `compliance/runbooks/`, `api/internal/`, `infra/`, etc.
4. When the evidence path is an `evidence/CCN.M-*.md` file, that file restates the control and names the live system to inspect — typically a log query, a console screenshot, or a 1Password vault path.
5. Follow [`evidence-collection-runbook.md`](./evidence-collection-runbook.md) to actually pull the artifact at audit time (queries, CLI commands, dashboard URLs).
6. Cross-check [`known-gaps.md`](./known-gaps.md) to see which controls Owera has marked `not-started` or `in-progress` with a remediation owner — these become the "noted in management response" items, not blockers.

## Trust Service Categories — scope decision

| TSC | In scope for V0 SOC 2 Type 1? | Why |
|-----|-------------------------------|-----|
| **Security (CC1–CC9)** | Yes — mandatory | Common Criteria are required for every SOC 2 report. |
| **Availability** | Yes | Support-SLA is a contractual commitment per [`compliance/runbooks/support-sla.md`](../runbooks/support-sla.md); availability claims must be auditable. |
| **Confidentiality** | Yes | Customer payloads + Stripe-derived billing data + Clerk-derived identity material flow through our system; confidentiality is implicit in the marketing claim. |
| **Processing Integrity** | Yes | The signed ledger ([`owera-fleet/internal/ledger/`](../../../owera-fleet/internal/ledger/)) and WORM audit log ([`api/internal/audit/`](../../api/internal/audit/)) only have value if Processing Integrity is attested. |
| **Privacy** | Yes | LGPD applies day-one (Owera Software Ltda is a Brazilian controller); SOC 2 Privacy provides the framework an enterprise customer expects alongside the LGPD/GDPR claim. |

## Status values

Each control entry in [`soc2-cc.yaml`](./soc2-cc.yaml) has a `status` field. The TSC docs surface the same values in their **Status** column:

| Status | Meaning |
|--------|---------|
| `not-started` / `TBD` | The control is documented in policy but no operational evidence exists yet. |
| `in-progress` / `documented-only` | Implementation is partial; some evidence collectible, gaps known. |
| `ready` / `implemented` | Implementation complete; auditor can pull evidence and we expect it to clear. |

For readability across audiences, the TSC docs use the auditor-vocabulary triple (`implemented` / `documented-only` / `TBD`); the YAML keeps the engineering-vocabulary triple (`ready` / `in-progress` / `not-started`). They map 1:1.

## Evidence types

| `evidence_type` | What the auditor expects |
|-----------------|---------------------------|
| `policy` | A markdown policy document under [`../policies/`](../policies/) |
| `runbook` | A markdown runbook under [`../runbooks/`](../runbooks/) |
| `config` | A version-controlled config file (typically under [`../../infra/`](../../infra/) or [`../../tunnel/`](../../tunnel/)) |
| `log-query` | A specific query against an operational log store (path / API / query string) |
| `screenshot` | A point-in-time visual proof (dashboard state, console output) |
| `attestation` | A signed document from a third party (vendor SOC 2, employee training certificate) |
| `code` | A path in the codebase that implements the control |

## Reconciliation cadence

- **Pre-audit (one-time at SOC 2 onboarding)**: every `not-started` becomes `in-progress` or `ready`. Owners listed in [`known-gaps.md`](./known-gaps.md) drive closure.
- **Quarterly**: CISO reviews status drift — any control that regressed from `ready` to `in-progress` is a finding.
- **Post-incident**: any control implicated in an incident is re-validated, and its evidence file gets a new "Version history" row.

## How to add a control

PR `soc2-cc.yaml`:

```yaml
- id: CCN.M
  description: <one-line auditor-facing description>
  evidence_type: policy|runbook|config|log-query|screenshot|attestation|code
  evidence_location: <relative path, log query, or URL>
  evidence_path: compliance/audit-controls/evidence/CCN.M-<slug>.md
  status: not-started|in-progress|ready
  owner: <role>
  notes: <optional clarifications>
```

The file is YAML-validated in CI; an unparseable entry blocks the merge. After the YAML row lands, add a corresponding row to the appropriate TSC doc (Security / Availability / Confidentiality / Processing Integrity / Privacy) so the auditor-facing view stays in sync.

## evidence_path contract

Every control entry has `evidence_path` pointing at a file under
[`evidence/`](./evidence/) that the auditor reads on the audit pull. The file:

- Restates the control description (so an auditor reading the evidence
  file in isolation knows what they're looking at).
- Names the evidence source (the live system, dashboard, log query, or
  out-of-tree document).
- Captures the TODO list before the audit window opens — typically
  "screenshot the SaaS console N days before the auditor's read date,
  check it into `<cc-id>-screenshots/`".
- Is the place to land long-form follow-up notes that don't belong in
  the YAML.

CI lints that every `evidence_path` value resolves to a file at HEAD.
The lint runs in `.github/workflows/compliance.yml` (TODO: wire) using:

```bash
yq '.controls[].evidence_path' compliance/audit-controls/soc2-cc.yaml \
  | xargs -I{} test -f {} || (echo "missing evidence file: {}" && exit 1)
```
