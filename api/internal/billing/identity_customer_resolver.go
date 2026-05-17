// Package billing — identity_customer_resolver.go provides the
// production CustomerResolver, backed by the identity store. V0 maps
// (tenant_id) → cus_... regardless of sku/meter — every meter event for
// a tenant attributes to the same Stripe customer. The sku/meter
// arguments on the CustomerResolver interface are preserved for V2+
// where per-SKU customer accounts (Stripe Connect) may matter.
package billing

import (
	"context"
	"errors"
	"fmt"
)

// IdentityCustomerLookup is the slice of identity.Store the resolver
// needs. Stated as an interface so tests can stub it without spinning
// up a real SQLite identity store.
type IdentityCustomerLookup interface {
	StripeCustomerID(ctx context.Context, tenantID string) (string, error)
}

// IdentityCustomerResolver adapts an IdentityCustomerLookup to the
// CustomerResolver interface. ErrNotFound from the identity store is
// surfaced verbatim so callers can distinguish "tenant doesn't exist"
// from "tenant exists but isn't billing-onboarded yet."
type IdentityCustomerResolver struct {
	id IdentityCustomerLookup
}

// NewIdentityCustomerResolver wires the resolver. id is typically the
// process-wide *identity.Store.
func NewIdentityCustomerResolver(id IdentityCustomerLookup) (*IdentityCustomerResolver, error) {
	if id == nil {
		return nil, errors.New("billing: nil IdentityCustomerLookup")
	}
	return &IdentityCustomerResolver{id: id}, nil
}

// ResolveCustomer implements CustomerResolver. Returns the empty string
// + nil error when the tenant exists but isn't billing-onboarded —
// EmitUsage treats that as a configuration condition (no charge), not
// a programmer error.
func (r *IdentityCustomerResolver) ResolveCustomer(ctx context.Context, tenantID, _ /*sku*/, _ /*meter*/ string) (string, error) {
	if tenantID == "" {
		return "", errors.New("billing: empty tenant_id")
	}
	custID, err := r.id.StripeCustomerID(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("billing: identity lookup: %w", err)
	}
	return custID, nil
}
