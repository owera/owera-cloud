// Package jobs is the lifecycle state machine for customer jobs.
//
// A job moves through these states:
//
//	submitted → queued → running → succeeded | failed | cancelled
//
// `submitted` is the brief moment between request acceptance and queue
// enqueue. `queued` means the dispatcher hasn't picked it up yet. `running`
// means it's been dispatched into the operator plane. The three terminal
// states are persisted with timestamps and (for failed) an error string.
//
// Persistence is SQLite, sharing the connection with the identity store.
// Every row carries tenant_id; every query is filtered by tenant_id from
// the request context.
package jobs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Status is the lifecycle phase of a job.
type Status string

const (
	StatusSubmitted Status = "submitted"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// IsTerminal returns true for the three end states.
func (s Status) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCancelled
}

// Valid returns true if s is one of the six known states.
func (s Status) Valid() bool {
	switch s {
	case StatusSubmitted, StatusQueued, StatusRunning,
		StatusSucceeded, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// allowed returns true if the from→to transition is permitted.
func allowed(from, to Status) bool {
	switch from {
	case StatusSubmitted:
		return to == StatusQueued || to == StatusCancelled || to == StatusFailed
	case StatusQueued:
		return to == StatusRunning || to == StatusCancelled || to == StatusFailed
	case StatusRunning:
		return to == StatusSucceeded || to == StatusFailed || to == StatusCancelled
	}
	return false
}

// Job is the persisted record.
type Job struct {
	ID             string
	TenantID       string
	SKU            string // "name@version"
	Status         Status
	Inputs         map[string]any
	Outputs        map[string]any
	Error          string
	IdempotencyKey string
	OperatorTaskID string // ledger task-id once dispatched
	SubmittedAt    time.Time
	UpdatedAt      time.Time
}

// Store persists jobs. It uses the connection from the identity store so
// SQLite migrations happen alongside.
type Store struct {
	db *sql.DB
}

// New returns a Store using the given DB. Migrations are applied.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS jobs (
			id                TEXT PRIMARY KEY,
			tenant_id         TEXT NOT NULL,
			sku               TEXT NOT NULL,
			status            TEXT NOT NULL,
			inputs_json       TEXT NOT NULL,
			outputs_json      TEXT,
			error             TEXT,
			idempotency_key   TEXT,
			operator_task_id  TEXT,
			submitted_at      DATETIME NOT NULL,
			updated_at        DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS jobs_tenant_status_idx ON jobs(tenant_id, status, submitted_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS jobs_idempotency_idx
			ON jobs(tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("jobs: migrate: %w", err)
		}
	}
	return nil
}

// Submit creates a new job in StatusSubmitted. If idempotencyKey is set
// and a job with the same (tenant_id, idempotency_key) already exists,
// the existing record is returned and ok=false signals "duplicate".
func (s *Store) Submit(ctx context.Context, tenantID, sku string, inputs map[string]any, idempotencyKey string) (job *Job, created bool, err error) {
	if tenantID == "" {
		return nil, false, errors.New("jobs: empty tenant_id")
	}
	if idempotencyKey != "" {
		existing, lookupErr := s.findByIdempotencyKey(ctx, tenantID, idempotencyKey)
		if lookupErr == nil {
			return existing, false, nil
		}
		if !errors.Is(lookupErr, ErrNotFound) {
			return nil, false, lookupErr
		}
	}
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, false, fmt.Errorf("jobs: marshal inputs: %w", err)
	}
	now := time.Now().UTC()
	j := &Job{
		ID:             newJobID(),
		TenantID:       tenantID,
		SKU:            sku,
		Status:         StatusSubmitted,
		Inputs:         inputs,
		IdempotencyKey: idempotencyKey,
		SubmittedAt:    now,
		UpdatedAt:      now,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs(id,tenant_id,sku,status,inputs_json,idempotency_key,submitted_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		j.ID, j.TenantID, j.SKU, string(j.Status), string(inputsJSON),
		nullableString(idempotencyKey), j.SubmittedAt, j.UpdatedAt,
	); err != nil {
		return nil, false, fmt.Errorf("jobs: insert: %w", err)
	}
	return j, true, nil
}

// Get returns the job, scoped to tenantID. Cross-tenant reads return
// ErrNotFound — never the row.
func (s *Store) Get(ctx context.Context, tenantID, id string) (*Job, error) {
	if tenantID == "" {
		return nil, errors.New("jobs: empty tenant_id")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id,tenant_id,sku,status,inputs_json,outputs_json,error,
		        idempotency_key,operator_task_id,submitted_at,updated_at
		 FROM jobs WHERE tenant_id=? AND id=?`, tenantID, id)
	return scanJob(row)
}

// ListParams narrows a list query.
type ListParams struct {
	TenantID string
	Status   Status
	SKU      string
	Limit    int
	Cursor   string // job ID; results return entries strictly older than this
}

// List returns jobs for the tenant, newest first, paginated by submitted_at
// then id (id is the tiebreaker for stable cursoring).
func (s *Store) List(ctx context.Context, p ListParams) (jobs []*Job, nextCursor string, err error) {
	if p.TenantID == "" {
		return nil, "", errors.New("jobs: empty tenant_id")
	}
	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id,tenant_id,sku,status,inputs_json,outputs_json,error,
	             idempotency_key,operator_task_id,submitted_at,updated_at
	      FROM jobs WHERE tenant_id=?`
	args := []any{p.TenantID}
	if p.Status != "" {
		q += " AND status=?"
		args = append(args, string(p.Status))
	}
	if p.SKU != "" {
		q += " AND sku=?"
		args = append(args, p.SKU)
	}
	if p.Cursor != "" {
		q += " AND id < ?"
		args = append(args, p.Cursor)
	}
	q += " ORDER BY submitted_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("jobs: list: %w", err)
	}
	defer rows.Close()
	var out []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(out) > limit {
		nextCursor = out[limit-1].ID
		out = out[:limit]
	}
	return out, nextCursor, nil
}

// Transition moves the job to the given state. Returns ErrInvalidTransition
// if the state change isn't allowed.
func (s *Store) Transition(ctx context.Context, tenantID, id string, to Status, opts ...TransitionOpt) (*Job, error) {
	if !to.Valid() {
		return nil, fmt.Errorf("jobs: invalid target state %q", to)
	}
	o := transitionOpts{}
	for _, fn := range opts {
		fn(&o)
	}
	current, err := s.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !allowed(current.Status, to) {
		return nil, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, to)
	}
	now := time.Now().UTC()
	args := []any{string(to), now}
	q := `UPDATE jobs SET status=?, updated_at=?`
	if o.setOperatorTaskID != "" {
		q += ", operator_task_id=?"
		args = append(args, o.setOperatorTaskID)
	}
	if o.setOutputs != nil {
		outJSON, err := json.Marshal(o.setOutputs)
		if err != nil {
			return nil, fmt.Errorf("jobs: marshal outputs: %w", err)
		}
		q += ", outputs_json=?"
		args = append(args, string(outJSON))
	}
	if o.setError != "" {
		q += ", error=?"
		args = append(args, o.setError)
	}
	q += " WHERE tenant_id=? AND id=? AND status=?"
	args = append(args, tenantID, id, string(current.Status))
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("jobs: update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, errors.New("jobs: transition lost race")
	}
	return s.Get(ctx, tenantID, id)
}

// TransitionOpt configures a transition.
type TransitionOpt func(*transitionOpts)

type transitionOpts struct {
	setOperatorTaskID string
	setOutputs        map[string]any
	setError          string
}

// WithOperatorTaskID records the ledger task-id returned by the operator plane.
func WithOperatorTaskID(id string) TransitionOpt {
	return func(o *transitionOpts) { o.setOperatorTaskID = id }
}

// WithOutputs records terminal outputs.
func WithOutputs(out map[string]any) TransitionOpt {
	return func(o *transitionOpts) { o.setOutputs = out }
}

// WithError records a terminal error message.
func WithError(msg string) TransitionOpt {
	return func(o *transitionOpts) { o.setError = msg }
}

// ErrNotFound is returned when no job matches the (tenant_id, id) lookup.
var ErrNotFound = errors.New("jobs: not found")

// ErrInvalidTransition is returned by Transition for a disallowed state change.
var ErrInvalidTransition = errors.New("jobs: invalid transition")

// --- internals ---

func (s *Store) findByIdempotencyKey(ctx context.Context, tenantID, key string) (*Job, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,tenant_id,sku,status,inputs_json,outputs_json,error,
		        idempotency_key,operator_task_id,submitted_at,updated_at
		 FROM jobs WHERE tenant_id=? AND idempotency_key=?`, tenantID, key)
	return scanJob(row)
}

type scanner interface {
	Scan(...any) error
}

func scanJob(r scanner) (*Job, error) {
	var (
		j            Job
		inputsJSON   string
		outputsJSON  sql.NullString
		errStr       sql.NullString
		idemKey      sql.NullString
		operatorTask sql.NullString
		status       string
	)
	if err := r.Scan(&j.ID, &j.TenantID, &j.SKU, &status, &inputsJSON, &outputsJSON, &errStr, &idemKey, &operatorTask, &j.SubmittedAt, &j.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("jobs: scan: %w", err)
	}
	j.Status = Status(status)
	if err := json.Unmarshal([]byte(inputsJSON), &j.Inputs); err != nil {
		return nil, fmt.Errorf("jobs: unmarshal inputs: %w", err)
	}
	if outputsJSON.Valid && outputsJSON.String != "" {
		if err := json.Unmarshal([]byte(outputsJSON.String), &j.Outputs); err != nil {
			return nil, fmt.Errorf("jobs: unmarshal outputs: %w", err)
		}
	}
	if errStr.Valid {
		j.Error = errStr.String
	}
	if idemKey.Valid {
		j.IdempotencyKey = idemKey.String
	}
	if operatorTask.Valid {
		j.OperatorTaskID = operatorTask.String
	}
	return &j, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func newJobID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "job_" + strings.TrimRight(base64.RawURLEncoding.EncodeToString(b[:]), "=")
}
