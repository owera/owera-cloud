package tamperdetect

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// mockReader is a minimal in-memory EntryReader. Test cases stage the
// canonical bytes per key; GetEntry returns ErrObjectNotFound for any
// key not present unless ForceError is set (which simulates an S3
// outage and fans out to the read_error alert kind).
type mockReader struct {
	mu         sync.Mutex
	objects    map[string][]byte
	forceErr   error
	getCalls   int
	missingKey string // when non-empty, this key alone returns ErrObjectNotFound
}

func newMockReader() *mockReader { return &mockReader{objects: map[string][]byte{}} }

func (m *mockReader) put(key string, body []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), body...)
}

func (m *mockReader) GetEntry(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++
	if m.forceErr != nil {
		return nil, m.forceErr
	}
	if m.missingKey != "" && key == m.missingKey {
		return nil, ErrObjectNotFound
	}
	body, ok := m.objects[key]
	if !ok {
		return nil, ErrObjectNotFound
	}
	return body, nil
}

// recordingAlerter captures (kind, payload) tuples so tests can assert
// which alerts fired.
type recordingAlerter struct {
	mu     sync.Mutex
	events []struct {
		Kind    string
		Payload map[string]any
	}
}

func (r *recordingAlerter) Alert(_ context.Context, kind string, payload map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, struct {
		Kind    string
		Payload map[string]any
	}{kind, payload})
	return nil
}

func (r *recordingAlerter) count(kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// newTestDB returns a fresh in-memory SQLite with the audit_log schema
// used by the audit package. The triggers from audit.migrate() are
// not installed here because the tests need DELETE to simulate the
// "row deleted to hide event" tamper case; the production audit
// package owns trigger installation and that path is tested
// separately in api/internal/audit/audit_test.go.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE audit_log (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id   TEXT NOT NULL,
		user_id     TEXT NOT NULL,
		action      TEXT NOT NULL,
		target      TEXT NOT NULL,
		ts          DATETIME NOT NULL,
		ip          TEXT NOT NULL,
		user_agent  TEXT NOT NULL,
		prev_hash   TEXT NOT NULL DEFAULT '',
		hash        TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

// insertRow inserts one audit_log row and returns its rowid, ts, hash
// so the test can stage the matching WORM object. canonical is what
// the streamer would have PUT; sha256(canonical) is the row's hash.
func insertRow(t *testing.T, db *sql.DB, tenantID string, ts time.Time, payload string) (int64, string, string) {
	t.Helper()
	sum := sha256.Sum256([]byte(payload))
	hash := hex.EncodeToString(sum[:])
	res, err := db.Exec(`INSERT INTO audit_log(tenant_id,user_id,action,target,ts,ip,user_agent,prev_hash,hash)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		tenantID, "usr-1", "job.submit", "job-1", ts, "127.0.0.1", "test/1", "", hash)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	id, _ := res.LastInsertId()
	return id, hash, payload
}

func TestCheck_HappyPath(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		payload := fmt.Sprintf(`{"row":%d}`, i)
		_, hash, body := insertRow(t, db, "fixture-a", ts, payload)
		reader.put(defaultKey("fixture-a", ts, hash), []byte(body))
	}

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.Clean() {
		t.Fatalf("expected clean, got %+v", rep)
	}
	if rep.RowsScanned != 3 {
		t.Fatalf("RowsScanned: got %d want 3", rep.RowsScanned)
	}
	if !rep.ColdTierEnabled {
		t.Fatal("ColdTierEnabled: expected true")
	}
	if got := reader.getCalls; got != 3 {
		t.Fatalf("reader.getCalls: got %d want 3", got)
	}
}

func TestCheck_MissingS3Object(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	// Row 1 + 3 have matching S3 objects; row 2 is "deleted from WORM".
	id1, h1, body1 := insertRow(t, db, "fixture-a", ts, `{"row":1}`)
	id2, _, _ := insertRow(t, db, "fixture-a", ts, `{"row":2}`)
	id3, h3, body3 := insertRow(t, db, "fixture-a", ts, `{"row":3}`)
	reader.put(defaultKey("fixture-a", ts, h1), []byte(body1))
	reader.put(defaultKey("fixture-a", ts, h3), []byte(body3))

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.Clean() {
		t.Fatalf("expected dirty, got %+v", rep)
	}
	if rep.MissingObjects != 1 {
		t.Fatalf("MissingObjects: got %d want 1", rep.MissingObjects)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("Findings: got %d want 1", len(rep.Findings))
	}
	f := rep.Findings[0]
	if f.RowID != id2 {
		t.Fatalf("Finding.RowID: got %d want %d (rows: %d, %d, %d)", f.RowID, id2, id1, id2, id3)
	}
	if f.Kind != AlertKindMissing {
		t.Fatalf("Finding.Kind: got %q want %q", f.Kind, AlertKindMissing)
	}

	// runOnce should fan out exactly one summary alert (PR #54 batched
	// per-finding fan-out into one per-pass alert; AlertKindMissing now
	// appears inside the summary payload's counters, not as a standalone
	// alerter call).
	d.runOnce(context.Background())
	if got := rec.count(AlertKindMissing); got != 0 {
		t.Fatalf("per-kind alerts fired: got %d want 0 (batched into summary)", got)
	}
	if got := rec.count(AlertKindSummary); got != 1 {
		t.Fatalf("summary alerts fired: got %d want 1", got)
	}
}

func TestCheck_HashMismatch(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	id, hash, _ := insertRow(t, db, "fixture-a", ts, `{"row":1}`)
	// Stage a body whose sha256 does NOT match the stored hash. This
	// is the read-only test fixture for "S3 object was rewritten to
	// hide the row's true content" — the BodyHash in SQLite still
	// holds the original hash so we detect the divergence.
	reader.put(defaultKey("fixture-a", ts, hash), []byte(`{"row":1,"injected":"evil"}`))

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.Clean() {
		t.Fatalf("expected dirty, got %+v", rep)
	}
	if rep.HashMismatches != 1 {
		t.Fatalf("HashMismatches: got %d want 1", rep.HashMismatches)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].RowID != id || rep.Findings[0].Kind != AlertKindMismatch {
		t.Fatalf("Findings: got %+v", rep.Findings)
	}
}

func TestCheck_RowidGap(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	// Insert 3 rows then DELETE the middle one to fabricate a gap.
	// The audit package's BEFORE-DELETE trigger would block this in
	// production — the test deliberately runs against a no-trigger
	// schema to assert that even if an attacker bypasses the trigger,
	// the gap detector still fires.
	_, h1, body1 := insertRow(t, db, "fixture-a", ts, `{"row":1}`)
	_, _, _ = insertRow(t, db, "fixture-a", ts, `{"row":2}`)
	_, h3, body3 := insertRow(t, db, "fixture-a", ts, `{"row":3}`)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE id = 2`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Stage S3 objects for the surviving rows so completeness +
	// integrity stay clean; only the gap should trip.
	reader.put(defaultKey("fixture-a", ts, h1), []byte(body1))
	reader.put(defaultKey("fixture-a", ts, h3), []byte(body3))

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.Clean() {
		t.Fatalf("expected dirty, got %+v", rep)
	}
	if rep.RowidGaps != 1 {
		t.Fatalf("RowidGaps: got %d want 1", rep.RowidGaps)
	}
	if rep.MissingObjects != 0 || rep.HashMismatches != 0 {
		t.Fatalf("expected only gap, got missing=%d mismatch=%d", rep.MissingObjects, rep.HashMismatches)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != AlertKindRowidGap || rep.Findings[0].RowID != 2 {
		t.Fatalf("Findings: got %+v", rep.Findings)
	}
}

func TestCheck_S3Disabled_NilReader(t *testing.T) {
	db := newTestDB(t)
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	insertRow(t, db, "fixture-a", ts, `{"row":1}`)
	insertRow(t, db, "fixture-a", ts, `{"row":2}`)

	d, err := New(db, nil, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.Clean() {
		t.Fatalf("expected clean (continuity-only), got %+v", rep)
	}
	if rep.ColdTierEnabled {
		t.Fatal("ColdTierEnabled: expected false when reader is nil")
	}
	if rep.RowsScanned != 2 {
		t.Fatalf("RowsScanned: got %d want 2", rep.RowsScanned)
	}
}

func TestCheck_ReadError_FoldedIntoSummary(t *testing.T) {
	// Per-row S3 transport errors used to fire AlertKindReadError as
	// standalone alerter calls (one per row). PR #54 folded those into
	// the per-pass summary: the read_errors counter is set, the
	// summary alert fires once, and AlertKindReadError as a standalone
	// kind is now reserved for the "Check itself failed" path (see
	// TestRunOnce_ScanLevelError_FiresReadErrorAlert).
	db := newTestDB(t)
	reader := newMockReader()
	reader.forceErr = errors.New("simulated s3 503")
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	insertRow(t, db, "fixture-a", ts, `{"row":1}`)

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.runOnce(context.Background())

	if got := rec.count(AlertKindReadError); got != 0 {
		t.Fatalf("standalone read_error alerts: got %d want 0 (now folded into summary)", got)
	}
	if got := rec.count(AlertKindSummary); got != 1 {
		t.Fatalf("summary alerts: got %d want 1", got)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	ev := rec.events[0]
	if got, _ := ev.Payload["read_errors"].(int); got != 1 {
		t.Fatalf("payload.read_errors: got %d want 1", got)
	}
}

// errorDBOpen returns a *sql.DB whose QueryContext returns an error.
// Achieved by closing the DB before handing it back — every subsequent
// driver call errors with sql.ErrConnDone. Cheaper than building a
// full driver shim for one assertion.
func errorDBOpen(t *testing.T) *sql.DB {
	t.Helper()
	db := newTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestRunOnce_ScanLevelError_FiresSummaryWithScanError(t *testing.T) {
	// When Check itself errors (the SQLite scan failed — we cannot
	// enumerate any findings), runOnce still fires exactly ONE
	// AlertKindSummary alert. The summary payload carries a
	// "scan_error" key with the failure detail so on-call can
	// distinguish "scan broke" from "scan ran and found nothing per-
	// row but had transport errors." This collapses the entire
	// detector into a single Alert site (eyeball-test invariant).
	db := errorDBOpen(t)
	rec := &recordingAlerter{}

	d, err := New(db, nil, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.runOnce(context.Background())

	if got := rec.count(AlertKindSummary); got != 1 {
		t.Fatalf("summary alerts: got %d want 1 (scan-level error folds into summary)", got)
	}
	if got := rec.count(AlertKindReadError); got != 0 {
		t.Fatalf("standalone read_error alerts: got %d want 0 (folded into summary)", got)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	ev := rec.events[0]
	scanErr, ok := ev.Payload["scan_error"].(string)
	if !ok || scanErr == "" {
		t.Fatalf("payload.scan_error: missing or empty (%T %q)", ev.Payload["scan_error"], scanErr)
	}
}

func TestCheck_HonoursMaxFindings(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	// Insert MaxFindings+5 rows, none of which have S3 objects. The
	// report counters should report all of them as MissingObjects,
	// but Findings is capped.
	for i := 0; i < MaxFindings+5; i++ {
		insertRow(t, db, "fixture-a", ts, fmt.Sprintf(`{"i":%d}`, i))
	}

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.MissingObjects != MaxFindings+5 {
		t.Fatalf("MissingObjects: got %d want %d", rep.MissingObjects, MaxFindings+5)
	}
	if len(rep.Findings) != MaxFindings {
		t.Fatalf("Findings: got %d want %d (cap)", len(rep.Findings), MaxFindings)
	}
}

func TestNew_RejectsBadArgs(t *testing.T) {
	db := newTestDB(t)
	if _, err := New(nil, nil, &recordingAlerter{}, 0); err == nil {
		t.Fatal("expected error for nil db")
	}
	if _, err := New(db, nil, nil, 0); err == nil {
		t.Fatal("expected error for nil alerter")
	}
}

func TestRun_HonoursContextCancel(t *testing.T) {
	db := newTestDB(t)
	d, err := New(db, nil, &recordingAlerter{}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestRun_IntervalZeroIsNoOp(t *testing.T) {
	db := newTestDB(t)
	d, err := New(db, nil, &recordingAlerter{}, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	done := make(chan struct{})
	go func() { d.Run(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run with interval=0 did not return immediately")
	}
}

// concurrencyReader wraps a mockReader and tracks the maximum number
// of in-flight GetEntry calls seen during a Check pass. The streaming
// scan never calls GetEntry concurrently (the loop is serial inside
// scanBatch), so the recorded max should always be 1 — but the test
// is structured so that if a future refactor accidentally buffers an
// entire page's worth of GETs in parallel, the assertion would catch
// it.
//
// The stronger guarantee — that no more than batchSize rows are
// resident in memory at any point — is enforced by construction: the
// scanBatch function uses LIMIT ? in the SQL, and rows.Next streams
// row-by-row from SQLite. The test below exercises the multi-page
// code path with a batch much smaller than the dataset and asserts
// the outcome is identical to a single-batch run.
type concurrencyReader struct {
	mu       sync.Mutex
	inflight int
	peak     int
	inner    *mockReader
}

func newConcurrencyReader(inner *mockReader) *concurrencyReader {
	return &concurrencyReader{inner: inner}
}

func (c *concurrencyReader) GetEntry(ctx context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	c.inflight++
	if c.inflight > c.peak {
		c.peak = c.inflight
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.inflight--
		c.mu.Unlock()
	}()
	return c.inner.GetEntry(ctx, key)
}

// runCheck is a small helper to build a Detector with the given batch
// size and run one Check pass.
func runCheck(t *testing.T, db *sql.DB, reader EntryReader, batchSize int) *Report {
	t.Helper()
	opts := []Option{}
	if batchSize > 0 {
		opts = append(opts, WithBatchSize(batchSize))
	}
	d, err := New(db, reader, &recordingAlerter{}, 0, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return rep
}

func TestCheck_StreamingBatch_5000Rows(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	const total = 5000
	for i := 0; i < total; i++ {
		_, hash, body := insertRow(t, db, "fixture-batch", ts, fmt.Sprintf(`{"i":%d}`, i))
		reader.put(defaultKey("fixture-batch", ts, hash), []byte(body))
	}

	conc := newConcurrencyReader(reader)
	rep := runCheck(t, db, conc, 100)

	if !rep.Clean() {
		t.Fatalf("expected clean, got %+v counters", rep)
	}
	if rep.RowsScanned != total {
		t.Fatalf("RowsScanned: got %d want %d", rep.RowsScanned, total)
	}
	if reader.getCalls != total {
		t.Fatalf("getCalls: got %d want %d (every row should be probed exactly once)", reader.getCalls, total)
	}
	// Streaming scan must NOT issue concurrent GETs (the loop is
	// serial). If peak ever exceeds 1 a future change has pre-
	// fetched a page's worth of objects in parallel — that's a
	// behaviour change that has to be opted into deliberately, not
	// snuck in via a refactor of this loop.
	if conc.peak > 1 {
		t.Fatalf("peak inflight GETs: got %d want 1 (streaming scan is serial)", conc.peak)
	}
}

func TestCheck_StreamingBatch_MatchesSingleBatch(t *testing.T) {
	// Same dataset, two passes: tiny batch vs. one-batch-bigger-than-
	// dataset. Counters and findings should be identical.
	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	const total = 250 // enough to exercise multi-page when batch=37

	build := func(t *testing.T) (*sql.DB, *mockReader) {
		t.Helper()
		db := newTestDB(t)
		reader := newMockReader()
		for i := 0; i < total; i++ {
			payload := fmt.Sprintf(`{"i":%d}`, i)
			_, hash, body := insertRow(t, db, "fixture-mix", ts, payload)
			// Stage every other object; the rest are missing
			// (this fans out into MissingObjects findings).
			if i%2 == 0 {
				reader.put(defaultKey("fixture-mix", ts, hash), []byte(body))
			}
		}
		return db, reader
	}

	dbSmall, readerSmall := build(t)
	repSmall := runCheck(t, dbSmall, readerSmall, 37)

	dbBig, readerBig := build(t)
	repBig := runCheck(t, dbBig, readerBig, total+1) // single batch

	if repSmall.RowsScanned != repBig.RowsScanned {
		t.Fatalf("RowsScanned: small=%d big=%d", repSmall.RowsScanned, repBig.RowsScanned)
	}
	if repSmall.MissingObjects != repBig.MissingObjects {
		t.Fatalf("MissingObjects: small=%d big=%d", repSmall.MissingObjects, repBig.MissingObjects)
	}
	if repSmall.HashMismatches != repBig.HashMismatches {
		t.Fatalf("HashMismatches: small=%d big=%d", repSmall.HashMismatches, repBig.HashMismatches)
	}
	if repSmall.RowidGaps != repBig.RowidGaps {
		t.Fatalf("RowidGaps: small=%d big=%d", repSmall.RowidGaps, repBig.RowidGaps)
	}
	if repSmall.ReadErrors != repBig.ReadErrors {
		t.Fatalf("ReadErrors: small=%d big=%d", repSmall.ReadErrors, repBig.ReadErrors)
	}
	if len(repSmall.Findings) != len(repBig.Findings) {
		t.Fatalf("Findings length: small=%d big=%d", len(repSmall.Findings), len(repBig.Findings))
	}
	for i := range repSmall.Findings {
		if repSmall.Findings[i] != repBig.Findings[i] {
			t.Fatalf("Findings[%d] diverged: small=%+v big=%+v", i, repSmall.Findings[i], repBig.Findings[i])
		}
	}
}

func TestCheck_StreamingBatch_GapAcrossPageBoundary(t *testing.T) {
	// Gap detection must work even when the missing rowid sits exactly
	// between two pages. With batch=2 and ids 1,2,4,5 the gap (id 3)
	// is at the page boundary; the streaming scan tracks prevID
	// across pages, so it still fires.
	db := newTestDB(t)

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	insertRow(t, db, "fixture-gap", ts, `{"row":1}`)
	insertRow(t, db, "fixture-gap", ts, `{"row":2}`)
	insertRow(t, db, "fixture-gap", ts, `{"row":3}`)
	insertRow(t, db, "fixture-gap", ts, `{"row":4}`)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE id = 3`); err != nil {
		t.Fatalf("delete: %v", err)
	}

	d, err := New(db, nil, &recordingAlerter{}, 0, WithBatchSize(2))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.RowidGaps != 1 {
		t.Fatalf("RowidGaps across page boundary: got %d want 1", rep.RowidGaps)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].RowID != 3 {
		t.Fatalf("Findings: got %+v", rep.Findings)
	}
}

func TestCheck_StreamingBatch_ContinuityOnlyStillStreams(t *testing.T) {
	// Continuity-only mode (reader == nil) still streams; the SQL
	// cursor is opened but no S3 calls happen. Verify a multi-page
	// continuity-only run is clean on a dense table.
	db := newTestDB(t)
	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 250; i++ {
		insertRow(t, db, "fixture-co", ts, fmt.Sprintf(`{"i":%d}`, i))
	}
	d, err := New(db, nil, &recordingAlerter{}, 0, WithBatchSize(33))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.Clean() {
		t.Fatalf("expected clean (continuity-only dense), got %+v", rep)
	}
	if rep.RowsScanned != 250 {
		t.Fatalf("RowsScanned: got %d want 250", rep.RowsScanned)
	}
	if rep.ColdTierEnabled {
		t.Fatal("ColdTierEnabled: expected false")
	}
}

func TestWithBatchSize_RejectsNonPositive(t *testing.T) {
	// Non-positive batch sizes must be ignored — the default kicks
	// in. This protects callers from accidentally building a
	// detector that loops forever on a zero-LIMIT page.
	db := newTestDB(t)
	d, err := New(db, nil, &recordingAlerter{}, 0,
		WithBatchSize(0), WithBatchSize(-1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if d.batchSize != DefaultBatchSize {
		t.Fatalf("batchSize: got %d want %d (default)", d.batchSize, DefaultBatchSize)
	}
}

// ---------------------------------------------------------------------
// PR #54 — batched per-pass alerter fan-out.
//
// The pre-#54 behaviour fired one alerter.Alert(...) per Finding (up to
// MaxFindings = 256 calls on a saturated pass). With PagerDuty wired,
// that meant one tamper event could open 256 incidents. The new
// contract: exactly one AlertKindSummary call per non-clean pass,
// carrying the per-kind breakdown + sample of affected rowids.
// ---------------------------------------------------------------------

func TestCheck_NoFindings_NoAlert(t *testing.T) {
	// A clean pass (everything staged in WORM, no rowid gaps) must NOT
	// fire any alerter call. This is the steady-state path; firing on
	// a clean pass would page on the daily heartbeat.
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_, hash, body := insertRow(t, db, "fixture-clean", ts, fmt.Sprintf(`{"i":%d}`, i))
		reader.put(defaultKey("fixture-clean", ts, hash), []byte(body))
	}

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.runOnce(context.Background())

	rec.mu.Lock()
	got := len(rec.events)
	rec.mu.Unlock()
	if got != 0 {
		t.Fatalf("clean pass fired %d alerter calls; want 0", got)
	}
}

func TestCheck_OneFinding_OneAlert(t *testing.T) {
	// One missing S3 object → one Finding → exactly one summary alert.
	// Asserts the new per-pass contract holds at the minimum finding
	// count.
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	_, h1, body1 := insertRow(t, db, "fixture-one", ts, `{"row":1}`)
	insertRow(t, db, "fixture-one", ts, `{"row":2}`) // S3 object NOT staged
	_, h3, body3 := insertRow(t, db, "fixture-one", ts, `{"row":3}`)
	reader.put(defaultKey("fixture-one", ts, h1), []byte(body1))
	reader.put(defaultKey("fixture-one", ts, h3), []byte(body3))

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.runOnce(context.Background())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if got := len(rec.events); got != 1 {
		t.Fatalf("alerter calls: got %d want 1", got)
	}
	ev := rec.events[0]
	if ev.Kind != AlertKindSummary {
		t.Fatalf("alert kind: got %q want %q", ev.Kind, AlertKindSummary)
	}
	if got, _ := ev.Payload["missing_objects"].(int); got != 1 {
		t.Fatalf("payload.missing_objects: got %d want 1", got)
	}
	if got, _ := ev.Payload["total_findings"].(int); got != 1 {
		t.Fatalf("payload.total_findings: got %d want 1", got)
	}
}

func TestCheck_256Findings_OneAlert(t *testing.T) {
	// Saturate the findings cap (MaxFindings = 256). Pre-#54 this would
	// have fired 256 alerter calls; the new contract is one summary.
	// Also asserts the per-kind breakdown lives in the summary payload.
	db := newTestDB(t)
	reader := newMockReader() // every row's WORM object is missing
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < MaxFindings; i++ {
		insertRow(t, db, "fixture-saturate", ts, fmt.Sprintf(`{"i":%d}`, i))
	}

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.runOnce(context.Background())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if got := len(rec.events); got != 1 {
		t.Fatalf("alerter calls: got %d want 1 (per-pass summary, NOT per-finding)", got)
	}
	ev := rec.events[0]
	if ev.Kind != AlertKindSummary {
		t.Fatalf("alert kind: got %q want %q", ev.Kind, AlertKindSummary)
	}
	// Per-kind breakdown must be present and accurate. Counters are
	// independent of the Findings cap (the cap drops Finding entries,
	// not counter increments — preserved invariant from earlier PRs).
	if got, _ := ev.Payload["missing_objects"].(int); got != MaxFindings {
		t.Fatalf("payload.missing_objects: got %d want %d", got, MaxFindings)
	}
	if got, _ := ev.Payload["hash_mismatches"].(int); got != 0 {
		t.Fatalf("payload.hash_mismatches: got %d want 0", got)
	}
	if got, _ := ev.Payload["rowid_gaps"].(int); got != 0 {
		t.Fatalf("payload.rowid_gaps: got %d want 0", got)
	}
	if got, _ := ev.Payload["read_errors"].(int); got != 0 {
		t.Fatalf("payload.read_errors: got %d want 0", got)
	}
	// total_findings reflects len(Findings), which is capped at MaxFindings.
	if got, _ := ev.Payload["total_findings"].(int); got != MaxFindings {
		t.Fatalf("payload.total_findings: got %d want %d", got, MaxFindings)
	}
	// Source label is present so operators can route the pager rule.
	if got, _ := ev.Payload["source"].(string); got == "" {
		t.Fatal("payload.source: empty; want a non-empty source label")
	}
	// run_id is present and looks like an RFC3339Nano timestamp.
	runID, _ := ev.Payload["run_id"].(string)
	if runID == "" {
		t.Fatal("payload.run_id: empty")
	}
	if _, err := time.Parse(time.RFC3339Nano, runID); err != nil {
		t.Fatalf("payload.run_id: not RFC3339Nano (%q): %v", runID, err)
	}
}

func TestCheck_AlertPayloadIncludesFirst50RowIDs(t *testing.T) {
	// Stage 100 missing rows. The summary payload must enumerate the
	// first MaxSampleRowIDs (50) rowids in scan order, set
	// sample_truncated=true, and include a truncation_note that
	// references the totals.
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	const total = 100
	expectedFirst := make([]int64, 0, MaxSampleRowIDs)
	for i := 0; i < total; i++ {
		id, _, _ := insertRow(t, db, "fixture-sample", ts, fmt.Sprintf(`{"i":%d}`, i))
		if i < MaxSampleRowIDs {
			expectedFirst = append(expectedFirst, id)
		}
	}

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.runOnce(context.Background())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if got := len(rec.events); got != 1 {
		t.Fatalf("alerter calls: got %d want 1", got)
	}
	ev := rec.events[0]
	sample, ok := ev.Payload["sample_row_ids"].([]int64)
	if !ok {
		t.Fatalf("payload.sample_row_ids: missing or wrong type (%T)", ev.Payload["sample_row_ids"])
	}
	if len(sample) != MaxSampleRowIDs {
		t.Fatalf("payload.sample_row_ids: len=%d want %d", len(sample), MaxSampleRowIDs)
	}
	for i, want := range expectedFirst {
		if sample[i] != want {
			t.Fatalf("sample_row_ids[%d]: got %d want %d", i, sample[i], want)
		}
	}
	truncated, _ := ev.Payload["sample_truncated"].(bool)
	if !truncated {
		t.Fatal("payload.sample_truncated: got false want true (100 findings > 50 sample cap)")
	}
	note, _ := ev.Payload["truncation_note"].(string)
	if note == "" {
		t.Fatal("payload.truncation_note: empty; want a human-readable note")
	}
	// Note must mention both the sample cap and the true total so the
	// on-call knows what's hidden. total_findings is capped at
	// MaxFindings (256) but our dataset is 100, so all 100 are
	// reflected in the counters and the note's "of N" should be 100.
	if !strings.Contains(note, fmt.Sprintf("first %d", MaxSampleRowIDs)) ||
		!strings.Contains(note, fmt.Sprintf("of %d", total)) {
		t.Fatalf("payload.truncation_note: missing expected fragments: %q", note)
	}
}
