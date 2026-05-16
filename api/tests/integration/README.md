# Integration tests

This directory holds future end-to-end tests that exercise the full
api → queue → dispatcher → operator-plane round trip with a live SQLite
database and an HTTP test server. Unit tests for each package live next
to the code in `api/internal/<pkg>/<pkg>_test.go`.

## Test contract

An integration test in this directory:

1. Boots the API server in-process (`server.New(deps)` returns an
   `http.Handler` you can hand to `httptest.NewServer`).
2. Uses a SQLite database under `t.TempDir()` so each test is hermetic.
3. Stubs the operator-plane via `dispatcher.NewInMemoryTransport()` (the
   same fake used by unit tests) configured with a custom `Responder`
   that simulates the operator-plane RPC contract.
4. Provisions one or more test tenants via `identity.Store.CreateTenant`
   + `identity.Store.IssueAPIKey` and uses the returned plaintext bearer
   token in `Authorization: Bearer ...`.
5. Asserts both the HTTP-level behaviour and the persisted side effects
   (jobs table, queue depth, audit log rows).
6. Never relies on real network or real Stripe.

## Scenarios to cover (Phase 3)

- `POST /v1/jobs` happy path through `queued` -> dispatcher pickup ->
  `running` -> ledger bill event -> billing outbox -> Reconcile -> Stripe
  fake records.
- Cross-tenant isolation: tenant A holds api key, requests
  `GET /v1/jobs/<id>` of tenant B's job → 404.
- Idempotency: same `idempotency_key` submitted twice returns the same
  `job_id`, only one queue row exists.
- Cancellation: queued job cancel transitions to `cancelled` and does not
  fire any operator-plane RPC; running job cancel calls
  `fleet.CancelTask`.
- Stripe webhook inbound with a valid signature is accepted; a stale or
  forged signature is rejected.
- `/readyz` returns 503 when the dispatcher transport is unhealthy.

The unit tests in each package cover the same contracts at the
function/method level, so an integration test failure here always
points to a wiring or boundary bug rather than a logic bug inside a
package.
