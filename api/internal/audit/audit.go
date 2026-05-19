// Package audit is the compliance audit log for customer-affecting
// actions. Every customer-affecting handler appends one row with the
// canonical schema:
//
//	{tenant_id, user_id, action, target, ts, ip, user_agent, prev_hash, hash}
//
// Rows are append-only at the API surface: the table has no UPDATE or
// DELETE path. Each row carries the SHA-256 hash of the previous row's
// canonical bytes, forming a hash chain that lets the auditor detect
// any tamper after the fact even before WORM kicks in.
//
// Storage is two-tier:
//
//   - Hot:  SQLite (sqlcipher in prod) — fast reads for tenant-scoped
//     compliance queries.
//   - Cold: S3 with Object Lock in Governance mode — the actual WORM
//     surface. The streamer (see WORMStreamer) PUTs each row exactly
//     once with a retention header; the bucket policy rejects any
//     overwrite of the key for the retention period.
//
// Tamper detection at the SQLite layer: SQLite triggers (see migrate())
// reject UPDATE and DELETE on audit_log entirely. Combined with the
// hash chain, an attacker who bypasses the trigger by editing the file
// directly still breaks the chain on the next verify().
package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Entry is one audit row.
type Entry struct {
	ID        int64     `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Ts        time.Time `json:"ts"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	PrevHash  string    `json:"prev_hash"`
	Hash      string    `json:"hash"`
}

// WORMStreamer is the cold-tier sink. Implementations write each entry
// to a write-once medium (e.g. S3 with Object Lock). Failures are
// returned to the caller; Append degrades to "hot only" but records a
// streamer_err in stderr — the operator is expected to backfill.
//
// PutEntry returns the cold-tier object's ETag (bare hex, no surrounding
// quotes) so Log.Append can record it in audit_log.s3_etag. The audit/
// tamperdetect cron uses the stored ETag to verify the WORM object's
// identity via HEAD instead of a full GET on every pass (1M GETs → ~100K
// HEADs at the planned 7-year retention horizon). An empty ETag is
// legal (returned when the streamer cannot capture one, e.g. a backend
// that does not surface ETags); tamperdetect falls back to the
// GET-and-rehash path for any row whose s3_etag is NULL or empty.
type WORMStreamer interface {
	PutEntry(ctx context.Context, e Entry, canonical []byte) (etag string, err error)
}

// Log is the audit sink. Storage is SQLite for the scaffold; the
// migration path to external WORM storage is via WORMStreamer.
type Log struct {
	db       *sql.DB
	streamer WORMStreamer
	mu       sync.Mutex // serializes Append so the chain stays linear
}

// Option mutates Log construction.
type Option func(*Log)

// WithStreamer attaches a WORM streamer. Without one, Log writes to
// SQLite only — fine for unit tests, not acceptable in production.
func WithStreamer(s WORMStreamer) Option {
	return func(l *Log) { l.streamer = s }
}

// New returns an audit log writing to db.
func New(db *sql.DB, opts ...Option) (*Log, error) {
	if db == nil {
		return nil, errors.New("audit: nil db")
	}
	l := &Log{db: db}
	for _, o := range opts {
		o(l)
	}
	if err := l.migrate(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Log) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS audit_log (
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
		)`,
		`CREATE INDEX IF NOT EXISTS audit_tenant_ts_idx ON audit_log(tenant_id, ts DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := l.db.Exec(stmt); err != nil {
			return fmt.Errorf("audit: migrate: %w", err)
		}
	}
	// Additive column for tamperdetect (#53). NULL on legacy rows; the
	// detector falls back to GET+rehash when the column is empty. We do
	// the ALTER TABLE conditionally by probing PRAGMA table_info so the
	// migration is run-twice safe (SQLite would otherwise error on a
	// duplicate column).
	if err := l.ensureColumnTEXT("audit_log", "s3_etag"); err != nil {
		return fmt.Errorf("audit: migrate s3_etag: %w", err)
	}
	// WORM enforcement: reject UPDATE and DELETE at the SQL layer.
	// An attacker who bypasses these by editing the .db directly still
	// breaks the hash chain, which Verify() detects.
	//
	// The UPDATE trigger uses BEFORE UPDATE OF <column-list> so it
	// only fires for the immutable, hash-covered columns. s3_etag is
	// intentionally excluded: Log.Append writes it via a follow-up
	// UPDATE after PutEntry returns the cold-tier ETag (no roll-back-
	// able tx involved, so the "no gaps" invariant on Append stays
	// intact). Every column listed below IS part of the canonical hash
	// input, so a tamperer cannot rewrite history through the s3_etag
	// loophole — the hash chain still breaks on the next Verify().
	if err := l.ensureNoUpdateTrigger(); err != nil {
		return fmt.Errorf("audit: migrate trigger: %w", err)
	}
	if _, err := l.db.Exec(`CREATE TRIGGER IF NOT EXISTS audit_log_no_delete
		BEFORE DELETE ON audit_log
		BEGIN SELECT RAISE(ABORT, 'audit_log is append-only (WORM)'); END`); err != nil {
		return fmt.Errorf("audit: migrate: %w", err)
	}
	// Backfill prev_hash/hash columns on pre-existing rows from earlier
	// scaffolds. Safe no-op on a fresh table.
	if err := l.backfillChain(); err != nil {
		return fmt.Errorf("audit: backfill: %w", err)
	}
	return nil
}

// noUpdateTriggerSQL is the canonical definition of the BEFORE-UPDATE
// guard. Kept in one place so backfillChain's drop/re-create cycle
// installs the exact same shape migrate() installs on fresh tables.
// The column list deliberately excludes id (autoincrement, never
// user-set) and s3_etag (post-write metadata, see migrate() comment).
const noUpdateTriggerSQL = `CREATE TRIGGER audit_log_no_update
	BEFORE UPDATE OF tenant_id, user_id, action, target, ts, ip, user_agent, prev_hash, hash
	ON audit_log
	BEGIN SELECT RAISE(ABORT, 'audit_log is append-only (WORM)'); END`

// ensureNoUpdateTrigger creates the BEFORE-UPDATE guard idempotently.
// Older deployments installed a trigger with no column filter (it
// fired on any column, including s3_etag — which would block #53's
// Append→Put→UPDATE flow). We detect that variant by inspecting
// sqlite_master.sql and replace it; matching triggers are left alone.
func (l *Log) ensureNoUpdateTrigger() error {
	var existing sql.NullString
	err := l.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='trigger' AND name='audit_log_no_update'`).
		Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Fresh table — create the column-scoped trigger.
		_, err := l.db.Exec(noUpdateTriggerSQL)
		return err
	case err != nil:
		return err
	}
	// Existing trigger covers s3_etag if it doesn't have an `OF` clause.
	// Replace it in that case; otherwise leave it.
	if !strings.Contains(strings.ToUpper(existing.String), "BEFORE UPDATE OF") {
		if _, err := l.db.Exec(`DROP TRIGGER audit_log_no_update`); err != nil {
			return err
		}
		if _, err := l.db.Exec(noUpdateTriggerSQL); err != nil {
			return err
		}
	}
	return nil
}

// ensureColumnTEXT issues `ALTER TABLE … ADD COLUMN … TEXT` exactly
// when the named column is absent, by probing PRAGMA table_info. SQLite
// has no `IF NOT EXISTS` on ADD COLUMN; this is the standard idiom.
func (l *Log) ensureColumnTEXT(table, col string) error {
	rows, err := l.db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return rows.Close()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = l.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s TEXT`, table, col))
	return err
}

func (l *Log) backfillChain() error {
	rows, err := l.db.Query(
		`SELECT id, tenant_id, user_id, action, target, ts, ip, user_agent
		 FROM audit_log WHERE hash = '' ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type pending struct {
		e Entry
	}
	var todo []pending
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.Action, &e.Target,
			&e.Ts, &e.IP, &e.UserAgent); err != nil {
			return err
		}
		todo = append(todo, pending{e})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(todo) == 0 {
		return nil
	}
	prev, err := l.lastHashLocked()
	if err != nil {
		return err
	}
	// UPDATEs to hash-covered columns would trip the no-update trigger.
	// Backfill happens via the sqlite_master raw path: drop the trigger,
	// write, re-create. We only run this on rows that have no hash, so
	// the WORM contract is preserved for hashed rows. The re-creation
	// uses noUpdateTriggerSQL so the column-scoped shape (which excludes
	// s3_etag) is the only shape that ever lands in production.
	if _, err := l.db.Exec(`DROP TRIGGER IF EXISTS audit_log_no_update`); err != nil {
		return err
	}
	defer func() {
		_, _ = l.db.Exec(noUpdateTriggerSQL)
	}()
	for _, p := range todo {
		p.e.PrevHash = prev
		canonical, h := chainHash(p.e)
		_ = canonical
		_, err := l.db.Exec(`UPDATE audit_log SET prev_hash=?, hash=? WHERE id=?`,
			p.e.PrevHash, h, p.e.ID)
		if err != nil {
			return err
		}
		prev = h
	}
	return nil
}

// Append writes one audit entry. Append serializes on the chain so two
// concurrent callers can't both build off the same prev_hash.
//
// # Invariant: do not wrap Append in a roll-back-able transaction
//
// Callers MUST NOT call Append from inside a SQL transaction (sql.Tx or
// equivalent) that may roll back. Append owns its own write; the audit
// log is not a participant in caller-level units of work.
//
// Why: the audit_log primary key is SQLite INTEGER PRIMARY KEY
// AUTOINCREMENT. SQLite reserves the next id at INSERT-statement start
// and bumps sqlite_sequence even if the surrounding transaction later
// rolls back. The reserved id is therefore consumed but no row is
// written — a permanent gap in the id sequence. The tamper-detect cron
// (see package audit/tamperdetect) treats any gap as evidence that a
// row was DELETEd to hide an event and fires an
// audit.tamper.rowid_gap Critical alert against that ghost id, on
// every run, forever. There is no in-band way to distinguish a
// rolled-back ghost id from a maliciously deleted row.
//
// Anti-pattern (do NOT do this):
//
//	tx, _ := db.BeginTx(ctx, nil)
//	// ... some work ...
//	if err := auditLog.Append(ctx, entry); err != nil {
//	    tx.Rollback() // BAD: leaks an audit_log id even on the happy
//	                  //     path if "some work" later fails.
//	    return err
//	}
//	if err := doMoreWork(tx); err != nil {
//	    tx.Rollback() // BAD: ditto — Append already inserted and
//	                  //     consumed an id; rollback won't undo the
//	                  //     id reservation.
//	    return err
//	}
//	tx.Commit()
//
// Correct pattern: append the audit row after the business transaction
// has committed (Append-after-commit), or — if you need an "undo"
// semantic on the audit trail itself — append a compensating audit
// row describing the reversal. The chain stays linear and gap-free
// either way:
//
//	if err := tx.Commit(); err != nil {
//	    return err
//	}
//	_ = auditLog.Append(ctx, entry) // post-commit
//
//	// If business logic later decides the action was wrong:
//	_ = auditLog.Append(ctx, compensatingEntry) // not a rollback
//
// This invariant is documentation-only today; there is no runtime or
// static check enforcing it. A vet/linter rule that flags tx.Rollback
// in code paths reachable from Append is a possible future addition
// (see owera-cloud#55) but is out of scope here.
func (l *Log) Append(ctx context.Context, e Entry) error {
	if e.TenantID == "" || e.Action == "" {
		return errors.New("audit: tenant_id and action required")
	}
	if e.Ts.IsZero() {
		e.Ts = time.Now().UTC()
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	prev, err := l.lastHashLocked()
	if err != nil {
		return fmt.Errorf("audit: lastHash: %w", err)
	}
	e.PrevHash = prev
	canonical, h := chainHash(e)
	e.Hash = h

	res, err := l.db.ExecContext(ctx,
		`INSERT INTO audit_log(tenant_id,user_id,action,target,ts,ip,user_agent,prev_hash,hash)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		e.TenantID, e.UserID, e.Action, e.Target, e.Ts, e.IP, e.UserAgent, e.PrevHash, e.Hash,
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	if id, err := res.LastInsertId(); err == nil {
		e.ID = id
	}
	// Stream to WORM cold store. Failure here is recorded but does
	// not undo the SQLite write — the hot log is the immediate truth;
	// the operator backfills cold from hot if the streamer was down.
	//
	// On success, capture the cold-tier ETag returned by PutEntry and
	// record it on audit_log.s3_etag via a follow-up UPDATE (not wrapped
	// in a transaction — see the long invariant doc above; the autoinc
	// id is already reserved by the INSERT above so an UPDATE here
	// cannot leak ids). If PutEntry returned an empty ETag (legal — the
	// streamer just didn't surface one), we skip the UPDATE; tamperdetect
	// treats NULL/empty as "fall back to full-body GET" so this row is
	// just-as-safe, only-as-cheap-as-it-was-before-#53.
	//
	// If PutEntry FAILS, we surface the error and leave s3_etag = NULL —
	// the row is already INSERTed (the SQLite truth is the immediate
	// source of truth, per the existing contract), and tamperdetect's
	// fallback handles legacy NULL rows correctly.
	if l.streamer != nil {
		etag, err := l.streamer.PutEntry(ctx, e, canonical)
		if err != nil {
			return fmt.Errorf("audit: stream: %w", err)
		}
		if etag != "" && e.ID > 0 {
			if _, err := l.db.ExecContext(ctx,
				`UPDATE audit_log SET s3_etag = ? WHERE id = ?`, etag, e.ID); err != nil {
				return fmt.Errorf("audit: persist etag: %w", err)
			}
		}
	}
	return nil
}

// lastHashLocked returns the hash of the most recent row, or "" if the
// table is empty. Caller holds l.mu.
func (l *Log) lastHashLocked() (string, error) {
	var h sql.NullString
	err := l.db.QueryRow(`SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h.String, nil
}

// FromRequest builds an Entry from an HTTP request, leaving Action/Target
// to the caller (those are handler-specific).
func FromRequest(r *http.Request, tenantID, userID, action, target string) Entry {
	return Entry{
		TenantID:  tenantID,
		UserID:    userID,
		Action:    action,
		Target:    target,
		Ts:        time.Now().UTC(),
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
	}
}

// List returns recent entries for a tenant, newest first.
func (l *Log) List(ctx context.Context, tenantID string, limit int) ([]*Entry, error) {
	if tenantID == "" {
		return nil, errors.New("audit: empty tenant_id")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := l.db.QueryContext(ctx,
		`SELECT id,tenant_id,user_id,action,target,ts,ip,user_agent,prev_hash,hash
		 FROM audit_log WHERE tenant_id=? ORDER BY ts DESC, id DESC LIMIT ?`,
		tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: list: %w", err)
	}
	defer rows.Close()
	var out []*Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.Action, &e.Target, &e.Ts,
			&e.IP, &e.UserAgent, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// Verify walks the chain from row 1 forward, recomputing each row's
// hash. Returns the id of the first mismatch (or 0 if intact) and a
// descriptive error. Used by the quarterly audit-log integrity check.
func (l *Log) Verify(ctx context.Context) (int64, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT id,tenant_id,user_id,action,target,ts,ip,user_agent,prev_hash,hash
		 FROM audit_log ORDER BY id ASC`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	prev := ""
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.Action, &e.Target, &e.Ts,
			&e.IP, &e.UserAgent, &e.PrevHash, &e.Hash); err != nil {
			return 0, err
		}
		if e.PrevHash != prev {
			return e.ID, fmt.Errorf("audit: chain break at id=%d (prev_hash mismatch)", e.ID)
		}
		_, expect := chainHash(e)
		if expect != e.Hash {
			return e.ID, fmt.Errorf("audit: chain break at id=%d (hash mismatch)", e.ID)
		}
		prev = e.Hash
	}
	return 0, rows.Err()
}

// chainHash returns the canonical JSON bytes for an entry (with Hash
// elided) and the SHA-256 hex of those bytes.
func chainHash(e Entry) ([]byte, string) {
	// Canonical form: id is omitted (assigned by SQLite after insert);
	// hash is omitted (it's what we're computing).
	can := struct {
		TenantID  string    `json:"tenant_id"`
		UserID    string    `json:"user_id"`
		Action    string    `json:"action"`
		Target    string    `json:"target"`
		Ts        time.Time `json:"ts"`
		IP        string    `json:"ip"`
		UserAgent string    `json:"user_agent"`
		PrevHash  string    `json:"prev_hash"`
	}{
		TenantID:  e.TenantID,
		UserID:    e.UserID,
		Action:    e.Action,
		Target:    e.Target,
		Ts:        e.Ts.UTC(),
		IP:        e.IP,
		UserAgent: e.UserAgent,
		PrevHash:  e.PrevHash,
	}
	b, _ := json.Marshal(can)
	sum := sha256.Sum256(b)
	return b, hex.EncodeToString(sum[:])
}

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		// Take the first hop only.
		for i := 0; i < len(xf); i++ {
			if xf[i] == ',' {
				return trimSpace(xf[:i])
			}
		}
		return trimSpace(xf)
	}
	if r.RemoteAddr != "" {
		// Strip the port if present.
		for i := len(r.RemoteAddr) - 1; i >= 0; i-- {
			if r.RemoteAddr[i] == ':' {
				return r.RemoteAddr[:i]
			}
		}
		return r.RemoteAddr
	}
	return ""
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
