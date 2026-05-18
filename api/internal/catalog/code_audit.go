package catalog

import "context"

// CodeAuditV1 is the V1 engineering-services SKU that runs a recurring
// code-quality + security audit against a target repo, opening issues
// and PRs nightly. It's the recurring-revenue counterweight to
// research-brief in the V1 GA cut.
//
// Pricing model: monthly subscription per repo with overage billed per
// finding above the tier inclusion. BaseCents below is a placeholder
// ($299/month) pending operator price-discovery in Wave 11; treat as
// TBD. Findings overage is metered via [BillingMeter] = "findings_reported"
// and emitted by the operator-plane router on each daily-run completion.
//
// SLA: daily run, ≤2 h completion. Margin lever is cached AST + diff-only
// analysis between runs (full audit only on first run; subsequent runs
// analyse the delta).
//
// Wave 11 follow-up: register "code-audit:base" and "code-audit:finding"
// entries in api/internal/billing/stripe_ids.go (plus a Stripe Billing
// Meter for findings_reported, analogous to MeterTriageWatchTickets)
// once the operator provisions live-mode Stripe products. The reconciler
// dead-letters or skip-and-continues until then.
var CodeAuditV1 = &SKU{
	Name:         "code-audit",
	Version:      "v1",
	Category:     "engineering-services",
	InputsSchema: codeAuditSchema,
	Pricing: PricingTier{
		Model:       "monthly_subscription",
		BaseCents:   29900, // TBD: placeholder $299/mo pending operator finalization
		OverageRule: "finding",
	},
	SLA: SLA{
		Description:       "Daily run, ≤2h completion",
		MaxLatencySeconds: 7200,
	},
	Dispatcher:   dispatchCodeAudit,
	BillingMeter: "findings_reported",
}

const codeAuditSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["repo_url"],
  "properties": {
    "repo_url": {
      "type": "string",
      "pattern": "^(https?://|git@|ssh://git@)[A-Za-z0-9._-]+[:/][A-Za-z0-9._/-]+?(\\.git)?$"
    },
    "branch": { "type": "string", "default": "main" },
    "severity_threshold": {
      "type": "string",
      "enum": ["low", "medium", "high"],
      "default": "medium"
    }
  },
  "additionalProperties": false
}`

// dispatchCodeAudit builds the operator-plane RPC params blob. The
// real audit is a long-running cronjob that the operator plane installs
// at Dispatch time (analogous to triage-watch). This function only
// shapes the validated inputs; the cron schedule, the cached-AST
// directory, and the git auth resolution all live on the operator side.
func dispatchCodeAudit(_ context.Context, jobID string, inputs map[string]any) (any, error) {
	branch := "main"
	if v, ok := inputs["branch"]; ok {
		if s, ok := v.(string); ok && s != "" {
			branch = s
		}
	}
	severity := "medium"
	if v, ok := inputs["severity_threshold"]; ok {
		if s, ok := v.(string); ok && s != "" {
			severity = s
		}
	}
	return map[string]any{
		"sku":                "code-audit@v1",
		"job_id":             jobID,
		"repo_url":           inputs["repo_url"],
		"branch":             branch,
		"severity_threshold": severity,
	}, nil
}

func init() { Register(CodeAuditV1) }
