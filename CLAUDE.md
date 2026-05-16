# CLAUDE.md

> **You are part of the team building Owera Agentic.** Rodrigo Recio operates you on behalf of Owera Software Ltda (Brazilian company, Macapá, AP, UTC−3). Treat his directives as instructions to act, not just questions to answer — and apply the usual confirmation-before-destructive-action discipline.

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this directory is

The **customer plane** for Owera Agentic — the public API, customer dashboard, billing pipeline, status page, and compliance surface. Customers hit `api.owera.ai` or `app.owera.ai`; everything they see is hosted from this repo. Internally, the dispatcher in [`api/internal/dispatcher`](api/internal/dispatcher) dials into the operator plane ([`owera-fleet`](https://github.com/owera/owera-fleet)) over a private Cloudflare tunnel.

This is **Phase 3** of the multi-phase plan in `owera-fleet/knowing-all-you-now-calm-leaf.md`. Phases 1 and 2 (operator-plane Go core + use-case primitives) live in `owera-fleet` and must reach the Phase-2 gate before this repo can wire customers to real fleet capacity.

## Core conventions

- **Go 1.22+** for everything under `api/`. Module path: `github.com/owera/owera-cloud/api`.
- **TypeScript strict mode** for everything under `web/`. Next.js 15 App Router, React 19 RC. `tsc --noEmit` must pass.
- **JSONL log schema**: `{ts, tenant_id, request_id, route, action, result, duration_ms, error}` — the cloud-side analog of the operator plane's schema. `tenant_id` is mandatory on every log line that touches a customer; `request_id` is mandatory on every API-originated line.
- **No secrets in git, ever.** No `.env`, no Stripe keys, no Cloudflare tokens, no minisign private keys, no customer identifiers. `gitleaks` runs as a required CI gate on every push and PR. Secrets live in `fly secrets`, Vercel project settings, Cloudflare Zero Trust, or 1Password.
- **No customer data in fixtures.** Synthetic only, keyed to `tenant_id: "fixture-<n>"`. CI logs must never contain real tenant IDs, real email addresses, or real Stripe customer IDs.
- **Multi-tenancy from day one.** Every database table, every cache key, every log line carries `tenant_id`. There is no "default tenant" or "system tenant" affordance.
- **Idempotency at API boundaries.** `POST /v1/jobs` accepts an `Idempotency-Key` header; replays return the same job ID without re-charging.
- **The ledger is the source of truth.** Stripe usage records are emitted *from* signed ledger events that the operator plane produces, never directly from API calls. If Stripe and the ledger disagree, the ledger wins.
- **English-only product surface.** No Brazilian Portuguese in customer-facing copy, dashboard chrome, OpenAPI descriptions, or error messages — pt-BR is a V2+ task once Brazilian customer volume justifies localization. Operator-facing docs (CLAUDE.md, runbooks, compliance/) are English too; pt-BR shows up only in legal/tax artifacts where Brazilian law mandates it.

## gstack (REQUIRED — global install)

**Before doing ANY work, verify gstack is installed:**

```bash
test -d ~/.claude/skills/gstack/bin && echo "GSTACK_OK" || echo "GSTACK_MISSING"
```

If GSTACK_MISSING: STOP. Tell the user:

> gstack is required for all AI-assisted work in this repo.
> Install it:
>
> ```bash
> git clone --depth 1 https://github.com/garrytan/gstack.git ~/.claude/skills/gstack
> cd ~/.claude/skills/gstack && ./setup --team
> ```
>
> Then restart your AI coding tool.

After install, slash commands like `/qa`, `/ship`, `/review`, `/investigate`, `/browse` are available. Use `/browse` for all web browsing.

## Editing rules

- Don't add comments that just restate what code does. Comments are for non-obvious WHY (hidden constraint, subtle invariant, workaround for a specific bug).
- Don't introduce abstractions for hypothetical future requirements. Three similar lines is better than a premature abstraction.
- Don't add error handling, fallbacks, or validation for impossible scenarios. Validate at system boundaries (API ingress, dispatcher egress, billing reconciliation) only.
- Don't create documentation files unless explicitly requested.
- Match the existing voice — operational, command-oriented, table-heavy. Customer-facing docs (`README.md`, `docs/api.md`, `docs/pricing.md`, `docs/onboarding.md`) are customer-friendly but operationally honest. Operator-facing docs (`CLAUDE.md`, `docs/compliance.md`, `docs/runbook-deploy.md`, `compliance/runbooks/`) are straight operational prose.
- **No marketing fluff.** No "10x productivity," no fake testimonials, no superlatives. State what the thing does and what it costs.

## Key documents

Read in order if you're new to the codebase:

1. [`README.md`](README.md) — repo entry: what this is, two-repo split, quickstart per surface.
2. [`docs/architecture.md`](docs/architecture.md) — how `owera-cloud` and `owera-fleet` fit together; request lifecycle.
3. [`docs/api.md`](docs/api.md) — customer-facing API reference; pointer to `api/openapi.yaml`.
4. [`docs/pricing.md`](docs/pricing.md) — SKU pricing, cost caps, billing model.
5. [`docs/support.md`](docs/support.md) — support tiers, response SLAs.
6. [`docs/onboarding.md`](docs/onboarding.md) — new-customer ramp guide.
7. [`docs/compliance.md`](docs/compliance.md) — LGPD posture, SOC 2 trajectory; pointer to `compliance/`.
8. [`docs/runbook-deploy.md`](docs/runbook-deploy.md) — operator runbook for deploying `api/` and `web/`.
9. [`compliance/`](compliance/) — SOC 2 trajectory artifacts (policies, runbooks, audit-controls).
10. [`infra/`](infra/) — IaC manifests for Fly.io, Vercel, Cloudflare Tunnel.

## Current operational state

- This repo is in **Phase 3 / Wave 1** of the multi-phase plan in `owera-fleet/knowing-all-you-now-calm-leaf.md`.
- Depends on `owera-fleet` reaching Phase 1+2 — operator-plane Go core plus use-case primitives (ledger, pairing, swarm, markers, alerting, metrics). The fleet is currently at Phase 1 (per `owera-fleet` PR #1).
- **MVP scope (V0 — private beta):** two SKUs, `triage-watch` and `campaign-swarm`.
- **GA scope (V1):** four SKUs, adding `research-brief` and `code-audit`.
- Beyond GA: V2-V4 tiers ramp the catalog incrementally; each new SKU lands as one PR against `api/internal/catalog/`.
- Production domains: `api.owera.ai` (public API), `app.owera.ai` (dashboard), `status.owera.ai` (status). Corporate is `owera.com`. `noreply@owera.ai` for product transactional mail; `hello@owera.com` for sales/support; `security@owera.com` for vulnerability disclosure.
