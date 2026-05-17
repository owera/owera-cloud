package billing

import (
	"context"
	"errors"
	"testing"
)

// TestFakeBackend_OneShot_RoundTrip exercises the FakeBackend's
// EmitOneShot fast-path: one call records one OneShot entry with the
// expected fields, quantity defaults to 1 when omitted.
func TestFakeBackend_OneShot_RoundTrip(t *testing.T) {
	f := &FakeBackend{}
	ctx := context.Background()

	err := f.EmitOneShot(ctx, OneShotEmit{
		TenantID:    "ten_a",
		SKU:         "campaign-swarm@v1",
		PriceID:     "price_1TY0vSADrWLjH4u9Thr8xXtB",
		Quantity:    0, // default
		Description: "Acme Q3 launch",
		IdemKey:     "oneshot:ten_a:t1:5",
	})
	if err != nil {
		t.Fatalf("EmitOneShot: %v", err)
	}
	if len(f.OneShots) != 1 {
		t.Fatalf("OneShots: got %d, want 1", len(f.OneShots))
	}
	got := f.OneShots[0]
	if got.TenantID != "ten_a" {
		t.Errorf("TenantID: %q", got.TenantID)
	}
	if got.SKU != "campaign-swarm@v1" {
		t.Errorf("SKU: %q", got.SKU)
	}
	if got.PriceID != "price_1TY0vSADrWLjH4u9Thr8xXtB" {
		t.Errorf("PriceID: %q", got.PriceID)
	}
	if got.Quantity != 1 {
		t.Errorf("Quantity default: got %d, want 1", got.Quantity)
	}
	if got.Description != "Acme Q3 launch" {
		t.Errorf("Description: %q", got.Description)
	}
}

// TestFakeBackend_OneShot_DedupesByIdemKey: same IdemKey within the
// fake's lifetime is a no-op, mirroring Stripe Idempotency-Key semantics.
func TestFakeBackend_OneShot_DedupesByIdemKey(t *testing.T) {
	f := &FakeBackend{}
	ctx := context.Background()
	ev := OneShotEmit{
		TenantID: "ten_a",
		SKU:      "campaign-swarm@v1",
		PriceID:  "price_X",
		Quantity: 1,
		IdemKey:  "oneshot:ten_a:t1:5",
	}
	for i := 0; i < 3; i++ {
		if err := f.EmitOneShot(ctx, ev); err != nil {
			t.Fatalf("EmitOneShot %d: %v", i, err)
		}
	}
	if len(f.OneShots) != 1 {
		t.Errorf("dedup: got %d entries, want 1", len(f.OneShots))
	}
}

// TestFakeBackend_OneShot_QuantityHonored: explicit non-zero quantity
// is preserved verbatim.
func TestFakeBackend_OneShot_QuantityHonored(t *testing.T) {
	f := &FakeBackend{}
	err := f.EmitOneShot(context.Background(), OneShotEmit{
		TenantID: "t", SKU: "s", PriceID: "p", Quantity: 7, IdemKey: "k",
	})
	if err != nil {
		t.Fatalf("EmitOneShot: %v", err)
	}
	if got := f.OneShots[0].Quantity; got != 7 {
		t.Errorf("Quantity: got %d, want 7", got)
	}
}

// stubCustomerLookup is a minimal CustomerLookup for StripeBackend
// negative-path tests. We can't exercise the Stripe call path in unit
// tests without an HTTP round-trip; instead test the input-validation
// surface and the placeholder guard.
type stubCustomerLookup struct {
	custID string
	err    error
}

func (s *stubCustomerLookup) StripeCustomerID(_ context.Context, _ string) (string, error) {
	return s.custID, s.err
}

// TestStripeBackend_OneShot_GuardsAndValidation covers the pre-flight
// checks that fire before the Stripe API call. The actual API success
// path is covered by integration tests against test-mode Stripe.
func TestStripeBackend_OneShot_GuardsAndValidation(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_dummy_for_construction")

	tests := []struct {
		name    string
		ev      OneShotEmit
		lookup  CustomerLookup
		wantMsg string
	}{
		{
			name:    "missing idem key",
			ev:      OneShotEmit{PriceID: "price_X"},
			lookup:  &stubCustomerLookup{custID: "cus_x"},
			wantMsg: "missing idempotency key",
		},
		{
			name:    "missing price id",
			ev:      OneShotEmit{IdemKey: "k"},
			lookup:  &stubCustomerLookup{custID: "cus_x"},
			wantMsg: "missing price_id",
		},
		{
			name:    "placeholder price id rejected",
			ev:      OneShotEmit{IdemKey: "k", PriceID: "price_TEST_x"},
			lookup:  &stubCustomerLookup{custID: "cus_x"},
			wantMsg: "placeholder price",
		},
		{
			name:    "nil customer lookup not configured",
			ev:      OneShotEmit{IdemKey: "k", PriceID: "price_real"},
			lookup:  nil,
			wantMsg: "EmitOneShot not configured",
		},
		{
			name:    "lookup error propagated",
			ev:      OneShotEmit{IdemKey: "k", PriceID: "price_real", TenantID: "t"},
			lookup:  &stubCustomerLookup{err: errors.New("db unreachable")},
			wantMsg: "db unreachable",
		},
		{
			name:    "empty customer = not onboarded",
			ev:      OneShotEmit{IdemKey: "k", PriceID: "price_real", TenantID: "t"},
			lookup:  &stubCustomerLookup{custID: ""},
			wantMsg: "billing onboarding incomplete",
		},
		{
			name:    "placeholder customer rejected",
			ev:      OneShotEmit{IdemKey: "k", PriceID: "price_real", TenantID: "t"},
			lookup:  &stubCustomerLookup{custID: "cus_TEST_x"},
			wantMsg: "placeholder customer",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := NewStripeBackend(&stubSubItemResolver{}, tc.lookup)
			if err != nil {
				t.Fatalf("NewStripeBackend: %v", err)
			}
			err = b.EmitOneShot(context.Background(), tc.ev)
			if err == nil {
				t.Fatal("expected error")
			}
			if !contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

// stubSubItemResolver satisfies the legacy SubscriptionItemResolver
// dep that NewStripeBackend still requires (until the meter_events
// stack lands).
type stubSubItemResolver struct{}

func (s *stubSubItemResolver) ResolveSubscriptionItem(_ context.Context, _, _, _ string) (string, error) {
	return "si_test_dummy", nil
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
