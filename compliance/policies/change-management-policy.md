# Change management policy

**Scope:** every change to production systems — source code, infrastructure config, secrets, DNS, third-party integrations.

## 1. Principles

1. **No direct prod commits.** Every change flows through a PR with at least one human reviewer.
2. **Staging precedes production.** Every change is observed in staging before promotion, except in declared incident response (break-glass).
3. **Rollback first, debug second.** When production breaks, the first move is to revert; root-causing happens in the post-incident phase.
4. **Idempotency.** Infrastructure changes are idempotent — re-running yields `no-change`, not failure.

## 2. PR review requirements

| Change type | Reviewers required | Additional gates |
|-------------|--------------------|--------------------|
| Application code (api, web) | 1 engineer | CI green: tests, lint, typecheck, gitleaks, gosec/npm audit |
| Infrastructure (infra/, tunnel/) | 1 SRE | CI green: `python3 -c "import yaml; ..."` against every YAML; `tomllib.load` against every TOML |
| Compliance docs (compliance/) | 1 CISO **or** 1 designate | Version block updated; cross-references resolve |
| Secrets manifest changes | 1 SRE + 1 CISO | Two-person review for any secret addition or removal |
| Emergency hotfix in active incident | 1 reviewer (in-thread approval acceptable) | Documented in incident timeline; backfilled as proper PR within 24h |

PR reviewers may not be the author. AI-assisted reviews (`/codex review`, `/review`) are encouraged but do NOT substitute for a human reviewer.

## 3. CI gates

The following are blocking checks before merge:

| Gate | Tool | Path |
|------|------|------|
| Secret scan | `gitleaks` | Repo-wide |
| Go static analysis | `gosec`, `staticcheck`, `go vet` | `api/` |
| Go tests | `go test ./...` | `api/` |
| TS lint/typecheck | `eslint`, `tsc --noEmit` | `web/` |
| Next.js build | `next build` | `web/` |
| YAML parse | `python3 -c "import yaml; yaml.safe_load(open(f))"` | All `*.yaml`, `*.yml` |
| TOML parse | `python3 -c "import tomllib; tomllib.load(open(f,'rb'))"` | All `*.toml` |
| Markdown link check | `lychee` or equivalent | All `*.md` |

PRs that fail any gate cannot merge without an explicit override commit from an SRE Lead or CISO, which is itself a Sev3 audit event.

## 4. Staging → production promotion

```
PR opened → CI green → human review → merge to main → autodeploy to staging
  → staging smoke tests green (10 min observation) → manual promote to prod
```

- **Staging environments**: `api-staging.owera.ai`, `app-staging.owera.ai`. Same Fly + Vercel setup as production, scaled to zero between deploys.
- **Smoke tests**: a minimal e2e suite that hits `/healthz`, exercises one customer flow end-to-end, and posts the result to `#deploys`.
- **Promote**: `gh workflow run promote-to-prod.yml --ref main` — runs only after staging is green for 10 min.
- **Observation window**: 30 min of prod observability after promote before the on-call rotation hands back to normal.

## 5. Rollback procedure

Per service:

| Service | Rollback mechanism | RTO |
|---------|---------------------|-----|
| api (Fly) | `fly deploy --image <previous-image-tag>` | 5 min |
| web (Vercel) | Vercel dashboard → previous deploy → Promote to Production | 1 min |
| DNS (Cloudflare) | Revert `dns.cloudflare.yaml` + manual reconcile (or `fleetctl dns sync` once it ships) | 5 min + TTL |
| Tunnel (cloudflared) | Revert `tunnel/config.example.yml` and `launchctl kickstart` | 1 min |
| Compliance docs | `git revert` + PR | n/a (no runtime effect) |

Rolling back is **not** a Sev3 event. It is the correct first move. The Sev3 (or higher) is whatever made the rollback necessary, and that gets its own post-mortem.

## 6. Emergency change (break-glass)

During a declared incident, the IC may authorize an emergency change that skips staging:

1. IC declares the emergency in `#incident-<n>` with rationale.
2. Change is committed to a hotfix branch, one reviewer approves in-thread.
3. Deployed directly to prod.
4. Within 24h: a proper PR is opened mirroring the hotfix, with the full review process, and is merged. If the proper PR reveals issues, a follow-up hotfix is needed — the emergency commit is not "blessed by being in main", it's still under review.

Emergency changes are logged to the audit-log WORM store with the IC's name, the incident id, and the diff.

## 7. Pre-merge AI review

PRs benefit from `/codex review` or `/review` (gstack) as a first-pass quality check. The AI review:

- Does NOT replace human review.
- Does NOT count as the required reviewer.
- IS useful for catching missing tests, naming inconsistencies, surface-level bugs.

Authors are encouraged but not required to run AI review before requesting human review.

## Ownership

| Role | Responsibility |
|------|----------------|
| SRE Lead | Accountable for the policy; owns CI configuration |
| CISO | Approves overrides; reviews compliance-doc changes |
| Engineering team | Adherence in daily practice |

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
