package catalog

import (
	"context"
	"fmt"

	"github.com/owera/owera-cloud/api/internal/billing"
)

// Dispatcher implements billing.SKUDispatcher against the global SKU
// registry. It routes a pending billing event to either a metered
// usage emit or a per-job-fixed one-shot invoice item based on the
// SKU's Pricing.Model.
//
// Wired in main.go via billing.Service.SetDispatcher. There is no
// billing → catalog import here: billing owns the SKUDispatcher
// interface; catalog merely produces an implementation.
type Dispatcher struct{}

// NewDispatcher returns a stateless Dispatcher. Safe for concurrent use.
func NewDispatcher() *Dispatcher { return &Dispatcher{} }

// Dispatch satisfies billing.SKUDispatcher. It looks the SKU up in the
// global registry and translates Pricing.Model into a DispatchPlan.
//
// For per_job_fixed SKUs the tier (S/M/L) is conventionally carried in
// PendingEvent.Meter — the operator plane writes the tier letter into
// that field on the bill event since the Meter field is unused for
// non-metered SKUs. The StripeRef key is "<sku.Name>:<tier>".
func (d *Dispatcher) Dispatch(_ context.Context, p billing.PendingEvent) (billing.DispatchPlan, error) {
	sku, err := Lookup(p.SKU)
	if err != nil {
		return billing.DispatchPlan{}, err
	}
	switch sku.Pricing.Model {
	case "metered", "monthly_subscription":
		return billing.DispatchPlan{
			Kind:      billing.DispatchKindMetered,
			MeterName: sku.BillingMeter,
		}, nil
	case "per_job_fixed":
		if p.Meter == "" {
			return billing.DispatchPlan{}, fmt.Errorf("catalog: per_job_fixed sku %s requires a tier in PendingEvent.Meter", sku.Name)
		}
		oweraRef := sku.Name + ":" + p.Meter
		ref, ok := billing.LookupRef(oweraRef)
		if !ok {
			return billing.DispatchPlan{}, fmt.Errorf("catalog: no StripeRef for %q", oweraRef)
		}
		return billing.DispatchPlan{
			Kind:        billing.DispatchKindOneShot,
			PriceID:     ref.PriceID,
			Quantity:    p.Units,
			Description: fmt.Sprintf("%s tier %s — task %s", sku.Name, p.Meter, p.EntryID),
		}, nil
	default:
		return billing.DispatchPlan{}, fmt.Errorf("catalog: unknown Pricing.Model %q on %s", sku.Pricing.Model, sku.Name)
	}
}
