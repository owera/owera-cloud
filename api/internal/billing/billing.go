// Package billing emits usage records to a billing backend (Stripe in
// production, a fake in tests) in response to ledger 'bill' events
// streamed back from the operator plane over the tunnel. It also exposes
// a daily reconciliation hook that flushes any unbilled events.
//
// The real Stripe SDK is intentionally NOT a dependency of this scaffold;
// the [Backend] interface is the substitution point. The live wiring
// follows in a separate PR per the v2 plan.
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
// metered usage line for tenantID against a SKU/meter.
type Backend interface {
	EmitUsage(ctx context.Context, tenantID, sku string, units int64) error
}

// LedgerEvent is the minimal projection of an operator-plane ledger entry
// the billing pipeline consumes. It deliberately mirrors the operator
// plane's ledger.Entry but adds tenant_id (which the operator plane will
// stamp on bill events going forward).
type LedgerEvent struct {
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
			task_id     TEXT NOT NULL,
			tenant_id   TEXT NOT NULL,
			sku         TEXT NOT NULL,
			meter       TEXT NOT NULL,
			units       INTEGER NOT NULL,
			ts          DATETIME NOT NULL,
			billed_at   DATETIME,
			UNIQUE(task_id, ts, meter)
		)`,
		`CREATE INDEX IF NOT EXISTS billing_pending_idx ON billing_outbox(billed_at)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("billing: migrate: %w", err)
		}
	}
	return nil
}

// Record stores a bill event in the outbox. Events with Result != "bill"
// are ignored. Duplicate (task_id, ts, meter) tuples are deduped by the
// unique index.
func (s *Service) Record(ctx context.Context, ev LedgerEvent) error {
	if ev.Result != "bill" {
		return nil
	}
	if ev.TenantID == "" || ev.SKU == "" || ev.Meter == "" {
		return errors.New("billing: incomplete event")
	}
	if ev.Ts.IsZero() {
		ev.Ts = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO billing_outbox(task_id,tenant_id,sku,meter,units,ts)
		 VALUES(?,?,?,?,?,?)`,
		ev.TaskID, ev.TenantID, ev.SKU, ev.Meter, ev.Units, ev.Ts,
	)
	if err != nil {
		return fmt.Errorf("billing: insert: %w", err)
	}
	return nil
}

// Reconcile flushes pending outbox rows to the backend. Each successfully
// emitted row is stamped billed_at. Returns the count emitted.
func (s *Service) Reconcile(ctx context.Context) (int, error) {
	if s.backend == nil {
		return 0, errors.New("billing: no backend")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,tenant_id,sku,units FROM billing_outbox WHERE billed_at IS NULL ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("billing: select pending: %w", err)
	}
	type pending struct {
		ID       int64
		TenantID string
		SKU      string
		Units    int64
	}
	var ps []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.ID, &p.TenantID, &p.SKU, &p.Units); err != nil {
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
		if err := s.backend.EmitUsage(ctx, p.TenantID, p.SKU, p.Units); err != nil {
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

// FakeBackend is an in-memory Backend for tests.
type FakeBackend struct {
	mu      sync.Mutex
	Records []FakeRecord
}

// FakeRecord captures one EmitUsage call.
type FakeRecord struct {
	TenantID string
	SKU      string
	Units    int64
}

// EmitUsage records the call.
func (f *FakeBackend) EmitUsage(_ context.Context, tenantID, sku string, units int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Records = append(f.Records, FakeRecord{tenantID, sku, units})
	return nil
}
