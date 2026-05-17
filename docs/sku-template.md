# SKU contribution template (T13.6)

> **Audience: contributors adding a new SKU to the catalogue.**
> Every SKU is **one PR**. The CI gate `catalog-one-pr-contract` rejects any PR that touches `api/internal/catalog/<sku>.go` AND another core package in the same change — that's the contract that keeps the catalogue extensible without coordination overhead.

This document walks the one-PR shape end to end so a second engineer can land a draft SKU in under two hours.

## What lands in the PR

| File | Purpose |
|---|---|
| `api/internal/catalog/<sku>.go` | The SKU declaration — `Register(...)` in `init()` |
| `api/internal/catalog/<sku>_test.go` *(optional)* | SKU-specific schema/dispatcher unit tests |
| `api/tests/scenarios/<sku>.yaml` | End-to-end scenario the test runner exercises |
| `docs/pricing.md` | Append a row to the SKU table for the matching tier |

Nothing else. If the dispatcher needs new primitives, that's a separate PR against `internal/dispatcher` (the SKU PR can land later, referencing the merged primitive).

## File 1: `<sku>.go`

Single-file shape. `Register` runs at package init, so the SKU is in the global registry the moment the binary boots.

```go
// Package catalog — file is one SKU.
package catalog

import (
	"context"
	"fmt"
)

func init() {
	Register(&SKU{
		Name:     "monitor-watch",
		Version:  "v1",
		Category: "research_and_intelligence",
		InputsSchema: `{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type": "object",
			"required": ["target_url", "cadence_minutes"],
			"properties": {
				"target_url":       {"type": "string", "format": "uri"},
				"cadence_minutes":  {"type": "integer", "minimum": 5, "maximum": 1440},
				"notify_channels":  {"type": "array", "items": {"type": "string"}, "default": []}
			},
			"additionalProperties": false
		}`,
		Pricing: PricingTier{
			Model:       "monthly_subscription",
			BaseCents:   29900,
			OverageRule: "watch_hour",
		},
		SLA: SLA{
			Description:       "Daily digest by 09:00 customer local time.",
			MaxLatencySeconds: 60 * 60 * 24,
		},
		Dispatcher:   dispatchMonitorWatch,
		BillingMeter: "monitor_watch_watch_hour",
	})
}

func dispatchMonitorWatch(ctx context.Context, jobID string, inputs map[string]any) (any, error) {
	// DispatcherFn returns the params blob that internal/dispatcher
	// sends to the operator plane over fleet.SubmitJob. Keep this
	// function pure — no I/O, no time.Now, no goroutines. The
	// dispatcher worker owns scheduling and the operator plane
	// owns execution.
	return map[string]any{
		"sku":     "monitor-watch",
		"version": "v1",
		"inputs":  inputs,
		"job_id":  jobID,
	}, fmt.Errorf("dispatcher stub — fill in cronjob params per operator-plane README") // remove before commit
}
```

### Field-by-field checklist

- **`Name`** — lower-kebab-case, no spaces, stable for the lifetime of the SKU. Customers reference it in API calls; renaming breaks contracts.
- **`Version`** — `v1`, `v2`, … Bump when the inputs schema or pricing changes in a way customers must opt into. Old version stays in the catalogue until customers migrate.
- **`Category`** — one of the five product categories: `software_engineering`, `marketing_and_content`, `customer_operations`, `research_and_intelligence`, `data_and_pipelines`. Drives dashboard grouping.
- **`InputsSchema`** — JSON Schema 2020-12, embedded as a string. The catalog compiles and memoizes it. Always set `additionalProperties: false` so future fields require a version bump rather than silently passing through.
- **`Pricing`** — `metered` or `monthly_subscription`. `BaseCents` is the per-unit price (metered) or monthly base (subscription). `OverageRule` is informational — the reconciler uses it to bucket billing meter names.
- **`SLA`** — `MaxLatencySeconds` is what the GA gate G2 measures against. Be conservative; missing the SLA on 5% of jobs is a public-status downgrade.
- **`Dispatcher`** — pure function from `(ctx, jobID, inputs)` to the operator-plane RPC params. No side effects.
- **`BillingMeter`** — Stripe usage record key. Must match the meter declared in `infra/secrets-manifest.md` for the production Stripe price.

## File 2: `<sku>_test.go` (optional but recommended)

Two test shapes:

```go
func TestMonitorWatch_Register(t *testing.T) {
	s, err := Lookup("monitor-watch@v1")
	if err != nil { t.Fatal(err) }
	if s.Pricing.Model != "monthly_subscription" {
		t.Errorf("pricing model: %s", s.Pricing.Model)
	}
}

func TestMonitorWatch_SchemaRejectsBadInputs(t *testing.T) {
	s, _ := Lookup("monitor-watch@v1")
	bad := map[string]any{"target_url": "not-a-url", "cadence_minutes": 0}
	if err := s.ValidateInputs(bad); err == nil {
		t.Fatal("expected schema rejection")
	}
}
```

The `Reset()` helper on the catalog (added in PR #4) lets a test isolate the registry if needed — call it in `TestMain` to prevent leakage across tests.

## File 3: `api/tests/scenarios/<sku>.yaml`

The test runner under `cmd/fleetctl test --tier=scenario` (operator plane) reads YAML scenarios. Mirror that shape so the operator-plane scenario test exercises the new SKU end-to-end:

```yaml
sku: monitor-watch@v1
description: Smoke-tests the daily-digest path for monitor-watch.
inputs:
  target_url: https://example.com/changelog
  cadence_minutes: 60
  notify_channels: ["email:fixture-1@example.invalid"]
assertions:
  - kind: ledger_entry_present
    phase: dispatch
    result: ok
  - kind: ledger_entry_present
    phase: complete
    result: bill
  - kind: latency_under
    seconds: 86400
```

## File 4: `docs/pricing.md` row

Append one row to the table for the SKU's category. Use the same shape as the existing rows (SKU | description | pricing | SLA | margin lever).

## CI gate

`.github/workflows/catalog-one-pr-contract.yml` walks the diff:

- If any file under `api/internal/catalog/` is added or modified, the diff may also touch only:
  - `api/tests/scenarios/<sku>.yaml`
  - `docs/pricing.md`
  - `api/internal/catalog/<sku>_test.go`
- Touching any other file in the same PR fails the check.

To split a change across two PRs (e.g., dispatcher primitive + new SKU), land the dispatcher primitive PR first; then the SKU PR references the new primitive without re-touching it.

## Review checklist for the reviewer

- [ ] SKU name is stable + lower-kebab-case.
- [ ] `additionalProperties: false` is set on the inputs schema.
- [ ] `MaxLatencySeconds` is realistic vs the operator-plane SLO; G2 will measure this in production.
- [ ] `Dispatcher` is pure (no `time.Now`, no I/O, no goroutines).
- [ ] `BillingMeter` matches an existing Stripe meter or has a follow-up PR opening one.
- [ ] Scenario YAML exercises both the happy path and at least one input-validation failure.
- [ ] Pricing row added to `docs/pricing.md` in the right category and tier.

## Worked example

The two V0 SKUs (`triage-watch`, `campaign-swarm`) are the canonical worked examples:

- `api/internal/catalog/triage_watch.go`
- `api/internal/catalog/campaign_swarm.go`

Read them before writing your first SKU.

## Change log

| Date | Author | Change |
|---|---|---|
| 2026-05-17 | TL (Wave 8.1) | Initial publication for T13.6 closeout |
