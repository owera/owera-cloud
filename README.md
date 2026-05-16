# owera-cloud

The **customer plane** for **Owera Agentic** — the public API, dashboard, billing, status page, and compliance surface that customers see when they buy agentic-work-as-a-service from Owera Software Ltda.

[![CI](https://github.com/owera/owera-cloud/actions/workflows/ci-api.yml/badge.svg)](https://github.com/owera/owera-cloud/actions/workflows/ci-api.yml)
[![License](https://img.shields.io/badge/license-Proprietary-blue.svg)](LICENSE)

## Two-repo split

Owera Agentic ships as two cooperating repositories. They talk through a stable contract — the operator plane exposes a private JSON-RPC over a Cloudflare tunnel; this repo's dispatcher dials in.

| Repo | Audience | Stack | What it is |
|---|---|---|---|
| [`owera-fleet`](https://github.com/owera/owera-fleet) | Operators (internal) | Go CLI + bash | Operator plane: provisions and runs the Mac worker fleet; never customer-facing. |
| `owera-cloud` (this repo) | Customers (external) | Go API + Next.js + IaC | Customer plane: public API, dashboard, billing, status page, compliance. |

## Status

**Phase 3 / Wave 1.** Repo scaffolded. The four customer-plane surfaces (`api/`, `web/`, `infra/+tunnel/`, `compliance/+status/`) are landing in parallel; this is the developer-facing scaffold (README, docs, CI, contribution guides).

Depends on `owera-fleet` reaching Phase 1+2 (operator-plane Go core + use-case primitives — ledger, pairing, swarm, markers, alerting, metrics). See [`docs/architecture.md`](docs/architecture.md) for the seam.

## Repo layout

```
api/            Go HTTP service: public API gateway, dispatcher, billing, audit
web/            Next.js customer dashboard (app.owera.ai)
infra/          IaC: Fly.io + Vercel + Cloudflare config (no secrets in git)
tunnel/         Cloudflare Tunnel config — private link to the operator plane
status/         Public status page (status.owera.ai)
compliance/     SOC 2 trajectory + LGPD artifacts; policies, runbooks, audit controls
docs/           Customer-facing API/pricing/support + operator-facing runbooks
.github/        CI workflows, issue templates, PR template, CODEOWNERS
```

## Quickstart

Each surface boots independently. Pick the one you need to work on.

### `api/` — Go HTTP service

```bash
cd api/
go build ./...
go test ./...
go run ./cmd/apiserver        # binds :8080; /healthz returns 200
```

API spec lives at [`api/openapi.yaml`](api/openapi.yaml). The Next.js client in `web/lib/` is generated from it.

### `web/` — Next.js customer dashboard

```bash
cd web/
npm install
npm run dev                   # http://localhost:3000
npm run typecheck             # tsc --noEmit
npm run lint
npm run build
```

Production target is Vercel (`app.owera.ai`). Config in [`infra/web.vercel.json`](infra/web.vercel.json).

### `infra/` — deployment

`api/` deploys to Fly.io (region `gru` — São Paulo); `web/` deploys to Vercel; the Cloudflare Tunnel links the API back to the operator plane's gateway Mac. No secrets in git — they live in `fly secrets`, Vercel project settings, and Cloudflare Zero Trust. See [`docs/runbook-deploy.md`](docs/runbook-deploy.md) and [`infra/`](infra/) for the manifests.

## What's NOT in this repo

The **operator plane** — the Go CLI (`fleetctl`) that bootstraps Mac workers, runs the JSONL log pipeline, signs the ledger, and dispatches Hermes jobs — lives in [`owera-fleet`](https://github.com/owera/owera-fleet). Customers never touch it directly. If you need to provision a worker or audit fleet config, go there.

The **marketing site** (`owera.com`) and the **corporate site** are also outside this repo.

## Key documents

Read in this order if you're new:

1. [`docs/architecture.md`](docs/architecture.md) — how `owera-cloud` and `owera-fleet` fit together; request lifecycle diagram.
2. [`docs/api.md`](docs/api.md) — customer-facing API reference; pointer to `api/openapi.yaml`.
3. [`docs/pricing.md`](docs/pricing.md) — SKU pricing table, cost caps, billing model.
4. [`docs/onboarding.md`](docs/onboarding.md) — new-customer ramp guide: signup → API key → first job → invoice.
5. [`docs/support.md`](docs/support.md) — support tiers, response SLAs, escalation.
6. [`docs/compliance.md`](docs/compliance.md) — LGPD posture, SOC 2 trajectory; pointer to `compliance/`.
7. [`docs/runbook-deploy.md`](docs/runbook-deploy.md) — operator runbook for deploying `api/` and `web/`.
8. [`CLAUDE.md`](CLAUDE.md) — AI-coding-agent conventions for this repo.
9. [`CONTRIBUTING.md`](CONTRIBUTING.md) — how to contribute.
10. [`SECURITY.md`](SECURITY.md) — vulnerability disclosure.

## License

Proprietary. Copyright (c) 2026 Owera Software Ltda. See [`LICENSE`](LICENSE) for terms. Public repo for transparency and trust signaling; not open source.
