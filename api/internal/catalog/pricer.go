// pricer.go — production [billing.SKUPricer] implementation backed by
// the catalog's per-SKU `Pricing.BaseCents`. Used by `billing.CostCap`
// to estimate whether a new job would push a tenant past their cap.
//
// The shape is intentionally simple:
//
//   - If `inputs["_units"]` is set (the billing.CostCap convention for
//     "current spent in cents" rollups), multiply units × BaseCents.
//   - Otherwise treat the call as "what does ONE invocation of this SKU
//     cost?" and return BaseCents.
//
// Per-job-fixed SKUs (campaign-swarm S/M/L) all share BaseCents=0 today
// because their real price lives on a tier-specific Stripe Price ID, not
// on the SKU record. The cap-estimate path therefore under-counts those
// SKUs; that's the deliberate V0 trade-off — the live cap check happens
// at submission time and currently only enforces against metered /
// monthly-subscription cost shapes. The follow-up to make per-job-fixed
// cap-aware lands when the tier resolver moves into catalog (today it
// lives in CatalogDispatcher).
package catalog

// Pricer satisfies billing.SKUPricer over the global SKU registry.
type Pricer struct{}

// NewPricer returns a pricer ready to drop into billing.NewCostCap.
func NewPricer() *Pricer { return &Pricer{} }

// EstimateCents returns the estimated cost for one (or N, via the
// "_units" convention) invocations of sku.
func (p *Pricer) EstimateCents(sku string, inputs map[string]any) (int64, error) {
	s, err := Lookup(sku)
	if err != nil {
		return 0, err
	}
	if u, ok := inputs["_units"]; ok {
		switch v := u.(type) {
		case int64:
			return v * s.Pricing.BaseCents, nil
		case int:
			return int64(v) * s.Pricing.BaseCents, nil
		case float64:
			return int64(v) * s.Pricing.BaseCents, nil
		}
	}
	return s.Pricing.BaseCents, nil
}
