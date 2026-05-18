package catalog

import (
	"context"
	"strings"
	"testing"
)

func TestLookup_FindsResearchBrief(t *testing.T) {
	s, err := Lookup("research-brief@v1")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if s.Name != "research-brief" {
		t.Fatalf("name: got %q want research-brief", s.Name)
	}
	if s.Pricing.Model != "per_job_fixed" {
		t.Fatalf("pricing model: got %q want per_job_fixed", s.Pricing.Model)
	}
	if s.BillingMeter != "briefs_delivered" {
		t.Fatalf("billing meter: got %q want briefs_delivered", s.BillingMeter)
	}
}

func TestResearchBrief_ValidatesGoodInputs(t *testing.T) {
	if err := ResearchBriefV1.ValidateInputs(map[string]any{
		"topic":              "Competitive landscape for AI-native developer tools in 2026",
		"depth":              "L",
		"citations_required": true,
	}); err != nil {
		t.Fatalf("ValidateInputs: %v", err)
	}
}

func TestResearchBrief_DefaultsApplyViaDispatcher(t *testing.T) {
	// `depth` and `citations_required` should default at dispatch time
	// when omitted. Validation accepts the inputs because both fields
	// are optional in the schema.
	if err := ResearchBriefV1.ValidateInputs(map[string]any{
		"topic": "Owera Fleet competitive teardown",
	}); err != nil {
		t.Fatalf("ValidateInputs: %v", err)
	}
	payload, err := ResearchBriefV1.Dispatcher(context.Background(), "job_rb_1", map[string]any{
		"topic": "Owera Fleet competitive teardown",
	})
	if err != nil {
		t.Fatalf("Dispatcher: %v", err)
	}
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type: got %T", payload)
	}
	if m["sku"] != "research-brief@v1" {
		t.Fatalf("sku: got %v", m["sku"])
	}
	if m["depth"] != "M" {
		t.Fatalf("default depth: got %v want M", m["depth"])
	}
	if m["citations_required"] != true {
		t.Fatalf("default citations_required: got %v want true", m["citations_required"])
	}
}

func TestResearchBrief_RejectsMissingTopic(t *testing.T) {
	err := ResearchBriefV1.ValidateInputs(map[string]any{
		"depth": "M",
	})
	if err == nil {
		t.Fatal("expected error for missing topic")
	}
	if !strings.Contains(err.Error(), "topic") {
		t.Fatalf("error should mention topic: %v", err)
	}
}

func TestResearchBrief_RejectsShortTopic(t *testing.T) {
	err := ResearchBriefV1.ValidateInputs(map[string]any{
		"topic": "abc",
	})
	if err == nil {
		t.Fatal("expected error for topic shorter than 5 chars")
	}
}

func TestResearchBrief_RejectsInvalidDepth(t *testing.T) {
	err := ResearchBriefV1.ValidateInputs(map[string]any{
		"topic": "Competitive landscape teardown",
		"depth": "XL", // not in enum
	})
	if err == nil {
		t.Fatal("expected error for depth outside enum")
	}
}

func TestResearchBrief_DispatcherEchoesDepth(t *testing.T) {
	payload, err := ResearchBriefV1.Dispatcher(context.Background(), "job_rb_2", map[string]any{
		"topic":              "Owera fleet GTM analysis",
		"depth":              "S",
		"citations_required": false,
	})
	if err != nil {
		t.Fatalf("Dispatcher: %v", err)
	}
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type: got %T", payload)
	}
	if m["job_id"] != "job_rb_2" {
		t.Fatalf("job_id: got %v", m["job_id"])
	}
	if m["depth"] != "S" {
		t.Fatalf("depth: got %v want S", m["depth"])
	}
	if m["citations_required"] != false {
		t.Fatalf("citations_required: got %v want false", m["citations_required"])
	}
}
