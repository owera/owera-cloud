# infra/

Operator runbook for the Owera cloud plane. This directory holds the **declarative configuration** for every service the customer-facing stack runs on. Actual secrets live in 1Password, fly.io secrets, and Vercel project settings — never here.

## What deploys where

| Service | Platform | Region(s) | Config file |
|---------|----------|-----------|-------------|
| `owera-agentic-api` (api/, Go) | Fly.io | gru (BR primary) | `api.fly.toml` |
| `owera-web` (web/, Next.js) | Vercel | gru1, iad1 | `web.vercel.json` |
| Operator-plane tunnel | Cloudflare Named Tunnel | edge | `tunnel.cloudflare.yaml` |
| DNS zone `owera.ai` | Cloudflare | edge | `dns.cloudflare.yaml` |
| Public status page | Vercel (separate project) | iad1 | `../status/` |

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

First-time setup: link the GitHub repo at <https://vercel.com/new>, set the Root Directory to `web`, populate env-vars per `secrets-manifest.md`, and Vercel picks up `infra/web.vercel.json` automatically (or paste the equivalent fields into the dashboard).

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
| [`tunnel.cloudflare.yaml`](./tunnel.cloudflare.yaml) | Cloudflare tunnel descriptor (operator-plane RPC ingress) |
| [`dns.cloudflare.yaml`](./dns.cloudflare.yaml) | Declarative DNS for `owera.ai` |
| [`secrets-manifest.md`](./secrets-manifest.md) | Every secret name + store + rotation cadence |
| [`disaster-recovery.md`](./disaster-recovery.md) | RTO/RPO + restore procedures |
