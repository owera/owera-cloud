// Package billing emits usage records to a billing backend (Stripe in
// production, a fake in tests) in response to ledger 'bill' events
// streamed back from the operator plane over the tunnel. It also exposes
// a daily reconciliation hook that flushes any unbilled events.
//
// The substitution point is the [Backend] interface. The Stripe wiring
// lives in stripe_backend.go; the in-memory FakeBackend (used by tests
// and by the apiserver scaffold) lives in this file.
//
// Idempotency is load-bearing: every usage emit carries an
// Idempotency-Key derived from the ledger entry id, so retries —
// whether from the cron, the subscriber, or a manual reconcile — never
// double-charge.
package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Backend is the billing-provider substitution point. EmitUsage records a
// metered usage line for tenantID against a SKU/meter. idemKey is the
// provider-side idempotency token; the same key + same payload must be
// safe to retry indefinitely.
type Backend interface {
	EmitUsage(ctx context.Context, ev UsageEmit) error
}

// UsageEmit is the payload one EmitUsage call expects.
type UsageEmit struct {
	TenantID string
	SKU      string // e.g. "triage-watch@v1"
	Meter    string // e.g. "tickets_processed"
	Units    int64
	Ts       time.Time
	IdemKey  string // Stripe Idempotency-Key header value
}

// LedgerEvent is the minimal projection of an operator-plane ledger entry
// the billing pipeline consumes. It deliberately mirrors the operator
// plane's ledger.Entry but adds tenant_id (which the operator plane will
// stamp on bill events going forward).
//
// EntryID is the stable per-entry identifier — for the operator-plane
// ledger it is the line offset within `<task_id>.jsonl` rendered as
// `<task_id>:<offset>`. It anchors the idempotency key.
type LedgerEvent struct {
	EntryID  string
	TaskID   string
	TenantID string
	SKU      string
	Result   string // expected: "bill"
	Units    int64
	Meter    string // e.g. "tickets_processed"
	Ts       time.Time
}

// Service wires a Backend to a durable outbox table. ledger events come
// in via Record; reconciliation flushes pending rows by calling the
// backend and marking them billed.
type Service struct {
	db      *sql.DB
	backend Backend
}

// New returns a billing service writing to db and emitting via backend.
func New(db *sql.DB, backend Backend) (*Service, error) {
	if db == nil {
		return nil, errors.New("billing: nil db")
	}
	s := &Service{db: db, backend: backend}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS billing_outbox (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			entry_id    TEXT NOT NULL UNIQUE,
			task_id     TEXT NOT NULL,
			tenant_id   TEXT NOT NULL,
			sku         TEXT NOT NULL,
			meter       TEXT NOT NULL,
			units       INTEGER NOT NULL,
			ts          DATETIME NOT NULL,
			billed_at   DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS billing_pending_idx ON billing_outbox(billed_at)`,
		`CREATE INDEX IF NOT EXISTS billing_tenant_period_idx ON billing_outbox(tenant_id, ts)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("billing: migrate: %w", err)
		}
	}
	return nil
}

// Record stores a bill event in the outbox. Events with Result != "bill"
// are ignored. Duplicate entry_id values are deduped by the unique index
// — that is the row-level idempotency guarantee on the ledger side. The
// Stripe-side idempotency guarantee is the IdemKey passed to the backend
// during Reconcile.
func (s *Service) Record(ctx context.Context, ev LedgerEvent) error {
	if ev.Result != "bill" {
		return nil
	}
	if ev.TenantID == "" || ev.SKU == "" || ev.Meter == "" {
		return errors.New("billing: incomplete event")
	}
	if ev.EntryID == "" {
		return errors.New("billing: empty entry_id")
	}
	if ev.Ts.IsZero() {
		ev.Ts = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO billing_outbox(entry_id,task_id,tenant_id,sku,meter,units,ts)
		 VALUES(?,?,?,?,?,?,?)`,
		ev.EntryID, ev.TaskID, ev.TenantID, ev.SKU, ev.Meter, ev.Units, ev.Ts,
	)
	if err != nil {
		return fmt.Errorf("billing: insert: %w", err)
	}
	return nil
}

// Reconcile flushes pending outbox rows to the backend. Each successfully
// emitted row is stamped billed_at. Returns the count emitted.
//
// The Stripe Idempotency-Key for each emit is `usage:{tenant_id}:{entry_id}`,
// stable across retries; if Reconcile is interrupted between the EmitUsage
// success and the billed_at UPDATE, the next pass replays the same key and
// Stripe returns the original record without double-charging.
func (s *Service) Reconcile(ctx context.Context) (int, error) {
	if s.backend == nil {
		return 0, errors.New("billing: no backend")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,entry_id,tenant_id,sku,meter,units,ts
		 FROM billing_outbox WHERE billed_at IS NULL ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("billing: select pending: %w", err)
	}
	type pending struct {
		ID       int64
		EntryID  string
		TenantID string
		SKU      string
		Meter    string
		Units    int64
		Ts       time.Time
	}
	var ps []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.ID, &p.EntryID, &p.TenantID, &p.SKU, &p.Meter, &p.Units, &p.Ts); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ps = append(ps, p)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	emitted := 0
	for _, p := range ps {
		emit := UsageEmit{
			TenantID: p.TenantID,
			SKU:      p.SKU,
			Meter:    p.Meter,
			Units:    p.Units,
			Ts:       p.Ts,
			IdemKey:  fmt.Sprintf("usage:%s:%s", p.TenantID, p.EntryID),
		}
		if err := s.backend.EmitUsage(ctx, emit); err != nil {
			return emitted, fmt.Errorf("billing: emit: %w", err)
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE billing_outbox SET billed_at=? WHERE id=?`,
			time.Now().UTC(), p.ID,
		); err != nil {
			return emitted, fmt.Errorf("billing: mark billed: %w", err)
		}
		emitted++
	}
	return emitted, nil
}

// UsageByTenant returns a sku→units aggregate over the outbox rows
// (pending or already-billed) for a tenant. Used by the /v1/usage endpoint.
func (s *Service) UsageByTenant(ctx context.Context, tenantID string) (map[string]int64, error) {
	if tenantID == "" {
		return nil, errors.New("billing: empty tenant_id")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT sku, SUM(units) FROM billing_outbox WHERE tenant_id=? GROUP BY sku`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("billing: usage: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var sku string
		var units int64
		if err := rows.Scan(&sku, &units); err != nil {
			return nil, err
		}
		out[sku] = units
	}
	return out, rows.Err()
}

// TenantPeriodSum sums units recorded for tenantID for ts within
// [periodStart, periodEnd). Used by the reconciler to compare against
// the same window queried from Stripe.
func (s *Service) TenantPeriodSum(ctx context.Context, tenantID string, periodStart, periodEnd time.Time) (int64, error) {
	if tenantID == "" {
		return 0, errors.New("billing: empty tenant_id")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(units),0) FROM billing_outbox
		 WHERE tenant_id=? AND ts>=? AND ts<?`,
		tenantID, periodStart, periodEnd)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("billing: period sum: %w", err)
	}
	return n, nil
}

// ListTenants returns every distinct tenant_id with rows in the outbox.
// Used by the reconciler to iterate.
func (s *Service) ListTenants(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT tenant_id FROM billing_outbox`)
	if err != nil {
		return nil, fmt.Errorf("billing: list tenants: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FakeBackend is an in-memory Backend for tests.
type FakeBackend struct {
	mu      sync.Mutex
	Records []FakeRecord
}

// FakeRecord captures one EmitUsage call.
type FakeRecord struct {
	TenantID string
	SKU      string
	Meter    string
	Units    int64
	IdemKey  string
}

// EmitUsage records the call. Duplicate IdemKey values are deduped, so the
// fake's behaviour matches Stripe's documented idempotency guarantee.
func (f *FakeBackend) EmitUsage(_ context.Context, ev UsageEmit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.Records {
		if r.IdemKey == ev.IdemKey && ev.IdemKey != "" {
			return nil
		}
	}
	f.Records = append(f.Records, FakeRecord{
		TenantID: ev.TenantID,
		SKU:      ev.SKU,
		Meter:    ev.Meter,
		Units:    ev.Units,
		IdemKey:  ev.IdemKey,
	})
	return nil
}
