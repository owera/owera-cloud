// Package dispatcher translates a (tenant_id, sku, inputs) tuple into an
// operator-plane JSON-RPC call over the private Cloudflare tunnel. The
// transport is abstracted behind [Transport] so tests run without any
// real network.
//
// Operator-plane contract (the seam we expect the operator plane to
// implement and that this package calls into):
//
//	Method: "fleet.SubmitJob"
//	Params: { "tenant_id": string, "sku": string, "inputs": object,
//	          "cloud_job_id": string }
//	Result: { "task_id": string }   // ledger task-id
//
// Method: "fleet.CancelTask"
//
//	Params: { "task_id": string }
//	Result: { "cancelled": bool }
//
// The result `task_id` is recorded on the customer-plane job so that
// subsequent ledger events streamed back over the tunnel can be matched
// to the originating customer job.
package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/owera/owera-cloud/api/internal/catalog"
)

// Transport is the operator-plane RPC surface. The production implementation
// targets the private Cloudflare tunnel; the fake [InMemoryTransport] is
// used in tests.
type Transport interface {
	Call(ctx context.Context, method string, params any, result any) error
}

// Dispatcher translates customer jobs into operator-plane RPCs.
type Dispatcher struct {
	transport Transport
}

// New returns a Dispatcher using transport.
func New(transport Transport) *Dispatcher {
	return &Dispatcher{transport: transport}
}

// Dispatch validates inputs against the SKU schema, shapes the operator-
// plane payload via the SKU's Dispatcher func, and calls fleet.SubmitJob.
// Returns the operator-plane task id (ledger task id) on success.
func (d *Dispatcher) Dispatch(ctx context.Context, tenantID, jobID, sku string, inputs map[string]any) (string, error) {
	if d.transport == nil {
		return "", errors.New("dispatcher: no transport configured")
	}
	skuRec, err := catalog.Lookup(sku)
	if err != nil {
		return "", fmt.Errorf("dispatcher: %w", err)
	}
	if err := skuRec.ValidateInputs(inputs); err != nil {
		return "", err
	}
	params, err := skuRec.Dispatcher(ctx, jobID, inputs)
	if err != nil {
		return "", fmt.Errorf("dispatcher: build payload: %w", err)
	}
	// Wrap with the operator-plane envelope so the operator plane has a
	// consistent shape across SKUs.
	envelope := map[string]any{
		"tenant_id":    tenantID,
		"sku":          skuRec.FullName(),
		"cloud_job_id": jobID,
		"inputs":       params,
	}
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := d.transport.Call(ctx, "fleet.SubmitJob", envelope, &resp); err != nil {
		return "", fmt.Errorf("dispatcher: transport: %w", err)
	}
	if resp.TaskID == "" {
		return "", errors.New("dispatcher: operator plane returned empty task_id")
	}
	return resp.TaskID, nil
}

// Cancel asks the operator plane to abort the task associated with a
// previously-dispatched job.
func (d *Dispatcher) Cancel(ctx context.Context, operatorTaskID string) error {
	if d.transport == nil {
		return errors.New("dispatcher: no transport configured")
	}
	if operatorTaskID == "" {
		return errors.New("dispatcher: empty operator task id")
	}
	var resp struct {
		Cancelled bool `json:"cancelled"`
	}
	if err := d.transport.Call(ctx, "fleet.CancelTask",
		map[string]any{"task_id": operatorTaskID}, &resp); err != nil {
		return fmt.Errorf("dispatcher: cancel transport: %w", err)
	}
	if !resp.Cancelled {
		return errors.New("dispatcher: operator plane rejected cancel")
	}
	return nil
}

// InMemoryTransport is a fake Transport for tests. It records every call
// and responds via the configured Responder.
type InMemoryTransport struct {
	mu        sync.Mutex
	Calls     []Call
	Responder func(method string, params any) (any, error)
}

// Call is one recorded RPC.
type Call struct {
	Method string
	Params any
}

// NewInMemoryTransport returns a fake with a default responder that
// returns a synthetic task_id for fleet.SubmitJob and {Cancelled: true}
// for fleet.CancelTask.
func NewInMemoryTransport() *InMemoryTransport {
	return &InMemoryTransport{
		Responder: func(method string, _ any) (any, error) {
			switch method {
			case "fleet.SubmitJob":
				return map[string]any{"task_id": "task_synthetic"}, nil
			case "fleet.CancelTask":
				return map[string]any{"cancelled": true}, nil
			}
			return nil, fmt.Errorf("unmocked method %s", method)
		},
	}
}

// Call records the invocation and routes to the Responder.
func (t *InMemoryTransport) Call(_ context.Context, method string, params any, result any) error {
	t.mu.Lock()
	t.Calls = append(t.Calls, Call{Method: method, Params: params})
	resp := t.Responder
	t.mu.Unlock()

	got, err := resp(method, params)
	if err != nil {
		return err
	}
	return decodeInto(got, result)
}

// CallCount returns how many times Call has been invoked.
func (t *InMemoryTransport) CallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.Calls)
}

// decodeInto unmarshals from a map[string]any-shaped response into the
// caller's result struct. We use JSON for one shape-normalization path.
func decodeInto(src, dst any) error {
	if dst == nil {
		return nil
	}
	m, ok := src.(map[string]any)
	if !ok {
		return fmt.Errorf("dispatcher: response not a map: %T", src)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
