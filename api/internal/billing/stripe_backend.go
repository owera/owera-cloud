package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/billingportal/session"
	"github.com/stripe/stripe-go/v79/client"
	"github.com/stripe/stripe-go/v79/usagerecord"
)

// SubscriptionItemResolver resolves a (tenant_id, sku, meter) triple to the
// Stripe SubscriptionItem ID against which usage records are posted. WS-15
// (identity) owns the customer↔tenant mapping; the resolver is the seam
// across that boundary. Production wires a resolver backed by the identity
// store; tests pass a stub.
type SubscriptionItemResolver interface {
	ResolveSubscriptionItem(ctx context.Context, tenantID, sku, meter string) (string, error)
}

// StripeBackend implements [Backend] against the real Stripe API. It reads
// STRIPE_SECRET_KEY from the process environment at construction time;
// missing env returns an error so the failure surfaces at startup rather
// than at the first emit.
//
// EmitUsage refuses to call Stripe when the resolved Price slot is still a
// `price_TEST_*` placeholder (see stripe_ids.go); this is a hard guard
// against the live-mode key landing under the test-mode placeholder IDs.
type StripeBackend struct {
	client   *client.API
	resolver SubscriptionItemResolver
}

// NewStripeBackend constructs the Stripe-backed Backend.
func NewStripeBackend(resolver SubscriptionItemResolver) (*StripeBackend, error) {
	key := os.Getenv("STRIPE_SECRET_KEY")
	if key == "" {
		return nil, errors.New("billing: STRIPE_SECRET_KEY not set")
	}
	if resolver == nil {
		return nil, errors.New("billing: nil SubscriptionItemResolver")
	}
	c := client.New(key, nil)
	return &StripeBackend{client: c, resolver: resolver}, nil
}

// EmitUsage posts one Stripe UsageRecord with action=increment and the
// Idempotency-Key carried in ev.IdemKey.
func (b *StripeBackend) EmitUsage(ctx context.Context, ev UsageEmit) error {
	if ev.IdemKey == "" {
		return errors.New("billing: missing idempotency key")
	}
	siID, err := b.resolver.ResolveSubscriptionItem(ctx, ev.TenantID, ev.SKU, ev.Meter)
	if err != nil {
		return fmt.Errorf("billing: resolve subscription_item: %w", err)
	}
	if siID == "" {
		return fmt.Errorf("billing: no subscription_item for tenant=%s sku=%s meter=%s", ev.TenantID, ev.SKU, ev.Meter)
	}
	if strings.HasPrefix(siID, "price_TEST_") || strings.HasPrefix(siID, "si_TEST_") {
		return fmt.Errorf("billing: refusing to call Stripe with placeholder ref %q", siID)
	}
	ts := ev.Ts.Unix()
	if ev.Ts.IsZero() {
		ts = time.Now().Unix()
	}
	params := &stripe.UsageRecordParams{
		SubscriptionItem: stripe.String(siID),
		Quantity:         stripe.Int64(ev.Units),
		Timestamp:        stripe.Int64(ts),
		Action:           stripe.String(string(stripe.UsageRecordActionIncrement)),
	}
	params.SetIdempotencyKey(ev.IdemKey)
	params.Context = ctx
	if _, err := usagerecord.New(params); err != nil {
		return fmt.Errorf("billing: stripe usage record: %w", err)
	}
	return nil
}

// PortalSessionURL mints a Stripe Customer Portal session for the given
// Stripe customer and returns the redirect URL. ReturnURL is where Stripe
// sends the customer after they close the portal.
func (b *StripeBackend) PortalSessionURL(ctx context.Context, stripeCustomerID, returnURL string) (string, error) {
	if stripeCustomerID == "" {
		return "", errors.New("billing: empty stripe customer_id")
	}
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(stripeCustomerID),
		ReturnURL: stripe.String(returnURL),
	}
	params.Context = ctx
	s, err := session.New(params)
	if err != nil {
		return "", fmt.Errorf("billing: portal session: %w", err)
	}
	return s.URL, nil
}
