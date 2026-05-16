# Disaster recovery

Recovery-time and recovery-point objectives per tier, plus the restore procedures that achieve them. Tested quarterly via tabletop exercises (every Q1/Q3 actual restore drill into a staging account).

## Targets

| Tier | Component | RTO | RPO | Tested |
|------|-----------|-----|-----|--------|
| Cloud / API | `owera-agentic-api` on Fly | 30 min | 5 min | `[ ]` Q3 2026 (target) |
| Cloud / Web | `owera-web` on Vercel | 15 min | 0 (stateless) | `[ ]` Q3 2026 (target) |
| Cloud / Status | `status.owera.ai` on Vercel | 60 min | 5 min | `[ ]` Q4 2026 (target) |
| Cloud / DNS | Cloudflare zone `owera.ai` | 5 min | 0 (declarative) | `[ ]` Q3 2026 (target) |
| Operator plane | Mac mini gateway + workers | 4 h | 24 h | `[ ]` Q4 2026 (target) — see `owera-fleet/docs/operation.md` |
| Tunnel | Cloudflare Named Tunnel | 15 min | 0 (stateless) | `[ ]` Q3 2026 (target) |

The cloud plane is genuinely stateless or near-stateless — all durable state lives either in (a) third-party SaaS (Stripe, Clerk, Cloudflare) which carries its own DR contract, or (b) the operator plane on the Mac mini, which has its own backup pipeline (restic SFTP, daily, restore-tested).

## Restore procedures

### Cloud / API (Fly)

1. Confirm scope: is the app down (`fly status -a owera-agentic-api`), the region down (`fly status -a owera-agentic-api -r gru`), or the platform down (`status.fly.io`)?
2. If platform-wide, escalate to Fly support; if regional, fail over to a secondary region (provision with `fly scale count 2 --region iad`). The api is stateless so a fresh region start carries no data loss.
3. If app-level, redeploy from the last green main: `gh workflow run deploy-api.yml --ref <last-green-sha>`.
4. Verify health: `curl https://api.owera.ai/healthz` returns 200 with a 5-second budget.
5. Verify tunnel reachability: `curl https://api.owera.ai/internal/operator-ping` (admin-only endpoint) round-trips to the operator plane.
6. Post-incident: capture timeline in `compliance/runbooks/incident-response.md` and write a Sev-classified post-mortem.

### Cloud / Web (Vercel)

1. Vercel deploys are atomic and instantly revertable: open the Vercel dashboard → `owera-web` → Deployments → pick the last green deploy → "Promote to Production". RTO is bounded by DNS TTL (the Vercel apex is proxied via Cloudflare with auto TTL, ~60s).
2. If Vercel itself is degraded, the `app.owera.ai` CNAME can be repointed to a static maintenance page hosted on Cloudflare Pages (provisioned but parked; deploy hook in `1password / Owera / Cloud Plane / Vercel maintenance-page deploy hook`).
3. Verify: `curl -I https://app.owera.ai` returns 200 from a Cloudflare edge close to the user.

### Cloud / DNS (Cloudflare)

The authoritative declarative spec is `dns.cloudflare.yaml`. Restoration is **idempotent**:

1. Authenticate to Cloudflare with the rotation API token (`CLOUDFLARE_API_TOKEN` from 1Password).
2. Run `fleetctl dns sync --zone owera.ai --spec infra/dns.cloudflare.yaml` (TODO: implement; until then, manually reconcile via the dashboard).
3. Cloudflare's per-record propagation is ~60s; cached resolvers may take up to the record TTL.

### Operator plane

Cross-references `owera-fleet/docs/operation.md` — that repo owns gateway restore. Summary:

1. The gateway's `~/.hermes/` is backed up nightly to restic SFTP (salmonpoke). Restore via `restic restore latest --target /Users/claw3/.hermes`.
2. Workers re-provision from the bootstrap script; their state is reconstructable from gateway state plus a few minutes of replay.
3. Tunnel credentials are restored from 1Password to `/usr/local/etc/cloudflared/<UUID>.json` (chmod 600) and the LaunchDaemon is reloaded.

### Tunnel

Cloudflared is stateless on the gateway. Failure modes:

- **cloudflared process crashed** — the LaunchDaemon (`tunnel/service.example.plist`) restarts it within 10s. RTO < 1 min.
- **Tunnel credentials lost or compromised** — provision a new tunnel: `cloudflared tunnel create owera-operator-plane-v2`, update `tunnel/config.example.yml` to reference the new UUID, update `dns.cloudflare.yaml` to point `internal-rpc.owera.ai` at the new tunnel CNAME, redeploy.

## Communications during DR

| Audience | Channel | Owner |
|----------|---------|-------|
| Internal team | Slack `#incidents` | Incident commander |
| Customers (live status) | status.owera.ai + Twitter/X | Comms lead |
| Affected customers (email) | SES, template `incident-update` | Customer success |
| Regulators (LGPD/GDPR) | If PII exposed, ANPD (BR) within 72h; relevant DPA in EU | CISO |

Communications cadence and templates: `../compliance/runbooks/incident-response.md`.
