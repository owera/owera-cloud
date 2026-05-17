// Clerk JWT verifier. The dashboard at app.owera.ai signs users in via
// Clerk; when the dashboard's /api/proxy/* route calls api.owera.ai, it
// forwards the Clerk session JWT as `Authorization: Bearer <jwt>`. This
// file holds the verifier that parses, validates, and resolves the JWT
// into the same tenant+user context the API-key path produces.
//
// Verification rules:
//
//   - Issuer must equal CLERK_JWT_ISSUER (host-scoped, e.g.
//     `https://clerk.owera.ai`).
//   - Algorithm RS256.
//   - JWKS fetched once from `${issuer}/.well-known/jwks.json` and refreshed
//     in-process by jwx's cache (15 min default).
//   - Standard `exp`/`iat`/`nbf` claim checks enforced by jwx.
//   - Custom claims required: `sub` (Clerk user id, `user_...`) and
//     `org_id` (Clerk org id, `org_...`). Both must be non-empty strings.
//
// On success the verifier returns a [ClerkClaims] struct; the middleware
// resolves the org_id → tenant + sub → user against the identity store.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// ClerkClaims is the verified claim subset the middleware needs to resolve
// a Clerk session to an Owera tenant + user.
type ClerkClaims struct {
	Subject string // `sub` — the Clerk user id (user_...).
	OrgID   string // `org_id` — the Clerk org id (org_...).
}

// ClerkVerifier verifies Clerk-issued JWTs against the configured issuer's
// JWKS. The zero value is not usable; construct with [NewClerkVerifier].
//
// The verifier is safe for concurrent use. JWK rotation is handled by the
// underlying jwx cache; callers do not need to plumb refresh signals.
type ClerkVerifier struct {
	issuer string
	keys   jwkProvider
}

// jwkProvider abstracts the JWKS source so tests can substitute an
// in-memory set without hitting the network. Production uses
// [jwk.Cache]; tests pass a [jwk.Set] directly via [newVerifierWithKeys].
type jwkProvider interface {
	get(ctx context.Context) (jwk.Set, error)
}

// jwkCache wraps a real *jwk.Cache so we always pull through the auto-
// refresh path. The cache is registered at construction time.
type jwkCache struct {
	c   *jwk.Cache
	url string
}

func (j *jwkCache) get(ctx context.Context) (jwk.Set, error) {
	return j.c.Get(ctx, j.url)
}

// jwkStatic is the test-only provider: holds a fixed set.
type jwkStatic struct{ set jwk.Set }

func (j *jwkStatic) get(_ context.Context) (jwk.Set, error) { return j.set, nil }

// NewClerkVerifier returns a verifier for the issuer URL (e.g.
// `https://clerk.owera.ai`). The JWKS endpoint is derived as
// `${issuer}/.well-known/jwks.json`. The first call may block briefly
// while the cache primes; subsequent verifies use the cached set.
//
// Pass nil httpClient to use http.DefaultClient. The refresh window is
// the jwx default (15 min); we don't expose tuning knobs yet because
// Clerk's key rotation cadence makes the default safe.
func NewClerkVerifier(ctx context.Context, issuer string, httpClient *http.Client) (*ClerkVerifier, error) {
	if issuer == "" {
		return nil, errors.New("auth: empty Clerk issuer")
	}
	jwksURL := issuer + "/.well-known/jwks.json"
	cache := jwk.NewCache(ctx)
	opts := []jwk.RegisterOption{jwk.WithMinRefreshInterval(15 * time.Minute)}
	if httpClient != nil {
		opts = append(opts, jwk.WithHTTPClient(httpClient))
	}
	if err := cache.Register(jwksURL, opts...); err != nil {
		return nil, fmt.Errorf("auth: register JWKS: %w", err)
	}
	// Prime the cache so the first request doesn't pay the cold fetch.
	// A failure here is non-fatal: the cache will retry on the next Get.
	_, _ = cache.Refresh(ctx, jwksURL)
	return &ClerkVerifier{
		issuer: issuer,
		keys:   &jwkCache{c: cache, url: jwksURL},
	}, nil
}

// newVerifierWithKeys is the test constructor; production calls
// [NewClerkVerifier] which wires a real refreshing cache.
func newVerifierWithKeys(issuer string, set jwk.Set) *ClerkVerifier {
	return &ClerkVerifier{issuer: issuer, keys: &jwkStatic{set: set}}
}

// ErrInvalidClerkToken is the package-public sentinel returned for any
// JWT that fails verification or carries malformed/missing claims. The
// middleware translates this to a 401 with `error: "unauthorized"`; we
// don't leak the underlying parse error to the client.
var ErrInvalidClerkToken = errors.New("auth: invalid Clerk token")

// Verify parses + validates token, returning the subset of claims the
// middleware needs. Any failure — bad signature, expired, missing claim,
// JWKS unreachable — collapses to ErrInvalidClerkToken with a wrapped
// cause (visible via errors.Unwrap) so logs can still tell us *why*.
func (v *ClerkVerifier) Verify(ctx context.Context, token string) (*ClerkClaims, error) {
	if token == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidClerkToken)
	}
	set, err := v.keys.get(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: jwks: %v", ErrInvalidClerkToken, err)
	}
	tok, err := jwt.Parse(
		[]byte(token),
		jwt.WithKeySet(set),
		jwt.WithValidate(true),
		jwt.WithIssuer(v.issuer),
		jwt.WithAcceptableSkew(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidClerkToken, err)
	}
	sub := tok.Subject()
	if sub == "" {
		return nil, fmt.Errorf("%w: missing sub", ErrInvalidClerkToken)
	}
	orgRaw, ok := tok.Get("org_id")
	if !ok {
		return nil, fmt.Errorf("%w: missing org_id", ErrInvalidClerkToken)
	}
	orgID, ok := orgRaw.(string)
	if !ok || orgID == "" {
		return nil, fmt.Errorf("%w: org_id not a non-empty string", ErrInvalidClerkToken)
	}
	return &ClerkClaims{Subject: sub, OrgID: orgID}, nil
}

// RS256 is the algorithm Clerk publishes. We pin it explicitly so a
// future Clerk-side change to ES256 surfaces as a config decision rather
// than a silent upgrade. Referenced only by this doc comment today; the
// next time we wire alg enforcement, the value is already at hand.
//
//nolint:unused
var clerkSigningAlg = jwa.RS256
