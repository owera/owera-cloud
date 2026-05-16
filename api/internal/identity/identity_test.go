package identity

import (
	"context"
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
	if rec.HashHex == "" {
		t.Fatal("expected hash stored")
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

func TestTenantContext(t *testing.T) {
	ctx := WithTenant(context.Background(), "ten_abc")
	if got := TenantID(ctx); got != "ten_abc" {
		t.Fatalf("TenantID: got %q want ten_abc", got)
	}
	if got := TenantID(context.Background()); got != "" {
		t.Fatalf("TenantID empty ctx: got %q want \"\"", got)
	}
}
