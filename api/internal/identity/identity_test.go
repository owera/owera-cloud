package identity

import (
	"context"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCreateTenantAndUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ten, err := s.CreateTenant(ctx, "Acme Inc")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if ten.ID == "" {
		t.Fatal("expected non-empty tenant id")
	}
	u, err := s.CreateUser(ctx, ten.ID, "ops@acme.example")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.TenantID != ten.ID {
		t.Fatalf("tenant_id mismatch: got %q want %q", u.TenantID, ten.ID)
	}
}

func TestIssueAndLookupAPIKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ten, _ := s.CreateTenant(ctx, "Acme")
	u, _ := s.CreateUser(ctx, ten.ID, "ops@acme.example")
	tok, rec, err := s.IssueAPIKey(ctx, ten.ID, u.ID, "primary")
	if err != nil {
		t.Fatalf("IssueAPIKey: %v", err)
	}
	if tok == "" {
		t.Fatal("expected plaintext token")
	}
	if rec.Prefix == "" {
		t.Fatal("expected display prefix stored")
	}
	// The display token must embed the prefix and must not be the secret
	// itself — leaking the stored row should not yield a working bearer.
	if !strings.Contains(tok, rec.Prefix) {
		t.Fatalf("token %q does not contain prefix %q", tok, rec.Prefix)
	}
	if strings.Contains(tok, "verifier") {
		t.Fatal("token leaks verifier")
	}

	got, err := s.LookupAPIKey(ctx, tok)
	if err != nil {
		t.Fatalf("LookupAPIKey: %v", err)
	}
	if got.TenantID != ten.ID {
		t.Fatalf("looked-up tenant_id mismatch: got %q want %q", got.TenantID, ten.ID)
	}

	if _, err := s.LookupAPIKey(ctx, "bogus-token"); err == nil {
		t.Fatal("expected error for bogus token")
	}
	// A token with the right prefix but wrong secret must also fail.
	tampered := tok[:len(tok)-4] + "xxxx"
	if _, err := s.LookupAPIKey(ctx, tampered); err == nil {
		t.Fatal("expected error for tampered secret tail")
	}
}

// TestVerifierIsArgon2id locks in the on-disk hash format so a future
// "let's swap algorithms" change has to update this test deliberately.
func TestVerifierIsArgon2id(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ten, _ := s.CreateTenant(ctx, "Acme")
	u, _ := s.CreateUser(ctx, ten.ID, "ops@acme.example")
	_, rec, _ := s.IssueAPIKey(ctx, ten.ID, u.ID, "primary")

	var verifier string
	row := s.DB().QueryRowContext(ctx, `SELECT verifier FROM api_keys WHERE id=?`, rec.ID)
	if err := row.Scan(&verifier); err != nil {
		t.Fatalf("scan verifier: %v", err)
	}
	if !strings.HasPrefix(verifier, "$argon2id$v=19$m=") {
		t.Fatalf("verifier not argon2id PHC: %q", verifier)
	}
	if strings.Contains(verifier, "sha256") {
		t.Fatalf("verifier still SHA-256: %q", verifier)
	}
}

func TestRevokeAPIKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ten, _ := s.CreateTenant(ctx, "Acme")
	u, _ := s.CreateUser(ctx, ten.ID, "ops@acme.example")
	tok, rec, _ := s.IssueAPIKey(ctx, ten.ID, u.ID, "primary")

	if err := s.RevokeAPIKey(ctx, rec.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	got, err := s.LookupAPIKey(ctx, tok)
	if err != nil {
		t.Fatalf("LookupAPIKey after revoke: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("expected RevokedAt to be set")
	}
}

// TestCrossTenantReadReturnsNothing is the load-bearing isolation contract
// test: a request scoped to tenant A must not see tenant B's rows even if
// it asks by tenant_id directly. We model this by populating two tenants
// and asserting ListUsers is filtered.
func TestCrossTenantReadReturnsNothing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateTenant(ctx, "TenantA")
	b, _ := s.CreateTenant(ctx, "TenantB")
	_, _ = s.CreateUser(ctx, a.ID, "alice@a.example")
	_, _ = s.CreateUser(ctx, a.ID, "andrea@a.example")
	_, _ = s.CreateUser(ctx, b.ID, "bob@b.example")

	usersA, err := s.ListUsers(ctx, a.ID)
	if err != nil {
		t.Fatalf("ListUsers A: %v", err)
	}
	if len(usersA) != 2 {
		t.Fatalf("tenant A users: got %d want 2", len(usersA))
	}
	for _, u := range usersA {
		if u.TenantID != a.ID {
			t.Fatalf("leak: tenant A query returned row with tenant_id=%q", u.TenantID)
		}
	}

	usersB, err := s.ListUsers(ctx, b.ID)
	if err != nil {
		t.Fatalf("ListUsers B: %v", err)
	}
	if len(usersB) != 1 {
		t.Fatalf("tenant B users: got %d want 1", len(usersB))
	}
	for _, u := range usersB {
		if u.TenantID != b.ID {
			t.Fatalf("leak: tenant B query returned row with tenant_id=%q", u.TenantID)
		}
	}

	// Now simulate a programmatic attempt by tenant A to read tenant B's
	// data by passing tenant A's id while expecting tenant B's row: the
	// store's tenant_id filter must hide it. We do that by listing with
	// tenant A's id and asserting bob is absent.
	for _, u := range usersA {
		if u.Email == "bob@b.example" {
			t.Fatal("leak: tenant A list contained tenant B row")
		}
	}
}

// TestGetUserCrossTenantReturnsNotFound is the per-row analog of the
// list-level cross-tenant test: looking up a user by id while scoped to
// the wrong tenant must return ErrNotFound, not the row.
func TestGetUserCrossTenantReturnsNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, _ := s.CreateTenant(ctx, "TenantA")
	b, _ := s.CreateTenant(ctx, "TenantB")
	ua, _ := s.CreateUser(ctx, a.ID, "alice@a.example")

	// Same-tenant lookup succeeds.
	if got, err := s.GetUser(ctx, a.ID, ua.ID); err != nil || got.ID != ua.ID {
		t.Fatalf("same-tenant GetUser: got=%v err=%v", got, err)
	}
	// Cross-tenant lookup returns ErrNotFound — never the row.
	_, err := s.GetUser(ctx, b.ID, ua.ID)
	if err != ErrNotFound {
		t.Fatalf("cross-tenant GetUser: got err=%v want ErrNotFound", err)
	}
}

func TestTenantContext(t *testing.T) {
	ctx := WithTenant(context.Background(), "ten_abc")
	if got := TenantID(ctx); got != "ten_abc" {
		t.Fatalf("TenantID: got %q want ten_abc", got)
	}
	if got := TenantID(context.Background()); got != "" {
		t.Fatalf("TenantID empty ctx: got %q want \"\"", got)
	}
}
