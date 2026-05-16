package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestOutbound_SignsAndDelivers(t *testing.T) {
	fake := &FakeSender{}
	s := New(fake, "secret-key")
	now := time.Unix(1700000000, 0).UTC()
	s.SetClock(func() time.Time { return now })

	err := s.Outbound(context.Background(), "https://customer.example/wh", map[string]any{
		"event": "job.succeeded",
		"job":   "job_123",
	})
	if err != nil {
		t.Fatalf("Outbound: %v", err)
	}
	if len(fake.Sent) != 1 {
		t.Fatalf("delivery count: got %d want 1", len(fake.Sent))
	}
	sent := fake.Sent[0]
	if sent.Headers["X-Owera-Timestamp"] != "1700000000" {
		t.Fatalf("timestamp header: got %q", sent.Headers["X-Owera-Timestamp"])
	}
	// Compute expected signature and compare.
	mac := hmac.New(sha256.New, []byte("secret-key"))
	mac.Write([]byte("1700000000."))
	mac.Write(sent.Body)
	want := hex.EncodeToString(mac.Sum(nil))
	if sent.Headers["X-Owera-Signature"] != want {
		t.Fatalf("signature mismatch")
	}
}

func TestVerifyStripe_Valid(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	secret := "whsec_test"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	s := New(nil, "")
	header := "t=" + ts + ",v1=" + sig
	if err := s.VerifyStripe(header, body, secret, 5*time.Minute); err != nil {
		t.Fatalf("VerifyStripe: %v", err)
	}
}

func TestVerifyStripe_BadSignature(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	header := "t=" + ts + ",v1=deadbeef"
	s := New(nil, "")
	if err := s.VerifyStripe(header, body, "whsec_test", 5*time.Minute); err == nil {
		t.Fatal("expected signature mismatch")
	}
}

func TestVerifyStripe_TimestampSkewRejected(t *testing.T) {
	body := []byte(`{}`)
	secret := "whsec_test"
	staleTs := strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(staleTs + "."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	header := "t=" + staleTs + ",v1=" + sig
	s := New(nil, "")
	if err := s.VerifyStripe(header, body, secret, 5*time.Minute); err == nil {
		t.Fatal("expected timestamp skew error")
	}
}

func TestVerifyStripe_MalformedHeader(t *testing.T) {
	s := New(nil, "")
	if err := s.VerifyStripe("garbage", []byte("{}"), "whsec", 5*time.Minute); err == nil {
		t.Fatal("expected error for malformed header")
	}
}

func TestHandleStripeRequest(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	secret := "whsec_test"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/stripe/wh", io.NopCloser(bytes.NewReader(body)))
	req.Header.Set("Stripe-Signature", "t="+ts+",v1="+sig)
	s := New(nil, "")
	got, err := s.HandleStripeRequest(req, secret, 5*time.Minute)
	if err != nil {
		t.Fatalf("HandleStripeRequest: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body mismatch")
	}
}
