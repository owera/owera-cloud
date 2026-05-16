// Package queue is a durable single-process FIFO queue backed by SQLite.
// Enqueued items survive a process restart; consumers atomically claim
// the oldest unclaimed row, then Ack on success or Nack to release.
//
// Idempotency: callers can pass an idempotency key on Enqueue; a second
// enqueue with the same (tenant_id, key) returns the existing item id
// rather than creating a duplicate row.
//
// The queue is single-table and not high-throughput — Owera's customer
// API is bursty but bounded. For very high write rates we'd swap this for
// NATS JetStream or similar; the interface is designed to make that
// substitution local to this package.
package queue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Item is one queued message.
type Item struct {
	ID             string
	TenantID       string
	JobID          string
	Payload        map[string]any
	IdempotencyKey string
	EnqueuedAt     time.Time
	ClaimedAt      *time.Time
	ClaimToken     string
}

// Queue is the durable queue.
type Queue interface {
	Enqueue(ctx context.Context, tenantID, jobID string, payload map[string]any, idempotencyKey string) (*Item, bool, error)
	Dequeue(ctx context.Context, claimToken string, maxAge time.Duration) (*Item, error)
	Ack(ctx context.Context, itemID, claimToken string) error
	Nack(ctx context.Context, itemID, claimToken string) error
	Depth(ctx context.Context) (int, error)
}

// SQLiteQueue is the production implementation.
type SQLiteQueue struct {
	db *sql.DB
}

// NewSQLite returns a queue backed by db. The queue migration is applied.
func NewSQLite(db *sql.DB) (*SQLiteQueue, error) {
	q := &SQLiteQueue{db: db}
	if err := q.migrate(); err != nil {
		return nil, err
	}
	return q, nil
}

func (q *SQLiteQueue) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS queue (
			id              TEXT PRIMARY KEY,
			tenant_id       TEXT NOT NULL,
			job_id          TEXT NOT NULL,
			payload_json    TEXT NOT NULL,
			idempotency_key TEXT,
			enqueued_at     DATETIME NOT NULL,
			claimed_at      DATETIME,
			claim_token     TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS queue_unclaimed_idx ON queue(claimed_at, enqueued_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS queue_idempotency_idx
			ON queue(tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`,
	}
	for _, stmt := range stmts {
		if _, err := q.db.Exec(stmt); err != nil {
			return fmt.Errorf("queue: migrate: %w", err)
		}
	}
	return nil
}

// Enqueue adds a payload to the queue.
func (q *SQLiteQueue) Enqueue(ctx context.Context, tenantID, jobID string, payload map[string]any, idempotencyKey string) (*Item, bool, error) {
	if tenantID == "" || jobID == "" {
		return nil, false, errors.New("queue: tenant_id and job_id required")
	}
	if idempotencyKey != "" {
		existing, err := q.findByKey(ctx, tenantID, idempotencyKey)
		if err == nil {
			return existing, false, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, false, err
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, false, fmt.Errorf("queue: marshal: %w", err)
	}
	now := time.Now().UTC()
	it := &Item{
		ID:             newID("q_"),
		TenantID:       tenantID,
		JobID:          jobID,
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
		EnqueuedAt:     now,
	}
	if _, err := q.db.ExecContext(ctx,
		`INSERT INTO queue(id,tenant_id,job_id,payload_json,idempotency_key,enqueued_at)
		 VALUES(?,?,?,?,?,?)`,
		it.ID, it.TenantID, it.JobID, string(body), nullableString(idempotencyKey), it.EnqueuedAt,
	); err != nil {
		return nil, false, fmt.Errorf("queue: insert: %w", err)
	}
	return it, true, nil
}

// Dequeue atomically claims the oldest unclaimed item, marking it with
// claimToken so a later Ack/Nack can verify the same worker is acting.
// Returns ErrEmpty if there's nothing to claim. maxAge bounds how old a
// previously-claimed-but-not-Ack'd item can be before becoming eligible
// again (claim lease expiry).
func (q *SQLiteQueue) Dequeue(ctx context.Context, claimToken string, maxAge time.Duration) (*Item, error) {
	if claimToken == "" {
		return nil, errors.New("queue: empty claim token")
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("queue: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	expiry := time.Now().UTC().Add(-maxAge)
	row := tx.QueryRowContext(ctx,
		`SELECT id FROM queue
		 WHERE claimed_at IS NULL OR claimed_at < ?
		 ORDER BY enqueued_at LIMIT 1`, expiry)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrEmpty
		}
		return nil, fmt.Errorf("queue: scan: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE queue SET claimed_at=?, claim_token=? WHERE id=?`,
		now, claimToken, id,
	); err != nil {
		return nil, fmt.Errorf("queue: update claim: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("queue: commit: %w", err)
	}

	return q.getByID(ctx, id)
}

// Ack removes a claimed item from the queue.
func (q *SQLiteQueue) Ack(ctx context.Context, itemID, claimToken string) error {
	res, err := q.db.ExecContext(ctx,
		`DELETE FROM queue WHERE id=? AND claim_token=?`, itemID, claimToken)
	if err != nil {
		return fmt.Errorf("queue: ack: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrClaimMismatch
	}
	return nil
}

// Nack releases a claimed item back to the queue.
func (q *SQLiteQueue) Nack(ctx context.Context, itemID, claimToken string) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE queue SET claimed_at=NULL, claim_token=NULL WHERE id=? AND claim_token=?`,
		itemID, claimToken,
	)
	if err != nil {
		return fmt.Errorf("queue: nack: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrClaimMismatch
	}
	return nil
}

// Depth returns the count of items in the queue (claimed or not).
func (q *SQLiteQueue) Depth(ctx context.Context) (int, error) {
	row := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue`)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("queue: depth: %w", err)
	}
	return n, nil
}

// --- errors ---

// ErrEmpty is returned by Dequeue when there's nothing eligible to claim.
var ErrEmpty = errors.New("queue: empty")

// ErrNotFound is returned by lookup helpers that miss.
var ErrNotFound = errors.New("queue: not found")

// ErrClaimMismatch is returned by Ack/Nack when the claim token doesn't match.
var ErrClaimMismatch = errors.New("queue: claim mismatch")

// --- internals ---

func (q *SQLiteQueue) findByKey(ctx context.Context, tenantID, key string) (*Item, error) {
	row := q.db.QueryRowContext(ctx,
		`SELECT id,tenant_id,job_id,payload_json,idempotency_key,enqueued_at,claimed_at,claim_token
		 FROM queue WHERE tenant_id=? AND idempotency_key=?`, tenantID, key)
	return scanItem(row)
}

func (q *SQLiteQueue) getByID(ctx context.Context, id string) (*Item, error) {
	row := q.db.QueryRowContext(ctx,
		`SELECT id,tenant_id,job_id,payload_json,idempotency_key,enqueued_at,claimed_at,claim_token
		 FROM queue WHERE id=?`, id)
	return scanItem(row)
}

type scanner interface {
	Scan(...any) error
}

func scanItem(r scanner) (*Item, error) {
	var (
		it          Item
		payloadJSON string
		idemKey     sql.NullString
		claimedAt   sql.NullTime
		claimToken  sql.NullString
	)
	if err := r.Scan(&it.ID, &it.TenantID, &it.JobID, &payloadJSON, &idemKey, &it.EnqueuedAt, &claimedAt, &claimToken); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("queue: scan: %w", err)
	}
	if payloadJSON != "" {
		if err := json.Unmarshal([]byte(payloadJSON), &it.Payload); err != nil {
			return nil, fmt.Errorf("queue: unmarshal payload: %w", err)
		}
	}
	if idemKey.Valid {
		it.IdempotencyKey = idemKey.String
	}
	if claimedAt.Valid {
		t := claimedAt.Time
		it.ClaimedAt = &t
	}
	if claimToken.Valid {
		it.ClaimToken = claimToken.String
	}
	return &it, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}
