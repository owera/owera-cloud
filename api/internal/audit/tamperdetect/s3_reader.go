package tamperdetect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// S3EntryReader is the production EntryReader. It GETs the WORM object
// at <Endpoint>/<Bucket>/<key> using a caller-supplied http.Client whose
// transport signs each request with SigV4 (see audit.NewSigV4HTTPClient).
//
// 404 → ErrObjectNotFound; non-2xx other than 404 → wrapped error so
// the detector can record it as a read_error finding.
type S3EntryReader struct {
	// HTTPClient must sign requests with SigV4 for the bucket's region.
	// In production this is the same client the S3WORMStreamer uses for
	// PUTs; sharing it keeps connection pooling effective.
	HTTPClient *http.Client

	// Endpoint is the S3 base URL, e.g. "https://s3.us-east-1.amazonaws.com".
	Endpoint string

	// Bucket is the object-lock-enabled audit bucket.
	Bucket string
}

// GetEntry implements EntryReader.
func (r *S3EntryReader) GetEntry(ctx context.Context, key string) ([]byte, error) {
	if r.HTTPClient == nil {
		return nil, errors.New("tamperdetect: S3EntryReader.HTTPClient is nil")
	}
	if r.Bucket == "" || r.Endpoint == "" {
		return nil, errors.New("tamperdetect: S3EntryReader needs Bucket and Endpoint")
	}
	url := fmt.Sprintf("%s/%s/%s", r.Endpoint, r.Bucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("tamperdetect: build request: %w", err)
	}
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tamperdetect: s3 get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrObjectNotFound
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tamperdetect: s3 get status=%d body=%s", resp.StatusCode, string(body))
	}
	// Cap at 8 MiB. Audit canonical bodies are well under 4 KiB in
	// practice; the limit exists to prevent a hostile mock or a
	// misconfigured bucket from blowing the apiserver's heap.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("tamperdetect: read body: %w", err)
	}
	return body, nil
}

// HeadEntry implements EntryReader. Issues an HTTP HEAD against the
// WORM object and returns the bare-hex ETag (S3's surrounding quotes
// stripped). Returns (etag, true, nil) on a 2xx, ("", false, nil) on a
// 404, and ("", false, err) on any other non-2xx or transport error.
// Callers fall back to GetEntry when this returns an error or an empty
// ETag, so a transient HEAD failure never fails closed — it just
// degrades to the original full-body integrity path.
func (r *S3EntryReader) HeadEntry(ctx context.Context, key string) (string, bool, error) {
	if r.HTTPClient == nil {
		return "", false, errors.New("tamperdetect: S3EntryReader.HTTPClient is nil")
	}
	if r.Bucket == "" || r.Endpoint == "" {
		return "", false, errors.New("tamperdetect: S3EntryReader needs Bucket and Endpoint")
	}
	url := fmt.Sprintf("%s/%s/%s", r.Endpoint, r.Bucket, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", false, fmt.Errorf("tamperdetect: build head request: %w", err)
	}
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("tamperdetect: s3 head: %w", err)
	}
	// HEAD has no body but we still close the response for connection
	// reuse on the SigV4 transport's keep-alive pool.
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if resp.StatusCode/100 != 2 {
		return "", false, fmt.Errorf("tamperdetect: s3 head status=%d", resp.StatusCode)
	}
	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	return etag, true, nil
}
