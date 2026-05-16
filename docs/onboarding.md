# Onboarding

> **Audience: new customers.** Five steps from "I want to try Owera Agentic" to "I have a first job result and an invoice in my inbox." Target time-to-first-success: 30 minutes.

## Step 1 — Sign up

Visit [`app.owera.ai`](https://app.owera.ai) and click **Sign up**. We support email + password and OAuth via Google or GitHub (whichever is wired to the identity backend for your tier).

After verification, you'll land on the dashboard with a fresh tenant. Tenant IDs look like `t_<24-char-base32>` and are visible in **Settings → Tenant**.

> **Private beta note.** During V0 (private beta), signup is gated. Email `hello@owera.com` with your name, company, and a one-paragraph description of what you'd run on the platform. You'll receive a signup link once provisioned.

## Step 2 — Provision payment + cost cap

From **Settings → Billing**, add a card via Stripe Checkout. The first card on file becomes the default for invoicing.

Set your **monthly cost cap** at the same time. The cap is the maximum we'll bill you in any single month — `POST /v1/jobs` returns `402 Payment Required` if a new job would push you above it. Default is **$200 USD/month**; raise from the same screen for self-serve tiers, or email `hello@owera.com` for managed tiers.

See [`pricing.md`](pricing.md) for the cost-cap details and the V0/V1 SKU pricing modes.

## Step 3 — Issue an API key

From **Settings → API Keys**, click **Create key**. Name it (e.g. `dev-laptop`, `ci-pipeline`); the dashboard shows the key string once — copy it now. We do not store the cleartext key.

Keys are tenant-scoped. Every API response is restricted to your tenant; you cannot read another tenant's data, and a leaked key only exposes your tenant.

Rotation: create a new key, switch your callers over, delete the old one. The API does not enforce a rotation cadence; for production deployments, monthly rotation is a good baseline.

## Step 4 — Submit your first job

V0 customers run one of `triage-watch` or `campaign-swarm`. V1 customers also have `research-brief` and `code-audit`. Pick the SKU that matches your evaluation goal.

Example: submit a small `research-brief`:

```bash
curl -X POST https://api.owera.ai/v1/jobs \
  -H "Authorization: Bearer $OWERA_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{
    "sku": "research-brief",
    "inputs": {
      "topic": "Competitive landscape for agentic-work platforms, Q2 2026",
      "depth": "S",
      "deliver_to": "you@example.com"
    }
  }'
```

Response:

```json
{
  "id": "j_01HXXX...",
  "status": "submitted",
  "sku": "research-brief",
  "submitted_at": "2026-05-16T18:42:01Z",
  "updated_at": "2026-05-16T18:42:01Z"
}
```

Poll for completion:

```bash
curl https://api.owera.ai/v1/jobs/j_01HXXX... \
  -H "Authorization: Bearer $OWERA_API_KEY"
```

When status moves to `succeeded`, the response includes an `outputs` object scoped to the SKU's output schema.

See [`api.md`](api.md) for the full endpoint reference and [`api/openapi.yaml`](../api/openapi.yaml) for SKU-specific input/output schemas.

## Step 5 — See your invoice

Invoices issue on the 1st of each month for the prior month's usage. You'll see them in the dashboard under **Billing → Invoices** and as a PDF in your inbox from `noreply@owera.ai`.

The line items map 1:1 to the SKU usage shown in `GET /v1/usage`. If a number doesn't match what you expect, the ledger is the source of truth — email `hello@owera.com` with your tenant ID and the invoice number; we'll reconcile.

## What to do next

- **Wire to your callers.** The Next.js dashboard at `app.owera.ai` consumes the API the same way you do — there's no privileged "internal" surface. The TypeScript client under [`web/lib/`](../web/) is generated from `api/openapi.yaml` and can be a reference.
- **Set up idempotency.** Use the `Idempotency-Key` header on every `POST /v1/jobs`. Stable UUID per logical submission. Retries are then safe.
- **Subscribe to the status page.** [`status.owera.ai`](https://status.owera.ai) notifies you of incidents affecting your SKUs.
- **Read [`support.md`](support.md).** Know which channel to use when something is wrong.

## Troubleshooting first-job failures

| Symptom | Likely cause | Fix |
|---|---|---|
| `401 Unauthorized` | Missing or wrong `Authorization` header. | Re-check `Bearer <key>` format; rotate the key if you suspect it's stale. |
| `400 Bad Request` with `detail: "unknown sku"` | SKU not enabled for your tenant. | V0 tenants only see V0 SKUs. Check `GET /v1/skus` for what's available; email us to enable a different tier. |
| `402 Payment Required` | Cost cap would be exceeded. | Raise the cap, or wait for next billing period. |
| `429 Too Many Requests` | Rate-limited (60 req/min default). | Respect `Retry-After`. Email us if you need a higher limit. |
| Job stuck in `queued` for >10 min | Operator plane backlog or fleet incident. | Check `status.owera.ai`. If clean, email support with the job ID. |
| Job moves to `failed` | SKU-specific input validation, or operator-plane error. | Inspect `outputs.error` and `outputs.error_detail`. Most failures are bad inputs; the rest are on us. |
