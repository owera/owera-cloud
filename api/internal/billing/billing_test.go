package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
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

func mkEvent(i int, tenant string) LedgerEvent {
	return LedgerEvent{
		EntryID:  fmt.Sprintf("task_1:%d", i),
		TaskID:   "task_1",
		TenantID: tenant,
		SKU:      "triage-watch@v1",
		Result:   "bill",
		Meter:    "tickets_processed",
		Units:    int64(i + 1),
		Ts:       time.Now().UTC().Add(time.Duration(i) * time.Millisecond),
	}
}

func TestRecord_IgnoresNonBillEvents(t *testing.T) {
	s, _ := newTestService(t)
	err := s.Record(context.Background(), LedgerEvent{
		EntryID: "t1:0", TaskID: "t1", TenantID: "ten_a", SKU: "x@v1",
		Result: "ok", Units: 1, Meter: "m", Ts: time.Now().UTC(),
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
		if err := s.Record(ctx, mkEvent(i, "ten_a")); err != nil {
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
	// Every emit carries a unique idempotency key derived from the entry id.
	for _, r := range fake.Records {
		if !strings.HasPrefix(r.IdemKey, "usage:ten_a:task_1:") {
			t.Fatalf("unexpected idem key: %q", r.IdemKey)
		}
	}
}

func TestReconcile_IdempotentAcrossRetries(t *testing.T) {
	s, fake := newTestService(t)
	ctx := context.Background()
	ev := mkEvent(0, "ten_a")
	_ = s.Record(ctx, ev)
	_, _ = s.Reconcile(ctx)
	n, _ := s.Reconcile(ctx)
	if n != 0 {
		t.Fatalf("second reconcile: got %d want 0", n)
	}
	if len(fake.Records) != 1 {
		t.Fatalf("fake records: got %d want 1", len(fake.Records))
	}
	// Even if the same ledger event is Recorded twice, the unique entry_id
	// index prevents a second outbox row.
	if err := s.Record(ctx, ev); err != nil {
		t.Fatalf("Record duplicate: %v", err)
	}
	n, _ = s.Reconcile(ctx)
	if n != 0 {
		t.Fatalf("third reconcile after duplicate: got %d want 0", n)
	}
	if len(fake.Records) != 1 {
		t.Fatalf("fake records after duplicate: got %d want 1", len(fake.Records))
	}
}

func TestFakeBackend_DedupesByIdemKey(t *testing.T) {
	f := &FakeBackend{}
	ctx := context.Background()
	emit := UsageEmit{TenantID: "ten_a", SKU: "x@v1", Meter: "m", Units: 1, IdemKey: "k1"}
	_ = f.EmitUsage(ctx, emit)
	_ = f.EmitUsage(ctx, emit)
	if len(f.Records) != 1 {
		t.Fatalf("expected 1 record after dedupe; got %d", len(f.Records))
	}
}

func TestUsageByTenant_IsolatesTenants(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	for _, ten := range []string{"ten_a", "ten_b"} {
		_ = s.Record(ctx, LedgerEvent{
			EntryID: "t-" + ten, TaskID: "t-" + ten, TenantID: ten, SKU: "x@v1",
			Result: "bill", Meter: "m", Units: 5, Ts: time.Now().UTC(),
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

// --- T16.3 Subscriber tests ---

func TestSubscriber_EndToEnd(t *testing.T) {
	svc, fake := newTestService(t)
	ch := make(chan LedgerEvent, 2)
	src := NewChannelSource(ch)
	sub := NewSubscriber(src, svc, nil)

	ch <- mkEvent(0, "ten_a")
	ch <- LedgerEvent{ // non-bill event should ack but not record
		EntryID: "task_1:99", TaskID: "task_1", TenantID: "ten_a",
		SKU: "x@v1", Result: "ok", Units: 0, Meter: "m", Ts: time.Now().UTC(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	go func() { _ = sub.Run(ctx) }()
	<-ctx.Done()
	sub.Stop()

	if _, err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(fake.Records) != 1 {
		t.Fatalf("emitted: got %d want 1", len(fake.Records))
	}
	acked := src.Acked()
	if len(acked) != 2 {
		t.Fatalf("acked: got %v want 2 entries", acked)
	}
}

// --- T16.4 Reconciler tests ---

type fakeReporter struct {
	per map[string]int64
}

func (f *fakeReporter) UsageForTenant(_ context.Context, t string, _, _ time.Time) (int64, error) {
	return f.per[t], nil
}

type fakeAlerter struct {
	mu     sync.Mutex
	alerts []map[string]any
	kinds  []string
}

func (a *fakeAlerter) Alert(_ context.Context, kind string, payload map[string]any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.kinds = append(a.kinds, kind)
	a.alerts = append(a.alerts, payload)
	return nil
}

func TestReconciler_NoDriftIsSilent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	_ = svc.Record(ctx, mkEvent(0, "ten_a")) // units=1
	_ = svc.Record(ctx, mkEvent(1, "ten_a")) // units=2 → sum 3

	rep := &fakeReporter{per: map[string]int64{"ten_a": 3}}
	al := &fakeAlerter{}
	rec, err := NewReconciler(svc, rep, al)
	if err != nil {
		t.Fatalf("NewReconciler: %v", err)
	}
	now := time.Now().UTC()
	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)
	reports, err := rec.Run(ctx, start, end)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(reports) != 1 || reports[0].Alerted {
		t.Fatalf("expected 1 unalerted report; got %+v", reports)
	}
	if len(al.alerts) != 0 {
		t.Fatalf("expected zero alerts on no-drift; got %d", len(al.alerts))
	}
}

func TestReconciler_DriftAlerts(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	// Insert 1000 units worth of ledger events for ten_a.
	for i := 0; i < 1000; i++ {
		_ = svc.Record(ctx, LedgerEvent{
			EntryID:  fmt.Sprintf("task_X:%d", i),
			TaskID:   "task_X",
			TenantID: "ten_a",
			SKU:      "x@v1", Result: "bill", Meter: "m",
			Units: 1, Ts: time.Now().UTC(),
		})
	}
	// Stripe reports 900 → drift = 100/1000 = 10% > 0.5%
	rep := &fakeReporter{per: map[string]int64{"ten_a": 900}}
	al := &fakeAlerter{}
	rec, _ := NewReconciler(svc, rep, al)
	now := time.Now().UTC()
	reports, err := rec.Run(ctx, now.Add(-1*time.Hour), now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(reports) != 1 || !reports[0].Alerted {
		t.Fatalf("expected 1 alerted report; got %+v", reports)
	}
	if len(al.alerts) != 1 {
		t.Fatalf("expected 1 alert; got %d", len(al.alerts))
	}
	if al.kinds[0] != "billing.reconcile.drift" {
		t.Fatalf("alert kind: got %q", al.kinds[0])
	}
}

func TestReconciler_BelowThresholdIsSilent(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		_ = svc.Record(ctx, LedgerEvent{
			EntryID:  fmt.Sprintf("task_Y:%d", i),
			TaskID:   "task_Y",
			TenantID: "ten_a",
			SKU:      "x@v1", Result: "bill", Meter: "m",
			Units: 1, Ts: time.Now().UTC(),
		})
	}
	// 1000 ledger, 997 Stripe → 0.3% drift, below threshold
	rep := &fakeReporter{per: map[string]int64{"ten_a": 997}}
	al := &fakeAlerter{}
	rec, _ := NewReconciler(svc, rep, al)
	now := time.Now().UTC()
	reports, err := rec.Run(ctx, now.Add(-1*time.Hour), now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reports[0].Alerted {
		t.Fatalf("expected unalerted at 0.3%% drift; got %+v", reports[0])
	}
	if len(al.alerts) != 0 {
		t.Fatalf("expected zero alerts; got %d", len(al.alerts))
	}
}

// --- T16.5 CostCap tests ---

type fakeCaps struct{ v int64 }

func (f *fakeCaps) GetMonthlyCapCents(context.Context, string) (int64, error) {
	return f.v, nil
}

type fakePricer struct {
	per int64
}

func (p *fakePricer) EstimateCents(_ string, inputs map[string]any) (int64, error) {
	if u, ok := inputs["_units"]; ok {
		if i64, ok := u.(int64); ok {
			return i64 * p.per, nil
		}
	}
	return p.per, nil
}

func TestCostCap_Allows(t *testing.T) {
	svc, _ := newTestService(t)
	cap, err := NewCostCap(svc, &fakeCaps{v: 100_000}, &fakePricer{per: 200}, 50_000, nil)
	if err != nil {
		t.Fatalf("NewCostCap: %v", err)
	}
	if err := cap.Enforce(context.Background(), "ten_a", "x@v1", nil); err != nil {
		t.Fatalf("Enforce: %v", err)
	}
}

func TestCostCap_Exceeds_Returns402(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	// Insert 600 units → spent = 600 * 200 cents = 120_000 cents (> 100_000 cap)
	for i := 0; i < 600; i++ {
		_ = svc.Record(ctx, LedgerEvent{
			EntryID:  fmt.Sprintf("task_C:%d", i),
			TaskID:   "task_C",
			TenantID: "ten_a",
			SKU:      "x@v1", Result: "bill", Meter: "m",
			Units: 1, Ts: time.Now().UTC(),
		})
	}
	cap, _ := NewCostCap(svc, &fakeCaps{v: 100_000}, &fakePricer{per: 200}, 50_000, nil)
	err := cap.Enforce(ctx, "ten_a", "x@v1", nil)
	if err == nil {
		t.Fatal("expected CapExceededError")
	}
	var capErr *CapExceededError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *CapExceededError; got %T %v", err, err)
	}
	if capErr.CapCents != 100_000 {
		t.Fatalf("cap: got %d want 100000", capErr.CapCents)
	}
	if capErr.ProjectedCents <= capErr.CapCents {
		t.Fatalf("projected %d should exceed cap %d", capErr.ProjectedCents, capErr.CapCents)
	}
	// RetryAfter should be in the future (start of next month).
	if !capErr.RetryAfter.After(time.Now().UTC()) {
		t.Fatalf("RetryAfter %s not in the future", capErr.RetryAfter)
	}
}

func TestCostCap_DefaultAppliesWhenStoreReturnsZero(t *testing.T) {
	svc, _ := newTestService(t)
	cap, _ := NewCostCap(svc, &fakeCaps{v: 0}, &fakePricer{per: 60_000}, 50_000, nil)
	err := cap.Enforce(context.Background(), "ten_a", "x@v1", nil)
	var capErr *CapExceededError
	if !errors.As(err, &capErr) {
		t.Fatalf("expected *CapExceededError; got %v", err)
	}
	if capErr.CapCents != 50_000 {
		t.Fatalf("expected default cap 50000; got %d", capErr.CapCents)
	}
}

func TestCostCap_NegativeMeansNoCap(t *testing.T) {
	svc, _ := newTestService(t)
	cap, _ := NewCostCap(svc, &fakeCaps{v: -1}, &fakePricer{per: 999_999_999}, 50_000, nil)
	if err := cap.Enforce(context.Background(), "ten_a", "x@v1", nil); err != nil {
		t.Fatalf("Enforce should pass with no cap; got %v", err)
	}
}

func TestCostCap_PeriodResetAtMonthRollover(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	// Insert 600 units in the *previous* month — outside the cap window.
	prev := time.Now().UTC().AddDate(0, -1, 0)
	for i := 0; i < 600; i++ {
		_ = svc.Record(ctx, LedgerEvent{
			EntryID:  fmt.Sprintf("task_PM:%d", i),
			TaskID:   "task_PM",
			TenantID: "ten_a",
			SKU:      "x@v1", Result: "bill", Meter: "m",
			Units: 1, Ts: prev,
		})
	}
	cap, _ := NewCostCap(svc, &fakeCaps{v: 100_000}, &fakePricer{per: 200}, 50_000, nil)
	if err := cap.Enforce(ctx, "ten_a", "x@v1", nil); err != nil {
		t.Fatalf("prev-month spend should not affect current cap; got %v", err)
	}
}

func TestLookupRef(t *testing.T) {
	r, ok := LookupRef("triage-watch:base")
	if !ok || r.ProductID == "" {
		t.Fatalf("expected lookup hit; got %+v ok=%v", r, ok)
	}
	if _, ok := LookupRef("does-not-exist"); ok {
		t.Fatal("expected miss")
	}
}
