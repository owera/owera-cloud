package dispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
)

func newWorkerFixture(t *testing.T) (*queue.SQLiteQueue, *jobs.Store, string) {
	t.Helper()
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
	tenant, err := id.CreateTenant(context.Background(), "fixture-tenant")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	return q, js, tenant.ID
}

func TestWorker_HappyPath_EndToEnd(t *testing.T) {
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
	cfg := DefaultWorkerConfig()
	cfg.ClaimToken = "worker-test"
	cfg.LedgerBackoff = time.Millisecond
	w := NewWorker(q, d, js, NewSyntheticLedgerPoller(), cfg)

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	got, err := js.Get(ctx, ten, job.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != jobs.StatusSucceeded {
		t.Fatalf("status: got %q want succeeded", got.Status)
	}
	if got.OperatorTaskID != "task_synthetic" {
		t.Fatalf("operator task: got %q want task_synthetic", got.OperatorTaskID)
	}
	if depth, _ := q.Depth(ctx); depth != 0 {
		t.Fatalf("queue depth: got %d want 0 (item should have been Ack'd)", depth)
	}
}

func TestWorker_DispatchError_MarksJobFailed(t *testing.T) {
	q, js, ten := newWorkerFixture(t)
	ctx := context.Background()
	job, _, _ := js.Submit(ctx, ten, "triage-watch@v1",
		map[string]any{"queue_url": "https://x.example/q"}, "")
	_, _ = js.Transition(ctx, ten, job.ID, jobs.StatusQueued)
	_, _, _ = q.Enqueue(ctx, ten, job.ID, map[string]any{}, job.ID)

	tr := NewInMemoryTransport()
	tr.Responder = func(string, any) (any, error) { return nil, errors.New("operator down") }
	d := New(tr)
	cfg := DefaultWorkerConfig()
	cfg.ClaimToken = "worker-test"
	w := NewWorker(q, d, js, NewSyntheticLedgerPoller(), cfg)

	_ = w.RunOnce(ctx)

	got, _ := js.Get(ctx, ten, job.ID)
	if got.Status != jobs.StatusFailed {
		t.Fatalf("status: got %q want failed", got.Status)
	}
	if got.Error == "" {
		t.Fatal("expected error message recorded")
	}
}

func TestWorker_AlreadyTerminalJob_AcksWithoutDispatch(t *testing.T) {
	q, js, ten := newWorkerFixture(t)
	ctx := context.Background()
	job, _, _ := js.Submit(ctx, ten, "triage-watch@v1",
		map[string]any{"queue_url": "https://x.example/q"}, "")
	_, _ = js.Transition(ctx, ten, job.ID, jobs.StatusCancelled)
	_, _, _ = q.Enqueue(ctx, ten, job.ID, map[string]any{}, job.ID)

	tr := NewInMemoryTransport()
	d := New(tr)
	cfg := DefaultWorkerConfig()
	cfg.ClaimToken = "worker-test"
	w := NewWorker(q, d, js, NewSyntheticLedgerPoller(), cfg)

	if err := w.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if tr.CallCount() != 0 {
		t.Fatalf("call count: got %d want 0 (cancelled job should not dispatch)", tr.CallCount())
	}
	if depth, _ := q.Depth(ctx); depth != 0 {
		t.Fatalf("queue depth: got %d want 0", depth)
	}
}

func TestWorker_EmptyQueue_ReturnsErrEmpty(t *testing.T) {
	q, js, _ := newWorkerFixture(t)
	d := New(NewInMemoryTransport())
	w := NewWorker(q, d, js, NewSyntheticLedgerPoller(), WorkerConfig{ClaimToken: "w"})
	err := w.RunOnce(context.Background())
	if !errors.Is(err, queue.ErrEmpty) {
		t.Fatalf("expected queue.ErrEmpty, got %v", err)
	}
}
