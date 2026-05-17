package billing

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TenantCapStore is the identity-side surface CostCap needs. WS-15
// (identity) is expected to add a matching method; until it does, this
// interface lets billing compile and test in isolation. The contract:
// GetMonthlyCapCents returns the tenant's cents/month spend cap; a zero
// value means "use the default", a negative value means "no cap".
type TenantCapStore interface {
	GetMonthlyCapCents(ctx context.Context, tenantID string) (int64, error)
}

// SKUPricer estimates the cents an in-flight job will cost so the cap can
// be enforced at submission time. Implemented in production by the catalog
// package (one method off PricingTier); tests pass a stub.
type SKUPricer interface {
	EstimateCents(sku string, inputs map[string]any) (int64, error)
}

// CostCap enforces the monthly spend cap. Construct one CostCap per
// process; the spend numerator is read live from the Service's outbox.
type CostCap struct {
	svc         *Service
	caps        TenantCapStore
	pricer      SKUPricer
	defaultCaps int64 // cents/month default when the store returns 0
	now         func() time.Time
}

// NewCostCap wires the cap enforcer. defaultCapCents is the cap applied
// when the tenant has not set their own. nowFn is the clock; pass nil to
// use time.Now.
func NewCostCap(svc *Service, caps TenantCapStore, pricer SKUPricer, defaultCapCents int64, nowFn func() time.Time) (*CostCap, error) {
	if svc == nil {
		return nil, errors.New("billing: costcap nil service")
	}
	if caps == nil {
		return nil, errors.New("billing: costcap nil cap store")
	}
	if pricer == nil {
		return nil, errors.New("billing: costcap nil pricer")
	}
	if defaultCapCents <= 0 {
		return nil, errors.New("billing: costcap default must be > 0")
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	return &CostCap{
		svc:         svc,
		caps:        caps,
		pricer:      pricer,
		defaultCaps: defaultCapCents,
		now:         nowFn,
	}, nil
}

// CapExceededError is returned by Enforce when the projected cents-spent
// for the current billing period would exceed the tenant's cap. The
// server middleware translates this to HTTP 402.
//
// RetryAfter is the time the cap resets (the start of the next calendar
// month, UTC) — exposed so the handler can populate Retry-After.
type CapExceededError struct {
	TenantID       string
	CapCents       int64
	SpentCents     int64
	ProjectedCents int64
	RetryAfter     time.Time
}

func (e *CapExceededError) Error() string {
	return fmt.Sprintf("billing: cap exceeded for tenant=%s: cap=%d spent=%d projected=%d",
		e.TenantID, e.CapCents, e.SpentCents, e.ProjectedCents)
}

// Enforce checks whether tenantID can afford one more job with the given
// SKU+inputs in the current period. nil → allowed; *CapExceededError →
// 402 path; other errors → 500.
func (c *CostCap) Enforce(ctx context.Context, tenantID, sku string, inputs map[string]any) error {
	if tenantID == "" {
		return errors.New("billing: costcap empty tenant_id")
	}
	cap, err := c.caps.GetMonthlyCapCents(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("billing: cost cap lookup: %w", err)
	}
	if cap == 0 {
		cap = c.defaultCaps
	}
	if cap < 0 {
		return nil // explicit "no cap" sentinel
	}
	periodStart, periodEnd := monthBounds(c.now().UTC())
	spent, err := c.svc.TenantPeriodSum(ctx, tenantID, periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("billing: cost cap spend lookup: %w", err)
	}
	// Outbox stores units (per the meter), but the cap is denominated in
	// cents. The pricer converts the projected cost; the spent-side
	// conversion is the same pricer applied to each row in aggregate. For
	// V0 SKUs the pricer treats one unit as one billable event at the
	// SKU's flat rate.
	spentCents, err := c.pricer.EstimateCents(sku, map[string]any{"_units": spent})
	if err != nil {
		spentCents = spent // graceful fallback if pricer can't handle aggregate
	}
	estimate, err := c.pricer.EstimateCents(sku, inputs)
	if err != nil {
		return fmt.Errorf("billing: estimate: %w", err)
	}
	projected := spentCents + estimate
	if projected > cap {
		return &CapExceededError{
			TenantID:       tenantID,
			CapCents:       cap,
			SpentCents:     spentCents,
			ProjectedCents: projected,
			RetryAfter:     periodEnd,
		}
	}
	return nil
}

// monthBounds returns [first-of-month, first-of-next-month) UTC for t.
func monthBounds(t time.Time) (time.Time, time.Time) {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return start, end
}
