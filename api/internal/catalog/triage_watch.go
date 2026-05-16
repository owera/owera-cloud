package catalog

import "context"

// TriageWatchV1 is the V0 customer-operations SKU that ingests a support
// queue, classifies incoming tickets by priority, and dispatches the high-
// severity ones into the operator plane for an autonomous first response.
//
// Pricing model: $499/month subscription with a per-ticket overage rule.
// SLA: ticket→first-response under 2 minutes.
var TriageWatchV1 = &SKU{
	Name:         "triage-watch",
	Version:      "v1",
	Category:     "customer-operations",
	InputsSchema: triageWatchSchema,
	Pricing: PricingTier{
		Model:       "monthly_subscription",
		BaseCents:   49900,
		OverageRule: "ticket",
	},
	SLA: SLA{
		Description:       "<2 min ticket response",
		MaxLatencySeconds: 120,
	},
	Dispatcher:   dispatchTriageWatch,
	BillingMeter: "tickets_processed",
}

const triageWatchSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["queue_url"],
  "properties": {
    "queue_url": { "type": "string" },
    "priority_threshold": { "type": "integer", "default": 8, "minimum": 1, "maximum": 10 }
  },
  "additionalProperties": false
}`

// dispatchTriageWatch builds the operator-plane RPC params blob. Transport
// happens in internal/dispatcher; this function only shapes the payload so
// the operator plane can pick it up by SKU.
func dispatchTriageWatch(_ context.Context, jobID string, inputs map[string]any) (any, error) {
	threshold := 8
	if v, ok := inputs["priority_threshold"]; ok {
		if f, ok := v.(float64); ok {
			threshold = int(f)
		}
	}
	return map[string]any{
		"sku":                "triage-watch@v1",
		"job_id":             jobID,
		"queue_url":          inputs["queue_url"],
		"priority_threshold": threshold,
	}, nil
}

func init() { Register(TriageWatchV1) }
