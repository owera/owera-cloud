package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/owera/owera-cloud/api/internal/billing"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
)

// JobStore is the subset of jobs.Store the worker needs. The narrow
// interface keeps worker tests free of a real SQLite migration.
type JobStore interface {
	Get(ctx context.Context, tenantID, id string) (*jobs.Job, error)
	Transition(ctx context.Context, tenantID, id string, to jobs.Status, opts ...jobs.TransitionOpt) (*jobs.Job, error)
}

// PollResult is the structured return shape of LedgerPoller.Poll. It
// extends the pre-WS-A.1 (terminal, status, outputs, errMsg) tuple with
// a Bills slice carrying any `phase: "bill"` entries seen in the same
// poll window. The worker hands those off to BillRecorder so the
// operator-plane bill markers end up in the cloud's billing outbox
// (WS-A.1: closes the gap that left billing_outbox empty even though
// the operator-plane ledger had a bill entry).
//
// Walking entries in the same Poll call (rather than a separate
// PollBills method) is load-bearing: each successful Poll advances the
// per-task cursor in the production LedgerTailClient, so a second
// "bills-only" RPC would return an empty entry set. WS-B's
// long-running SKUs (e.g. monthly-subscription triage-watch emitting
// per-tick bill events outside any single worker poll cycle) will need
// a separate persistent subscriber; tracked in ROADMAP.md.
type PollResult struct {
	Terminal bool
	Status   jobs.Status
	Outputs  map[string]any
	ErrMsg   string
	// Bills is the per-poll batch of `phase: "bill"` entries. The
	// worker translates each into a billing.LedgerEvent and hands it to
	// BillRecorder.Record.
	Bills []BillEntry
}

// BillEntry is the cloud-side projection of an operator-plane
// `phase: "bill"` ledger entry, parsed out of its Data payload. The
// shape mirrors operator-plane `skus.BillEvent` (docs/sku-execution-spec.md
// "Billing-emission contract") plus the (TaskID, TenantID, Ts) the
// ledger entry stamps directly.
type BillEntry struct {
	TaskID       string
	TenantID     string
	Ts           time.Time // entry-level timestamp; used to derive a stable EntryID
	SKU          string    `json:"sku"`
	Meter        string    `json:"meter"`
	Units        int64     `json:"units"`
	OneShotCents int64     `json:"one_shot_cents,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
}

// LedgerPoller asks the operator plane whether a previously-dispatched
// task has reached a terminal state. The production implementation calls
// fleet.LedgerTail over the Cloudflare tunnel; the in-memory transport's
// default responder simulates immediate success.
//
// Poll also surfaces `phase: "bill"` entries via PollResult.Bills so
// the worker can fan them into the cloud's billing outbox in the same
// loop that follows terminal state.
type LedgerPoller interface {
	Poll(ctx context.Context, operatorTaskID string) (PollResult, error)
}

// BillRecorder is the subset of billing.Service the worker calls to
// stash a bill marker in the outbox. Narrow interface so tests can pass
// a recording double without spinning up a SQLite DB. nil is the
// "no billing wired" sentinel — callers must guard.
type BillRecorder interface {
	Record(ctx context.Context, ev billing.LedgerEvent) error
}

// WorkerConfig tunes the dispatcher worker loop.
type WorkerConfig struct {
	ClaimToken    string        // identifies this worker for queue claim leases
	PollInterval  time.Duration // delay between queue polls when empty
	ClaimLease    time.Duration // how long a claim is held before becoming re-stealable
	LedgerBackoff time.Duration // delay between ledger polls per dispatched task
	MaxLedgerWait time.Duration // give up after this long and mark the job failed
}

// DefaultWorkerConfig returns sane defaults for production.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		ClaimToken:    "worker-default",
		PollInterval:  500 * time.Millisecond,
		ClaimLease:    5 * time.Minute,
		LedgerBackoff: 250 * time.Millisecond,
		MaxLedgerWait: 30 * time.Minute,
	}
}

// Worker pulls items off the queue and dispatches them through to the
// operator plane, then polls the ledger for completion and advances the
// job's lifecycle state. Acks on terminal status; never drops a row.
type Worker struct {
	q      queue.Queue
	d      *Dispatcher
	js     JobStore
	ledger LedgerPoller
	bills  BillRecorder // nil-safe; when nil, bill entries are still parsed but not recorded
	cfg    WorkerConfig
}

// NewWorker constructs a worker. ledger may be nil for tests that only
// exercise the dispatch path; in that case the worker advances the job to
// "running" and stops there. bills may also be nil — when set, the worker
// fans `phase: "bill"` ledger entries from each poll into the cloud's
// billing outbox via BillRecorder.Record.
func NewWorker(q queue.Queue, d *Dispatcher, js JobStore, ledger LedgerPoller, bills BillRecorder, cfg WorkerConfig) *Worker {
	if cfg.ClaimToken == "" {
		cfg = DefaultWorkerConfig()
	}
	return &Worker{q: q, d: d, js: js, ledger: ledger, bills: bills, cfg: cfg}
}

// Run loops until ctx is cancelled. Errors from individual jobs are
// logged but never fatal — at-least-once is preserved by Nack-on-error.
func (w *Worker) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.tick(ctx); err != nil {
			if !errors.Is(err, queue.ErrEmpty) {
				log.Printf("dispatcher.Worker: tick: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(w.cfg.PollInterval):
			}
		}
	}
}

// RunOnce processes a single queue item synchronously. Returns
// queue.ErrEmpty when nothing's claimable. Tests call this to drive
// progress without spawning a goroutine.
func (w *Worker) RunOnce(ctx context.Context) error {
	return w.tick(ctx)
}

func (w *Worker) tick(ctx context.Context) error {
	item, err := w.q.Dequeue(ctx, w.cfg.ClaimToken, w.cfg.ClaimLease)
	if err != nil {
		return err
	}
	if err := w.handle(ctx, item); err != nil {
		// Best-effort Nack so the lease releases now rather than at expiry.
		_ = w.q.Nack(ctx, item.ID, w.cfg.ClaimToken)
		return err
	}
	return w.q.Ack(ctx, item.ID, w.cfg.ClaimToken)
}

func (w *Worker) handle(ctx context.Context, item *queue.Item) error {
	j, err := w.js.Get(ctx, item.TenantID, item.JobID)
	if err != nil {
		return fmt.Errorf("worker: load job %s: %w", item.JobID, err)
	}
	// Skip already-terminal jobs (cancel raced ahead of the worker pickup).
	if j.Status.IsTerminal() {
		return nil
	}
	taskID, err := w.d.Dispatch(ctx, item.TenantID, item.JobID, j.SKU, j.Inputs)
	if err != nil {
		_, _ = w.js.Transition(ctx, item.TenantID, item.JobID, jobs.StatusFailed, jobs.WithError(err.Error()))
		return fmt.Errorf("worker: dispatch %s: %w", item.JobID, err)
	}
	if _, err := w.js.Transition(ctx, item.TenantID, item.JobID, jobs.StatusRunning, jobs.WithOperatorTaskID(taskID)); err != nil {
		return fmt.Errorf("worker: transition running: %w", err)
	}
	if w.ledger == nil {
		return nil
	}
	return w.followTask(ctx, item.TenantID, item.JobID, taskID)
}

func (w *Worker) followTask(ctx context.Context, tenantID, jobID, taskID string) error {
	deadline := time.Now().Add(w.cfg.MaxLedgerWait)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			_, _ = w.js.Transition(ctx, tenantID, jobID, jobs.StatusFailed,
				jobs.WithError("ledger wait exceeded"))
			return fmt.Errorf("worker: ledger wait exceeded for %s", taskID)
		}
		res, err := w.ledger.Poll(ctx, taskID)
		if err != nil {
			return fmt.Errorf("worker: ledger poll: %w", err)
		}
		// Drain any `phase: "bill"` entries seen in this poll window into
		// the billing outbox. Outbox insert dedupes on EntryID so retries
		// (cursor regression on restart, redelivery, …) are safe.
		w.recordBills(ctx, tenantID, res.Bills)
		if res.Terminal {
			opts := []jobs.TransitionOpt{}
			if res.Outputs != nil {
				opts = append(opts, jobs.WithOutputs(res.Outputs))
			}
			if res.ErrMsg != "" {
				opts = append(opts, jobs.WithError(res.ErrMsg))
			}
			_, err := w.js.Transition(ctx, tenantID, jobID, res.Status, opts...)
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.cfg.LedgerBackoff):
		}
	}
}

// recordBills fans the per-poll bill batch into the billing outbox.
// Errors are logged but never propagate — losing the worker because
// SQLite is momentarily busy would also fail the ledger transition and
// (worse) leak the queue lease. The outbox UNIQUE(entry_id) and the
// operator plane's at-least-once redelivery on reconnect make the
// next poll-cycle a self-healing retry path.
func (w *Worker) recordBills(ctx context.Context, jobTenantID string, bills []BillEntry) {
	if w.bills == nil || len(bills) == 0 {
		return
	}
	for _, b := range bills {
		tenantID := b.TenantID
		if tenantID == "" {
			// Operator-plane stamps tenant_id on bill entries (per WS-A
			// contract), but older entries pre-dating that field may
			// arrive blank — fall back to the job's tenant so the
			// outbox row is still attributable.
			tenantID = jobTenantID
		}
		entryID := buildBillEntryID(b)
		ts := b.OccurredAt
		if ts.IsZero() {
			ts = b.Ts
		}
		ev := billing.LedgerEvent{
			EntryID:  entryID,
			TaskID:   b.TaskID,
			TenantID: tenantID,
			SKU:      b.SKU,
			Result:   "bill", // gates Service.Record's accept-list
			Units:    b.Units,
			Meter:    b.Meter,
			Ts:       ts,
		}
		if err := w.bills.Record(ctx, ev); err != nil {
			log.Printf("dispatcher: worker bill record err task=%s entry=%s: %v", b.TaskID, entryID, err)
			continue
		}
		log.Printf("dispatcher: worker recorded bill entry_id=%s task=%s tenant=%s sku=%s meter=%s units=%d",
			entryID, b.TaskID, tenantID, b.SKU, b.Meter, b.Units)
	}
}

// buildBillEntryID derives a stable, deterministic outbox key for a
// bill entry. The operator-plane ledger does not assign per-entry IDs
// over the wire (the on-disk `<task_id>.jsonl` line offset is the
// internal anchor and is NOT exposed via fleet.LedgerTail), so we hash
// (task_id, entry-Ts) into a token that survives cursor regressions
// and worker restarts. Ts is RFC3339Nano-stable from the operator
// signer; the outbox UNIQUE(entry_id) dedupes any same-event
// redelivery. SKU+Meter are included so distinct bill emits within
// the same ledger Ts (rare but legal) don't collide.
func buildBillEntryID(b BillEntry) string {
	ts := b.Ts
	if ts.IsZero() {
		ts = b.OccurredAt
	}
	return fmt.Sprintf("%s:%s:%s:%s", b.TaskID, ts.UTC().Format(time.RFC3339Nano), b.SKU, b.Meter)
}

// SyntheticLedgerPoller is a test LedgerPoller that returns
// `succeeded` on the first poll for every task. Production wires a real
// operator-plane RPC client here.
type SyntheticLedgerPoller struct {
	mu     sync.Mutex
	calls  map[string]int
	Status jobs.Status
}

// NewSyntheticLedgerPoller returns a poller that immediately reports
// success (or whatever status you configure via the returned struct).
func NewSyntheticLedgerPoller() *SyntheticLedgerPoller {
	return &SyntheticLedgerPoller{
		calls:  map[string]int{},
		Status: jobs.StatusSucceeded,
	}
}

// Poll implements LedgerPoller.
func (s *SyntheticLedgerPoller) Poll(_ context.Context, taskID string) (PollResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[taskID]++
	return PollResult{
		Terminal: true,
		Status:   s.Status,
		Outputs:  map[string]any{"task_id": taskID, "synthetic": true},
	}, nil
}

// Calls returns how many times Poll has been called for taskID.
func (s *SyntheticLedgerPoller) Calls(taskID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[taskID]
}
