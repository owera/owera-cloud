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
	"log"
	"sync"
	"time"
)

// Backend is the billing-provider substitution point. EmitUsage records a
// metered usage line for tenantID against a SKU/meter. idemKey is the
// provider-side idempotency token; the same key + same payload must be
// safe to retry indefinitely.
type Backend interface {
	EmitUsage(ctx context.Context, ev UsageEmit) error
	// EmitOneShot bills a single-fire charge against a Stripe customer's
	// upcoming invoice. Used for "per-job fixed" SKUs (campaign-swarm S/M/L,
	// app-build, research-brief, …) where each unit of work is a billable
	// event in its own right rather than a usage increment against a
	// monthly subscription.
	EmitOneShot(ctx context.Context, ev OneShotEmit) error
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

// OneShotEmit is the payload one EmitOneShot call expects.
//
// PriceID identifies the specific tier (campaign-swarm:S/M/L). The
// catalog lookup that selects S vs M vs L happens upstream of this
// boundary; this layer just emits whatever it's told.
//
// Quantity defaults to 1 if unset; emitting Quantity=N produces a
// single invoice item with line-total = N × PriceID.unit_amount.
//
// Description appears on the customer's invoice line. Keep it human:
// "campaign-swarm M — 'Q3 product launch' (task t_abc)" not "ev_xyz".
type OneShotEmit struct {
	TenantID    string
	SKU         string // e.g. "campaign-swarm@v1"
	PriceID     string // Stripe price id (one_time)
	Quantity    int64
	Description string
	Ts          time.Time
	IdemKey     string // Stripe Idempotency-Key
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

// SKUDispatcher decides how a pending billing event should be emitted —
// metered (usage_records / meter_events) vs per-job-fixed (invoice
// items). The production implementation lives in the catalog package
// (or a thin adapter) and routes by Pricing.Model. Service does not
// import catalog directly; tests stub via this interface.
//
// A nil SKUDispatcher on Service means "treat every event as metered"
// — preserves the pre-PR-21 behaviour for callers that haven't been
// updated.
type SKUDispatcher interface {
	Dispatch(ctx context.Context, p PendingEvent) (DispatchPlan, error)
}

// PendingEvent is the row Reconcile pulled off the outbox, projected
// into the shape SKUDispatcher needs (no DB primary key, no billed_at).
type PendingEvent struct {
	EntryID  string
	TenantID string
	SKU      string
	Meter    string
	Units    int64
	Ts       time.Time
}

// DispatchPlan tells Reconcile which Backend method to call for a
// pending event, with the args that method needs.
//
//	Kind == "metered"      → backend.EmitUsage(UsageEmit{Meter: MeterName, ...})
//	Kind == "oneshot"      → backend.EmitOneShot(OneShotEmit{PriceID, Quantity, Description, ...})
//	Kind == "skip"         → no-op; mark billed without emitting (e.g., free tier)
type DispatchPlan struct {
	Kind        string
	MeterName   string // metered only
	PriceID     string // oneshot only
	Quantity    int64  // oneshot only (0 → 1)
	Description string // oneshot only
}

// DispatchKind enumeration constants for type-safety at callsites.
const (
	DispatchKindMetered = "metered"
	DispatchKindOneShot = "oneshot"
	DispatchKindSkip    = "skip"
)

// Service wires a Backend to a durable outbox table. ledger events come
// in via Record; reconciliation flushes pending rows by calling the
// backend and marking them billed.
type Service struct {
	db         *sql.DB
	backend    Backend
	dispatcher SKUDispatcher // optional; nil → all-metered
}

// SetDispatcher installs the per-SKU dispatch policy. Pass nil to fall
// back to all-metered. Safe to call at any time before Reconcile; not
// safe to call concurrently with Reconcile.
func (s *Service) SetDispatcher(d SKUDispatcher) { s.dispatcher = d }

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
		evt := PendingEvent{
			EntryID:  p.EntryID,
			TenantID: p.TenantID,
			SKU:      p.SKU,
			Meter:    p.Meter,
			Units:    p.Units,
			Ts:       p.Ts,
		}
		plan, err := s.planFor(ctx, evt)
		if err != nil {
			// Skip-and-continue: a single bad row (e.g. legacy meter
			// name with no StripeRef) must not block the rest of the
			// queue. The row stays pending (billed_at = NULL) so it
			// retries on the next tick — operator can fix the
			// StripeRef or delete the row out-of-band.
			log.Printf("billing: dispatch %s: %v (skipping; row remains pending)", p.EntryID, err)
			continue
		}
		switch plan.Kind {
		case DispatchKindSkip:
			// No emit; just mark billed so we don't replay.
		case DispatchKindOneShot:
			emit := OneShotEmit{
				TenantID:    p.TenantID,
				SKU:         p.SKU,
				PriceID:     plan.PriceID,
				Quantity:    plan.Quantity,
				Description: plan.Description,
				Ts:          p.Ts,
				IdemKey:     fmt.Sprintf("oneshot:%s:%s", p.TenantID, p.EntryID),
			}
			if err := s.backend.EmitOneShot(ctx, emit); err != nil {
				log.Printf("billing: emit oneshot %s: %v (skipping; row remains pending)", p.EntryID, err)
				continue
			}
		default: // DispatchKindMetered or any unrecognised value
			meter := plan.MeterName
			if meter == "" {
				meter = p.Meter
			}
			emit := UsageEmit{
				TenantID: p.TenantID,
				SKU:      p.SKU,
				Meter:    meter,
				Units:    p.Units,
				Ts:       p.Ts,
				IdemKey:  fmt.Sprintf("usage:%s:%s", p.TenantID, p.EntryID),
			}
			if err := s.backend.EmitUsage(ctx, emit); err != nil {
				log.Printf("billing: emit usage %s: %v (skipping; row remains pending)", p.EntryID, err)
				continue
			}
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE billing_outbox SET billed_at=? WHERE id=?`,
			time.Now().UTC(), p.ID,
		); err != nil {
			log.Printf("billing: mark billed %s: %v (will retry)", p.EntryID, err)
			continue
		}
		emitted++
	}
	return emitted, nil
}

// planFor returns the DispatchPlan for evt. If no dispatcher is wired
// the default is "metered with the event's own Meter" — preserves the
// behaviour from before SKUDispatcher existed.
func (s *Service) planFor(ctx context.Context, evt PendingEvent) (DispatchPlan, error) {
	if s.dispatcher == nil {
		return DispatchPlan{Kind: DispatchKindMetered, MeterName: evt.Meter}, nil
	}
	return s.dispatcher.Dispatch(ctx, evt)
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
	mu       sync.Mutex
	Records  []FakeRecord  // EmitUsage call log
	OneShots []FakeOneShot // EmitOneShot call log
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

// FakeOneShot captures one EmitOneShot call.
type FakeOneShot struct {
	TenantID    string
	SKU         string
	PriceID     string
	Quantity    int64
	Description string
	IdemKey     string
}

// EmitOneShot records the call. Idempotency dedup mirrors EmitUsage —
// same IdemKey within the fake's lifetime is a no-op.
func (f *FakeBackend) EmitOneShot(_ context.Context, ev OneShotEmit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.OneShots {
		if r.IdemKey == ev.IdemKey && ev.IdemKey != "" {
			return nil
		}
	}
	qty := ev.Quantity
	if qty == 0 {
		qty = 1
	}
	f.OneShots = append(f.OneShots, FakeOneShot{
		TenantID:    ev.TenantID,
		SKU:         ev.SKU,
		PriceID:     ev.PriceID,
		Quantity:    qty,
		Description: ev.Description,
		IdemKey:     ev.IdemKey,
	})
	return nil
}
