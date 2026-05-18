# Trust Service Category — Availability

> **Scope.** Availability covers the Common Criteria (by reference — see [`tsc-security.md`](./tsc-security.md)) plus the additional A-series criteria specific to availability commitments. Owera elects this TSC because the support-SLA in [`compliance/runbooks/support-sla.md`](../runbooks/support-sla.md) is a contractual commitment to customers, and uptime claims appear in our public marketing.

> **In-scope commitments.** The auditor will read management's system description for stated availability commitments. As of 2026-05-17 those are:
> - Tier-1 cloud plane (`api.owera.ai`, `app.owera.ai`): **99.5%** monthly target.
> - Tier-2 status page (`status.owera.ai`): independent host (Vercel) so it survives a Cloudflare-Tunnel outage.
> - Support-SLA: first-response targets per [`compliance/runbooks/support-sla.md`](../runbooks/support-sla.md) §1.

## Control mapping

| A # | Description | Owera control | Evidence path | Owner workstream | Status |
|-----|-------------|---------------|---------------|-------------------|--------|
| A1.1 | Maintains current processing capacity and demand-management practices to meet availability commitments. | Fly autoscaler config; manual capacity review on the SRE on-call quarterly checklist. | [`infra/api.fly.toml`](../../infra/api.fly.toml) (scaling block); on-call runbook capacity-review section [`compliance/runbooks/on-call-runbook.md`](../runbooks/on-call-runbook.md) | SRE | documented-only |
| A1.2 | Authorizes/designs/develops/implements environmental protections (backups, redundancy, DR). | restic-encrypted off-host backup of operator-plane state; daily 03:15; restore-tested bit-identical; disaster-recovery doc with per-tier RTO/RPO. | [`hermes-setup/scripts/backup-hermes-state.sh`](../../../hermes-setup/scripts/backup-hermes-state.sh), [`hermes-setup/scripts/install-backup.sh`](../../../hermes-setup/scripts/install-backup.sh), [`infra/disaster-recovery.md`](../../infra/disaster-recovery.md) → [`evidence/CC9.1-disaster-recovery.md`](./evidence/CC9.1-disaster-recovery.md) | SRE / PE | implemented |
| A1.3 | Tests recovery-plan procedures. | Quarterly DR drills tabletop + Q1/Q3 actual restore drill into staging. | [`infra/disaster-recovery.md`](../../infra/disaster-recovery.md) "Targets" table column **Tested** → screenshot-on-drill-day check-in (process documented in [`evidence-collection-runbook.md`](./evidence-collection-runbook.md) §A1.3) | SRE | TBD |
| A1.4 (Owera-defined) | Heartbeat-monitors the operator-plane workers; alerts via ntfy on stale heartbeats. | gateway watchdog scans heartbeats every 2 minutes; missing heartbeat triggers alert. | [`hermes-setup/scripts/heartbeat-watchdog.sh`](../../../hermes-setup/scripts/heartbeat-watchdog.sh), [`hermes-setup/scripts/install-watchdog.sh`](../../../hermes-setup/scripts/install-watchdog.sh) | PE | implemented |
| A1.5 (Owera-defined) | Public status page communicates availability to customers in real time. | `status.owera.ai` powered by separate Vercel project + status-incident workflow. | [`status/`](../../status/), [`infra/status.vercel.json`](../../infra/status.vercel.json), [`compliance/runbooks/web-incident.md`](../runbooks/web-incident.md) | SRE | implemented |
| A1.6 (Owera-defined) | Support SLA published with measurable first-response targets. | [`compliance/runbooks/support-sla.md`](../runbooks/support-sla.md) §1 (sev-1: 1h business hours / 4h after-hours; cascade down). | [`compliance/runbooks/support-sla.md`](../runbooks/support-sla.md), monthly SLA roll-up `compliance/sla-retros/YYYY-MM.md` (created at first month-end). | CISO | documented-only |

## Common-Criteria controls invoked by Availability

The auditor cross-references these Security controls when evaluating Availability:

| CC # | Description | Why it matters for Availability |
|------|-------------|----------------------------------|
| CC7.2 | Monitors components for anomalies. | Sentry + Fly metrics + heartbeat watchdog detect availability incidents in time to act. |
| CC7.4 | Responds to identified incidents. | The incident runbook is what we follow when availability degrades. |
| CC7.5 | Recovers from incidents. | Recovery, post-mortem, and remediation close the availability incident loop. |
| CC9.1 | Mitigates business-disruption risk. | Disaster-recovery doc + restic backup are the systemic availability mitigations. |
| CC9.2 | Manages vendor risk. | Fly, Vercel, Cloudflare, Stripe carry independent SOC 2 attestations; their availability claims chain into ours. |

## Known gaps (summary)

See [`known-gaps.md`](./known-gaps.md) for the consolidated list. Availability-specific gaps as of 2026-05-17:

- **A1.1 capacity-management evidence.** The Fly autoscaler is configured, but the quarterly capacity-review artifact is not yet a recurring deliverable. Owner: SRE. Target: Q3 2026 (first review one quarter before SOC 2 onboarding).
- **A1.3 DR-drill evidence.** [`infra/disaster-recovery.md`](../../infra/disaster-recovery.md) lists every restore target as `[ ] Q3/Q4 2026 (target)`. The first end-to-end drill must produce a check-in under `compliance/audit-controls/evidence/A1.3-drill-screenshots/`. Owner: SRE.
- **A1.6 SLA roll-up automation.** The monthly SLA roll-up is currently a TODO script (`scripts/support-sla-rollup.sh`) per [`compliance/runbooks/support-sla.md`](../runbooks/support-sla.md). Until automated, the on-call CSE produces the roll-up by hand at month-end.

## Cross-references

- **Security** — see [`tsc-security.md`](./tsc-security.md) for CC1–CC9.
- **Processing Integrity** — see [`tsc-processing-integrity.md`](./tsc-processing-integrity.md) for the signed-ledger evidence that downtime did not produce inconsistent state.
- **Evidence collection** — see [`evidence-collection-runbook.md`](./evidence-collection-runbook.md) for the exact commands to pull the restic snapshot listing, Fly release history, and SLA roll-up at audit time.
