// admin.go — operator-side endpoints under /v1/admin/* that close the
// onboarding tooling-debt from compliance/runbooks/onboarding-playbook.md.
// Authed by a single env-var bearer token (OWERA_ADMIN_TOKEN); fails
// closed (503) when the token is unset.
//
// Every state-changing call writes an audit row with Action prefix
// "admin." — operator actions are regulatorily interesting and must be
// reconstructable from the audit log alone.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/identity"
)

// AdminTokenEnv is the environment variable the admin middleware reads
// at request time. Reading per-request (not at startup) lets ops rotate
// the token without restarting the api server.
const AdminTokenEnv = "OWERA_ADMIN_TOKEN"

// adminMiddleware authorises requests carrying `Authorization: Bearer
// <OWERA_ADMIN_TOKEN>`. Fails closed: missing env → 503 "admin disabled".
func adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := os.Getenv(AdminTokenEnv)
		if want == "" {
			writeErr(w, http.StatusServiceUnavailable, "admin_disabled",
				"admin endpoints not configured (OWERA_ADMIN_TOKEN unset)")
			return
		}
		got, err := extractBearerLocal(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", err.Error())
			return
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractBearerLocal is a slim copy of auth.extractBearer that doesn't
// require importing the auth package's request-id machinery — the admin
// path is intentionally decoupled from the tenant-auth flow.
func extractBearerLocal(r *http.Request) (string, error) {
	v := r.Header.Get("Authorization")
	if v == "" {
		return "", errors.New("missing Authorization header")
	}
	const prefix = "Bearer "
	if len(v) < len(prefix) || v[:len(prefix)] != prefix {
		return "", errors.New("malformed Authorization header")
	}
	tok := v[len(prefix):]
	if tok == "" {
		return "", errors.New("empty bearer token")
	}
	return tok, nil
}

// registerAdmin mounts the admin subrouter on r. Called from New().
func registerAdmin(r chi.Router, d Deps) {
	r.Route("/v1/admin", func(r chi.Router) {
		r.Use(adminMiddleware)
		r.Post("/tenants", adminCreateTenant(d))
		r.Get("/tenants", adminListTenants(d))
		r.Post("/tenants/{tenantID}/users", adminCreateUser(d))
		r.Post("/tenants/{tenantID}/stripe-customer", adminSetStripeCustomer(d))
		r.Post("/tenants/{tenantID}/cap", adminSetCap(d))
		r.Post("/tenants/{tenantID}/clerk-org", adminSetClerkOrg(d))
		r.Post("/tenants/{tenantID}/users/{userID}/clerk-user", adminSetClerkUser(d))
	})
}

// --- handlers ---

type adminCreateTenantReq struct {
	Name string `json:"name"`
}

func adminCreateTenant(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminCreateTenantReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.Name == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "name required")
			return
		}
		t, err := d.Identity.CreateTenant(r.Context(), req.Name)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		recordAdminAudit(r, d, t.ID, "admin.tenant.create", t.ID)
		writeJSON(w, http.StatusCreated, map[string]any{
			"tenant_id":  t.ID,
			"name":       t.Name,
			"created_at": t.CreatedAt,
		})
	}
}

type adminListTenantsResp struct {
	Tenants    []adminTenantRow `json:"tenants"`
	NextCursor string           `json:"next_cursor"`
}

type adminTenantRow struct {
	ID               string `json:"tenant_id"`
	Name             string `json:"name"`
	CreatedAt        string `json:"created_at"`
	StripeCustomerID string `json:"stripe_customer_id,omitempty"`
	MonthlyCapCents  int64  `json:"monthly_cap_cents,omitempty"`
}

func adminListTenants(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		cursor := r.URL.Query().Get("cursor")

		rows, err := d.Identity.DB().QueryContext(r.Context(),
			`SELECT id,name,created_at,COALESCE(stripe_customer_id,''),COALESCE(monthly_cap_cents,0)
			 FROM tenants
			 WHERE id > ?
			 ORDER BY id
			 LIMIT ?`, cursor, limit+1)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		defer rows.Close()

		out := []adminTenantRow{}
		for rows.Next() {
			var row adminTenantRow
			var created string
			if err := rows.Scan(&row.ID, &row.Name, &created, &row.StripeCustomerID, &row.MonthlyCapCents); err != nil {
				writeErr(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			row.CreatedAt = created
			out = append(out, row)
		}
		next := ""
		if len(out) > limit {
			next = out[limit-1].ID
			out = out[:limit]
		}
		writeJSON(w, http.StatusOK, adminListTenantsResp{Tenants: out, NextCursor: next})
	}
}

type adminCreateUserReq struct {
	Email string `json:"email"`
}

func adminCreateUser(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		if _, err := d.Identity.GetTenant(r.Context(), tenantID); err != nil {
			writeErr(w, http.StatusNotFound, "tenant_not_found", err.Error())
			return
		}
		var req adminCreateUserReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.Email == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "email required")
			return
		}
		u, err := d.Identity.CreateUser(r.Context(), tenantID, req.Email)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		recordAdminAudit(r, d, tenantID, "admin.tenant.user.create", u.ID)
		writeJSON(w, http.StatusCreated, map[string]any{
			"user_id":    u.ID,
			"tenant_id":  u.TenantID,
			"email":      u.Email,
			"created_at": u.CreatedAt,
		})
	}
}

type adminSetStripeCustomerReq struct {
	StripeCustomerID string `json:"stripe_customer_id"`
}

func adminSetStripeCustomer(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		var req adminSetStripeCustomerReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.StripeCustomerID == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "stripe_customer_id required")
			return
		}
		if err := d.Identity.SetStripeCustomerID(r.Context(), tenantID, req.StripeCustomerID); err != nil {
			if errors.Is(err, identity.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "tenant_not_found", err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		recordAdminAudit(r, d, tenantID, "admin.tenant.set_stripe_customer", req.StripeCustomerID)
		w.WriteHeader(http.StatusNoContent)
	}
}

type adminSetCapReq struct {
	MonthlyCapCents int64 `json:"monthly_cap_cents"`
}

func adminSetCap(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		var req adminSetCapReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if err := d.Identity.SetMonthlyCap(r.Context(), tenantID, req.MonthlyCapCents); err != nil {
			if errors.Is(err, identity.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "tenant_not_found", err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		recordAdminAudit(r, d, tenantID, "admin.tenant.set_cap", strconv.FormatInt(req.MonthlyCapCents, 10))
		w.WriteHeader(http.StatusNoContent)
	}
}

type adminSetClerkOrgReq struct {
	ClerkOrgID string `json:"clerk_org_id"`
}

// adminSetClerkOrg binds a tenant to a Clerk Organisation id (org_...).
// After this call, JWTs from that Clerk org resolve to the tenant via
// the auth middleware's Clerk path.
func adminSetClerkOrg(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		var req adminSetClerkOrgReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.ClerkOrgID == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "clerk_org_id required")
			return
		}
		if err := d.Identity.SetClerkOrgID(r.Context(), tenantID, req.ClerkOrgID); err != nil {
			if errors.Is(err, identity.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "tenant_not_found", err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		recordAdminAudit(r, d, tenantID, "admin.tenant.set_clerk_org", req.ClerkOrgID)
		w.WriteHeader(http.StatusNoContent)
	}
}

type adminSetClerkUserReq struct {
	ClerkUserID string `json:"clerk_user_id"`
}

// adminSetClerkUser binds a user row to a Clerk subject id (user_...).
// After this call, dashboard JWTs with this subject claim resolve to
// the user via the auth middleware's Clerk path.
func adminSetClerkUser(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		userID := chi.URLParam(r, "userID")
		var req adminSetClerkUserReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.ClerkUserID == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "clerk_user_id required")
			return
		}
		if err := d.Identity.SetClerkUserID(r.Context(), tenantID, userID, req.ClerkUserID); err != nil {
			if errors.Is(err, identity.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "user_not_found", err.Error())
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		recordAdminAudit(r, d, tenantID, "admin.tenant.user.set_clerk_user", req.ClerkUserID)
		w.WriteHeader(http.StatusNoContent)
	}
}

// recordAdminAudit best-effort logs an admin action. Failure is silent
// — the action already succeeded; audit gaps surface in reconciliation
// rather than blocking the response.
func recordAdminAudit(r *http.Request, d Deps, tenantID, action, target string) {
	if d.Audit == nil || tenantID == "" {
		return
	}
	_ = d.Audit.Append(context.Background(), audit.FromRequest(r, tenantID, "", action, target))
}
