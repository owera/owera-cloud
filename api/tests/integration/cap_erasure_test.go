// Integration tests for the WS-16 cost-cap rejection path and the WS-18
// LGPD/GDPR erasure path. The HTTP server, identity, jobs, queue, audit,
// billing, costcap, and erasure are all real and wired through the same
// `server.Deps` shape main.go uses.
//
// jobs_roundtrip_test.go covers the WS-14 happy-path round trip. This
// file covers WS-16 + WS-18 — the two workstreams that had only unit-test
// coverage at Wave 8 close.
package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/billing"
	_ "github.com/owera/owera-cloud/api/internal/catalog" // register SKUs
	"github.com/owera/owera-cloud/api/internal/dispatcher"
	"github.com/owera/owera-cloud/api/internal/erasure"
	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
	"github.com/owera/owera-cloud/api/internal/server"
	"github.com/owera/owera-cloud/api/internal/status"

	"net/http/httptest"
)

// fullHarness wires CostCap and Erasure in addition to the components
// covered by newHarness. The simpler harness is preserved for the
// existing WS-14 round-trip tests.
type fullHarness struct {
	*harness
	audit   *audit.Log
	billing *billing.Service
	erasure *erasure.Service
	caps    *stubCapStore
}

// stubCapStore lets each test set the tenant's cap inline.
type stubCapStore struct{ centsPerTenant map[string]int64 }

func (s *stubCapStore) GetMonthlyCapCents(_ context.Context, tenantID string) (int64, error) {
	if v, ok := s.centsPerTenant[tenantID]; ok {
		return v, nil
	}
	return 0, nil
}

// stubPricer charges a flat cost per job regardless of inputs (matching
// the simplest catalog.Pricing.metered shape).
type stubPricer struct{ flatCents int64 }

func (p *stubPricer) EstimateCents(_ string, inputs map[string]any) (int64, error) {
	// Honor a "_units" hint so the cap test can drive the over-cap path
	// by varying the spent-so-far without needing to submit many jobs.
	if u, ok := inputs["_units"]; ok {
		switch v := u.(type) {
		case int64:
			return v * p.flatCents, nil
		case int:
			return int64(v) * p.flatCents, nil
		case float64:
			return int64(v) * p.flatCents, nil
		}
	}
	return p.flatCents, nil
}

func newFullHarness(t *testing.T, capCents int64) *fullHarness {
	t.Helper()
	id, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = id.Close() })

	js, err := jobs.New(id.DB())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	q, err := queue.NewSQLite(id.DB())
	if err != nil {
		t.Fatalf("queue.NewSQLite: %v", err)
	}
	al, err := audit.New(id.DB())
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}
	bs, err := billing.New(id.DB(), &billing.FakeBackend{})
	if err != nil {
		t.Fatalf("billing.New: %v", err)
	}
	es, err := erasure.New(id.DB(), erasure.AdaptQueue(q), al)
	if err != nil {
		t.Fatalf("erasure.New: %v", err)
	}

	transport := dispatcher.NewInMemoryTransport()
	disp := dispatcher.New(transport)
	st := status.New(transport, 30*time.Second)

	caps := &stubCapStore{centsPerTenant: map[string]int64{}}
	pricer := &stubPricer{flatCents: 10_000} // $100.00 per job
	cap, err := billing.NewCostCap(bs, caps, pricer, capCents, nil)
	if err != nil {
		t.Fatalf("NewCostCap: %v", err)
	}

	h := server.New(server.Deps{
		Identity:   id,
		Jobs:       js,
		Queue:      q,
		Dispatcher: disp,
		Audit:      al,
		Billing:    bs,
		CostCap:    cap,
		Status:     st,
		Erasure:    es,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	tenant, err := id.CreateTenant(ctx, "fixture-acme-full")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	user, err := id.CreateUser(ctx, tenant.ID, "fixture-1@example.invalid")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tok, _, err := id.IssueAPIKey(ctx, tenant.ID, user.ID, "integration-full")
	if err != nil {
		t.Fatalf("IssueAPIKey: %v", err)
	}

	return &fullHarness{
		harness: &harness{
			srv:        srv,
			token:      tok,
			tenantID:   tenant.ID,
			jobs:       js,
			queue:      q,
			transport:  transport,
			dispatcher: disp,
		},
		audit:   al,
		billing: bs,
		erasure: es,
		caps:    caps,
	}
}

// TestIntegration_CostCapBlocksSubmit verifies the WS-16 T16.5 path:
// a job whose estimated cost would push the tenant past its monthly cap
// is rejected at submission with 402 + Retry-After.
//
// The cap is set to $50 and the SKU costs $100/job — first submit should
// be rejected (estimate exceeds remaining headroom).
func TestIntegration_CostCapBlocksSubmit(t *testing.T) {
	h := newFullHarness(t, 50_000)            // $500 default cap (not the trigger; per-tenant overrides win)
	h.caps.centsPerTenant[h.tenantID] = 5_000 // $50/month — below the $100 per-job estimate

	resp, body := h.do(t, "POST", "/v1/jobs", map[string]any{
		"sku": "triage-watch@v1",
		"inputs": map[string]any{
			"queue_url": "https://example.invalid/q",
		},
	})
	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("status: got %d want 402, body=%s", resp.StatusCode, body)
	}
	if ra := resp.Header.Get("Retry-After"); ra == "" {
		t.Errorf("expected Retry-After header; got empty")
	}
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if env.Error != "cost_cap_exceeded" {
		t.Errorf("error code: got %q want cost_cap_exceeded", env.Error)
	}
}

// TestIntegration_ErasureRequest_QueuesAndAudits verifies the WS-18
// T18.2 path: DELETE /v1/tenants/me/data returns 202, writes an audit
// row, and enqueues a deletion job for the erasure worker. SLA must be
// ≤15 working days (LGPD Art. 18).
func TestIntegration_ErasureRequest_QueuesAndAudits(t *testing.T) {
	h := newFullHarness(t, 1_000_000) // cap irrelevant for this test

	resp, body := h.do(t, "DELETE", "/v1/tenants/me/data", nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status: got %d want 202, body=%s", resp.StatusCode, body)
	}
	var env struct {
		RequestID   string    `json:"request_id"`
		State       string    `json:"state"`
		RequestedAt time.Time `json:"requested_at"`
		SLADueAt    time.Time `json:"sla_due_at"`
		Notice      string    `json:"notice"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if env.RequestID == "" {
		t.Errorf("empty request_id; body=%s", body)
	}
	if env.State != "queued" {
		t.Errorf("state: got %q want queued", env.State)
	}
	sla := env.SLADueAt.Sub(env.RequestedAt)
	maxSLA := 21 * 24 * time.Hour // 15 working days = ≤21 calendar days
	if sla > maxSLA {
		t.Errorf("SLA window %s exceeds LGPD Art. 18 ceiling (≤15 working days = ≤%s)", sla, maxSLA)
	}
	if env.Notice == "" {
		t.Errorf("notice text missing; should reference LGPD/GDPR")
	}

	// Audit row written?
	entries, err := h.audit.List(context.Background(), h.tenantID, 10)
	if err != nil {
		t.Fatalf("audit.List: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Action == erasure.ActionRequest && e.Target == env.RequestID {
			found = true
			if e.TenantID != h.tenantID {
				t.Errorf("audit tenant_id: got %q want %q", e.TenantID, h.tenantID)
			}
		}
	}
	if !found {
		t.Errorf("audit log missing %s row for %s; got %d entries", erasure.ActionRequest, env.RequestID, len(entries))
	}

	// Erasure request status visible via the GET endpoint?
	resp2, body2 := h.do(t, "GET", "/v1/tenants/me/data/erasures/"+env.RequestID, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET erasure status: got %d want 200, body=%s", resp2.StatusCode, body2)
	}
}
