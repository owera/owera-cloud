# PagerDuty setup + first incident drill (T19.6)

The cloud plane has runbooks (`on-call-runbook.md`, `web-incident.md`, `incident-response.md`) that **assume PagerDuty exists** and reference schedules `owera-primary-oncall` / `owera-secondary-oncall` and escalation policy `owera-ic`. The pager itself is not yet wired. This runbook stands the account up, registers the in-process alerts the apiserver already emits, then executes the T19.6 acceptance drill ("pager fires within 2 min, runbook followed end-to-end, post-mortem authored within 48 h").

**Owner:** SRE Lead. CISO for the LGPD-class escalation policy.
**Prerequisites:** none external — PagerDuty has a 14-day free trial; Business tier (`$21/user/month`) is sufficient for V0 (1 primary + 1 secondary).
**Estimated time:** 90 minutes including the drill.

---

## 1. Account creation

1. Sign up at https://www.pagerduty.com/sign-up/ — choose **Business** tier (or trial it; the free Developer tier doesn't support SMS escalation which the T19.6 acceptance requires).
2. Subdomain: `owera.pagerduty.com`.
3. Invite a second user immediately (someone other than Rodrigo) — Settings → Users → Invite. The escalation tree in `on-call-runbook.md` §3 assumes a secondary exists. For V0, this can be a holding-pattern user that only exists to receive the page; replace once a real second operator joins.

---

## 2. Teams + services

PagerDuty: Configuration → Teams → New Team → **Owera Cloud**.

Inside the team, create three services (Configuration → Services → New Service):

| Service | Purpose | Integration |
|---|---|---|
| `owera-agentic-api` | Cloud-plane apiserver (Fly app `owera-agentic-api`) | **Events API v2** integration |
| `operator-plane` | Gateway + SSH-attached workers (`claw3` + `claw1` + `claw2`) | **Events API v2** integration |
| `web-edge` | Vercel dashboard + status page | **Vercel** native integration (auto-discovered) |

For each service, capture the **Integration Key** (32-char hex). These keys are the secrets the alerting paths in this repo + the fleet repo will POST to.

---

## 3. Schedules

Configuration → Schedules → New Schedule:

| Schedule name | Layer 1 (V0) | Layer 2 (V1, when 2nd op joins) |
|---|---|---|
| `owera-primary-oncall` | Rodrigo, 24×7, indefinite | Weekly rotation primary/secondary |
| `owera-secondary-oncall` | (placeholder secondary user), 24×7 | Weekly rotation, offset by 1 day from primary |

For V0 the schedule is a no-op (Rodrigo is always primary). The structure matters because the runbook references it; populate properly when the secondary operator is a real person.

---

## 4. Escalation policy

Configuration → Escalation Policies → New:

**`owera-ic`** (matches `on-call-runbook.md` §3 tree):

| Step | After | Notify | Method |
|---|---|---|---|
| 1 | Immediately | `owera-primary-oncall` schedule | Push notification + SMS |
| 2 | 10 min unack | `owera-secondary-oncall` schedule | Push + SMS |
| 3 | 30 min unack | SRE Lead (named user, not schedule) | Push + SMS + voice call |
| 4 | 60 min unack | CTO/CEO (named user) | Voice call |

Attach this escalation policy to **all three services** from §2.

---

## 5. Integration: cloud-plane apiserver → PagerDuty

The apiserver has an in-process `logAlerter` at `api/cmd/apiserver/main.go:375-392` that emits JSONL on stderr today. The drift reconciler at `runDriftReconciler` (same file) fires `logAlerter.Alert(ctx, "drift_detected", payload)` per tenant where billing drift exceeds 0.5 %.

Extend it to also POST to PagerDuty Events API v2 when a key is configured. Sketch (does **not** belong in this runbook; capture as a separate PR):

```go
// New file: api/internal/alerting/pagerduty.go
type PagerDutyAlerter struct {
    Client         *http.Client
    IntegrationKey string
    // Service routing key tells PD which service the page hits.
    // Use this to differentiate apiserver alerts from operator-plane.
    DefaultSeverity string // "critical" | "error" | "warning" | "info"
}

func (a *PagerDutyAlerter) Alert(ctx context.Context, kind string, payload map[string]any) error {
    body := map[string]any{
        "routing_key":  a.IntegrationKey,
        "event_action": "trigger",
        "payload": map[string]any{
            "summary":   kind + ": " + summarize(payload),
            "source":    "owera-agentic-api",
            "severity":  a.DefaultSeverity,
            "custom_details": payload,
        },
    }
    // POST https://events.pagerduty.com/v2/enqueue
}

// In main.go:
type multiAlerter struct{ alerters []billing.Alerter }
func (m multiAlerter) Alert(ctx context.Context, kind string, payload map[string]any) error {
    for _, a := range m.alerters {
        // log all errors but keep going so one failure doesn't suppress the
        // others. logAlerter is the fallback; PD failure must not silence it.
        _ = a.Alert(ctx, kind, payload)
    }
    return nil
}
```

In `chooseWiring()`, if `PAGERDUTY_INTEGRATION_KEY` is set:

```go
if key := os.Getenv("PAGERDUTY_INTEGRATION_KEY"); key != "" {
    pdAlerter := &alerting.PagerDutyAlerter{
        Client:          &http.Client{Timeout: 10 * time.Second},
        IntegrationKey:  key,
        DefaultSeverity: "critical",
    }
    rec.Alerter = multiAlerter{alerters: []billing.Alerter{logAlerter{}, pdAlerter}}
}
```

Fly secret to set after the PR lands:

```bash
fly secrets set --app owera-agentic-api \
  PAGERDUTY_INTEGRATION_KEY=<from §2, owera-agentic-api service>
```

Confirm via boot log fingerprint extension (`alerting=pagerduty+log` or similar — add to the same line as `audit=...`).

---

## 6. Integration: operator plane → PagerDuty

The gateway's `heartbeat-watchdog.sh` currently fires `ntfy.sh` push notifications when a worker heartbeat is older than 5 min. Add PagerDuty as a parallel channel (do not replace ntfy — ntfy is cheap and gives a second visibility path).

Sketch:

```bash
# In scripts/heartbeat-watchdog.sh, alongside the ntfy curl:
if [ -n "${PAGERDUTY_INTEGRATION_KEY:-}" ]; then
  curl -fsS https://events.pagerduty.com/v2/enqueue \
    -H 'Content-Type: application/json' \
    -d "$(jq -n --arg key "$PAGERDUTY_INTEGRATION_KEY" \
                --arg summary "Worker heartbeat stale: $host" \
                '{routing_key:$key,event_action:"trigger",payload:{summary:$summary,source:"hermes-gateway",severity:"error",custom_details:{host:$host,age_seconds:$age}}}')"
fi
```

`PAGERDUTY_INTEGRATION_KEY` lives in `~/.hermes/.env` (chmod 600); the watchdog script reads it via the standard `source ~/.hermes/.env` preamble.

---

## 7. Integration: Vercel → PagerDuty (web edge)

PagerDuty has a native Vercel integration. In PagerDuty:

1. Configure `web-edge` service → Integrations → "Vercel" → Connect.
2. Auto-discovers the `owera-web` and `owera-status` projects.
3. Pages on deploy failures + canary regressions.

No code to write on the Owera side; the integration is dashboard-only.

---

## 8. T19.6 drill (acceptance gate)

Once §1-§7 are wired (at minimum §5 for the apiserver; §6 and §7 can land separately), execute the drill:

### 8a. Trigger a synthetic drift alert

```bash
# Force the apiserver's drift reconciler to fire by injecting a synthetic
# drift event. Simplest path: run the reconciler binary with a mocked
# Stripe usage report that differs from the ledger by 5 %.

cd ~/owera-cloud/api
go run ./cmd/reconciler --synthetic-drift=0.05 --tenant=tnt-drill-001
# expect: HTTP 202 from PagerDuty Events API; stderr JSONL also emitted
```

### 8b. Confirm the page arrives (<2 min)

- [ ] Phone vibrates / rings within 2 minutes of the trigger.
- [ ] PagerDuty incident UI shows the page with correct severity + payload.
- [ ] Acknowledge the page in the PagerDuty mobile app.

### 8c. Walk through `web-incident.md` / `incident-response.md`

Pretend the drift is real:

- [ ] Open the relevant runbook from the page link.
- [ ] Follow the triage section step-by-step. Mark each checkbox as you go.
- [ ] If anything in the runbook is wrong, ambiguous, or out-of-date, note it (this is the most valuable output of the drill).

### 8d. Resolve the incident

- [ ] In PagerDuty, click Resolve. Tag the incident as `drill`.
- [ ] Confirm escalation did not fire (you acknowledged inside the 5-min window).

### 8e. Post-mortem (within 48 h)

Use the post-mortem template at `compliance/runbooks/incident-response.md` §"Post-mortem template" (if absent, this drill produces a v1 template — which is itself valuable artefact).

Required fields:

- **Trigger**: synthetic drift via `cmd/reconciler --synthetic-drift=0.05`.
- **Time to page**: actual minutes from trigger to phone ring.
- **Time to ack**: actual minutes from page to ack.
- **Runbook gaps**: list every checkbox in 8c that was unclear or out-of-date.
- **Action items**: one row per gap. Owner = the person who should fix the runbook. Due = next on-call rotation.

Post the post-mortem in `#oncall` and link from the PagerDuty incident.

---

## 9. Escalation policy: LGPD/GDPR-class events

The `on-call-runbook.md` §3 note says: "For LGPD/GDPR-class events: **immediately** escalate to CISO, regardless of pager state."

Implement this with a **dedicated PagerDuty service** named `data-protection` that has its own escalation policy:

| Step | After | Notify |
|---|---|---|
| 1 | Immediately | CISO (named user) | Push + SMS + voice |
| 2 | 10 min | SRE Lead + CTO/CEO | Push + SMS |

Tag any alert with `severity: critical` AND custom_detail `classification: LGPD-personal-data` and route it to this service via integration key `PAGERDUTY_LGPD_INTEGRATION_KEY`.

Right now no code path emits LGPD-classified alerts; this is forward-prep. Capture as TODO in `api/internal/audit/audit.go` next to the LGPD comment block.

---

## 10. Cost + scaling notes

- V0: Business tier, 2 users, 3 services, 1 integration = **$42 / month**.
- V1 (4 users + 4th service): ~$84 / month.
- Stays under $200/month through V2; non-issue against the $200/mo per-customer cap.

If cost becomes a concern, OpsGenie ($9/user) is the cheapest replacement; the on-call-runbook references are PagerDuty-specific but the schedule + escalation concepts map 1:1.

---

## Cross-references

- `compliance/runbooks/on-call-runbook.md` — escalation tree (this runbook implements the PagerDuty side of it)
- `compliance/runbooks/web-incident.md` — example runbook walked in the drill
- `compliance/runbooks/incident-response.md` — Sev1 declaration + post-mortem template
- `api/cmd/apiserver/main.go:375-392` — `logAlerter` (extension point for `multiAlerter`)
- `api/cmd/apiserver/main.go:340-368` — `runDriftReconciler` (calls `rec.Alerter.Alert` per drift)
- `~/hermes-setup/scripts/heartbeat-watchdog.sh` — operator-plane alert integration point
- `infra/secrets-manifest.md` — register `PAGERDUTY_INTEGRATION_KEY` + `PAGERDUTY_LGPD_INTEGRATION_KEY` rows after this lands
