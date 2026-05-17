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
	"github.com/stripe/stripe-go/v79/invoiceitem"
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

// CustomerLookup resolves a tenant_id to its Stripe customer (cus_...).
// EmitOneShot needs the customer directly — invoice items are scoped to
// a customer, not a subscription_item. The meter_events stack (PR #20)
// adds a richer CustomerResolver with sku+meter routing; this slimmer
// surface is independent of that stack so EmitOneShot can land
// standalone.
type CustomerLookup interface {
	StripeCustomerID(ctx context.Context, tenantID string) (string, error)
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
	client     *client.API
	resolver   SubscriptionItemResolver
	customers  CustomerLookup
}

// NewStripeBackend constructs the Stripe-backed Backend.
//
// customers is the (tenant_id) → cus_... lookup used by EmitOneShot.
// It may be nil in dev configurations where one-shot billing is not
// wired; EmitOneShot returns an error in that case. EmitUsage works
// independently of customers and continues to function.
func NewStripeBackend(resolver SubscriptionItemResolver, customers CustomerLookup) (*StripeBackend, error) {
	key := os.Getenv("STRIPE_SECRET_KEY")
	if key == "" {
		return nil, errors.New("billing: STRIPE_SECRET_KEY not set")
	}
	if resolver == nil {
		return nil, errors.New("billing: nil SubscriptionItemResolver")
	}
	c := client.New(key, nil)
	return &StripeBackend{client: c, resolver: resolver, customers: customers}, nil
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

// EmitOneShot creates a Stripe InvoiceItem against the tenant's
// upcoming invoice, billing the price identified by ev.PriceID. The
// item lands on the customer's next subscription invoice (or the next
// manually-finalized invoice for tenants without an active subscription).
//
// Idempotency: Stripe's Idempotency-Key prevents duplicate items when
// Reconcile retries between EmitOneShot success and the billed_at
// UPDATE. The key shape is `oneshot:{tenant_id}:{entry_id}`, set by the
// Subscriber.
//
// Refuses to call Stripe when the resolved customer or price is a
// placeholder ref — mirroring the EmitUsage guard against the
// live-mode key landing under placeholder IDs.
func (b *StripeBackend) EmitOneShot(ctx context.Context, ev OneShotEmit) error {
	if ev.IdemKey == "" {
		return errors.New("billing: missing idempotency key")
	}
	if ev.PriceID == "" {
		return errors.New("billing: missing price_id")
	}
	if strings.HasPrefix(ev.PriceID, "price_TEST_") ||
		strings.HasPrefix(ev.PriceID, "price_PENDING_") {
		return fmt.Errorf("billing: refusing to call Stripe with placeholder price %q", ev.PriceID)
	}
	if b.customers == nil {
		return errors.New("billing: EmitOneShot not configured (CustomerLookup nil)")
	}
	custID, err := b.customers.StripeCustomerID(ctx, ev.TenantID)
	if err != nil {
		return fmt.Errorf("billing: resolve customer: %w", err)
	}
	if custID == "" {
		return fmt.Errorf("billing: no customer for tenant=%s (billing onboarding incomplete)", ev.TenantID)
	}
	if strings.HasPrefix(custID, "cus_TEST_") || strings.HasPrefix(custID, "cus_PENDING_") {
		return fmt.Errorf("billing: refusing to call Stripe with placeholder customer %q", custID)
	}
	qty := ev.Quantity
	if qty == 0 {
		qty = 1
	}
	params := &stripe.InvoiceItemParams{
		Customer: stripe.String(custID),
		Price:    stripe.String(ev.PriceID),
		Quantity: stripe.Int64(qty),
	}
	if ev.Description != "" {
		params.Description = stripe.String(ev.Description)
	}
	params.SetIdempotencyKey(ev.IdemKey)
	params.Context = ctx
	if _, err := invoiceitem.New(params); err != nil {
		return fmt.Errorf("billing: stripe invoice item: %w", err)
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
