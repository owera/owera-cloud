package billing

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newDispatchSvc constructs a Service backed by in-memory SQLite + a
// fresh FakeBackend, returning both for assertion access.
func newDispatchSvc(t *testing.T) (*Service, *FakeBackend) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &FakeBackend{}
	svc, err := New(db, f)
	if err != nil {
		t.Fatalf("billing.New: %v", err)
	}
	return svc, f
}

// stubDispatcher returns canned plans by SKU.
type stubDispatcher struct {
	plans map[string]DispatchPlan
	err   error
}

func (s *stubDispatcher) Dispatch(_ context.Context, p PendingEvent) (DispatchPlan, error) {
	if s.err != nil {
		return DispatchPlan{}, s.err
	}
	plan, ok := s.plans[p.SKU]
	if !ok {
		// Default for unspecified SKUs in the test fixture.
		return DispatchPlan{Kind: DispatchKindMetered, MeterName: p.Meter}, nil
	}
	return plan, nil
}

// recordOne is a helper that submits one outbox row with controlled
// fields. Tests use it to drive Reconcile against specific shapes.
func recordOne(t *testing.T, s *Service, sku, meter, entryID string, units int64) {
	t.Helper()
	if err := s.Record(context.Background(), LedgerEvent{
		EntryID:  entryID,
		TaskID:   "task-" + entryID,
		TenantID: "ten_a",
		SKU:      sku,
		Result:   "bill",
		Units:    units,
		Meter:    meter,
		Ts:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

// TestReconcile_NoDispatcher_AllMetered preserves the pre-PR behaviour
// for callers that haven't wired a dispatcher: every outbox row goes
// through EmitUsage with the event's Meter.
func TestReconcile_NoDispatcher_AllMetered(t *testing.T) {
	svc, f := newDispatchSvc(t)
	recordOne(t, svc, "triage-watch@v1", "tickets_processed", "e1", 3)
	recordOne(t, svc, "campaign-swarm@v1", "campaigns_launched", "e2", 1)

	n, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n != 2 {
		t.Errorf("emitted: got %d, want 2", n)
	}
	if len(f.Records) != 2 {
		t.Errorf("Records: got %d, want 2", len(f.Records))
	}
	if len(f.OneShots) != 0 {
		t.Errorf("OneShots: got %d, want 0 (no dispatcher = all metered)", len(f.OneShots))
	}
}

// TestReconcile_Dispatcher_RoutesByKind: with a dispatcher wired, two
// rows route to two different backend methods based on Kind.
func TestReconcile_Dispatcher_RoutesByKind(t *testing.T) {
	svc, f := newDispatchSvc(t)
	svc.SetDispatcher(&stubDispatcher{
		plans: map[string]DispatchPlan{
			"triage-watch@v1": {
				Kind:      DispatchKindMetered,
				MeterName: "tickets_processed",
			},
			"campaign-swarm@v1": {
				Kind:        DispatchKindOneShot,
				PriceID:     "price_1TY0vSADrWLjH4u9Thr8xXtB",
				Quantity:    1,
				Description: "Acme Q3 launch",
			},
		},
	})

	recordOne(t, svc, "triage-watch@v1", "tickets_processed", "e1", 5)
	recordOne(t, svc, "campaign-swarm@v1", "campaigns_launched", "e2", 1)

	n, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n != 2 {
		t.Errorf("emitted: got %d, want 2", n)
	}
	if len(f.Records) != 1 {
		t.Errorf("Records (metered): got %d, want 1", len(f.Records))
	} else if f.Records[0].SKU != "triage-watch@v1" {
		t.Errorf("metered SKU: got %q, want triage-watch@v1", f.Records[0].SKU)
	}
	if len(f.OneShots) != 1 {
		t.Errorf("OneShots: got %d, want 1", len(f.OneShots))
	} else {
		os := f.OneShots[0]
		if os.PriceID != "price_1TY0vSADrWLjH4u9Thr8xXtB" {
			t.Errorf("oneshot PriceID: got %q", os.PriceID)
		}
		if os.Description != "Acme Q3 launch" {
			t.Errorf("oneshot Description: got %q", os.Description)
		}
	}
}

// TestReconcile_Dispatcher_Skip: Kind="skip" marks billed without
// touching the backend. Useful for free-tier handling.
func TestReconcile_Dispatcher_Skip(t *testing.T) {
	svc, f := newDispatchSvc(t)
	svc.SetDispatcher(&stubDispatcher{
		plans: map[string]DispatchPlan{
			"freebie@v1": {Kind: DispatchKindSkip},
		},
	})
	recordOne(t, svc, "freebie@v1", "demo", "e1", 1)

	n, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n != 1 {
		t.Errorf("emitted (mark billed): got %d, want 1", n)
	}
	if len(f.Records) != 0 || len(f.OneShots) != 0 {
		t.Errorf("backend touched on skip: Records=%d OneShots=%d", len(f.Records), len(f.OneShots))
	}

	// Second Reconcile must not replay (billed_at is set).
	n, _ = svc.Reconcile(context.Background())
	if n != 0 {
		t.Errorf("replay after skip: got %d, want 0", n)
	}
}

// TestReconcile_Dispatcher_ErrorAborts: a dispatcher error halts the
// loop and returns the count emitted before the error.
func TestReconcile_Dispatcher_ErrorAborts(t *testing.T) {
	svc, _ := newDispatchSvc(t)
	svc.SetDispatcher(&stubDispatcher{err: errors.New("catalog: lookup failed")})
	recordOne(t, svc, "x@v1", "y", "e1", 1)

	n, err := svc.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected error from dispatcher")
	}
	if n != 0 {
		t.Errorf("count: got %d, want 0", n)
	}
}

// TestReconcile_OneShot_QuantityDefault: dispatcher plan with
// Quantity==0 → FakeBackend records 1.
func TestReconcile_OneShot_QuantityDefault(t *testing.T) {
	svc, f := newDispatchSvc(t)
	svc.SetDispatcher(&stubDispatcher{
		plans: map[string]DispatchPlan{
			"campaign-swarm@v1": {
				Kind:    DispatchKindOneShot,
				PriceID: "price_X",
				// Quantity intentionally omitted
			},
		},
	})
	recordOne(t, svc, "campaign-swarm@v1", "campaigns_launched", "e1", 1)

	if _, err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := f.OneShots[0].Quantity; got != 1 {
		t.Errorf("default qty: got %d, want 1", got)
	}
}
