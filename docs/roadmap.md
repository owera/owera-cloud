# Owera Agentic — product roadmap

> **Audience: internal team + select prospects under NDA.** This is the planned ramp; it changes with demand and engineering signal. Source of truth for delivered state is the git log on `main` + the GA gates (`compliance/policies/ga-gate.md`). For the master plan (waves, workstreams, ticket backlog), see [`knowing-all-you-now-calm-leaf.md`](https://github.com/owera/owera-fleet/blob/main/knowing-all-you-now-calm-leaf.md) in the operator-plane repo.

Last updated: 2026-05-17.

## Status at a glance

| Wave | Scope | State |
|---|---|---|
| 1–6 | Operator plane (Phases 1–2) | Shipped to `owera-fleet/main` |
| 7 | Phase-3 cloud scaffolding | Shipped to `owera-cloud/main` |
| 8 | Phase-3 core build (WS-14, 15, 16, 17, 18, 19) | Integration PRs open; merges in flight |
| 8.1 | Wave-8 follow-ups (PRAGMA, usage shape, user_id, this doc) | Open PRs |
| 9 | Phase-4 launch readiness (staging fleet, drill, beta-1) | Planned |
| 10 | Beta → GA decision | Gated on GA policy |
| Post-GA | V2–V4 catalogue ramp | Demand-driven |

## Catalogue ramp

The catalogue ramps in four tiers post-V0. Each SKU's promotion through tiers is gated on (a) an operator-plane primitive ready and (b) a PM signal of demand from customer discovery — neither alone is enough.

### V0 — private beta (now)

| SKU | Why first | Status |
|---|---|---|
| `triage-watch` | Long-running cronjob + ledger-backed billing — exercises the most platform muscle for the smallest revenue surface area; cheapest end-to-end smoke test of the productisation work | Registered in catalog (PR #1) |
| `campaign-swarm` | Multi-node fan-out — exercises the swarm primitive; high-perceived-value one-shot SKU for early-design-partner conversations | Registered in catalog (PR #1) |

**Promotion gate to V1:** All six GA gates green (`compliance/policies/ga-gate.md`) + ≥3 paying customers for ≥30 d.

### V1 — public GA

| SKU | Promotion driver | Pre-reqs |
|---|---|---|
| `research-brief` | Hermes web+browser toolsets already in the operator plane; one-shot revenue + diversifies ICP beyond cron-heavy workloads | T14.7 catalog PR |
| `code-audit` | Recurring revenue with the highest WTP from early customer-discovery interviews; reuses the diff-only-analysis pattern the operator plane already proved in `review-branch` | T14.7 catalog PR |

**Promotion gate to V2:** ≥3 paying customers on V1 SKUs for ≥90 d.

### V2 — 90 days post-GA

| SKU | Promotion driver | Demand signal needed |
|---|---|---|
| `dep-upgrade` | Operator plane has `gh + test-runner` reuse; the SKU is a thin wrapper. High recurring MRR if it lands. | ≥2 customer-discovery asks |
| `inbox-triage` | Reuses `triage-watch` core; broadens ICP into sales/ops teams without engineering budget. | ≥2 customer-discovery asks |
| `monitor-watch` | Complementary to `research-brief`; one cron + one one-shot per topic. | Existing `research-brief` customers asking for "but daily" |
| `content-batch` | Marketing-heavy ICP overlap with `campaign-swarm`; fills the gap when customers want bulk content without a launch context. | ≥2 customer-discovery asks from marketing-heavy customers |

### V3 — 180 days post-GA

| SKU | Promotion driver | Demand signal needed |
|---|---|---|
| `xcode-ci` | Mac fleet is the unique advantage — every dev shop without their own Mac CI is a prospect; differentiated against generic CI providers. | First inbound from a Mac-CI-curious team OR Apple-toolchain certification readiness |
| `app-build` | High-value one-shot; long-running pattern + ledger replay is the killer feature for migrations. | First paid pilot for a mobile app shop |
| `docs-author` | Diff-only doc regeneration; reuses `code-audit` AST cache. | Two existing `code-audit` customers asking |
| `incident-postmortem` | Web + Hermes research; one-shot revenue. Reuses `research-brief` patterns. | Two existing customers asking after a real incident |

### V4 — 12 months+ (demand-driven)

| SKU | Why deferred |
|---|---|
| `test-author` | Flake metrics in ledger is a months-long signal collection — needs production data before pricing makes sense |
| `migration-pilot` | High-touch, project-shape revenue — best after `app-build` + `code-audit` prove the team can sell + deliver one-shot projects |
| `lead-enrich` | Marketing-CRM ICP is far from current target — defer until V2/V3 builds the marketing customer base |
| `etl-flow` | Upstream/downstream credential management at customer scale is harder than the v2 §3.4 plan implied; defer until simpler SKUs prove the platform |

## Operator-plane horizon (not in catalogue but on the path)

These are operator-plane capabilities that gate new catalogue tiers but don't themselves ship as SKUs.

| Capability | Gates | Notes |
|---|---|---|
| `fleet.LedgerTail` streaming RPC | T16.3 production-shape billing | Currently mocked via `SyntheticLedgerPoller` in WS-14; gates real-time usage emission |
| Worker self-update Tirith policy | Hermes-on-worker mode (latent in `ROADMAP.md` Layer-2 P7 from hermes-setup era) | Required if a worker ever runs its own agent loop |
| Shared write target for swarm fan-out | T11.x ETL-shape workloads | Required for genuine ledger-coordinated multi-node concurrent appends |
| Apple-toolchain hardening | V3 `xcode-ci` + `app-build` | Includes signed-build provenance + provisioning-profile rotation |
| Per-target SSH key split | n_workers ≥ 4 with tag-routing | Operator-side hygiene; not customer-visible |

## Out of scope until 18 months+

These are deliberate non-goals. Re-evaluate at the 18-month mark or when the corresponding signal arrives.

- **Sustained x86 native** beyond what the Mac fleet handles. Most non-Apple commercial-software builds need this; defer until SOC 2 Type 2 + a paid customer asking specifically.
- **Heavy GPU** for training workloads. The Hermes fleet is optimised for orchestration, not parallel compute.
- **HIPAA / PCI regulated data flows.** Both demand audit + insurance investments out of proportion to early-stage revenue. SOC 2 Type 2 is the entry ramp.
- **On-prem deployment** of the operator plane. Demand-gated on enterprise spend; would split the codebase between SaaS and shipped binaries.

## Process

This document is reviewed quarterly by PM + TL. Each review:

1. Audit each "promotion driver" claim against `customer-discovery-notes.md` (private repo).
2. Move SKUs forward, backward, or out based on signal.
3. Update the V2-V4 tables with new demand signals or remove SKUs that signal told us nobody wants.
4. Commit with `roadmap: <quarter> review` and tag the approvers in the PR.

Anything in V0-V2 is committed to. V3-V4 entries are intent, not promise.

## Change log

| Date | Author | Change |
|---|---|---|
| 2026-05-17 | TL (Wave 8.1) | Initial publication for T13.7 closeout; ramp matches master-plan §"SKU rollout sequence" |
