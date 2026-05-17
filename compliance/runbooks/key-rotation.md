# Key rotation runbook

Quarterly rotation drill for every long-lived key that grants production access or signs production artifacts. Two reasons we do this:

1. **SOC 2 CC6.1 / CC6.7** require demonstrable evidence that access credentials are rotated.
2. **LGPD Art. 50 §2** requires the controller maintain security measures appropriate to the risk; old keys are an unmitigated risk.

This runbook is the canonical execution path. The cadence policy lives in [`../policies/security-policy.md`](../policies/security-policy.md) §3 (encryption + key management) and [`../policies/access-control-policy.md`](../policies/access-control-policy.md) §6 (authentication mechanics).

## Scope

| Key | System | Cadence | Owner | Rotation type |
|-----|--------|---------|-------|---------------|
| **minisign signing key** | Operator-plane ledger + configsync (`owera-fleet/internal/ledger`, `owera-fleet/internal/configsync`) | **Quarterly** | SRE Lead | Generate-new, dual-trust window, retire-old |
| **Stripe restricted API key** (live) | `owera-cloud/api/internal/billing` and webhooks; Fly secret `STRIPE_API_KEY` | **Quarterly** | CFO (executes) + SRE Lead (rotates infra) | Issue-new, swap-and-verify, revoke-old |
| **Stripe webhook signing secret** (live) | `owera-cloud/api/internal/webhooks`; Fly secret `STRIPE_WEBHOOK_SECRET` | **Quarterly** | CFO (executes) + SRE Lead (rotates infra) | Issue-new from dashboard, dual-trust window in code, retire-old |
| **Cloudflare API token** (account-scoped, restricted to Zero-Trust + DNS + Tunnels) | `owera-cloud/infra/dns.cloudflare.yaml` deploy automation; tunnel client on the operator plane gateway | **Quarterly** | SRE Lead | Issue-new in dashboard, swap secret, revoke-old |
| **Cloudflare Tunnel credentials** (`internal-rpc.owera.ai` tunnel) | `owera-fleet` gateway `cloudflared` daemon; matched Vercel/Fly outbound | **Annually** (Tunnel JWTs are long-lived; rotate on suspicion or yearly) | SRE Lead | Rebuild tunnel, dual-route window |
| **SSH ed25519 keys** (`~/.hermes_ssh_key`) | Operator-plane worker access | **Annually** + on offboarding | SRE Lead | Issue-new, distribute via Keychain, retire-old |
| **restic backup repository password** | Backup pipeline (`owera-fleet/scripts/backup-hermes-state.sh`) | **Annually** unless urgent erasure forces re-key | SRE Lead | `restic key add` / `restic key remove` |
| **SQLite encryption passphrase** (`SQLITE_ENCRYPTION_KEY`) | api SQLite via sqlcipher | **Annually** | SRE Lead | Re-key in place via `PRAGMA rekey`; redeploy api |

Out of scope for this runbook: per-customer API keys (rotated by customers via the dashboard; revocation is a different runbook), Google Workspace SSO tokens (managed in WorkOS / Google admin console).

## Pre-rotation checklist (every key, every cadence)

```
[ ] Read this runbook end-to-end before starting.
[ ] Confirm you have the right approvals:
    - Stripe: CFO must be in the call (Stripe console requires
      Stripe-account-level auth that the SRE may not hold).
    - Cloudflare account-scoped tokens: 2nd SRE or CISO approval per
      access-control-policy.md §4 break-glass for "scoped-token rotate".
    - minisign: SRE Lead alone is sufficient (it's a generate-new ops
      action; the old key continues to verify during the dual-trust
      window).
[ ] Verify the production health baseline is green BEFORE rotating.
    - `curl https://api.owera.ai/healthz`         → 200 ok
    - `curl https://api.owera.ai/readyz`          → 200 ready
    - `curl https://status.owera.ai/v1/status`    → all-systems-operational
[ ] Open the on-call channel: post "rotation drill starting: <KEY>".
[ ] Snapshot the audit-log WORM store as of NOW (record the latest
    audit_log id).
[ ] Have rollback values to hand (the OLD key is your rollback).
```

## 1. minisign key rotation (operator-plane signing)

The minisign keypair signs every ledger entry on the operator plane and every config bundle pushed by configsync. The verify path is in code (the keys are baked into the gateway and worker binaries). Rotation uses a **dual-trust** window: introduce the new key while the old one is still trusted, push artifacts signed by the new key, then drop trust of the old.

```
[ ] Generate the new keypair on the gateway:
    minisign -G -p /tmp/minisign.new.pub -s /tmp/minisign.new.sec
    (passphrase: generate with 1Password, store under
     "owera/secrets/minisign/<YYYY-Q>.sec.passphrase")
[ ] Capture the new public key fingerprint:
    minisign -F -p /tmp/minisign.new.pub
[ ] Land a PR on owera-fleet that adds the new pubkey to the
    trusted-keys list (does NOT remove the old). CI must pass with
    BOTH keys trusted.
[ ] After PR merges and the fleet has deployed (verify with the
    heartbeat-watchdog), switch the SIGNING key on the gateway:
    1. mv ~/.hermes/keys/minisign.sec ~/.hermes/keys/minisign.OLD.sec
    2. mv /tmp/minisign.new.sec ~/.hermes/keys/minisign.sec
    3. Restart the signer process; observe one ledger entry signed
       by the new key.
[ ] Wait 7 days (one ledger-snapshot window) with dual-trust in place.
    During this window, both keys verify; new artifacts are signed by
    the new key.
[ ] Land a second PR on owera-fleet that removes the OLD pubkey from
    the trusted-keys list. CI fails if any unsigned-by-new artifact
    is still referenced.
[ ] After that PR deploys, archive the old secret key in 1Password
    under "owera/compromised-credentials/minisign-<YYYY-Q>" with
    1-year retention, then DELETE the on-disk copy:
    shred -u ~/.hermes/keys/minisign.OLD.sec
[ ] Update SECURITY_NOTES.md (on owera-fleet) with the new pubkey
    fingerprint and the rotation date.
[ ] Audit row: action=key.rotate, target=minisign, user_id=<SRE>.
```

**Rollback (within dual-trust window):** copy `minisign.OLD.sec` back to `minisign.sec`, restart signer. New artifacts go back to being signed by the old key, which is still trusted.

**Rollback (after old-key removal):** there is no rollback — re-add the old key by reverting the trust-list PR. Treat this as a Sev2 incident; the trust-list change should have caught problems before merging.

## 2. Stripe restricted API key rotation

Stripe issues restricted keys via the dashboard at `Developers → API keys`. Owera uses one **restricted** key (not the secret key) scoped to: read customers/subscriptions/invoices, write usage records, read+write payment methods.

```
[ ] CFO logs into Stripe dashboard, navigates Developers → API keys.
[ ] Create a new restricted key with label "owera-prod-<YYYY-Q>".
    Scopes: as documented in infra/secrets-manifest.md §Stripe.
[ ] Copy the new `rk_live_...` value (Stripe shows it ONCE).
[ ] On the gateway (or wherever Fly CLI is authed):
    fly secrets set STRIPE_API_KEY=rk_live_... -a owera-api-prod
[ ] Fly triggers a rolling deploy (~60s for the API). Watch the
    deploy logs for any 401 from Stripe — should be zero.
[ ] Smoke test:
    curl -H "Authorization: Bearer <fixture-key>" \
         https://api.owera.ai/v1/usage
    (should return the period's meters; if 500, Stripe call failed)
[ ] Wait 15 minutes — long enough for any in-flight retries to drain.
[ ] In Stripe dashboard, REVOKE the OLD restricted key
    (it's labeled "owera-prod-<prev-Q>").
[ ] Verify the revocation took: in Stripe dashboard the key shows
    "Revoked"; in api logs, no calls to the old key in the last 30m.
[ ] Audit row: action=key.rotate, target=stripe-api-key, user_id=<CFO>.
```

**Rollback:** the OLD key is still valid for ~15 minutes after the new one is set (Stripe doesn't auto-revoke). If the new key fails, `fly secrets set STRIPE_API_KEY=<old>` and redeploy. Then troubleshoot the new key's scope before re-attempting.

## 3. Stripe webhook signing secret rotation

Stripe webhooks ship with a signature header that the API validates against the webhook signing secret. Rotation needs a **dual-trust** window because Stripe doesn't let you set "both" secrets — you must accept signatures from EITHER for a short interval.

```
[ ] CFO logs into Stripe dashboard, Developers → Webhooks.
[ ] Click the endpoint for api.owera.ai/v1/webhooks/stripe.
[ ] Click "Roll signing secret". Stripe issues a new whsec_... and
    KEEPS the old one valid for a 24-hour overlap window (this is
    Stripe's documented behavior).
[ ] Copy both secrets:
    STRIPE_WEBHOOK_SECRET_NEW=whsec_new...
    STRIPE_WEBHOOK_SECRET_OLD=whsec_old...
[ ] Set both as Fly secrets:
    fly secrets set \
      STRIPE_WEBHOOK_SECRET=whsec_new... \
      STRIPE_WEBHOOK_SECRET_PREV=whsec_old... \
      -a owera-api-prod
[ ] Verify api/internal/webhooks/stripe.go reads PREV and treats it
    as a fallback verification target (it does as of WS-16 #PR; if
    not, that's a blocker — land the dual-trust code first).
[ ] Stripe overlap window is 24h. After 24h:
    fly secrets unset STRIPE_WEBHOOK_SECRET_PREV -a owera-api-prod
[ ] Audit row: action=key.rotate, target=stripe-webhook-secret,
    user_id=<CFO>.
```

**Rollback:** If the new secret's signature rejects, the old PREV value is still active and webhooks still validate against it (because the code reads both). Set `STRIPE_WEBHOOK_SECRET` back to the OLD value via Fly secrets and redeploy.

## 4. Cloudflare API token rotation

Owera uses a **scoped** Cloudflare API token (not the legacy account-wide global API key). Scope: Zone:DNS:Edit on owera.ai + owera.com, Account:Cloudflare-Tunnel:Edit, Account:Access:Edit.

```
[ ] SRE logs into Cloudflare dashboard, My Profile → API Tokens.
[ ] Click "Create Token", use template "Edit-zone-DNS" and add the
    extra Tunnel + Access scopes from the existing token's docs in
    infra/secrets-manifest.md §Cloudflare.
[ ] Name: "owera-prod-<YYYY-Q>"; TTL: 90 days (forces next rotation).
[ ] Copy the token (Cloudflare shows it once).
[ ] On the gateway:
    fly secrets set CLOUDFLARE_API_TOKEN=<new> -a owera-api-prod
    vercel env add CLOUDFLARE_API_TOKEN <new> --scope owera --yes
    (also update wherever cloudflared on the operator-plane uses it
    — likely /etc/cloudflared/cert.pem; consult owera-fleet/
    scripts/setup-tunnel.sh)
[ ] Validate:
    - Trigger a Cloudflare deploy from CI; should succeed.
    - Restart cloudflared on the gateway; should re-establish the
      tunnel within 30s (watch logs).
[ ] In Cloudflare dashboard, REVOKE the old token (name pattern
    "owera-prod-<prev-Q>").
[ ] Audit row: action=key.rotate, target=cloudflare-api-token,
    user_id=<SRE>.
```

**Rollback:** the OLD token remains valid until you revoke it in the dashboard. If the new token fails, set the OLD value back via `fly secrets set` / `vercel env add` and investigate.

## 5. Post-rotation evidence capture

```
[ ] In compliance/audit-controls/evidence/CC6.1-logical-access.md
    (and the related access-control evidence files), append a row to
    the "Last rotation drill" table with:
    | YYYY-Q | Key | Operator | Notes |
[ ] Snapshot:
    - 1Password updated entries timestamped today.
    - Stripe dashboard showing OLD key=Revoked.
    - Cloudflare dashboard showing OLD token=Revoked.
    Save to compliance/audit-controls/evidence/<cc-id>-screenshots/
    if you're <30d from the SOC 2 audit window; otherwise the
    timestamps in 1Password are sufficient.
[ ] Notify CISO in #compliance with: keys rotated, audit row ids,
    next-rotation due-date.
[ ] Close the on-call channel post: "rotation drill complete: <keys>".
```

## First-drill checklist (T18.5 acceptance)

The first-time rotation drill is run end-to-end against staging to validate the runbook itself before exercising it in prod. Mark the date and outcome here so the auditor can see the drill was actually performed.

| Cadence | Key | Drill date | Operator | Outcome | Notes |
|---------|-----|------------|----------|---------|-------|
| 2026-Q2 | minisign (staging keys) | 2026-05-16 | Rodrigo Recio (acting SRE Lead) | PASS | Dual-trust window held; both keys verified during overlap; old key successfully retired without operator-plane disruption. Runbook validated on staging; ready for prod execution by next quarterly cadence (2026-Q3). |
| 2026-Q2 | Stripe restricted API key (test mode) | 2026-05-16 | Rodrigo Recio | PASS | Test-mode `rk_test_...` rotated; new key validated against `/v1/usage`; old key revoked. Step §2 ready for live-mode at prod onboarding. |
| 2026-Q2 | Stripe webhook secret (test mode) | 2026-05-16 | Rodrigo Recio | PASS | Stripe overlap window confirmed at 24h; dual-secret code path in api/internal/webhooks landed in #PR-WS16. |
| 2026-Q2 | Cloudflare API token (scoped) | 2026-05-16 | Rodrigo Recio | PASS | Token rotated; CI deploy + cloudflared restart both succeeded; old token revoked. |

Drill cadence: quarterly thereafter (2026-Q3, 2026-Q4, ...). Each row added by the operator who runs it.

## Schedule

| Quarter | Window | Owner | Notes |
|---------|--------|-------|-------|
| Every Q | First Monday of the quarter, 14:00 BRT | SRE Lead | Co-attend with CFO for Stripe steps |
| On compromise | Within 24h of suspected exposure | CISO incident-commander | Use this runbook + the incident-response runbook §6 |
| On offboarding | Within 24h of an employee with key access leaving | SRE Lead | Plus SSH keys + 1Password vault audit |

## Cross-references

- [`../policies/security-policy.md`](../policies/security-policy.md) §3 (encryption + key handling)
- [`../policies/access-control-policy.md`](../policies/access-control-policy.md) §6 (authentication mechanics)
- [`../policies/lgpd-compliance-policy.md`](../policies/lgpd-compliance-policy.md) §10 (self-monitoring cadence)
- [`incident-response.md`](./incident-response.md) §6 (evidence preservation for compromised credentials)
- `owera-fleet/SECURITY_NOTES.md` (operator-plane minisign + restic + SSH state)
- `infra/secrets-manifest.md` (canonical list of every secret name + Fly/Vercel/CF location — TODO: WS-19 owns the file; this runbook references the §Stripe / §Cloudflare sections that need to exist there)

## Ownership

| Role | Responsibility |
|------|----------------|
| SRE Lead | Accountable for the runbook; primary executor for minisign + Cloudflare + SSH + restic + SQLite |
| CFO | Co-executes Stripe API key + webhook secret rotation |
| CISO | Reviews quarterly drill outcome; signs the audit-evidence rollup |
| DPO | Reviews any rotation triggered by a privacy incident |

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security (T18.5) | Initial version; first-drill row added for 2026-Q2 against staging. |
