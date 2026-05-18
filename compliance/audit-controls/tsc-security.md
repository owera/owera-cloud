# Trust Service Category — Security

> **Scope.** All nine Common Criteria sub-series (CC1 control environment through CC9 risk mitigation) apply to the SOC 2 Security category. Security is the only TSC that is mandatory in every SOC 2 report; the other four are in scope by management's election (see [README.md](./README.md)).

> **System boundary.** The Owera Agentic system as scoped for SOC 2 covers: the cloud-plane API (`owera-agentic-api` on Fly.io), the web dashboard (`owera-web` on Vercel), the status page (`status.owera.ai` on Vercel), the Cloudflare Named Tunnel terminating at the operator-plane gateway (`Owera-Op-Mini`), the operator-plane control daemon (`owera-fleet`) and its worker fleet, and the developer/admin access paths to those systems. Out of scope: customer end-user devices, customer-side network, third-party SaaS internals.

> **Authoritative source.** This document is a flattened, auditor-friendly view of [`soc2-cc.yaml`](./soc2-cc.yaml). When the two disagree, the YAML wins. Per-CC long-form evidence files live under [`evidence/`](./evidence/).

## Owner workstreams (role codes)

The plan-of-record names each workstream with a two- or three-letter role code; the controls below cite them in the **Owner workstream** column.

| Code | Role | Owner of |
|------|------|----------|
| **CSE** | Compliance & Security Engineer (WS-18) | Audit log, data retention, secrets policy, key rotation, this directory |
| **SRE** | Site Reliability Engineer (WS-15) | Cloud infra, CI/CD, deploy gates, on-call, DR drills |
| **IDE** | Identity & Access Engineer (WS-14) | Clerk integration, API-key issuance, tenant model |
| **PE** | Platform Engineer (WS-08) | Operator-plane gateway, tunnel, worker provisioning |
| **CISO** | Chief Information Security Officer (fractional, Rodrigo today) | Policy ownership, incident command, vendor risk |
| **CEO** | Chief Executive Officer (Rodrigo) | Board oversight, hiring, accountability |

## Control mapping (CC1 – CC9)

### CC1 — Control Environment

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC1.1 | Demonstrates commitment to integrity and ethical values. | Acceptable-use clause + employee handbook reference. | [`compliance/policies/security-policy.md`](../policies/security-policy.md) §11 → [`evidence/CC1.1-ethics.md`](./evidence/CC1.1-ethics.md) | CISO | implemented |
| CC1.2 | Board demonstrates independence and oversight. | Owera Software Ltda board minutes, quarterly. | 1Password `Owera/Governance/board-minutes/` → [`evidence/CC1.2-board-oversight.md`](./evidence/CC1.2-board-oversight.md) | CEO | TBD |
| CC1.3 | Management establishes structures, reporting lines, authorities. | Role/permission matrix in access-control policy. | [`compliance/policies/access-control-policy.md`](../policies/access-control-policy.md) §3 → [`evidence/CC1.3-org-structure.md`](./evidence/CC1.3-org-structure.md) | CISO | implemented |
| CC1.4 | Commitment to attract, develop, and retain competent individuals. | Onboarding training attestations. | HR SaaS export → [`evidence/CC1.4-hiring-training.md`](./evidence/CC1.4-hiring-training.md) | CEO | documented-only |
| CC1.5 | Holds individuals accountable for internal-control responsibilities. | Disciplinary policy + acceptable-use clause. | [`compliance/policies/security-policy.md`](../policies/security-policy.md) §11 → [`evidence/CC1.5-accountability.md`](./evidence/CC1.5-accountability.md) | CISO | documented-only |

### CC2 — Communication and Information

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC2.1 | Obtains/generates quality information to support internal control. | WORM hash-chained audit log + signed ledger. | [`api/internal/audit/audit.go`](../../api/internal/audit/audit.go), [`api/internal/audit/worm.go`](../../api/internal/audit/worm.go), [`owera-fleet/internal/ledger/`](../../../owera-fleet/internal/ledger/) → [`evidence/CC2.1-audit-log.md`](./evidence/CC2.1-audit-log.md) | CSE / SRE | documented-only |
| CC2.2 | Internally communicates info to support internal control. | On-call runbook + `#oncall` Slack archive. | [`compliance/runbooks/on-call-runbook.md`](../runbooks/on-call-runbook.md) → [`evidence/CC2.2-internal-comms.md`](./evidence/CC2.2-internal-comms.md) | SRE | implemented |
| CC2.3 | Communicates with external parties about internal control. | Incident-response policy + customer-comms templates. | [`compliance/runbooks/incident-response.md`](../runbooks/incident-response.md), [`compliance/runbooks/incident-comms-templates.md`](../runbooks/incident-comms-templates.md) → [`evidence/CC2.3-external-comms.md`](./evidence/CC2.3-external-comms.md) | CISO | implemented |

### CC3 — Risk Assessment

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC3.1 | Specifies suitable objectives. | Security policy purpose + data-classification section. | [`compliance/policies/security-policy.md`](../policies/security-policy.md) §1–§2 → [`evidence/CC3.1-objectives.md`](./evidence/CC3.1-objectives.md) | CISO | implemented |
| CC3.2 | Identifies and analyzes risks. | Annual risk assessment doc. | 1Password `Owera/Compliance/risk-assessment-YYYY.md` → [`evidence/CC3.2-risk-assessment.md`](./evidence/CC3.2-risk-assessment.md) | CISO | TBD |
| CC3.3 | Considers fraud risk. | Fraud-risk section inside the annual risk assessment. | 1Password (same as CC3.2) → [`evidence/CC3.3-fraud-risk.md`](./evidence/CC3.3-fraud-risk.md) | CISO | TBD |
| CC3.4 | Identifies and assesses changes impacting internal control. | Change-management policy. | [`compliance/policies/change-management-policy.md`](../policies/change-management-policy.md) → [`evidence/CC3.4-change-risk.md`](./evidence/CC3.4-change-risk.md) | SRE | implemented |

### CC4 — Monitoring Activities

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC4.1 | Performs ongoing and/or separate evaluations of internal control. | Quarterly access review per access-control policy §5. | [`compliance/policies/access-control-policy.md`](../policies/access-control-policy.md) §5; quarterly artifacts under `compliance/audit-controls/reviews/` (created with first review) → [`evidence/CC4.1-quarterly-reviews.md`](./evidence/CC4.1-quarterly-reviews.md) | CISO | documented-only |
| CC4.2 | Evaluates and communicates deficiencies. | Post-mortem template per incident-response §8. | [`compliance/runbooks/incident-response.md`](../runbooks/incident-response.md) §8 → [`evidence/CC4.2-deficiency-comms.md`](./evidence/CC4.2-deficiency-comms.md) | CISO | implemented |

### CC5 — Control Activities

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC5.1 | Selects/develops control activities that mitigate risks. | The seven policy docs (security, access, change, incident, retention, vendor, LGPD). | [`compliance/policies/`](../policies/) → [`evidence/CC5.1-control-activities.md`](./evidence/CC5.1-control-activities.md) | CISO | implemented |
| CC5.2 | Selects/develops general technology controls. | Cloud-plane configs version-controlled in repo. | [`infra/api.fly.toml`](../../infra/api.fly.toml), [`infra/web.vercel.json`](../../infra/web.vercel.json), [`infra/dns.cloudflare.yaml`](../../infra/dns.cloudflare.yaml), [`tunnel/config.example.yml`](../../tunnel/config.example.yml) → [`evidence/CC5.2-tech-controls.md`](./evidence/CC5.2-tech-controls.md) | SRE | implemented |
| CC5.3 | Deploys control activities through policies and procedures. | Change-management policy §3 CI gates (catalog-one-PR contract, CodeQL, secret-scan, tests). | [`compliance/policies/change-management-policy.md`](../policies/change-management-policy.md) §3, [`.github/workflows/`](../../.github/workflows/) → [`evidence/CC5.3-procedures.md`](./evidence/CC5.3-procedures.md) | SRE | implemented |

### CC6 — Logical & Physical Access Controls

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC6.1 | Logical access controls over protected assets. | API-key model: argon2id-hashed secrets, prefix lookup, constant-time verify. Clerk JWT verification for dashboard sessions. | [`api/internal/identity/identity.go`](../../api/internal/identity/identity.go) (argon2id, OWASP-2024 params), [`api/internal/auth/clerk.go`](../../api/internal/auth/clerk.go) (Clerk JWT + JWKS), [`compliance/policies/access-control-policy.md`](../policies/access-control-policy.md) → [`evidence/CC6.1-logical-access.md`](./evidence/CC6.1-logical-access.md) | IDE | implemented |
| CC6.2 | Registers and authorizes users before issuing credentials. | Onboarding playbook + Clerk-driven user provisioning. | [`compliance/runbooks/onboarding-playbook.md`](../runbooks/onboarding-playbook.md), [`compliance/policies/access-control-policy.md`](../policies/access-control-policy.md) §5 → [`evidence/CC6.2-user-registration.md`](./evidence/CC6.2-user-registration.md) | IDE | documented-only |
| CC6.3 | Authorizes/modifies/removes access based on roles. | Audit log captures `access-grant` / `access-revoke` actions; offboarding ≤24h SLA. | [`api/internal/audit/`](../../api/internal/audit/) (query `action IN ('access-grant','access-revoke')`) → [`evidence/CC6.3-role-authz.md`](./evidence/CC6.3-role-authz.md) | IDE / CSE | documented-only |
| CC6.4 | Restricts physical access to facilities. | Mac-mini gateway in locked office, FileVault, Kensington lock; SSH key (`~/.hermes_ssh_key`) passphrase-protected + stored in macOS Keychain. | 1Password `Owera/Compliance/physical-security/` (photos) → [`evidence/CC6.4-physical-access.md`](./evidence/CC6.4-physical-access.md) | PE | documented-only |
| CC6.5 | Discontinues access after use (offboarding, asset disposal). | Access-control policy §5 offboarding checklist with ≤24h revocation SLA. | [`compliance/policies/access-control-policy.md`](../policies/access-control-policy.md) §5 → [`evidence/CC6.5-asset-disposal.md`](./evidence/CC6.5-asset-disposal.md) | IDE / PE | implemented |
| CC6.6 | Boundary protection against external threats. | Cloudflare Tunnel terminating at gateway; control daemon binds **127.0.0.1 only**; admin tunnel separate from customer ingress; Vercel security headers; Cloudflare WAF + CAA + DMARC + SPF. | [`tunnel/config.example.yml`](../../tunnel/config.example.yml) (`metrics: 127.0.0.1:9300`), [`infra/tunnel.cloudflare.yaml`](../../infra/tunnel.cloudflare.yaml), [`infra/dns.cloudflare.yaml`](../../infra/dns.cloudflare.yaml), [`infra/web.vercel.json`](../../infra/web.vercel.json) → [`evidence/CC6.6-perimeter.md`](./evidence/CC6.6-perimeter.md) | PE / SRE | implemented |
| CC6.7 | Restricts data transmission/movement to authorized parties. | TLS everywhere (Cloudflare-terminated for inbound; Fly-managed for service-to-service); per-tenant scoping at the SQL layer; `customer-data-export.md` runbook gates ad-hoc exports. | [`compliance/policies/security-policy.md`](../policies/security-policy.md) §3, [`compliance/runbooks/customer-data-export.md`](../runbooks/customer-data-export.md) → [`evidence/CC6.7-data-movement.md`](./evidence/CC6.7-data-movement.md) | CISO | implemented |
| CC6.8 | Prevents/detects/acts on unauthorized or malicious software. | gitleaks pre-merge secret scan + CodeQL static analysis + Dependabot. macOS Gatekeeper on gateway; Homebrew-only third-party installs. | [`.github/workflows/secret-scan.yml`](../../.github/workflows/secret-scan.yml), [`.github/workflows/codeql.yml`](../../.github/workflows/codeql.yml), [`compliance/policies/security-policy.md`](../policies/security-policy.md) §8 → [`evidence/CC6.8-malware-defense.md`](./evidence/CC6.8-malware-defense.md) | SRE / CSE | documented-only |

### CC7 — System Operations

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC7.1 | Detects and monitors changes to configurations. | Git history + Fly releases + Vercel deployments + Cloudflare audit log. | `git log --follow infra/ tunnel/ compliance/`, `fly releases -a owera-agentic-api`, Vercel dashboard, Cloudflare audit log → [`evidence/CC7.1-config-monitoring.md`](./evidence/CC7.1-config-monitoring.md) | SRE | implemented |
| CC7.2 | Monitors components for anomalies. | Sentry (errors), Fly metrics (latency, restarts), Cloudflare analytics (edge), operator-plane heartbeat watchdog. | Sentry org `owera`; Fly dashboard; [`hermes-setup/scripts/heartbeat-watchdog.sh`](../../../hermes-setup/scripts/heartbeat-watchdog.sh) → [`evidence/CC7.2-anomaly-detection.md`](./evidence/CC7.2-anomaly-detection.md) | SRE / PE | documented-only |
| CC7.3 | Evaluates security events for material impact. | Severity classification in incident-response §1. | [`compliance/runbooks/incident-response.md`](../runbooks/incident-response.md) §1 → [`evidence/CC7.3-security-eval.md`](./evidence/CC7.3-security-eval.md) | CISO | implemented |
| CC7.4 | Responds to identified incidents. | Incident-response runbook; on-call rotation; web-incident playbook for cloud-plane outages. | [`compliance/runbooks/incident-response.md`](../runbooks/incident-response.md), [`compliance/runbooks/on-call-runbook.md`](../runbooks/on-call-runbook.md), [`compliance/runbooks/web-incident.md`](../runbooks/web-incident.md) → [`evidence/CC7.4-incident-response.md`](./evidence/CC7.4-incident-response.md) | CISO / SRE | implemented |
| CC7.5 | Recovers from incidents (post-mortem, remediation). | Incident-response §4 (Resolve) + §8 (Post-mortem); disaster-recovery doc for systemic recovery. | [`compliance/runbooks/incident-response.md`](../runbooks/incident-response.md) §4 + §8, [`infra/disaster-recovery.md`](../../infra/disaster-recovery.md) → [`evidence/CC7.5-recovery.md`](./evidence/CC7.5-recovery.md) | CISO / SRE | implemented |

### CC8 — Change Management

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC8.1 | Authorizes/designs/develops/configures/tests/approves changes. | Change-management policy with mandatory PR review, CI gates, catalog one-PR contract, GA-gate checklist. | [`compliance/policies/change-management-policy.md`](../policies/change-management-policy.md), [`compliance/policies/ga-gate.md`](../policies/ga-gate.md), [`.github/workflows/catalog-one-pr-contract.yml`](../../.github/workflows/catalog-one-pr-contract.yml) → [`evidence/CC8.1-change-management.md`](./evidence/CC8.1-change-management.md) | SRE | implemented |

### CC9 — Risk Mitigation

| CC # | Description | Owera control | Evidence path | Owner workstream | Status |
|------|-------------|---------------|---------------|-------------------|--------|
| CC9.1 | Mitigates risks from business disruptions. | Disaster-recovery doc with per-tier RTO/RPO; restic-encrypted off-host backup of operator plane. | [`infra/disaster-recovery.md`](../../infra/disaster-recovery.md), [`hermes-setup/scripts/backup-hermes-state.sh`](../../../hermes-setup/scripts/backup-hermes-state.sh) → [`evidence/CC9.1-disaster-recovery.md`](./evidence/CC9.1-disaster-recovery.md) | SRE / PE | implemented |
| CC9.2 | Manages risks associated with vendors. | Vendor management policy with criticality tiers; collects vendor SOC 2 reports annually. | [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md) → [`evidence/CC9.2-vendor-risk.md`](./evidence/CC9.2-vendor-risk.md) | CISO | implemented |

## Cross-references

- **Availability** — CC9.1 (DR) is the bridge; see [`tsc-availability.md`](./tsc-availability.md) for A-series detail (capacity, monitoring, backup-restore drill).
- **Confidentiality** — CC6.1 / CC6.7 / CC6.8 are the bridge; see [`tsc-confidentiality.md`](./tsc-confidentiality.md) for handling of customer payloads and secrets at rest/in transit.
- **Processing Integrity** — CC2.1 (audit log) is the bridge; see [`tsc-processing-integrity.md`](./tsc-processing-integrity.md) for the signed-ledger and cost-cap controls.
- **Privacy** — CC6.7 (data movement) is the bridge; see [`tsc-privacy.md`](./tsc-privacy.md) for LGPD/GDPR right-to-erasure plumbing.

## How to refresh this document

1. Update the canonical row in [`soc2-cc.yaml`](./soc2-cc.yaml).
2. Update the matching row in the table above. Status enum mapping: `ready`→`implemented`, `in-progress`→`documented-only`, `not-started`→`TBD`.
3. If a new Common Criterion is added, append a row to both files and write its `evidence/CCN.M-*.md` placeholder using the template at the end of any existing evidence file.
4. Run `yq '.controls[].evidence_path' soc2-cc.yaml | xargs -I{} test -f {}` locally to confirm all evidence files resolve.
