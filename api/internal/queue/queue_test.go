package queue

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestQueue(t *testing.T) *SQLiteQueue {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	q, err := NewSQLite(db)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	return q
}

func TestEnqueueDequeueAck(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	_, _, err := q.Enqueue(ctx, "ten_a", "job_1", map[string]any{"x": 1}, "")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	it, err := q.Dequeue(ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if it.JobID != "job_1" {
		t.Fatalf("job_id: got %q want job_1", it.JobID)
	}
	if err := q.Ack(ctx, it.ID, "worker-1"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	depth, _ := q.Depth(ctx)
	if depth != 0 {
		t.Fatalf("depth: got %d want 0", depth)
	}
}

func TestDequeueEmpty(t *testing.T) {
	q := newTestQueue(t)
	_, err := q.Dequeue(context.Background(), "worker-1", time.Minute)
	if !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestEnqueueIdempotency(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	a, ca, _ := q.Enqueue(ctx, "ten_a", "job_1", map[string]any{"x": 1}, "key-1")
	b, cb, err := q.Enqueue(ctx, "ten_a", "job_1", map[string]any{"x": 1}, "key-1")
	if err != nil {
		t.Fatalf("second Enqueue: %v", err)
	}
	if !ca || cb {
		t.Fatalf("created flags: %v %v", ca, cb)
	}
	if a.ID != b.ID {
		t.Fatalf("idempotency: ids differ %q vs %q", a.ID, b.ID)
	}
}

func TestNackReturnsToQueue(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	_, _, _ = q.Enqueue(ctx, "ten_a", "job_1", map[string]any{}, "")
	it, _ := q.Dequeue(ctx, "worker-1", time.Minute)
	if err := q.Nack(ctx, it.ID, "worker-1"); err != nil {
		t.Fatalf("Nack: %v", err)
	}
	it2, err := q.Dequeue(ctx, "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("re-Dequeue: %v", err)
	}
	if it2.ID != it.ID {
		t.Fatalf("expected same item, got %q vs %q", it.ID, it2.ID)
	}
}

func TestAckWrongTokenFails(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	_, _, _ = q.Enqueue(ctx, "ten_a", "job_1", map[string]any{}, "")
	it, _ := q.Dequeue(ctx, "worker-1", time.Minute)
	if err := q.Ack(ctx, it.ID, "wrong-token"); !errors.Is(err, ErrClaimMismatch) {
		t.Fatalf("expected ErrClaimMismatch, got %v", err)
	}
}

func TestClaimExpiryAllowsResteal(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	_, _, _ = q.Enqueue(ctx, "ten_a", "job_1", map[string]any{}, "")
	it1, _ := q.Dequeue(ctx, "worker-1", time.Minute)
	// Force-expire the claim by going back in time on the row.
	_, err := q.db.Exec(`UPDATE queue SET claimed_at=? WHERE id=?`,
		time.Now().Add(-2*time.Hour), it1.ID)
	if err != nil {
		t.Fatalf("force expire: %v", err)
	}
	it2, err := q.Dequeue(ctx, "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("re-steal Dequeue: %v", err)
	}
	if it2.ID != it1.ID {
		t.Fatalf("expected re-steal of same item")
	}
}

func TestFIFOOrder(t *testing.T) {
	q := newTestQueue(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _, _ = q.Enqueue(ctx, "ten_a", "job", map[string]any{"i": i}, "")
		time.Sleep(2 * time.Millisecond)
	}
	first, _ := q.Dequeue(ctx, "w1", time.Minute)
	second, _ := q.Dequeue(ctx, "w1", time.Minute)
	if first.Payload["i"].(float64) > second.Payload["i"].(float64) {
		t.Fatalf("FIFO violated: %v then %v", first.Payload["i"], second.Payload["i"])
	}
}
