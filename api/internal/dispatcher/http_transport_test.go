package dispatcher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPTransport_SubmitJobRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("server: decode: %v", err)
		}
		if req.JSONRPC != "2.0" || req.Method != "fleet.SubmitJob" {
			t.Errorf("server: bad envelope %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":{"task_id":"task_abc"}}`))
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.URL, &http.Client{Timeout: 2 * time.Second})
	var resp struct {
		TaskID string `json:"task_id"`
	}
	if err := tr.Call(context.Background(), "fleet.SubmitJob", map[string]any{"tenant_id": "t1"}, &resp); err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.TaskID != "task_abc" {
		t.Fatalf("task_id want task_abc, got %q", resp.TaskID)
	}
}

func TestHTTPTransport_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32601,"message":"method not found"}}`))
	}))
	defer srv.Close()

	tr := NewHTTPTransport(srv.URL, nil)
	err := tr.Call(context.Background(), "fleet.Unknown", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("expected rpc error, got %v", err)
	}
}
