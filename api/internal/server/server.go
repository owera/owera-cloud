// Package server wires the API together: chi router, middleware chain,
// route registration, and the /healthz and /readyz probes. Handlers
// read the tenant_id from context (set by the auth middleware) and use
// the package-level Store/Dispatcher/etc to do their work.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/auth"
	"github.com/owera/owera-cloud/api/internal/billing"
	"github.com/owera/owera-cloud/api/internal/catalog"
	"github.com/owera/owera-cloud/api/internal/dispatcher"
	"github.com/owera/owera-cloud/api/internal/erasure"
	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
	"github.com/owera/owera-cloud/api/internal/status"
)

// Deps bundles the dependencies the server handler chain needs. main()
// constructs one and passes it to New().
type Deps struct {
	Identity    *identity.Store
	Jobs        *jobs.Store
	Queue       queue.Queue
	Dispatcher  *dispatcher.Dispatcher
	Audit       *audit.Log
	Billing     *billing.Service
	CostCap     *billing.CostCap     // WS-16 T16.5; optional in dev (nil → uncapped)
	BillPortal  BillingPortalMinter  // WS-16 T16.2; optional in dev (nil → 503)
	BillCustLkp TenantCustomerLookup // WS-16 T16.2; optional in dev (nil → 503)
	Status      *status.Service
	Erasure     *erasure.Service // WS-18 T18.2; LGPD/GDPR right-to-erasure
}

// BillingPortalMinter is the surface the /v1/billing/portal handler needs.
// Implemented in production by *billing.StripeBackend; tests stub it.
type BillingPortalMinter interface {
	PortalSessionURL(ctx context.Context, stripeCustomerID, returnURL string) (string, error)
}

// TenantCustomerLookup resolves a tenant_id to its Stripe customer_id.
// WS-15 (identity) eventually exposes this; until then the Deps wiring may
// pass nil and the portal handler responds 503.
type TenantCustomerLookup interface {
	StripeCustomerID(ctx context.Context, tenantID string) (string, error)
}

// New returns the http.Handler with all routes registered.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	// Authentication: every endpoint except /healthz, /readyz, the
	// public /v1/skus listing, and the operator-only /v1/admin/* surface
	// requires a Bearer api key. Admin paths run their own bearer-token
	// check (see admin.go) and intentionally skip tenant-resolution since
	// they operate above the tenant boundary.
	skip := func(p string) bool {
		switch p {
		case "/healthz", "/readyz", "/v1/skus":
			return true
		}
		if len(p) >= 10 && p[:10] == "/v1/admin/" {
			return true
		}
		return false
	}
	r.Use(auth.Middleware(d.Identity, skip))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// DB ping
		if err := d.Identity.DB().PingContext(r.Context()); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		// Dispatcher transport reachability is a soft check via the status
		// service. If status is unconfigured we treat as ready (transport
		// may not be wired in local dev).
		if d.Status != nil {
			if !d.Status.Ready(r.Context()) {
				http.Error(w, "fleet not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	r.Get("/v1/skus", listSKUs)

	r.Route("/v1/jobs", func(r chi.Router) {
		r.Post("/", postJob(d))
		r.Get("/", listJobs(d))
		r.Get("/{id}", getJob(d))
		r.Post("/{id}/cancel", cancelJob(d))
	})

	r.Get("/v1/usage", getUsage(d))
	r.Post("/v1/billing/portal", postBillingPortal(d))

	// LGPD Art. 18 / GDPR Art. 17 right-to-erasure. Returns 202 with
	// the request id; the actual purge runs in the erasure worker
	// against the durable queue. See compliance/runbooks/customer-
	// data-deletion.md.
	r.Delete("/v1/tenants/me/data", deleteTenantData(d))
	r.Get("/v1/tenants/me/data/erasures/{id}", getErasure(d))

	// Operator-only admin surface — tenant create, user create, set
	// Stripe customer, set monthly cap. Guarded by OWERA_ADMIN_TOKEN.
	// See compliance/runbooks/onboarding-playbook.md.
	registerAdmin(r, d)

	return r
}

// --- handlers ---

type jobCreateReq struct {
	SKU            string         `json:"sku"`
	Inputs         map[string]any `json:"inputs"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

type jobCreateResp struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

func postJob(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		if tenID == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing tenant")
			return
		}
		var req jobCreateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.SKU == "" {
			writeErr(w, http.StatusBadRequest, "bad_request", "sku required")
			return
		}
		sku, err := catalog.Lookup(req.SKU)
		if err != nil {
			writeErr(w, http.StatusNotFound, "sku_not_found", err.Error())
			return
		}
		if err := sku.ValidateInputs(req.Inputs); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_inputs", err.Error())
			return
		}
		// WS-16 T16.5: cost cap at submission boundary. 402 is the
		// semantic match ("Payment Required") — not 403 (forbidden) or
		// 429 (rate-limited) — because the cap is a money decision the
		// customer can raise from the dashboard.
		if d.CostCap != nil {
			if err := d.CostCap.Enforce(r.Context(), tenID, sku.FullName(), req.Inputs); err != nil {
				var capErr *billing.CapExceededError
				if errors.As(err, &capErr) {
					retry := int(time.Until(capErr.RetryAfter).Seconds())
					if retry < 1 {
						retry = 1
					}
					w.Header().Set("Retry-After", strconv.Itoa(retry))
					writeErr(w, http.StatusPaymentRequired, "cost_cap_exceeded",
						capErr.Error())
					return
				}
				writeErr(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
		}
		j, _, err := d.Jobs.Submit(r.Context(), tenID, sku.FullName(), req.Inputs, req.IdempotencyKey)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		// Enqueue for the dispatcher worker. The cloud-job-id doubles as
		// the enqueue idempotency key so retries don't double-queue.
		if _, _, err := d.Queue.Enqueue(r.Context(), tenID, j.ID,
			map[string]any{"sku": j.SKU, "inputs": j.Inputs}, j.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, "queue", err.Error())
			return
		}
		// Move submitted -> queued so the public state is consistent.
		if _, err := d.Jobs.Transition(r.Context(), tenID, j.ID, jobs.StatusQueued); err != nil {
			// Not fatal — the job exists; subsequent worker will retry.
			_ = err
		}
		if d.Audit != nil {
			_ = d.Audit.Append(r.Context(), audit.FromRequest(r, tenID, identity.UserID(r.Context()), "job.submit", j.ID))
		}
		writeJSON(w, http.StatusAccepted, jobCreateResp{JobID: j.ID, Status: string(jobs.StatusQueued)})
	}
}

func listJobs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		p := jobs.ListParams{
			TenantID: tenID,
			Status:   jobs.Status(r.URL.Query().Get("status")),
			SKU:      r.URL.Query().Get("sku"),
			Cursor:   r.URL.Query().Get("cursor"),
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			var n int
			_, _ = fmtSscanf(l, &n)
			p.Limit = n
		}
		js, next, err := d.Jobs.List(r.Context(), p)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		out := map[string]any{
			"jobs":        jobsToWire(js),
			"next_cursor": next,
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getJob(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		id := chi.URLParam(r, "id")
		j, err := d.Jobs.Get(r.Context(), tenID, id)
		if err != nil {
			if errors.Is(err, jobs.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not_found", "job not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, jobToWire(j))
	}
}

func cancelJob(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		id := chi.URLParam(r, "id")
		j, err := d.Jobs.Get(r.Context(), tenID, id)
		if err != nil {
			if errors.Is(err, jobs.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not_found", "job not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if j.Status.IsTerminal() {
			writeErr(w, http.StatusConflict, "conflict", "job already terminal")
			return
		}
		// Best-effort operator-plane cancel if we already dispatched.
		if j.OperatorTaskID != "" && d.Dispatcher != nil {
			_ = d.Dispatcher.Cancel(r.Context(), j.OperatorTaskID)
		}
		if _, err := d.Jobs.Transition(r.Context(), tenID, j.ID, jobs.StatusCancelled); err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if d.Audit != nil {
			_ = d.Audit.Append(r.Context(), audit.FromRequest(r, tenID, identity.UserID(r.Context()), "job.cancel", j.ID))
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	}
}

func listSKUs(w http.ResponseWriter, _ *http.Request) {
	skus := catalog.List()
	wire := make([]map[string]any, 0, len(skus))
	for _, s := range skus {
		var schema map[string]any
		_ = json.Unmarshal([]byte(s.InputsSchema), &schema)
		wire = append(wire, map[string]any{
			"name":          s.Name,
			"version":       s.Version,
			"category":      s.Category,
			"inputs_schema": schema,
			"pricing": map[string]any{
				"model":        s.Pricing.Model,
				"base_cents":   s.Pricing.BaseCents,
				"overage_rule": s.Pricing.OverageRule,
			},
			"sla": map[string]any{
				"description":         s.SLA.Description,
				"max_latency_seconds": s.SLA.MaxLatencySeconds,
			},
			"billing_meter": s.BillingMeter,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"skus": wire})
}

type billingPortalReq struct {
	ReturnURL string `json:"return_url"`
}

type billingPortalResp struct {
	URL string `json:"url"`
}

func postBillingPortal(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		if tenID == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing tenant")
			return
		}
		if d.BillPortal == nil || d.BillCustLkp == nil {
			writeErr(w, http.StatusServiceUnavailable, "billing_unconfigured",
				"stripe portal not wired in this build")
			return
		}
		var req billingPortalReq
		_ = json.NewDecoder(r.Body).Decode(&req) // body optional
		if req.ReturnURL == "" {
			req.ReturnURL = "https://app.owera.ai/billing"
		}
		custID, err := d.BillCustLkp.StripeCustomerID(r.Context(), tenID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if custID == "" {
			writeErr(w, http.StatusConflict, "no_stripe_customer",
				"tenant has no Stripe customer; complete onboarding first")
			return
		}
		url, err := d.BillPortal.PortalSessionURL(r.Context(), custID, req.ReturnURL)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "stripe_portal", err.Error())
			return
		}
		if d.Audit != nil {
			_ = d.Audit.Append(r.Context(), audit.FromRequest(r, tenID, identity.UserID(r.Context()), "billing.portal.open", custID))
		}
		writeJSON(w, http.StatusOK, billingPortalResp{URL: url})
	}
}

func getUsage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		period := r.URL.Query().Get("period")
		if period == "" {
			period = "current"
		}
		start, end := currentBillingPeriod(time.Now().UTC())

		var rawJobs map[string]int64
		if d.Billing != nil {
			m, err := d.Billing.UsageByTenant(r.Context(), tenID)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			rawJobs = m
		} else {
			rawJobs = map[string]int64{}
		}

		// Compose the meter map in the shape the dashboard's adaptUsage
		// expects: per-SKU `<sku>:jobs` + `<sku>:cost_cents`, plus rolled-up
		// `total_jobs` + `total_cost_cents`. Cost is computed from the
		// catalog's PricingTier; billing-outbox rows carry units only.
		meters := make(map[string]int64, len(rawJobs)*2+2)
		var totalJobs, totalCostCents int64
		for sku, jobs := range rawJobs {
			meters[sku+":jobs"] = jobs
			totalJobs += jobs
			cost := costForSKU(sku, jobs)
			meters[sku+":cost_cents"] = cost
			totalCostCents += cost
		}
		meters["total_jobs"] = totalJobs
		meters["total_cost_cents"] = totalCostCents

		writeJSON(w, http.StatusOK, map[string]any{
			"period":       period,
			"period_start": start.Format(time.RFC3339),
			"period_end":   end.Format(time.RFC3339),
			"meters":       meters,
		})
	}
}

// currentBillingPeriod returns the [start, end] of the calendar month
// containing now. Owera bills on calendar-month boundaries (per
// docs/pricing.md). end is the last second of the month.
func currentBillingPeriod(now time.Time) (start, end time.Time) {
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0).Add(-time.Second)
	return start, end
}

// costForSKU computes the period cost for jobs of one SKU. Returns 0 for
// unregistered SKUs (e.g., a SKU was deregistered between bill events).
func costForSKU(sku string, jobs int64) int64 {
	s, err := catalog.Lookup(sku)
	if err != nil {
		return 0
	}
	switch s.Pricing.Model {
	case "metered":
		return jobs * s.Pricing.BaseCents
	case "monthly_subscription":
		// Flat subscription cost regardless of job count for the period;
		// overage rules are SKU-specific and applied by the reconciler.
		return s.Pricing.BaseCents
	default:
		return 0
	}
}

func deleteTenantData(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		if tenID == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing tenant")
			return
		}
		if d.Erasure == nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable", "erasure service not configured")
			return
		}
		// auth.Middleware stamps both tenant_id and user_id from the
		// API key binding. When the request comes via the dashboard
		// (Clerk session) the same context carries the Clerk subject
		// as user_id after the dual-auth wiring lands.
		userID := identity.UserID(r.Context())
		ip := ""
		ua := r.UserAgent()
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			ip = xf
		} else {
			ip = r.RemoteAddr
		}
		req, err := d.Erasure.Submit(r.Context(), tenID, userID, ip, ua)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"request_id":   req.ID,
			"state":        req.State,
			"requested_at": req.RequestedAt,
			"sla_due_at":   req.SLADueAt,
			"notice": "LGPD Art. 18 / GDPR Art. 17 erasure scheduled; " +
				"completion within 15 working days. Status: GET /v1/tenants/me/data/erasures/" + req.ID,
		})
	}
}

func getErasure(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		if tenID == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing tenant")
			return
		}
		if d.Erasure == nil {
			writeErr(w, http.StatusServiceUnavailable, "unavailable", "erasure service not configured")
			return
		}
		id := chi.URLParam(r, "id")
		req, err := d.Erasure.Get(r.Context(), tenID, id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if req == nil {
			writeErr(w, http.StatusNotFound, "not_found", "erasure request not found")
			return
		}
		out := map[string]any{
			"request_id":   req.ID,
			"state":        req.State,
			"requested_at": req.RequestedAt,
			"sla_due_at":   req.SLADueAt,
		}
		if req.CompletedAt != nil {
			out["completed_at"] = req.CompletedAt
		}
		if req.Report != nil {
			out["report"] = req.Report
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, detail string) {
	writeJSON(w, status, map[string]string{"error": code, "detail": detail})
}

func jobsToWire(in []*jobs.Job) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, j := range in {
		out = append(out, jobToWire(j))
	}
	return out
}

func jobToWire(j *jobs.Job) map[string]any {
	w := map[string]any{
		"id":           j.ID,
		"status":       string(j.Status),
		"sku":          j.SKU,
		"submitted_at": j.SubmittedAt,
		"updated_at":   j.UpdatedAt,
	}
	if len(j.Outputs) > 0 {
		w["outputs"] = j.Outputs
	}
	if j.Error != "" {
		w["error"] = j.Error
	}
	return w
}

// fmtSscanf wraps fmt.Sscanf to avoid a top-level fmt import just for the
// limit parser. Defined here so the handler stays compact.
func fmtSscanf(s string, dst *int) (int, error) {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	*dst = n
	return 1, nil
}
