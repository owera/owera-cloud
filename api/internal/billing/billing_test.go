package billing

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestService(t *testing.T) (*Service, *FakeBackend) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fake := &FakeBackend{}
	s, err := New(db, fake)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, fake
}

func TestRecord_IgnoresNonBillEvents(t *testing.T) {
	s, _ := newTestService(t)
	err := s.Record(context.Background(), LedgerEvent{
		TaskID: "t1", TenantID: "ten_a", SKU: "x@v1",
		Result: "ok", Units: 1, Meter: "m", Ts: time.Now(),
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	usage, _ := s.UsageByTenant(context.Background(), "ten_a")
	if len(usage) != 0 {
		t.Fatalf("expected empty usage; got %v", usage)
	}
}

func TestRecordReconcile(t *testing.T) {
	s, fake := newTestService(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := s.Record(ctx, LedgerEvent{
			TaskID:   "task_1",
			TenantID: "ten_a",
			SKU:      "triage-watch@v1",
			Result:   "bill",
			Meter:    "tickets_processed",
			Units:    int64(i + 1),
			Ts:       time.Now().Add(time.Duration(i) * time.Millisecond),
		})
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	n, err := s.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n != 3 {
		t.Fatalf("emitted: got %d want 3", n)
	}
	if got := len(fake.Records); got != 3 {
		t.Fatalf("fake records: got %d want 3", got)
	}
}

func TestReconcile_Idempotent(t *testing.T) {
	s, fake := newTestService(t)
	ctx := context.Background()
	ts := time.Now()
	_ = s.Record(ctx, LedgerEvent{
		TaskID: "task_1", TenantID: "ten_a", SKU: "x@v1",
		Result: "bill", Meter: "m", Units: 1, Ts: ts,
	})
	_, _ = s.Reconcile(ctx)
	n, _ := s.Reconcile(ctx)
	if n != 0 {
		t.Fatalf("second reconcile: got %d want 0", n)
	}
	if len(fake.Records) != 1 {
		t.Fatalf("fake records: got %d want 1", len(fake.Records))
	}
}

func TestUsageByTenant_IsolatesTenants(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	for _, ten := range []string{"ten_a", "ten_b"} {
		_ = s.Record(ctx, LedgerEvent{
			TaskID: "t-" + ten, TenantID: ten, SKU: "x@v1",
			Result: "bill", Meter: "m", Units: 5, Ts: time.Now(),
		})
	}
	usage, _ := s.UsageByTenant(ctx, "ten_a")
	if usage["x@v1"] != 5 {
		t.Fatalf("ten_a usage: got %v", usage)
	}
	usage, _ = s.UsageByTenant(ctx, "ten_b")
	if usage["x@v1"] != 5 {
		t.Fatalf("ten_b usage: got %v", usage)
	}
}
