// SigV4 HTTP transport for the S3WORMStreamer.
//
// S3WORMStreamer (worm.go) expects a caller-supplied *http.Client whose
// transport signs each request with AWS SigV4. We use aws-sdk-go-v2 to
// resolve credentials (env, shared config, IMDS, web-identity, SSO),
// then wrap http.DefaultTransport with a RoundTripper that signs the
// outgoing PUT bodies.
//
// We do not use the SDK's S3 service client because the streamer is a
// minimal "PUT with object-lock headers" path that pre-dates the SDK
// integration; pulling in service/s3 would be heavier than necessary.

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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
)

// NewSigV4HTTPClient returns an *http.Client whose transport signs each
// request with SigV4 for the given service+region. Credentials are
// resolved via the standard AWS chain (env → shared config → IMDS).
//
// service is typically "s3"; region is e.g. "us-east-1".
func NewSigV4HTTPClient(ctx context.Context, region, service string) (*http.Client, error) {
	if service == "" {
		return nil, errors.New("audit: SigV4 service is empty (expected e.g. \"s3\")")
	}
	if region == "" {
		return nil, errors.New("audit: SigV4 region is empty (expected e.g. \"us-east-1\")")
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("audit: load AWS config: %w", err)
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &sigv4Transport{
			creds:   cfg.Credentials,
			signer:  v4.NewSigner(),
			service: service,
			region:  region,
			base:    http.DefaultTransport,
		},
	}, nil
}

// sigv4Transport buffers the request body once (SigV4 needs the SHA256
// of the payload for header generation) and replays it on the wire.
//
// All requests routed through this transport are signed for one
// service/region pair. This matches the streamer's single-bucket
// scope; if multi-region or multi-service signing is ever needed,
// instantiate one client per (service, region) pair.
type sigv4Transport struct {
	creds   aws.CredentialsProvider
	signer  *v4.Signer
	service string
	region  string
	base    http.RoundTripper
}

func (t *sigv4Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := drainBody(req)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])

	creds, err := t.creds.Retrieve(req.Context())
	if err != nil {
		return nil, fmt.Errorf("audit: retrieve AWS creds: %w", err)
	}

	// S3 requires X-Amz-Content-Sha256 on every signed request; the v4
	// signer signs the value we set here. Without this header, S3
	// returns InvalidRequest. (The standard SDK S3 client sets it via
	// the smithy v4 middleware; we mirror that behaviour explicitly.)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	if err := t.signer.SignHTTP(req.Context(), creds, req, payloadHash, t.service, t.region, time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("audit: SigV4 sign: %w", err)
	}
	return t.base.RoundTrip(req)
}

// drainBody reads + replaces req.Body so the SigV4 signer sees a stable
// hash and the wire transport sees the same bytes. Empty-body requests
// return (nil, nil) so the streamer's GET-style probes also work.
func drainBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("audit: drain request body: %w", err)
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.ContentLength = int64(len(body))
	return body, nil
}
