package server

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/owera/owera-cloud/api/internal/identity"
)

// TestRegisteredRoutes walks the chi router and confirms the set of
// registered (method, path) pairs matches the OpenAPI spec contract. This
// is a guard against orphan routes drifting either direction.
func TestRegisteredRoutes(t *testing.T) {
	idStore, err := identity.Open(":memory:")
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = idStore.Close() })
	h := New(Deps{Identity: idStore})
	r, ok := h.(chi.Router)
	if !ok {
		t.Fatalf("New() did not return a chi.Router")
	}
	var got []string
	walker := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got = append(got, method+" "+route)
		return nil
	}
	if err := chi.Walk(r, walker); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Strings(got)

	want := []string{
		"DELETE /v1/tenants/me/data",
		"GET /healthz",
		"GET /readyz",
		"GET /v1/admin/tenants",
		"GET /v1/jobs/",
		"GET /v1/jobs/{id}",
		"GET /v1/skus",
		"GET /v1/tenants/me/data/erasures/{id}",
		"GET /v1/usage",
		"POST /v1/admin/tenants",
		"POST /v1/admin/tenants/{tenantID}/cap",
		"POST /v1/admin/tenants/{tenantID}/stripe-customer",
		"POST /v1/admin/tenants/{tenantID}/users",
		"POST /v1/billing/portal",
		"POST /v1/jobs/",
		"POST /v1/jobs/{id}/cancel",
	}
	sort.Strings(want)

	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("routes mismatch\n got: %v\nwant: %v", got, want)
	}
}
