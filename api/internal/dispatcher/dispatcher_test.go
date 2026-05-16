package dispatcher

import (
	"context"
	"errors"
	"testing"

	_ "github.com/owera/owera-cloud/api/internal/catalog" // registers seed SKUs
)

func TestDispatch_HappyPath(t *testing.T) {
	tr := NewInMemoryTransport()
	d := New(tr)
	taskID, err := d.Dispatch(context.Background(), "ten_a", "job_1", "triage-watch@v1", map[string]any{
		"queue_url": "https://acme.example/q",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if taskID != "task_synthetic" {
		t.Fatalf("task id: got %q want task_synthetic", taskID)
	}
	if tr.CallCount() != 1 {
		t.Fatalf("call count: got %d want 1", tr.CallCount())
	}
	if tr.Calls[0].Method != "fleet.SubmitJob" {
		t.Fatalf("method: got %q want fleet.SubmitJob", tr.Calls[0].Method)
	}
}

func TestDispatch_UnknownSKU(t *testing.T) {
	d := New(NewInMemoryTransport())
	_, err := d.Dispatch(context.Background(), "ten_a", "job_1", "ghost@v1", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDispatch_BadInputs(t *testing.T) {
	d := New(NewInMemoryTransport())
	_, err := d.Dispatch(context.Background(), "ten_a", "job_1", "triage-watch@v1", map[string]any{
		// missing required queue_url
		"priority_threshold": 7,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDispatch_TransportError(t *testing.T) {
	tr := NewInMemoryTransport()
	tr.Responder = func(string, any) (any, error) { return nil, errors.New("boom") }
	d := New(tr)
	_, err := d.Dispatch(context.Background(), "ten_a", "job_1", "triage-watch@v1", map[string]any{
		"queue_url": "https://x.example/q",
	})
	if err == nil {
		t.Fatal("expected transport error")
	}
}

func TestCancel_HappyPath(t *testing.T) {
	tr := NewInMemoryTransport()
	d := New(tr)
	if err := d.Cancel(context.Background(), "task_99"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if tr.Calls[0].Method != "fleet.CancelTask" {
		t.Fatalf("method: got %q want fleet.CancelTask", tr.Calls[0].Method)
	}
}

func TestCancel_OperatorRejects(t *testing.T) {
	tr := NewInMemoryTransport()
	tr.Responder = func(string, any) (any, error) {
		return map[string]any{"cancelled": false}, nil
	}
	d := New(tr)
	if err := d.Cancel(context.Background(), "task_99"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNoTransport(t *testing.T) {
	d := New(nil)
	if _, err := d.Dispatch(context.Background(), "ten_a", "job_1", "triage-watch@v1", map[string]any{
		"queue_url": "https://x.example/q",
	}); err == nil {
		t.Fatal("expected error for nil transport")
	}
}
