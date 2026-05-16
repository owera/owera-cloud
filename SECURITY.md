# Security Policy

## Supported versions

`owera-cloud` is in early development. Only the latest tagged release (see [`VERSION`](VERSION) and [`CHANGELOG.md`](CHANGELOG.md)) is supported. Backports to earlier versions are made only for critical issues at maintainer discretion.

## Reporting a vulnerability

**Do not open a public GitHub issue for security-relevant findings.** Instead, email `security@owera.com` with:

- A description of the issue and the affected component (`api/`, `web/`, `infra/`, `tunnel/`, `compliance/`, or this scaffold).
- Steps to reproduce. A minimal failing case (curl invocation, browser console log, screenshot) is ideal.
- Your assessment of impact and severity.
- Any suggested mitigation.

We'll acknowledge receipt within **2 business days (Brazil business hours, UTC−3)** and aim to publish a fix or mitigation within **30 days for high-severity issues**. Lower-severity issues are addressed in normal release cadence.

We support **coordinated disclosure on a 90-day window**: if we haven't shipped a fix or coordinated an extension with you 90 days after your initial report, you may disclose publicly. We will not retaliate against good-faith disclosures.

We don't run a paid bug bounty program at this time; we will acknowledge contributors in release notes and on the public status page if you wish.

## Safe-harbor for good-faith research

If you act in good faith — meaning you test against your own tenant or test mode, you don't access other customers' data, you don't exfiltrate data beyond what's needed to demonstrate the issue, you don't degrade availability for other customers, and you report privately — we will not pursue legal action against you. This applies to:

- API fuzzing against your own tenant.
- Authentication boundary testing using your own credentials.
- Static analysis of public artifacts (this repo, `api/openapi.yaml`, published binaries).
- Probing for misconfigured infrastructure (open buckets, exposed admin endpoints, dangling DNS).

Out of scope for safe-harbor:

- Accessing or attempting to access another tenant's data.
- Social engineering of Owera employees or customers.
- Physical attacks on Owera infrastructure.
- DoS / volumetric testing beyond what's needed to demonstrate a vulnerability.

## Scope

In scope:

- `api/` — the public HTTP API gateway, dispatcher, billing, audit log.
- `web/` — the Next.js customer dashboard at `app.owera.ai`.
- `infra/` and `tunnel/` — Fly.io, Vercel, and Cloudflare Tunnel configuration.
- `status/` — the public status page at `status.owera.ai`.
- This scaffold (CI workflows, issue templates).

Out of scope (handled in the sister repo or vendor relationship):

- The **operator plane** — `fleetctl`, the Mac worker fleet, the signed ledger. Report to [`owera-fleet`](https://github.com/owera/owera-fleet) `security@owera.com`.
- **Third-party services** — Stripe, Cloudflare, Fly.io, Vercel, Clerk/WorkOS, PagerDuty. Report to the vendor directly under their disclosure program.
- **Marketing site** at `owera.com` — separate codebase, separate disclosure path.

## Compliance posture

Owera Software Ltda is a Brazilian controller subject to **LGPD** (Lei Geral de Proteção de Dados, 13.709/2018). Customer data residency, deletion (right-to-erasure), and audit-log retention are documented at [`https://owera.com/privacy`](https://owera.com/privacy) and in [`compliance/policies/`](compliance/).

This repo also targets **SOC 2 Type 1** readiness within 12 months of GA. Technical evidence — signed ledger, audit log, secret handling, signed config sync — is built progressively; the control-to-evidence mapping lives in [`compliance/audit-controls/`](compliance/) and is published openly as a trust signal.

For GDPR / EU customer questions (DPA, SCCs, data residency), see [`docs/compliance.md`](docs/compliance.md).
