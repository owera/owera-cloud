// Package status produces the fleet/SLA health snapshot consumed by the
// public status page. The snapshot is pulled from the operator plane via
// the same Transport interface the dispatcher uses; the operator plane
// exposes a "fleet.HealthSnapshot" method that returns the current
// readiness state of each worker plus per-SKU SLA conformance.
package status

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Transport mirrors dispatcher.Transport. We re-declare here rather than
// importing to keep the package import graph a DAG (dispatcher and status
// are peers, both wrapping the same underlying tunnel).
type Transport interface {
	Call(ctx context.Context, method string, params any, result any) error
}

// Snapshot is the published health view.
type Snapshot struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Fleet       FleetStatus    `json:"fleet"`
	SLAs        []SKUSLAStatus `json:"slas"`
}

// FleetStatus captures the operator-plane fleet count + readiness.
type FleetStatus struct {
	Total int    `json:"total"`
	Ready int    `json:"ready"`
	State string `json:"state"` // "green", "yellow", "red"
}

// SKUSLAStatus is per-SKU conformance.
type SKUSLAStatus struct {
	SKU             string  `json:"sku"`
	WindowMinutes   int     `json:"window_minutes"`
	ConformanceRate float64 `json:"conformance_rate"` // 0.0-1.0
	State           string  `json:"state"`
}

// Service is the status snapshot generator.
type Service struct {
	transport Transport
	mu        sync.RWMutex
	last      *Snapshot
	lastAt    time.Time
	cacheFor  time.Duration
}

// New returns a Service that fetches snapshots from the operator plane via
// transport, with cacheFor in-memory caching (recommended: 30s).
func New(transport Transport, cacheFor time.Duration) *Service {
	if cacheFor <= 0 {
		cacheFor = 30 * time.Second
	}
	return &Service{transport: transport, cacheFor: cacheFor}
}

// Get returns the current snapshot, refreshing if the cache is stale.
func (s *Service) Get(ctx context.Context) (*Snapshot, error) {
	s.mu.RLock()
	if s.last != nil && time.Since(s.lastAt) < s.cacheFor {
		out := *s.last
		s.mu.RUnlock()
		return &out, nil
	}
	s.mu.RUnlock()

	if s.transport == nil {
		return nil, errors.New("status: no transport configured")
	}
	var snap Snapshot
	if err := s.transport.Call(ctx, "fleet.HealthSnapshot", nil, &snap); err != nil {
		return nil, fmt.Errorf("status: fetch: %w", err)
	}
	if snap.GeneratedAt.IsZero() {
		snap.GeneratedAt = time.Now().UTC()
	}
	s.mu.Lock()
	s.last = &snap
	s.lastAt = time.Now()
	s.mu.Unlock()
	return &snap, nil
}

// Ready returns true if the most recent snapshot reports the fleet as
// green. The transport is consulted if no cache exists. Used by /readyz.
func (s *Service) Ready(ctx context.Context) bool {
	snap, err := s.Get(ctx)
	if err != nil {
		return false
	}
	return snap.Fleet.State == "green"
}
