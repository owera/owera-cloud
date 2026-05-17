// Package tests holds end-to-end attack-path tests. Each test boots the
// API server in-process (no network listener) and exercises a security
// invariant that spans more than one package.
//
// cross_tenant_test.go is the load-bearing isolation contract for WS-15:
// tenant B's API key must never see tenant A's job — and the rejection
// must come back as 404 (not 403, not 401), because leaking existence of
// a resource is itself a security finding.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/billing"
	_ "github.com/owera/owera-cloud/api/internal/catalog" // registers SKUs
	"github.com/owera/owera-cloud/api/internal/dispatcher"
	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
	"github.com/owera/owera-cloud/api/internal/server"
	"github.com/owera/owera-cloud/api/internal/status"
)

// tenant bundles the artefacts a test needs to drive one tenant through
// the API: its id and the plaintext bearer token to put in Authorization.
type tenant struct {
	ID    string
	Token string
}

func bootServer(t *testing.T) (*httptest.Server, *identity.Store) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "owera.db")
	idStore, err := identity.Open(dbPath)
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = idStore.Close() })

	jobsStore, err := jobs.New(idStore.DB())
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	q, err := queue.NewSQLite(idStore.DB())
	if err != nil {
		t.Fatalf("queue.NewSQLite: %v", err)
	}
	auditLog, err := audit.New(idStore.DB())
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}
	billingSvc, err := billing.New(idStore.DB(), &billing.FakeBackend{})
	if err != nil {
		t.Fatalf("billing.New: %v", err)
	}
	transport := dispatcher.NewInMemoryTransport()
	disp := dispatcher.New(transport)
	statusSvc := status.New(transport, 30*time.Second)

	h := server.New(server.Deps{
		Identity:   idStore,
		Jobs:       jobsStore,
		Queue:      q,
		Dispatcher: disp,
		Audit:      auditLog,
		Billing:    billingSvc,
		Status:     statusSvc,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, idStore
}

func provisionTenant(t *testing.T, s *identity.Store, name string) tenant {
	t.Helper()
	ctx := context.Background()
	ten, err := s.CreateTenant(ctx, name)
	if err != nil {
		t.Fatalf("CreateTenant(%s): %v", name, err)
	}
	u, err := s.CreateUser(ctx, ten.ID, "ops@"+name+".example")
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", name, err)
	}
	tok, _, err := s.IssueAPIKey(ctx, ten.ID, u.ID, "primary")
	if err != nil {
		t.Fatalf("IssueAPIKey(%s): %v", name, err)
	}
	return tenant{ID: ten.ID, Token: tok}
}

// TestCrossTenant_JobAccessReturns404 is the load-bearing attack-path
// test for WS-15.
//
// Setup: two tenants A and B, each with an API key. Tenant A submits a
// job. Tenant B then asks for that job by id, presenting its own bearer.
//
// Required outcome: HTTP 404 (not 403). 403 would confirm the job exists,
// leaking information across the tenant boundary. The job body must also
// not appear in the response.
func TestCrossTenant_JobAccessReturns404(t *testing.T) {
	srv, store := bootServer(t)
	tenantA := provisionTenant(t, store, "alpha")
	tenantB := provisionTenant(t, store, "beta")

	// Tenant A submits a job using the smallest live SKU in the catalog.
	jobID := submitJobForTenant(t, srv, tenantA, "triage-watch", map[string]any{
		"queue_url": "https://example.test/alpha",
	})

	// Sanity: tenant A can see their own job.
	if status := getJobStatus(t, srv, tenantA, jobID); status != http.StatusOK {
		t.Fatalf("tenant A self-read: got %d want 200", status)
	}

	// Attack path: tenant B asks for tenant A's job by id.
	resp, body := doRequest(t, srv, http.MethodGet, "/v1/jobs/"+jobID, tenantB.Token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant read: got status %d want 404 (403 would leak existence)\nbody=%s",
			resp.StatusCode, body)
	}
	if bytes.Contains(body, []byte(jobID)) {
		// Even the id echoing back is a leak. The 404 envelope should be
		// generic.
		t.Fatalf("404 body leaks the cross-tenant job id: %s", body)
	}
	// Tenant B's cancel attempt against tenant A's job must also 404.
	cancelResp, cancelBody := doRequest(t, srv, http.MethodPost,
		"/v1/jobs/"+jobID+"/cancel", tenantB.Token, nil)
	if cancelResp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant cancel: got %d want 404\nbody=%s",
			cancelResp.StatusCode, cancelBody)
	}
}

// TestCrossTenant_ListNeverLeaks confirms that listing jobs scoped to one
// tenant never returns rows belonging to another tenant.
func TestCrossTenant_ListNeverLeaks(t *testing.T) {
	srv, store := bootServer(t)
	tenantA := provisionTenant(t, store, "alpha")
	tenantB := provisionTenant(t, store, "beta")

	aJob := submitJobForTenant(t, srv, tenantA, "triage-watch", map[string]any{
		"queue_url": "https://example.test/alpha",
	})
	bJob := submitJobForTenant(t, srv, tenantB, "triage-watch", map[string]any{
		"queue_url": "https://example.test/beta",
	})

	listA := listJobsForTenant(t, srv, tenantA)
	if !containsID(listA, aJob) {
		t.Fatalf("tenant A list missing own job %s", aJob)
	}
	if containsID(listA, bJob) {
		t.Fatalf("tenant A list leaked tenant B job %s", bJob)
	}

	listB := listJobsForTenant(t, srv, tenantB)
	if !containsID(listB, bJob) {
		t.Fatalf("tenant B list missing own job %s", bJob)
	}
	if containsID(listB, aJob) {
		t.Fatalf("tenant B list leaked tenant A job %s", aJob)
	}
}

// TestCrossTenant_AuditLogIsTenantScoped confirms that no audit-log row
// from tenant A appears under tenant B's tenant_id after the cross-tenant
// attack runs. We diff the audit log directly via the store.
func TestCrossTenant_AuditLogIsTenantScoped(t *testing.T) {
	srv, store := bootServer(t)
	tenantA := provisionTenant(t, store, "alpha")
	tenantB := provisionTenant(t, store, "beta")

	jobID := submitJobForTenant(t, srv, tenantA, "triage-watch", map[string]any{
		"queue_url": "https://example.test/alpha",
	})

	// Tenant B pokes at tenant A's job (will 404).
	_, _ = doRequest(t, srv, http.MethodGet, "/v1/jobs/"+jobID, tenantB.Token, nil)

	auditLog, err := audit.New(store.DB())
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}
	bEntries, err := auditLog.List(context.Background(), tenantB.ID, 100)
	if err != nil {
		t.Fatalf("audit.List(B): %v", err)
	}
	for _, e := range bEntries {
		if e.Target == jobID {
			t.Fatalf("audit leak: tenant B has a row pointing at tenant A's job %s", jobID)
		}
		if e.TenantID != tenantB.ID {
			t.Fatalf("audit leak: tenant B query returned tenant_id=%q", e.TenantID)
		}
	}
}

// --- helpers ---

func submitJobForTenant(t *testing.T, srv *httptest.Server, ten tenant, sku string, inputs map[string]any) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"sku":    sku,
		"inputs": inputs,
	})
	resp, raw := doRequest(t, srv, http.MethodPost, "/v1/jobs/", ten.Token, body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("submit job: got %d want 202\nbody=%s", resp.StatusCode, raw)
	}
	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode submit resp: %v (body=%s)", err, raw)
	}
	if out.JobID == "" {
		t.Fatalf("empty job_id in submit resp: %s", raw)
	}
	return out.JobID
}

func getJobStatus(t *testing.T, srv *httptest.Server, ten tenant, jobID string) int {
	t.Helper()
	resp, _ := doRequest(t, srv, http.MethodGet, "/v1/jobs/"+jobID, ten.Token, nil)
	return resp.StatusCode
}

func listJobsForTenant(t *testing.T, srv *httptest.Server, ten tenant) []string {
	t.Helper()
	resp, raw := doRequest(t, srv, http.MethodGet, "/v1/jobs/", ten.Token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list jobs: got %d\nbody=%s", resp.StatusCode, raw)
	}
	var out struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode list resp: %v (body=%s)", err, raw)
	}
	ids := make([]string, 0, len(out.Jobs))
	for _, j := range out.Jobs {
		ids = append(ids, j.ID)
	}
	return ids
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func doRequest(t *testing.T, srv *httptest.Server, method, path, token string, body []byte) (*http.Response, []byte) {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	var req *http.Request
	var err error
	if reqBody != nil {
		req, err = http.NewRequest(method, srv.URL+path, reqBody)
	} else {
		req, err = http.NewRequest(method, srv.URL+path, nil)
	}
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	raw := readAll(t, resp)
	return resp, raw
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes()
}
