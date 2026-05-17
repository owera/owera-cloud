package catalog

import "context"

// CampaignSwarmV1 is the V0 growth SKU that takes a campaign brief +
// audience segment and fans out multi-channel outreach via the operator
// plane's worker fleet. Pricing is per-job fixed: each campaign is a
// discrete billable event at one of three tier prices (S/M/L), with no
// monthly commitment. The tier the operator plane records on the bill
// event is the StripeRef key (campaign-swarm:S / :M / :L) consulted by
// the catalog [Dispatcher] at reconcile time.
var CampaignSwarmV1 = &SKU{
	Name:         "campaign-swarm",
	Version:      "v1",
	Category:     "growth",
	InputsSchema: campaignSwarmSchema,
	Pricing: PricingTier{
		Model:       "per_job_fixed",
		BaseCents:   0,
		OverageRule: "campaign",
	},
	SLA: SLA{
		Description:       "<15 min campaign launch",
		MaxLatencySeconds: 900,
	},
	Dispatcher:   dispatchCampaignSwarm,
	BillingMeter: "campaigns_launched",
}

const campaignSwarmSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["brief", "audience_segment"],
  "properties": {
    "brief": { "type": "string", "minLength": 10 },
    "audience_segment": { "type": "string" },
    "channels": {
      "type": "array",
      "items": { "type": "string", "enum": ["email", "linkedin", "x", "phone"] },
      "minItems": 1,
      "default": ["email"]
    },
    "max_outreach": { "type": "integer", "minimum": 1, "maximum": 10000, "default": 500 }
  },
  "additionalProperties": false
}`

func dispatchCampaignSwarm(_ context.Context, jobID string, inputs map[string]any) (any, error) {
	channels := []any{"email"}
	if v, ok := inputs["channels"]; ok {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			channels = arr
		}
	}
	maxOutreach := 500
	if v, ok := inputs["max_outreach"]; ok {
		if f, ok := v.(float64); ok {
			maxOutreach = int(f)
		}
	}
	return map[string]any{
		"sku":              "campaign-swarm@v1",
		"job_id":           jobID,
		"brief":            inputs["brief"],
		"audience_segment": inputs["audience_segment"],
		"channels":         channels,
		"max_outreach":     maxOutreach,
	}, nil
}

func init() { Register(CampaignSwarmV1) }
