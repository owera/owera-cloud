package identity

import (
	"context"
	"errors"
	"testing"
)

// TestSetAndGetStripeCustomerID covers the round trip plus the empty
// and not-found edges.
func TestSetAndGetStripeCustomerID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	ten, err := s.CreateTenant(ctx, "Acme")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	// Fresh tenant → empty customer id, no error.
	got, err := s.StripeCustomerID(ctx, ten.ID)
	if err != nil {
		t.Fatalf("StripeCustomerID pre-set: %v", err)
	}
	if got != "" {
		t.Errorf("pre-set: got %q, want empty", got)
	}

	// Set, then read back.
	if err := s.SetStripeCustomerID(ctx, ten.ID, "cus_test_acme_1"); err != nil {
		t.Fatalf("SetStripeCustomerID: %v", err)
	}
	got, err = s.StripeCustomerID(ctx, ten.ID)
	if err != nil {
		t.Fatalf("StripeCustomerID post-set: %v", err)
	}
	if got != "cus_test_acme_1" {
		t.Errorf("post-set: got %q, want cus_test_acme_1", got)
	}

	// GetTenant also returns the value.
	te, err := s.GetTenant(ctx, ten.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if te.StripeCustomerID != "cus_test_acme_1" {
		t.Errorf("GetTenant.StripeCustomerID: got %q, want cus_test_acme_1", te.StripeCustomerID)
	}

	// Overwrite is allowed.
	if err := s.SetStripeCustomerID(ctx, ten.ID, "cus_test_acme_2"); err != nil {
		t.Fatalf("SetStripeCustomerID overwrite: %v", err)
	}
	got, _ = s.StripeCustomerID(ctx, ten.ID)
	if got != "cus_test_acme_2" {
		t.Errorf("overwrite: got %q, want cus_test_acme_2", got)
	}

	// Unknown tenant → ErrNotFound.
	if _, err := s.StripeCustomerID(ctx, "ten_does_not_exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown tenant: err = %v, want ErrNotFound", err)
	}
	if err := s.SetStripeCustomerID(ctx, "ten_does_not_exist", "cus_x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("set on unknown tenant: err = %v, want ErrNotFound", err)
	}

	// Empty args → input-validation error (not ErrNotFound).
	if err := s.SetStripeCustomerID(ctx, "", "cus_x"); err == nil {
		t.Error("expected error on empty tenant_id")
	}
	if err := s.SetStripeCustomerID(ctx, ten.ID, ""); err == nil {
		t.Error("expected error on empty stripe_customer_id")
	}
	if _, err := s.StripeCustomerID(ctx, ""); err == nil {
		t.Error("expected error on empty tenant_id (Get)")
	}
}

// TestMigrate_IdempotentReopen ensures Open() on an existing DB with the
// new column applied doesn't error on the duplicate-column ALTER.
func TestMigrate_IdempotentReopen(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/identity.db"

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	_ = s.Close()
	// Second open must not surface the duplicate-column error.
	s, err = Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	_ = s.Close()
}
