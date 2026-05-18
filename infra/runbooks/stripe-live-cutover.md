# Stripe test → live cutover

Switch the apiserver from Stripe **test mode** (current production state) to Stripe **live mode** so paying customers can be charged. Splits into three commits + one secrets rotation + one smoke.

**Owner:** CFO (rotation cadence per `infra/secrets-manifest.md`).
**Prerequisites:** Stripe **live** account access (Owera Fleet, not Piton Tec); 1Password vault `Owera / Cloud Plane`.
**Estimated time:** 60 minutes including smoke.
**Pre-checked stale items:** the two prior Piton Tec live products (`prod_UWxqPxgISCp6QI` triage-watch, `prod_UWxqGIt0waFuwb` campaign-swarm, and the misfire `prod_UX4s2ufwHsFhMV`) are already archived per `api/internal/billing/stripe_ids.go:26-32` cleanup record. No additional Stripe-console cleanup is needed.

---

## 1. Pre-flight

Confirm the live Stripe account is the right one:

```bash
fly secrets list --app owera-agentic-api | grep STRIPE || echo "(none set yet — expected for first cutover)"
# Decide: is this the first cutover (no live keys ever), or a key rotation?
```

Make sure the team-permission inventory in Stripe Dashboard → Team grants the operator the `Developer` and `Administrator` roles in the live account. View-only is not sufficient.

Open both Stripe dashboards side-by-side: **test** (to copy specs) and **live** (to recreate). Toggle is the upper-left "Test mode" switch.

---

## 2. Recreate the SKU catalog in live mode

The current `api/internal/billing/stripe_ids.go` `StripeRefs` table has 5 entries, all `Mode: "test"`. Each needs an equivalent live-mode product + price.

For each entry, in the Stripe **live** dashboard:

| Owera ref | Product type | Price |
|---|---|---|
| `triage-watch:base` | Service, name "Owera triage-watch (base)" | $499/month, **recurring licensed** |
| `triage-watch:ticket` | Same product as above (re-attach) | $2 per ticket, **recurring metered**, units `ticket` |
| `campaign-swarm:S` | Service, name "Owera campaign-swarm" | $499 **one-time** |
| `campaign-swarm:M` | same product | $999 **one-time** |
| `campaign-swarm:L` | same product | $1,999 **one-time** |

For the metered price (`triage-watch:ticket`):

1. **Create a Billing Meter** named `tickets_processed` (Stripe Dashboard → Billing → Meters → Create).
2. Note the meter ID (will look like `mtr_live_…`); this replaces `MeterTriageWatchTickets` in `stripe_ids.go:38`.
3. When creating the metered price, link it to this meter.

Capture all the live IDs in a temporary text file `live-ids.txt`:

```
MeterTriageWatchTickets = "mtr_live_..."
triage-watch:base      prod_..._live  price_..._live
triage-watch:ticket    prod_..._live  price_..._live
campaign-swarm:S       prod_..._live  price_..._live
campaign-swarm:M       prod_..._live  price_..._live
campaign-swarm:L       prod_..._live  price_..._live
```

---

## 3. Land the live IDs (PR #1)

The cleanest cutover is env-driven: add a `Mode` selector that picks live vs test refs at boot. Alternative (simpler, lossier): swap the whole table in-place and append a new cleanup-record comment.

**Recommended:** add a `StripeRefsLive` table alongside `StripeRefs` (test) and have `LookupRef` pick between them based on a flag.

Sketch:

```go
// In api/internal/billing/stripe_ids.go, alongside the existing StripeRefs:
var StripeRefsLive = []StripeRef{
    { OweraRef: "triage-watch:base",   ProductID: "prod_…", PriceID: "price_…", Mode: "live" },
    { OweraRef: "triage-watch:ticket", ProductID: "prod_…", PriceID: "price_…", Mode: "live" },
    { OweraRef: "campaign-swarm:S",    ProductID: "prod_…", PriceID: "price_…", Mode: "live" },
    { OweraRef: "campaign-swarm:M",    ProductID: "prod_…", PriceID: "price_…", Mode: "live" },
    { OweraRef: "campaign-swarm:L",    ProductID: "prod_…", PriceID: "price_…", Mode: "live" },
}
const MeterTriageWatchTicketsLive = "mtr_live_…"

// LookupRef respects the OWERA_STRIPE_MODE env (set in api/main.go boot
// path). Default is "test" so dev + staging keep behaviour unchanged.
```

Wire `OWERA_STRIPE_MODE` parsing in `api/cmd/apiserver/main.go` `chooseWiring()` alongside `STRIPE_SECRET_KEY`. Add a new `wiring` field `stripeMode string` and surface it in the boot-log fingerprint (`stripe-mode=live`).

Open a PR, run the existing billing tests with both modes (`OWERA_STRIPE_MODE=test` and `=live` against test keys — won't actually hit live), and merge.

---

## 4. Rotate the keys (PR #2 is a no-code secrets rotation)

Generate the live secret key in Stripe **live** dashboard → Developers → API keys. Capture as **restricted key** rather than full secret if scopes allow (current usage only needs InvoiceItem + UsageRecord + Customer + Subscription writes).

Generate the live webhook signing secret in Stripe **live** → Developers → Webhooks → endpoint for `https://owera-agentic-api.fly.dev/v1/webhooks/stripe`. If no live-mode webhook endpoint exists, create one now with the same event subscriptions as the test endpoint (see `api/internal/webhooks/` for the handled event types).

Generate the live restricted reporting key (Stripe → Developers → Restricted keys → "Reporting" template).

Apply all three together — Fly redeploys after each `secrets set`:

```bash
fly secrets set --app owera-agentic-api \
  STRIPE_SECRET_KEY=sk_live_… \
  STRIPE_WEBHOOK_SECRET=whsec_… \
  STRIPE_RESTRICTED_KEY_REPORTING=rk_live_… \
  OWERA_STRIPE_MODE=live
```

Watch the redeploy:

```bash
fly logs --app owera-agentic-api | grep -E "apiserver: billing="
# expect:
#   apiserver: billing=stripe, ..., stripe-mode=live, ...
```

Mirror the same values into 1Password vault (`Owera / Cloud Plane` → "Stripe live API keys 2026-MM-DD") for break-glass recovery.

---

## 5. Smoke (real money, $499 + $2 metered)

Pick one design partner or sentinel customer with a **real** payment method on file in the live account. (Do *not* smoke against a stranger; do *not* smoke against `cus_UXImEhwCti1Aq6` — that customer is in TEST mode.)

```bash
# Trigger one paid job submission per SKU through the production API:
curl -X POST https://owera-agentic-api.fly.dev/v1/jobs \
  -H "Authorization: Bearer owc_…" \
  -H "Content-Type: application/json" \
  -d '{"sku":"triage-watch","variant":"base"}'
```

Verify in Stripe **live** dashboard:

- `Invoices → Drafts` shows a new line item under the design-partner customer.
- For `triage-watch:ticket` (metered), `Billing → Meters → tickets_processed` shows the event count incrementing within ~60 s (the apiserver's outbox flusher tick).
- The boot-log line `reconciler=on (drift detector, daily)` is present (the 24 h drift reconciler now runs against live data).

If anything is off:

- Outbox stuck → `fly logs --app owera-agentic-api | grep reconciler`
- Auth fails → confirm `sk_live_…` not `sk_test_…` in `fly secrets list`
- Wrong product → confirm `OWERA_STRIPE_MODE=live` in `fly secrets list` and that the boot-log fingerprint says `stripe-mode=live`

---

## 6. Rollback

Single secret flip reverts the cutover without redeploying code:

```bash
fly secrets set --app owera-agentic-api \
  STRIPE_SECRET_KEY=sk_test_… \
  OWERA_STRIPE_MODE=test
# (leave the live webhook + restricted keys set — they're no-ops without sk_live)
```

PR #1 (the `StripeRefsLive` table) stays merged; the env flip is the cutover.

If a charge needs reversing in live mode, do it in Stripe dashboard (refund the invoice). Do **not** delete the invoice — that breaks the audit trail the daily drift reconciler depends on.

---

## 7. Update `infra/secrets-manifest.md`

Mark the three Stripe rows `[x]` and add the rotation date in the commit. The manifest is the source of truth for the quarterly rotation audit.

---

## Cross-references

- Existing test-mode catalog: `api/internal/billing/stripe_ids.go`.
- StripeBackend: `api/internal/billing/stripe_backend.go` (UsageRecord + Portal entry points).
- Webhook handlers: `api/internal/webhooks/`.
- Drift reconciler: `api/cmd/reconciler/` + in-process daily tick at `api/cmd/apiserver/main.go` `runDriftReconciler`.
- Secrets manifest: `infra/secrets-manifest.md`.
- Audit ship to S3 (independent): `infra/runbooks/audit-s3-bucket.md`.
