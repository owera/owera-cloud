package catalog

import (
	"context"
	"strings"
	"testing"
)

func TestLookup_FindsCodeAudit(t *testing.T) {
	s, err := Lookup("code-audit@v1")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if s.Name != "code-audit" {
		t.Fatalf("name: got %q want code-audit", s.Name)
	}
	if s.Pricing.Model != "monthly_subscription" {
		t.Fatalf("pricing model: got %q want monthly_subscription", s.Pricing.Model)
	}
	if s.BillingMeter != "findings_reported" {
		t.Fatalf("billing meter: got %q want findings_reported", s.BillingMeter)
	}
}

func TestCodeAudit_ValidatesGoodInputs(t *testing.T) {
	cases := []map[string]any{
		{"repo_url": "https://github.com/owera/owera-cloud"},
		{"repo_url": "https://github.com/owera/owera-fleet.git", "branch": "feat/h4-v1-sku-stubs"},
		{"repo_url": "git@github.com:owera/owera-cloud.git", "severity_threshold": "high"},
		{"repo_url": "ssh://git@gitlab.example.com:2222/owera/internal.git", "branch": "release-1.0", "severity_threshold": "low"},
	}
	for i, c := range cases {
		if err := CodeAuditV1.ValidateInputs(c); err != nil {
			t.Errorf("case %d ValidateInputs: %v (inputs=%+v)", i, err, c)
		}
	}
}

func TestCodeAudit_DefaultsApplyViaDispatcher(t *testing.T) {
	payload, err := CodeAuditV1.Dispatcher(context.Background(), "job_ca_1", map[string]any{
		"repo_url": "https://github.com/owera/owera-cloud",
	})
	if err != nil {
		t.Fatalf("Dispatcher: %v", err)
	}
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type: got %T", payload)
	}
	if m["sku"] != "code-audit@v1" {
		t.Fatalf("sku: got %v", m["sku"])
	}
	if m["branch"] != "main" {
		t.Fatalf("default branch: got %v want main", m["branch"])
	}
	if m["severity_threshold"] != "medium" {
		t.Fatalf("default severity_threshold: got %v want medium", m["severity_threshold"])
	}
}

func TestCodeAudit_RejectsMissingRepoURL(t *testing.T) {
	err := CodeAuditV1.ValidateInputs(map[string]any{
		"branch": "main",
	})
	if err == nil {
		t.Fatal("expected error for missing repo_url")
	}
	if !strings.Contains(err.Error(), "repo_url") {
		t.Fatalf("error should mention repo_url: %v", err)
	}
}

func TestCodeAudit_RejectsMalformedRepoURL(t *testing.T) {
	bad := []string{
		"not-a-url-at-all",
		"ftp://example.com/repo.git",
		"github.com/owera/owera-cloud", // missing scheme
		"",
	}
	for _, u := range bad {
		err := CodeAuditV1.ValidateInputs(map[string]any{
			"repo_url": u,
		})
		if err == nil {
			t.Errorf("expected error for malformed repo_url %q", u)
		}
	}
}

func TestCodeAudit_RejectsInvalidSeverity(t *testing.T) {
	err := CodeAuditV1.ValidateInputs(map[string]any{
		"repo_url":           "https://github.com/owera/owera-cloud",
		"severity_threshold": "critical", // not in enum
	})
	if err == nil {
		t.Fatal("expected error for severity_threshold outside enum")
	}
}

func TestCodeAudit_DispatcherEchoesAllFields(t *testing.T) {
	payload, err := CodeAuditV1.Dispatcher(context.Background(), "job_ca_2", map[string]any{
		"repo_url":           "git@github.com:owera/owera-fleet.git",
		"branch":             "release/v0.5",
		"severity_threshold": "high",
	})
	if err != nil {
		t.Fatalf("Dispatcher: %v", err)
	}
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type: got %T", payload)
	}
	if m["job_id"] != "job_ca_2" {
		t.Fatalf("job_id: got %v", m["job_id"])
	}
	if m["repo_url"] != "git@github.com:owera/owera-fleet.git" {
		t.Fatalf("repo_url: got %v", m["repo_url"])
	}
	if m["branch"] != "release/v0.5" {
		t.Fatalf("branch: got %v", m["branch"])
	}
	if m["severity_threshold"] != "high" {
		t.Fatalf("severity_threshold: got %v", m["severity_threshold"])
	}
}
