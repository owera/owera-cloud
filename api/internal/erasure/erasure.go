// Package erasure implements LGPD Art. 18 (right to erasure) and GDPR
// Art. 17 (right to be forgotten) for customer-plane data.
//
// The user-facing surface is a single endpoint, DELETE /v1/tenants/me/data,
// which:
//
//  1. Verifies the caller is authenticated to the tenant they're erasing.
//  2. Writes an audit row of the request (with PII still un-tokenized;
//     the row itself is what proves the request happened).
//  3. Enqueues an erasure job into the durable queue (shared with the
//     job queue — same SQLite table, queue.Queue interface).
//  4. Returns 202 Accepted + the request id.
//
// A background worker dequeues, performs the actual deletion, and writes
// a completion audit row. The Owera SLA target is **15 working days**
// (LGPD Art. 18 §V; GDPR Art. 12 gives 30 calendar days but LGPD is
// stricter, so we honor the LGPD window).
//
// Queue contract: this package only needs Enqueue + Dequeue + Ack /
// Nack. The interface is intentionally narrower than queue.Queue so a
// future swap of the queue implementation (e.g. NATS JetStream) only
// has to satisfy what erasure uses.
package erasure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
)

// Action names emitted into the audit log. Keep stable — they're queried
// by the compliance dashboard and SOC 2 evidence pulls.
const (
	ActionRequest  = "tenant.data.erasure.requested"
	ActionStart    = "tenant.data.erasure.started"
	ActionComplete = "tenant.data.erasure.completed"
	ActionFail     = "tenant.data.erasure.failed"
)

// Queue is the slice of queue.Queue this package needs. Defining it here
// rather than importing queue.Queue keeps the package decoupled and lets
// the dispatcher worker (WS-14) swap queue.Queue for a non-SQLite
// backend without touching erasure.
type Queue interface {
	Enqueue(ctx context.Context, tenantID, jobID string, payload map[string]any, idempotencyKey string) (queueItem, bool, error)
	Dequeue(ctx context.Context, claimToken string, maxAge time.Duration) (queueItem, error)
	Ack(ctx context.Context, itemID, claimToken string) error
	Nack(ctx context.Context, itemID, claimToken string) error
}

// queueItem is the minimal item shape the worker needs from the queue.
// Mirrors queue.Item's exported fields; the queue adapter below converts.
type queueItem interface {
	GetID() string
	GetTenantID() string
	GetJobID() string
	GetPayload() map[string]any
}

// Purger performs the actual data deletion for one tenant. In production
// this is wired to:
//
//   - the operator-plane purge RPC (deletes ~/.hermes/jobs/<tenant>/ etc),
//   - the api SQLite cache delete (jobs, queue items, identity scopes),
//   - the Stripe customer set-inactive (NOT delete, fiscal hold),
//   - the audit-log PII tokenization (preserves chain integrity).
//
// The interface is single-method to keep tests trivial; the production
// implementation is a struct that fans out to each subsystem.
type Purger interface {
	Purge(ctx context.Context, tenantID, requestID string) (PurgeReport, error)
}

// PurgeReport is the per-tenant deletion record. Persisted verbatim into
// the audit log on the ActionComplete row so the auditor can reconstruct
// what was deleted vs. what was retained under legal-basis carve-outs.
type PurgeReport struct {
	TenantID           string         `json:"tenant_id"`
	RequestID          string         `json:"request_id"`
	StartedAt          time.Time      `json:"started_at"`
	CompletedAt        time.Time      `json:"completed_at"`
	ScopesDeleted      []string       `json:"scopes_deleted"`       // e.g. ["jobs", "queue", "operator_payloads", "vector_store"]
	ScopesRetained     map[string]any `json:"scopes_retained"`      // {"stripe_invoices": "5y_receita_federal", "audit_log": "tokenized"}
	BytesDeleted       int64          `json:"bytes_deleted"`
	HashesBeforeAfter  map[string]any `json:"hashes_before_after"`  // {"jobs_table": {"before": "...", "after": "..."}}
	VerificationStatus string         `json:"verification_status"`  // "pending" | "complete" | "failed"
}

// Service is the request-side of erasure (called by the HTTP handler).
type Service struct {
	db    *sql.DB
	queue Queue
	audit *audit.Log
	// SLA is the LGPD window — 15 working days. Recorded on the request
	// row so the worker can detect SLA-busts before they happen.
	SLA time.Duration
}

// New constructs the service. SLA defaults to the LGPD window.
func New(db *sql.DB, q Queue, a *audit.Log) (*Service, error) {
	if db == nil || q == nil || a == nil {
		return nil, errors.New("erasure: db, queue, and audit are required")
	}
	s := &Service{db: db, queue: q, audit: a, SLA: 15 * 24 * time.Hour}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS erasure_requests (
			id            TEXT PRIMARY KEY,
			tenant_id     TEXT NOT NULL,
			requester_id  TEXT NOT NULL,
			requested_at  DATETIME NOT NULL,
			sla_due_at    DATETIME NOT NULL,
			state         TEXT NOT NULL,
			completed_at  DATETIME,
			report_json   TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS erasure_tenant_state_idx ON erasure_requests(tenant_id, state)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("erasure: migrate: %w", err)
		}
	}
	return nil
}

// Request is the persisted erasure record.
type Request struct {
	ID          string
	TenantID    string
	RequesterID string
	RequestedAt time.Time
	SLADueAt    time.Time
	State       string // "queued" | "in_progress" | "complete" | "failed"
	CompletedAt *time.Time
	Report      *PurgeReport
}

// Submit registers an erasure request, audits it, and enqueues the
// background job. Idempotent by (tenant_id, day) — a duplicate request
// on the same UTC day returns the existing request id rather than
// queueing again.
func (s *Service) Submit(ctx context.Context, tenantID, requesterID, ip, userAgent string) (*Request, error) {
	if tenantID == "" {
		return nil, errors.New("erasure: empty tenant_id")
	}
	now := time.Now().UTC()
	idemKey := fmt.Sprintf("erasure:%s:%s", tenantID, now.Format("2006-01-02"))

	// De-dupe by inspecting the table directly — we don't trust the
	// queue's idempotency alone because the row in erasure_requests is
	// the source of truth for the customer-facing acknowledgement.
	if existing, err := s.findByIdempotency(ctx, idemKey); err == nil && existing != nil {
		return existing, nil
	}

	req := &Request{
		ID:          newRequestID(),
		TenantID:    tenantID,
		RequesterID: requesterID,
		RequestedAt: now,
		SLADueAt:    now.Add(s.SLA),
		State:       "queued",
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO erasure_requests(id, tenant_id, requester_id, requested_at, sla_due_at, state)
		 VALUES(?,?,?,?,?,?)`,
		req.ID, req.TenantID, req.RequesterID, req.RequestedAt, req.SLADueAt, req.State,
	); err != nil {
		return nil, fmt.Errorf("erasure: insert: %w", err)
	}

	// Audit row BEFORE the queue write — if the queue fails we still
	// have proof the request was accepted and the operator backfills.
	if err := s.audit.Append(ctx, audit.Entry{
		TenantID: tenantID, UserID: requesterID,
		Action: ActionRequest, Target: req.ID,
		Ts: now, IP: ip, UserAgent: userAgent,
	}); err != nil {
		return nil, fmt.Errorf("erasure: audit request: %w", err)
	}

	payload := map[string]any{
		"request_id":   req.ID,
		"tenant_id":    tenantID,
		"requester_id": requesterID,
		"sla_due_at":   req.SLADueAt.Format(time.RFC3339),
	}
	if _, _, err := s.queue.Enqueue(ctx, tenantID, req.ID, payload, idemKey); err != nil {
		return nil, fmt.Errorf("erasure: enqueue: %w", err)
	}
	return req, nil
}

// Get returns one request by id, scoped to the tenant. Used by the
// "where is my erasure?" follow-up endpoint.
func (s *Service) Get(ctx context.Context, tenantID, requestID string) (*Request, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, requester_id, requested_at, sla_due_at, state, completed_at, report_json
		 FROM erasure_requests WHERE tenant_id=? AND id=?`,
		tenantID, requestID)
	return scanRequest(row)
}

// ListPending returns all non-terminal requests. Used by the worker on
// startup to claim orphans whose queue row was lost.
func (s *Service) ListPending(ctx context.Context) ([]*Request, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, requester_id, requested_at, sla_due_at, state, completed_at, report_json
		 FROM erasure_requests WHERE state IN ('queued', 'in_progress') ORDER BY requested_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Request
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) findByIdempotency(ctx context.Context, idemKey string) (*Request, error) {
	// idem key format: erasure:<tenant>:<YYYY-MM-DD>. Split on the
	// first/last colons rather than Sscanf which doesn't honor the
	// `:` literal between %s tokens.
	parts := strings.SplitN(idemKey, ":", 3)
	if len(parts) != 3 || parts[0] != "erasure" {
		return nil, fmt.Errorf("erasure: malformed idem key %q", idemKey)
	}
	tenant, day := parts[1], parts[2]
	// Prefix match: requested_at is stored as RFC3339Nano starting
	// with YYYY-MM-DD; modernc.org/sqlite's DATE() can't parse the
	// `T` separator, so we substring-compare instead.
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, requester_id, requested_at, sla_due_at, state, completed_at, report_json
		 FROM erasure_requests
		 WHERE tenant_id=? AND SUBSTR(requested_at, 1, 10) = ?
		 ORDER BY requested_at DESC LIMIT 1`,
		tenant, day)
	return scanRequest(row)
}

// markStarted, markComplete, markFailed are called by the worker.

func (s *Service) markStarted(ctx context.Context, requestID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE erasure_requests SET state='in_progress' WHERE id=?`, requestID)
	return err
}

func (s *Service) markComplete(ctx context.Context, requestID string, report PurgeReport) error {
	b, _ := json.Marshal(report)
	_, err := s.db.ExecContext(ctx,
		`UPDATE erasure_requests SET state='complete', completed_at=?, report_json=? WHERE id=?`,
		report.CompletedAt, string(b), requestID)
	return err
}

func (s *Service) markFailed(ctx context.Context, requestID, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE erasure_requests SET state='failed', report_json=? WHERE id=?`,
		`{"failure":`+jsonString(reason)+`}`, requestID)
	return err
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRequest(s scanner) (*Request, error) {
	var (
		r          Request
		completed  sql.NullTime
		reportJSON sql.NullString
	)
	err := s.Scan(&r.ID, &r.TenantID, &r.RequesterID, &r.RequestedAt, &r.SLADueAt,
		&r.State, &completed, &reportJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if completed.Valid {
		t := completed.Time
		r.CompletedAt = &t
	}
	if reportJSON.Valid && reportJSON.String != "" {
		var p PurgeReport
		if jerr := json.Unmarshal([]byte(reportJSON.String), &p); jerr == nil {
			r.Report = &p
		}
	}
	return &r, nil
}

func newRequestID() string {
	return fmt.Sprintf("ers_%d", time.Now().UTC().UnixNano())
}
