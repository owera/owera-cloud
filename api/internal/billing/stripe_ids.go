package billing

// StripeRef is the Stripe product + price slot for one SKU/meter combination.
// The Owera ref is the internal key the catalog and reconciler use; ProductID
// and PriceID are the Stripe-side opaque identifiers.
type StripeRef struct {
	OweraRef  string // e.g. "triage-watch:base", "triage-watch:ticket"
	ProductID string
	PriceID   string
	Mode      string // "test" or "live"
}

// StripeRefs is the V0 SKU → Stripe product/price map for the billing pipeline.
//
// Created 2026-05-17 in Stripe TEST mode on account acct_1TY0f6ADrWLjH4u9
// ("Owera Fleet") via the Claude Stripe MCP. All entries below are real,
// live-mode-safe (livemode: false on the Stripe objects). The Reconciler
// and Subscriber may exercise these IDs end-to-end without billing risk.
//
// triage-watch:ticket is metered against the Stripe Billing Meter
// referenced by [MeterTriageWatchTickets] — Subscriber must emit
// `meter_events` with event_name="tickets_processed" carrying
// `stripe_customer_id` and `value` payload keys.
//
// Cleanup record:
//   - prod_UWxqPxgISCp6QI (Owera triage-watch, Piton Tec live) — archived
//   - prod_UWxqGIt0waFuwb (Owera campaign-swarm, Piton Tec live) — archived
//   - prod_UX4s2ufwHsFhMV (Owera Agentic — triage-watch, Owera Fleet live
//     misfire before test-mode was confirmed) — archived
//   - price_1TY183ADrWLjH4u9Bvzx0Dbn (triage-watch:ticket at $0.02 — initial
//     dashboard-instruction math error) — archived; replaced by the $2.00
//     metered price below.
const (
	// MeterTriageWatchTickets is the Stripe Billing Meter ID for the
	// tickets_processed event. Used by the Subscriber when emitting
	// meter_events under Stripe API ≥ 2025-03-31. Event payload shape:
	//   { "stripe_customer_id": "cus_...", "value": <ticket_count> }
	MeterTriageWatchTickets = "mtr_test_61UhP1kSUm0YEN6dq41ADrWLjH4u9DgG"
)

var StripeRefs = []StripeRef{
	{
		OweraRef:  "triage-watch:base",
		ProductID: "prod_UX51tmYQqoapDb",
		PriceID:   "price_1TY0t6ADrWLjH4u99oBiv3pC", // $499/mo recurring licensed
		Mode:      "test",
	},
	{
		OweraRef:  "triage-watch:ticket",
		ProductID: "prod_UX51tmYQqoapDb",
		PriceID:   "price_1TY1BVADrWLjH4u9FmGDE6n5", // $2/ticket metered, meter=MeterTriageWatchTickets
		Mode:      "test",
	},
	{
		OweraRef:  "campaign-swarm:S",
		ProductID: "prod_UX52Sgla8ZRujL",
		PriceID:   "price_1TY0vIADrWLjH4u9Fxi2Pdyt", // $499 one_time
		Mode:      "test",
	},
	{
		OweraRef:  "campaign-swarm:M",
		ProductID: "prod_UX52Sgla8ZRujL",
		PriceID:   "price_1TY0vSADrWLjH4u9Thr8xXtB", // $999 one_time
		Mode:      "test",
	},
	{
		OweraRef:  "campaign-swarm:L",
		ProductID: "prod_UX52Sgla8ZRujL",
		PriceID:   "price_1TY0vnADrWLjH4u9PPfkd7B4", // $1,999 one_time
		Mode:      "test",
	},
}

// LookupRef returns the StripeRef for an owera ref ("sku:meter"), or zero
// value + false if unknown.
func LookupRef(oweraRef string) (StripeRef, bool) {
	for _, r := range StripeRefs {
		if r.OweraRef == oweraRef {
			return r, true
		}
	}
	return StripeRef{}, false
}
