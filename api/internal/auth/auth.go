// Package auth implements the API-key bearer-token middleware. The
// middleware extracts `Authorization: Bearer <token>` from each request,
// resolves it to a tenant via the identity store, and injects the tenant_id
// into the request context. Handlers downstream call
// identity.TenantID(r.Context()) to read it.
//
// Failures are JSON-encoded with the shape:
//
//	{"error": "unauthorized", "message": "...", "request_id": "..."}
//
// and X-Request-Id is echoed on every response so support tickets can be
// correlated to log lines without re-deriving the id client-side.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/owera/owera-cloud/api/internal/identity"
)

// Middleware returns an http middleware that authenticates requests using
// the given identity store. The path predicate skipAuth marks endpoints
// (like /healthz) that should bypass authentication entirely. Every
// request — authed, skipped, or rejected — receives an X-Request-Id and
// has its id available via [RequestID] on the request context.
//
// Identical to [MiddlewareWithClerk] with a nil verifier — kept as a
// thin wrapper so existing callers (and tests built before dual-auth
// landed) compile unchanged.
func Middleware(store *identity.Store, skipAuth func(path string) bool) func(http.Handler) http.Handler {
	return MiddlewareWithClerk(store, nil, skipAuth)
}

// MiddlewareWithClerk returns an http middleware that accepts BOTH the
// API-key bearer-token shape and the Clerk JWT shape. The dispatch rule
// is purely syntactic on the token: tokens that begin with the Owera
// API-key scheme (`owc_`) go through the API-key path; everything else
// is tried against the Clerk verifier when one is configured.
//
// Pass clerk=nil to disable the Clerk path entirely (tokens lacking the
// `owc_` prefix get the same "invalid api key" 401 as before this PR).
// main.go wires a real verifier whenever CLERK_JWT_ISSUER is set in
// the environment; dev mode without that env keeps API-key-only auth.
func MiddlewareWithClerk(store *identity.Store, clerk ClerkAuthenticator, skipAuth func(path string) bool) func(http.Handler) http.Handler {
	if skipAuth == nil {
		skipAuth = func(string) bool { return false }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := ensureRequestID(r)
			w.Header().Set("X-Request-Id", rid)
			r = r.WithContext(withRequestID(r.Context(), rid))

			if skipAuth(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			tok, err := extractBearer(r)
			if err != nil {
				writeAuthError(w, rid, errMessage(err))
				return
			}

			// API-key path: tokens carrying the `owc_` scheme are
			// always tried as Owera API keys; cross-shape fallback
			// would weaken the signal that key revocation matters.
			if strings.HasPrefix(tok, "owc_") {
				key, err := store.LookupAPIKey(r.Context(), tok)
				if err != nil {
					writeAuthError(w, rid, "invalid api key")
					return
				}
				if key.RevokedAt != nil {
					writeAuthError(w, rid, "api key revoked")
					return
				}
				ctx := identity.WithTenant(r.Context(), key.TenantID)
				ctx = identity.WithUser(ctx, key.UserID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Clerk JWT path (dashboard requests).
			if clerk == nil {
				writeAuthError(w, rid, "invalid api key")
				return
			}
			claims, err := clerk.Verify(r.Context(), tok)
			if err != nil {
				writeAuthError(w, rid, "invalid clerk token")
				return
			}
			tenant, err := store.LookupByClerkOrgID(r.Context(), claims.OrgID)
			if err != nil {
				writeAuthError(w, rid, "unknown tenant for clerk org")
				return
			}
			user, err := store.LookupUserByClerkUserID(r.Context(), claims.Subject)
			if err != nil {
				writeAuthError(w, rid, "unknown user for clerk subject")
				return
			}
			ctx := identity.WithTenant(r.Context(), tenant.ID)
			ctx = identity.WithUser(ctx, user.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClerkAuthenticator is the slice of *ClerkVerifier the middleware
// actually consumes. Stated as an interface so tests can stub the
// verifier without spinning up a full JWKS server.
type ClerkAuthenticator interface {
	Verify(ctx context.Context, token string) (*ClerkClaims, error)
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

func errMessage(err error) string {
	switch {
	case errors.Is(err, ErrMissingAuth):
		return "missing Authorization header"
	case errors.Is(err, ErrMalformedAuth):
		return "malformed Authorization header"
	default:
		return "unauthorized"
	}
}

// authErrorBody is the canonical 401 envelope. Kept in lock-step with the
// dashboard's ApiError type (web/lib/types.ts).
type authErrorBody struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeAuthError(w http.ResponseWriter, requestID, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(authErrorBody{
		Error:     "unauthorized",
		Message:   message,
		RequestID: requestID,
	})
}

// --- request-id helpers ---

type ridCtxKey int

const requestIDKey ridCtxKey = 1

// RequestID returns the request id attached to ctx, or "" if absent.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// ensureRequestID returns the incoming X-Request-Id if the client supplied
// one (sanity-trimmed to 64 chars), otherwise mints a fresh one.
func ensureRequestID(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Request-Id")); v != "" {
		if len(v) > 64 {
			v = v[:64]
		}
		return v
	}
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req_unknown"
	}
	return "req_" + base64.RawURLEncoding.EncodeToString(b[:])
}
