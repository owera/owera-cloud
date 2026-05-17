# Customer onboarding playbook (T20.1)

> **Audience: customer-success operator (today: Rodrigo Recio).** Step-by-step procedure to take a new design partner from "MSA signed" to "running their first paid job" within seven days. This is the operator-side runbook; the customer-facing equivalent is [`docs/onboarding.md`](../../docs/onboarding.md).

**Owner:** SOE (Solutions / Operator Engineer) workstream
**Target SLA:** Design partner #1 → first paid job ≤7 calendar days from MSA signature.
**Review cadence:** After each completed onboarding; quarterly review for systemic friction.

## Pre-flight (before customer signs)

| Step | Who | Notes |
|---|---|---|
| MSA + DPA reviewed by legal | Rodrigo + counsel | DPA covers LGPD (Brazilian customers) + GDPR (EU customers); reuse the template under `compliance/templates/` (TODO: extract from prior signings). |
| Pricing tier confirmed in writing | PM + Rodrigo | Document the agreed cost-cap, SKUs in scope, and overage rates in the MSA. |
| Stripe customer record exists | Operator | Use the [Stripe MCP](mcp__claude_ai_Stripe__create_customer) in test mode for V0; switch to live mode at first invoice. Record the `cus_*` id. |

If any pre-flight item is missing, **stop the onboarding** and resolve before issuing credentials.

## Day 0 — MSA signed

| # | Action | Tool | Output / Done when |
|---|---|---|---|
| 1 | Create tenant in identity store | `fleetctl tenant create --name <Customer>` (operator helper; falls back to direct SQL until the helper lands) | `tenant_id` recorded in onboarding-tracker.md |
| 2 | Create initial user (customer's primary contact) | Identity API: `POST /v1/admin/users` (TODO: admin endpoint is V1 work; for V0 use direct SQL) | `user_id` recorded; email validated |
| 3 | Map tenant_id ↔ Stripe customer_id | `billing.SetTenantCustomer(tenant_id, cus_*)` (TODO: helper not yet exposed; record manually for V0) | Mapping in onboarding-tracker.md |
| 4 | Set cost cap to MSA value | `billing.SetTenantCap(tenant_id, cents)` | Cap matches MSA; verify via /v1/usage |
| 5 | Mint first API key + share securely | Dashboard "Create key" flow OR direct `IssueAPIKey` for V0 | Customer confirms receipt via the same secure channel (1Password share / encrypted email) |

Total Day-0 work: ~30 minutes per customer on V0.

## Day 1 — Kickoff call

Agenda (45 minutes, video):

1. **Introductions** (5 min) — operator team + customer technical lead + customer business owner.
2. **Architecture walkthrough** (10 min) — share `docs/architecture.md`; show the request lifecycle diagram.
3. **SKU selection** (10 min) — confirm which V0 SKUs the customer will run first; walk through inputs schema and SLAs.
4. **First job, live** (10 min) — operator demonstrates a smoke job from the dashboard; customer mirrors with their key in a side terminal.
5. **Q&A + next steps** (10 min) — schedule Day-7 review; set the customer's success metric (e.g., "10 jobs/day for 30 days at 95% SLA").

**Output:** kickoff notes committed to `compliance/onboarding-history/<customer-slug>.md` (private; gitignored variant if needed).

## Day 2–6 — Customer self-driven

Customer runs jobs against their tenant. The operator monitors via:

- `audit_log` rows for the customer's tenant_id (rate of `job.submit`, failures, billing events)
- Stripe usage records (matches expected unit-pricing for the SKUs in scope)
- Status page incident feed (any S1/S2 affecting their tier)

If usage is below 10% of MSA-committed volume by Day 4, **proactively reach out** to find the friction:

- Inputs schema confusing? → enrich `docs/sku-template.md` and notify customer.
- SLA missed? → escalate to TL; root-cause before Day 7.
- Billing surprise? → re-walk the cost-cap math; consider raising the cap or restructuring the SKU.

## Day 7 — Review call

Agenda (30 minutes, video):

1. **Usage review** (10 min) — pull `audit_log` count + Stripe usage; compare against MSA expectations.
2. **Friction inventory** (10 min) — what was hard? What's still confusing? Anything missing from the SKU catalogue?
3. **30-day commitment** (10 min) — confirm next-30-day targets, escalation path, paged-incident contact.

**Output:** Day-7 review notes appended to the customer's onboarding history file.

## Escalation paths

| Situation | First responder | Backup |
|---|---|---|
| Account locked out (API key issue) | Rodrigo (any hour) | DPO email `dpo@owera.com` |
| Billing dispute | Operator + finance (next business day) | Stripe dashboard manual override |
| SLA breach reported by customer | Operator (within S2 SLA — 4 h business hours) | TL on-call |
| Suspected security issue | `security@owera.com` (24×7) | Rodrigo direct |

## Tooling debt (V1+ improvements)

These are friction points caught during V0 onboarding that get fixed as the customer base grows:

- [ ] `fleetctl tenant create --name` operator helper (Day 0 step 1) — currently direct SQL.
- [ ] `POST /v1/admin/users` admin endpoint (Day 0 step 2) — currently direct SQL.
- [ ] `billing.SetTenantCustomer` + `billing.SetTenantCap` admin helpers (Day 0 steps 3-4) — currently manual record-keeping.
- [ ] Onboarding-tracker dashboard — replaces the manual `compliance/onboarding-history/` markdown approach once customer count > 5.
- [ ] Pre-call self-serve onboarding flow — eliminates the Day-1 kickoff for self-serve tier customers.

Each item gets a Wave-N+ ticket as the customer count justifies the build cost.

## Related

- [`docs/onboarding.md`](../../docs/onboarding.md) — customer-facing version
- [`docs/support.md`](../../docs/support.md) — what customer hears about response SLAs
- [`compliance/policies/ga-gate.md`](../policies/ga-gate.md) — G1 (paying customers) measures the output of this playbook
- [`incident-response.md`](incident-response.md) — escalation when onboarding hits a real incident

## Change log

| Date | Author | Change |
|---|---|---|
| 2026-05-17 | TL (Wave 9 prep) | Initial draft for T20.1 closeout |
