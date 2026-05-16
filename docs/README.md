# docs/

Documentation index for `owera-cloud`. Customer-facing docs and operator-facing runbooks live side-by-side here; the audience for each is called out at the top of the file.

## Customer-facing

| File | What it covers |
|---|---|
| [`api.md`](api.md) | API reference. Authentication, endpoints, error codes. Authoritative spec is [`api/openapi.yaml`](../api/openapi.yaml). |
| [`pricing.md`](pricing.md) | SKU pricing table, cost caps, usage-based vs subscription model, billing cadence. |
| [`onboarding.md`](onboarding.md) | New-customer ramp guide: signup → API key → first job → invoice. |
| [`support.md`](support.md) | Support tiers, response SLAs, escalation paths, how to reach us. |

## Operator-facing

| File | What it covers |
|---|---|
| [`architecture.md`](architecture.md) | How `owera-cloud` and `owera-fleet` fit together. Request-lifecycle mermaid diagram. |
| [`compliance.md`](compliance.md) | LGPD posture, SOC 2 trajectory, data residency. Pointer into [`../compliance/`](../compliance/). |
| [`runbook-deploy.md`](runbook-deploy.md) | How to deploy `api/` (Fly.io) and `web/` (Vercel). Rotation procedures. |

## Conventions

- Markdown only (`.md`). No `.rst`, no `.txt`. GitHub renders these; we want them readable in PR review.
- Customer-facing docs are customer-friendly but operationally honest. No marketing fluff.
- Operator-facing docs are straight operational prose — tables, lists, commands.
- Every internal link must resolve to a real file. Update links when files move.
- Mermaid diagrams render natively on GitHub. Prefer mermaid over ASCII when the diagram is more than ~10 lines.
