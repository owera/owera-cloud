# Contributing

This is a public repository operated by Owera Software Ltda. Issues are welcome; PRs are accepted at maintainer discretion.

## Before opening an issue

- For **bug reports**, use the [`Bug report`](.github/ISSUE_TEMPLATE/bug_report.md) template. Include: which surface (`api/`, `web/`, `infra/`), the version (from [`VERSION`](VERSION) or the `X-Owera-Version` response header), repro steps, and the JSONL log line that surfaced the issue if you have one. Don't paste secrets — redact API keys, tenant IDs, and Stripe customer IDs first.
- For **feature requests**, use the [`Feature request`](.github/ISSUE_TEMPLATE/feature_request.md) template. Frame the customer outcome before the implementation. SKU-level requests (new SKU, new pricing tier, new SLA) should land via a separate proposal — open an issue first, get maintainer sign-off, then PR against `api/internal/catalog/`.
- For **paying-customer issues**, use the [`Customer issue`](.github/ISSUE_TEMPLATE/customer_issue.md) template. These auto-route to support and skip the public triage queue.
- For **security issues**, see [`SECURITY.md`](SECURITY.md) — disclose privately via `security@owera.com`, not via a public issue.

## Before opening a PR

- Discuss in an issue first for anything beyond a small fix.
- Follow the existing voice: operational, command-oriented, table-heavy. No fluff in comments or copy. Customer-facing docs stay customer-friendly without marketing-speak; operator-facing docs are straight operational prose.
- Match the JSONL log schema, multi-tenancy rules, and idempotency conventions documented in [`CLAUDE.md`](CLAUDE.md).
- CI must be green:
  - `go build ./...` and `go test -race ./...` in `api/` (`ci-api.yml`).
  - `npm run typecheck`, `npm run lint`, and `npm run build` in `web/` (`ci-web.yml`).
  - CodeQL passes for Go and JavaScript (`codeql.yml`).
  - `gitleaks` finds no secrets (`secret-scan.yml`).
- Each PR is small and self-contained. Multi-surface changes (touches `api/` and `web/` together) are flagged for review.
- New SKUs: one PR per SKU, scoped to `api/internal/catalog/<sku>.go` + scenario fixture + customer-facing doc entry under `docs/pricing.md`. Bigger structural changes don't ride along.
- Bump [`VERSION`](VERSION) and update [`CHANGELOG.md`](CHANGELOG.md) for anything that affects customers (API change, pricing change, SLA change). Internal refactors don't bump the version.
- Fill out the [`PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md) honestly — including the **Compliance impact** section. Compliance review happens at PR time, not after merge.

## What's out of scope here

The **operator plane** — `fleetctl`, the Mac worker bootstrap, JSONL log pipeline, signed ledger, Hermes integration — lives in [`owera-fleet`](https://github.com/owera/owera-fleet). PRs targeting fleet behaviour belong there.

The **marketing site** is on a separate codebase and is not open to external contribution.

## Generated-by-Claude flag

If your PR was authored by an AI coding agent (Claude Code, Cursor, etc.), tick the box in the PR template. We don't reject AI-authored PRs, but we want the audit trail — and we read those diffs with extra care.

## Code of conduct

Be respectful. Owera is a small team; we expect collaborators to be likewise. Harassment, discrimination, or abusive conduct toward maintainers or other contributors gets you banned from the repo and reported to GitHub.
