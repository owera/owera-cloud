// Integration tests for the WS-14 job-lifecycle round trip: HTTP submit
// → SQLite queue → worker pickup → mocked operator plane → ledger poll →
// terminal status visible via GET /v1/jobs/{id}. The HTTP server, queue,
// jobs store, and dispatcher are all real; only the operator-plane
// transport and the ledger poller are stubbed.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/billing"
	_ "github.com/owera/owera-cloud/api/internal/catalog" // register SKUs
	"github.com/owera/owera-cloud/api/internal/dispatcher"
	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
	"github.com/owera/owera-cloud/api/internal/server"
	"github.com/owera/owera-cloud/api/internal/status"
)

type harness struct {
	srv        *httptest.Server
	token      string
	tenantID   string
	jobs       *jobs.Store
	queue      *queue.SQLiteQueue
	worker     *dispatcher.Worker
	transport  *dispatcher.InMemoryTransport
	ledger     *dispatcher.SyntheticLedgerPoller
	dispatcher *dispatcher.Dispatcher
}

func newHarness(t *testing.T) *harness {
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

	transport := dispatcher.NewInMemoryTransport()
	disp := dispatcher.New(transport)
	ledger := dispatcher.NewSyntheticLedgerPoller()
	st := status.New(transport, 30*time.Second)

	cfg := dispatcher.DefaultWorkerConfig()
	cfg.ClaimToken = "worker-integration"
	cfg.LedgerBackoff = time.Millisecond
	w := dispatcher.NewWorker(q, disp, js, ledger, bs, cfg)

	h := server.New(server.Deps{
		Identity:   id,
		Jobs:       js,
		Queue:      q,
		Dispatcher: disp,
		Audit:      al,
		Billing:    bs,
		Status:     st,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	tenant, err := id.CreateTenant(ctx, "fixture-acme")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	user, err := id.CreateUser(ctx, tenant.ID, "fixture-1@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tok, _, err := id.IssueAPIKey(ctx, tenant.ID, user.ID, "integration")
	if err != nil {
		t.Fatalf("IssueAPIKey: %v", err)
	}

	return &harness{
		srv:        srv,
		token:      tok,
		tenantID:   tenant.ID,
		jobs:       js,
		queue:      q,
		worker:     w,
		transport:  transport,
		ledger:     ledger,
		dispatcher: disp,
	}
}

func (h *harness) do(t *testing.T, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rd)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestRoundTrip_SubmitStatusCancel(t *testing.T) {
	h := newHarness(t)

	// Submit
	resp, body := h.do(t, "POST", "/v1/jobs", map[string]any{
		"sku": "campaign-swarm@v1",
		"inputs": map[string]any{
			"brief":            "Fixture campaign brief that is long enough to pass schema validation",
			"audience_segment": "fixture-segment",
		},
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /v1/jobs: status %d body=%s", resp.StatusCode, body)
	}
	var created struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.JobID == "" || created.Status != "queued" {
		t.Fatalf("created: %+v", created)
	}

	// Status (pre-worker run)
	resp, body = h.do(t, "GET", "/v1/jobs/"+created.JobID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/jobs/<id>: status %d body=%s", resp.StatusCode, body)
	}

	// Cancel before the worker picks it up.
	resp, body = h.do(t, "POST", "/v1/jobs/"+created.JobID+"/cancel", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST cancel: status %d body=%s", resp.StatusCode, body)
	}

	// Confirm terminal.
	resp, body = h.do(t, "GET", "/v1/jobs/"+created.JobID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET post-cancel: status %d body=%s", resp.StatusCode, body)
	}
	var final map[string]any
	_ = json.Unmarshal(body, &final)
	if final["status"] != "cancelled" {
		t.Fatalf("status after cancel: got %v want cancelled", final["status"])
	}
}

func TestRoundTrip_DispatcherDrivesToSucceeded(t *testing.T) {
	h := newHarness(t)

	resp, body := h.do(t, "POST", "/v1/jobs", map[string]any{
		"sku": "triage-watch@v1",
		"inputs": map[string]any{
			"queue_url": "https://fixture.example/q",
		},
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /v1/jobs: %d body=%s", resp.StatusCode, body)
	}
	var created struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(body, &created)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.worker.RunOnce(ctx); err != nil {
		t.Fatalf("worker.RunOnce: %v", err)
	}

	resp, body = h.do(t, "GET", "/v1/jobs/"+created.JobID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET: %d body=%s", resp.StatusCode, body)
	}
	var fetched map[string]any
	_ = json.Unmarshal(body, &fetched)
	if fetched["status"] != "succeeded" {
		t.Fatalf("status: got %v want succeeded; body=%s", fetched["status"], body)
	}
}

func TestRoundTrip_Idempotency(t *testing.T) {
	h := newHarness(t)
	req := map[string]any{
		"sku": "triage-watch@v1",
		"inputs": map[string]any{
			"queue_url": "https://fixture.example/q",
		},
		"idempotency_key": "fixture-key-1",
	}
	resp1, body1 := h.do(t, "POST", "/v1/jobs", req)
	resp2, body2 := h.do(t, "POST", "/v1/jobs", req)
	if resp1.StatusCode != http.StatusAccepted || resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("statuses: %d %d", resp1.StatusCode, resp2.StatusCode)
	}
	var a, b struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(body1, &a)
	_ = json.Unmarshal(body2, &b)
	if a.JobID != b.JobID {
		t.Fatalf("idempotency: %q != %q", a.JobID, b.JobID)
	}
}

func TestRoundTrip_InvalidInputs(t *testing.T) {
	h := newHarness(t)
	resp, body := h.do(t, "POST", "/v1/jobs", map[string]any{
		"sku": "triage-watch@v1",
		"inputs": map[string]any{
			"priority_threshold": 5,
		},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing-required: status %d body=%s", resp.StatusCode, body)
	}
}

func TestRoundTrip_SKUsListed(t *testing.T) {
	h := newHarness(t)
	req, _ := http.NewRequest("GET", h.srv.URL+"/v1/skus", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/skus: %d body=%s", resp.StatusCode, body)
	}
	var out struct {
		SKUs []map[string]any `json:"skus"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.SKUs) != 2 {
		t.Fatalf("skus: got %d want 2; body=%s", len(out.SKUs), body)
	}
}
