# Compliance

> **Audience: operators and contributors.** Pointer document. The authoritative artifacts (policies, runbooks, control evidence) live under [`../compliance/`](../compliance/). This file orients you to what's where and what we owe whom.

## Regulatory posture

| Framework | Status | Why it applies |
|---|---|---|
| **LGPD** (Lei Geral de Proteção de Dados, Brazil 13.709/2018) | **In scope from day one** | Owera Software Ltda is a Brazilian controller. LGPD applies to any data on Brazilian residents regardless of where it's processed. |
| **SOC 2 Type 1** | **Targeting 12 months post-GA** | US enterprise customers will require it. Evidence built progressively; control mapping under [`../compliance/audit-controls/`](../compliance/). |
| **SOC 2 Type 2** | **Targeting 24 months post-GA** | Follow-up after Type 1. Requires 12 months of operational evidence. |
| **GDPR** | **Triggered on first EU customer** | We will sign DPAs and apply SCCs (Standard Contractual Clauses) for the EU↔BR data transfer. No EU-resident data is processed until the first EU customer is onboarded. |
| **HIPAA / PCI** | **Out of scope** | Not pursued until V4+ at the earliest. Healthcare and PCI workloads are not supported in any SKU. |

## Data residency

- Customer plane (`api/`, `web/`): hosted on Fly.io `gru` (São Paulo) primary; Vercel multi-region `gru1` + `iad1` for the dashboard.
- Operator plane (`owera-fleet`): physically in Macapá, Brazil. Worker Macs and gateway Mac mini are on-premises.
- Customer data at rest: encrypted on Fly.io Postgres (gru) and on the operator plane's encrypted APFS volumes.
- Stripe: US-based. Tenant metadata (name, email, billing address, tenant ID) is replicated to Stripe; jobs, inputs, and outputs are not.
- Backups: encrypted via restic to an off-site SFTP target. Bit-identical restore tested at provisioning time.

**For EU customers:** "Data is processed in Brazil under SCCs. We sign a DPA on request." If residency is a hard blocker, we evaluate a cloud-Mac sidecar in an EU region — defer until demand justifies the spend.

## Customer rights (LGPD / GDPR alignment)

| Right | How to exercise | Response SLA |
|---|---|---|
| **Access** — get a copy of your data. | Email `privacy@owera.com` or use the dashboard's **Settings → Privacy → Export**. | 15 days. |
| **Rectification** — correct inaccurate data. | Dashboard for tenant metadata; email for anything else. | 5 business days. |
| **Erasure / right-to-be-forgotten** — delete your data. | Email `privacy@owera.com`. Confirms via signed link; we then purge tenant data within 30 days and emit a deletion certificate. | 30 days to purge; 5 business days to acknowledge. |
| **Portability** — receive your data in a structured format. | Same as Access; export is JSON Lines, schema documented in `api/openapi.yaml`. | 15 days. |
| **Objection** — opt out of processing. | Email `privacy@owera.com`. Effectively equivalent to canceling the subscription. | 5 business days. |

Deletion certificate format and audit-log retention are documented in [`../compliance/policies/`](../compliance/).

## Where the artifacts live

| Artifact | Location |
|---|---|
| Privacy policy (public) | `https://owera.com/privacy` |
| Terms of service (public) | `https://owera.com/terms` |
| DPA template (provided on request) | `compliance/policies/dpa-template.md` |
| Security policy | `compliance/policies/security-policy.md` |
| Incident response policy | `compliance/policies/incident-response.md` |
| Data retention policy | `compliance/policies/data-retention.md` |
| Access control policy | `compliance/policies/access-control.md` |
| Incident runbooks | `compliance/runbooks/` |
| SOC 2 control-to-evidence mapping | `compliance/audit-controls/` |
| Vulnerability disclosure | [`../SECURITY.md`](../SECURITY.md) |

The exact filenames under `compliance/` are owned by the compliance agent; check [`../compliance/`](../compliance/) for the current layout.

## What this repo owes the auditor

For SOC 2 Type 1 readiness, we owe the auditor evidence on:

| Common Criteria | Evidence sources in this repo |
|---|---|
| CC1 — Control environment | `CONTRIBUTING.md`, `compliance/policies/*` |
| CC2 — Communication & info | `CHANGELOG.md`, `SECURITY.md`, status page archive |
| CC3 — Risk assessment | `compliance/policies/risk-assessment.md`, threat model in `compliance/runbooks/` |
| CC4 — Monitoring | `api/internal/audit/`, JSONL audit log, `compliance/runbooks/audit-log-review.md` |
| CC5 — Control activities | `.github/workflows/*` (CI gates), branch protection, CODEOWNERS |
| CC6 — Logical & physical access | `compliance/policies/access-control.md`, Cloudflare Zero Trust config |
| CC7 — System operations | `docs/runbook-deploy.md`, `compliance/runbooks/*`, change-management via PR |
| CC8 — Change management | PR template, CODEOWNERS, CI gates, `CHANGELOG.md` |
| CC9 — Risk mitigation | `compliance/policies/incident-response.md`, post-mortem cadence |

The auditor reads these directly from the public repo — that's intentional. Public artifacts are a trust signal; sensitive specifics (PagerDuty schedules, named individuals, customer IDs) live outside git in 1Password and cloud secret managers.
