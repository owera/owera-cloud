# API reference

> **Audience: customers.** This document is a human-readable orientation to the Owera Agentic customer API. The authoritative machine-readable spec is [`api/openapi.yaml`](../api/openapi.yaml) — generate clients from it; don't hand-code against the prose here.

## Base URL

| Environment | Base URL |
|---|---|
| Production | `https://api.owera.ai` |
| Local development | `http://localhost:8080` |

The API is versioned via the URL path (`/v1/...`). Breaking changes ship under a new path version; additive changes ship under the existing version.

## Authentication

All endpoints except `GET /healthz` require an API key passed as a bearer token:

```http
Authorization: Bearer <api-key>
```

API keys are issued from the dashboard at `app.owera.ai` under **Settings → API Keys**. Each key resolves to exactly one tenant; every response is scoped to that tenant.

Lost or compromised keys: rotate immediately in the dashboard, or email `security@owera.com` to revoke out-of-band.

## Endpoints (V0 surface)

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/healthz` | Liveness probe. No auth. Returns 200 if the API is up. |
| `GET` | `/v1/skus` | List available SKUs for your tenant. |
| `POST` | `/v1/jobs` | Submit a new job. Body: `{ sku, inputs, idempotency_key? }`. |
| `GET` | `/v1/jobs/{id}` | Fetch a job by ID. Scoped to your tenant. |
| `POST` | `/v1/jobs/{id}/cancel` | Cancel a queued or running job. |
| `GET` | `/v1/usage` | Current billing period usage by SKU. |

See [`api/openapi.yaml`](../api/openapi.yaml) for full request/response schemas, status codes, and error shapes.

## Job lifecycle

Every job moves through this state machine:

```text
submitted → queued → running → succeeded
                              ↘ failed
                              ↘ cancelled
```

The customer dashboard renders the same states as the API. Terminal states (`succeeded`, `failed`, `cancelled`) are immutable.

## Idempotency

`POST /v1/jobs` accepts an optional `Idempotency-Key` header. Replays of the same key within 24 hours return the same job ID without re-charging. Use a stable UUID per logical submission — typically a hash of `(your-internal-request-id, sku, inputs)`.

## Rate limits

Default per-tenant limit: **60 requests per minute** across all endpoints. Rate-limited responses return `429` with `Retry-After` header (seconds).

Higher limits are available on request — email `hello@owera.com` with your tenant ID and expected RPS.

## Errors

All errors return a JSON body matching the `Error` schema in `api/openapi.yaml`:

```json
{
  "error": "string",
  "detail": "string"
}
```

| Status | Meaning |
|---|---|
| `400` | Malformed request body, bad SKU, invalid inputs schema. |
| `401` | Missing or invalid API key. |
| `404` | Job ID not found, or SKU not available for your tenant. |
| `409` | Job is in a terminal state; cancel/replay not allowed. |
| `429` | Rate-limited. Respect `Retry-After`. |
| `5xx` | Server-side. Retry with exponential backoff. |

Note: a job ID that belongs to a different tenant returns `404`, not `403` — the existence of a sibling tenant's job ID is itself sensitive.

## SDK availability

There are no official SDKs yet. The Next.js dashboard's API client under [`web/lib/`](../web/) is generated from `api/openapi.yaml` and can serve as a reference TypeScript implementation. For other languages, generate a client from the OpenAPI spec with `openapi-generator-cli`.

## Versioning policy

- Additive changes (new endpoint, new optional field, new SKU) land under `/v1/...` without notice.
- Breaking changes (removed field, renamed field, changed semantics, new required field) ship under `/v2/...` with a minimum 90-day deprecation window for `/v1/...`.
- SKU-level breaking changes are handled separately — SKU schemas are versioned (`triage-watch@v1`); a `@v2` is a parallel SKU, not a replacement.

See [`pricing.md`](pricing.md) for the SKU catalogue and the V0 → V4 rollout schedule.
