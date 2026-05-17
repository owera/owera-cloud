// Synthetic load test for the durable queue (T14.3 acceptance):
// 1,000 jobs are enqueued in parallel, then drained by a pool of
// dispatcher workers. The test asserts:
//   - no enqueue returns an error,
//   - every job ID submitted is retrievable,
//   - queue depth converges to 0 (no drops),
//   - every job reaches a terminal state (succeeded under the synthetic
//     ledger poller).
//
// Set OWERA_LOAD_N to override the job count locally; the default is
// 1000 to match the plan's acceptance bar. Use `go test -run
// TestSynth1000Jobs ./tests/load/...` to execute.
package load

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/owera/owera-cloud/api/internal/catalog" // register SKUs
	"github.com/owera/owera-cloud/api/internal/dispatcher"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
	_ "modernc.org/sqlite"
)

func TestSynth1000Jobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in -short mode")
	}
	n := 1000
	if v := os.Getenv("OWERA_LOAD_N"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			t.Fatalf("OWERA_LOAD_N must be a positive int, got %q", v)
		}
		n = parsed
	}

	// A file-backed DB with per-connection PRAGMAs in the DSN. modernc's
	// sqlite gives each pool connection its own ":memory:" database, and
	// db.Exec("PRAGMA …") only sets PRAGMAs on one pool connection. The
	// production path uses a file with litestream replication, so this
	// matches reality. _pragma values are applied on every new connection.
	dbPath := filepath.Join(t.TempDir(), "load.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	js, err := jobs.New(db)
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	q, err := queue.NewSQLite(db)
	if err != nil {
		t.Fatalf("queue.NewSQLite: %v", err)
	}

	ctx := context.Background()
	tenantID := "fixture-load-tenant"

	// Phase 1: parallel submit.
	const submitWorkers = 16
	submitCh := make(chan int, n)
	for i := 0; i < n; i++ {
		submitCh <- i
	}
	close(submitCh)

	ids := make([]string, n)
	var enqueueFailures atomic.Int32
	var submitWG sync.WaitGroup
	t0 := time.Now()
	for w := 0; w < submitWorkers; w++ {
		submitWG.Add(1)
		go func() {
			defer submitWG.Done()
			for i := range submitCh {
				inputs := map[string]any{
					"queue_url":          fmt.Sprintf("https://fixture.example/q/%d", i),
					"priority_threshold": (i % 10) + 1,
				}
				j, _, err := js.Submit(ctx, tenantID, "triage-watch@v1", inputs, fmt.Sprintf("load-key-%d", i))
				if err != nil {
					enqueueFailures.Add(1)
					t.Errorf("Submit %d: %v", i, err)
					continue
				}
				if _, err := js.Transition(ctx, tenantID, j.ID, jobs.StatusQueued); err != nil {
					enqueueFailures.Add(1)
					t.Errorf("Transition queued %d: %v", i, err)
					continue
				}
				if _, _, err := q.Enqueue(ctx, tenantID, j.ID, map[string]any{"sku": j.SKU}, j.ID); err != nil {
					enqueueFailures.Add(1)
					t.Errorf("Enqueue %d: %v", i, err)
					continue
				}
				ids[i] = j.ID
			}
		}()
	}
	submitWG.Wait()
	if enqueueFailures.Load() != 0 {
		t.Fatalf("enqueue failures: %d", enqueueFailures.Load())
	}
	submitDur := time.Since(t0)

	depth, err := q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth after submit: %v", err)
	}
	if depth != n {
		t.Fatalf("queue depth after submit: got %d want %d", depth, n)
	}

	// Phase 2: drain with a pool of dispatcher workers.
	const dispatchWorkers = 8
	tr := dispatcher.NewInMemoryTransport()
	disp := dispatcher.New(tr)
	ledger := dispatcher.NewSyntheticLedgerPoller()

	drainCtx, drainCancel := context.WithTimeout(ctx, 60*time.Second)
	defer drainCancel()

	t1 := time.Now()
	var drainWG sync.WaitGroup
	for w := 0; w < dispatchWorkers; w++ {
		cfg := dispatcher.WorkerConfig{
			ClaimToken:    fmt.Sprintf("load-worker-%d", w),
			PollInterval:  5 * time.Millisecond,
			ClaimLease:    30 * time.Second,
			LedgerBackoff: time.Millisecond,
			MaxLedgerWait: 30 * time.Second,
		}
		worker := dispatcher.NewWorker(q, disp, js, ledger, cfg)
		drainWG.Add(1)
		go func() {
			defer drainWG.Done()
			for {
				if drainCtx.Err() != nil {
					return
				}
				d, _ := q.Depth(drainCtx)
				if d == 0 {
					return
				}
				if err := worker.RunOnce(drainCtx); err != nil {
					// queue.ErrEmpty is benign — another worker won the race.
					time.Sleep(2 * time.Millisecond)
				}
			}
		}()
	}
	drainWG.Wait()
	drainDur := time.Since(t1)

	// Phase 3: assertions.
	depth, err = q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth after drain: %v", err)
	}
	if depth != 0 {
		t.Fatalf("queue depth after drain: got %d want 0", depth)
	}

	terminal := 0
	missingTask := 0
	notTerminal := []string{}
	for _, id := range ids {
		j, err := js.Get(ctx, tenantID, id)
		if err != nil {
			t.Fatalf("Get %s: %v", id, err)
		}
		if !j.Status.IsTerminal() {
			notTerminal = append(notTerminal, fmt.Sprintf("%s=%s", id, j.Status))
			continue
		}
		terminal++
		if j.OperatorTaskID == "" {
			missingTask++
		}
	}
	if terminal != n {
		head := notTerminal
		if len(head) > 5 {
			head = head[:5]
		}
		t.Fatalf("terminal: got %d want %d (sample non-terminal: %v)", terminal, n, head)
	}
	if missingTask > 0 {
		t.Fatalf("%d/%d jobs missing operator_task_id", missingTask, n)
	}

	// Phase 4: idempotency replay — re-enqueue with the same keys; depth
	// must stay zero because all jobs are already terminal and the
	// idempotency-keyed queue row no longer exists. Replay the Submit
	// against the jobs.Store instead to confirm replays return existing.
	for i := 0; i < min(50, n); i++ {
		got, created, err := js.Submit(ctx, tenantID, "triage-watch@v1",
			map[string]any{"queue_url": "https://fixture.example/q/0"},
			fmt.Sprintf("load-key-%d", i))
		if err != nil {
			t.Fatalf("replay Submit %d: %v", i, err)
		}
		if created {
			t.Fatalf("replay Submit %d created a new job", i)
		}
		if got.ID != ids[i] {
			t.Fatalf("replay %d: got %q want %q", i, got.ID, ids[i])
		}
	}

	t.Logf("synthetic load summary: n=%d submit=%s drain=%s submitRate=%.0f/s drainRate=%.0f/s",
		n, submitDur, drainDur,
		float64(n)/submitDur.Seconds(),
		float64(n)/drainDur.Seconds())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
