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
