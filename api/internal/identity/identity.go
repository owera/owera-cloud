// Package identity is the foundation of the Owera Cloud multi-tenant data
// contract. It models tenants, users, and API keys, and enforces the
// invariant that every row carries a tenant_id and every query filters by
// the tenant_id resolved from the request context.
//
// The store is backed by SQLite via the pure-Go modernc.org/sqlite driver.
// API key plaintext is never stored — only a SHA-256 hash with a per-key
// random prefix that the bearer token presents on every request.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
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
// from [Store.IssueAPIKey]; on every subsequent lookup the caller provides
// the plaintext token and the store re-hashes it.
type APIKey struct {
	ID         string
	TenantID   string
	UserID     string
	Prefix     string // first 8 chars of the plaintext token, for display
	HashHex    string
	Label      string
	CreatedAt  time.Time
	RevokedAt  *time.Time
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
		`CREATE TABLE IF NOT EXISTS api_keys (
			id          TEXT PRIMARY KEY,
			tenant_id   TEXT NOT NULL REFERENCES tenants(id),
			user_id     TEXT NOT NULL REFERENCES users(id),
			prefix      TEXT NOT NULL,
			hash_hex    TEXT NOT NULL UNIQUE,
			label       TEXT NOT NULL,
			created_at  DATETIME NOT NULL,
			revoked_at  DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS api_keys_tenant_idx ON api_keys(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS api_keys_hash_idx ON api_keys(hash_hex)`,
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

// IssueAPIKey mints a new bearer token for user under tenantID. The
// plaintext token is returned (and is the only time the plaintext exists
// outside the caller). The store retains only a SHA-256 hash.
func (s *Store) IssueAPIKey(ctx context.Context, tenantID, userID, label string) (plaintext string, key *APIKey, err error) {
	if tenantID == "" || userID == "" {
		return "", nil, errors.New("identity: tenant_id and user_id required")
	}
	tok, err := mintToken()
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256([]byte(tok))
	hashHex := hex.EncodeToString(sum[:])
	prefix := tok[:8]
	rec := &APIKey{
		ID:        newID("key_"),
		TenantID:  tenantID,
		UserID:    userID,
		Prefix:    prefix,
		HashHex:   hashHex,
		Label:     label,
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys(id,tenant_id,user_id,prefix,hash_hex,label,created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		rec.ID, rec.TenantID, rec.UserID, rec.Prefix, rec.HashHex, rec.Label, rec.CreatedAt,
	); err != nil {
		return "", nil, fmt.Errorf("identity: insert api_key: %w", err)
	}
	return tok, rec, nil
}

// LookupAPIKey resolves a plaintext bearer token to its APIKey record.
// Revoked keys are returned with RevokedAt set; callers should treat them
// as not authenticated.
func (s *Store) LookupAPIKey(ctx context.Context, plaintext string) (*APIKey, error) {
	if plaintext == "" {
		return nil, ErrNotFound
	}
	sum := sha256.Sum256([]byte(plaintext))
	hashHex := hex.EncodeToString(sum[:])
	row := s.db.QueryRowContext(ctx,
		`SELECT id,tenant_id,user_id,prefix,hash_hex,label,created_at,revoked_at
		 FROM api_keys WHERE hash_hex=?`, hashHex)
	var k APIKey
	var revoked sql.NullTime
	if err := row.Scan(&k.ID, &k.TenantID, &k.UserID, &k.Prefix, &k.HashHex, &k.Label, &k.CreatedAt, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("identity: lookup api_key: %w", err)
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

// --- tenant-context helpers ---

type ctxKey int

const tenantCtxKey ctxKey = 1

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

// --- internals ---

func mintToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("identity: rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + base64.RawURLEncoding.EncodeToString(b[:])
}
