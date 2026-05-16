// Package webhooks handles inbound Stripe events and outbound customer
// notifications. Outbound deliveries are HMAC-signed so customers can
// verify origin: the signature is sent in the X-Owera-Signature header,
// computed as HMAC-SHA256(secret, ts + "." + body) and presented as
// hex.
//
// Inbound Stripe verification follows Stripe's official scheme: the
// Stripe-Signature header contains t=<ts> and v1=<signature>. We
// compute the same HMAC over (ts + "." + body) using the customer's
// Stripe signing secret and constant-time-compare.
//
// No live network calls happen in this package; outbound is via a
// pluggable Sender interface.
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sender posts an outbound webhook payload to a customer's endpoint.
type Sender interface {
	Send(ctx context.Context, url string, body []byte, headers map[string]string) error
}

// Service composes inbound verification + outbound signing.
type Service struct {
	sender         Sender
	outboundSecret string // shared signing secret for outbound webhooks
	now            func() time.Time
}

// New returns a webhooks service. outboundSecret is the HMAC key used
// when signing outbound deliveries. sender is the HTTP poster (nil
// disables outbound delivery).
func New(sender Sender, outboundSecret string) *Service {
	return &Service{
		sender:         sender,
		outboundSecret: outboundSecret,
		now:            func() time.Time { return time.Now().UTC() },
	}
}

// Outbound delivers payload to url with a signed header. The body is
// JSON-encoded payload.
func (s *Service) Outbound(ctx context.Context, url string, payload any) error {
	if s.sender == nil {
		return errors.New("webhooks: no outbound sender")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhooks: marshal: %w", err)
	}
	ts := strconv.FormatInt(s.now().Unix(), 10)
	sig := signHMAC(s.outboundSecret, ts, body)
	headers := map[string]string{
		"Content-Type":       "application/json",
		"X-Owera-Timestamp":  ts,
		"X-Owera-Signature":  sig,
	}
	return s.sender.Send(ctx, url, body, headers)
}

// VerifyStripe verifies a Stripe-Signature header. Returns nil on match.
//
// tolerance is the max skew between server time and the timestamp in the
// signature header; 5 minutes matches Stripe's published default.
func (s *Service) VerifyStripe(header string, body []byte, signingSecret string, tolerance time.Duration) error {
	if header == "" {
		return errors.New("webhooks: missing Stripe-Signature header")
	}
	if signingSecret == "" {
		return errors.New("webhooks: empty signing secret")
	}
	var (
		ts    string
		v1Sig string
	)
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			v1Sig = kv[1]
		}
	}
	if ts == "" || v1Sig == "" {
		return errors.New("webhooks: malformed Stripe-Signature")
	}
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("webhooks: parse ts: %w", err)
	}
	skew := s.now().Sub(time.Unix(tsInt, 0))
	if skew < 0 {
		skew = -skew
	}
	if tolerance > 0 && skew > tolerance {
		return fmt.Errorf("webhooks: timestamp skew %s exceeds tolerance %s", skew, tolerance)
	}
	want := signHMAC(signingSecret, ts, body)
	if !hmac.Equal([]byte(want), []byte(v1Sig)) {
		return errors.New("webhooks: signature mismatch")
	}
	return nil
}

// HandleStripeRequest is a convenience wrapper that reads the body and
// verifies a request. Returns the raw body on success so the handler
// can dispatch on event type.
func (s *Service) HandleStripeRequest(r *http.Request, signingSecret string, tolerance time.Duration) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("webhooks: read body: %w", err)
	}
	if err := s.VerifyStripe(r.Header.Get("Stripe-Signature"), body, signingSecret, tolerance); err != nil {
		return nil, err
	}
	return body, nil
}

// signHMAC computes hex(HMAC-SHA256(secret, ts + "." + body)).
func signHMAC(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// FakeSender is an in-memory Sender used in tests.
type FakeSender struct {
	Sent []FakeSend
}

// FakeSend records one delivery.
type FakeSend struct {
	URL     string
	Body    []byte
	Headers map[string]string
}

// Send records the delivery.
func (f *FakeSender) Send(_ context.Context, url string, body []byte, headers map[string]string) error {
	f.Sent = append(f.Sent, FakeSend{URL: url, Body: bytes.Clone(body), Headers: headers})
	return nil
}

// SetClock overrides the time source. Tests only.
func (s *Service) SetClock(now func() time.Time) { s.now = now }
