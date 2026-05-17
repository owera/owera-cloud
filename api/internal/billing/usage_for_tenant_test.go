package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/client"
)

// stubResolver fakes the production IdentityCustomerResolver: maps one
// tenant → one Stripe customer id, regardless of sku/meter. Empty id
// signals "not billing-onboarded" — the same not-found semantics the
// real resolver exposes.
type stubResolver struct{ custID string }

func (s stubResolver) ResolveCustomer(_ context.Context, _, _, _ string) (string, error) {
	return s.custID, nil
}

// TestUsageForTenant exercises StripeBackend.UsageForTenant end-to-end
// against an httptest server returning fixture JSON for the two
// endpoints the implementation hits:
//
//   - GET /v1/subscriptions?customer=cus_… → one subscription with two
//     subscription_items (one metered, one licensed)
//   - GET /v1/subscription_items/{si_metered}/usage_record_summaries →
//     three summaries: one fully inside the window, one partially
//     overlapping, one fully outside
//
// The licensed item must be skipped (no usage_record_summaries query
// against it); the outside-window summary must not contribute. The
// expected sum is 100 + 50 = 150.
func TestUsageForTenant(t *testing.T) {
	const (
		custID         = "cus_live_acme"
		subID          = "sub_acme_1"
		meteredItemID  = "si_metered"
		licensedItemID = "si_licensed"
	)
	periodStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Summary 1: fully inside window, total 100.
	// Summary 2: partial overlap (starts before, ends inside), total 50.
	// Summary 3: fully outside (entirely before periodStart), total 999.
	// Expected sum: 150. Summary 3 must be excluded.
	insideStart := periodStart.Add(24 * time.Hour).Unix()
	insideEnd := periodStart.Add(48 * time.Hour).Unix()
	partialStart := periodStart.Add(-24 * time.Hour).Unix()
	partialEnd := periodStart.Add(12 * time.Hour).Unix()
	outsideStart := periodStart.Add(-48 * time.Hour).Unix()
	outsideEnd := periodStart.Add(-24 * time.Hour).Unix()

	var (
		gotSubsCustomer string
		usageHits       int
		licensedHit     bool
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		gotSubsCustomer = r.URL.Query().Get("customer")
		resp := map[string]any{
			"object":   "list",
			"has_more": false,
			"data": []map[string]any{
				{
					"id":     subID,
					"object": "subscription",
					"items": map[string]any{
						"object":   "list",
						"has_more": false,
						"data": []map[string]any{
							{
								"id":     meteredItemID,
								"object": "subscription_item",
								"price": map[string]any{
									"id":     "price_metered",
									"object": "price",
									"recurring": map[string]any{
										"interval":   "month",
										"usage_type": "metered",
									},
								},
							},
							{
								"id":     licensedItemID,
								"object": "subscription_item",
								"price": map[string]any{
									"id":     "price_licensed",
									"object": "price",
									"recurring": map[string]any{
										"interval":   "month",
										"usage_type": "licensed",
									},
								},
							},
						},
					},
				},
			},
		}
		writeJSON(t, w, resp)
	})
	mux.HandleFunc("/v1/subscription_items/", func(w http.ResponseWriter, r *http.Request) {
		// Routes:
		//   /v1/subscription_items/si_metered/usage_record_summaries  → 3 summaries
		//   /v1/subscription_items/si_licensed/usage_record_summaries → must not be called
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, "/v1/subscription_items/"+meteredItemID+"/usage_record_summaries"):
			usageHits++
			resp := map[string]any{
				"object":   "list",
				"has_more": false,
				"data": []map[string]any{
					{
						"id":                "urs_inside",
						"object":            "usage_record_summary",
						"subscription_item": meteredItemID,
						"total_usage":       100,
						"period": map[string]any{
							"start": insideStart,
							"end":   insideEnd,
						},
					},
					{
						"id":                "urs_partial",
						"object":            "usage_record_summary",
						"subscription_item": meteredItemID,
						"total_usage":       50,
						"period": map[string]any{
							"start": partialStart,
							"end":   partialEnd,
						},
					},
					{
						"id":                "urs_outside",
						"object":            "usage_record_summary",
						"subscription_item": meteredItemID,
						"total_usage":       999,
						"period": map[string]any{
							"start": outsideStart,
							"end":   outsideEnd,
						},
					},
				},
			}
			writeJSON(t, w, resp)
		case strings.HasPrefix(path, "/v1/subscription_items/"+licensedItemID+"/usage_record_summaries"):
			licensedHit = true
			writeJSON(t, w, map[string]any{"object": "list", "data": []any{}})
		default:
			t.Fatalf("unexpected request: %s", path)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	backends := stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		URL:        stripe.String(srv.URL),
		HTTPClient: srv.Client(),
	})
	sc := client.New("sk_test_fake", backends)

	b, err := newStripeBackendWithClient(sc, stubResolver{custID: custID})
	if err != nil {
		t.Fatalf("newStripeBackendWithClient: %v", err)
	}

	got, err := b.UsageForTenant(context.Background(), "ten_acme", periodStart, periodEnd)
	if err != nil {
		t.Fatalf("UsageForTenant: %v", err)
	}
	if want := int64(150); got != want {
		t.Errorf("UsageForTenant = %d, want %d (100 inside + 50 partial overlap)", got, want)
	}
	if gotSubsCustomer != custID {
		t.Errorf("subscriptions?customer = %q, want %q", gotSubsCustomer, custID)
	}
	if usageHits != 1 {
		t.Errorf("usage_record_summaries hits for metered item = %d, want 1", usageHits)
	}
	if licensedHit {
		t.Errorf("usage_record_summaries was queried for the licensed item; it should be skipped")
	}
}

// TestUsageForTenant_PlaceholderCustomerReturnsZero verifies the
// cus_TEST_/cus_PENDING_ guard: UsageForTenant returns 0 + nil without
// touching Stripe. The test fails the HTTP handler on any hit; if the
// guard were broken the test would fatal.
func TestUsageForTenant_PlaceholderCustomerReturnsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("Stripe was called for a placeholder customer: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	backends := stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		URL:        stripe.String(srv.URL),
		HTTPClient: srv.Client(),
	})
	sc := client.New("sk_test_fake", backends)

	for _, custID := range []string{"cus_TEST_acme", "cus_PENDING_acme", ""} {
		b, err := newStripeBackendWithClient(sc, stubResolver{custID: custID})
		if err != nil {
			t.Fatalf("newStripeBackendWithClient: %v", err)
		}
		got, err := b.UsageForTenant(context.Background(), "ten_acme",
			time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Errorf("UsageForTenant(custID=%q) err = %v, want nil", custID, err)
		}
		if got != 0 {
			t.Errorf("UsageForTenant(custID=%q) = %d, want 0", custID, got)
		}
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}
