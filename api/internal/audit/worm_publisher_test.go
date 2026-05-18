package audit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestS3WORMStreamer_HeadersAndBody verifies that S3WORMStreamer issues
// a PUT with the canonical body unchanged and the two object-lock
// headers (mode + retain-until-date) present. The SigV4 Authorization
// header is verified separately in TestSigV4Transport_AddsAuthHeader
// — here we leave HTTPClient as plain http.DefaultClient so the body
// and object-lock semantics are tested in isolation.
func TestS3WORMStreamer_HeadersAndBody(t *testing.T) {
	var (
		gotMethod, gotPath, gotMode, gotRetainUntil, gotContentType string
		gotBody                                                     []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotMode = r.Header.Get("x-amz-object-lock-mode")
		gotRetainUntil = r.Header.Get("x-amz-object-lock-retain-until-date")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	streamer := &S3WORMStreamer{
		HTTPClient:    srv.Client(),
		Endpoint:      srv.URL,
		Bucket:        "owera-audit-test",
		Region:        "us-east-1",
		RetentionDays: 30,
	}

	entry := Entry{
		TenantID: "tnt-abc",
		UserID:   "usr-1",
		Action:   "job.submit",
		Target:   "job-1",
		Ts:       time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		IP:       "127.0.0.1",
		Hash:     "deadbeef",
	}
	canonical := []byte(`{"hash":"deadbeef"}`)

	if err := streamer.PutEntry(context.Background(), entry, canonical); err != nil {
		t.Fatalf("PutEntry: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Fatalf("method: got %q want PUT", gotMethod)
	}
	wantPath := "/owera-audit-test/audit/tnt-abc/2026-05-18/deadbeef.json"
	if gotPath != wantPath {
		t.Fatalf("path: got %q want %q", gotPath, wantPath)
	}
	if gotMode != "GOVERNANCE" {
		t.Fatalf("object-lock mode: got %q want GOVERNANCE", gotMode)
	}
	if gotRetainUntil == "" {
		t.Fatal("object-lock retain-until-date header missing")
	}
	if _, err := time.Parse(time.RFC3339, gotRetainUntil); err != nil {
		t.Fatalf("retain-until-date not RFC3339: %q (%v)", gotRetainUntil, err)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type: got %q want application/json", gotContentType)
	}
	if string(gotBody) != string(canonical) {
		t.Fatalf("body: got %q want %q", string(gotBody), string(canonical))
	}
}

// TestS3WORMStreamer_LockedRejection verifies the ErrWORMLocked wrap
// when the backend rejects the PUT with HTTP 403 + AccessDenied body.
// Real S3 returns this when a write hits an object still inside its
// retention window; the streamer must surface it as ErrWORMLocked so
// callers can distinguish "retention violation" from "transport error".
func TestS3WORMStreamer_LockedRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("AccessDenied: Cannot modify object with object lock"))
	}))
	t.Cleanup(srv.Close)

	streamer := &S3WORMStreamer{
		HTTPClient: srv.Client(),
		Endpoint:   srv.URL,
		Bucket:     "owera-audit-test",
		Region:     "us-east-1",
	}
	entry := Entry{TenantID: "t", Ts: time.Now().UTC(), Hash: "h"}
	err := streamer.PutEntry(context.Background(), entry, []byte("x"))
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
	if !strings.Contains(err.Error(), "object-lock retention prevents overwrite") {
		t.Fatalf("expected ErrWORMLocked wrap, got: %v", err)
	}
}

// TestSigV4Transport_AddsAuthHeader verifies that NewSigV4HTTPClient
// produces a transport that signs each request: every outgoing PUT
// must carry an Authorization: AWS4-HMAC-SHA256 header. We point the
// streamer at an httptest server and feed creds via env vars so the
// SDK's default config chain picks them up without any AWS network
// calls.
func TestSigV4Transport_AddsAuthHeader(t *testing.T) {
	// Deliberately non-AWS-pattern dummy strings so gitleaks doesn't
	// match the AKIA... regex on this test file. The signer doesn't
	// validate format; it just hashes the value into the canonical
	// request, so any non-empty string is acceptable for the unit test.
	const fakeAccessKey = "TESTACCESSKEY-NOT-AWS"
	const fakeSecretKey = "testSecret/NotARealAwsSecretKey/000000000"
	t.Setenv("AWS_ACCESS_KEY_ID", fakeAccessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", fakeSecretKey)
	t.Setenv("AWS_REGION", "us-east-1")
	// Belt-and-suspenders: clear AWS_PROFILE so shared-config probing
	// doesn't accidentally pick up the dev machine's real profile.
	t.Setenv("AWS_PROFILE", "")

	var gotAuth, gotPayloadHash string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPayloadHash = r.Header.Get("X-Amz-Content-Sha256")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	httpClient, err := NewSigV4HTTPClient(context.Background(), "us-east-1", "s3")
	if err != nil {
		t.Fatalf("NewSigV4HTTPClient: %v", err)
	}
	streamer := &S3WORMStreamer{
		HTTPClient: httpClient,
		Endpoint:   srv.URL,
		Bucket:     "owera-audit-test",
		Region:     "us-east-1",
	}
	entry := Entry{TenantID: "t", Ts: time.Now().UTC(), Hash: "h"}
	if err := streamer.PutEntry(context.Background(), entry, []byte(`{"h":"x"}`)); err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	if !strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 ") {
		t.Fatalf("Authorization header: got %q, want AWS4-HMAC-SHA256 prefix", gotAuth)
	}
	if !strings.Contains(gotAuth, "Credential="+fakeAccessKey+"/") {
		t.Fatalf("Authorization header missing access-key credential scope: %q", gotAuth)
	}
	if gotPayloadHash == "" {
		t.Fatal("X-Amz-Content-Sha256 header missing; signer should have set it")
	}
}
