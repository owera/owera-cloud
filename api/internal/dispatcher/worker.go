package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
)

// JobStore is the subset of jobs.Store the worker needs. The narrow
// interface keeps worker tests free of a real SQLite migration.
type JobStore interface {
	Get(ctx context.Context, tenantID, id string) (*jobs.Job, error)
	Transition(ctx context.Context, tenantID, id string, to jobs.Status, opts ...jobs.TransitionOpt) (*jobs.Job, error)
}

// LedgerPoller asks the operator plane whether a previously-dispatched
// task has reached a terminal state. The production implementation calls
// fleet.LedgerTail over the Cloudflare tunnel; the in-memory transport's
// default responder simulates immediate success. See TL open question in
// the PR body about whether the operator plane should expose
// fleet.LedgerTail vs. fleet.LedgerFetch.
type LedgerPoller interface {
	Poll(ctx context.Context, operatorTaskID string) (terminal bool, status jobs.Status, outputs map[string]any, errMsg string, err error)
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
	cfg    WorkerConfig
}

// NewWorker constructs a worker. ledger may be nil for tests that only
// exercise the dispatch path; in that case the worker advances the job to
// "running" and stops there.
func NewWorker(q queue.Queue, d *Dispatcher, js JobStore, ledger LedgerPoller, cfg WorkerConfig) *Worker {
	if cfg.ClaimToken == "" {
		cfg = DefaultWorkerConfig()
	}
	return &Worker{q: q, d: d, js: js, ledger: ledger, cfg: cfg}
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
		terminal, status, outputs, errMsg, err := w.ledger.Poll(ctx, taskID)
		if err != nil {
			return fmt.Errorf("worker: ledger poll: %w", err)
		}
		if terminal {
			opts := []jobs.TransitionOpt{}
			if outputs != nil {
				opts = append(opts, jobs.WithOutputs(outputs))
			}
			if errMsg != "" {
				opts = append(opts, jobs.WithError(errMsg))
			}
			_, err := w.js.Transition(ctx, tenantID, jobID, status, opts...)
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.cfg.LedgerBackoff):
		}
	}
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
func (s *SyntheticLedgerPoller) Poll(_ context.Context, taskID string) (bool, jobs.Status, map[string]any, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[taskID]++
	return true, s.Status, map[string]any{"task_id": taskID, "synthetic": true}, "", nil
}

// Calls returns how many times Poll has been called for taskID.
func (s *SyntheticLedgerPoller) Calls(taskID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[taskID]
}
