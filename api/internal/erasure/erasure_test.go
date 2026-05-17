package erasure

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/queue"
)

func newTestSvc(t *testing.T) (*Service, *audit.Log, *queue.SQLiteQueue, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	a, err := audit.New(db)
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}
	q, err := queue.NewSQLite(db)
	if err != nil {
		t.Fatalf("queue.NewSQLite: %v", err)
	}
	s, err := New(db, AdaptQueue(q), a)
	if err != nil {
		t.Fatalf("erasure.New: %v", err)
	}
	return s, a, q, db
}

func TestSubmit_EnqueuesAndAudits(t *testing.T) {
	s, a, q, _ := newTestSvc(t)
	ctx := context.Background()

	req, err := s.Submit(ctx, "fixture-1", "fixture-usr-1", "203.0.113.5", "fixture-ua/1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if req.State != "queued" {
		t.Fatalf("state: got %s want queued", req.State)
	}
	if req.SLADueAt.Sub(req.RequestedAt) < 15*24*time.Hour-time.Minute {
		t.Fatalf("SLA window too short: %v", req.SLADueAt.Sub(req.RequestedAt))
	}

	rows, err := a.List(ctx, "fixture-1", 10)
	if err != nil {
		t.Fatalf("audit.List: %v", err)
	}
	found := false
	for _, e := range rows {
		if e.Action == ActionRequest && e.Target == req.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ActionRequest audit row for the submitted request")
	}

	depth, _ := q.Depth(ctx)
	if depth != 1 {
		t.Fatalf("queue depth: got %d want 1", depth)
	}
}

func TestSubmit_Idempotent_SameDay(t *testing.T) {
	s, _, q, _ := newTestSvc(t)
	ctx := context.Background()

	r1, err := s.Submit(ctx, "fixture-1", "fixture-usr-1", "1.1.1.1", "t")
	if err != nil {
		t.Fatalf("Submit 1: %v", err)
	}
	r2, err := s.Submit(ctx, "fixture-1", "fixture-usr-1", "1.1.1.1", "t")
	if err != nil {
		t.Fatalf("Submit 2: %v", err)
	}
	if r1.ID != r2.ID {
		t.Fatalf("expected dedupe by tenant+day, got %s vs %s", r1.ID, r2.ID)
	}
	depth, _ := q.Depth(ctx)
	if depth != 1 {
		t.Fatalf("queue depth: got %d want 1 (dedupe)", depth)
	}
}

func TestSubmit_RejectsEmptyTenant(t *testing.T) {
	s, _, _, _ := newTestSvc(t)
	if _, err := s.Submit(context.Background(), "", "u", "1.1.1.1", "t"); err == nil {
		t.Fatal("expected error for empty tenant_id")
	}
}

// Worker test: enqueue → ProcessOne → status complete + completion audit row.
func TestWorker_ProcessOne_CompletesAndAudits(t *testing.T) {
	s, a, q, db := newTestSvc(t)
	ctx := context.Background()

	req, err := s.Submit(ctx, "fixture-7", "fixture-usr-7", "1.1.1.1", "t")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	worker := &Worker{
		Service:      s,
		Queue:        AdaptQueue(q),
		Audit:        a,
		Purger:       &CompositePurger{DB: db},
		ClaimMaxAge:  time.Minute,
		PollInterval: time.Millisecond,
	}
	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !processed {
		t.Fatal("expected ProcessOne to handle the queued request")
	}

	got, err := s.Get(ctx, "fixture-7", req.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.State != "complete" {
		t.Fatalf("state after worker: got %v want complete", got)
	}
	if got.Report == nil {
		t.Fatal("expected non-nil report")
	}
	if got.Report.VerificationStatus != "pending" {
		t.Fatalf("verification status: got %s want pending", got.Report.VerificationStatus)
	}

	rows, _ := a.List(ctx, "fixture-7", 50)
	var hasStart, hasComplete bool
	for _, e := range rows {
		switch e.Action {
		case ActionStart:
			hasStart = true
		case ActionComplete:
			hasComplete = true
		}
	}
	if !hasStart || !hasComplete {
		t.Fatalf("expected start+complete audit rows, got start=%v complete=%v", hasStart, hasComplete)
	}

	// Chain integrity holds across all the rows the worker emitted.
	if bad, err := a.Verify(ctx); err != nil {
		t.Fatalf("audit.Verify: bad=%d err=%v", bad, err)
	}
}

func TestWorker_ProcessOne_PurgerFailureMarksFailed(t *testing.T) {
	s, a, q, _ := newTestSvc(t)
	ctx := context.Background()

	req, _ := s.Submit(ctx, "fixture-9", "fixture-usr-9", "1.1.1.1", "t")
	worker := &Worker{
		Service:      s,
		Queue:        AdaptQueue(q),
		Audit:        a,
		Purger:       failingPurger{},
		ClaimMaxAge:  time.Minute,
		PollInterval: time.Millisecond,
	}
	if _, err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	got, _ := s.Get(ctx, "fixture-9", req.ID)
	if got == nil || got.State != "failed" {
		t.Fatalf("state: got %v want failed", got)
	}
}

type failingPurger struct{}

func (failingPurger) Purge(_ context.Context, _, _ string) (PurgeReport, error) {
	return PurgeReport{}, errExample
}

var errExample = errStr("synthetic test failure")

type errStr string

func (e errStr) Error() string { return string(e) }
