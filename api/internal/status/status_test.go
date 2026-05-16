package status

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeTransport struct {
	resp     Snapshot
	err      error
	calls    int
	respFunc func(method string) (Snapshot, error)
}

func (f *fakeTransport) Call(_ context.Context, method string, _ any, result any) error {
	f.calls++
	if f.respFunc != nil {
		s, err := f.respFunc(method)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(s)
		return json.Unmarshal(b, result)
	}
	if f.err != nil {
		return f.err
	}
	b, _ := json.Marshal(f.resp)
	return json.Unmarshal(b, result)
}

func TestGet_FetchesAndCaches(t *testing.T) {
	tr := &fakeTransport{
		resp: Snapshot{
			Fleet: FleetStatus{Total: 5, Ready: 5, State: "green"},
			SLAs:  []SKUSLAStatus{{SKU: "triage-watch@v1", ConformanceRate: 0.99, State: "green"}},
		},
	}
	s := New(tr, time.Minute)
	a, err := s.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if a.Fleet.State != "green" {
		t.Fatalf("state: got %q want green", a.Fleet.State)
	}
	// Second call hits cache.
	_, _ = s.Get(context.Background())
	if tr.calls != 1 {
		t.Fatalf("calls: got %d want 1 (cache)", tr.calls)
	}
}

func TestGet_TransportError(t *testing.T) {
	tr := &fakeTransport{err: errors.New("boom")}
	s := New(tr, time.Minute)
	if _, err := s.Get(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestReady_ReturnsFalseOnError(t *testing.T) {
	tr := &fakeTransport{err: errors.New("boom")}
	s := New(tr, time.Minute)
	if s.Ready(context.Background()) {
		t.Fatal("expected Ready=false")
	}
}

func TestReady_TrueOnGreen(t *testing.T) {
	tr := &fakeTransport{resp: Snapshot{Fleet: FleetStatus{State: "green"}}}
	s := New(tr, time.Minute)
	if !s.Ready(context.Background()) {
		t.Fatal("expected Ready=true")
	}
}

func TestNoTransport(t *testing.T) {
	s := New(nil, time.Minute)
	if _, err := s.Get(context.Background()); err == nil {
		t.Fatal("expected error for nil transport")
	}
}
