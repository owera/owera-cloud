// Package dispatcher — http_transport.go is the production Transport
// implementation. It POSTs JSON-RPC 2.0 envelopes to the operator-plane
// endpoint exposed by the Cloudflare tunnel (e.g.
// https://internal-rpc.owera.com/), so fleet.SubmitJob / fleet.CancelTask
// and fleet.HealthSnapshot reach the real `fleetctl serve` on the
// gateway Mac.
//
// The shape mirrors LedgerTailClient's request/response envelopes; we
// keep them in a separate file because the dispatcher transport is
// generic over `method` and `result`, while LedgerTailClient is typed
// against fleet.LedgerTail.
package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPTransport is the production [Transport]. It serializes a JSON-RPC
// 2.0 request to endpoint and decodes the result into the caller's
// struct.
type HTTPTransport struct {
	endpoint string
	http     *http.Client
}

// NewHTTPTransport returns a transport pointed at the operator-plane
// JSON-RPC endpoint (the full tunnel URL). hc may be nil for the default
// 10s timeout — generous because cold-cache fleet.HealthSnapshot pulls
// the worker list, hermes versions, and SKU conformance metrics.
func NewHTTPTransport(endpoint string, hc *http.Client) *HTTPTransport {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPTransport{endpoint: endpoint, http: hc}
}

// Call implements [Transport]. method, params, and result follow the
// JSON-RPC 2.0 envelope; the result struct is decoded only when the
// server returns a non-error envelope.
func (t *HTTPTransport) Call(ctx context.Context, method string, params any, result any) error {
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})
	if err != nil {
		return fmt.Errorf("http-transport: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", t.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http-transport: req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return fmt.Errorf("http-transport: do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("http-transport: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http-transport: http %d: %s", resp.StatusCode, raw)
	}

	var env rpcResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("http-transport: decode envelope: %w", err)
	}
	if env.Error != nil {
		return fmt.Errorf("http-transport: rpc %d: %s", env.Error.Code, env.Error.Message)
	}
	if result == nil || len(env.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Result, result); err != nil {
		return fmt.Errorf("http-transport: decode result: %w", err)
	}
	return nil
}
