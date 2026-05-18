package billing

import (
	"context"
	"errors"
	"testing"
)

// alwaysFailDispatcher returns the same error on every Dispatch call.
// Used to drive a row to the dead-letter threshold.
type alwaysFailDispatcher struct {
	err error
}

func (a *alwaysFailDispatcher) Dispatch(_ context.Context, _ PendingEvent) (DispatchPlan, error) {
	return DispatchPlan{}, a.err
}

// rowState reads the failure_count + dead_lettered_at fields for one
// entry. Both columns are NULLable so we use COALESCE for the timestamp
// to keep the scan simple.
func rowState(t *testing.T, s *Service, entryID string) (int, string) {
	t.Helper()
	var fc int
	var dl string
	if err := s.db.QueryRow(
		`SELECT failure_count, COALESCE(dead_lettered_at,'')
		 FROM billing_outbox WHERE entry_id=?`, entryID,
	).Scan(&fc, &dl); err != nil {
		t.Fatalf("rowState scan: %v", err)
	}
	return fc, dl
}

// TestReconcile_DeadLetter_AfterThreshold: a permanently-bad row is
// dead-lettered exactly on the Nth attempt and skipped silently
// thereafter (no further log spam, no further failure_count bumps).
func TestReconcile_DeadLetter_AfterThreshold(t *testing.T) {
	t.Setenv(DeadLetterThresholdEnv, "5")

	svc, _ := newDispatchSvc(t)
	svc.SetDispatcher(&alwaysFailDispatcher{err: errors.New("catalog: no StripeRef for campaign-swarm:campaigns_launched")})
	recordOne(t, svc, "campaign-swarm@v1", "campaigns_launched", "bad-row", 1)

	// 4 attempts: each bumps failure_count, dead_lettered_at stays NULL.
	for i := 1; i <= 4; i++ {
		n, err := svc.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("Reconcile %d: unexpected error %v", i, err)
		}
		if n != 0 {
			t.Errorf("Reconcile %d emitted: got %d, want 0", i, n)
		}
		fc, dl := rowState(t, svc, "bad-row")
		if fc != i {
			t.Errorf("after attempt %d: failure_count=%d, want %d", i, fc, i)
		}
		if dl != "" {
			t.Errorf("after attempt %d: prematurely dead-lettered: %q", i, dl)
		}
	}

	// 5th attempt: dead_lettered_at gets set.
	if _, err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile 5: unexpected error %v", err)
	}
	fc, dl := rowState(t, svc, "bad-row")
	if fc != 5 {
		t.Errorf("after attempt 5: failure_count=%d, want 5", fc)
	}
	if dl == "" {
		t.Errorf("after attempt 5: expected dead_lettered_at to be set")
	}

	// 6th call: row is filtered out by the dead_lettered_at IS NULL
	// scan, so failure_count must NOT advance.
	if _, err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile 6: unexpected error %v", err)
	}
	fc6, dl6 := rowState(t, svc, "bad-row")
	if fc6 != 5 {
		t.Errorf("after attempt 6 (post-DL): failure_count=%d, want 5 (row should be skipped)", fc6)
	}
	if dl6 != dl {
		t.Errorf("after attempt 6: dead_lettered_at changed: %q -> %q", dl, dl6)
	}
}

// TestReconcile_DeadLetter_EnvOverride: OWERA_DEAD_LETTER_THRESHOLD=3
// dead-letters on the 3rd attempt.
func TestReconcile_DeadLetter_EnvOverride(t *testing.T) {
	t.Setenv(DeadLetterThresholdEnv, "3")

	svc, _ := newDispatchSvc(t)
	svc.SetDispatcher(&alwaysFailDispatcher{err: errors.New("catalog: transient")})
	recordOne(t, svc, "x@v1", "y", "env-bad", 1)

	for i := 1; i <= 2; i++ {
		if _, err := svc.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile %d: %v", i, err)
		}
		_, dl := rowState(t, svc, "env-bad")
		if dl != "" {
			t.Fatalf("prematurely dead-lettered on attempt %d (threshold=3)", i)
		}
	}
	if _, err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile 3: %v", err)
	}
	fc, dl := rowState(t, svc, "env-bad")
	if fc != 3 {
		t.Errorf("failure_count: got %d, want 3", fc)
	}
	if dl == "" {
		t.Errorf("expected dead-letter on attempt 3, dead_lettered_at empty")
	}
}

// TestReconcile_DeadLetter_DefaultThreshold: with no env var set, the
// default threshold (10) governs.
func TestReconcile_DeadLetter_DefaultThreshold(t *testing.T) {
	// Belt-and-braces: explicitly unset to override anything inherited.
	t.Setenv(DeadLetterThresholdEnv, "")

	svc, _ := newDispatchSvc(t)
	svc.SetDispatcher(&alwaysFailDispatcher{err: errors.New("permafail")})
	recordOne(t, svc, "x@v1", "y", "default-bad", 1)

	// 9 attempts: still pending.
	for i := 1; i <= 9; i++ {
		if _, err := svc.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile %d: %v", i, err)
		}
		_, dl := rowState(t, svc, "default-bad")
		if dl != "" {
			t.Fatalf("dead-lettered too early on attempt %d (default threshold=10)", i)
		}
	}
	// 10th: dead-letter.
	if _, err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile 10: %v", err)
	}
	fc, dl := rowState(t, svc, "default-bad")
	if fc != DefaultDeadLetterThreshold {
		t.Errorf("failure_count: got %d, want %d", fc, DefaultDeadLetterThreshold)
	}
	if dl == "" {
		t.Errorf("expected dead-letter at default threshold")
	}
}

// TestReconcile_DeadLetter_NewRowsNotPenalised: a fresh row enters with
// failure_count=0 and must be emitted on its first attempt — not treated
// as "already failed" by any off-by-one in the threshold check.
func TestReconcile_DeadLetter_NewRowsNotPenalised(t *testing.T) {
	t.Setenv(DeadLetterThresholdEnv, "1")

	svc, f := newDispatchSvc(t)
	// No dispatcher → all-metered default; backend never errors.
	recordOne(t, svc, "triage-watch@v1", "tickets_processed", "fresh", 2)

	n, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n != 1 {
		t.Errorf("emitted: got %d, want 1 (fresh row, no failures)", n)
	}
	if len(f.Records) != 1 {
		t.Errorf("Records: got %d, want 1", len(f.Records))
	}
	_, dl := rowState(t, svc, "fresh")
	if dl != "" {
		t.Errorf("fresh row was dead-lettered: %q", dl)
	}
}

// TestReconcile_DeadLetter_RowDoesNotBlockQueue preserves the PR #41
// invariant under the new dead-letter regime: a bad row sitting at any
// failure_count below threshold does not stop other rows from flushing.
func TestReconcile_DeadLetter_RowDoesNotBlockQueue(t *testing.T) {
	t.Setenv(DeadLetterThresholdEnv, "10")

	svc, f := newDispatchSvc(t)
	// Dispatcher fails for one specific bad SKU; the stub-dispatcher's
	// `err` field applies to all calls, so we use a SKU-aware variant.
	svc.SetDispatcher(&selectiveFailDispatcher{badSKU: "bad@v1"})

	recordOne(t, svc, "good@v1", "good_meter", "g1", 1)
	recordOne(t, svc, "bad@v1", "bad_meter", "b1", 1)
	recordOne(t, svc, "good@v1", "good_meter", "g2", 2)

	n, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if n != 2 {
		t.Errorf("emitted: got %d, want 2 (two good rows; bad row stays pending)", n)
	}
	if len(f.Records) != 2 {
		t.Errorf("Records: got %d, want 2", len(f.Records))
	}
	fc, dl := rowState(t, svc, "b1")
	if fc != 1 {
		t.Errorf("bad row failure_count: got %d, want 1", fc)
	}
	if dl != "" {
		t.Errorf("bad row prematurely dead-lettered: %q", dl)
	}
}

// selectiveFailDispatcher fails on one specific SKU; everything else
// gets the default metered plan.
type selectiveFailDispatcher struct{ badSKU string }

func (s *selectiveFailDispatcher) Dispatch(_ context.Context, p PendingEvent) (DispatchPlan, error) {
	if p.SKU == s.badSKU {
		return DispatchPlan{}, errors.New("catalog: no StripeRef")
	}
	return DispatchPlan{Kind: DispatchKindMetered, MeterName: p.Meter}, nil
}

// TestReconcile_DeadLetter_SuccessAfterFailuresDoesNotReset: if a row
// finally flushes after a few failures, it transitions to billed_at
// set (we don't auto-recover failure_count, but that's harmless — the
// row no longer enters the scan).
func TestReconcile_DeadLetter_SuccessAfterFailuresDoesNotReset(t *testing.T) {
	t.Setenv(DeadLetterThresholdEnv, "10")

	svc, _ := newDispatchSvc(t)
	flake := &flakyDispatcher{failsRemaining: 3}
	svc.SetDispatcher(flake)
	recordOne(t, svc, "x@v1", "y", "recoverable", 1)

	// 3 failed attempts.
	for i := 1; i <= 3; i++ {
		if _, err := svc.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile %d: %v", i, err)
		}
	}
	fc, dl := rowState(t, svc, "recoverable")
	if fc != 3 {
		t.Errorf("pre-recovery failure_count: got %d, want 3", fc)
	}
	if dl != "" {
		t.Errorf("dead-lettered before threshold: %q", dl)
	}

	// 4th attempt: dispatcher succeeds; row gets billed_at set.
	n, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile 4: %v", err)
	}
	if n != 1 {
		t.Errorf("emitted on recovery: got %d, want 1", n)
	}

	// failure_count is NOT reset (documented intent: we don't auto-
	// recover the counter; the row leaves the pending scan via
	// billed_at instead).
	fc2, dl2 := rowState(t, svc, "recoverable")
	if fc2 != 3 {
		t.Errorf("post-recovery failure_count: got %d, want 3 (no reset)", fc2)
	}
	if dl2 != "" {
		t.Errorf("post-recovery dead_lettered_at: got %q, want empty", dl2)
	}

	// 5th call: row no longer pending, so Reconcile is a no-op.
	n2, err := svc.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile 5: %v", err)
	}
	if n2 != 0 {
		t.Errorf("post-billed Reconcile emitted: got %d, want 0", n2)
	}
}

// flakyDispatcher returns an error for the first N calls, then succeeds.
type flakyDispatcher struct{ failsRemaining int }

func (f *flakyDispatcher) Dispatch(_ context.Context, p PendingEvent) (DispatchPlan, error) {
	if f.failsRemaining > 0 {
		f.failsRemaining--
		return DispatchPlan{}, errors.New("transient")
	}
	return DispatchPlan{Kind: DispatchKindMetered, MeterName: p.Meter}, nil
}

// TestReconcile_DeadLetter_PreservedAcrossMigrate: re-calling migrate()
// on an existing DB (idempotency) does not drop or reset any of the new
// columns.
func TestReconcile_DeadLetter_PreservedAcrossMigrate(t *testing.T) {
	svc, _ := newDispatchSvc(t)
	svc.SetDispatcher(&alwaysFailDispatcher{err: errors.New("boom")})
	recordOne(t, svc, "x@v1", "y", "persist", 1)

	if _, err := svc.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	fcBefore, _ := rowState(t, svc, "persist")
	if fcBefore != 1 {
		t.Fatalf("setup: failure_count=%d, want 1", fcBefore)
	}

	// Idempotency check: re-running migrate must not error or wipe state.
	if err := svc.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	fcAfter, _ := rowState(t, svc, "persist")
	if fcAfter != fcBefore {
		t.Errorf("failure_count clobbered by re-migrate: before=%d after=%d", fcBefore, fcAfter)
	}
}

// TestService_ListDeadLetters: after a row dead-letters, it surfaces in
// the operator-facing list with the recorded metadata.
func TestService_ListDeadLetters(t *testing.T) {
	t.Setenv(DeadLetterThresholdEnv, "2")

	svc, _ := newDispatchSvc(t)
	svc.SetDispatcher(&alwaysFailDispatcher{err: errors.New("catalog: gone")})
	recordOne(t, svc, "campaign-swarm@v1", "campaigns_launched", "dl-1", 1)

	for i := 0; i < 2; i++ {
		if _, err := svc.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile %d: %v", i, err)
		}
	}
	got, err := svc.ListDeadLetters(context.Background())
	if err != nil {
		t.Fatalf("ListDeadLetters: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListDeadLetters: got %d rows, want 1", len(got))
	}
	if got[0].EntryID != "dl-1" {
		t.Errorf("entry_id: got %q, want dl-1", got[0].EntryID)
	}
	if got[0].FailureCount != 2 {
		t.Errorf("failure_count: got %d, want 2", got[0].FailureCount)
	}
	if got[0].LastError == "" {
		t.Errorf("last_error empty; want recorded message")
	}
	if got[0].DeadLetteredAt == "" {
		t.Errorf("dead_lettered_at empty on dead-lettered row")
	}
}
