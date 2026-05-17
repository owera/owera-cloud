package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/billing/meterevent"
	"github.com/stripe/stripe-go/v79/billingportal/session"
	"github.com/stripe/stripe-go/v79/client"
	"github.com/stripe/stripe-go/v79/invoiceitem"
	"github.com/stripe/stripe-go/v79/subscription"
	"github.com/stripe/stripe-go/v79/usagerecordsummary"
)

// CustomerResolver maps (tenant_id, sku, meter) to the Stripe customer
// (cus_...) that a billing event should attribute to. WS-15 (identity)
// owns the tenant↔customer mapping; the resolver is the seam across
// that boundary. EmitUsage uses the sku+meter context; EmitOneShot
// passes empty strings for those fields since one_time prices aren't
// meter-scoped.
//
// Under Stripe API ≥ 2025-03-31, metered prices bind through a Meter
// object (customer_mapping.event_payload_key default = "stripe_customer_id")
// rather than a SubscriptionItem — so the resolver yields a customer
// reference, not a subscription_item.
type CustomerResolver interface {
	ResolveCustomer(ctx context.Context, tenantID, sku, meter string) (string, error)
}

// StripeBackend implements [Backend] against the real Stripe API. It reads
// STRIPE_SECRET_KEY from the process environment at construction time;
// missing env returns an error so the failure surfaces at startup rather
// than at the first emit.
//
// Both EmitUsage and EmitOneShot refuse to call Stripe when the resolved
// customer reference is a `cus_TEST_*` placeholder OR a `cus_PENDING_*`
// slot; this is a hard guard against the live-mode key landing under a
// placeholder. EmitOneShot adds the same guard for `price_TEST_*` /
// `price_PENDING_*` price IDs.
type StripeBackend struct {
	client   *client.API
	resolver CustomerResolver
}

// NewStripeBackend constructs the Stripe-backed Backend.
func NewStripeBackend(resolver CustomerResolver) (*StripeBackend, error) {
	key := os.Getenv("STRIPE_SECRET_KEY")
	if key == "" {
		return nil, errors.New("billing: STRIPE_SECRET_KEY not set")
	}
	if resolver == nil {
		return nil, errors.New("billing: nil CustomerResolver")
	}
	c := client.New(key, nil)
	return &StripeBackend{client: c, resolver: resolver}, nil
}

// newStripeBackendWithClient builds a StripeBackend around an explicit
// *client.API. Unexported because production code should always go
// through NewStripeBackend (which reads the env key and uses the
// default Stripe backends); the override exists so unit tests can point
// the SDK at an httptest server.
func newStripeBackendWithClient(c *client.API, resolver CustomerResolver) (*StripeBackend, error) {
	if c == nil {
		return nil, errors.New("billing: nil stripe client")
	}
	if resolver == nil {
		return nil, errors.New("billing: nil CustomerResolver")
	}
	return &StripeBackend{client: c, resolver: resolver}, nil
}

// EmitUsage posts one Stripe billing_meter_event with event_name
// matching the operator-plane Meter and a payload carrying the
// resolved stripe_customer_id and the numeric value. Idempotency is
// provided via the `identifier` field — Stripe enforces uniqueness
// over a 24-hour rolling window keyed on the meter + event_name +
// identifier triple.
func (b *StripeBackend) EmitUsage(ctx context.Context, ev UsageEmit) error {
	if ev.IdemKey == "" {
		return errors.New("billing: missing idempotency key")
	}
	if ev.Meter == "" {
		return errors.New("billing: missing meter")
	}
	custID, err := b.resolver.ResolveCustomer(ctx, ev.TenantID, ev.SKU, ev.Meter)
	if err != nil {
		return fmt.Errorf("billing: resolve customer: %w", err)
	}
	if custID == "" {
		return fmt.Errorf("billing: no stripe customer for tenant=%s sku=%s meter=%s", ev.TenantID, ev.SKU, ev.Meter)
	}
	if strings.HasPrefix(custID, "cus_TEST_") || strings.HasPrefix(custID, "cus_PENDING_") {
		return fmt.Errorf("billing: refusing to call Stripe with placeholder customer ref %q", custID)
	}
	params := buildMeterEventParams(ev, custID, time.Now)
	params.Context = ctx
	if _, err := meterevent.New(params); err != nil {
		return fmt.Errorf("billing: stripe meter_event: %w", err)
	}
	return nil
}

// buildMeterEventParams composes the BillingMeterEventParams from a
// UsageEmit + resolved customer id. Extracted from EmitUsage so the
// parameter shape can be unit-tested without an HTTP round trip
// against Stripe. now is the clock the zero-Ts fallback consults.
func buildMeterEventParams(ev UsageEmit, custID string, now func() time.Time) *stripe.BillingMeterEventParams {
	ts := ev.Ts.Unix()
	if ev.Ts.IsZero() {
		ts = now().Unix()
	}
	return &stripe.BillingMeterEventParams{
		EventName:  stripe.String(ev.Meter),
		Identifier: stripe.String(ev.IdemKey),
		Timestamp:  stripe.Int64(ts),
		Payload: map[string]string{
			"stripe_customer_id": custID,
			"value":              strconv.FormatInt(ev.Units, 10),
		},
	}
}

// EmitOneShot creates a Stripe InvoiceItem against the tenant's
// upcoming invoice, billing the price identified by ev.PriceID. The
// item lands on the customer's next subscription invoice (or the next
// manually-finalized invoice for tenants without an active subscription).
//
// Idempotency: Stripe's Idempotency-Key prevents duplicate items when
// Reconcile retries between EmitOneShot success and the billed_at
// UPDATE. The key shape is `oneshot:{tenant_id}:{entry_id}`, set by
// the Subscriber.
//
// Customer resolution goes through the same CustomerResolver used by
// EmitUsage — sku is passed for routing parity, meter is empty since
// one_time prices are not meter-scoped.
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
	custID, err := b.resolver.ResolveCustomer(ctx, ev.TenantID, ev.SKU, "")
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

// UsageForTenant implements [StripeUsageReporter] against the live
// Stripe API. It resolves the tenant's Stripe customer via the same
// CustomerResolver used by EmitUsage, lists the customer's active
// subscriptions, picks out every metered subscription item (Price with
// recurring.usage_type=="metered" or a non-empty meter binding), pulls
// /v1/subscription_items/{id}/usage_record_summaries for each, and
// returns the sum of total_usage over the summaries whose period
// intersects [periodStart, periodEnd].
//
// Placeholder customer refs (cus_TEST_* / cus_PENDING_*) return 0 with
// nil error — same posture as EmitUsage: we never touch Stripe with a
// pre-production customer id. An empty resolver result (tenant exists
// but has no Stripe customer yet) also returns 0.
//
// Period overlap is checked inclusively: any summary whose
// [period.start, period.end] overlaps [periodStart, periodEnd]
// contributes its total_usage to the sum. This matches the reconciler's
// drift semantics — the ledger sum and the Stripe sum cover the same
// window even when Stripe's billing-period boundaries don't line up
// with the reconciler's calendar window.
func (b *StripeBackend) UsageForTenant(ctx context.Context, tenantID string, periodStart, periodEnd time.Time) (int64, error) {
	if tenantID == "" {
		return 0, errors.New("billing: empty tenant_id")
	}
	custID, err := b.resolver.ResolveCustomer(ctx, tenantID, "", "")
	if err != nil {
		return 0, fmt.Errorf("billing: resolve customer: %w", err)
	}
	if custID == "" {
		// Tenant exists but isn't billing-onboarded — nothing for Stripe
		// to report on. Return 0 so the reconciler compares ledger-vs-0
		// (which surfaces as drift if the ledger has rows).
		return 0, nil
	}
	if strings.HasPrefix(custID, "cus_TEST_") || strings.HasPrefix(custID, "cus_PENDING_") {
		return 0, nil
	}

	itemIDs, err := b.listMeteredSubscriptionItems(ctx, custID)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, itemID := range itemIDs {
		n, err := b.sumUsageForItem(ctx, itemID, periodStart, periodEnd)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// listMeteredSubscriptionItems returns the IDs of every metered
// subscription item across all active subscriptions for custID. Both
// the legacy usage_type=="metered" marker and the post-2025-03-31
// Meter binding (Price.Recurring.Meter != "") are treated as metered;
// either makes a subscription_item eligible for usage_record_summaries.
func (b *StripeBackend) listMeteredSubscriptionItems(ctx context.Context, custID string) ([]string, error) {
	subs := subscription.Client{B: b.client.Subscriptions.B, Key: b.client.Subscriptions.Key}
	params := &stripe.SubscriptionListParams{
		Customer: stripe.String(custID),
		Status:   stripe.String("all"),
	}
	params.Context = ctx
	var out []string
	it := subs.List(params)
	for it.Next() {
		sub := it.Subscription()
		if sub == nil || sub.Items == nil {
			continue
		}
		for _, item := range sub.Items.Data {
			if item == nil || item.Price == nil || item.Price.Recurring == nil {
				continue
			}
			rec := item.Price.Recurring
			if rec.UsageType == stripe.PriceRecurringUsageTypeMetered || rec.Meter != "" {
				out = append(out, item.ID)
			}
		}
	}
	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("billing: list subscriptions: %w", err)
	}
	return out, nil
}

// sumUsageForItem fetches usage_record_summaries for one subscription
// item and returns the sum of total_usage over summaries whose Period
// overlaps [periodStart, periodEnd]. A summary with a nil Period or
// zero start/end timestamps is conservatively included — Stripe emits
// such summaries for the in-progress period before the first invoice.
func (b *StripeBackend) sumUsageForItem(ctx context.Context, itemID string, periodStart, periodEnd time.Time) (int64, error) {
	urs := usagerecordsummary.Client{B: b.client.UsageRecordSummaries.B, Key: b.client.UsageRecordSummaries.Key}
	params := &stripe.UsageRecordSummaryListParams{
		SubscriptionItem: stripe.String(itemID),
	}
	params.Context = ctx
	var total int64
	it := urs.List(params)
	for it.Next() {
		s := it.UsageRecordSummary()
		if s == nil {
			continue
		}
		if !summaryOverlaps(s, periodStart, periodEnd) {
			continue
		}
		total += s.TotalUsage
	}
	if err := it.Err(); err != nil {
		return 0, fmt.Errorf("billing: list usage summaries item=%s: %w", itemID, err)
	}
	return total, nil
}

// summaryOverlaps returns true when the summary's billing period
// intersects [periodStart, periodEnd]. A missing/zero period is treated
// as overlapping — the reconciler errs on the side of attributing
// in-progress usage to the current window.
func summaryOverlaps(s *stripe.UsageRecordSummary, periodStart, periodEnd time.Time) bool {
	if s.Period == nil || (s.Period.Start == 0 && s.Period.End == 0) {
		return true
	}
	start := time.Unix(s.Period.Start, 0).UTC()
	end := time.Unix(s.Period.End, 0).UTC()
	// Two intervals [a,b] and [c,d] overlap iff a <= d && c <= b.
	return !start.After(periodEnd) && !periodStart.After(end)
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
