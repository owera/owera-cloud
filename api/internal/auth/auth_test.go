package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestMiddleware_AllowsValidKey(t *testing.T) {
	s, tenID, tok := setup(t)
	var seenTenant string
	h := Middleware(s, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTenant = identity.TenantID(r.Context())
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
}

func TestMiddleware_RejectsRevokedKey(t *testing.T) {
	s, tenID, tok := setup(t)
	_ = tenID
	// Find and revoke the key we just issued. Look it up via the token then revoke.
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
}
