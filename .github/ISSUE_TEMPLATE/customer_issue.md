---
name: Customer issue
about: For paying customers — auto-routes to support.
title: "[customer] "
labels: [customer, support, triage]
assignees: []
---

# Customer issue

> **Paying customers:** this template auto-routes to support. We respond per the SLAs in [`docs/support.md`](../../docs/support.md). For severity-1 production outages, prefer the in-product support form at [`app.owera.ai`](https://app.owera.ai) → Support, which carries your tenant context automatically.

## Tenant

Tenant ID: `t_...`

(Find it in the dashboard under **Settings → Tenant**. Don't paste your API key.)

## Severity

- [ ] **S1 — Critical.** Production outage, billing materially incorrect, security breach in progress.
- [ ] **S2 — High.** SLA breach on a SKU, major feature broken.
- [ ] **S3 — Normal.** Functional bug affecting your workflow.
- [ ] **S4 — Low.** Cosmetic or documentation.

## What's happening

A clear description of the problem, including which SKU / endpoint / dashboard view is involved.

## Job IDs (if applicable)

- `j_...`
- `j_...`

## When this started

Approximate timestamp (with timezone) when you first observed the issue.

## What you expected

What should have happened, with a reference to the doc or contract that supports the expectation.

## Logs / evidence

Paste relevant responses, screenshots, or job outputs. **Redact secrets** before pasting (API keys, your customers' personal data, payment info).

## Business impact

Optional — helps us prioritize. Are jobs blocked? Customers affected? Revenue at risk?
