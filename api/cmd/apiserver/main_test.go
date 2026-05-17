package main

import (
	"strings"
	"testing"

	"github.com/owera/owera-cloud/api/internal/billing"
	"github.com/owera/owera-cloud/api/internal/dispatcher"
	"github.com/owera/owera-cloud/api/internal/identity"
)

// TestChooseWiring covers the four env-var permutations. We can't
// instantiate a real StripeBackend in CI (no live key) so the
// stripe-on case is exercised via a fake STRIPE_SECRET_KEY — that's
// sufficient for the wiring decision: NewStripeBackend reads the env
// at construction but doesn't dial Stripe until first emit.
func TestChooseWiring(t *testing.T) {
	idStore, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = idStore.Close() })

	cases := []struct {
		name           string
		stripeKey      string
		operatorRPCURL string
		wantBilling    string
		wantLedger     string
		wantPortal     bool
	}{
		{
			name:        "all unset → all fakes (dev)",
			wantBilling: "fake",
			wantLedger:  "synthetic",
		},
		{
			name:        "stripe only",
			stripeKey:   "sk_test_dummy",
			wantBilling: "stripe",
			wantLedger:  "synthetic",
			wantPortal:  true,
		},
		{
			name:           "ledger tail only",
			operatorRPCURL: "https://op.internal.owera.ai/rpc",
			wantBilling:    "fake",
			wantLedger:     "tunnel (https://op.internal.owera.ai/rpc)",
		},
		{
			name:           "both set → full prod",
			stripeKey:      "sk_test_dummy",
			operatorRPCURL: "https://op.internal.owera.ai/rpc",
			wantBilling:    "stripe",
			wantLedger:     "tunnel (https://op.internal.owera.ai/rpc)",
			wantPortal:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STRIPE_SECRET_KEY", tc.stripeKey)
			t.Setenv("OPERATOR_RPC_URL", tc.operatorRPCURL)
			// Keep this matrix hermetic across the post-APE-2 env
			// additions (CLERK_JWT_ISSUER, OWERA_DEFAULT_CAP_CENTS):
			// the parent cases predate them, so force off here.
			t.Setenv("CLERK_JWT_ISSUER", "")
			t.Setenv("OWERA_DEFAULT_CAP_CENTS", "")

			w, err := chooseWiring(idStore)
			if err != nil {
				t.Fatalf("chooseWiring: %v", err)
			}

			if w.billingLabel != tc.wantBilling {
				t.Errorf("billingLabel = %q, want %q", w.billingLabel, tc.wantBilling)
			}
			if w.ledgerLabel != tc.wantLedger {
				t.Errorf("ledgerLabel = %q, want %q", w.ledgerLabel, tc.wantLedger)
			}

			switch tc.wantBilling {
			case "fake":
				if _, ok := w.billing.(*billing.FakeBackend); !ok {
					t.Errorf("billing backend = %T, want *billing.FakeBackend", w.billing)
				}
			case "stripe":
				if _, ok := w.billing.(*billing.StripeBackend); !ok {
					t.Errorf("billing backend = %T, want *billing.StripeBackend", w.billing)
				}
			}

			switch {
			case strings.HasPrefix(tc.wantLedger, "tunnel"):
				if _, ok := w.ledger.(*dispatcher.LedgerTailClient); !ok {
					t.Errorf("ledger poller = %T, want *dispatcher.LedgerTailClient", w.ledger)
				}
			case tc.wantLedger == "synthetic":
				if _, ok := w.ledger.(*dispatcher.SyntheticLedgerPoller); !ok {
					t.Errorf("ledger poller = %T, want *dispatcher.SyntheticLedgerPoller", w.ledger)
				}
			}

			if tc.wantPortal && w.portal == nil {
				t.Errorf("portal minter is nil; want non-nil (Stripe-backed)")
			}
			if !tc.wantPortal && w.portal != nil {
				t.Errorf("portal minter is %T; want nil (no stripe key)", w.portal)
			}
		})
	}
}

// TestChooseWiring_Clerk covers the CLERK_JWT_ISSUER env-var path.
// Unset → clerk verifier is nil + label "disabled". Set → verifier is
// constructed (we can't dial a real JWKS endpoint in CI, but
// NewClerkVerifier validates the issuer string at construction and
// only does network I/O lazily, so the wiring succeeds).
func TestChooseWiring_Clerk(t *testing.T) {
	idStore, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = idStore.Close() })

	t.Run("unset", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "")
		t.Setenv("OPERATOR_RPC_URL", "")
		t.Setenv("CLERK_JWT_ISSUER", "")
		w, err := chooseWiring(idStore)
		if err != nil {
			t.Fatalf("chooseWiring: %v", err)
		}
		if w.clerk != nil {
			t.Errorf("clerk verifier = %T, want nil", w.clerk)
		}
		if w.clerkLabel != "disabled" {
			t.Errorf("clerkLabel = %q, want disabled", w.clerkLabel)
		}
	})

	t.Run("set", func(t *testing.T) {
		t.Setenv("STRIPE_SECRET_KEY", "")
		t.Setenv("OPERATOR_RPC_URL", "")
		t.Setenv("CLERK_JWT_ISSUER", "https://clerk.owera.ai")
		w, err := chooseWiring(idStore)
		if err != nil {
			t.Fatalf("chooseWiring: %v", err)
		}
		if w.clerk == nil {
			t.Errorf("clerk verifier is nil; want non-nil")
		}
		if w.clerkLabel != "clerk (https://clerk.owera.ai)" {
			t.Errorf("clerkLabel = %q", w.clerkLabel)
		}
	})
}

// TestDefaultCapCentsFromEnv covers the cap-from-env parse path.
func TestDefaultCapCentsFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want int64
	}{
		{"unset → $200 fallback", "", 20000},
		{"valid integer", "50000", 50000},
		{"zero is honored (no implicit cap)", "0", 0},
		{"negative falls back", "-1", 20000},
		{"garbage falls back", "not-a-number", 20000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OWERA_DEFAULT_CAP_CENTS", tc.env)
			if got := defaultCapCentsFromEnv(); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
