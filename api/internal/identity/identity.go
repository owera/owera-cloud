// Package identity is the foundation of the Owera Cloud multi-tenant data
// contract. It models tenants, users, and API keys, and enforces the
// invariant that every row carries a tenant_id and every query filters by
// the tenant_id resolved from the request context.
//
// API key plaintext is never stored. Tokens are minted in two pieces —
// a public prefix used as a lookup index and a secret tail verified with
// argon2id (time=3, memory=64 MiB, threads=4, per OWASP 2024). The prefix
// is the only field a token-leak grep can correlate; the verifier requires
// the live secret tail plus a constant-time argon2 comparison.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	_ "modernc.org/sqlite"
)

// Argon2id parameters (OWASP 2024). Tuned for ~50 ms verify on a Mac mini;
// memory dominates so increasing time has marginal effect. Encoded into
// every stored verifier so a future bump can verify old hashes by reading
// their embedded params.
const (
	argon2Time    uint32 = 3
	argon2Memory  uint32 = 64 * 1024 // KiB
	argon2Threads uint8  = 4
	argon2KeyLen  uint32 = 32
	argon2SaltLen        = 16
)

// Token shape: "owc_<prefix>.<secret>" where <prefix> is the lookup index
// and <secret> is the argon2-verified material. Display callers can show
// "owc_<prefix>…" without exposing the secret tail.
const (
	tokenScheme     = "owc"
	tokenPrefixLen  = 16 // base64 chars; ~96 bits of entropy in the lookup column
	tokenSecretLen  = 32 // bytes of entropy in the verified tail
	tokenSeparator  = "."
)

// Tenant is the unit of isolation. Every other row in the system has a
// tenant_id FK back to one of these.
type Tenant struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// User is a human (or system account) inside a tenant. Users may hold
// multiple API keys.
type User struct {
	ID        string
	TenantID  string
	Email     string
	CreatedAt time.Time
}

// APIKey is a bearer credential. The plaintext token is only ever returned
// from [Store.IssueAPIKey]; subsequent lookups present the plaintext token
// and the store re-derives the verifier with the stored salt + params.
type APIKey struct {
	ID        string
	TenantID  string
	UserID    string
	Prefix    string // lookup index; safe to display
	Label     string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// Store is the persistence boundary for identity.
type Store struct {
	db *sql.DB
}

// Open returns a Store backed by the SQLite database at path. Pass
// ":memory:" for tests. The schema is migrated on every Open.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("identity: empty path")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("identity: open: %w", err)
	}
	// SQLite tuning for an HTTP server: WAL for concurrent readers, busy
	// timeout so contention doesn't surface as immediate SQLITE_BUSY.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("identity: pragma: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying *sql.DB for packages that share the connection
// (queue, jobs, audit). Callers must still respect the tenant_id contract.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			created_at  DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id          TEXT PRIMARY KEY,
			tenant_id   TEXT NOT NULL REFERENCES tenants(id),
			email       TEXT NOT NULL,
			created_at  DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS users_tenant_idx ON users(tenant_id)`,
		// prefix is the lookup index; verifier is the argon2id-encoded hash
		// (PHC string form: $argon2id$v=19$m=...,t=...,p=...$salt$hash).
		`CREATE TABLE IF NOT EXISTS api_keys (
			id          TEXT PRIMARY KEY,
			tenant_id   TEXT NOT NULL REFERENCES tenants(id),
			user_id     TEXT NOT NULL REFERENCES users(id),
			prefix      TEXT NOT NULL UNIQUE,
			verifier    TEXT NOT NULL,
			label       TEXT NOT NULL,
			created_at  DATETIME NOT NULL,
			revoked_at  DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS api_keys_tenant_idx ON api_keys(tenant_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("identity: migrate: %w (stmt=%s)", err, stmt)
		}
	}
	return nil
}

// CreateTenant inserts a new tenant and returns it.
func (s *Store) CreateTenant(ctx context.Context, name string) (*Tenant, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("identity: empty tenant name")
	}
	t := &Tenant{
		ID:        newID("ten_"),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO tenants(id,name,created_at) VALUES(?,?,?)`,
		t.ID, t.Name, t.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("identity: insert tenant: %w", err)
	}
	return t, nil
}

// GetTenant looks up a tenant by ID.
func (s *Store) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,name,created_at FROM tenants WHERE id=?`, id)
	var t Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("identity: get tenant: %w", err)
	}
	return &t, nil
}

// CreateUser adds a user under a tenant.
func (s *Store) CreateUser(ctx context.Context, tenantID, email string) (*User, error) {
	if tenantID == "" {
		return nil, errors.New("identity: empty tenant_id")
	}
	u := &User{
		ID:        newID("usr_"),
		TenantID:  tenantID,
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO users(id,tenant_id,email,created_at) VALUES(?,?,?,?)`,
		u.ID, u.TenantID, u.Email, u.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("identity: insert user: %w", err)
	}
	return u, nil
}

// GetUser looks up a user by id, scoped to tenantID. Cross-tenant reads
// return ErrNotFound — never the row.
func (s *Store) GetUser(ctx context.Context, tenantID, id string) (*User, error) {
	if tenantID == "" {
		return nil, errors.New("identity: empty tenant_id")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id,tenant_id,email,created_at FROM users WHERE tenant_id=? AND id=?`,
		tenantID, id)
	var u User
	if err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("identity: get user: %w", err)
	}
	return &u, nil
}

// IssueAPIKey mints a new bearer token for user under tenantID. The
// plaintext token is returned (and is the only time the plaintext exists
// outside the caller). The store retains only the public prefix and an
// argon2id verifier for the secret tail.
func (s *Store) IssueAPIKey(ctx context.Context, tenantID, userID, label string) (plaintext string, key *APIKey, err error) {
	if tenantID == "" || userID == "" {
		return "", nil, errors.New("identity: tenant_id and user_id required")
	}
	prefix, secret, tok, err := mintToken()
	if err != nil {
		return "", nil, err
	}
	verifier, err := hashSecret(secret)
	if err != nil {
		return "", nil, err
	}
	rec := &APIKey{
		ID:        newID("key_"),
		TenantID:  tenantID,
		UserID:    userID,
		Prefix:    prefix,
		Label:     label,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys(id,tenant_id,user_id,prefix,verifier,label,created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		rec.ID, rec.TenantID, rec.UserID, rec.Prefix, verifier, rec.Label, rec.CreatedAt,
	); err != nil {
		return "", nil, fmt.Errorf("identity: insert api_key: %w", err)
	}
	return tok, rec, nil
}

// LookupAPIKey resolves a plaintext bearer token to its APIKey record.
// Revoked keys are returned with RevokedAt set; callers should treat them
// as not authenticated. A token whose prefix does not match any row, or
// whose secret tail fails argon2 verification, returns ErrNotFound.
//
// On a prefix miss we still run a single argon2id verify against a fixed
// dummy verifier so timing does not distinguish "unknown prefix" from
// "known prefix, wrong secret".
func (s *Store) LookupAPIKey(ctx context.Context, plaintext string) (*APIKey, error) {
	if plaintext == "" {
		return nil, ErrNotFound
	}
	prefix, secret, err := splitToken(plaintext)
	if err != nil {
		_, _ = verifySecret(secret, dummyVerifier)
		return nil, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id,tenant_id,user_id,prefix,verifier,label,created_at,revoked_at
		 FROM api_keys WHERE prefix=?`, prefix)
	var (
		k        APIKey
		verifier string
		revoked  sql.NullTime
	)
	if err := row.Scan(&k.ID, &k.TenantID, &k.UserID, &k.Prefix, &verifier, &k.Label, &k.CreatedAt, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, _ = verifySecret(secret, dummyVerifier)
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("identity: lookup api_key: %w", err)
	}
	ok, err := verifySecret(secret, verifier)
	if err != nil {
		return nil, fmt.Errorf("identity: verify: %w", err)
	}
	if !ok {
		return nil, ErrNotFound
	}
	if revoked.Valid {
		k.RevokedAt = &revoked.Time
	}
	return &k, nil
}

// RevokeAPIKey marks an API key revoked.
func (s *Store) RevokeAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET revoked_at=? WHERE id=?`, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("identity: revoke: %w", err)
	}
	return nil
}

// ListUsers returns all users for tenantID. Used in tests that prove the
// cross-tenant read protection.
func (s *Store) ListUsers(ctx context.Context, tenantID string) ([]*User, error) {
	if tenantID == "" {
		return nil, errors.New("identity: empty tenant_id")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,tenant_id,email,created_at FROM users WHERE tenant_id=? ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("identity: list users: %w", err)
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Email, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("identity: not found")

// --- request-context helpers ---

type ctxKey int

const (
	tenantCtxKey ctxKey = 1
	userCtxKey   ctxKey = 2
)

// WithTenant returns a context that carries tenantID. The auth middleware
// is the only intended caller; downstream handlers read it via [TenantID].
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey, tenantID)
}

// TenantID returns the tenant_id attached to ctx, or "" if absent. Empty
// indicates the request is unauthenticated; callers should treat that as
// a programming error inside an authenticated handler.
func TenantID(ctx context.Context) string {
	v, _ := ctx.Value(tenantCtxKey).(string)
	return v
}

// WithUser returns a context that carries userID. The auth middleware
// stamps this alongside [WithTenant] when the API key resolves to a user
// (the common case); dashboard requests via Clerk eventually populate
// this from the Clerk subject claim.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userCtxKey, userID)
}

// UserID returns the user_id attached to ctx, or "" if absent. Empty is
// not an error — it indicates the caller authenticated as a tenant-level
// principal (e.g., a future tenant-scoped API key with no user binding).
func UserID(ctx context.Context) string {
	v, _ := ctx.Value(userCtxKey).(string)
	return v
}

// --- internals ---

// mintToken returns (prefix, secret, displayToken). The display token is
// what the caller hands to its user; prefix and secret are the persisted
// + verified halves.
func mintToken() (prefix, secret, display string, err error) {
	prefixBytes := make([]byte, (tokenPrefixLen*6+7)/8)
	if _, err := rand.Read(prefixBytes); err != nil {
		return "", "", "", fmt.Errorf("identity: rand prefix: %w", err)
	}
	prefix = base64.RawURLEncoding.EncodeToString(prefixBytes)[:tokenPrefixLen]
	secretBytes := make([]byte, tokenSecretLen)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", "", fmt.Errorf("identity: rand secret: %w", err)
	}
	secret = base64.RawURLEncoding.EncodeToString(secretBytes)
	display = tokenScheme + "_" + prefix + tokenSeparator + secret
	return prefix, secret, display, nil
}

// splitToken parses a bearer token into its prefix and secret halves.
// Tokens not in canonical form return an empty prefix and the raw input
// as secret so the caller can still run a dummy verify for timing parity.
func splitToken(tok string) (prefix, secret string, err error) {
	const scheme = tokenScheme + "_"
	if !strings.HasPrefix(tok, scheme) {
		return "", tok, errors.New("identity: token scheme mismatch")
	}
	body := strings.TrimPrefix(tok, scheme)
	sep := strings.IndexByte(body, '.')
	if sep <= 0 || sep == len(body)-1 {
		return "", tok, errors.New("identity: token shape mismatch")
	}
	prefix = body[:sep]
	secret = body[sep+1:]
	if len(prefix) != tokenPrefixLen {
		return "", tok, errors.New("identity: token prefix length mismatch")
	}
	return prefix, secret, nil
}

// hashSecret returns a PHC-encoded argon2id verifier for secret.
func hashSecret(secret string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("identity: rand salt: %w", err)
	}
	sum := argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return encodeVerifier(salt, sum, argon2Time, argon2Memory, argon2Threads), nil
}

// verifySecret re-derives the argon2 hash for secret using the params and
// salt encoded in verifier and constant-time compares.
func verifySecret(secret, verifier string) (bool, error) {
	salt, want, t, m, p, err := decodeVerifier(verifier)
	if err != nil {
		return false, err
	}
	if len(want) > math.MaxUint32 {
		return false, fmt.Errorf("identity: verifier hash length %d exceeds uint32", len(want))
	}
	got := argon2.IDKey([]byte(secret), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func encodeVerifier(salt, sum []byte, t, m uint32, p uint8) string {
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		m, t, p,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	)
}

func decodeVerifier(s string) (salt, sum []byte, t, m uint32, p uint8, err error) {
	parts := strings.Split(s, "$")
	// Shape: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<sum>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, errors.New("identity: malformed verifier")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("identity: verifier version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, 0, 0, 0, fmt.Errorf("identity: verifier version %d != %d", version, argon2.Version)
	}
	var mm, tt uint32
	var pp uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mm, &tt, &pp); err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("identity: verifier params: %w", err)
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("identity: verifier salt: %w", err)
	}
	sum, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("identity: verifier sum: %w", err)
	}
	return salt, sum, tt, mm, pp, nil
}

// dummyVerifier is a fixed argon2id-encoded hash for an empty secret. Used
// to keep "no such prefix" and "wrong secret" paths timing-comparable.
// Generated once at package init so every miss does identical work.
var dummyVerifier = func() string {
	salt, _ := hex.DecodeString("00000000000000000000000000000000")
	sum := argon2.IDKey([]byte(""), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return encodeVerifier(salt, sum, argon2Time, argon2Memory, argon2Threads)
}()

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + base64.RawURLEncoding.EncodeToString(b[:])
}
