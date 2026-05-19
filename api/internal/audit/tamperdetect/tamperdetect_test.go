package tamperdetect

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// mockReader is a minimal in-memory EntryReader. Test cases stage the
// canonical bytes per key; GetEntry returns ErrObjectNotFound for any
// key not present unless ForceError is set (which simulates an S3
// outage and fans out to the read_error alert kind).
//
// HEAD support (#53): putWithEtag stages an explicit ETag for HEAD
// probes. Otherwise HeadEntry derives an ETag deterministically as
// sha256-hex(body), which matches what audit.MockWORMStreamer does in
// production-mock paths. headForceErr (separate from forceErr) lets a
// test exercise the "HEAD fails, fall back to GET" defensive path
// without making GET fail too.
type mockReader struct {
	mu           sync.Mutex
	objects      map[string][]byte
	etags        map[string]string // explicit per-key ETag override
	forceErr     error
	headForceErr error
	getCalls     int
	headCalls    int
	missingKey   string // when non-empty, this key alone returns ErrObjectNotFound
}

func newMockReader() *mockReader {
	return &mockReader{objects: map[string][]byte{}, etags: map[string]string{}}
}

func (m *mockReader) put(key string, body []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), body...)
}

// putWithEtag stages a body AND pins an explicit ETag for HEAD lookup.
// Used by HEAD-path tests that need to make HEAD's ETag diverge from
// the body's sha256 (or match a value Append recorded earlier).
func (m *mockReader) putWithEtag(key string, body []byte, etag string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), body...)
	m.etags[key] = etag
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

// HeadEntry returns the per-key ETag if one was staged via putWithEtag,
// otherwise the sha256-hex of the staged body (the natural
// MockWORMStreamer convention). missingKey suppresses HEAD too;
// headForceErr lets tests exercise the GET-fallback path.
func (m *mockReader) HeadEntry(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.headCalls++
	if m.headForceErr != nil {
		return "", false, m.headForceErr
	}
	if m.missingKey != "" && key == m.missingKey {
		return "", false, nil
	}
	body, ok := m.objects[key]
	if !ok {
		return "", false, nil
	}
	if etag, pinned := m.etags[key]; pinned {
		return etag, true, nil
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), true, nil
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
		hash        TEXT NOT NULL DEFAULT '',
		s3_etag     TEXT
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

// setEtag stamps audit_log.s3_etag for the given row. Tests use this
// to simulate the post-#53 Append flow without depending on the audit
// package's UPDATE machinery — keeps the tamperdetect tests focused on
// the read path.
func setEtag(t *testing.T, db *sql.DB, id int64, etag string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE audit_log SET s3_etag = ? WHERE id = ?`, etag, id); err != nil {
		t.Fatalf("set etag: %v", err)
	}
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

	// runOnce should fan out exactly one alert.
	d.runOnce(context.Background())
	if got := rec.count(AlertKindMissing); got != 1 {
		t.Fatalf("alerts fired: got %d want 1 of kind=%s", got, AlertKindMissing)
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

func TestCheck_ReadError_FiresReadErrorAlert(t *testing.T) {
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
	if got := rec.count(AlertKindReadError); got != 1 {
		t.Fatalf("read_error alerts: got %d want 1", got)
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

func (c *concurrencyReader) HeadEntry(ctx context.Context, key string) (string, bool, error) {
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
	return c.inner.HeadEntry(ctx, key)
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

// --- #53: HEAD-based ETag integrity (+ GET fallback) ---

// TestCheck_HeadETagMatch_PassesWithoutGET stages rows with a known
// s3_etag stamped on audit_log.s3_etag and a matching object in the
// mock reader. The detector must take the HEAD-only path: zero GETs,
// one HEAD per row, clean report.
func TestCheck_HeadETagMatch_PassesWithoutGET(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	const total = 5
	for i := 0; i < total; i++ {
		payload := fmt.Sprintf(`{"row":%d}`, i)
		id, hash, body := insertRow(t, db, "fixture-head", ts, payload)
		key := defaultKey("fixture-head", ts, hash)
		// Mock's HeadEntry returns sha256(body) when no explicit ETag
		// is pinned — mirror that on the SQLite side so HEAD agrees
		// with the stored s3_etag.
		sum := sha256.Sum256([]byte(body))
		etag := hex.EncodeToString(sum[:])
		reader.put(key, []byte(body))
		setEtag(t, db, id, etag)
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
	if reader.headCalls != total {
		t.Fatalf("headCalls: got %d want %d (HEAD per row)", reader.headCalls, total)
	}
	if reader.getCalls != 0 {
		t.Fatalf("getCalls: got %d want 0 (HEAD-only happy path; #53 is broken)", reader.getCalls)
	}
}

// TestCheck_HeadETagMismatch_FallsBackToGET pins HEAD to return an
// ETag that does NOT match the row's stored s3_etag. The detector must
// fall back to GET-and-rehash, and (since we ALSO inject a tampered
// body) report a hash mismatch finding via the canonical GET path —
// not a cryptic ETag-level finding. This is the integrity-critical
// path: a sophisticated attacker who could rewrite the object would
// also rewrite the ETag; the body hash chain is the authoritative
// source of truth.
func TestCheck_HeadETagMismatch_FallsBackToGET(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	id, hash, _ := insertRow(t, db, "fixture-mm", ts, `{"row":1}`)
	// Append-time we recorded etag="aaa"; HEAD now says "bbb" (something
	// changed in S3). The body we stage also doesn't hash to `hash`, so
	// GET-and-rehash will fire the AlertKindMismatch finding.
	key := defaultKey("fixture-mm", ts, hash)
	reader.putWithEtag(key, []byte(`{"row":1,"injected":"evil"}`), "bbb")
	setEtag(t, db, id, "aaa")

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.HashMismatches != 1 {
		t.Fatalf("HashMismatches: got %d want 1", rep.HashMismatches)
	}
	if reader.headCalls != 1 {
		t.Fatalf("headCalls: got %d want 1", reader.headCalls)
	}
	if reader.getCalls != 1 {
		t.Fatalf("getCalls: got %d want 1 (mismatch must fall back to GET)", reader.getCalls)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].RowID != id || rep.Findings[0].Kind != AlertKindMismatch {
		t.Fatalf("Findings: got %+v", rep.Findings)
	}
}

// TestCheck_LegacyRowNullETag_FallsBackToGET covers the legacy / pre-#53
// row whose audit_log.s3_etag IS NULL. The detector must skip HEAD
// entirely (since there's nothing to compare against) and take the
// original GET-and-rehash code path. Behaviour is unchanged from PR #51
// for any row written before #53 lands.
func TestCheck_LegacyRowNullETag_FallsBackToGET(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	_, hash, body := insertRow(t, db, "fixture-legacy", ts, `{"row":1}`)
	reader.put(defaultKey("fixture-legacy", ts, hash), []byte(body))
	// Deliberately do NOT call setEtag — the row keeps s3_etag NULL.

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
	if reader.headCalls != 0 {
		t.Fatalf("headCalls: got %d want 0 (legacy NULL must skip HEAD)", reader.headCalls)
	}
	if reader.getCalls != 1 {
		t.Fatalf("getCalls: got %d want 1 (legacy NULL must take GET path)", reader.getCalls)
	}
}

// TestCheck_HeadError_FallsBackToGET covers the defensive fallback: a
// transient HEAD failure (S3 503, etc.) must NOT fail the row closed —
// the GET path runs and either confirms integrity or surfaces the
// authoritative finding.
func TestCheck_HeadError_FallsBackToGET(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	reader.headForceErr = errors.New("simulated head 503")
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	id, hash, body := insertRow(t, db, "fixture-headerr", ts, `{"row":1}`)
	reader.put(defaultKey("fixture-headerr", ts, hash), []byte(body))
	setEtag(t, db, id, "some-etag")

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !rep.Clean() {
		t.Fatalf("expected clean (HEAD-error → GET-and-rehash succeeds), got %+v", rep)
	}
	if reader.headCalls != 1 || reader.getCalls != 1 {
		t.Fatalf("expected exactly one HEAD then one GET; got head=%d get=%d",
			reader.headCalls, reader.getCalls)
	}
}

// TestCheck_HeadMissing_FiresMissingFinding asserts that a HEAD 404
// short-circuits to the same AlertKindMissing finding the GET 404
// path produces — no wasted body fetch on objects we already know
// are gone.
func TestCheck_HeadMissing_FiresMissingFinding(t *testing.T) {
	db := newTestDB(t)
	reader := newMockReader()
	rec := &recordingAlerter{}

	ts := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	id, _, _ := insertRow(t, db, "fixture-headmiss", ts, `{"row":1}`)
	setEtag(t, db, id, "some-etag")
	// Body is NOT staged → mockReader.HeadEntry returns (_, false, nil).

	d, err := New(db, reader, rec, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep, err := d.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.MissingObjects != 1 {
		t.Fatalf("MissingObjects: got %d want 1", rep.MissingObjects)
	}
	if reader.headCalls != 1 {
		t.Fatalf("headCalls: got %d want 1", reader.headCalls)
	}
	if reader.getCalls != 0 {
		t.Fatalf("getCalls: got %d want 0 (HEAD 404 must not fall through to GET)", reader.getCalls)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Kind != AlertKindMissing {
		t.Fatalf("Findings: got %+v", rep.Findings)
	}
}
