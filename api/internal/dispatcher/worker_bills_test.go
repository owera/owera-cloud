package dispatcher

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/owera/owera-cloud/api/internal/billing"
	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
)

// stubBillRecorder is a recording BillRecorder for unit tests. It
// captures every LedgerEvent passed to Record and exposes them via
// Records() for assertions.
type stubBillRecorder struct {
	mu      sync.Mutex
	records []billing.LedgerEvent
}

func (s *stubBillRecorder) Record(_ context.Context, ev billing.LedgerEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, ev)
	return nil
}

func (s *stubBillRecorder) Records() []billing.LedgerEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]billing.LedgerEvent, len(s.records))
	copy(out, s.records)
	return out
}

// fixedLedgerPoller returns a fixed PollResult on the first call (with
// any bill entries the test wants the worker to see) and a terminal
// success on subsequent calls. Mimics the operator-plane LedgerTail
// emitting bill entries during the run, terminal at the end.
type fixedLedgerPoller struct {
	mu       sync.Mutex
	calls    int
	firstRes PollResult
}

func (f *fixedLedgerPoller) Poll(_ context.Context, taskID string) (PollResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 1 {
		return f.firstRes, nil
	}
	return PollResult{
		Terminal: true,
		Status:   jobs.StatusSucceeded,
		Outputs:  map[string]any{"task_id": taskID},
	}, nil
}

// TestWorker_RecordsBillEntries_ToBillRecorder is the WS-A.1 lock-in
// test. The synthetic ledger emits one `phase: "bill"` entry; the
// worker must hand it to the BillRecorder verbatim (TaskID, TenantID,
// SKU, Meter, Units, Result="bill", non-empty EntryID derived from
// task_id + entry Ts).
func TestWorker_RecordsBillEntries_ToBillRecorder(t *testing.T) {
	q, js, ten := newWorkerFixture(t)
	ctx := context.Background()

	job, _, err := js.Submit(ctx, ten, "triage-watch@v1",
		map[string]any{"queue_url": "https://acme.example/q"}, "")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := js.Transition(ctx, ten, job.ID, jobs.StatusQueued); err != nil {
		t.Fatalf("Transition queued: %v", err)
	}
	if _, _, err := q.Enqueue(ctx, ten, job.ID, map[string]any{}, job.ID); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	tr := NewInMemoryTransport()
	d := New(tr)
	billTs := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	poller := &fixedLedgerPoller{
		firstRes: PollResult{
			Bills: []BillEntry{
				{
					TaskID:     "task_synthetic",
					TenantID:   ten,
					Ts:         billTs,
					SKU:        "campaign-swarm@v1",
					Meter:      "campaigns_launched",
					Units:      1,
					OccurredAt: billTs,
				},
			},
		},
	}
	rec := &stubBillRecorder{}
	cfg := DefaultWorkerConfig()
	cfg.ClaimToken = "worker-bills-test"
	cfg.LedgerBackoff = time.Millisecond
	w := NewWorker(q, d, js, poller, rec, cfg)

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	got := rec.Records()
	if len(got) != 1 {
		t.Fatalf("BillRecorder.Record calls: got %d, want 1", len(got))
	}
	ev := got[0]
	if ev.Result != "bill" {
		t.Errorf("LedgerEvent.Result = %q, want %q", ev.Result, "bill")
	}
	if ev.TaskID != "task_synthetic" {
		t.Errorf("LedgerEvent.TaskID = %q, want %q", ev.TaskID, "task_synthetic")
	}
	if ev.TenantID != ten {
		t.Errorf("LedgerEvent.TenantID = %q, want %q", ev.TenantID, ten)
	}
	if ev.SKU != "campaign-swarm@v1" {
		t.Errorf("LedgerEvent.SKU = %q, want %q", ev.SKU, "campaign-swarm@v1")
	}
	if ev.Meter != "campaigns_launched" {
		t.Errorf("LedgerEvent.Meter = %q, want %q", ev.Meter, "campaigns_launched")
	}
	if ev.Units != 1 {
		t.Errorf("LedgerEvent.Units = %d, want 1", ev.Units)
	}
	if ev.EntryID == "" {
		t.Error("LedgerEvent.EntryID is empty; want stable derived id")
	}
	if !ev.Ts.Equal(billTs) {
		t.Errorf("LedgerEvent.Ts = %v, want %v", ev.Ts, billTs)
	}

	// And the job should still have advanced to succeeded — bill
	// entries are non-terminal, terminal entry arrived on second poll.
	gotJob, err := js.Get(ctx, ten, job.ID)
	if err != nil {
		t.Fatalf("jobs.Get: %v", err)
	}
	if gotJob.Status != jobs.StatusSucceeded {
		t.Errorf("job status = %q, want succeeded", gotJob.Status)
	}
}

// TestWorker_NilBillRecorder_DoesNotPanic locks in the nil-safety
// contract: a worker built without a BillRecorder still processes
// bill entries (they're just dropped). Existing tests rely on this
// to keep their NewWorker(... nil ...) call sites simple.
func TestWorker_NilBillRecorder_DoesNotPanic(t *testing.T) {
	q, js, ten := newWorkerFixture(t)
	ctx := context.Background()

	job, _, _ := js.Submit(ctx, ten, "triage-watch@v1",
		map[string]any{"queue_url": "https://acme.example/q"}, "")
	_, _ = js.Transition(ctx, ten, job.ID, jobs.StatusQueued)
	_, _, _ = q.Enqueue(ctx, ten, job.ID, map[string]any{}, job.ID)

	tr := NewInMemoryTransport()
	d := New(tr)
	poller := &fixedLedgerPoller{
		firstRes: PollResult{
			Bills: []BillEntry{{
				TaskID:   "task_synthetic",
				TenantID: ten,
				Ts:       time.Now().UTC(),
				SKU:      "campaign-swarm@v1",
				Meter:    "campaigns_launched",
				Units:    1,
			}},
		},
	}
	cfg := DefaultWorkerConfig()
	cfg.ClaimToken = "worker-bills-nil-test"
	cfg.LedgerBackoff = time.Millisecond
	w := NewWorker(q, d, js, poller, nil, cfg) // <-- nil BillRecorder
	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}

// TestWorker_BillRecorder_EndToEnd_OutboxInsert wires a real
// billing.Service against an in-memory SQLite identity DB and asserts
// that the bill entry surfaced by the synthetic ledger lands as a
// row in billing_outbox. This is the integration-shaped contract
// closing the WS-A.1 gap end-to-end inside dispatcher tests.
func TestWorker_BillRecorder_EndToEnd_OutboxInsert(t *testing.T) {
	id, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = id.Close() })

	q, err := queue.NewSQLite(id.DB())
	if err != nil {
		t.Fatalf("queue.NewSQLite: %v", err)
	}
	js, err := jobs.New(id.DB())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	bs, err := billing.New(id.DB(), &billing.FakeBackend{})
	if err != nil {
		t.Fatalf("billing.New: %v", err)
	}
	tenant, err := id.CreateTenant(context.Background(), "fixture-tenant")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	ten := tenant.ID
	ctx := context.Background()

	job, _, err := js.Submit(ctx, ten, "triage-watch@v1",
		map[string]any{"queue_url": "https://acme.example/q"}, "")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := js.Transition(ctx, ten, job.ID, jobs.StatusQueued); err != nil {
		t.Fatalf("Transition queued: %v", err)
	}
	if _, _, err := q.Enqueue(ctx, ten, job.ID, map[string]any{}, job.ID); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	tr := NewInMemoryTransport()
	d := New(tr)
	poller := &fixedLedgerPoller{
		firstRes: PollResult{
			Bills: []BillEntry{{
				TaskID:     "task_synthetic",
				TenantID:   ten,
				Ts:         time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
				SKU:        "campaign-swarm@v1",
				Meter:      "campaigns_launched",
				Units:      1,
				OccurredAt: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
			}},
		},
	}
	cfg := DefaultWorkerConfig()
	cfg.ClaimToken = "worker-bills-e2e-test"
	cfg.LedgerBackoff = time.Millisecond
	w := NewWorker(q, d, js, poller, bs, cfg)

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	usage, err := bs.UsageByTenant(ctx, ten)
	if err != nil {
		t.Fatalf("UsageByTenant: %v", err)
	}
	if got, want := usage["campaign-swarm@v1"], int64(1); got != want {
		t.Errorf("outbox usage for campaign-swarm@v1: got %d, want %d (full row: %+v)", got, want, usage)
	}
}

// TestLedgerTailClient_ParsesBillEntries asserts the ledger-tail
// client surfaces bill entries via PollResult.Bills with the typed
// BillEvent payload decoded from the entry's Data blob.
func TestLedgerTailClient_ParsesBillEntries(t *testing.T) {
	// Build a synthetic server that returns one bill entry + one
	// terminal complete in a single poll. The bill entry's Data is
	// the operator-plane BillEvent JSON shape from skus.BillEvent.
	ts := time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC)
	srv := newMockRPCServer(t, func(call int, _ string) ledgerTailResult {
		billData := []byte(`{"sku":"triage-watch@v1","meter":"tickets_handled","units":3,"occurred_at":"2026-05-17T14:00:00Z"}`)
		return ledgerTailResult{
			Entries: []ledgerEntry{
				{
					Ts:       ts,
					TaskID:   "tid",
					TenantID: "ten",
					Phase:    "bill",
					Action:   "triage-watch@v1",
					Result:   "emitted",
					Data:     billData,
				},
				{Ts: ts.Add(time.Second), TaskID: "tid", Phase: "complete", Action: "done", Result: "ok"},
			},
			Cursor: ts.Add(time.Second).Format(time.RFC3339Nano),
		}
	})
	t.Cleanup(srv.Close)

	c := NewLedgerTailClient(srv.URL, nil)
	res, err := c.Poll(context.Background(), "tid")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if !res.Terminal {
		t.Error("expected terminal=true (complete entry follows the bill)")
	}
	if got, want := len(res.Bills), 1; got != want {
		t.Fatalf("Bills len = %d, want %d", got, want)
	}
	b := res.Bills[0]
	if b.TaskID != "tid" {
		t.Errorf("bill.TaskID = %q, want %q", b.TaskID, "tid")
	}
	if b.TenantID != "ten" {
		t.Errorf("bill.TenantID = %q, want %q", b.TenantID, "ten")
	}
	if b.SKU != "triage-watch@v1" {
		t.Errorf("bill.SKU = %q, want %q", b.SKU, "triage-watch@v1")
	}
	if b.Meter != "tickets_handled" {
		t.Errorf("bill.Meter = %q, want %q", b.Meter, "tickets_handled")
	}
	if b.Units != 3 {
		t.Errorf("bill.Units = %d, want 3", b.Units)
	}
	if !b.OccurredAt.Equal(ts) {
		t.Errorf("bill.OccurredAt = %v, want %v", b.OccurredAt, ts)
	}
}
