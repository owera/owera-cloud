package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/owera/owera-cloud/api/internal/identity"
)

func setup(t *testing.T) (*identity.Store, string, string) {
	t.Helper()
	s, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	ten, _ := s.CreateTenant(ctx, "Acme")
	u, _ := s.CreateUser(ctx, ten.ID, "ops@acme.example")
	tok, _, err := s.IssueAPIKey(ctx, ten.ID, u.ID, "primary")
	if err != nil {
		t.Fatalf("IssueAPIKey: %v", err)
	}
	return s, ten.ID, tok
}

func decodeAuthError(t *testing.T, body []byte) authErrorBody {
	t.Helper()
	var e authErrorBody
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode error body: %v (body=%q)", err, body)
	}
	return e
}

func TestMiddleware_AllowsValidKey(t *testing.T) {
	s, tenID, tok := setup(t)
	var seenTenant, seenReqID string
	h := Middleware(s, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTenant = identity.TenantID(r.Context())
		seenReqID = RequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	if seenTenant != tenID {
		t.Fatalf("tenant: got %q want %q", seenTenant, tenID)
	}
	if seenReqID == "" {
		t.Fatal("request id was not propagated into the handler ctx")
	}
	if rec.Header().Get("X-Request-Id") != seenReqID {
		t.Fatalf("X-Request-Id mismatch: header=%q ctx=%q",
			rec.Header().Get("X-Request-Id"), seenReqID)
	}
}

func TestMiddleware_RejectsMissingHeader(t *testing.T) {
	s, _, _ := setup(t)
	h := Middleware(s, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type: got %q want application/json", got)
	}
	body := decodeAuthError(t, rec.Body.Bytes())
	if body.Error != "unauthorized" {
		t.Fatalf("error code: got %q want unauthorized", body.Error)
	}
	if body.RequestID == "" {
		t.Fatal("request_id missing from error body")
	}
	if !strings.Contains(body.Message, "Authorization") {
		t.Fatalf("message: got %q", body.Message)
	}
}

func TestMiddleware_RejectsMalformedHeader(t *testing.T) {
	s, _, _ := setup(t)
	h := Middleware(s, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Token abc123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
	body := decodeAuthError(t, rec.Body.Bytes())
	if body.Error != "unauthorized" || body.RequestID == "" {
		t.Fatalf("error body: %+v", body)
	}
}

func TestMiddleware_RejectsRevokedKey(t *testing.T) {
	s, _, tok := setup(t)
	rec0, err := s.LookupAPIKey(context.Background(), tok)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if err := s.RevokeAPIKey(context.Background(), rec0.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	h := Middleware(s, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}
	body := decodeAuthError(t, rec.Body.Bytes())
	if body.Message != "api key revoked" {
		t.Fatalf("revoked message: got %q", body.Message)
	}
}

func TestMiddleware_SkipAuthPaths(t *testing.T) {
	s, _, _ := setup(t)
	called := false
	h := Middleware(s, func(p string) bool { return p == "/healthz" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("X-Request-Id missing on skip-auth path")
	}
}

func TestMiddleware_EchoesIncomingRequestID(t *testing.T) {
	s, _, tok := setup(t)
	h := Middleware(s, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Request-Id", "req_supplied_by_caller")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-Id"); got != "req_supplied_by_caller" {
		t.Fatalf("echo: got %q want req_supplied_by_caller", got)
	}
}

// TestMiddleware_StampsUserIDFromAPIKey verifies the middleware injects
// both tenant_id AND user_id into the request context. Pre-fix the
// middleware only stamped tenant_id; audit rows recorded empty UserID
// even though the API key was bound to a user. WS-18's compliance
// trajectory needs user-level attribution.
func TestMiddleware_StampsUserIDFromAPIKey(t *testing.T) {
	s, tenID, tok := setup(t)
	var seenTenant, seenUser string
	h := Middleware(s, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTenant = identity.TenantID(r.Context())
		seenUser = identity.UserID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	if seenTenant != tenID {
		t.Errorf("tenant: got %q want %q", seenTenant, tenID)
	}
	if seenUser == "" {
		t.Fatalf("expected non-empty user_id in context; got %q", seenUser)
	}
	if !strings.HasPrefix(seenUser, "usr_") {
		t.Errorf("user_id shape: got %q want usr_-prefixed", seenUser)
	}
}
