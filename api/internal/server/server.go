// Package server wires the API together: chi router, middleware chain,
// route registration, and the /healthz and /readyz probes. Handlers
// read the tenant_id from context (set by the auth middleware) and use
// the package-level Store/Dispatcher/etc to do their work.
package server

import (
	"encoding/json"
	"errors"
	"net/http"

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
	Identity   *identity.Store
	Jobs       *jobs.Store
	Queue      queue.Queue
	Dispatcher *dispatcher.Dispatcher
	Audit      *audit.Log
	Billing    *billing.Service
	Status     *status.Service
	Erasure    *erasure.Service
}

// New returns the http.Handler with all routes registered.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	// Authentication: every endpoint except /healthz, /readyz, and the
	// public /v1/skus listing requires a Bearer api key.
	skip := func(p string) bool {
		switch p {
		case "/healthz", "/readyz", "/v1/skus":
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

	// LGPD Art. 18 / GDPR Art. 17 right-to-erasure. Returns 202 with
	// the request id; the actual purge runs in the erasure worker
	// against the durable queue. See compliance/runbooks/customer-
	// data-deletion.md.
	r.Delete("/v1/tenants/me/data", deleteTenantData(d))
	r.Get("/v1/tenants/me/data/erasures/{id}", getErasure(d))

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
			_ = d.Audit.Append(r.Context(), audit.FromRequest(r, tenID, "", "job.submit", j.ID))
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
			_ = d.Audit.Append(r.Context(), audit.FromRequest(r, tenID, "", "job.cancel", j.ID))
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
			"name":     s.Name,
			"version":  s.Version,
			"category": s.Category,
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

func getUsage(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenID := identity.TenantID(r.Context())
		period := r.URL.Query().Get("period")
		if period == "" {
			period = "current"
		}
		var meters map[string]int64
		if d.Billing != nil {
			m, err := d.Billing.UsageByTenant(r.Context(), tenID)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			meters = m
		} else {
			meters = map[string]int64{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"period": period,
			"meters": meters,
		})
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
		// auth.Middleware only stamps tenant_id today; user_id will
		// land when WS-15 extends the identity context. Until then
		// the audit row carries an empty user_id for self-service
		// erasures (still attributable via api_key prefix in the
		// auth log).
		userID := ""
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

