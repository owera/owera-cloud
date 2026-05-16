// Package auth implements the API-key bearer-token middleware. The
// middleware extracts `Authorization: Bearer <token>` from each request,
// resolves it to a tenant via the identity store, and injects the tenant_id
// into the request context. Handlers downstream call
// identity.TenantID(r.Context()) to read it.
package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/owera/owera-cloud/api/internal/identity"
)

// Middleware returns an http middleware that authenticates requests using
// the given identity store. The path predicate skipAuth marks endpoints
// (like /healthz) that should bypass authentication entirely.
func Middleware(store *identity.Store, skipAuth func(path string) bool) func(http.Handler) http.Handler {
	if skipAuth == nil {
		skipAuth = func(string) bool { return false }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipAuth(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			tok, err := extractBearer(r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
				return
			}
			key, err := store.LookupAPIKey(r.Context(), tok)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "invalid api key")
				return
			}
			if key.RevokedAt != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "api key revoked")
				return
			}
			ctx := identity.WithTenant(r.Context(), key.TenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ErrMissingAuth is reported when the request has no Authorization header.
var ErrMissingAuth = errors.New("missing Authorization header")

// ErrMalformedAuth is reported when the Authorization header is present but
// not in the expected `Bearer <token>` shape.
var ErrMalformedAuth = errors.New("malformed Authorization header")

// extractBearer pulls the token out of an `Authorization: Bearer <token>`
// header. Returns ErrMissingAuth if absent, ErrMalformedAuth otherwise.
func extractBearer(r *http.Request) (string, error) {
	v := r.Header.Get("Authorization")
	if v == "" {
		return "", ErrMissingAuth
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(v, prefix) {
		return "", ErrMalformedAuth
	}
	tok := strings.TrimSpace(strings.TrimPrefix(v, prefix))
	if tok == "" {
		return "", ErrMalformedAuth
	}
	return tok, nil
}

func writeError(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  code,
		"detail": detail,
	})
}
