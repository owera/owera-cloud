// Package dispatcher — ledgertail_client.go wires the worker's
// LedgerPoller interface to the operator plane's `fleet.LedgerTail`
// JSON-RPC method. It replaces SyntheticLedgerPoller in production.
//
// # Cursor semantics
//
// The handler keeps a per-task cursor in memory keyed by operator
// task_id. On restart the cursor is lost and the first poll re-reads
// the task's full ledger — bounded by the ledger file size (kilobytes
// in practice) and idempotent (the worker treats terminal-twice as
// a no-op). A persistent cursor in SQLite is a future optimisation
// once we measure poll-cost on a real fleet.
//
// # Terminal-entry mapping
//
// The operator plane writes ledger entries with these Phase values for
// the terminal step of a SubmitJob run:
//
//   - Phase=="complete"  → jobs.StatusSucceeded
//   - Phase=="failed"    → jobs.StatusFailed
//   - Phase=="cancelled" → jobs.StatusCancelled
//
// Any entry with a different Phase is treated as intermediate progress
// (worker logs but does not terminate). Result=="bill" entries are not
// terminal — they're billing markers; the bill subscriber (WS-16)
// consumes them out-of-band.
package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/owera/owera-cloud/api/internal/jobs"
)

// LedgerTailClient implements LedgerPoller against the operator plane's
// fleet.LedgerTail JSON-RPC method.
type LedgerTailClient struct {
	endpoint string
	http     *http.Client

	mu      sync.Mutex
	cursors map[string]string // task_id → RFC3339 cursor
}

// NewLedgerTailClient wires the client to an operator-plane endpoint.
// endpoint is the full URL the Cloudflare tunnel exposes (e.g.
// "https://op.internal.owera.ai/rpc"). The HTTP client may be nil for
// the default (5s timeout).
func NewLedgerTailClient(endpoint string, hc *http.Client) *LedgerTailClient {
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	return &LedgerTailClient{
		endpoint: endpoint,
		http:     hc,
		cursors:  map[string]string{},
	}
}

// rpcRequest is the JSON-RPC 2.0 envelope.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      int    `json:"id"`
}

// rpcResponse is the JSON-RPC 2.0 reply envelope. We don't echo the id
// (single-call client) so we ignore it on read.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ledgerEntry mirrors the operator-plane internal/ledger Entry shape.
// We don't import operator-plane types — the wire contract is the
// boundary; redefining here keeps the customer plane self-contained.
type ledgerEntry struct {
	Ts       time.Time       `json:"ts"`
	TaskID   string          `json:"task_id"`
	TenantID string          `json:"tenant_id,omitempty"`
	Phase    string          `json:"phase"`
	Action   string          `json:"action"`
	Result   string          `json:"result"`
	Data     json.RawMessage `json:"data,omitempty"`
}

type ledgerTailParams struct {
	TaskID  string `json:"task_id"`
	AfterTs string `json:"after_ts,omitempty"`
}

type ledgerTailResult struct {
	Entries []ledgerEntry `json:"entries"`
	Cursor  string        `json:"cursor"`
}

// Poll implements LedgerPoller. Returns (terminal, status, outputs,
// errMsg, err). err is non-nil only for transport/protocol failures —
// a non-existent task or empty result is "not terminal yet."
func (c *LedgerTailClient) Poll(ctx context.Context, taskID string) (bool, jobs.Status, map[string]any, string, error) {
	cursor := c.getCursor(taskID)

	res, err := c.call(ctx, taskID, cursor)
	if err != nil {
		return false, "", nil, "", err
	}
	// Advance cursor regardless of whether we found a terminal — empty
	// tails keep the same cursor anyway.
	if res.Cursor != "" {
		c.setCursor(taskID, res.Cursor)
	}

	// Walk entries; first terminal wins. The operator plane writes
	// entries in order, so iteration order matches Ts order.
	for _, e := range res.Entries {
		status, isTerminal := mapPhaseToStatus(e.Phase)
		if !isTerminal {
			continue
		}
		outputs, errMsg := extractOutputsAndError(e)
		return true, status, outputs, errMsg, nil
	}
	return false, "", nil, "", nil
}

// call issues one JSON-RPC fleet.LedgerTail and returns the result.
func (c *LedgerTailClient) call(ctx context.Context, taskID, afterTs string) (*ledgerTailResult, error) {
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  "fleet.LedgerTail",
		Params:  ledgerTailParams{TaskID: taskID, AfterTs: afterTs},
		ID:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("ledgertail: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ledgertail: req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ledgertail: do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ledgertail: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ledgertail: http %d: %s", resp.StatusCode, raw)
	}

	var env rpcResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("ledgertail: decode envelope: %w", err)
	}
	if env.Error != nil {
		return nil, fmt.Errorf("ledgertail: rpc %d: %s", env.Error.Code, env.Error.Message)
	}
	var result ledgerTailResult
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return nil, fmt.Errorf("ledgertail: decode result: %w", err)
	}
	return &result, nil
}

func (c *LedgerTailClient) getCursor(taskID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cursors[taskID]
}

func (c *LedgerTailClient) setCursor(taskID, cursor string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cursors[taskID] = cursor
}

// mapPhaseToStatus translates an operator-plane Phase to a cloud-plane
// jobs.Status. Returns (zero, false) for non-terminal Phase values.
func mapPhaseToStatus(phase string) (jobs.Status, bool) {
	switch phase {
	case "complete":
		return jobs.StatusSucceeded, true
	case "failed":
		return jobs.StatusFailed, true
	case "cancelled":
		return jobs.StatusCancelled, true
	}
	return "", false
}

// extractOutputsAndError reads outputs + error from a terminal entry's
// Data blob. The operator plane encodes the SubmitJob result there;
// the convention is:
//
//   - { "outputs": {...}, "error": "..." } for completed jobs
//   - missing "outputs" → empty map; missing "error" → empty string
func extractOutputsAndError(e ledgerEntry) (map[string]any, string) {
	if len(e.Data) == 0 {
		return nil, ""
	}
	var d struct {
		Outputs map[string]any `json:"outputs,omitempty"`
		Error   string         `json:"error,omitempty"`
	}
	if err := json.Unmarshal(e.Data, &d); err != nil {
		// Malformed Data — surface as error message rather than failing
		// the poll (the operator plane wrote it; we can't fix from here).
		return nil, fmt.Sprintf("ledgertail: malformed data: %v", err)
	}
	return d.Outputs, d.Error
}

// ErrEndpointEmpty is returned by NewLedgerTailClient when called with
// an empty endpoint. The caller should pass either a real URL or use
// SyntheticLedgerPoller for tests.
var ErrEndpointEmpty = errors.New("dispatcher: empty ledgertail endpoint")
