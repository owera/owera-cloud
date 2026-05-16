# On-call runbook

The operational realization of [`../policies/access-control-policy.md`](../policies/access-control-policy.md) §9 and [`../policies/incident-response-policy.md`](../policies/incident-response-policy.md).

## 1. Rotation

| Role | Hours | Source of truth |
|------|-------|-----------------|
| Primary on-call | 24×7, 1-week rotation | PagerDuty schedule `owera-primary-oncall` |
| Secondary on-call | 24×7, 1-week rotation (offset by 1 day) | PagerDuty schedule `owera-secondary-oncall` |
| Incident commander | Same as primary during business hours; escalates to SRE Lead overnight | PagerDuty escalation policy `owera-ic` |
| Data protection lead | Business hours only; CISO is the constant | n/a — paged manually for LGPD/GDPR-class events |

Hand-offs happen at **09:00 BRT on Mondays**. Outgoing primary posts a 1-paragraph hand-off note in `#oncall` covering: open incidents, recent post-mortems with pending action items, anything weird in production over the past week.

## 2. Pager hygiene

- Acknowledge within **5 minutes** of page. If you can't, the page escalates to secondary at 10 minutes.
- Take the page seriously even if it looks like a false positive — false positives are debugged after the page is closed, not silenced.
- Keep your phone charged, sound on, and a laptop within 15 minutes' reach during your rotation.

## 3. Escalation tree

```
Page → Primary on-call (5 min ack)
       │
       └── No ack in 10 min → Secondary on-call
                              │
                              └── No ack in 20 min → SRE Lead
                                                     │
                                                     └── No ack in 30 min → CTO/CEO
```

For LGPD/GDPR-class events: **immediately** escalate to CISO, regardless of pager state.

For security-class events (suspected breach, key compromise): **immediately** escalate to CISO + SRE Lead in parallel.

## 4. Common pages

| Page text | First check | Likely cause | First mitigation |
|-----------|-------------|--------------|------------------|
| `api.owera.ai healthz 5xx` | `fly status -a owera-agentic-api` | Bad deploy, dependency outage | `fly deploy --image <last-green>` |
| `app.owera.ai TTFB > 5s` | Vercel dashboard, region map | Edge function cold start, dependency timeout | Promote previous Vercel deploy |
| `internal-rpc.owera.ai down` | `launchctl print system/ai.owera.cloudflared` on gateway | cloudflared crashed, gateway offline | `sudo launchctl kickstart -k system/ai.owera.cloudflared` |
| `Worker heartbeat missing` | `fleetctl status` from owera-fleet repo | Worker offline, SSH key issue | Per owera-fleet runbook |
| `Audit log hash chain mismatch` | This is a Sev1 immediately | Tampering or storage corruption | Freeze writes; escalate to CISO; preserve current state |
| `Stripe webhook signature failures` | Stripe dashboard → Webhooks → recent events | Webhook secret rotation skew | Verify `STRIPE_WEBHOOK_SECRET` matches Stripe; rotate if compromised |
| `restic backup failed` | gateway logs `~/.hermes/logs/backup.jsonl` | SFTP target unreachable, disk full | Per owera-fleet runbook |

## 5. Tools

| Tool | URL / command | Purpose |
|------|---------------|---------|
| Fly | `fly status -a owera-agentic-api`; <https://fly.io/dashboard> | API tier |
| Vercel | <https://vercel.com/owera/owera-web> | Web tier |
| Cloudflare | <https://dash.cloudflare.com> | DNS, tunnel, WAF |
| PagerDuty | <https://owera.pagerduty.com> | Pager state, escalations |
| Sentry | <https://sentry.io/organizations/owera> | Errors, performance |
| Stripe | <https://dashboard.stripe.com> | Payments, webhooks |
| Status page | <https://status.owera.ai> | Customer-facing comms |
| `fleetctl` | (owera-fleet) CLI | Operator plane control |

## 6. Hand-off note template

```
## Hand-off YYYY-MM-DD

**Outgoing:** <name>
**Incoming:** <name>

### Open
- [ ] <thing>

### Recent post-mortems / action items
- INC-NNN: <one-liner>; AI owner @<name> due <date>

### Watch list
- <anything weird this week>
```

Posted in `#oncall` at 09:00 BRT Monday.

## 7. After-action

After every paged event (even if no incident is declared), the on-call writes a one-line summary in `#oncall-after-action` with: time, page text, root cause, mitigation, time-to-resolve. Used in quarterly retrospectives to spot false-positive patterns and runbook gaps.

## Cross-references

- [`../policies/access-control-policy.md`](../policies/access-control-policy.md) — break-glass procedure
- [`../policies/incident-response-policy.md`](../policies/incident-response-policy.md) — severity classification
- [`incident-response.md`](./incident-response.md) — Sev1 step-by-step

## Version history

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-16 | Owera Security | Initial version |
