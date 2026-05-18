# Evidence collection runbook

> **Purpose.** This document tells the CSE-on-duty (and, by extension, the SOC 2 auditor sitting next to the CSE during the audit window) exactly how to collect each evidence type called out in [`soc2-cc.yaml`](./soc2-cc.yaml) and the per-TSC docs.

> **When to run.** Two situations:
> 1. **Pre-audit screenshot pass** — 7 days before the auditor's read date, the CSE walks this runbook end-to-end and checks every out-of-tree screenshot into `compliance/audit-controls/evidence/<CC-id>-screenshots/`.
> 2. **Live audit walkthrough** — the auditor selects a sample of controls; the CSE re-runs the relevant section on a screenshare so the auditor sees the artifact appear in real time.

> **Operator identity.** All commands assume the CSE is logged in to the operator's Mac (host user `claw3`, UID 501) with `gh`, `fly`, `vercel`, `restic`, `aws`, and `sqlite3` on `$PATH`. Anything that requires a passphrase (the operator-plane SSH key) prompts for it and writes to macOS Keychain on first use.

## Table of contents

1. [Repo-resident evidence (markdown files)](#1-repo-resident-evidence)
2. [Code evidence (source-tree paths)](#2-code-evidence)
3. [Config evidence (version-controlled cloud-plane configs)](#3-config-evidence)
4. [Log-query evidence (SQLite, JSONL ledger, Sentry, Cloudflare)](#4-log-query-evidence)
5. [Screenshot evidence (third-party dashboards)](#5-screenshot-evidence)
6. [Attestation evidence (vendor SOC 2s, employee training, board minutes)](#6-attestation-evidence)
7. [Per-TSC live walkthroughs](#7-per-tsc-live-walkthroughs)

---

## 1. Repo-resident evidence

### Policies and runbooks

```bash
cd ~/owera-cloud
git rev-parse HEAD                                  # capture commit SHA the auditor reads
ls compliance/policies/ compliance/runbooks/        # confirm both directories present
git log --follow --pretty='%h %ad %s' --date=short \
  compliance/policies/<policy>.md                   # show the change history for any specific policy
```

What to hand the auditor: the commit SHA, plus a printed copy of the policy/runbook in question. The git log proves the policy existed at audit-window start and was version-controlled.

### Per-CC evidence files

```bash
cd ~/owera-cloud
yq '.controls[] | [.id, .evidence_path] | @tsv' \
  compliance/audit-controls/soc2-cc.yaml \
  | column -t                                       # CC id ↔ evidence file
xargs -I{} test -f {} <<<"$(yq '.controls[].evidence_path' \
  compliance/audit-controls/soc2-cc.yaml)" \
  && echo "all evidence files present"              # CI's lint, run by hand
```

Every CC id resolves to a markdown file under `compliance/audit-controls/evidence/`. The lint should be clean.

---

## 2. Code evidence

### API-key argon2id hashing (CC6.1 / C1.5)

```bash
cd ~/owera-cloud
grep -n 'argon2Time\|argon2Memory\|argon2Threads\|argon2KeyLen' \
  api/internal/identity/identity.go
# Expected: argon2Time=3, argon2Memory=64 MiB, argon2Threads=4, argon2KeyLen=32 (OWASP 2024)
go test ./api/internal/identity/...                 # confirms the verifier round-trips
```

### Clerk JWT verification (CC6.1)

```bash
cd ~/owera-cloud
grep -n 'JWKS\|issuer\|verify' api/internal/auth/clerk.go | head
go test ./api/internal/auth/...
```

### WORM audit log + SQLite triggers (CC2.1 / PI1.5)

```bash
cd ~/owera-cloud
grep -n 'ErrWORMLocked\|object lock\|retention' \
  api/internal/audit/audit.go api/internal/audit/worm.go
go test ./api/internal/audit/...
```

### Signed JSONL ledger (PI1.3 / PI1.5)

```bash
cd ~/owera-fleet
grep -n 'signing key\|Ed25519\|prev_hash\|Signature' internal/ledger/ledger.go
go test ./internal/ledger/...
# Take one task id and verify its chain end-to-end:
fleetctl ledger verify <task-id>                    # exit 0 = signatures + chain valid
```

### LGPD/GDPR right-to-erasure (P4.3 / CC6.5)

```bash
cd ~/owera-cloud
grep -n 'LGPD Art\|GDPR Art\|SLA\|15 working days' \
  api/internal/erasure/erasure.go
go test ./api/internal/erasure/...
```

### Per-tenant cost cap (PI1.6)

```bash
cd ~/owera-cloud
grep -n '402\|Retry-After\|costcap' \
  api/internal/billing/costcap.go
go test ./api/internal/billing/... -run TestCostCap
```

---

## 3. Config evidence

### Cloud-plane configs (CC5.2 / CC6.6)

```bash
cd ~/owera-cloud
git log --follow --pretty='%h %ad %s' --date=short -- infra/api.fly.toml
git log --follow --pretty='%h %ad %s' --date=short -- infra/web.vercel.json
git log --follow --pretty='%h %ad %s' --date=short -- infra/dns.cloudflare.yaml
git log --follow --pretty='%h %ad %s' --date=short -- tunnel/config.example.yml
```

### Tunnel binding (CC6.6 / boundary protection)

```bash
cd ~/owera-cloud
grep -n '127.0.0.1\|localhost' tunnel/config.example.yml
# Expected: `metrics: 127.0.0.1:9300` — confirms control-daemon metrics are not exposed to the tunnel.
```

### Secrets manifest (C1.6)

```bash
cd ~/owera-cloud
ls infra/secrets-manifest.md                        # exists
fly secrets list -a owera-agentic-api               # live list (names only; values redacted)
vercel env ls --scope owera                         # web tier
```

### CI gates (CC5.3 / CC6.8)

```bash
cd ~/owera-cloud
ls .github/workflows/
# Expected files: ci-api.yml, ci-web.yml, ci-status.yml, codeql.yml,
# secret-scan.yml, catalog-one-pr-contract.yml, release.yml
gh workflow list --repo owera/owera-cloud
gh run list --workflow secret-scan.yml --limit 10
gh run list --workflow codeql.yml --limit 10
```

---

## 4. Log-query evidence

### WORM audit log — recent 90 days (CC2.1 / CC6.3)

```bash
# On the Fly machine (or via fly ssh):
fly ssh console -a owera-agentic-api -C \
  "sqlite3 /data/audit.sqlite \
   'SELECT ts, tenant_id, action, target, hash, prev_hash \
    FROM audit WHERE ts >= datetime(\"now\",\"-90 days\") \
    ORDER BY ts DESC LIMIT 100;'"
```

For full coverage of a tenant during the audit window:

```bash
sqlite3 /data/audit.sqlite \
  "SELECT count(*) FROM audit \
   WHERE tenant_id = '<tenant-id>' \
   AND ts BETWEEN '<window-start>' AND '<window-end>';"
```

To demonstrate tamper-evidence (PI1.5): pick a row, attempt an UPDATE, observe the trigger fires.

```sql
-- inside sqlite3 prompt; expected: UPDATE fails with trigger error
UPDATE audit SET action = 'tampered' WHERE id = <test-id>;
```

### Signed ledger replay (PI1.3 / PI1.5)

```bash
# On the operator-plane gateway:
fleetctl ledger list --since 90d
fleetctl ledger verify <task-id>                    # exit 0 = chain + signatures valid
fleetctl ledger dump <task-id> | head -5            # show entries
```

### Sentry anomaly history (CC7.2)

URL: `https://sentry.io/organizations/owera/issues/?statsPeriod=90d`. Filter by `project:owera-agentic-api`. Capture the issue list as a PDF for the audit folder.

### Cloudflare audit log (CC7.1 — config changes)

URL: `https://dash.cloudflare.com/<account-id>/audit-log`. Filter by zone `owera.ai`. Export CSV for the 90-day window.

### Fly release history (CC7.1)

```bash
fly releases -a owera-agentic-api
fly releases -a owera-web
fly releases -a owera-status
```

### Vercel deployment history (CC7.1)

```bash
vercel ls --scope owera                             # one-line per deployment
vercel inspect <deployment-url>                     # full record incl. who deployed
```

### Heartbeat watchdog status (A1.4)

```bash
cd ~/hermes-setup
ls -la ~/.hermes/heartbeats/                        # most recent file per worker
tail -50 ~/.hermes/logs/watchdog.jsonl              # most recent scans
launchctl list | grep com.hermes.watchdog           # confirm the LaunchAgent is running
```

### restic backup verification (A1.2 / C1.3)

```bash
cd ~/hermes-setup
restic snapshots --json | jq '.[-5:]'               # five most recent snapshots
restic check --read-data-subset=1%                  # bit-level integrity check
tail -100 ~/.hermes/logs/backup.log                 # last week of nightly runs
```

### Erasure pipeline trace (P4.3)

```bash
# 1. Issue a test deletion (in staging):
curl -X DELETE https://api.staging.owera.ai/v1/tenants/me/data \
  -H "Authorization: Bearer <staging-api-key>"
# 2. Confirm audit row:
sqlite3 /data/audit.sqlite \
  "SELECT * FROM audit WHERE action='erasure-request' \
   ORDER BY ts DESC LIMIT 1;"
# 3. Wait for the worker to drain the queue (≤15 working days; in staging it's seconds):
# 4. Confirm rows for that tenant are gone:
sqlite3 /data/main.sqlite \
  "SELECT count(*) FROM jobs WHERE tenant_id = '<test-tenant>';" # expected: 0
```

---

## 5. Screenshot evidence

For every SaaS-console-based control, the CSE takes a dated screenshot 7 days before the audit-window start date and checks it into `compliance/audit-controls/evidence/<CC-id>-screenshots/`. The naming convention is `YYYY-MM-DD-<short-description>.png`.

| Control | Console / URL | What to capture |
|---------|---------------|-----------------|
| CC1.4 | HR SaaS (TBD vendor) | Onboarding-checklist completion record per employee. |
| CC1.2 | 1Password `Owera/Governance/board-minutes/` | Index page listing four quarterly board-meeting minutes. |
| CC6.4 | 1Password `Owera/Compliance/physical-security/` | Photos of gateway in locked-office setup, FileVault active. |
| CC6.6 | Cloudflare dashboard → Security → WAF | WAF ruleset enabled; rate-limit rules active. |
| CC6.8 | GitHub Actions → secret-scan + codeql workflow runs | "Last successful run" page for both, dated within the audit window. |
| CC7.2 | Sentry → Issues page | "Open issues" count, filtered by `owera-agentic-api` project. |
| CC7.2 | Fly → Metrics dashboard for `owera-agentic-api` | Latency + error-rate over the 90-day audit window. |
| CC9.2 | Vendor SOC 2 reports collected in 1Password `Owera/Compliance/vendor-soc2/` | Index listing one report per critical vendor. |
| A1.1 | Fly autoscaler config page | Current `min`/`max` machine count + last scaling event. |
| A1.5 | `status.owera.ai` | Status page showing 90-day uptime history. |
| C1.6 | Fly Secrets list + Vercel env-var list (names only, values redacted) | Confirms zero plaintext secrets in source tree. |
| P1.1 | `owera.ai/privacy` rendered page | Bilingual privacy notice, dated. |

---

## 6. Attestation evidence

Attestations are documents signed (or sealed) by a third party. They live out-of-tree, typically in 1Password:

| Control | Document | Location |
|---------|----------|----------|
| CC1.2 | Quarterly board minutes | 1Password `Owera/Governance/board-minutes/YYYY-QN.pdf` |
| CC1.4 | Onboarding training certificates per employee | HR SaaS export → 1Password `Owera/HR/training/` |
| CC3.2 | Annual risk assessment | 1Password `Owera/Compliance/risk-assessment-YYYY.md` |
| CC9.2 / P6.2 | Vendor SOC 2 Type 2 reports | 1Password `Owera/Compliance/vendor-soc2/<vendor>/<year>.pdf`; one per critical/important vendor in [`compliance/policies/vendor-management-policy.md`](../policies/vendor-management-policy.md) |
| CC6.4 | Physical-security photos + Mac mini serial-number registry | 1Password `Owera/Compliance/physical-security/` |
| Privacy DPA | Signed Data Processing Agreement per customer | 1Password `Owera/Customers/<tenant-name>/dpa.pdf` |

At audit time the CSE walks the auditor through the 1Password vault structure on screenshare; the auditor confirms each referenced document exists and is dated within the audit window.

---

## 7. Per-TSC live walkthroughs

The auditor selects a sample of controls; the CSE walks each one live. Estimated time per walkthrough: 10-20 minutes.

### Security walkthrough (CC6.1)

1. Open [`api/internal/identity/identity.go`](../../api/internal/identity/identity.go) in the editor; point at the argon2id constants.
2. Run `go test ./api/internal/identity/...` — passes.
3. `sqlite3 /data/main.sqlite "SELECT prefix, length(verifier) FROM api_keys LIMIT 5;"` — every `verifier` is a PHC string starting `$argon2id$v=19$`.
4. Demonstrate that no row contains plaintext: `SELECT * FROM api_keys WHERE verifier NOT LIKE '$argon2id$%';` — returns zero rows.

### Availability walkthrough (CC9.1 + A1.2)

1. Open [`infra/disaster-recovery.md`](../../infra/disaster-recovery.md); show the RTO/RPO targets table.
2. Show the most recent quarterly DR drill check-in (file under `compliance/audit-controls/evidence/A1.3-drill-screenshots/`).
3. Run `restic snapshots --json | jq '.[-1]'` — most recent snapshot is from last night.
4. Run `restic check --read-data-subset=1%` — integrity-clean.

### Confidentiality walkthrough (C1.5 + C1.6)

1. Pull a fresh API key via the dashboard; note the returned plaintext only appears once.
2. SQL: `SELECT prefix, length(plaintext) FROM api_keys LIMIT 5;` — column `plaintext` does not exist (referencing the schema confirms the absence).
3. Run `gh run list --workflow secret-scan.yml --limit 1` — most recent run is `success`.
4. Run `gitleaks detect --no-git -v` locally — clean exit.

### Processing Integrity walkthrough (PI1.3 + PI1.5)

1. Issue a chargeable API call against staging with curl.
2. Show the audit row appearing: `sqlite3 /data/audit.sqlite "SELECT * FROM audit ORDER BY ts DESC LIMIT 1;"`.
3. Show the ledger entry on the operator plane: `fleetctl ledger verify <task-id>` — exit 0.
4. Attempt `UPDATE audit SET action='x' WHERE id=<id-from-step-2>;` — trigger fires; row is unchanged.

### Privacy walkthrough (P4.3 / right-to-erasure)

1. Issue `DELETE /v1/tenants/me/data` against staging with curl.
2. Confirm the audit row: `sqlite3 /data/audit.sqlite "SELECT * FROM audit WHERE action='erasure-request' ORDER BY ts DESC LIMIT 1;"`.
3. Wait for the worker tick (seconds in staging).
4. Confirm tenant rows are gone: `SELECT count(*) FROM jobs WHERE tenant_id='<test-tenant>';` returns 0.
5. Open [`api/internal/erasure/erasure.go`](../../api/internal/erasure/erasure.go) header comment showing LGPD/GDPR statutory basis.

---

## Refresh

This runbook is regenerated whenever:

- A new control row is added to [`soc2-cc.yaml`](./soc2-cc.yaml) with an evidence type not already covered.
- A SaaS console URL changes.
- A CLI in the procedure is renamed (e.g. if `fleetctl` re-bases to `owerafleetctl`).

Last reviewed: 2026-05-17 (initial creation, Wave 10 Track A item H5).
