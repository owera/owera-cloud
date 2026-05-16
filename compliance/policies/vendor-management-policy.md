# Vendor management policy

**Scope:** every third party Owera depends on for production services, including SaaS, infrastructure providers, and external auditors.

## 1. Principle

Owera's compliance posture is bounded by its vendors' posture. We onboard only vendors that meet our minimum security bar, monitor their continued compliance, and document the dependency chain.

## 2. Tier classification

Vendors are tiered by the blast radius of a vendor compromise:

| Tier | Definition | Examples | Onboarding requirements |
|------|------------|----------|--------------------------|
| **Critical** | Compromise = customer data exposure or platform unavailability | Stripe, Cloudflare, Fly.io, Vercel, Clerk/WorkOS, 1Password | SOC 2 Type 2 OR equivalent (ISO 27001, BSI C5, FedRAMP); DPA signed; quarterly attestation review |
| **Important** | Compromise = degraded operations or internal-only data exposure | GitHub, Google Workspace, Slack, PagerDuty, Sentry, Anthropic (Claude API), OpenAI | SOC 2 Type 2 OR equivalent; DPA signed; annual review |
| **Standard** | Compromise = inconvenience only | Marketing tools, calendar, accounting (within BR compliance) | Vendor terms reviewed; annual lightweight review |

## 3. Current vendor register

| Vendor | Tier | Service | DPA on file | Last review | SOC2/ISO evidence |
|--------|------|---------|-------------|-------------|--------------------|
| **Stripe** | Critical | Payments | `[ ]` TODO | n/a (pre-GA) | Stripe SOC 2 Type 2 (annual public report) |
| **Cloudflare** | Critical | DNS, CDN, tunnel | `[ ]` TODO | n/a | Cloudflare SOC 2 Type 2 |
| **Fly.io** | Critical | API hosting | `[ ]` TODO | n/a | Fly SOC 2 Type 2 |
| **Vercel** | Critical | Web hosting | `[ ]` TODO | n/a | Vercel SOC 2 Type 2 |
| **Clerk** *or* **WorkOS** | Critical | Customer auth | `[ ]` TBD | pending bake-off | Both have SOC 2 Type 2 |
| **1Password** | Critical | Secret management | `[ ]` TODO | n/a | 1Password SOC 2 Type 2 + ISO 27001 |
| **GitHub** | Important | Source hosting, Actions | `[ ]` TODO | n/a | GitHub SOC 2 Type 2 |
| **Google Workspace** | Important | Identity, email | `[ ]` TODO | n/a | Google Workspace SOC 2 Type 2 + ISO 27001 |
| **Slack** | Important | Internal comms | `[ ]` TODO | n/a | Slack SOC 2 Type 2 |
| **PagerDuty** | Important | On-call paging | `[ ]` TODO | n/a | PagerDuty SOC 2 Type 2 |
| **Sentry** | Important | Error reporting | `[ ]` TODO | n/a | Sentry SOC 2 Type 2 |
| **Anthropic** | Important | Claude API (operator-plane LLM) | `[ ]` TODO | n/a | Anthropic SOC 2 Type 2 + zero-retention API option |
| **Amazon SES** | Important | Transactional email | `[ ]` TODO | n/a | AWS SOC 2 Type 2 |
| **External SOC 2 auditor** | Standard (engagement-scoped) | Annual audit | `[ ]` TBD | n/a | Their own SOC 2 / AICPA peer review |
| **External pentester** | Standard (engagement-scoped) | Annual pentest | `[ ]` TBD | n/a | Vendor diligence at engagement |
| **External Brazilian counsel** | Standard | LGPD legal review | `[ ]` TBD | n/a | OAB-registered firm |

`[ ]` markers track DPA execution status. Onboarding is **not complete** until the DPA is signed and stored in 1Password under `Owera / Vendor DPAs`.

## 4. Onboarding procedure

1. **Justify**: PR or doc explaining why the vendor is needed, what data they'll see, what alternatives were considered.
2. **Tier**: CISO assigns the tier per §2.
3. **Diligence**: collect the vendor's SOC 2 / ISO report (or equivalent). For Critical vendors, the report must be Type 2 and dated within the last 12 months.
4. **DPA**: execute a Data Processing Agreement covering LGPD (and GDPR where applicable). Stored in 1Password.
5. **Onboard**: provision the vendor with the minimum data/access scope needed. Document the integration in the relevant runbook.
6. **Register**: add to the table in §3 with the review date and evidence pointer.

## 5. Ongoing monitoring

- **Critical vendors**: review the current SOC 2 / ISO evidence every quarter. If a vendor falls out of audit (rare but happens during transitions), escalate to CISO; consider an alternative.
- **Important vendors**: annual review.
- **Standard vendors**: annual lightweight review.
- **Breach notifications**: subscribe to each Critical vendor's security advisories. Vendor-disclosed incidents trigger our incident response if our data was potentially affected.

## 6. Offboarding procedure

1. Identify all integrations (search secrets-manifest.md, IaC, application code).
2. Migrate or decommission each integration.
3. Revoke API tokens / sessions on the vendor side.
4. Delete or export customer data on the vendor side (DPA-mandated procedure varies by vendor).
5. Remove from §3 register with offboarding date noted in commit history.

## 7. Anthropic / OpenAI specifics

Operator plane uses Claude (Anthropic) and may use OpenAI models. Both vendors offer zero-retention API tiers. We **must** use zero-retention by default for any customer-payload-bearing call. The CLI/SDK configuration that enforces this lives in `owera-fleet` (`internal/llm/` package — TODO) and is part of the security-policy attestation.

## Ownership

| Role | Responsibility |
|------|----------------|
| CISO | Accountable for the policy; tier assignment; review cadence |
| CFO | Vendor budget; DPA execution liaison with external counsel |
| SRE Lead | Technical integration security; secret hygiene per vendor |

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
