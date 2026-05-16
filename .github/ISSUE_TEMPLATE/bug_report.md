---
name: Bug report
about: Something is broken or not working as documented.
title: "[bug] "
labels: [bug, triage]
assignees: []
---

# Bug report

## Surface

Which part of `owera-cloud` is affected?

- [ ] `api/` (Go HTTP gateway)
- [ ] `web/` (Next.js dashboard)
- [ ] `infra/` or `tunnel/` (deployment config)
- [ ] `status/` (status page)
- [ ] `docs/` or root scaffold
- [ ] Other (please describe)

## Version

- API version: (from the `X-Owera-Version` response header, or [`VERSION`](../../VERSION) on `main`)
- Browser / client (if applicable):
- Operating system:

## What happened

A clear description of the unexpected behavior.

## What you expected

A clear description of what should have happened, ideally with a reference to the relevant doc (`docs/api.md`, `api/openapi.yaml`, etc.).

## Steps to reproduce

1.
2.
3.

## Logs and evidence

Paste the relevant JSONL log line(s), HTTP response body, or screenshot. **Redact secrets** (API keys, tenant IDs, customer email addresses, Stripe customer IDs) before pasting.

```text
<log line here>
```

## Additional context

Anything else that helps us understand the bug — environment, recent changes, whether this is reproducible.
