package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/owera/owera-cloud/api/internal/identity"
)

// setupTestVerifier returns a *ClerkVerifier wired against a freshly-
// minted RSA key + a httptest JWKS endpoint. issuer is the URL of the
// JWKS server; sign tokens with the returned key.
func setupTestVerifier(t *testing.T) (v *ClerkVerifier, key jwk.Key, issuer string) {
	t.Helper()
	// 1. Generate an RSA key pair.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	privKey, err := jwk.FromRaw(rsaKey)
	if err != nil {
		t.Fatalf("jwk.FromRaw priv: %v", err)
	}
	_ = privKey.Set(jwk.KeyIDKey, "test-key-1")
	_ = privKey.Set(jwk.AlgorithmKey, jwa.RS256)

	pubKey, err := jwk.PublicKeyOf(privKey)
	if err != nil {
		t.Fatalf("PublicKeyOf: %v", err)
	}
	set := jwk.NewSet()
	if err := set.AddKey(pubKey); err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	// Bypass the cache + httptest server; production constructor uses
	// jwk.Cache against issuer/.well-known/jwks.json. The static
	// provider matches that contract for unit tests.
	v = newVerifierWithKeys("https://test-issuer.invalid", set)
	return v, privKey, "https://test-issuer.invalid"
}

func signTestToken(t *testing.T, key jwk.Key, issuer string, mutate func(jwt.Token)) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Subject("user_test_alice").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Claim("org_id", "org_test_acme").
		Build()
	if err != nil {
		t.Fatalf("jwt.Build: %v", err)
	}
	if mutate != nil {
		mutate(tok)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key))
	if err != nil {
		t.Fatalf("jwt.Sign: %v", err)
	}
	return string(signed)
}

func TestClerkVerifier_HappyPath(t *testing.T) {
	v, key, issuer := setupTestVerifier(t)
	token := signTestToken(t, key, issuer, nil)

	claims, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "user_test_alice" {
		t.Errorf("Subject: %q", claims.Subject)
	}
	if claims.OrgID != "org_test_acme" {
		t.Errorf("OrgID: %q", claims.OrgID)
	}
}

func TestClerkVerifier_RejectsEmpty(t *testing.T) {
	v, _, _ := setupTestVerifier(t)
	if _, err := v.Verify(context.Background(), ""); !errors.Is(err, ErrInvalidClerkToken) {
		t.Errorf("err = %v, want ErrInvalidClerkToken", err)
	}
}

func TestClerkVerifier_RejectsWrongIssuer(t *testing.T) {
	v, key, _ := setupTestVerifier(t)
	bad := signTestToken(t, key, "https://other-issuer.invalid", nil)
	if _, err := v.Verify(context.Background(), bad); !errors.Is(err, ErrInvalidClerkToken) {
		t.Errorf("err = %v, want ErrInvalidClerkToken", err)
	}
}

func TestClerkVerifier_RejectsExpired(t *testing.T) {
	v, key, issuer := setupTestVerifier(t)
	expired := signTestToken(t, key, issuer, func(tok jwt.Token) {
		_ = tok.Set(jwt.ExpirationKey, time.Now().Add(-time.Hour))
	})
	if _, err := v.Verify(context.Background(), expired); !errors.Is(err, ErrInvalidClerkToken) {
		t.Errorf("err = %v, want ErrInvalidClerkToken", err)
	}
}

func TestClerkVerifier_RejectsMissingOrgID(t *testing.T) {
	v, key, issuer := setupTestVerifier(t)
	noOrg := signTestToken(t, key, issuer, func(tok jwt.Token) {
		_ = tok.Remove("org_id")
	})
	if _, err := v.Verify(context.Background(), noOrg); !errors.Is(err, ErrInvalidClerkToken) {
		t.Errorf("err = %v, want ErrInvalidClerkToken", err)
	} else if !strings.Contains(err.Error(), "org_id") {
		t.Errorf("err message should mention org_id: %v", err)
	}
}

func TestClerkVerifier_RejectsBadSignature(t *testing.T) {
	v, _, issuer := setupTestVerifier(t)
	// Mint a token with a DIFFERENT key — verifier's JWKS doesn't have it.
	otherRaw, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherKey, _ := jwk.FromRaw(otherRaw)
	_ = otherKey.Set(jwk.KeyIDKey, "rogue")
	_ = otherKey.Set(jwk.AlgorithmKey, jwa.RS256)
	bad := signTestToken(t, otherKey, issuer, nil)
	if _, err := v.Verify(context.Background(), bad); !errors.Is(err, ErrInvalidClerkToken) {
		t.Errorf("err = %v, want ErrInvalidClerkToken", err)
	}
}

// TestNewClerkVerifier_RejectsEmptyIssuer covers the constructor's
// input-validation path. Real Clerk-issuer URLs are nontrivial to host
// in CI, so the happy-path production constructor isn't unit-tested
// here; setupTestVerifier exercises the same Verify code path via the
// static provider.
func TestNewClerkVerifier_RejectsEmptyIssuer(t *testing.T) {
	if _, err := NewClerkVerifier(context.Background(), "", nil); err == nil {
		t.Error("expected error on empty issuer")
	}
}

// --- middleware dual-auth tests ---

// stubClerk implements ClerkAuthenticator for the middleware tests.
// Maps token → fixed claims; any other token returns an error.
type stubClerk struct {
	tokens map[string]ClerkClaims
}

func (s *stubClerk) Verify(_ context.Context, token string) (*ClerkClaims, error) {
	c, ok := s.tokens[token]
	if !ok {
		return nil, errors.New("stub: unknown token")
	}
	return &c, nil
}

func TestMiddlewareWithClerk_APIKeyPath(t *testing.T) {
	s, tenID, tok := setup(t)
	clerk := &stubClerk{} // empty — Clerk path should not be tried for owc_ tokens

	called := false
	h := MiddlewareWithClerk(s, clerk, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// API-key flow should populate tenant + user from the key row.
		if got := readTenantID(r); got != tenID {
			t.Errorf("tenant: got %q want %q", got, tenID)
		}
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("downstream handler not invoked")
	}
}

func TestMiddlewareWithClerk_JWTPath(t *testing.T) {
	s, tenID, _ := setup(t)
	ctx := context.Background()

	// Bind tenant + user to Clerk identifiers.
	if err := s.SetClerkOrgID(ctx, tenID, "org_test_acme"); err != nil {
		t.Fatalf("SetClerkOrgID: %v", err)
	}
	users, err := s.ListUsers(ctx, tenID)
	if err != nil || len(users) == 0 {
		t.Fatalf("ListUsers: %v users=%d", err, len(users))
	}
	userID := users[0].ID
	if err := s.SetClerkUserID(ctx, tenID, userID, "user_test_alice"); err != nil {
		t.Fatalf("SetClerkUserID: %v", err)
	}

	clerk := &stubClerk{
		tokens: map[string]ClerkClaims{
			"valid-clerk-jwt": {Subject: "user_test_alice", OrgID: "org_test_acme"},
		},
	}

	h := MiddlewareWithClerk(s, clerk, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := readTenantID(r); got != tenID {
			t.Errorf("tenant: got %q want %q", got, tenID)
		}
		if got := readUserID(r); got != userID {
			t.Errorf("user: got %q want %q", got, userID)
		}
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer valid-clerk-jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMiddlewareWithClerk_JWTPath_UnknownOrg_401(t *testing.T) {
	s, _, _ := setup(t)
	clerk := &stubClerk{tokens: map[string]ClerkClaims{
		"jwt-x": {Subject: "user_x", OrgID: "org_does_not_exist"},
	}}
	h := MiddlewareWithClerk(s, clerk, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream invoked despite 401")
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer jwt-x")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status %d want 401", rec.Code)
	}
}

func TestMiddlewareWithClerk_NilClerk_NonOwcRejected(t *testing.T) {
	s, _, _ := setup(t)
	h := MiddlewareWithClerk(s, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream invoked despite no Clerk auth")
	}))
	req := httptest.NewRequest("GET", "/v1/jobs", nil)
	req.Header.Set("Authorization", "Bearer some-non-owc-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status %d want 401", rec.Code)
	}
	var body authErrorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error != "unauthorized" {
		t.Errorf("error code: %q", body.Error)
	}
}

// --- context-read helpers ---

func readTenantID(r *http.Request) string {
	return identity.TenantID(r.Context())
}

func readUserID(r *http.Request) string {
	return identity.UserID(r.Context())
}
