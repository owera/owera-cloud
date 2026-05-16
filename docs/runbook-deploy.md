# Runbook — deploy

> **Audience: operators.** How to push `api/` and `web/` to production, how to roll back, and how to rotate the secrets each surface depends on. The IaC manifests live in [`../infra/`](../infra/); this runbook is the operational layer on top of them.

## Surfaces and targets

| Surface | Path | Target | Region | Public URL |
|---|---|---|---|---|
| API gateway | [`../api/`](../api/) | Fly.io | `gru` (São Paulo) primary | `https://api.owera.ai` |
| Dashboard | [`../web/`](../web/) | Vercel | `gru1` + `iad1` | `https://app.owera.ai` |
| Status page | [`../status/`](../status/) | Vercel (or equivalent) | `gru1` + `iad1` | `https://status.owera.ai` |
| Tunnel | [`../tunnel/`](../tunnel/) | Cloudflare Tunnel | Anycast | private |

## Prerequisites — one-time

```bash
# Install the CLIs
brew install flyctl
npm install -g vercel
brew install cloudflared

# Authenticate
flyctl auth login
vercel login
cloudflared login
```

You also need:

- 1Password access to the **Owera Agentic** vault for secret rotation.
- GitHub permissions on `owera/owera-cloud` (push to release tags, read Actions).
- Stripe dashboard access at restricted-admin scope.

## Deploy — `api/`

`api/` deploys via Fly.io. The image is built from [`../api/Dockerfile`](../api/) (provided by the api/ agent). The Fly config lives at [`../infra/api.fly.toml`](../infra/api.fly.toml).

### Normal deploy

```bash
cd api/
flyctl deploy --config ../infra/api.fly.toml --remote-only
flyctl status -a owera-agentic-api
curl -fsS https://api.owera.ai/healthz   # expect 200 + {"ok": true}
```

`fly deploy` runs immutable releases — each release gets a sequential number visible in `fly releases`.

### Roll back

```bash
flyctl releases -a owera-agentic-api               # find the prior release number
flyctl deploy -a owera-agentic-api --image <ref>   # redeploy that release's image
```

Or use the Fly dashboard's one-click rollback. Either way, watch `/healthz` recover before doing anything else.

### Rotate API secrets

Stripe keys, operator-plane verification pubkey, identity provider secrets, etc.:

```bash
flyctl secrets list -a owera-agentic-api          # current keys
flyctl secrets set STRIPE_API_KEY="$(op read ...)" -a owera-agentic-api
flyctl secrets unset OLD_KEY -a owera-agentic-api
```

Setting a secret triggers a new release automatically. The authoritative inventory of secrets lives at `infra/secrets-manifest.md` (owned by the infra agent).

## Deploy — `web/`

`web/` deploys via Vercel from the `main` branch. Connected repo: `owera/owera-cloud`. Project name: `owera-agentic-web` (Vercel project alias `app.owera.ai`).

### Normal deploy

Push to `main`. Vercel auto-deploys; preview deploys land on every PR. Manual deploy:

```bash
cd web/
vercel --prod
```

### Roll back

```bash
vercel rollback --token "$VERCEL_TOKEN"  # opens the rollback UI for app.owera.ai
```

Or use the Vercel dashboard's **Deployments** view → **Promote to production** on a prior deploy.

### Rotate web secrets

`NEXT_PUBLIC_*` env vars and server-side secrets live in Vercel project settings. Rotate via the Vercel dashboard or:

```bash
vercel env rm NEXT_PUBLIC_API_BASE production
vercel env add NEXT_PUBLIC_API_BASE production
```

Server-side rotations require a redeploy to take effect.

## Deploy — Cloudflare Tunnel

The tunnel is the private link between `api/` (Fly) and `fleetctl serve` on the operator plane's gateway Mac in Macapá. Config lives at [`../infra/tunnel.cloudflare.yaml`](../infra/tunnel.cloudflare.yaml) and on the gateway at `~/.cloudflared/`.

### Normal change

```bash
# On the gateway Mac:
cloudflared tunnel ingress validate
cloudflared tunnel run owera-fleet
```

Cloudflare config changes (DNS, access policy) happen in the Cloudflare dashboard; we mirror state in [`../tunnel/`](../tunnel/) for diffability.

### Rotate the tunnel credential

```bash
cloudflared tunnel create owera-fleet-new
# Update infra/tunnel.cloudflare.yaml with the new tunnel ID
# Update DNS to point at the new tunnel
cloudflared tunnel delete owera-fleet     # old tunnel
```

This is a planned cutover; do it during a maintenance window with API in `503` mode.

## Maintenance windows

| Type | Cadence | Notice | Impact |
|---|---|---|---|
| Fly.io node migration | As needed by Fly | None — Fly handles transparently | None expected, brief connection blips possible |
| Operator-plane Hermes upgrade | When `owera-fleet` ships a version bump | ≥48 h on status page | Jobs queue but don't dispatch for the upgrade window |
| Tunnel credential rotation | Quarterly | ≥48 h on status page | 5-10 min of `503` |
| Major dependency upgrade (Next.js, Go) | Per major | None — preview deploys verify | None expected |

## Pre-deploy checklist

Before any production deploy that touches customer-observable behavior:

- [ ] CI green on `main` (or the deploy ref). `ci-api.yml`, `ci-web.yml`, `codeql.yml`, `secret-scan.yml` all passing.
- [ ] [`CHANGELOG.md`](../CHANGELOG.md) updated; [`VERSION`](../VERSION) bumped if appropriate.
- [ ] PR template's **Compliance impact** filled out honestly. If the change affects SOC 2 controls or LGPD data flows, compliance review signed off.
- [ ] Status page maintenance window posted ≥48 h ahead for breaking changes.
- [ ] Roll-forward plan and roll-back plan written down in the PR description.

## Post-deploy verification

```bash
curl -fsS https://api.owera.ai/healthz
curl -fsS https://app.owera.ai           # 200 + HTML
curl -fsS https://status.owera.ai        # 200 + HTML
flyctl logs -a owera-agentic-api | head -50
```

Spot-check the dashboard with a real tenant session. Watch the JSONL log stream on Fly for at least 5 minutes after deploy.

## Escalation

If a deploy is materially broken and rollback is failing too:

1. Take the API offline cleanly — `flyctl scale count 0 -a owera-agentic-api`. Customers see `503` immediately, which is correct behavior.
2. Update the status page to S1 — incident open, ETA unknown.
3. Page Rodrigo. Number is in 1Password under **Owera Agentic / On-call**.
4. Write the post-mortem within 5 business days. Publish on `status.owera.ai`.
