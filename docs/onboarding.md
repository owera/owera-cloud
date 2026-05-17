# Onboarding

> **Audience: new customers.** This is the customer-facing guide from "I want to try Owera Agentic" to "I have a first job result and an invoice in my inbox." Target time-to-first-success: 30 minutes once your tenant is provisioned. For the internal counterpart used by our customer-success operator, see [`compliance/runbooks/onboarding-playbook.md`](../compliance/runbooks/onboarding-playbook.md).

The Owera Agentic API is live at `https://owera-agentic-api.fly.dev` (the canonical hostname `api.owera.ai` resolves here once DNS is cut over). All examples below run against the Fly hostname so they keep working through cutover.

## Step 1 — Sign up

Visit [`app.owera.ai`](https://app.owera.ai) and click **Sign up**. Authentication is provided by Clerk; we support email + password and social providers wired in the Clerk app (Google, GitHub).

After verification, you'll land on the dashboard. Your account is bound to an **organisation** in Clerk and a **tenant** in Owera; both bindings are managed by our customer-success operator during onboarding. Tenant IDs look like `ten_<24-char-base32>` and are visible in **Settings → Tenant**.

> **Private beta note.** During V0 (private beta), signup is gated. Email `hello@owera.com` with your name, company, and a one-paragraph description of what you'd run on the platform. Our operator will provision your Stripe customer, Clerk org, Owera tenant, and the four bindings between them (see the [operator playbook](../compliance/runbooks/onboarding-playbook.md)) before sending you a Clerk invitation. Day-0 takes about 15 minutes on our side.

## Step 2 — Confirm billing + cost cap

Your Stripe customer is created and bound to your Owera tenant by the operator during pre-flight. You'll receive a Stripe Customer Portal link via email; from there you can manage cards, view upcoming invoices, and cancel.

You can also open the portal from the dashboard at any time. The dashboard calls:

```
POST https://owera-agentic-api.fly.dev/v1/billing/portal
Authorization: Bearer <api-key>
```

which returns a one-time portal URL.

The **monthly cost cap** was agreed in your MSA and recorded on the tenant. `POST /v1/jobs` returns `402 Payment Required` if a new job would push you over it. To raise the cap, email `hello@owera.com` — self-serve cap adjustment is on the roadmap but not yet wired.

See [`pricing.md`](pricing.md) for the SKU pricing modes and how the cap is computed.

## Step 3 — Get an API key

You have two paths to a working API key:

**Self-serve (recommended for most callers).** From **Settings → API Keys** in the dashboard, click **Create key**. Name it (e.g. `dev-laptop`, `ci-pipeline`); the dashboard shows the key string (prefix `owc_*`) once — copy it now. We do not store the cleartext key, only an argon2id hash.

**Operator-issued (for batch-CLI customers who need a key before they ever sign in).** During pre-flight, the operator can mint a primary key via the admin endpoint:

```
POST https://owera-agentic-api.fly.dev/v1/admin/tenants/{tenant_id}/users/{user_id}/api-keys
Authorization: Bearer $OWERA_ADMIN_TOKEN
Content-Type: application/json

{ "label": "primary" }
```

The plaintext token is returned **once** in the response and handed off to you through your secrets channel. Subsequent admin lookups only return the prefix.

Keys are tenant-scoped. Every API response is restricted to your tenant; you cannot read another tenant's data, and a leaked key only exposes your tenant.

**Rotation.** Create a new key, switch your callers over, delete the old one in **Settings → API Keys**. The API does not enforce a rotation cadence; for production deployments, monthly rotation is a good baseline, and immediate rotation is required on any suspected compromise.

## Step 4 — Submit your first job

V0 customers run one of two SKUs: **`triage-watch`** or **`campaign-swarm`**. V1 adds `research-brief` and `code-audit` at GA; what's enabled for your tenant is always visible at `GET /v1/skus`.

```bash
curl https://owera-agentic-api.fly.dev/v1/skus \
  -H "Authorization: Bearer $OWERA_API_KEY"
```

### `triage-watch` (V0)

Ingests a support queue, classifies incoming tickets by priority, and autonomously responds to the high-severity ones. Pricing: $499/month subscription with per-ticket overage. SLA: under 2 minutes from ticket to first response.

Inputs schema (JSON Schema; source: [`api/internal/catalog/triage_watch.go`](../api/internal/catalog/triage_watch.go)):

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `queue_url` | string | yes | — | Source queue endpoint (e.g. Zendesk view URL, Intercom inbox, IMAP). |
| `priority_threshold` | integer (1–10) | no | `8` | Tickets at or above this priority dispatch to the operator plane. |

Submit:

```bash
curl -X POST https://owera-agentic-api.fly.dev/v1/jobs \
  -H "Authorization: Bearer $OWERA_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{
    "sku": "triage-watch",
    "inputs": {
      "queue_url": "https://acme.zendesk.com/agent/filters/360001234567",
      "priority_threshold": 7
    }
  }'
```

### `campaign-swarm` (V0)

Takes a campaign brief plus an audience segment and fans out multi-channel outreach via the operator plane's worker fleet. Pricing: per-job fixed (S/M/L tier; tier is computed from `max_outreach`). SLA: under 15 minutes from submit to launched.

Inputs schema (source: [`api/internal/catalog/campaign_swarm.go`](../api/internal/catalog/campaign_swarm.go)):

| Field | Type | Required | Default | Notes |
|---|---|---|---|---|
| `brief` | string (≥10 chars) | yes | — | Campaign brief: what's the offer, who's it for, what's the call to action. |
| `audience_segment` | string | yes | — | Segment descriptor (e.g. `crm:segment:q2-warm-leads`). |
| `channels` | array of `email`/`linkedin`/`x`/`phone` | no | `["email"]` | At least one channel. |
| `max_outreach` | integer (1–10000) | no | `500` | Hard ceiling on contacts the swarm will touch. Drives S/M/L tier pricing. |

Submit:

```bash
curl -X POST https://owera-agentic-api.fly.dev/v1/jobs \
  -H "Authorization: Bearer $OWERA_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{
    "sku": "campaign-swarm",
    "inputs": {
      "brief": "Launch the Q2 dev-tools beta to senior platform engineers at Series-B startups; CTA is a 30-min discovery call.",
      "audience_segment": "crm:segment:q2-warm-leads",
      "channels": ["email", "linkedin"],
      "max_outreach": 750
    }
  }'
```

### Response shape

```json
{
  "job_id": "j_01HXXX...",
  "status": "queued",
  "sku": "triage-watch",
  "submitted_at": "2026-05-17T18:42:01Z"
}
```

Poll for completion:

```bash
curl https://owera-agentic-api.fly.dev/v1/jobs/j_01HXXX... \
  -H "Authorization: Bearer $OWERA_API_KEY"
```

When status moves to `succeeded`, the response includes an `outputs` object scoped to the SKU's output schema. You can also cancel a queued or running job:

```bash
curl -X POST https://owera-agentic-api.fly.dev/v1/jobs/j_01HXXX.../cancel \
  -H "Authorization: Bearer $OWERA_API_KEY"
```

See [`api.md`](api.md) for the full endpoint reference and [`api/openapi.yaml`](../api/openapi.yaml) for SKU-specific input/output schemas.

## Step 5 — See your invoice

Invoices issue on the 1st of each month for the prior month's usage. You'll see them in the Stripe Customer Portal (linked from the dashboard `Billing` page) and as a PDF emailed from `noreply@owera.ai`.

Mid-month, your in-progress usage is queryable:

```bash
curl https://owera-agentic-api.fly.dev/v1/usage \
  -H "Authorization: Bearer $OWERA_API_KEY"
```

The line items on the invoice map 1:1 to the SKU usage shown here. If a number doesn't match what you expect, the operator-plane ledger is the source of truth — email `hello@owera.com` with your tenant ID and the invoice number; we'll reconcile.

## What to do next

- **Wire to your callers.** The Next.js dashboard at `app.owera.ai` consumes the same public API you do — there's no privileged "internal" surface. The TypeScript client under [`web/lib/`](../web/) is generated from `api/openapi.yaml` and can serve as a reference implementation.
- **Set up idempotency.** Use the `Idempotency-Key` header on every `POST /v1/jobs`. Stable UUID per logical submission. Retries are then safe; replays within 24 hours return the same `job_id` without re-charging.
- **Subscribe to the status page.** [`status.owera.ai`](https://status.owera.ai) notifies you of incidents affecting your SKUs.
- **Read [`support.md`](support.md).** Know which channel to use when something is wrong, and what response SLA applies.

## Troubleshooting first-job failures

| Symptom | Likely cause | Fix |
|---|---|---|
| `401 Unauthorized` | Missing or wrong `Authorization` header. | Re-check `Bearer <key>` format; rotate the key if you suspect it's stale. |
| `400 Bad Request` with `detail: "unknown sku"` | SKU not enabled for your tenant. | V0 tenants only see V0 SKUs. Check `GET /v1/skus` for what's available; email `hello@owera.com` to enable a different tier. |
| `400 Bad Request` with a JSON-Schema validation error | Inputs don't match the SKU schema. | Compare your payload against the inputs table above (or `api/openapi.yaml`). |
| `402 Payment Required` | Cost cap would be exceeded. | Raise the cap (email `hello@owera.com`), or wait for next billing period. |
| `429 Too Many Requests` | Rate-limited (60 req/min default). | Respect `Retry-After`. Email us if you need a higher limit. |
| Job stuck in `queued` for >10 min | Operator-plane backlog or fleet incident. | Check `status.owera.ai`. If clean, email support with the job ID. |
| Job moves to `failed` | SKU-specific input validation, or operator-plane error. | Inspect `outputs.error` and `outputs.error_detail`. Most failures are bad inputs; the rest are on us. |

## Related

- [`support.md`](support.md) — how to reach us, severity definitions, response SLAs
- [`api.md`](api.md) — full endpoint reference
- [`pricing.md`](pricing.md) — SKU pricing modes, cost-cap math
- [`compliance/runbooks/onboarding-playbook.md`](../compliance/runbooks/onboarding-playbook.md) — the operator-facing counterpart of this document (what our customer-success operator does on Day 0)
