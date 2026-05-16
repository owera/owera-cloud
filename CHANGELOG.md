# Changelog

All notable changes to `owera-cloud` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Bump policy:

- **Major** — breaking changes to the customer-facing API (`api/openapi.yaml`), the dashboard URL contract, or the billing model.
- **Minor** — new SKU, new endpoint, new dashboard feature, new compliance artifact.
- **Patch** — bug fixes, doc-only changes, dependency bumps, internal refactors that customers don't observe.

## [Unreleased]

## [0.1.0] - 2026-05-16

### Added

- Repo scaffolded. Root markdown surface (`README.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `SECURITY.md`), customer-facing docs under `docs/`, CI workflows under `.github/workflows/`, issue templates and PR template under `.github/`.
- CI workflows: `ci-api.yml` (Go build + test + vet for `api/`), `ci-web.yml` (Next.js typecheck + lint + build for `web/`), `codeql.yml` (CodeQL for Go and JavaScript), `secret-scan.yml` (gitleaks on every push and PR), `release.yml` (build `api/` binary on tag push, upload to GitHub Release).
- Compliance-impact section on the PR template to surface SOC 2 / LGPD considerations at review time.

[Unreleased]: https://github.com/owera/owera-cloud/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/owera/owera-cloud/releases/tag/v0.1.0
