package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/owera/owera-cloud/api/internal/identity"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	idStore, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = idStore.Close() })
	js, err := New(idStore.DB())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	ten, _ := idStore.CreateTenant(context.Background(), "Acme")
	return js, ten.ID
}

func TestSubmit_CreatesSubmitted(t *testing.T) {
	js, ten := newTestStore(t)
	j, created, err := js.Submit(context.Background(), ten, "triage-watch@v1",
		map[string]any{"queue_url": "https://x.example/q"}, "")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if j.Status != StatusSubmitted {
		t.Fatalf("status: got %q want submitted", j.Status)
	}
}

func TestSubmit_IdempotencyReturnsExisting(t *testing.T) {
	js, ten := newTestStore(t)
	ctx := context.Background()
	j1, c1, _ := js.Submit(ctx, ten, "triage-watch@v1", map[string]any{}, "key-abc")
	j2, c2, err := js.Submit(ctx, ten, "triage-watch@v1", map[string]any{}, "key-abc")
	if err != nil {
		t.Fatalf("second Submit: %v", err)
	}
	if !c1 {
		t.Fatal("first Submit should have created")
	}
	if c2 {
		t.Fatal("second Submit should not have created")
	}
	if j1.ID != j2.ID {
		t.Fatalf("idempotency: ids differ %q vs %q", j1.ID, j2.ID)
	}
}

func TestTransition_HappyPath(t *testing.T) {
	js, ten := newTestStore(t)
	ctx := context.Background()
	j, _, _ := js.Submit(ctx, ten, "triage-watch@v1", map[string]any{}, "")

	j, err := js.Transition(ctx, ten, j.ID, StatusQueued)
	if err != nil {
		t.Fatalf("submitted->queued: %v", err)
	}
	if j.Status != StatusQueued {
		t.Fatalf("status: got %q want queued", j.Status)
	}
	j, err = js.Transition(ctx, ten, j.ID, StatusRunning, WithOperatorTaskID("task_99"))
	if err != nil {
		t.Fatalf("queued->running: %v", err)
	}
	if j.OperatorTaskID != "task_99" {
		t.Fatalf("operator task id: got %q want task_99", j.OperatorTaskID)
	}
	j, err = js.Transition(ctx, ten, j.ID, StatusSucceeded, WithOutputs(map[string]any{"ok": true}))
	if err != nil {
		t.Fatalf("running->succeeded: %v", err)
	}
	if j.Outputs["ok"] != true {
		t.Fatalf("outputs: got %#v", j.Outputs)
	}
}

func TestTransition_RejectsInvalid(t *testing.T) {
	js, ten := newTestStore(t)
	ctx := context.Background()
	j, _, _ := js.Submit(ctx, ten, "triage-watch@v1", map[string]any{}, "")
	// submitted -> running is not allowed (must go via queued)
	_, err := js.Transition(ctx, ten, j.ID, StatusRunning)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestList_FiltersByTenant(t *testing.T) {
	js, ten := newTestStore(t)
	ctx := context.Background()
	_, _, _ = js.Submit(ctx, ten, "triage-watch@v1", map[string]any{"q": 1}, "")
	_, _, _ = js.Submit(ctx, ten, "triage-watch@v1", map[string]any{"q": 2}, "")

	// Now insert via another tenant id and confirm it's not returned.
	_, _, _ = js.Submit(ctx, "ten_other", "triage-watch@v1", map[string]any{"q": 3}, "")

	jobs, _, err := js.List(ctx, ListParams{TenantID: ten, Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len: got %d want 2", len(jobs))
	}
	for _, j := range jobs {
		if j.TenantID != ten {
			t.Fatalf("leak: tenant_id %q", j.TenantID)
		}
	}
}

func TestGet_CrossTenantReadReturnsNotFound(t *testing.T) {
	js, ten := newTestStore(t)
	ctx := context.Background()
	j, _, _ := js.Submit(ctx, ten, "triage-watch@v1", map[string]any{}, "")

	if _, err := js.Get(ctx, "ten_other", j.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant Get: got %v want ErrNotFound", err)
	}
}

func TestStatus_IsTerminal(t *testing.T) {
	for _, s := range []Status{StatusSucceeded, StatusFailed, StatusCancelled} {
		if !s.IsTerminal() {
			t.Fatalf("%s should be terminal", s)
		}
	}
	for _, s := range []Status{StatusSubmitted, StatusQueued, StatusRunning} {
		if s.IsTerminal() {
			t.Fatalf("%s should not be terminal", s)
		}
	}
}
