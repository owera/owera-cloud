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

func (alwaysFail) PutEntry(_ context.Context, _ Entry, _ []byte) error {
	return errors.ErrUnsupported
}
