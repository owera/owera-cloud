# Web / status-page incident runbook

What to do when **app.owera.ai** (the customer dashboard) or **status.owera.ai** (the public status page) is misbehaving. This runbook covers the cloud-edge surfaces; for API-tier incidents see [`incident-response.md`](./incident-response.md), and for operator-plane gateway outages see the operator-plane runbook in `owera-fleet/`.

## 0. Trigger

You are reading this because:

- Vercel deployment for the dashboard or status page failed, OR
- `status.owera.ai` is rendering "Status unknown" / "Snapshot fetch failed" for longer than 5 min, OR
- A customer reports `app.owera.ai` is 5xx-ing or stuck on a stale build, OR
- PagerDuty paged you for `status-page-stale` or `dashboard-down` (Wave-9 monitors).

For full Sev1 declaration mechanics, see [`incident-response.md`](./incident-response.md) §1.

## 1. Triage (first 5 min)

```
[ ] Open https://app.owera.ai in an incognito window. Is it 5xx? Stale? Up?
[ ] Open https://status.owera.ai. What does the overall banner say?
[ ] In Vercel: check the most recent production deploy for both projects.
    - Did it succeed? When? Who pushed?
    - Is there a stuck deploy?
[ ] Check Cloudflare → owera.ai zone → Analytics for spikes in 5xx or
    bandwidth that point at the edge rather than origin.
```

## 2. Decision tree

### Dashboard 5xx or blank

1. Roll back to the last known-good Vercel deployment:
   - Vercel dashboard → owera-web project → Deployments → previous successful prod deploy → **Promote to Production**.
   - Time to recover: ~30s.
2. Once recovered, open a Sev2 in `#incident-declare` with the broken deploy SHA and assign the responsible engineer to triage on a branch.

### Status page stale or showing "Snapshot unknown"

The status page reads from `NEXT_PUBLIC_SNAPSHOT_URL` (a public R2 / S3 object the operator plane writes every 30s). If the snapshot is stale:

1. **Confirm the symptom is not the page itself:** `curl -sf "$NEXT_PUBLIC_SNAPSHOT_URL" | jq '.ts'` from your laptop. If the URL returns a fresh `ts`, the page is the problem — re-deploy.
2. If the snapshot URL itself is stale, the **operator-plane snapshot publisher** is the problem:
   - SSH to the gateway (`claw3@<gateway-host>`).
   - `tail -50 ~/.hermes/logs/snapshot-publisher.jsonl` — look for the last successful PUT.
   - Restart the publisher LaunchAgent: `launchctl kickstart -k gui/501/com.owera.snapshot-publisher`.
   - If R2 credentials rotated, re-run `scripts/install-snapshot-publisher.sh` to refresh the keychain entry.
3. While the snapshot is broken, the status page automatically marks operator-plane and worker components as `down` (because absence-of-data === ambiguity-resolved-as-bad on a public status surface). That is correct behaviour — don't fight it.

### Cloudflare TLS / DNS issues

- `app.owera.ai` and `status.owera.ai` both CNAME to `cname.vercel-dns.com` with `proxied: false` in `dns.cloudflare.yaml`. Vercel issues the cert. If the cert is failing:
  1. Vercel dashboard → Domains → re-verify the domain.
  2. If `dns.cloudflare.yaml` and the live record diverge, that's the bug — reconcile.

## 3. Comms

- **Sev1 (full outage):** post a holding update to status.owera.ai (drop a new file into `status/incidents/`) within 15 min per [`incident-response.md`](./incident-response.md) §2. The status page renders new incident files within ~30s of the deploy.
- **Sev2 (degraded but reachable):** post the incident file at resolution, not at start. Adding a banner mid-incident is currently a code change; an in-page banner without a deploy is a Wave-9 enhancement.

## 4. Post-incident

Same flow as `incident-response.md` §5–6. Add the customer-facing incident summary to `status/incidents/` (template at `status/incidents/TEMPLATE.md`). The post-mortem itself stays in `compliance/runbooks/post-mortems/` and never gets surfaced to customers.

## 5. T19.4 acceptance — forced-incident drill

To verify the <60s detection requirement after a deploy or once a quarter:

```
[ ] Open a PR that sets NEXT_PUBLIC_FORCE_INCIDENT=1 in the preview environment.
[ ] Wait for the preview deploy URL.
[ ] Open it. The overall banner should read "Active incident in progress"
    and the operator-plane-gateway component should be marked `down`
    within 30s of first paint (the client polls every 30s).
[ ] Close the PR. The next production deploy should clear the flag and
    return to operational.
```

Record the drill outcome in `compliance/audit-controls/` per the SOC 2 readiness plan.
