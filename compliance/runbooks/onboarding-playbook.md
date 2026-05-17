# Customer onboarding playbook (T20.1)

> **Audience: customer-success operator (today: Rodrigo Recio).** Step-by-step procedure to take a new design partner from "MSA signed" to "running their first paid job" within seven days. This is the operator-side runbook; the customer-facing equivalent is [`docs/onboarding.md`](../../docs/onboarding.md).

**Owner:** SOE (Solutions / Operator Engineer) workstream
**Target SLA:** Design partner #1 → first paid job ≤7 calendar days from MSA signature.
**Review cadence:** After each completed onboarding; quarterly review for systemic friction.

## Identity topology — what gets created where

One Owera customer maps to a row in three independent systems. The onboarding flow binds them together:

```
   Clerk                       Owera                    Stripe
 (dashboard auth)         (api + ledger)              (billing)

   Organisation             Tenant                    Customer
   org_2abc...   ────────►  ten_xyz       ────────►   cus_def...
       │                      │                          │
       │ has-members          │ has-users                │ has-subscriptions
       ▼                      ▼                          │
    User                    User                         │
    user_2def... ─────────►  usr_abc                     │
                                                         │
                              │                          │
                              └──────── monthly cap ─────┘
                                       (cents in identity)
```

- **Clerk org_id ↔ Owera tenant_id** — set via `POST /v1/admin/tenants/{id}/clerk-org`
- **Clerk user_id ↔ Owera user_id** — set via `POST /v1/admin/tenants/{id}/users/{user_id}/clerk-user`
- **Owera tenant_id ↔ Stripe customer_id** — set via `POST /v1/admin/tenants/{id}/stripe-customer`
- **Owera tenant_id ↔ monthly cap** — set via `POST /v1/admin/tenants/{id}/cap`

When all four bindings exist, the customer can:
- Sign in to `app.owera.ai` via Clerk → dashboard requests resolve to their Owera tenant + user.
- Mint API keys → CLI / programmatic submission against the same tenant.
- Submit jobs → Owera bills the Stripe customer (subject to the cap).

## Pre-flight (before customer signs)

| Step | Who | Notes |
|---|---|---|
| MSA + DPA reviewed by legal | Rodrigo + counsel | DPA covers LGPD (Brazilian customers) + GDPR (EU customers); reuse the template under `compliance/templates/` (TODO: extract from prior signings). |
| Pricing tier confirmed in writing | PM + Rodrigo | Document the agreed cost-cap, SKUs in scope, and overage rates in the MSA. |
| `OWERA_ADMIN_TOKEN` available | Operator | The shared admin bearer token gates all `/v1/admin/*` calls. Stored in `fly secrets` on `owera-agentic-api`; rotate via `fly secrets set OWERA_ADMIN_TOKEN=<new>` on operator turnover. Treat like a master password: never paste into any tool's argv or transcript. |

If any pre-flight item is missing, **stop the onboarding** and resolve before issuing credentials.

## Day 0 — MSA signed

All times approximate; everything in this section runs from the operator's laptop with `OWERA_ADMIN_TOKEN` in a shell env (`read -rs OWERA_ADMIN_TOKEN`; export it for the session; `unset` when done).

### Step 1 — Provision Stripe customer (2 min)

Create the Stripe customer in **test mode** (V0; switch to live mode at first paid invoice):

```bash
# Via Stripe MCP or dashboard. Captures: cus_*, email matches MSA primary contact.
# Record cus_id in compliance/onboarding-history/<customer-slug>.md
```

### Step 2 — Provision Clerk organisation (3 min)

In the Clerk dashboard (`https://dashboard.clerk.com` → your app):

1. **Organisations** → **+ Create**
2. Name: customer's legal entity name
3. Capture the resulting `org_2abc...` ID
4. (If the customer has multiple users) **Users** → for each: **+ Create user** → set their email → **Memberships** → **+ Add to organisation** → select the org you just created

Capture each `user_2abc...` ID. The Clerk-side session token must already include `org_id` per the [Clerk JWT template setup](#clerk-jwt-template-setup) — verify once per Clerk app, not per customer.

### Step 3 — Create Owera tenant + user (1 min)

```bash
# Token never enters argv: it's exported once at the top of the session.
TENANT=$(curl -sS https://owera-agentic-api.fly.dev/v1/admin/tenants \
  -H "Authorization: Bearer $OWERA_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme Inc"}' | jq -r .tenant_id)

USER=$(curl -sS https://owera-agentic-api.fly.dev/v1/admin/tenants/$TENANT/users \
  -H "Authorization: Bearer $OWERA_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email":"ops@customer.example"}' | jq -r .user_id)
```

Record `TENANT` + `USER` in the customer's onboarding history file.

### Step 4 — Bind the three external IDs (1 min)

```bash
# Clerk org binding — dashboard JWTs from this org resolve to TENANT
curl -sS -X POST https://owera-agentic-api.fly.dev/v1/admin/tenants/$TENANT/clerk-org \
  -H "Authorization: Bearer $OWERA_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"clerk_org_id":"org_2abc..."}'

# Clerk user binding — JWTs with this sub claim resolve to USER
curl -sS -X POST https://owera-agentic-api.fly.dev/v1/admin/tenants/$TENANT/users/$USER/clerk-user \
  -H "Authorization: Bearer $OWERA_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"clerk_user_id":"user_2abc..."}'

# Stripe customer binding — meter_events + invoice_items target this customer
curl -sS -X POST https://owera-agentic-api.fly.dev/v1/admin/tenants/$TENANT/stripe-customer \
  -H "Authorization: Bearer $OWERA_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"stripe_customer_id":"cus_..."}'

# Monthly cap — cents per month; 0 = use system default, negative = no cap
curl -sS -X POST https://owera-agentic-api.fly.dev/v1/admin/tenants/$TENANT/cap \
  -H "Authorization: Bearer $OWERA_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"monthly_cap_cents":50000}'
```

Each call returns `204 No Content` on success or a typed error (`400 bad_request`, `404 tenant_not_found`, `404 user_not_found`).

### Step 5 — Verify bindings (1 min)

```bash
curl -sS https://owera-agentic-api.fly.dev/v1/admin/tenants \
  -H "Authorization: Bearer $OWERA_ADMIN_TOKEN" | jq ".tenants[] | select(.tenant_id == \"$TENANT\")"
```

Expected fields: `name`, `stripe_customer_id` (non-empty), `monthly_cap_cents` (matches MSA).

### Step 6 — Hand off Clerk sign-in to customer (5 min via email)

The customer signs in to `https://app.owera.ai` with their own Clerk credentials (they receive a Clerk invitation email when you added them to the org). On first sign-in:

1. They select **Acme Inc** in the org switcher.
2. The dashboard `/api/proxy/*` route exchanges their Clerk session for a JWT.
3. Our api's auth middleware → `verifyClerkJWT` → `LookupByClerkOrgID(org_2abc)` → tenant → `LookupUserByClerkUserID(user_2abc)` → user → `WithTenant + WithUser` on the request context.
4. The dashboard renders `/jobs`, `/api-keys`, `/billing`, `/support` — all scoped to their tenant.

### Step 7 — (Optional) mint a primary API key (2 min)

For customers who want CLI / programmatic submission alongside the dashboard, mint an API key:

- Via dashboard: customer goes to **Settings → API Keys → + Create**. Key shown once.
- Via api directly (operator courtesy): there's no admin endpoint for issuing API keys yet — customers must use the dashboard. (Future: `POST /v1/admin/tenants/{id}/users/{user_id}/api-keys`.)

**Total Day-0 work: ~15 minutes per customer.** Down from 30 min (pre-admin-endpoints) and far below the V0 SLA.

## Clerk JWT template setup

This is a **one-time** configuration per Clerk app, not per customer. Verify once after creating the Clerk app; revisit on Clerk version upgrades.

1. Clerk dashboard → **Configure** → **Sessions**
2. **Customize session token** → **Edit**
3. Paste:
   ```json
   {
     "org_id": "{{org.id}}",
     "org_slug": "{{org.slug}}",
     "org_role": "{{org.role}}"
   }
   ```
4. Save.

Our api's verifier (`api/internal/auth/clerk.go`) reads `org_id` to resolve tenant. Without this template, `org_id` is missing from the JWT and every dashboard request 401s with `unknown tenant for clerk org`.

Verify by signing in with a test user that has an active org, paste the resulting JWT into `https://jwt.io`, confirm payload includes `org_id`.

## Pre-built shell wrapper (optional)

If onboarding becomes frequent, drop this in `~/.local/bin/owera-onboard.sh`:

```bash
#!/usr/bin/env bash
# Usage: owera-onboard.sh <customer-name> <email> <clerk_org_id> <clerk_user_id> <stripe_cus_id> <cap_cents>
# Requires OWERA_ADMIN_TOKEN in env, jq, curl.
set -euo pipefail
NAME="$1" EMAIL="$2" CLERK_ORG="$3" CLERK_USER="$4" STRIPE_CUS="$5" CAP="$6"
API=${OWERA_API:-https://owera-agentic-api.fly.dev}
TOK="Authorization: Bearer $OWERA_ADMIN_TOKEN"
JSON="Content-Type: application/json"

TENANT=$(curl -sSf "$API/v1/admin/tenants" -H "$TOK" -H "$JSON" -d "{\"name\":\"$NAME\"}" | jq -r .tenant_id)
USER=$(curl -sSf "$API/v1/admin/tenants/$TENANT/users" -H "$TOK" -H "$JSON" -d "{\"email\":\"$EMAIL\"}" | jq -r .user_id)
curl -sSf -X POST "$API/v1/admin/tenants/$TENANT/clerk-org" -H "$TOK" -H "$JSON" -d "{\"clerk_org_id\":\"$CLERK_ORG\"}"
curl -sSf -X POST "$API/v1/admin/tenants/$TENANT/users/$USER/clerk-user" -H "$TOK" -H "$JSON" -d "{\"clerk_user_id\":\"$CLERK_USER\"}"
curl -sSf -X POST "$API/v1/admin/tenants/$TENANT/stripe-customer" -H "$TOK" -H "$JSON" -d "{\"stripe_customer_id\":\"$STRIPE_CUS\"}"
curl -sSf -X POST "$API/v1/admin/tenants/$TENANT/cap" -H "$TOK" -H "$JSON" -d "{\"monthly_cap_cents\":$CAP}"

echo "onboarded: tenant=$TENANT user=$USER"
```

Reduces Day-0 to a single command.

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

Resolved during the Wave-8 → Wave-8.1+ → main-wire-up sequence; left here as a record of what was Day-0 toil and isn't anymore.

- [x] ~~Direct-SQL tenant create~~ → `POST /v1/admin/tenants` (PR #25)
- [x] ~~Direct-SQL user create~~ → `POST /v1/admin/tenants/{id}/users` (PR #25)
- [x] ~~Manual Stripe-customer mapping~~ → `POST /v1/admin/tenants/{id}/stripe-customer` (PR #25)
- [x] ~~Manual cap recording~~ → `POST /v1/admin/tenants/{id}/cap` (PR #25)
- [x] ~~Clerk binding unwired~~ → `POST /v1/admin/tenants/{id}/clerk-org` + `/users/{user_id}/clerk-user` (PR #29)

Still open:

- [ ] **`POST /v1/admin/tenants/{id}/users/{user_id}/api-keys`** — operator-side API-key minting so onboarding can hand the customer their key without making them log in to the dashboard first. Self-serve flow today works (customer goes to **Settings → API Keys**) but a hand-mintable key shortens TTL for batch-CLI customers.
- [ ] **Onboarding-tracker dashboard** — replaces manual `compliance/onboarding-history/` markdown once customer count > 5.
- [ ] **Pre-call self-serve onboarding flow** — Clerk-only signup → Stripe Checkout → auto-create tenant/user/cap via webhook. Eliminates the Day-1 kickoff for self-serve tier customers (likely V3+).
- [ ] **Onboarding shell wrapper packaged** — the `owera-onboard.sh` snippet above could ship under `infra/scripts/` once the team is more than one person.

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
| 2026-05-17 | TL (post-deploy) | Rewrote Day-0 against the live `/v1/admin/*` endpoints (PRs #25, #29). Added identity-topology diagram, Clerk JWT template setup, Clerk-org + clerk-user binding steps, `owera-onboard.sh` shell wrapper. Marked direct-SQL tooling debt resolved. |
