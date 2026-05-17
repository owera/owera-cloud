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
// The PriceID values prefixed `price_TEST_` are placeholders — they need to be
// replaced once a Stripe MCP / API key bound to TEST mode is wired up (the
// initial test-mode product creation could not complete because the only
// available Stripe credential is in live mode; see PR body for the follow-up).
// The ProductID values for the two services were created during T16.1 and are
// real Stripe objects; live-mode prices were intentionally NOT attached.
//
// Reconciler and Subscriber must NEVER hit live prices until the placeholders
// are replaced. The presence of `price_TEST_` is the guard.
var StripeRefs = []StripeRef{
	{
		OweraRef:  "triage-watch:base",
		ProductID: "prod_UWxqPxgISCp6QI",
		PriceID:   "price_TEST_triagewatch_base",
		Mode:      "test",
	},
	{
		OweraRef:  "triage-watch:ticket",
		ProductID: "prod_UWxqPxgISCp6QI",
		PriceID:   "price_TEST_triagewatch_ticket",
		Mode:      "test",
	},
	{
		OweraRef:  "campaign-swarm:S",
		ProductID: "prod_UWxqGIt0waFuwb",
		PriceID:   "price_TEST_campaignswarm_S",
		Mode:      "test",
	},
	{
		OweraRef:  "campaign-swarm:M",
		ProductID: "prod_UWxqGIt0waFuwb",
		PriceID:   "price_TEST_campaignswarm_M",
		Mode:      "test",
	},
	{
		OweraRef:  "campaign-swarm:L",
		ProductID: "prod_UWxqGIt0waFuwb",
		PriceID:   "price_TEST_campaignswarm_L",
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
