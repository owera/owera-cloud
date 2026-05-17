package catalog

import (
	"context"
	"strings"
	"testing"

	"github.com/owera/owera-cloud/api/internal/billing"
)

func TestDispatcher_TriageWatchMetered(t *testing.T) {
	d := NewDispatcher()
	plan, err := d.Dispatch(context.Background(), billing.PendingEvent{
		EntryID:  "task_t1:0",
		TenantID: "tenant_acme",
		SKU:      "triage-watch@v1",
		Meter:    "tickets_processed",
		Units:    5,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if plan.Kind != billing.DispatchKindMetered {
		t.Fatalf("kind: got %q want %q", plan.Kind, billing.DispatchKindMetered)
	}
	if plan.MeterName != "tickets_processed" {
		t.Fatalf("meter: got %q want %q", plan.MeterName, "tickets_processed")
	}
	if plan.PriceID != "" || plan.Quantity != 0 || plan.Description != "" {
		t.Fatalf("metered plan must not set oneshot fields: %+v", plan)
	}
}

func TestDispatcher_CampaignSwarmOneShot_M(t *testing.T) {
	d := NewDispatcher()
	plan, err := d.Dispatch(context.Background(), billing.PendingEvent{
		EntryID:  "task_c9:0",
		TenantID: "tenant_acme",
		SKU:      "campaign-swarm@v1",
		Meter:    "M",
		Units:    1,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if plan.Kind != billing.DispatchKindOneShot {
		t.Fatalf("kind: got %q want %q", plan.Kind, billing.DispatchKindOneShot)
	}
	// M tier price from stripe_ids.go
	wantPrice := "price_1TY0vSADrWLjH4u9Thr8xXtB"
	if plan.PriceID != wantPrice {
		t.Fatalf("price_id: got %q want %q", plan.PriceID, wantPrice)
	}
	if plan.Quantity != 1 {
		t.Fatalf("quantity: got %d want 1", plan.Quantity)
	}
	if !strings.Contains(plan.Description, "campaign-swarm") ||
		!strings.Contains(plan.Description, "M") ||
		!strings.Contains(plan.Description, "task_c9:0") {
		t.Fatalf("description should mention sku/tier/task: %q", plan.Description)
	}
}

func TestDispatcher_CampaignSwarmOneShot_AllTiers(t *testing.T) {
	d := NewDispatcher()
	cases := []struct {
		tier      string
		wantPrice string
	}{
		{"S", "price_1TY0vIADrWLjH4u9Fxi2Pdyt"},
		{"M", "price_1TY0vSADrWLjH4u9Thr8xXtB"},
		{"L", "price_1TY0vnADrWLjH4u9PPfkd7B4"},
	}
	for _, tc := range cases {
		t.Run(tc.tier, func(t *testing.T) {
			plan, err := d.Dispatch(context.Background(), billing.PendingEvent{
				EntryID:  "task_x:0",
				TenantID: "tenant_acme",
				SKU:      "campaign-swarm@v1",
				Meter:    tc.tier,
				Units:    1,
			})
			if err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			if plan.PriceID != tc.wantPrice {
				t.Fatalf("price_id for tier %s: got %q want %q", tc.tier, plan.PriceID, tc.wantPrice)
			}
		})
	}
}

func TestDispatcher_CampaignSwarmMissingTierErrors(t *testing.T) {
	d := NewDispatcher()
	_, err := d.Dispatch(context.Background(), billing.PendingEvent{
		EntryID:  "task_x:0",
		TenantID: "tenant_acme",
		SKU:      "campaign-swarm@v1",
		Meter:    "",
		Units:    1,
	})
	if err == nil {
		t.Fatal("expected error for empty tier")
	}
}

func TestDispatcher_CampaignSwarmUnknownTierErrors(t *testing.T) {
	d := NewDispatcher()
	_, err := d.Dispatch(context.Background(), billing.PendingEvent{
		EntryID:  "task_x:0",
		TenantID: "tenant_acme",
		SKU:      "campaign-swarm@v1",
		Meter:    "XL",
		Units:    1,
	})
	if err == nil {
		t.Fatal("expected error for unknown tier")
	}
	if !strings.Contains(err.Error(), "StripeRef") {
		t.Fatalf("error should mention StripeRef: %v", err)
	}
}

func TestDispatcher_UnknownSKUErrors(t *testing.T) {
	d := NewDispatcher()
	_, err := d.Dispatch(context.Background(), billing.PendingEvent{
		EntryID: "task_x:0",
		SKU:     "no-such-sku@v9",
	})
	if err == nil {
		t.Fatal("expected error for unknown sku")
	}
}

// TestCampaignSwarm_PricingModel pins the round-trip value so future
// edits to campaign_swarm.go don't silently regress the dispatch path.
func TestCampaignSwarm_PricingModel(t *testing.T) {
	if got, want := CampaignSwarmV1.Pricing.Model, "per_job_fixed"; got != want {
		t.Fatalf("campaign-swarm Pricing.Model: got %q want %q", got, want)
	}
}

func TestTriageWatch_PricingModel(t *testing.T) {
	if got, want := TriageWatchV1.Pricing.Model, "monthly_subscription"; got != want {
		t.Fatalf("triage-watch Pricing.Model: got %q want %q", got, want)
	}
}

// TestDispatcher_SatisfiesInterface ensures *Dispatcher implements
// billing.SKUDispatcher at compile time. If billing.SKUDispatcher
// changes shape, this fails at build, not at runtime.
func TestDispatcher_SatisfiesInterface(t *testing.T) {
	var _ billing.SKUDispatcher = NewDispatcher()
}
