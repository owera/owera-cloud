// Package catalog defines the global SKU registry. Each SKU is one Go
// file under this package that declares its Name/Version/Category, JSON
// schema for inputs, pricing, SLA, dispatcher function, and billing meter.
// The catalog itself is global and not tenant-scoped — every tenant sees
// the same SKU list, but only authenticated tenants can submit jobs
// against them.
//
// SKUs register themselves via [Register] at init() time. New SKUs are
// added by dropping a new .go file in this package and calling Register
// from its init().
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// PricingTier describes how a SKU is billed.
//
// Model is one of the canonical lower-snake-case values below. The
// catalog [Dispatcher] reads Model at billing-reconcile time and routes
// the event to the matching billing.DispatchKind:
//
//	Model                   Meaning                                              Stripe shape                            Billing dispatch
//	---------------------   --------------------------------------------------   -------------------------------------   ----------------
//	"metered"               Pure usage-based, no base fee.                       recurring + meter                       EmitUsage
//	"monthly_subscription"  Base monthly fee with optional metered overage.      recurring (licensed base + metered)     EmitUsage on overage events
//	"per_job_fixed"         Each unit of work is a discrete billable event.      one_time                                EmitOneShot
//
//	BaseCents: the recurring base price (0 for purely metered or per-job SKUs).
//	OverageRule: the per-unit billing rule applied on top of base, e.g.
//	             "ticket", "campaign", "minute". For per_job_fixed SKUs this
//	             names the unit of work even though there is no base+overage
//	             relationship.
type PricingTier struct {
	Model       string `json:"model"`
	BaseCents   int64  `json:"base_cents"`
	OverageRule string `json:"overage_rule,omitempty"`
}

// SLA captures the customer-facing service guarantee. Owera's commercial
// promise is paired to the operator plane's actual measured latency.
type SLA struct {
	Description       string `json:"description"`
	MaxLatencySeconds int    `json:"max_latency_seconds"`
}

// DispatcherFn produces the operator-plane RPC payload (params) for a job.
// The actual transport happens in internal/dispatcher; this function is
// pure: given (ctx, jobID, validated inputs) it returns the params blob
// the operator plane expects on its JSON-RPC endpoint.
type DispatcherFn func(ctx context.Context, jobID string, inputs map[string]any) (any, error)

// SKU is the registry record for one billable agent capability.
type SKU struct {
	Name         string
	Version      string
	Category     string
	InputsSchema string // JSON Schema document
	Pricing      PricingTier
	SLA          SLA
	Dispatcher   DispatcherFn
	BillingMeter string

	once     sync.Once
	compiled *jsonschema.Schema
	compErr  error
}

// FullName returns "name@version" — the externally visible identifier.
func (s *SKU) FullName() string { return s.Name + "@" + s.Version }

// Schema returns the compiled JSON Schema, compiling on first call. The
// compile is memoized.
func (s *SKU) Schema() (*jsonschema.Schema, error) {
	s.once.Do(func() {
		c := jsonschema.NewCompiler()
		if err := c.AddResource(s.FullName(), strings.NewReader(s.InputsSchema)); err != nil {
			s.compErr = fmt.Errorf("catalog: add schema %s: %w", s.FullName(), err)
			return
		}
		sch, err := c.Compile(s.FullName())
		if err != nil {
			s.compErr = fmt.Errorf("catalog: compile schema %s: %w", s.FullName(), err)
			return
		}
		s.compiled = sch
	})
	return s.compiled, s.compErr
}

// ValidateInputs runs the JSON Schema over the inputs map.
func (s *SKU) ValidateInputs(inputs map[string]any) error {
	sch, err := s.Schema()
	if err != nil {
		return err
	}
	// Round-trip through JSON to normalize numeric types (jsonschema/v5
	// expects float64 for numbers, which is what json.Unmarshal produces).
	raw, err := json.Marshal(inputs)
	if err != nil {
		return fmt.Errorf("catalog: marshal inputs: %w", err)
	}
	var norm any
	if err := json.Unmarshal(raw, &norm); err != nil {
		return fmt.Errorf("catalog: normalize inputs: %w", err)
	}
	if err := sch.Validate(norm); err != nil {
		return fmt.Errorf("catalog: invalid inputs for %s: %w", s.FullName(), err)
	}
	return nil
}

// registry is the package-global SKU map. Mutated only at init() time via
// Register; reads are unsynchronized after server startup completes.
var (
	regMu    sync.RWMutex
	registry = map[string]*SKU{}
)

// Register adds a SKU to the global registry. Intended to be called from
// init() in each SKU's own .go file. Panics on duplicate FullName so
// missing-version-bump bugs surface loudly at startup.
func Register(s *SKU) {
	if s == nil {
		panic("catalog: Register(nil)")
	}
	if s.Name == "" || s.Version == "" {
		panic("catalog: SKU missing name or version")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[s.FullName()]; dup {
		panic("catalog: duplicate SKU " + s.FullName())
	}
	registry[s.FullName()] = s
}

// Lookup returns the SKU with the given full name (name@version). If
// version is omitted the latest registered version is returned (lexicographic).
func Lookup(fullName string) (*SKU, error) {
	regMu.RLock()
	defer regMu.RUnlock()
	if s, ok := registry[fullName]; ok {
		return s, nil
	}
	// Allow name-only lookup → latest version.
	if !strings.Contains(fullName, "@") {
		var latest *SKU
		for _, s := range registry {
			if s.Name == fullName {
				if latest == nil || s.Version > latest.Version {
					latest = s
				}
			}
		}
		if latest != nil {
			return latest, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, fullName)
}

// List returns every registered SKU in deterministic order.
func List() []*SKU {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]*SKU, 0, len(registry))
	for _, s := range registry {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName() < out[j].FullName() })
	return out
}

// ErrNotFound is returned by [Lookup] when the SKU is not registered.
var ErrNotFound = errors.New("catalog: sku not found")

// Reset clears the package-level registry. Intended for tests that need
// an isolated registry; production code must never call it.
func Reset() {
	regMu.Lock()
	defer regMu.Unlock()
	registry = map[string]*SKU{}
}
