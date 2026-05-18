# Trust Service Category — Confidentiality

> **Scope.** Confidentiality covers data identified by Owera or its customers as confidential. The auditor will read management's system description for the confidentiality commitments. As of 2026-05-17 those are:
> - Customer job payloads (request bodies, attachments) — confidential, accessible only to the requesting tenant.
> - Billing-derived data (Stripe customer ids, invoice metadata) — confidential, accessible only to Owera operators with a billing role.
> - Identity material (Clerk JWTs, API-key plaintext, hashed credentials) — confidential, with API-key plaintext **never persisted**.
> - Secrets (Fly secrets, Vercel env vars, Cloudflare tokens, Stripe keys, restic password, minisign signing keys) — confidential, distributed only through approved secret stores (Fly Secrets, Vercel env vars, 1Password, macOS Keychain).

## Control mapping

| C # | Description | Owera control | Evidence path | Owner workstream | Status |
|-----|-------------|---------------|---------------|-------------------|--------|
| C1.1 | Identifies and maintains confidential information. | Data-classification matrix in security policy + secrets manifest naming every secret. | [`compliance/policies/security-policy.md`](../policies/security-policy.md) §2 (classification), [`infra/secrets-manifest.md`](../../infra/secrets-manifest.md), [`compliance/policies/data-retention-policy.md`](../policies/data-retention-policy.md) | CISO | implemented |
| C1.2 | Disposes of confidential information when no longer needed. | Data-retention policy with per-class retention windows + LGPD/GDPR erasure pipeline. | [`compliance/policies/data-retention-policy.md`](../policies/data-retention-policy.md), [`api/internal/erasure/erasure.go`](../../api/internal/erasure/erasure.go), [`api/internal/erasure/purger.go`](../../api/internal/erasure/purger.go), [`compliance/runbooks/customer-data-deletion.md`](../runbooks/customer-data-deletion.md) → cross-link [`evidence/CC6.5-asset-disposal.md`](./evidence/CC6.5-asset-disposal.md) | CSE | implemented |
| C1.3 (Owera-defined) | Encrypts confidential data at rest. | Fly volume encryption (provider-managed); operator-plane FileVault on the gateway Mac mini; restic-encrypted off-host backup with password held in 1Password. | [`infra/api.fly.toml`](../../infra/api.fly.toml) (Fly volumes are encrypted by default), [`hermes-setup/scripts/backup-hermes-state.sh`](../../../hermes-setup/scripts/backup-hermes-state.sh) (restic AES-256), 1Password `Owera/Operator/restic-password` | SRE / PE | implemented |
| C1.4 (Owera-defined) | Encrypts confidential data in transit. | TLS everywhere: Cloudflare-terminated inbound, Cloudflare Named Tunnel for gateway ingress, Fly-managed TLS for internal service-to-service. | [`infra/tunnel.cloudflare.yaml`](../../infra/tunnel.cloudflare.yaml), [`infra/dns.cloudflare.yaml`](../../infra/dns.cloudflare.yaml), [`compliance/policies/security-policy.md`](../policies/security-policy.md) §3 → cross-link [`evidence/CC6.7-data-movement.md`](./evidence/CC6.7-data-movement.md) | PE / SRE | implemented |
| C1.5 (Owera-defined) | Stores credentials as one-way hashes; never persists plaintext API-key material. | API-key plaintext is minted, returned once, then discarded; only the `prefix` (lookup index) and argon2id verifier (PHC string `$argon2id$v=19$m=65536,t=3,p=4$salt$hash`) are persisted. Constant-time compare on verify; fixed-cost verify on prefix miss. | [`api/internal/identity/identity.go`](../../api/internal/identity/identity.go) (see header comment + argon2 constants `argon2Time=3`, `argon2Memory=64 MiB`, `argon2Threads=4`, per OWASP 2024) → cross-link [`evidence/CC6.1-logical-access.md`](./evidence/CC6.1-logical-access.md) | IDE | implemented |
| C1.6 (Owera-defined) | Secrets are distributed only through approved secret stores; never committed to git. | gitleaks pre-merge scan; `infra/secrets-manifest.md` is the canonical catalogue; key rotation runbook for quarterly rotation. | [`.github/workflows/secret-scan.yml`](../../.github/workflows/secret-scan.yml), [`infra/secrets-manifest.md`](../../infra/secrets-manifest.md), [`compliance/runbooks/key-rotation.md`](../runbooks/key-rotation.md) | CSE / SRE | implemented |
| C1.7 (Owera-defined) | Confidential information transmitted off-platform follows a documented export procedure. | Customer-data export runbook gates ad-hoc exports (until the self-serve endpoint lands per WS-18 backlog). | [`compliance/runbooks/customer-data-export.md`](../runbooks/customer-data-export.md) | CISO | documented-only |
| C1.8 (Owera-defined) | Operator-plane gateway access is gated by a passphrase-protected SSH key held in macOS Keychain. | `~/.hermes_ssh_key` is the only path; bootstrap script provisions it; no shared accounts. | [`hermes-setup/scripts/bootstrap-hermes-node.sh`](../../../hermes-setup/scripts/bootstrap-hermes-node.sh), `~/.hermes_ssh_key` (out of tree, on operator's Mac), 1Password `Owera/Operator/ssh-key-passphrase` → cross-link [`evidence/CC6.4-physical-access.md`](./evidence/CC6.4-physical-access.md) | PE | implemented |

## Common-Criteria controls invoked by Confidentiality

| CC # | Description | Why it matters for Confidentiality |
|------|-------------|------------------------------------|
| CC6.1 | Logical access controls. | argon2id-hashed API keys + Clerk JWT verification are the confidentiality controls at the access layer. |
| CC6.6 | Boundary protection. | The 127.0.0.1 binding on the operator-plane control daemon and the Cloudflare Tunnel are what keep confidential payloads off the public internet. |
| CC6.7 | Data movement. | The export runbook + TLS-everywhere posture are the data-movement controls. |
| CC6.8 | Malware defense. | gitleaks + CodeQL keep confidentiality-violating regressions out of main. |

## Known gaps (summary)

See [`known-gaps.md`](./known-gaps.md). Confidentiality-specific gaps:

- **C1.7 self-serve export endpoint.** Today the runbook says "until the self-serve endpoint lands, the SRE-on-duty exports each tenant's data manually." Need the endpoint built; tracked in WS-18 backlog. Owner: CSE.
- **C1.6 vendor SOC 2 collection.** [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md) has `[ ] TODO` boxes for every vendor; the auditor will want each one populated with a "last collected" date. Owner: CISO.

## Cross-references

- **Security** — see [`tsc-security.md`](./tsc-security.md) for CC1–CC9.
- **Privacy** — see [`tsc-privacy.md`](./tsc-privacy.md) for the personal-data sub-scope of Confidentiality (LGPD/GDPR).
- **Evidence collection** — see [`evidence-collection-runbook.md`](./evidence-collection-runbook.md) for the commands to demonstrate `gitleaks` clean status, dump the argon2id verifier shape from a test API key, and list Fly secrets without revealing values.
