# Owera Agentic — product roadmap

> **Audience: internal team + select prospects under NDA.** This is the planned ramp; it changes with demand and engineering signal. Source of truth for delivered state is the git log on `main` + the GA gates (`compliance/policies/ga-gate.md`). For the master plan (waves, workstreams, ticket backlog), see [`knowing-all-you-now-calm-leaf.md`](https://github.com/owera/owera-fleet/blob/main/knowing-all-you-now-calm-leaf.md) in the operator-plane repo.

Last updated: 2026-05-17 (late evening — Wave 10 Track A + V0 Stripe demo + public flip).

## What we're working on right now

The single most important question this doc has to answer. Updated whenever the focus shifts; the answer below is current as of the "Last updated" timestamp.

| Thread | State | Owner | Blocker |
|---|---|---|---|
| **V0 end-to-end paid job demoed live in production** — `campaign-swarm@v1` traversed cloud → tunnel → operator → ledger → reconciler → real Stripe InvoiceItem on `cus_UXImEhwCti1Aq6` (test mode) | ✅ Demo'd 2026-05-18 ~00:03 UTC | — | None |
| **Repo flipped public** + free unlimited Actions + GHAS + CodeQL live | ✅ 2026-05-18 ~00:50 UTC | — | None |
| **Wave 10 Track A hardening sprint** — long-running SKURouter, campaign-swarm tier selection, reconciler dead-letter, V1 SKU stubs (research-brief + code-audit), SOC 2 evidence map | ✅ All 7 PRs merged (cloud #43-#46, fleet #25-#27, plus #28 CodeQL) | — | None |
| **Track B (real triage-watch + campaign-swarm)** | ⏳ Specs ready (`owera-fleet/docs/sku-execution-spec.md`) | Engineering on-demand once external creds land | Zendesk / Twitter / LinkedIn / SendGrid dev accounts (operator action) |
| **Operator-plane work** (sibling repo) | ⏩ See [`owera-fleet` roadmap](https://github.com/owera/owera-fleet/blob/main/docs/roadmap.md) | — | — |

> **TL;DR for "where are we at?":** Phases 1, 2, 2.5, 3 + the Track A hardening sprint are all engineering-complete. The platform demoed end-to-end last night: a real Stripe InvoiceItem fired against a real Stripe customer for a real V0 SKU job submission. **Every remaining item is operator-action-bound** — `claw-staging.local` hardware, Stripe live mode, PagerDuty, BR tax/legal, design partner #1.

## Status at a glance

| Wave | Scope | State |
|---|---|---|
| 1–6 | Operator plane (Phases 1–2) | Shipped to `owera-fleet/main` |
| 7 | Phase-3 cloud scaffolding | Shipped to `owera-cloud/main` |
| 8 | Phase-3 core build (WS-14, 15, 16, 17, 18, 19) | Shipped to `owera-cloud/main` |
| 8.1 | Wave-8 follow-ups (PRAGMA, usage shape, user_id, this doc) | Shipped |
| 8.2 | Production wire-up: API Dockerfile + Fly deploy, Clerk JWT + admin endpoints, `/readyz` operator-plane RPC, admin API-key mint, onboarding playbook v2; on fleet side: `fleet.LedgerTail` RPC + snapshot publisher + launchd installer | Shipped |
| 9-A + B1 | Phase-1 verification gate + Phase-2 e2e coverage + reconciler wire-up: 11 PRs (cloud #35 CodeQL, #36 customer docs+SLA, #37 reconciler, #38 lint cleanup; fleet #14 audit-config, #15 swarm e2e, #16 cronjob+alert e2e, #17 markers CLI, #18 drift cleanup, #19 bootstrap phases 1-9, #20 state parity) | Shipped |
| 9-C | Live verification gauntlet + hermes-setup cutover | Engineering ready; gated on `claw-staging.local` Mac procurement (operator) |
| WS-A / WS-A.1 | First-V0-SKU end-to-end execution: stub routers register in `fleet.SubmitJob`; bill-event subscriber bridges operator ledger → cloud outbox; reconciler dead-letter policy + skip-and-continue; tier-letter meter convention; **real Stripe InvoiceItem demoed in production 2026-05-18 ~00:03 UTC** (cloud PRs #37 #40 #41 #42, fleet PR #24) | Shipped |
| 10-A | Hardening sprint: long-running SKURouter; tier selection; reconciler dead-letter; V1 SKU stubs; SOC 2 evidence map; CodeQL on fleet repo | Shipped — cloud PRs #43-#46, fleet PRs #25-#28 |
| Public flip | `owera/owera-cloud` + `owera/owera-fleet` flipped to public 2026-05-18; CodeQL + GHAS + unlimited Actions all active | Shipped |
| 10-B | Real `triage-watch` (WS-B) + real `campaign-swarm` (WS-C) | Specs ready; engineering blocked on external API credentials (Zendesk / Twitter / LinkedIn / SendGrid dev accounts) |
| 10-C | Operator-action items: claw-staging.local procurement, Stripe live-mode + product cleanup, PagerDuty drill (T19.6), BR tax accountant, BR SaaS lawyer + DPA/SCC, design partner #1 (T20.1) | Planned — every item is operator-action, no engineering blocked |
| 10-D | Cutover gauntlet (C1 staging bootstrap → C2 e2e gauntlet → C3 hermes-setup archive) | Engineering ready; gated on 10-C `claw-staging.local` |
| 11 | Beta → GA decision | Gated on GA policy (6 gates in `compliance/policies/ga-gate.md`) |
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
| 2026-05-17 (PM) | TL (Wave 8.2) | Status table updated: Waves 8 + 8.1 marked Shipped; new Wave 8.2 row covers production wire-up across both repos (cloud PRs #25–#32, fleet PRs #7–#11). Wave 9 gating clarified — Stripe account cleanup, `owera-cloud` repo visibility flip, and `claw-staging.local` provisioning are operator-action blockers, not engineering. See master-plan execution log for the per-PR breakdown. |
| 2026-05-17 (later PM) | Claude (under Rodrigo) | Added "What we're working on right now" stanza at the top so the answer to "where are we at?" is on the first screen, not buried in a table. Mirrors the equivalent stanza in `owera-fleet/docs/roadmap.md` (created the same evening). |
| 2026-05-17 (evening) | Claude (under Rodrigo) | Wave 9-A + B1 close-out. 11 PRs merged across both repos (cloud #35 CodeQL, #36 customer docs+SLA, #37 reconciler wire-up + Fly deploy verified, #38 lint cleanup; fleet #14 audit-config, #15/#16/#17 real e2e scenarios, #18 drift cleanup, #19 bootstrap phases 1-9, #20 state parity). Production billing emission live: `apiserver: reconciler=on (drift detector, daily)` in boot log. Wave 9 row renamed to "9-A + B1"; added 9-C row for verification gauntlet + cutover (gated on `claw-staging.local`). Now operator-action-bound on every remaining Phase-4 item. |
| 2026-05-17 (late evening) | Claude (under Rodrigo) | WS-A + WS-A.1 + Track A + V0 Stripe demo + public flip. PRs landed: cloud #40 (WS-A.1 bill subscriber), #41 (reconciler skip-and-continue), #42 (stripe.Key global), #43 (V1 SKU stubs), #44 (H3 dead-letter), #45 (H5 SOC 2 evidence map), #46 (test loosener); fleet #23 (WS-A stub routers), #24 (campaign-swarm tier-letter meter), #25 (H2 tier selection), #26 (H1 long-running SKURouter), #27 (H4 operator stubs), #28 (CodeQL workflow). **Real Stripe InvoiceItem fired 2026-05-18 ~00:03 UTC** on `cus_UXImEhwCti1Aq6` from a live `campaign-swarm@v1` job — Phase-3 verification step 10 demonstrably closed. Both repos flipped public; CodeQL active on both with zero day-one findings on owera-fleet. Engineering-side critical path is fully closed; only operator actions remain. |
