# Information security policy

**Scope:** all systems, data, and personnel that touch the Owera Agentic cloud plane (api.owera.ai, app.owera.ai, status.owera.ai) and the operator plane (Mac mini gateway + Hermes worker fleet).

**Audience:** every Owera Software Ltda employee and contractor with system access.

## 1. Purpose

Establish the baseline security commitments Owera makes to its customers, its regulators (ANPD under LGPD, and conditionally EU DPAs under GDPR), and its prospective SOC 2 auditor. Anything *not* contradicted by this policy is permitted; anything that contradicts it is a security incident.

## 2. Information classification

| Class | Examples | Handling |
|-------|----------|----------|
| **Public** | Marketing pages, open-source code, status page content | No restrictions. |
| **Internal** | Operator-plane source code, runbooks, internal Slack | Owera personnel only; not shared externally without CISO approval. |
| **Confidential** | Customer account metadata, billing records, audit logs | Encrypted at rest and in transit; access logged. |
| **Restricted** | Customer payloads (the actual content of agent jobs), secrets, encryption keys | Encrypted at rest with per-tenant keys where supported; access requires named-individual authorization; access fully audited. |

Customer payloads are **Restricted** by default. Customers may opt their data into a lower class only through an explicit contractual amendment.

## 3. Encryption

| State | Standard | Notes |
|-------|----------|-------|
| In transit (public) | TLS 1.3, ECDHE, AEAD ciphers only | Enforced by Cloudflare on api.owera.ai, app.owera.ai, status.owera.ai. HSTS preloaded. |
| In transit (operator plane) | Cloudflare Named Tunnel (TLS 1.3) + minisign-signed payloads | Tunnel CNAME is `internal-rpc.owera.ai`; see `infra/tunnel.cloudflare.yaml`. |
| At rest (api SQLite cache) | sqlcipher (AES-256) | Passphrase = `SQLITE_ENCRYPTION_KEY`, rotated annually. |
| At rest (Fly volumes) | Fly platform-default LUKS | We do not depend on Fly volumes for durable state. |
| At rest (operator plane) | macOS FileVault (full disk) | Gateway and workers. |
| At rest (backups) | restic (AES-256, per-repo key) | Daily encrypted snapshots; see `owera-fleet/scripts/backup-hermes-state.sh`. |

## 4. Access control

Detailed in [`access-control-policy.md`](./access-control-policy.md). Summary: RBAC, principle of least privilege, MFA mandatory, quarterly access reviews.

## 5. Incident response

Detailed in [`incident-response-policy.md`](./incident-response-policy.md). Operational runbook for an active Sev1: [`../runbooks/incident-response.md`](../runbooks/incident-response.md).

## 6. Consent (LGPD Art. 8)

All customer data treatment is grounded in either (a) explicit consent collected at signup or (b) a legal basis under LGPD Art. 7. Consent is captured per-treatment-purpose, is revocable, and is logged in the audit-log WORM store. The signup flow surfaces the privacy notice prominently and requires affirmative action — pre-ticked boxes are forbidden.

## 7. Data retention

Detailed in [`data-retention-policy.md`](./data-retention-policy.md). Summary: customer payloads deleted within 30d of account closure (LGPD Art. 19); audit logs kept WORM for 7 years.

## 8. Vulnerability management

| Layer | Tooling | Cadence |
|-------|---------|---------|
| Source code | `gitleaks` (secrets), `gosec` (api), `npm audit` (web), Dependabot | Every PR + weekly scheduled |
| Container images | `trivy` against Fly deploys | Every deploy + weekly |
| Dependencies | Renovate (low-noise mode) + Dependabot security alerts | Continuous |
| Infrastructure | `cloudflared` + Fly + Vercel CVE feeds | Subscribed; review weekly |
| Penetration test | External vendor | Annual; first scheduled GA + 6 months |

Sev1 vulnerabilities are remediated within 24h; Sev2 within 7d; Sev3 within 30d.

## 9. Change management

Detailed in [`change-management-policy.md`](./change-management-policy.md). Summary: peer-reviewed PRs, staging gate, rollback procedure, no direct prod commits.

## 10. Vendor management

Detailed in [`vendor-management-policy.md`](./vendor-management-policy.md). Summary: third-party risk assessed at onboarding, reviewed annually, attested via SOC 2 / ISO / LGPD compliance evidence.

## 11. Acceptable use

All Owera personnel agree, in writing at hire and annually:

- Use Owera systems for authorized business purposes only.
- Do not bypass authentication, MFA, or encryption controls.
- Report suspected security events to `security@owera.ai` within 1 business hour.
- Do not export customer data to personal devices, personal accounts, or non-approved SaaS.
- Treat the operator-plane SSH keys and any tunnel credentials as Restricted.

Violations are handled per Owera's HR disciplinary policy and, if applicable, reported to authorities.

## 12. Awareness and training

All personnel complete security awareness training at hire and annually thereafter. Engineers with production access complete additional secure-coding and incident-response training. Records are kept in the HR system and surfaced to the auditor on request.

## Ownership

| Role | Responsibility |
|------|----------------|
| CISO | Accountable for the policy; reviews annually |
| SRE Lead | Operational enforcement of §3, §4, §8 |
| CFO | Operational enforcement of §10 |
| All personnel | Adherence to §11 |

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
