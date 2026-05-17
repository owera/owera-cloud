package status

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeOpTransport struct {
	method string
	result any
	err    error
	called bool
}

func (f *fakeOpTransport) Call(_ context.Context, method string, _ any, out any) error {
	f.called = true
	f.method = method
	if f.err != nil {
		return f.err
	}
	// Marshal-then-unmarshal so the caller's struct gets typed values
	// (mirrors what the JSON-RPC transport does in production).
	src, ok := f.result.(operatorHealthSnapshot)
	if !ok {
		return errors.New("test transport: unexpected result type")
	}
	*(out.(*operatorHealthSnapshot)) = src
	return nil
}

func TestOperatorFetcher_GreenState(t *testing.T) {
	tr := &fakeOpTransport{
		result: operatorHealthSnapshot{
			Ts:      "2026-05-17T20:00:00Z",
			Gateway: operatorGatewayHealth{OK: true, HermesVersion: "v0.13.0", UptimeSeconds: 3600},
			Workers: []operatorWorkerHealth{
				{Node: "claw1.local", OK: true, LastHeartbeatAgeSeconds: 4},
				{Node: "claw2.local", OK: true, LastHeartbeatAgeSeconds: 6},
			},
		},
	}
	f := NewOperatorFetcher(tr)
	snap, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !tr.called || tr.method != "fleet.HealthSnapshot" {
		t.Fatalf("transport not called with fleet.HealthSnapshot, got %q", tr.method)
	}
	if snap.Fleet.Total != 2 || snap.Fleet.Ready != 2 {
		t.Fatalf("Total/Ready mismatch: %+v", snap.Fleet)
	}
	if snap.Fleet.State != "green" {
		t.Fatalf("state want green, got %q", snap.Fleet.State)
	}
}

func TestOperatorFetcher_YellowAndRed(t *testing.T) {
	cases := []struct {
		name    string
		op      operatorHealthSnapshot
		wantSt  string
		wantOK  int
		wantAll int
	}{
		{
			name: "one_worker_down",
			op: operatorHealthSnapshot{
				Gateway: operatorGatewayHealth{OK: true},
				Workers: []operatorWorkerHealth{
					{Node: "claw1.local", OK: true},
					{Node: "claw2.local", OK: false},
				},
			},
			wantSt: "yellow", wantOK: 1, wantAll: 2,
		},
		{
			name: "gateway_down",
			op: operatorHealthSnapshot{
				Gateway: operatorGatewayHealth{OK: false},
				Workers: []operatorWorkerHealth{{Node: "claw1.local", OK: true}},
			},
			wantSt: "red", wantOK: 1, wantAll: 1,
		},
		{
			name:   "no_workers",
			op:     operatorHealthSnapshot{Gateway: operatorGatewayHealth{OK: true}},
			wantSt: "red", wantOK: 0, wantAll: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			snap := translate(c.op)
			if snap.Fleet.State != c.wantSt {
				t.Errorf("state want %q, got %q", c.wantSt, snap.Fleet.State)
			}
			if snap.Fleet.Ready != c.wantOK || snap.Fleet.Total != c.wantAll {
				t.Errorf("counts want %d/%d, got %d/%d",
					c.wantOK, c.wantAll, snap.Fleet.Ready, snap.Fleet.Total)
			}
		})
	}
}

func TestOperatorFetcher_SKUStates(t *testing.T) {
	op := operatorHealthSnapshot{
		Gateway: operatorGatewayHealth{OK: true},
		Workers: []operatorWorkerHealth{{Node: "claw1.local", OK: true}},
		SKUConformance: []operatorSKUConform{
			{SKU: "marketing.fanout", P50LatencyMs: 50, P95LatencyMs: 200, ErrorRatePct: 0.1, SLATargetMs: 500},     // green
			{SKU: "etl.local",        P50LatencyMs: 400, P95LatencyMs: 600, ErrorRatePct: 1.5, SLATargetMs: 500},   // yellow
			{SKU: "llm.triage",       P50LatencyMs: 800, P95LatencyMs: 1200, ErrorRatePct: 7, SLATargetMs: 500},    // red
		},
	}
	snap := translate(op)
	if len(snap.SLAs) != 3 {
		t.Fatalf("want 3 SLAs, got %d", len(snap.SLAs))
	}
	wantStates := []string{"green", "yellow", "red"}
	for i, want := range wantStates {
		if snap.SLAs[i].State != want {
			t.Errorf("sla[%d] %s: state want %q, got %q", i, snap.SLAs[i].SKU, want, snap.SLAs[i].State)
		}
	}
}

func TestService_NewWithFetcher_ReadyGreen(t *testing.T) {
	tr := &fakeOpTransport{
		result: operatorHealthSnapshot{
			Gateway: operatorGatewayHealth{OK: true},
			Workers: []operatorWorkerHealth{{Node: "claw1.local", OK: true}},
		},
	}
	svc := NewWithFetcher(NewOperatorFetcher(tr), 100*time.Millisecond)
	if !svc.Ready(context.Background()) {
		t.Fatal("ready want true, got false")
	}
}
