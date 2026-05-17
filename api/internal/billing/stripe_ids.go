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
// One slot is intentionally a placeholder:
//
//   - triage-watch:ticket requires a metered Stripe price, which (under
//     Stripe API ≥ 2025-03-31) must be backed by a Billing Meter object.
//     The Claude Stripe MCP does not expose the /v1/billing/meters API,
//     so the meter + matching metered price must be created in a follow-
//     up via the Stripe dashboard or a direct API call. Until then this
//     ref carries `price_PENDING_meter_setup` and Subscriber must short-
//     circuit when it sees that value (see UsageEmit guard).
//
// The Piton-Tec-account refs the earlier WS-16 PR shipped (prod_UWxqPxgISCp6QI
// + prod_UWxqGIt0waFuwb) were archived (active=false) on 2026-05-17 once
// the wrong-account state was caught; they are replaced here.
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
		PriceID:   "price_PENDING_meter_setup", // see package comment
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
