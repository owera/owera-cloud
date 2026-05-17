package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/identity"
)

// admingHarness wires identity + audit. No tenant-auth machinery needed
// — admin routes bypass it.
type adminHarness struct {
	srv    *httptest.Server
	token  string
	idStore *identity.Store
	audit   *audit.Log
}

func newAdminHarness(t *testing.T) *adminHarness {
	t.Helper()
	id, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = id.Close() })
	al, err := audit.New(id.DB())
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}
	h := New(Deps{Identity: id, Audit: al})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	const adminTok = "admin-test-token"
	t.Setenv(AdminTokenEnv, adminTok)
	return &adminHarness{srv: srv, token: adminTok, idStore: id, audit: al}
}

func (h *adminHarness) do(t *testing.T, method, path, bearer string, body any) (*http.Response, []byte) {
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
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// TestAdmin_CreateTenant — happy path + audit row.
func TestAdmin_CreateTenant(t *testing.T) {
	h := newAdminHarness(t)
	resp, body := h.do(t, "POST", "/v1/admin/tenants", h.token, map[string]any{"name": "Acme Inc"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		TenantID  string `json:"tenant_id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if got.TenantID == "" || got.Name != "Acme Inc" {
		t.Errorf("unexpected response %+v", got)
	}

	// Audit row written?
	entries, err := h.audit.List(context.Background(), got.TenantID, 10)
	if err != nil {
		t.Fatalf("audit.List: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "admin.tenant.create" {
		t.Errorf("audit entries: %+v", entries)
	}
}

// TestAdmin_CreateTenant_MissingName — input validation.
func TestAdmin_CreateTenant_MissingName(t *testing.T) {
	h := newAdminHarness(t)
	resp, _ := h.do(t, "POST", "/v1/admin/tenants", h.token, map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", resp.StatusCode)
	}
}

// TestAdmin_NoToken_401 — missing Authorization.
func TestAdmin_NoToken_401(t *testing.T) {
	h := newAdminHarness(t)
	resp, _ := h.do(t, "POST", "/v1/admin/tenants", "", map[string]any{"name": "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", resp.StatusCode)
	}
}

// TestAdmin_WrongToken_401 — wrong Authorization value.
func TestAdmin_WrongToken_401(t *testing.T) {
	h := newAdminHarness(t)
	resp, _ := h.do(t, "POST", "/v1/admin/tenants", "not-the-token", map[string]any{"name": "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d want 401", resp.StatusCode)
	}
}

// TestAdmin_EnvUnset_503 — admin disabled when env not set.
func TestAdmin_EnvUnset_503(t *testing.T) {
	h := newAdminHarness(t)
	// override the env back to empty
	t.Setenv(AdminTokenEnv, "")
	resp, _ := h.do(t, "POST", "/v1/admin/tenants", "anything", map[string]any{"name": "x"})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503", resp.StatusCode)
	}
}

// TestAdmin_CreateUser_ScopedToTenant — happy path + tenant validation.
func TestAdmin_CreateUser(t *testing.T) {
	h := newAdminHarness(t)
	// First create tenant.
	_, body := h.do(t, "POST", "/v1/admin/tenants", h.token, map[string]any{"name": "Acme"})
	var tres struct{ TenantID string `json:"tenant_id"` }
	_ = json.Unmarshal(body, &tres)

	resp, body := h.do(t, "POST", "/v1/admin/tenants/"+tres.TenantID+"/users", h.token,
		map[string]any{"email": "ops@acme.example"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		UserID   string `json:"user_id"`
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
	}
	_ = json.Unmarshal(body, &got)
	if got.TenantID != tres.TenantID || got.Email != "ops@acme.example" {
		t.Errorf("unexpected response %+v", got)
	}
}

func TestAdmin_CreateUser_UnknownTenant_404(t *testing.T) {
	h := newAdminHarness(t)
	resp, _ := h.do(t, "POST", "/v1/admin/tenants/ten_does_not_exist/users", h.token,
		map[string]any{"email": "x@y.example"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d want 404", resp.StatusCode)
	}
}

// TestAdmin_SetStripeCustomer_RoundTrip — set + verify on GetTenant.
func TestAdmin_SetStripeCustomer(t *testing.T) {
	h := newAdminHarness(t)
	_, body := h.do(t, "POST", "/v1/admin/tenants", h.token, map[string]any{"name": "Acme"})
	var tres struct{ TenantID string `json:"tenant_id"` }
	_ = json.Unmarshal(body, &tres)

	resp, _ := h.do(t, "POST", "/v1/admin/tenants/"+tres.TenantID+"/stripe-customer", h.token,
		map[string]any{"stripe_customer_id": "cus_test_acme"})
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status: got %d want 204", resp.StatusCode)
	}

	te, err := h.idStore.GetTenant(context.Background(), tres.TenantID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if te.StripeCustomerID != "cus_test_acme" {
		t.Errorf("StripeCustomerID: got %q want cus_test_acme", te.StripeCustomerID)
	}
}

func TestAdmin_SetStripeCustomer_UnknownTenant_404(t *testing.T) {
	h := newAdminHarness(t)
	resp, _ := h.do(t, "POST", "/v1/admin/tenants/ten_xyz/stripe-customer", h.token,
		map[string]any{"stripe_customer_id": "cus_x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d want 404", resp.StatusCode)
	}
}

// TestAdmin_SetCap — set + read back via identity.MonthlyCap.
func TestAdmin_SetCap(t *testing.T) {
	h := newAdminHarness(t)
	_, body := h.do(t, "POST", "/v1/admin/tenants", h.token, map[string]any{"name": "Acme"})
	var tres struct{ TenantID string `json:"tenant_id"` }
	_ = json.Unmarshal(body, &tres)

	resp, _ := h.do(t, "POST", "/v1/admin/tenants/"+tres.TenantID+"/cap", h.token,
		map[string]any{"monthly_cap_cents": 50000})
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status: got %d want 204", resp.StatusCode)
	}
	cents, err := h.idStore.MonthlyCap(context.Background(), tres.TenantID)
	if err != nil {
		t.Fatalf("MonthlyCap: %v", err)
	}
	if cents != 50000 {
		t.Errorf("cap: got %d want 50000", cents)
	}
}

// TestAdmin_ListTenants — create three, list returns all with shape.
func TestAdmin_ListTenants(t *testing.T) {
	h := newAdminHarness(t)
	for _, name := range []string{"Acme", "Beta", "Gamma"} {
		h.do(t, "POST", "/v1/admin/tenants", h.token, map[string]any{"name": name})
	}
	resp, body := h.do(t, "GET", "/v1/admin/tenants", h.token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d body=%s", resp.StatusCode, body)
	}
	var got adminListTenantsResp
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, body)
	}
	if len(got.Tenants) != 3 {
		t.Errorf("count: got %d want 3 tenants=%+v", len(got.Tenants), got.Tenants)
	}
}
