package audit

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestLog(t *testing.T) *Log {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	l, err := New(db)
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
			TenantID: "ten_a", UserID: "usr_1",
			Action: "job.submit", Target: "job_1",
			IP: "127.0.0.1", UserAgent: "test/1",
		})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	rows, err := l.List(ctx, "ten_a", 10)
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
	_ = l.Append(ctx, Entry{TenantID: "ten_a", Action: "x", IP: "1.1.1.1", UserAgent: "t"})
	_ = l.Append(ctx, Entry{TenantID: "ten_b", Action: "x", IP: "2.2.2.2", UserAgent: "t"})
	rows, _ := l.List(ctx, "ten_a", 10)
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
	e := FromRequest(r, "ten_a", "usr_1", "job.submit", "job_1")
	if e.IP != "10.0.0.1" {
		t.Fatalf("IP: got %q want 10.0.0.1", e.IP)
	}
}

func TestFromRequest_ParsesXForwardedFor(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/jobs", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 70.41.3.18, 150.172.238.178")
	e := FromRequest(r, "ten_a", "usr_1", "job.submit", "job_1")
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
