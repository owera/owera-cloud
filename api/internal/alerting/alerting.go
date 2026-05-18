// Package alerting is the cloud-plane fan-out for operational alerts.
//
// The drift reconciler and any future alert source produce an Alert and
// hand it to a Router. The Router fans out to every registered Backend
// that accepts the alert's Severity. Backends each speak a different
// channel (stderr JSONL, PagerDuty Events API v2, future Slack/email);
// adding a new channel is a new Backend implementation, not a change to
// the call sites.
//
// The shape of Alert intentionally mirrors the operator-plane
// alerting package in the owera-fleet repo so the operator mental model
// stays consistent across both planes — the two implementations are
// separate packages because the repos do not share Go modules, but the
// fields are the same.
//
// Design constraints (see compliance/runbooks/pagerduty-setup.md §5):
//
//   - A PagerDuty (or other remote) outage MUST NOT silence the local
//     audit-stream backend. Router.Fire invokes every accepting backend
//     and only returns the first error it saw — it does not short-circuit.
//   - PagerDuty pages only on Critical. Warning + Info go to the local
//     audit stream and (eventually) Slack, never to the pager.
//   - Every Backend is responsible for its own timeouts; the Router does
//     not impose one so a fast in-process backend isn't punished by a
//     slow remote one.
package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Severity classifies an alert's urgency. PagerDuty's Events API v2
// recognises "critical" / "error" / "warning" / "info"; we collapse
// "error" into Critical for V0 because every error-class alert the
// apiserver currently emits should page (drift, reconcile failures).
type Severity string

// Severity constants. Callers should reference these rather than
// constructing Severity values from string literals.
const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Alert is one fan-out-ready alert. The shape intentionally mirrors the
// operator-plane alerting package so the operator mental model stays
// consistent across both planes.
//
// Required fields: Severity, Title, Source. Body, Dedup, Labels, and
// OccurredAt are optional; Backends supply sensible defaults for any
// they need (PagerDutyBackend defaults OccurredAt to time.Now(), Dedup
// to Title).
type Alert struct {
	// Severity is one of the SeverityXxx constants. Backends use this
	// to decide whether they handle the alert via HandlesSeverity.
	Severity Severity

	// Title is the short summary shown in the pager UI. Required.
	Title string

	// Body is a longer human-readable description. Optional.
	Body string

	// Source identifies the alerting service (e.g. "owera-agentic-api").
	// Required because PagerDuty's Events API rejects payloads without it.
	Source string

	// Dedup is the PagerDuty dedup_key — multiple Fire() calls with the
	// same Dedup collapse into one open incident. Optional; backends
	// default to Title when empty.
	Dedup string

	// Labels carry structured context for the backend's custom_details
	// payload (e.g. tenant_id, drift_frac). Optional.
	Labels map[string]string

	// OccurredAt is the wall-clock time the alerted condition was
	// observed. Optional; defaults to time.Now() at Fire() time.
	OccurredAt time.Time
}

// Backend is one fan-out destination for alerts. Implementations:
//
//   - LogBackend (stderr JSONL — wired in apiserver/main.go as the
//     always-on baseline)
//   - PagerDutyBackend (POST to events.pagerduty.com)
//
// Future: SlackBackend, EmailBackend, OpsGenieBackend.
type Backend interface {
	// Name returns a short stable identifier used in boot-log
	// fingerprints (e.g. "log", "pagerduty"). Names are joined with
	// "+" in the fingerprint, so they should be lowercase + dash-free.
	Name() string

	// Send delivers the alert. Returning an error logs but does not
	// block fan-out to other Backends — the Router calls every Backend
	// regardless of earlier failures.
	Send(ctx context.Context, alert Alert) error

	// HandlesSeverity returns true when this Backend should be invoked
	// for the given Severity. PagerDutyBackend returns true only for
	// SeverityCritical; LogBackend returns true for all severities.
	HandlesSeverity(s Severity) bool
}

// Router fans an Alert out across every registered Backend that accepts
// its Severity.
//
// Router is safe for concurrent use after construction — AddBackend is
// not concurrency-safe and should be called only during wiring.
type Router struct {
	backends []Backend
}

// NewRouter returns an empty Router. Use AddBackend to register sinks
// before calling Fire.
func NewRouter() *Router {
	return &Router{}
}

// AddBackend appends b to the fan-out list. Order is preserved; the
// always-on LogBackend should be added first so a panic in a later
// backend cannot prevent the local audit emission.
func (r *Router) AddBackend(b Backend) {
	if b == nil {
		return
	}
	r.backends = append(r.backends, b)
}

// Backends returns the names of every registered backend in
// fan-out order. Used by apiserver/main.go to build the boot-log
// fingerprint (e.g. "log+pagerduty").
func (r *Router) Backends() []string {
	names := make([]string, 0, len(r.backends))
	for _, b := range r.backends {
		names = append(names, b.Name())
	}
	return names
}

// Fire dispatches alert to every Backend that accepts its Severity. All
// accepting Backends are invoked even if an earlier one errors — the
// goal is "never silence the local audit emission because PagerDuty is
// down." The first error encountered is returned; subsequent errors are
// dropped on the floor (callers needing per-backend errors should wrap
// their Backend in a logging decorator).
func (r *Router) Fire(ctx context.Context, alert Alert) error {
	var firstErr error
	for _, b := range r.backends {
		if !b.HandlesSeverity(alert.Severity) {
			continue
		}
		if err := b.Send(ctx, alert); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("alerting: backend %s: %w", b.Name(), err)
		}
	}
	return firstErr
}

// PagerDutyEndpoint is the production Events API v2 enqueue URL.
const PagerDutyEndpoint = "https://events.pagerduty.com/v2/enqueue"

// PagerDutyBackend POSTs alerts to the PagerDuty Events API v2.
//
// Construct via NewPagerDuty for the standard 30s-timeout HTTP client.
// The zero value is not usable (RoutingKey must be set).
type PagerDutyBackend struct {
	// RoutingKey is the 32-char hex integration key from PagerDuty's
	// Events API v2 service integration. Required.
	RoutingKey string

	// Endpoint is the enqueue URL. Defaults to PagerDutyEndpoint when
	// empty. Override only for tests (httptest server) or for a
	// PagerDuty EU-residency endpoint.
	Endpoint string

	// HTTPClient is the client used for the POST. Defaults to a
	// 30s-timeout client when nil — matching the shape used by
	// internal/audit/sigv4_transport.go.
	HTTPClient *http.Client
}

// NewPagerDuty returns a PagerDutyBackend with the production endpoint
// and a 30s-timeout HTTP client (matches the convention from
// internal/audit/sigv4_transport.go).
func NewPagerDuty(routingKey string) (*PagerDutyBackend, error) {
	if routingKey == "" {
		return nil, errors.New("alerting: PagerDuty routing key is empty")
	}
	return &PagerDutyBackend{
		RoutingKey: routingKey,
		Endpoint:   PagerDutyEndpoint,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Name returns "pagerduty".
func (*PagerDutyBackend) Name() string { return "pagerduty" }

// HandlesSeverity returns true only for SeverityCritical. Warning and
// Info alerts are intentionally not paged — they go to the local audit
// stream (and, in the future, Slack).
func (*PagerDutyBackend) HandlesSeverity(s Severity) bool {
	return s == SeverityCritical
}

// pagerDutyPayload is the inner "payload" object PagerDuty's Events API
// v2 expects. We declare it as a struct so the field set is reviewable
// and the JSON encoding is stable across Go versions.
type pagerDutyPayload struct {
	Summary       string            `json:"summary"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity"`
	Timestamp     string            `json:"timestamp,omitempty"`
	CustomDetails map[string]string `json:"custom_details,omitempty"`
}

// pagerDutyEnvelope is the top-level POST body.
type pagerDutyEnvelope struct {
	RoutingKey  string           `json:"routing_key"`
	EventAction string           `json:"event_action"`
	DedupKey    string           `json:"dedup_key,omitempty"`
	Payload     pagerDutyPayload `json:"payload"`
}

// Send POSTs the alert to PagerDuty Events API v2 as a "trigger" event.
// HTTP 202 is the success status per the PagerDuty docs; any other
// status is reported as an error so the Router can record it. Network
// failure is also reported — the Router will not silence the local
// log emission either way.
func (b *PagerDutyBackend) Send(ctx context.Context, alert Alert) error {
	if b.RoutingKey == "" {
		return errors.New("alerting: PagerDuty routing key is empty")
	}
	endpoint := b.Endpoint
	if endpoint == "" {
		endpoint = PagerDutyEndpoint
	}
	client := b.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	occurred := alert.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now()
	}
	dedup := alert.Dedup
	if dedup == "" {
		dedup = alert.Title
	}
	severity := string(alert.Severity)
	if severity == "" {
		severity = string(SeverityCritical)
	}

	env := pagerDutyEnvelope{
		RoutingKey:  b.RoutingKey,
		EventAction: "trigger",
		DedupKey:    dedup,
		Payload: pagerDutyPayload{
			Summary:       alert.Title,
			Source:        alert.Source,
			Severity:      severity,
			Timestamp:     occurred.UTC().Format(time.RFC3339),
			CustomDetails: alert.Labels,
		},
	}
	if alert.Body != "" {
		if env.Payload.CustomDetails == nil {
			env.Payload.CustomDetails = map[string]string{}
		}
		env.Payload.CustomDetails["body"] = alert.Body
	}

	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	// PagerDuty Events API v2 returns 202 Accepted on success.
	if resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("PagerDuty status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
