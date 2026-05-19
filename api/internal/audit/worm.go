package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrWORMLocked is returned by WORM streamers when a caller attempts
// to overwrite a key that is still within its object-lock retention
// window. In S3 this is HTTP 403 with `AccessDenied` and a
// "Cannot modify object with object lock" message; the test mock
// returns this sentinel so the test does not depend on AWS network
// error shapes.
var ErrWORMLocked = errors.New("audit: object-lock retention prevents overwrite")

// MockWORMStreamer is an in-process WORM implementation that mirrors
// the semantics of S3 with Object Lock in Governance mode:
//
//   - Each PutEntry writes the canonical bytes under a deterministic
//     key derived from {tenant_id, ts, hash} so re-puts for the same
//     row hit the same key.
//   - Once a key is written with a retention timestamp, any subsequent
//     PUT to the same key before that timestamp returns ErrWORMLocked.
//
// This is the test-time substitute for the S3 streamer; the cloud
// production wiring is S3WORMStreamer below.
type MockWORMStreamer struct {
	mu        sync.Mutex
	retention time.Duration
	objects   map[string]mockObject // key → object
}

type mockObject struct {
	body          []byte
	etag          string
	retentionTill time.Time
}

// NewMockWORMStreamer returns a streamer with the given retention
// window. Tests typically use a long window (24h) to guarantee the
// tamper attempt falls inside it.
func NewMockWORMStreamer(retention time.Duration) *MockWORMStreamer {
	return &MockWORMStreamer{
		retention: retention,
		objects:   make(map[string]mockObject),
	}
}

// PutEntry writes an entry to the mock WORM. If a key is already
// present and inside its retention window, returns ErrWORMLocked.
// Returns the stored object's ETag (sha256-hex of the canonical bytes)
// so Log.Append can record it on audit_log.s3_etag — mirrors what real
// S3 returns in the ETag response header for non-multipart PUTs.
func (s *MockWORMStreamer) PutEntry(_ context.Context, e Entry, canonical []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := wormKey(e)
	now := time.Now().UTC()
	if existing, ok := s.objects[key]; ok && now.Before(existing.retentionTill) {
		return "", fmt.Errorf("%w: key=%s retained_until=%s", ErrWORMLocked, key, existing.retentionTill.Format(time.RFC3339))
	}
	sum := sha256.Sum256(canonical)
	etag := hex.EncodeToString(sum[:])
	s.objects[key] = mockObject{
		body:          append([]byte(nil), canonical...),
		etag:          etag,
		retentionTill: now.Add(s.retention),
	}
	return etag, nil
}

// TamperPut directly attempts to overwrite an existing key with new
// bytes. This is the read-only test fixture for the tamper-rejection
// acceptance criterion: the underlying WORM rejects the write.
func (s *MockWORMStreamer) TamperPut(key string, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.objects[key]; ok && time.Now().UTC().Before(existing.retentionTill) {
		return fmt.Errorf("%w: key=%s", ErrWORMLocked, key)
	}
	s.objects[key] = mockObject{
		body: append([]byte(nil), body...),
	}
	return nil
}

// Keys returns the set of keys currently stored, sorted insertion-
// order is not guaranteed. For test assertions only.
func (s *MockWORMStreamer) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.objects))
	for k := range s.objects {
		out = append(out, k)
	}
	return out
}

// KeyFor exposes the deterministic key for a given entry.
func (s *MockWORMStreamer) KeyFor(e Entry) string { return wormKey(e) }

// WormKey returns the deterministic WORM object key for an entry's
// (tenant_id, ts, hash) triple. Exported so the tamper-detect
// reconciler can recompute the same key when verifying objects against
// SQLite rows without importing the package-local helper.
func WormKey(tenantID string, ts time.Time, hash string) string {
	day := ts.UTC().Format("2006-01-02")
	return fmt.Sprintf("audit/%s/%s/%s.json", tenantID, day, hash)
}

func wormKey(e Entry) string {
	// Layout chosen so S3 lifecycle queries by tenant/day are cheap:
	//   audit/<tenant_id>/<YYYY-MM-DD>/<hash>.json
	day := e.Ts.UTC().Format("2006-01-02")
	return fmt.Sprintf("audit/%s/%s/%s.json", e.TenantID, day, e.Hash)
}

// S3WORMStreamer streams audit entries to an S3 bucket configured with
// Object Lock in Governance mode. The bucket must be created with
// Object Lock enabled at creation time (S3 cannot enable it on an
// existing bucket); we PUT each object with x-amz-object-lock-mode
// = GOVERNANCE and a retain-until-date computed from RetentionDays.
//
// The implementation talks to S3 via the HTTP PUT path because the
// AWS SDK is out-of-scope for the WS-18 dependency budget; the
// signature is SigV4-AWS-S3 supplied by the caller's transport.
// Production deployments wire a real SigV4-signing http.RoundTripper
// (typically from aws-sdk-go-v2's smithy transport) onto HTTPClient.
//
// For local dev the same struct points at MinIO with --object-lock.
type S3WORMStreamer struct {
	HTTPClient    *http.Client
	Endpoint      string // e.g. "https://s3.us-east-1.amazonaws.com"
	Bucket        string // e.g. "owera-audit-prod"
	Region        string // e.g. "us-east-1"
	RetentionDays int    // ≥ 2555 (7y) for production compliance
}

// PutEntry uploads one entry to the bucket with object-lock retention.
// Returns ErrWORMLocked wrapped in a fmt.Errorf if the bucket rejects
// the PUT for retention reasons. On success, returns the object's ETag
// (bare hex; S3's surrounding double-quotes are stripped) so the caller
// can persist it on audit_log.s3_etag. An empty string is returned if
// the backend did not surface an ETag header — that is treated as a
// soft signal by tamperdetect (NULL/empty → fall back to full-body GET).
func (s *S3WORMStreamer) PutEntry(ctx context.Context, e Entry, canonical []byte) (string, error) {
	if s.HTTPClient == nil {
		return "", errors.New("audit: S3WORMStreamer.HTTPClient is nil (need SigV4 transport)")
	}
	if s.Bucket == "" || s.Endpoint == "" {
		return "", errors.New("audit: S3WORMStreamer needs Bucket and Endpoint")
	}
	if s.RetentionDays <= 0 {
		s.RetentionDays = 2555 // 7y default
	}
	key := wormKey(e)
	url := fmt.Sprintf("%s/%s/%s", s.Endpoint, s.Bucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(canonical))
	if err != nil {
		return "", err
	}
	retainUntil := time.Now().UTC().Add(time.Duration(s.RetentionDays) * 24 * time.Hour)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-amz-object-lock-mode", "GOVERNANCE")
	req.Header.Set("x-amz-object-lock-retain-until-date", retainUntil.Format(time.RFC3339))
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("audit: s3 put: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: %s", ErrWORMLocked, string(body))
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("audit: s3 put status=%d body=%s", resp.StatusCode, string(body))
	}
	// S3 returns the ETag wrapped in double quotes, e.g. `"abc123..."`.
	// Strip them so the value matches what HEAD will return after the
	// reader package's own normalisation — both sides store bare hex.
	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	return etag, nil
}
