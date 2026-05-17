package dispatcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/owera/owera-cloud/api/internal/jobs"
)

// mockRPCServer is a single-method JSON-RPC server that returns canned
// LedgerTail results based on the (taskID, cursor) pair.
type mockRPCServer struct {
	mu      sync.Mutex
	calls   int
	cursors []string // one entry per call, in order, for assertion
	respond func(int, string) ledgerTailResult
}

func newMockRPCServer(t *testing.T, respond func(callIndex int, cursor string) ledgerTailResult) *httptest.Server {
	t.Helper()
	mock := &mockRPCServer{respond: respond}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var p ledgerTailParams
		raw, _ := json.Marshal(env.Params)
		if err := json.Unmarshal(raw, &p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		mock.mu.Lock()
		idx := mock.calls
		mock.calls++
		mock.cursors = append(mock.cursors, p.AfterTs)
		respond := mock.respond
		mock.mu.Unlock()

		res := respond(idx, p.AfterTs)
		result, _ := json.Marshal(res)
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			Result:  result,
		})
	}))
}

// TestLedgerTailClient_NonTerminalThenComplete: first poll returns one
// non-terminal entry (Phase="step"); second poll returns the terminal
// entry with outputs. Client must echo the cursor on the second call.
func TestLedgerTailClient_NonTerminalThenComplete(t *testing.T) {
	t0 := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Second)
	t2 := t0.Add(2 * time.Second)

	srv := newMockRPCServer(t, func(call int, cursor string) ledgerTailResult {
		switch call {
		case 0:
			return ledgerTailResult{
				Entries: []ledgerEntry{
					{Ts: t0, TaskID: "tid", Phase: "step", Action: "begin", Result: "ok"},
					{Ts: t1, TaskID: "tid", Phase: "step", Action: "halfway", Result: "ok"},
				},
				Cursor: t1.Format(time.RFC3339Nano),
			}
		case 1:
			data, _ := json.Marshal(map[string]any{"outputs": map[string]any{"tickets_handled": 5}})
			return ledgerTailResult{
				Entries: []ledgerEntry{
					{Ts: t2, TaskID: "tid", Phase: "complete", Action: "done", Result: "bill",
						Data: data},
				},
				Cursor: t2.Format(time.RFC3339Nano),
			}
		}
		t.Fatalf("unexpected call index %d", call)
		return ledgerTailResult{}
	})
	t.Cleanup(srv.Close)

	c := NewLedgerTailClient(srv.URL, nil)

	// First poll — non-terminal.
	term, status, outputs, errMsg, err := c.Poll(context.Background(), "tid")
	if err != nil {
		t.Fatalf("Poll 1: %v", err)
	}
	if term {
		t.Errorf("call 1: expected non-terminal; got status=%q", status)
	}
	if outputs != nil {
		t.Errorf("call 1: expected nil outputs, got %v", outputs)
	}
	if c.getCursor("tid") != t1.Format(time.RFC3339Nano) {
		t.Errorf("cursor after call 1: got %q want %q", c.getCursor("tid"), t1.Format(time.RFC3339Nano))
	}

	// Second poll — terminal + outputs.
	term, status, outputs, errMsg, err = c.Poll(context.Background(), "tid")
	if err != nil {
		t.Fatalf("Poll 2: %v", err)
	}
	if !term {
		t.Errorf("call 2: expected terminal")
	}
	if status != jobs.StatusSucceeded {
		t.Errorf("status: got %q want %q", status, jobs.StatusSucceeded)
	}
	if outputs["tickets_handled"].(float64) != 5 {
		t.Errorf("outputs: got %v want tickets_handled=5", outputs)
	}
	if errMsg != "" {
		t.Errorf("errMsg: got %q want empty", errMsg)
	}
}

// TestLedgerTailClient_FailedPhase: terminal entry with Phase="failed"
// and an error message in Data. Status maps to StatusFailed.
func TestLedgerTailClient_FailedPhase(t *testing.T) {
	tnow := time.Now().UTC()
	srv := newMockRPCServer(t, func(call int, cursor string) ledgerTailResult {
		data, _ := json.Marshal(map[string]any{"error": "ssh: connection refused"})
		return ledgerTailResult{
			Entries: []ledgerEntry{
				{Ts: tnow, TaskID: "tid", Phase: "failed", Action: "delegate", Result: "error",
					Data: data},
			},
			Cursor: tnow.Format(time.RFC3339Nano),
		}
	})
	t.Cleanup(srv.Close)

	c := NewLedgerTailClient(srv.URL, nil)
	term, status, _, errMsg, err := c.Poll(context.Background(), "tid")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if !term {
		t.Fatal("expected terminal")
	}
	if status != jobs.StatusFailed {
		t.Errorf("status: got %q want %q", status, jobs.StatusFailed)
	}
	if errMsg != "ssh: connection refused" {
		t.Errorf("errMsg: got %q", errMsg)
	}
}

// TestLedgerTailClient_RPCError: server returns a JSON-RPC error
// envelope. Client surfaces it as err.
func TestLedgerTailClient_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32602, Message: "bad task_id"},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewLedgerTailClient(srv.URL, nil)
	_, _, _, _, err := c.Poll(context.Background(), "tid")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestLedgerTailClient_EmptyResultNotTerminal: server returns no
// entries (the SubmitJob race). Client returns terminal=false, no err.
func TestLedgerTailClient_EmptyResultNotTerminal(t *testing.T) {
	srv := newMockRPCServer(t, func(call int, cursor string) ledgerTailResult {
		return ledgerTailResult{Entries: []ledgerEntry{}, Cursor: cursor}
	})
	t.Cleanup(srv.Close)

	c := NewLedgerTailClient(srv.URL, nil)
	term, _, _, _, err := c.Poll(context.Background(), "tid")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if term {
		t.Errorf("expected non-terminal on empty result")
	}
}
