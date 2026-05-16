# status/

The declarative spec for **status.owera.ai** — Owera's public status page. A Next.js implementation of the page lands in a follow-on PR; this directory is the source of truth for **what is monitored, what the SLA targets are, and where incident reports live**.

## What status.owera.ai is

A read-only public surface that answers two questions for customers:

1. **Is it working right now?** Per-component traffic-light view (operational / degraded / down).
2. **Was there a problem recently?** Reverse-chronological feed of incidents with affected components, start/end times, and post-incident summaries.

The page derives current state from `api.owera.ai/internal/status` (implemented in `api/internal/status/` — outside this directory's scope), which itself aggregates probes against the operator plane.

## How uptime is computed

For each component listed in [`components.yaml`](./components.yaml):

```
uptime_pct(month) = (probe_pass_count / probe_total_count) over the trailing 30 days
```

A probe failure is **not** an outage on its own — three consecutive failures over a 90-second window (or a single declared incident affecting the component) tips the component to `degraded` or `down`.

SLAs are tracked **at the component level** and shown publicly. SLOs (the internal, tighter targets the team operates against) are not surfaced.

## Files

| File | Purpose |
|------|---------|
| [`components.yaml`](./components.yaml) | Declarative list of monitored components, probes, SLAs. Source of truth. |
| [`incidents/`](./incidents/) | One Markdown file per incident, after the post-incident phase. The page renders these as the feed. |

## Incident report format

After an incident is resolved and the post-mortem is published (see [`../compliance/runbooks/incident-response.md`](../compliance/runbooks/incident-response.md)), the comms lead writes a **customer-facing summary** here. This is *not* the post-mortem — that document is internal and lives in `../compliance/runbooks/post-mortems/`. The customer-facing version omits internal details, vendor specifics where they don't help customers, and any forensic information.

Template:

```markdown
---
id: INC-NNN
title: "Brief, factual summary"
status: resolved
severity: sev1 | sev2 | sev3
affected_components: [public-api, customer-dashboard]
started_at: 2026-MM-DDTHH:MM:SSZ
resolved_at: 2026-MM-DDTHH:MM:SSZ
---

## What happened

One paragraph plain-English explanation. No internal jargon.

## Impact

Who was affected, what they would have noticed, and for how long.

## What we did

The mitigation steps the customer can see.

## What we are doing to prevent recurrence

The action items from the internal post-mortem, summarized.

## Timeline (UTC)

| Time | Event |
|------|-------|
| HH:MM | We detected ... |
| HH:MM | Mitigation deployed. |
| HH:MM | All systems operational. |
```

Filename convention: `incidents/<YYYY-MM-DD>-<short-slug>.md`. Once the file exists in main, the status page auto-renders it.

## SLA framing

The SLAs in `components.yaml` are the **public commitment**. Internal SLOs are tighter:

| Component | Public SLA | Internal SLO |
|-----------|-----------|--------------|
| Public API | 99.9% | 99.95% |
| Customer Dashboard | 99.9% | 99.95% |
| Operator Plane Gateway | 99.5% | 99.7% |
| Hermes Worker Fleet | 99.0% | 99.5% |

The gap between SLA and SLO is the team's tolerance for occasional bad days without breaching customer commitments. Burn alerts (multi-window, multi-rate per Google SRE) fire on SLO budget exhaustion, not SLA — that's the early warning system.

## Cross-references

- [`components.yaml`](./components.yaml) — what is monitored
- [`../api/internal/status/`](../api/internal/status/) — the implementation of the probes (api slice; not in this directory's ownership)
- [`../compliance/runbooks/incident-response.md`](../compliance/runbooks/incident-response.md) — incident handling that produces these reports
- [`../infra/disaster-recovery.md`](../infra/disaster-recovery.md) — RTO/RPO that constrains realistic SLAs
