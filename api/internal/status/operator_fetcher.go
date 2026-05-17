// Package status — operator_fetcher.go translates the operator-plane
// `fleet.HealthSnapshot` JSON-RPC response (see
// owera-fleet/internal/rpc/healthsnapshot.go) into the cloud-side
// [Snapshot]. The two schemas don't share fields; decoding directly
// through dispatcher.Transport leaves Snapshot.Fleet.State as the zero
// value, which makes Service.Ready() permanently return false and
// /readyz return 503. This file is the translation seam.
package status

import (
	"context"
	"time"
)

// OperatorTransport mirrors dispatcher.Transport (re-declared so this
// package keeps its DAG-shaped imports). The production wiring passes a
// dispatcher.HTTPTransport here.
type OperatorTransport interface {
	Call(ctx context.Context, method string, params any, result any) error
}

// operatorHealthSnapshot mirrors the wire shape returned by the operator
// plane. Kept private — the public seam is [Snapshot].
type operatorHealthSnapshot struct {
	Ts             string                  `json:"ts"`
	Gateway        operatorGatewayHealth   `json:"gateway"`
	Workers        []operatorWorkerHealth  `json:"workers"`
	SKUConformance []operatorSKUConform    `json:"sku_conformance"`
}

type operatorGatewayHealth struct {
	OK            bool   `json:"ok"`
	HermesVersion string `json:"hermes_version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type operatorWorkerHealth struct {
	Node                    string  `json:"node"`
	OK                      bool    `json:"ok"`
	LastHeartbeatAgeSeconds float64 `json:"last_heartbeat_age_seconds"`
	HermesVersion           string  `json:"hermes_version"`
}

type operatorSKUConform struct {
	SKU          string  `json:"sku"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	ErrorRatePct float64 `json:"error_rate_pct"`
	SLATargetMs  int64   `json:"sla_target_ms"`
}

// OperatorFetcher implements [Fetcher] against the operator-plane
// JSON-RPC endpoint. It calls fleet.HealthSnapshot and translates.
type OperatorFetcher struct {
	transport OperatorTransport
}

// NewOperatorFetcher wires a Fetcher to transport (typically a
// dispatcher.HTTPTransport).
func NewOperatorFetcher(transport OperatorTransport) *OperatorFetcher {
	return &OperatorFetcher{transport: transport}
}

// Fetch implements [Fetcher].
func (f *OperatorFetcher) Fetch(ctx context.Context) (*Snapshot, error) {
	var op operatorHealthSnapshot
	if err := f.transport.Call(ctx, "fleet.HealthSnapshot", nil, &op); err != nil {
		return nil, err
	}
	return translate(op), nil
}

// translate maps the operator schema into the cloud Snapshot.
//
// Fleet state policy:
//   - "red"    : gateway not OK, or no workers at all
//   - "yellow" : gateway OK, ≥1 worker but not all workers OK
//   - "green"  : gateway OK and ≥1 worker, all workers OK
//
// SKU state policy (per-SKU): the operator returns ErrorRatePct +
// P95LatencyMs vs SLATargetMs. We flip:
//   - "red"    : error_rate_pct ≥ 5 or P95 ≥ 2×SLA
//   - "yellow" : error_rate_pct ≥ 1 or P95 ≥ SLA
//   - "green"  : everything within target
func translate(op operatorHealthSnapshot) *Snapshot {
	total := len(op.Workers)
	ready := 0
	for _, w := range op.Workers {
		if w.OK {
			ready++
		}
	}
	state := "red"
	switch {
	case !op.Gateway.OK || total == 0:
		state = "red"
	case ready == total:
		state = "green"
	default:
		state = "yellow"
	}

	slas := make([]SKUSLAStatus, 0, len(op.SKUConformance))
	for _, c := range op.SKUConformance {
		conformance := 1.0 - c.ErrorRatePct/100.0
		if conformance < 0 {
			conformance = 0
		}
		skuState := "green"
		switch {
		case c.ErrorRatePct >= 5 || (c.SLATargetMs > 0 && c.P95LatencyMs >= 2*float64(c.SLATargetMs)):
			skuState = "red"
		case c.ErrorRatePct >= 1 || (c.SLATargetMs > 0 && c.P95LatencyMs >= float64(c.SLATargetMs)):
			skuState = "yellow"
		}
		slas = append(slas, SKUSLAStatus{
			SKU:             c.SKU,
			WindowMinutes:   5,
			ConformanceRate: conformance,
			State:           skuState,
		})
	}

	generatedAt, _ := time.Parse(time.RFC3339Nano, op.Ts)
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	return &Snapshot{
		GeneratedAt: generatedAt,
		Fleet: FleetStatus{
			Total: total,
			Ready: ready,
			State: state,
		},
		SLAs: slas,
	}
}
