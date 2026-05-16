// Package audit is the compliance audit log for customer-affecting
// actions. Every customer-affecting handler appends one row with the
// canonical schema:
//
//	{tenant_id, user_id, action, target, ts, ip, user_agent}
//
// Rows are append-only at the API surface: the table has no UPDATE or
// DELETE path. The intent is WORM-friendly storage; future work can
// migrate the underlying table to an external append-only sink (S3 with
// Object Lock, BigQuery streaming) without changing this package's API.
package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Entry is one audit row.
type Entry struct {
	TenantID  string
	UserID    string
	Action    string
	Target    string
	Ts        time.Time
	IP        string
	UserAgent string
}

// Log is the audit sink. Storage is SQLite for the scaffold; the migration
// path to external WORM storage is comment-tracked in the package doc.
type Log struct {
	db *sql.DB
}

// New returns an audit log writing to db.
func New(db *sql.DB) (*Log, error) {
	if db == nil {
		return nil, errors.New("audit: nil db")
	}
	l := &Log{db: db}
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
			user_agent  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS audit_tenant_ts_idx ON audit_log(tenant_id, ts DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := l.db.Exec(stmt); err != nil {
			return fmt.Errorf("audit: migrate: %w", err)
		}
	}
	return nil
}

// Append writes one audit entry. Append never returns errors that should
// block the caller's primary action — callers should log and continue.
func (l *Log) Append(ctx context.Context, e Entry) error {
	if e.TenantID == "" || e.Action == "" {
		return errors.New("audit: tenant_id and action required")
	}
	if e.Ts.IsZero() {
		e.Ts = time.Now().UTC()
	}
	_, err := l.db.ExecContext(ctx,
		`INSERT INTO audit_log(tenant_id,user_id,action,target,ts,ip,user_agent)
		 VALUES(?,?,?,?,?,?,?)`,
		e.TenantID, e.UserID, e.Action, e.Target, e.Ts, e.IP, e.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
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
		`SELECT tenant_id,user_id,action,target,ts,ip,user_agent
		 FROM audit_log WHERE tenant_id=? ORDER BY ts DESC, id DESC LIMIT ?`,
		tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: list: %w", err)
	}
	defer rows.Close()
	var out []*Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.TenantID, &e.UserID, &e.Action, &e.Target, &e.Ts, &e.IP, &e.UserAgent); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
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
