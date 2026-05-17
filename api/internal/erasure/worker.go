package erasure

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
)

// Worker dequeues erasure requests and invokes the Purger. One worker
// instance is enough — erasure is a low-rate operation, and serializing
// avoids two purges interleaving on the same tenant.
type Worker struct {
	Service *Service
	Queue   Queue
	Purger  Purger
	Audit   *audit.Log
	// PollInterval is how often we poll the queue when it was empty
	// last time. Production: 15s.
	PollInterval time.Duration
	// ClaimMaxAge is the visibility timeout — if the worker dies mid-
	// purge, another worker may re-claim after this many minutes.
	ClaimMaxAge time.Duration
	// Logger is optional; nil falls back to the std logger.
	Logger *log.Logger
}

// Run loops until ctx is cancelled. One iteration: Dequeue → start →
// purge → audit → mark complete (or fail).
func (w *Worker) Run(ctx context.Context) error {
	if w.Service == nil || w.Queue == nil || w.Purger == nil || w.Audit == nil {
		return errors.New("erasure: Worker.{Service,Queue,Purger,Audit} all required")
	}
	if w.PollInterval == 0 {
		w.PollInterval = 15 * time.Second
	}
	if w.ClaimMaxAge == 0 {
		w.ClaimMaxAge = 15 * time.Minute
	}
	claimToken := newClaimToken()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := w.Queue.Dequeue(ctx, claimToken, w.ClaimMaxAge)
		if err != nil {
			w.logf("dequeue: %v", err)
			if !sleep(ctx, w.PollInterval) {
				return ctx.Err()
			}
			continue
		}
		if item == nil {
			if !sleep(ctx, w.PollInterval) {
				return ctx.Err()
			}
			continue
		}
		w.processOne(ctx, item, claimToken)
	}
}

// ProcessOne dequeues at most one item and runs it. Returned for use
// by tests so they don't need to manage the polling loop.
func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	claimToken := newClaimToken()
	item, err := w.Queue.Dequeue(ctx, claimToken, w.ClaimMaxAge)
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}
	w.processOne(ctx, item, claimToken)
	return true, nil
}

func (w *Worker) processOne(ctx context.Context, item queueItem, claimToken string) {
	tenantID := item.GetTenantID()
	payload := item.GetPayload()
	requestID, _ := payload["request_id"].(string)
	if requestID == "" {
		// Garbage payload — ack so we don't loop, but log loudly.
		w.logf("malformed erasure item id=%s tenant=%s payload=%v",
			item.GetID(), tenantID, payload)
		_ = w.Queue.Ack(ctx, item.GetID(), claimToken)
		return
	}

	if err := w.Service.markStarted(ctx, requestID); err != nil {
		w.logf("markStarted request=%s: %v", requestID, err)
		_ = w.Queue.Nack(ctx, item.GetID(), claimToken)
		return
	}

	_ = w.Audit.Append(ctx, audit.Entry{
		TenantID: tenantID, UserID: "system",
		Action: ActionStart, Target: requestID,
		IP: "", UserAgent: "erasure-worker",
	})

	started := time.Now().UTC()
	report, perr := w.Purger.Purge(ctx, tenantID, requestID)
	if perr != nil {
		_ = w.Audit.Append(ctx, audit.Entry{
			TenantID: tenantID, UserID: "system",
			Action: ActionFail, Target: requestID,
			IP: "", UserAgent: "erasure-worker",
		})
		_ = w.Service.markFailed(ctx, requestID, perr.Error())
		// Nack lets another worker (or a retry after backoff) pick up.
		// Future: add an attempt counter and DLQ after N failures.
		_ = w.Queue.Nack(ctx, item.GetID(), claimToken)
		w.logf("purge request=%s tenant=%s: %v", requestID, tenantID, perr)
		return
	}
	if report.StartedAt.IsZero() {
		report.StartedAt = started
	}
	if report.CompletedAt.IsZero() {
		report.CompletedAt = time.Now().UTC()
	}
	if report.TenantID == "" {
		report.TenantID = tenantID
	}
	if report.RequestID == "" {
		report.RequestID = requestID
	}

	_ = w.Audit.Append(ctx, audit.Entry{
		TenantID: tenantID, UserID: "system",
		Action: ActionComplete, Target: requestID,
		IP: "", UserAgent: "erasure-worker",
	})

	if err := w.Service.markComplete(ctx, requestID, report); err != nil {
		w.logf("markComplete request=%s: %v", requestID, err)
		_ = w.Queue.Nack(ctx, item.GetID(), claimToken)
		return
	}
	if err := w.Queue.Ack(ctx, item.GetID(), claimToken); err != nil {
		w.logf("ack request=%s: %v", requestID, err)
	}
}

func (w *Worker) logf(format string, args ...any) {
	if w.Logger != nil {
		w.Logger.Printf(format, args...)
		return
	}
	log.Printf("erasure: "+format, args...)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func newClaimToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("erasure-%s", hex.EncodeToString(b[:]))
}
