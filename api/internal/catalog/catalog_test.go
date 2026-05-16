package catalog

import (
	"context"
	"strings"
	"testing"
)

func TestLookup_FindsTriageWatch(t *testing.T) {
	s, err := Lookup("triage-watch@v1")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if s.Name != "triage-watch" {
		t.Fatalf("name: got %q want triage-watch", s.Name)
	}
}

func TestLookup_NameOnlyReturnsLatest(t *testing.T) {
	s, err := Lookup("triage-watch")
	if err != nil {
		t.Fatalf("Lookup name-only: %v", err)
	}
	if s.FullName() != "triage-watch@v1" {
		t.Fatalf("full name: got %q want triage-watch@v1", s.FullName())
	}
}

func TestLookup_NotFound(t *testing.T) {
	if _, err := Lookup("does-not-exist@v9"); err == nil {
		t.Fatal("expected error")
	}
}

func TestList_IsDeterministic(t *testing.T) {
	skus := List()
	if len(skus) < 2 {
		t.Fatalf("List: got %d want >= 2 (seed SKUs)", len(skus))
	}
	for i := 1; i < len(skus); i++ {
		if skus[i-1].FullName() >= skus[i].FullName() {
			t.Fatalf("List not sorted: %q >= %q", skus[i-1].FullName(), skus[i].FullName())
		}
	}
}

func TestTriageWatch_ValidatesGoodInputs(t *testing.T) {
	if err := TriageWatchV1.ValidateInputs(map[string]any{
		"queue_url":          "https://acme.example/queue",
		"priority_threshold": 7,
	}); err != nil {
		t.Fatalf("ValidateInputs: %v", err)
	}
}

func TestTriageWatch_RejectsMissingRequired(t *testing.T) {
	err := TriageWatchV1.ValidateInputs(map[string]any{
		"priority_threshold": 7,
	})
	if err == nil {
		t.Fatal("expected error for missing queue_url")
	}
	if !strings.Contains(err.Error(), "queue_url") {
		t.Fatalf("error should mention queue_url: %v", err)
	}
}

func TestTriageWatch_RejectsWrongType(t *testing.T) {
	err := TriageWatchV1.ValidateInputs(map[string]any{
		"queue_url":          "https://acme.example/queue",
		"priority_threshold": "not-a-number",
	})
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestTriageWatch_DispatcherShapesPayload(t *testing.T) {
	payload, err := TriageWatchV1.Dispatcher(context.Background(), "job_123", map[string]any{
		"queue_url":          "https://acme.example/queue",
		"priority_threshold": float64(9),
	})
	if err != nil {
		t.Fatalf("Dispatcher: %v", err)
	}
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type: got %T", payload)
	}
	if m["sku"] != "triage-watch@v1" {
		t.Fatalf("sku: got %v", m["sku"])
	}
	if m["job_id"] != "job_123" {
		t.Fatalf("job_id: got %v", m["job_id"])
	}
	if m["priority_threshold"] != 9 {
		t.Fatalf("priority_threshold: got %v", m["priority_threshold"])
	}
}

func TestCampaignSwarm_RejectsShortBrief(t *testing.T) {
	if err := CampaignSwarmV1.ValidateInputs(map[string]any{
		"brief":            "too short",
		"audience_segment": "smb",
	}); err == nil {
		t.Fatal("expected error for short brief")
	}
}

func TestCampaignSwarm_ValidatesGoodInputs(t *testing.T) {
	if err := CampaignSwarmV1.ValidateInputs(map[string]any{
		"brief":            "Launch our Q4 fleet automation campaign across SMB CTOs",
		"audience_segment": "smb-cto",
		"channels":         []any{"email", "linkedin"},
		"max_outreach":     250,
	}); err != nil {
		t.Fatalf("ValidateInputs: %v", err)
	}
}
