package audit

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestLog(t *testing.T, opts ...Option) *Log {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	l, err := New(db, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l
}

func TestAppendAndList(t *testing.T) {
	l := newTestLog(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := l.Append(ctx, Entry{
			TenantID: "fixture-a", UserID: "fixture-usr-1",
			Action: "job.submit", Target: "fixture-job-1",
			IP: "127.0.0.1", UserAgent: "test/1",
		})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	rows, err := l.List(ctx, "fixture-a", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len: got %d want 3", len(rows))
	}
}

func TestList_FiltersByTenant(t *testing.T) {
	l := newTestLog(t)
	ctx := context.Background()
	_ = l.Append(ctx, Entry{TenantID: "fixture-a", Action: "x", IP: "1.1.1.1", UserAgent: "t"})
	_ = l.Append(ctx, Entry{TenantID: "fixture-b", Action: "x", IP: "2.2.2.2", UserAgent: "t"})
	rows, _ := l.List(ctx, "fixture-a", 10)
	if len(rows) != 1 {
		t.Fatalf("len: got %d want 1", len(rows))
	}
	if rows[0].IP != "1.1.1.1" {
		t.Fatalf("ip: got %q want 1.1.1.1", rows[0].IP)
	}
}

func TestFromRequest_StripsPort(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/jobs", nil)
	r.RemoteAddr = "10.0.0.1:54321"
	e := FromRequest(r, "fixture-a", "fixture-usr-1", "job.submit", "fixture-job-1")
	if e.IP != "10.0.0.1" {
		t.Fatalf("IP: got %q want 10.0.0.1", e.IP)
	}
}

func TestFromRequest_ParsesXForwardedFor(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/jobs", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 70.41.3.18, 150.172.238.178")
	e := FromRequest(r, "fixture-a", "fixture-usr-1", "job.submit", "fixture-job-1")
	if e.IP != "203.0.113.5" {
		t.Fatalf("IP: got %q want 203.0.113.5", e.IP)
	}
}

func TestAppend_RejectsIncomplete(t *testing.T) {
	l := newTestLog(t)
	if err := l.Append(context.Background(), Entry{Action: "x"}); err == nil {
		t.Fatal("expected error for missing tenant_id")
	}
}

// --- Hash-chain + WORM tamper tests (T18.1 acceptance) ---

func TestHashChain_LinksRowsForward(t *testing.T) {
	l := newTestLog(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := l.Append(ctx, Entry{
			TenantID: "fixture-a", Action: "job.submit",
			Target: "fixture-job", IP: "127.0.0.1", UserAgent: "t",
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if bad, err := l.Verify(ctx); err != nil {
		t.Fatalf("Verify: bad=%d err=%v", bad, err)
	}
}

func TestVerify_DetectsTamperedRow(t *testing.T) {
	l := newTestLog(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = l.Append(ctx, Entry{
			TenantID: "fixture-a", Action: "job.submit",
			Target: "fixture-job", IP: "127.0.0.1", UserAgent: "t",
		})
	}
	// Direct DB poke past the trigger by dropping it first — simulates
	// an attacker with raw file access. Verify() must still flag it.
	if _, err := l.db.Exec(`DROP TRIGGER IF EXISTS audit_log_no_update`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	if _, err := l.db.Exec(`UPDATE audit_log SET action='tampered' WHERE id=2`); err != nil {
		t.Fatalf("tamper update: %v", err)
	}
	bad, err := l.Verify(ctx)
	if err == nil {
		t.Fatalf("Verify: expected error, got nil (bad=%d)", bad)
	}
	if bad != 2 {
		t.Fatalf("Verify: expected break at id=2, got %d", bad)
	}
}

func TestSQLTrigger_BlocksUpdateAndDelete(t *testing.T) {
	l := newTestLog(t)
	ctx := context.Background()
	_ = l.Append(ctx, Entry{
		TenantID: "fixture-a", Action: "job.submit", Target: "fixture-job",
		IP: "127.0.0.1", UserAgent: "t",
	})
	if _, err := l.db.ExecContext(ctx, `UPDATE audit_log SET action='hax' WHERE id=1`); err == nil {
		t.Fatal("expected UPDATE on audit_log to fail (WORM trigger)")
	}
	if _, err := l.db.ExecContext(ctx, `DELETE FROM audit_log WHERE id=1`); err == nil {
		t.Fatal("expected DELETE on audit_log to fail (WORM trigger)")
	}
}

func TestWORMStreamer_RejectsOverwrite(t *testing.T) {
	streamer := NewMockWORMStreamer(24 * time.Hour)
	l := newTestLog(t, WithStreamer(streamer))
	ctx := context.Background()

	e := Entry{
		TenantID: "fixture-a", Action: "job.submit", Target: "fixture-job-1",
		IP: "127.0.0.1", UserAgent: "t", Ts: time.Now().UTC(),
	}
	if err := l.Append(ctx, e); err != nil {
		t.Fatalf("Append: %v", err)
	}
	keys := streamer.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 WORM object, got %d", len(keys))
	}
	// Adversary attempts to overwrite the same key with new bytes.
	// The mock mirrors S3 object-lock GOVERNANCE: blocked.
	if err := streamer.TamperPut(keys[0], []byte(`{"action":"forged"}`)); err == nil {
		t.Fatal("expected ErrWORMLocked on tamper PUT, got nil")
	}
}

func TestAppend_StreamFailureSurfaced(t *testing.T) {
	streamer := &alwaysFail{}
	l := newTestLog(t, WithStreamer(streamer))
	err := l.Append(context.Background(), Entry{
		TenantID: "fixture-a", Action: "x", IP: "1.1.1.1", UserAgent: "t",
	})
	if err == nil {
		t.Fatal("expected stream error to surface")
	}
}

type alwaysFail struct{}

func (alwaysFail) PutEntry(_ context.Context, _ Entry, _ []byte) (string, error) {
	return "", errors.ErrUnsupported
}

// fakeEtagStreamer is a minimal WORMStreamer that returns a stable
// canned ETag so the test can assert Append populated audit_log.s3_etag.
type fakeEtagStreamer struct {
	etag string
}

func (f *fakeEtagStreamer) PutEntry(_ context.Context, _ Entry, _ []byte) (string, error) {
	return f.etag, nil
}

// TestAppend_PersistsEtagFromStreamer is the #53 acceptance check.
// Append must record the streamer's returned ETag on audit_log.s3_etag
// so the tamperdetect cron can take the HEAD-based integrity path on
// the next pass. An empty ETag is legal (we don't error); a non-empty
// ETag MUST be persisted.
func TestAppend_PersistsEtagFromStreamer(t *testing.T) {
	streamer := &fakeEtagStreamer{etag: "deadbeef-mock-etag"}
	l := newTestLog(t, WithStreamer(streamer))
	ctx := context.Background()

	if err := l.Append(ctx, Entry{
		TenantID: "fixture-a", Action: "job.submit", Target: "fixture-job",
		IP: "127.0.0.1", UserAgent: "t",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	var got sql.NullString
	if err := l.db.QueryRowContext(ctx,
		`SELECT s3_etag FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("select s3_etag: %v", err)
	}
	if !got.Valid || got.String != "deadbeef-mock-etag" {
		t.Fatalf("s3_etag: got valid=%v value=%q want valid=true value=%q",
			got.Valid, got.String, "deadbeef-mock-etag")
	}
}

// TestAppend_EmptyEtagLeavesNull confirms the "streamer cannot surface
// an ETag" path: Append must not error and must leave s3_etag NULL so
// tamperdetect's legacy fallback runs.
func TestAppend_EmptyEtagLeavesNull(t *testing.T) {
	streamer := &fakeEtagStreamer{etag: ""}
	l := newTestLog(t, WithStreamer(streamer))
	ctx := context.Background()

	if err := l.Append(ctx, Entry{
		TenantID: "fixture-a", Action: "job.submit", Target: "fixture-job",
		IP: "127.0.0.1", UserAgent: "t",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	var got sql.NullString
	if err := l.db.QueryRowContext(ctx,
		`SELECT s3_etag FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&got); err != nil {
		t.Fatalf("select s3_etag: %v", err)
	}
	if got.Valid {
		t.Fatalf("s3_etag: got valid=true value=%q; want NULL", got.String)
	}
}

// TestMigrate_IsIdempotent runs New twice on the same db handle to
// confirm the s3_etag column + trigger replacement are run-twice safe.
// A second New() call must not error.
func TestMigrate_IsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := New(db); err != nil {
		t.Fatalf("New (first): %v", err)
	}
	if _, err := New(db); err != nil {
		t.Fatalf("New (second, idempotency check): %v", err)
	}
	// Sanity: the column actually exists.
	rows, err := db.Query(`PRAGMA table_info(audit_log)`)
	if err != nil {
		t.Fatalf("PRAGMA: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var (
			cid              int
			name, ctype      string
			notnull, pk      int
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "s3_etag" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected audit_log.s3_etag column after migrate()")
	}
}
