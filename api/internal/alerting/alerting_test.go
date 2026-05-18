package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockBackend captures every Send call so tests can assert fan-out
// semantics. It can be configured to accept only certain severities and
// to return an error.
type mockBackend struct {
	name     string
	accepts  map[Severity]bool
	sendErr  error
	mu       sync.Mutex
	received []Alert
}

func (m *mockBackend) Name() string { return m.name }

func (m *mockBackend) HandlesSeverity(s Severity) bool {
	if m.accepts == nil {
		return true
	}
	return m.accepts[s]
}

func (m *mockBackend) Send(_ context.Context, a Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.received = append(m.received, a)
	return m.sendErr
}

func (m *mockBackend) calls() []Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Alert, len(m.received))
	copy(out, m.received)
	return out
}

func TestRouter_FireFansOutToAllAcceptingBackends(t *testing.T) {
	all := &mockBackend{name: "all"}
	critOnly := &mockBackend{name: "crit", accepts: map[Severity]bool{SeverityCritical: true}}
	warnOnly := &mockBackend{name: "warn", accepts: map[Severity]bool{SeverityWarning: true}}

	r := NewRouter()
	r.AddBackend(all)
	r.AddBackend(critOnly)
	r.AddBackend(warnOnly)

	alert := Alert{Severity: SeverityCritical, Title: "drift", Source: "owera-agentic-api"}
	if err := r.Fire(context.Background(), alert); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	if got := len(all.calls()); got != 1 {
		t.Fatalf("all backend: got %d calls, want 1", got)
	}
	if got := len(critOnly.calls()); got != 1 {
		t.Fatalf("crit-only backend: got %d calls, want 1", got)
	}
	if got := len(warnOnly.calls()); got != 0 {
		t.Fatalf("warn-only backend: got %d calls, want 0 (severity filter must drop)", got)
	}
}

func TestRouter_FireContinuesAfterBackendError(t *testing.T) {
	// Acceptance gate from compliance/runbooks/pagerduty-setup.md §5:
	// a PagerDuty outage must not silence the local audit emission.
	// First backend errors; second must still receive the alert.
	failing := &mockBackend{name: "pagerduty", sendErr: errors.New("network down")}
	logging := &mockBackend{name: "log"}

	r := NewRouter()
	r.AddBackend(failing)
	r.AddBackend(logging)

	alert := Alert{Severity: SeverityCritical, Title: "drift"}
	err := r.Fire(context.Background(), alert)
	if err == nil {
		t.Fatal("Fire: expected first-error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "pagerduty") {
		t.Fatalf("Fire error: want backend name in message, got %q", err.Error())
	}
	if got := len(logging.calls()); got != 1 {
		t.Fatalf("log backend: got %d calls, want 1 (must run after pagerduty failure)", got)
	}
}

func TestRouter_BackendsReturnsRegistrationOrder(t *testing.T) {
	r := NewRouter()
	r.AddBackend(&mockBackend{name: "log"})
	r.AddBackend(&mockBackend{name: "pagerduty"})
	got := r.Backends()
	want := []string{"log", "pagerduty"}
	if len(got) != len(want) {
		t.Fatalf("Backends len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Backends[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestRouter_AddBackendIgnoresNil(t *testing.T) {
	r := NewRouter()
	r.AddBackend(nil)
	if got := len(r.Backends()); got != 0 {
		t.Fatalf("got %d backends, want 0", got)
	}
}

func TestPagerDutyBackend_HandlesSeverity(t *testing.T) {
	b := &PagerDutyBackend{RoutingKey: "x"}
	cases := []struct {
		sev  Severity
		want bool
	}{
		{SeverityCritical, true},
		{SeverityWarning, false},
		{SeverityInfo, false},
		{Severity("unknown"), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.sev), func(t *testing.T) {
			if got := b.HandlesSeverity(tc.sev); got != tc.want {
				t.Fatalf("HandlesSeverity(%q): got %v want %v", tc.sev, got, tc.want)
			}
		})
	}
}

func TestPagerDutyBackend_SendPostsCorrectShape(t *testing.T) {
	var (
		gotMethod, gotPath, gotContentType string
		gotBody                            []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted) // PagerDuty's success status
	}))
	t.Cleanup(srv.Close)

	b := &PagerDutyBackend{
		RoutingKey: "test-routing-key",
		Endpoint:   srv.URL,
		HTTPClient: srv.Client(),
	}

	occurred := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	alert := Alert{
		Severity:   SeverityCritical,
		Title:      "billing.reconcile.drift",
		Body:       "drift exceeded 0.5%",
		Source:     "owera-agentic-api",
		Dedup:      "drift-tnt-abc-2026-05-17",
		Labels:     map[string]string{"tenant_id": "tnt-abc", "drift_frac": "0.012"},
		OccurredAt: occurred,
	}

	if err := b.Send(context.Background(), alert); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method: got %q want POST", gotMethod)
	}
	if gotPath != "/" {
		t.Fatalf("path: got %q want /", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type: got %q want application/json", gotContentType)
	}

	var env pagerDutyEnvelope
	if err := json.Unmarshal(gotBody, &env); err != nil {
		t.Fatalf("decode body: %v\nbody: %s", err, string(gotBody))
	}
	if env.RoutingKey != "test-routing-key" {
		t.Fatalf("routing_key: got %q want test-routing-key", env.RoutingKey)
	}
	if env.EventAction != "trigger" {
		t.Fatalf("event_action: got %q want trigger", env.EventAction)
	}
	if env.DedupKey != "drift-tnt-abc-2026-05-17" {
		t.Fatalf("dedup_key: got %q want drift-tnt-abc-2026-05-17", env.DedupKey)
	}
	if env.Payload.Summary != "billing.reconcile.drift" {
		t.Fatalf("summary: got %q", env.Payload.Summary)
	}
	if env.Payload.Source != "owera-agentic-api" {
		t.Fatalf("source: got %q", env.Payload.Source)
	}
	if env.Payload.Severity != "critical" {
		t.Fatalf("severity: got %q want critical", env.Payload.Severity)
	}
	if env.Payload.Timestamp != occurred.Format(time.RFC3339) {
		t.Fatalf("timestamp: got %q want %q", env.Payload.Timestamp, occurred.Format(time.RFC3339))
	}
	if got := env.Payload.CustomDetails["tenant_id"]; got != "tnt-abc" {
		t.Fatalf("custom_details.tenant_id: got %q want tnt-abc", got)
	}
	if got := env.Payload.CustomDetails["drift_frac"]; got != "0.012" {
		t.Fatalf("custom_details.drift_frac: got %q want 0.012", got)
	}
	if got := env.Payload.CustomDetails["body"]; got != "drift exceeded 0.5%" {
		t.Fatalf("custom_details.body: got %q want body field", got)
	}
}

func TestPagerDutyBackend_SendDefaultsDedupToTitle(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	b := &PagerDutyBackend{RoutingKey: "k", Endpoint: srv.URL, HTTPClient: srv.Client()}
	if err := b.Send(context.Background(), Alert{Severity: SeverityCritical, Title: "fallback-title"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var env pagerDutyEnvelope
	if err := json.Unmarshal(gotBody, &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.DedupKey != "fallback-title" {
		t.Fatalf("dedup_key: got %q want fallback-title", env.DedupKey)
	}
}

func TestPagerDutyBackend_SendErrorsOnNon202Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream down"}`))
	}))
	t.Cleanup(srv.Close)

	b := &PagerDutyBackend{RoutingKey: "k", Endpoint: srv.URL, HTTPClient: srv.Client()}
	err := b.Send(context.Background(), Alert{Severity: SeverityCritical, Title: "t"})
	if err == nil {
		t.Fatal("Send: want error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("Send error: want status code in message, got %q", err.Error())
	}
}

func TestNewPagerDuty_RejectsEmptyRoutingKey(t *testing.T) {
	if _, err := NewPagerDuty(""); err == nil {
		t.Fatal("NewPagerDuty(\"\"): want error, got nil")
	}
}

func TestNewPagerDuty_DefaultsEndpointAndTimeout(t *testing.T) {
	b, err := NewPagerDuty("k")
	if err != nil {
		t.Fatalf("NewPagerDuty: %v", err)
	}
	if b.Endpoint != PagerDutyEndpoint {
		t.Fatalf("endpoint: got %q want %q", b.Endpoint, PagerDutyEndpoint)
	}
	if b.HTTPClient == nil {
		t.Fatal("HTTPClient: nil")
	}
	if b.HTTPClient.Timeout != 30*time.Second {
		t.Fatalf("HTTPClient.Timeout: got %v want 30s", b.HTTPClient.Timeout)
	}
}
