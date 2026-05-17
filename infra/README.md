# infra/

Operator runbook for the Owera cloud plane. This directory holds the **declarative configuration** for every service the customer-facing stack runs on. Actual secrets live in 1Password, fly.io secrets, and Vercel project settings — never here.

## What deploys where

| Service | Platform | Region(s) | Config file |
|---------|----------|-----------|-------------|
| `owera-agentic-api` (api/, Go) | Fly.io | gru (BR primary) | `api.fly.toml` |
| `owera-web` (web/, Next.js) | Vercel | gru1, iad1 | `web.vercel.json` |
| Operator-plane tunnel | Cloudflare Named Tunnel | edge | `tunnel.cloudflare.yaml` |
| DNS zone `owera.ai` | Cloudflare | edge | `dns.cloudflare.yaml` |
| Public status page | Vercel (separate project) | iad1, gru1 | `status.vercel.json` (app at `../status/app/`) |

## Deploy procedures

### API (Fly)

```bash
# From repo root, with FLY_API_TOKEN exported (or `fly auth login` interactive)
fly deploy --config infra/api.fly.toml --remote-only
fly status -a owera-agentic-api
curl https://api.owera.ai/healthz   # expect 200
```

First-time setup:

```bash
fly apps create owera-agentic-api --org owera-software
fly secrets set STRIPE_SECRET_KEY=... -a owera-agentic-api   # see secrets-manifest.md
# ... repeat for every secret marked `[ ]` in the manifest
fly deploy --config infra/api.fly.toml --remote-only
```

### Web (Vercel)

```bash
# Vercel autodeploys main from GitHub; manual deploys via:
npx vercel --prod --local-config infra/web.vercel.json
```

First-time setup (operator runs once):

1. Sign in to <https://vercel.com> with the `owera-software` org.
2. Click **Add New → Project**, import this GitHub repo.
3. Set **Root Directory** to `web`.
4. Set **Framework Preset** to "Next.js" (auto-detected from the root).
5. In **Settings → General → Build & Development**, point the Vercel config file at `../infra/web.vercel.json` (or paste the equivalent fields into the dashboard).
6. In **Settings → Environment Variables**, populate every `vercel`-store secret from `secrets-manifest.md` against the `Production`, `Preview`, and `Development` scopes.
7. In **Settings → Domains**, add `app.owera.ai` and follow Vercel's CNAME instructions — the `dns.cloudflare.yaml` entry for `app.owera.ai` already points at `cname.vercel-dns.com` (unproxied, so Vercel manages TLS).
8. **Git Integration** is enabled by default: pushes to `main` deploy to production; every other branch and every PR produces a preview deploy under `<branch>-owera-web.vercel.app`.

To temporarily disable PR previews (e.g., during an incident), unset `git.deploymentEnabled.main` in Vercel's project settings — `infra/web.vercel.json` is the source of truth but Vercel reads the dashboard value first.

### Status page (Vercel — separate project)

`status.owera.ai` is a **separate Vercel project** so it can outlive the dashboard during an incident. Same first-time procedure as above, with these differences:

1. **Root Directory** = `status/app`.
2. Vercel config = `../../infra/status.vercel.json`.
3. Env var: `NEXT_PUBLIC_SNAPSHOT_URL` — the public URL where the operator plane publishes the latest health snapshot JSON (Cloudflare R2 / S3 bucket, no auth). Defaults to `https://snapshots.owera.ai/health/latest.json` if unset.
4. Domain: `status.owera.ai` (DNS already in `dns.cloudflare.yaml`).

The status page reads its data from the snapshot URL on every request and polls every 30s from the client. It is intentionally decoupled from `api.owera.ai` — if the API is down the page must still render, with the gateway component flipped red. The acceptance test for **T19.4** sets `NEXT_PUBLIC_FORCE_INCIDENT=1` on a preview deploy and verifies the page paints "down" within 60s.

### DNS (Cloudflare)

Until `fleetctl dns sync` lands, DNS is reconciled by hand against `dns.cloudflare.yaml`. Procedure:

1. Open the Cloudflare dashboard → owera.ai zone → DNS.
2. Diff the live records against `dns.cloudflare.yaml`. Any divergence is a bug — fix it.
3. Update both sides in one commit + dashboard session.

### Tunnel

See `../tunnel/README.md`. The runtime config that lives on the Mac mini is in `../tunnel/`; this `infra/tunnel.cloudflare.yaml` is the declarative spec the operator copies into `/usr/local/etc/cloudflared/config.yml`.

## Secret rotation

Authoritative: [`secrets-manifest.md`](./secrets-manifest.md). Every secret has a name, store, owner, and rotation cadence. Rotation procedure (overlap-window for the secrets that support it) is documented at the bottom of that file.

## Disaster recovery

RTO / RPO targets and restore procedures: [`disaster-recovery.md`](./disaster-recovery.md). Tabletop drills quarterly; live restore exercises every Q1/Q3.

## File index

| File | What it is |
|------|------------|
| [`api.fly.toml`](./api.fly.toml) | Fly app config for the Go API |
| [`web.vercel.json`](./web.vercel.json) | Vercel framework config for the Next.js dashboard |
| [`status.vercel.json`](./status.vercel.json) | Vercel framework config for the status page (separate project) |
| [`tunnel.cloudflare.yaml`](./tunnel.cloudflare.yaml) | Cloudflare tunnel descriptor (operator-plane RPC ingress) |
| [`dns.cloudflare.yaml`](./dns.cloudflare.yaml) | Declarative DNS for `owera.ai` |
| [`secrets-manifest.md`](./secrets-manifest.md) | Every secret name + store + rotation cadence |
| [`disaster-recovery.md`](./disaster-recovery.md) | RTO/RPO + restore procedures |
