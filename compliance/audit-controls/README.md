# audit-controls/

The evidence layer. Maps compliance frameworks → controls → evidence locations the auditor can pull. Machine-readable so we can lint coverage during CI and surface gaps in the next compliance review.

## Files

| File | Framework | Purpose |
|------|-----------|---------|
| [`soc2-cc.yaml`](./soc2-cc.yaml) | SOC 2 Common Criteria (CC1.1 – CC9.2) | One entry per Common Criterion with description, evidence type, evidence location, current status |

LGPD and GDPR mapping files are deferred until SOC 2 Type 1 readiness — the Common Criteria framework covers ~80% of the LGPD operational controls, and the residual gap is small enough to hold in the policy documents themselves until we engage a dedicated DPO. When we onboard the first EU customer, a `gdpr-art.yaml` joins this directory.

## Status values

Each control entry has a `status` field:

| Status | Meaning |
|--------|---------|
| `not-started` | The control is documented in policy but no operational evidence exists yet |
| `in-progress` | Implementation is partial; some evidence collectible, gaps known |
| `ready` | Implementation complete; auditor can pull evidence and we expect it to clear |

## Evidence types

| `evidence_type` | What the auditor expects |
|-----------------|---------------------------|
| `policy` | A markdown policy document under `../policies/` |
| `runbook` | A markdown runbook under `../runbooks/` |
| `config` | A version-controlled config file (typically under `../../infra/` or `../../tunnel/`) |
| `log-query` | A specific query against an operational log store (path / API / query string) |
| `screenshot` | A point-in-time visual proof (dashboard state, console output) |
| `attestation` | A signed document from a third party (vendor SOC 2, employee training certificate) |
| `code` | A path in the codebase that implements the control |

## Reconciliation cadence

- **Pre-audit (one-time at SOC 2 onboarding)**: every `not-started` becomes `in-progress` or `ready`.
- **Quarterly**: CISO reviews status drift — any control that regressed from `ready` to `in-progress` is a finding.
- **Post-incident**: any control implicated in an incident is re-validated.

## How to add a control

PR `soc2-cc.yaml`:

```yaml
- id: CCN.M
  description: <one-line auditor-facing description>
  evidence_type: policy|runbook|config|log-query|screenshot|attestation|code
  evidence_location: <relative path, log query, or URL>
  status: not-started|in-progress|ready
  owner: <role>
  notes: <optional clarifications>
```

The file is YAML-validated in CI; an unparseable entry blocks the merge.
