package billing

import (
	"context"
	"errors"
	"testing"
)

type stubIDLookup struct {
	mapping map[string]string
	err     error
}

func (s *stubIDLookup) StripeCustomerID(_ context.Context, tenantID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.mapping[tenantID], nil
}

func TestIdentityCustomerResolver_RoundTrip(t *testing.T) {
	r, err := NewIdentityCustomerResolver(&stubIDLookup{
		mapping: map[string]string{
			"ten_a": "cus_test_a",
			"ten_b": "cus_test_b",
		},
	})
	if err != nil {
		t.Fatalf("NewIdentityCustomerResolver: %v", err)
	}

	got, err := r.ResolveCustomer(context.Background(), "ten_a", "triage-watch@v1", "tickets_processed")
	if err != nil {
		t.Fatalf("ResolveCustomer: %v", err)
	}
	if got != "cus_test_a" {
		t.Errorf("got %q, want cus_test_a", got)
	}
}

func TestIdentityCustomerResolver_EmptyOnNotOnboarded(t *testing.T) {
	r, _ := NewIdentityCustomerResolver(&stubIDLookup{mapping: map[string]string{}})

	got, err := r.ResolveCustomer(context.Background(), "ten_unknown", "x@v1", "m")
	if err != nil {
		t.Fatalf("expected nil err on unmapped tenant; got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty customer id; got %q", got)
	}
}

func TestIdentityCustomerResolver_PropagatesLookupError(t *testing.T) {
	wantErr := errors.New("identity: db unreachable")
	r, _ := NewIdentityCustomerResolver(&stubIDLookup{err: wantErr})

	if _, err := r.ResolveCustomer(context.Background(), "ten_a", "x@v1", "m"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestIdentityCustomerResolver_EmptyTenantRejected(t *testing.T) {
	r, _ := NewIdentityCustomerResolver(&stubIDLookup{})
	if _, err := r.ResolveCustomer(context.Background(), "", "x@v1", "m"); err == nil {
		t.Fatal("expected error on empty tenant_id")
	}
}

func TestNewIdentityCustomerResolver_NilLookup(t *testing.T) {
	if _, err := NewIdentityCustomerResolver(nil); err == nil {
		t.Fatal("expected error on nil lookup")
	}
}
