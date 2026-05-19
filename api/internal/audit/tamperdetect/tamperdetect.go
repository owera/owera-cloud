// Package tamperdetect is the daily WORM audit tamper-detection cron for
// the Owera Cloud apiserver. It closes launch gate G4 (audit integrity)
// by periodically verifying that every SQLite audit row has a matching
// object in the S3 WORM bucket, that each object's body still hashes to
// the value SQLite recorded at append time, and that the SQLite rowid
// sequence is gap-free (so a row that was DELETEd to hide an event is
// detected even though the BEFORE-DELETE trigger should have already
// rejected the operation).
//
// The detector runs in-process inside the apiserver as a 24h ticker
// (see cmd/apiserver/main.go). It is intentionally read-only: every
// finding fans out through the existing alerting Router (severity
// Critical). It never tries to "repair" — that is an operator decision
// because a repair could mask a real attack.
//
// The three checks:
//
//   - Completeness: every audit_log row has a corresponding S3 object at
//     the deterministic key {tenant_id, ts-date, hash}. A 404 → alert.
//   - Integrity:    sha256(S3 body) == audit_log.hash. A mismatch → alert.
//   - Continuity:   audit_log rowids form a dense 1..N sequence. A gap
//     means a row was DELETEd; the BEFORE-DELETE trigger
//     should have prevented this, so a gap is evidence the
//     trigger was dropped or the .db was edited offline.
//
// If the WORM streamer is not configured (no S3 bucket env var on the
// apiserver), the detector still runs but limits itself to the
// continuity check. The completeness + integrity checks are no-ops
// because there is nothing on the cold tier to compare against. The
// Check method short-circuits with a clean report in that case so the
// boot fingerprint can still advertise "tamper_detect=on (continuity-
// only)" without burying the operator in noise.
//
// The continuity check is sensitive to rowid gaps. The "no
// rolled-back transactions around audit.Append" invariant that keeps
// the id sequence dense is documented on audit.Log.Append; see that
// godoc before adding any new caller that touches the audit log from
// inside a transaction.
//
// # Memory: bounded streaming scan
//
// Check pages through audit_log in batches of DefaultBatchSize rows
// (1000) using keyset pagination (WHERE id > ? ORDER BY id ASC LIMIT ?).
// Each batch is processed end-to-end (per-row S3 verification + gap
// detection) before the next is fetched, so the row-buffer footprint is
// O(batch_size) regardless of how big the audit_log gets. At 7-year
// retention horizons this is the difference between ~150 KB resident
// and ~150 MB resident.
//
// Tests can override the batch size via WithBatchSize for deterministic
// multi-page coverage on small synthetic datasets.
package tamperdetect

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Severity-Critical alert kinds emitted by the detector. The strings
// are stable; operators wire dashboards + PagerDuty event rules off
// them. Adding a new alert kind is fine; renaming an existing one
// requires a coordinated PagerDuty service-rule update.
const (
	AlertKindMissing   = "audit.tamper.missing_object"
	AlertKindMismatch  = "audit.tamper.hash_mismatch"
	AlertKindRowidGap  = "audit.tamper.rowid_gap"
	AlertKindReadError = "audit.tamper.read_error"
)

// Alerter mirrors billing.Alerter so the apiserver can hand the same
// multiAlerter to both subsystems. We do not import billing because the
// audit package must not depend on it.
type Alerter interface {
	Alert(ctx context.Context, kind string, payload map[string]any) error
}

// EntryReader is the cold-tier read interface. Implementations fetch
// the body of one audit object by its WORM key. A missing object MUST
// be reported as (nil, ErrObjectNotFound) so the detector can
// distinguish "tampered: row deleted from S3" from "transport error".
type EntryReader interface {
	GetEntry(ctx context.Context, key string) ([]byte, error)
}

// ErrObjectNotFound signals "the WORM object does not exist." S3
// implementations should map HTTP 404 to this sentinel; the in-memory
// mock returns it directly.
var ErrObjectNotFound = errors.New("tamperdetect: object not found")

// Report summarises one Check pass. The Findings slice has one entry
// per row that failed at least one check, plus continuity-gap
// pseudo-rows whose RowID is the first missing id. Counters are
// totals across the run for the boot-log one-liner.
type Report struct {
	// StartedAt / FinishedAt bracket the run; FinishedAt − StartedAt
	// is the wall-clock cost of one Check pass (useful for capacity
	// planning when the audit_log gets huge).
	StartedAt  time.Time
	FinishedAt time.Time

	// RowsScanned is the count of audit_log rows examined in this pass.
	// Equal to MAX(id) when continuity is clean.
	RowsScanned int

	// MissingObjects counts rows whose WORM object was a 404.
	MissingObjects int

	// HashMismatches counts rows whose WORM body's sha256 didn't equal
	// audit_log.hash.
	HashMismatches int

	// RowidGaps counts holes in the rowid sequence (each contiguous
	// run of missing ids counts as one gap, anchored at the first
	// missing id).
	RowidGaps int

	// ReadErrors counts transport / unexpected errors reading from S3
	// that were neither 404 nor hash mismatches. These get alerted at
	// the same Critical severity but as audit.tamper.read_error so
	// the operator can distinguish "S3 is down" from "data is bad".
	ReadErrors int

	// Findings is the per-row detail. Cap at MaxFindings to keep one
	// report bounded; the total counters above remain accurate.
	Findings []Finding

	// ColdTierEnabled is false when the detector was built without an
	// EntryReader. Continuity is still checked; completeness +
	// integrity are skipped. Surfaced in Report so callers can log a
	// less alarming summary when S3 isn't wired yet.
	ColdTierEnabled bool
}

// MaxFindings caps Report.Findings so one pathological pass can't pin
// the alerting fan-out. The counters in Report stay accurate even
// when truncation happens.
const MaxFindings = 256

// DefaultBatchSize is the number of audit_log rows the streaming scan
// fetches per SQLite page. Picked to keep the row-buffer footprint
// well under 1 MB even with verbose tenant_ids while still amortising
// the SQL round-trip overhead. Override with WithBatchSize for tests.
const DefaultBatchSize = 1000

// Finding is one mismatched row.
type Finding struct {
	// RowID is the audit_log.id for the offending row. For rowid-gap
	// findings, RowID is the first missing id in the gap.
	RowID int64

	// TenantID is informational — useful for routing the alert in
	// PagerDuty by tenant escalation policy. Empty for gap findings
	// since by definition the row isn't there to read.
	TenantID string

	// Kind is one of the AlertKind* constants. Drives the operator's
	// triage path (404 vs hash mismatch vs gap).
	Kind string

	// Detail is a short human-readable string, e.g. "key=audit/...
	// expected_hash=abc... got=def...". Mirrored into the alert
	// payload so PagerDuty incidents have enough context for the
	// first responder.
	Detail string
}

// Clean reports whether the run found zero anomalies. An "alerter-down"
// situation is not Clean; ReadErrors > 0 also flips Clean to false.
func (r *Report) Clean() bool {
	return r.MissingObjects == 0 && r.HashMismatches == 0 && r.RowidGaps == 0 && r.ReadErrors == 0
}

// Detector is the daily tamper-detection cron. Construct with New, then
// start with Run(ctx). Run blocks until ctx is cancelled.
type Detector struct {
	db        *sql.DB
	reader    EntryReader // nil → continuity-only mode
	alerter   Alerter
	interval  time.Duration
	source    string
	batchSize int

	// keyFn maps an entry's identifying fields to its WORM key. The
	// default mirrors audit.wormKey; injecting a custom one lets tests
	// drive the same package without depending on the audit package's
	// unexported helper. Production wires audit.WormKey via the
	// exported KeyForRow constructor option.
	keyFn func(tenantID string, ts time.Time, hash string) string

	mu        sync.Mutex
	lastClean time.Time // last successful Clean run; exposed via LastClean()
}

// Option mutates Detector construction.
type Option func(*Detector)

// WithKeyFunc overrides the default WORM-key calculator. Production
// passes a closure over audit.WormKey; tests can use their own scheme.
func WithKeyFunc(fn func(tenantID string, ts time.Time, hash string) string) Option {
	return func(d *Detector) { d.keyFn = fn }
}

// WithSource sets the alerting.Alert.Source label propagated to remote
// backends. Defaults to "owera-agentic-api" to match the existing
// drift-reconciler convention.
func WithSource(s string) Option {
	return func(d *Detector) { d.source = s }
}

// WithBatchSize overrides the streaming-scan page size. Values ≤ 0 are
// ignored and the default (DefaultBatchSize) is kept. Tests use small
// values to exercise the multi-page code path on tiny synthetic
// datasets; production keeps the default.
func WithBatchSize(n int) Option {
	return func(d *Detector) {
		if n > 0 {
			d.batchSize = n
		}
	}
}

// New builds a Detector. db is required. reader may be nil — when nil
// the detector runs in continuity-only mode (no S3 GETs). alerter is
// required; pass a no-op implementation for tests if you don't care
// about alert fan-out. interval is the ticker period; values ≤ 0
// disable the loop (Run returns immediately).
func New(db *sql.DB, reader EntryReader, alerter Alerter, interval time.Duration, opts ...Option) (*Detector, error) {
	if db == nil {
		return nil, errors.New("tamperdetect: nil db")
	}
	if alerter == nil {
		return nil, errors.New("tamperdetect: nil alerter")
	}
	d := &Detector{
		db:        db,
		reader:    reader,
		alerter:   alerter,
		interval:  interval,
		source:    "owera-agentic-api",
		batchSize: DefaultBatchSize,
		keyFn:     defaultKey,
	}
	for _, o := range opts {
		o(d)
	}
	return d, nil
}

// defaultKey mirrors audit.wormKey: audit/<tenant_id>/<YYYY-MM-DD>/<hash>.json.
// Kept package-local so tamperdetect doesn't import audit's unexported
// helpers; the apiserver wiring passes audit.WormKey via WithKeyFunc.
func defaultKey(tenantID string, ts time.Time, hash string) string {
	return fmt.Sprintf("audit/%s/%s/%s.json", tenantID, ts.UTC().Format("2006-01-02"), hash)
}

// Interval returns the configured tick interval. Useful for the boot
// fingerprint line.
func (d *Detector) Interval() time.Duration { return d.interval }

// ColdTierEnabled reports whether the detector has an EntryReader.
// Used for the boot fingerprint suffix.
func (d *Detector) ColdTierEnabled() bool { return d.reader != nil }

// LastClean returns the timestamp of the most recent fully-clean run.
// Zero value means the detector has not yet seen a clean run since
// boot (or has never run). Surfaced for /healthz-style probes when we
// build one.
func (d *Detector) LastClean() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastClean
}

// Run blocks until ctx is cancelled, running one Check immediately and
// then on each interval tick. interval ≤ 0 disables the loop entirely
// (Run returns immediately) — that path is for the env-var-disabled
// case so cmd/apiserver/main.go can call Run unconditionally.
func (d *Detector) Run(ctx context.Context) {
	if d.interval <= 0 {
		return
	}
	// Initial pass at boot so we don't have to wait a full interval to
	// surface tamper from before this apiserver started. The drift
	// reconciler does the same.
	d.runOnce(ctx)

	t := time.NewTicker(d.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.runOnce(ctx)
		}
	}
}

// runOnce wraps Check with the boot-log + alerting fan-out so Run
// stays focused on scheduling. Errors from Check itself (the SQLite
// query failed, etc.) are alerted as audit.tamper.read_error; non-
// clean reports fan their findings out per-row.
func (d *Detector) runOnce(ctx context.Context) {
	rep, err := d.Check(ctx)
	if err != nil {
		// A Check-level error means we couldn't even iterate the
		// audit_log — alert and bail. We do NOT propagate; the loop
		// must keep ticking.
		_ = d.alerter.Alert(ctx, AlertKindReadError, map[string]any{
			"error":  err.Error(),
			"source": d.source,
		})
		return
	}
	if rep == nil {
		return
	}
	if rep.Clean() {
		d.mu.Lock()
		d.lastClean = rep.FinishedAt
		d.mu.Unlock()
		return
	}
	for _, f := range rep.Findings {
		payload := map[string]any{
			"row_id":    f.RowID,
			"tenant_id": f.TenantID,
			"detail":    f.Detail,
			"source":    d.source,
		}
		_ = d.alerter.Alert(ctx, f.Kind, payload)
	}
}

// Check performs one tamper-detection pass and returns the report.
//
// The scan is streamed: rows are fetched from audit_log in keyset-
// paginated batches of d.batchSize, and each batch is processed end-
// to-end (continuity-gap detection + per-row S3 verification) before
// the next page is requested. The transient row buffer is O(batchSize),
// not O(total rows), which matters because the WORM retention horizon
// is seven years.
//
// A SQLite-level read error returns (nil, err); per-row anomalies are
// returned in the report and the run is still considered successful at
// the cron level.
func (d *Detector) Check(ctx context.Context) (*Report, error) {
	rep := &Report{
		StartedAt:       time.Now().UTC(),
		ColdTierEnabled: d.reader != nil,
	}
	defer func() {
		rep.FinishedAt = time.Now().UTC()
	}()

	// Streaming state. lastID is the keyset cursor; prevID tracks
	// continuity across page boundaries (so a gap that straddles two
	// pages is still detected). firstRow flips to false after the
	// very first row is seen — until then, prevID's value is moot
	// and gap detection is skipped (the first row's id can legitimately
	// be anything ≥ 1).
	var (
		lastID   int64
		prevID   int64
		firstRow = true
	)

	batchSize := d.batchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	for {
		fetched, newLastID, err := d.scanBatch(ctx, lastID, batchSize, &prevID, &firstRow, rep)
		if err != nil {
			return nil, err
		}
		if fetched == 0 {
			break
		}
		lastID = newLastID
		if fetched < batchSize {
			// Short page → no more rows. Skip the next zero-row
			// round-trip.
			break
		}
	}
	return rep, nil
}

// scanBatch fetches one keyset page of audit_log rows (id > afterID,
// ORDER BY id ASC, LIMIT batchSize) and processes each row inline:
// continuity-gap detection runs against *prevID, and (if cold-tier is
// enabled) per-row S3 verification runs via checkRow. Returns the
// number of rows fetched on this page and the largest id seen (the
// next page's cursor).
//
// Splitting this out of Check keeps the per-row work concentrated in
// checkRow — future PRs that change S3 access (GET→HEAD) or alerter
// fan-out (per-finding → per-pass) can do so without touching the
// pagination loop.
func (d *Detector) scanBatch(
	ctx context.Context,
	afterID int64,
	batchSize int,
	prevID *int64,
	firstRow *bool,
	rep *Report,
) (int, int64, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, tenant_id, ts, hash FROM audit_log
		 WHERE id > ? ORDER BY id ASC LIMIT ?`,
		afterID, batchSize)
	if err != nil {
		return 0, 0, fmt.Errorf("tamperdetect: query audit_log: %w", err)
	}
	defer rows.Close()

	fetched := 0
	var newLastID int64
	for rows.Next() {
		var r rowMeta
		if err := rows.Scan(&r.id, &r.tenantID, &r.ts, &r.hash); err != nil {
			return 0, 0, fmt.Errorf("tamperdetect: scan audit_log: %w", err)
		}
		fetched++
		rep.RowsScanned++
		newLastID = r.id

		// Continuity sweep — runs across page boundaries via the
		// shared prevID pointer. The first row in the entire run
		// is exempt (any starting id is legal); subsequent rows
		// must be exactly prev+1.
		if *firstRow {
			*firstRow = false
		} else if r.id != *prevID+1 {
			rep.RowidGaps++
			appendFinding(rep, Finding{
				RowID:  *prevID + 1,
				Kind:   AlertKindRowidGap,
				Detail: fmt.Sprintf("rowid gap: expected %d, next present is %d", *prevID+1, r.id),
			})
		}
		*prevID = r.id

		// Cold-tier completeness + integrity. checkRow centralises
		// the per-row S3 work so #53 (GET→HEAD) can swap the read
		// strategy without touching the pagination loop.
		if d.reader != nil {
			d.checkRow(ctx, r, rep)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("tamperdetect: iterate audit_log: %w", err)
	}
	return fetched, newLastID, nil
}

// checkRow runs the per-row WORM completeness + integrity checks for
// one audit_log row and appends any finding(s) to rep. Callers must
// have already established d.reader != nil — checkRow does no nil-
// guarding so the cold-tier-disabled path can skip the call entirely
// rather than paying for a function call per row.
//
// This helper is the seam for the two follow-on PRs:
//
//   - #53 will swap the GetEntry call for a HEAD-style integrity check
//     (ETag comparison) on the happy path, falling back to GET only
//     when ETag is unavailable or the hash mismatches.
//   - #54 will change the alerter fan-out from per-finding to per-pass,
//     which only touches runOnce — but keeping the per-row append
//     centralised here (via appendFinding) means #54 doesn't have to
//     reshape this helper either.
func (d *Detector) checkRow(ctx context.Context, r rowMeta, rep *Report) {
	key := d.keyFn(r.tenantID, r.ts, r.hash)
	body, err := d.reader.GetEntry(ctx, key)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			rep.MissingObjects++
			appendFinding(rep, Finding{
				RowID:    r.id,
				TenantID: r.tenantID,
				Kind:     AlertKindMissing,
				Detail:   fmt.Sprintf("key=%s missing from WORM (expected_hash=%s)", key, r.hash),
			})
			return
		}
		rep.ReadErrors++
		appendFinding(rep, Finding{
			RowID:    r.id,
			TenantID: r.tenantID,
			Kind:     AlertKindReadError,
			Detail:   fmt.Sprintf("key=%s read_err=%v", key, err),
		})
		return
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if got != r.hash {
		rep.HashMismatches++
		appendFinding(rep, Finding{
			RowID:    r.id,
			TenantID: r.tenantID,
			Kind:     AlertKindMismatch,
			Detail:   fmt.Sprintf("key=%s expected_hash=%s got=%s", key, r.hash, got),
		})
	}
}

// rowMeta is the row-level slice used by Check + checkRow. Kept
// package-local so the SQL scan path and the per-row helper share one
// concrete type.
type rowMeta struct {
	id       int64
	tenantID string
	ts       time.Time
	hash     string
}

// appendFinding adds f to rep.Findings up to MaxFindings; further
// findings are dropped silently (the counters remain accurate so the
// boot-log + alert fan-out still reflects the true severity).
//
// This is the single append site for Report.Findings. PR #54 will
// intercept here to change per-finding alerter fan-out into a per-pass
// batched alert, so any new code path that wants to record a finding
// MUST go through this helper.
func appendFinding(rep *Report, f Finding) {
	if len(rep.Findings) >= MaxFindings {
		return
	}
	rep.Findings = append(rep.Findings, f)
}
