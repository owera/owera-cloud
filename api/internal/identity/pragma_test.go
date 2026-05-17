package identity

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestOpen_PragmasOnEveryPoolConnection verifies that PRAGMAs reach every
// connection the pool checks out, not just the first one. The previous
// db.Exec("PRAGMA …") shape only applied to one connection; verifying the
// fix means probing journal_mode from multiple concurrent queries.
func TestOpen_PragmasOnEveryPoolConnection(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "identity.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Force several connections by running parallel probes. Each
	// connection independently reports journal_mode.
	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var mode string
			if err := s.DB().QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode); err != nil {
				errs <- err
				return
			}
			if !strings.EqualFold(mode, "wal") {
				errs <- errPragmaWrong(mode)
				return
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("pragma probe: %v", err)
	}
}

// TestOpen_ConcurrentWritesNoBusy reproduces the failure mode WS-14's
// 1,000-job load test surfaced — concurrent writes against a file-backed
// store. Without WAL + busy_timeout on every pool connection, this
// produces SQLITE_BUSY. With the DSN-pragma fix, all writes complete.
func TestOpen_ConcurrentWritesNoBusy(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "identity.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	const writers = 32
	const perWriter = 8
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, writers*perWriter)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				if _, err := s.CreateTenant(ctx, tenantName(w, i)); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		// SQLITE_BUSY surfaces as `database is locked` in modernc.org/sqlite.
		if err != nil && strings.Contains(err.Error(), "database is locked") {
			t.Fatalf("SQLITE_BUSY surfaced — PRAGMA not applied to every pool connection: %v", err)
		}
		if err != nil {
			t.Errorf("CreateTenant: %v", err)
		}
	}
}

func errPragmaWrong(got string) error {
	return errors.New("expected journal_mode=wal, got " + got)
}

func tenantName(writer, i int) string {
	return "tenant-w" + itoa(writer) + "-i" + itoa(i)
}

func itoa(i int) string {
	// Avoid pulling strconv into the package for a tiny helper.
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
