package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/owera/owera-cloud/api/internal/identity"
)

// TestGetUsage_DashboardShape verifies the /v1/usage response carries the
// keys the dashboard's adaptUsage() consumes:
//
//	<sku>:jobs, <sku>:cost_cents, total_jobs, total_cost_cents
//	+ period_start / period_end (ISO-8601)
//
// Pre-fix the response was {period, meters: map[sku]units} which the
// adapter couldn't parse — the UI fell back to fixture data.
//
// This test covers the Billing=nil path (no billing service wired) — the
// keys must still be present, just with total_jobs=0, total_cost_cents=0.
// The end-to-end Billing-attached case is covered in tests/integration.
func TestGetUsage_DashboardShape_NilBilling(t *testing.T) {
	idStore := newIdentityForUsageTest(t)
	_, key := provisionTenantAndKeyForUsage(t, idStore)

	h := New(Deps{Identity: idStore}) // Billing intentionally nil
	req := httptest.NewRequest("GET", "/v1/usage?period=current", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Period      string           `json:"period"`
		PeriodStart string           `json:"period_start"`
		PeriodEnd   string           `json:"period_end"`
		Meters      map[string]int64 `json:"meters"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if got.Period != "current" {
		t.Errorf("period = %q, want %q", got.Period, "current")
	}
	if got.PeriodStart == "" || got.PeriodEnd == "" {
		t.Errorf("period_start/end empty: %+v", got)
	}
	if !strings.HasSuffix(got.PeriodStart, "T00:00:00Z") {
		t.Errorf("period_start = %q, want first second of month", got.PeriodStart)
	}
	for _, k := range []string{"total_jobs", "total_cost_cents"} {
		if _, ok := got.Meters[k]; !ok {
			t.Errorf("meters missing roll-up key %q; have %v", k, got.Meters)
		}
	}
	if got.Meters["total_jobs"] != 0 {
		t.Errorf("total_jobs = %d, want 0 (no billing wired)", got.Meters["total_jobs"])
	}
	if got.Meters["total_cost_cents"] != 0 {
		t.Errorf("total_cost_cents = %d, want 0", got.Meters["total_cost_cents"])
	}
}

func newIdentityForUsageTest(t *testing.T) *identity.Store {
	t.Helper()
	s, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func provisionTenantAndKeyForUsage(t *testing.T, s *identity.Store) (*identity.Tenant, string) {
	t.Helper()
	ctx := context.Background()
	ten, err := s.CreateTenant(ctx, "Acme")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	user, err := s.CreateUser(ctx, ten.ID, "ops@acme.example")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	plain, _, err := s.IssueAPIKey(ctx, ten.ID, user.ID, "test")
	if err != nil {
		t.Fatalf("IssueAPIKey: %v", err)
	}
	return ten, plain
}
