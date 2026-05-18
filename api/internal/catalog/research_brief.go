package catalog

import "context"

// ResearchBriefV1 is the V1 research-and-intelligence SKU that produces a
// deep web-research brief on a topic — competitive intel, market sizing,
// due diligence — with citations and a structured rendered report.
//
// Pricing model: per-job fixed with S/M/L tiers selected by `depth`. The
// operator-plane router records the chosen tier letter as the BillEvent
// Meter (see campaign-swarm@v1 for the same shape); the catalog
// Dispatcher composes the StripeRef key as "research-brief:<Meter>" at
// reconcile time. BaseCents on the registered PricingTier is therefore
// 0 — the real per-tier price lives in [billing.StripeRefs] once the
// operator provisions live-mode Stripe products. Placeholder per-tier
// figures (operator-finalization-pending):
//   - S: $199 (light, ≤4 h delivery)
//   - M: $499 (default, ≤12 h delivery)
//   - L: $999 (deep, ≤24 h delivery)
//
// SLA: ≤24 h delivery for the deepest tier; lighter tiers finish sooner.
//
// Wave 11 follow-up: register the three "research-brief:{S,M,L}" entries
// in api/internal/billing/stripe_ids.go once the Stripe products land.
// Until then the reconciler will skip-and-continue (PR #41) on jobs
// submitted against this SKU.
var ResearchBriefV1 = &SKU{
	Name:         "research-brief",
	Version:      "v1",
	Category:     "research-intelligence",
	InputsSchema: researchBriefSchema,
	Pricing: PricingTier{
		Model:       "per_job_fixed",
		BaseCents:   0,
		OverageRule: "brief",
	},
	SLA: SLA{
		Description:       "≤24h delivery (L tier); ≤12h M; ≤4h S",
		MaxLatencySeconds: 86400,
	},
	Dispatcher:   dispatchResearchBrief,
	BillingMeter: "briefs_delivered",
}

const researchBriefSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["topic"],
  "properties": {
    "topic": { "type": "string", "minLength": 5 },
    "depth": { "type": "string", "enum": ["S", "M", "L"], "default": "M" },
    "citations_required": { "type": "boolean", "default": true }
  },
  "additionalProperties": false
}`

// dispatchResearchBrief builds the operator-plane RPC params blob. The
// real long-form research happens via Hermes web/browser toolsets on the
// operator plane; this function just shapes the validated inputs so the
// router can pick them up. The dispatcher does NOT pre-resolve the tier
// price — that mapping lives in StripeRefs and is consulted at billing
// reconcile time.
func dispatchResearchBrief(_ context.Context, jobID string, inputs map[string]any) (any, error) {
	depth := "M"
	if v, ok := inputs["depth"]; ok {
		if s, ok := v.(string); ok && s != "" {
			depth = s
		}
	}
	citationsRequired := true
	if v, ok := inputs["citations_required"]; ok {
		if b, ok := v.(bool); ok {
			citationsRequired = b
		}
	}
	return map[string]any{
		"sku":                "research-brief@v1",
		"job_id":             jobID,
		"topic":              inputs["topic"],
		"depth":              depth,
		"citations_required": citationsRequired,
	}, nil
}

func init() { Register(ResearchBriefV1) }
