// Command apiserver runs the Owera Cloud customer API on :8080. It wires
// the SQLite-backed identity/jobs/queue/audit/billing stores, the
// dispatcher transport, the status service, and the chi router; then
// serves HTTP with graceful shutdown on SIGINT/SIGTERM.
//
// Production wiring is env-driven so the same binary boots in dev (all
// fakes) or prod (Stripe + LedgerTail) without recompiling:
//
//   - STRIPE_SECRET_KEY set → real *billing.StripeBackend backed by
//     the identity-store customer resolver (and the same backend is
//     reused as the BillingPortalMinter). Unset → *billing.FakeBackend.
//   - OPERATOR_RPC_URL set → real dispatcher.LedgerTailClient pointed
//     at the Cloudflare-tunnel JSON-RPC endpoint. Unset →
//     dispatcher.SyntheticLedgerPoller.
//
// Both unset is the all-fakes dev mode and matches the pre-wire-up
// behaviour.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/auth"
	"github.com/owera/owera-cloud/api/internal/billing"
	"github.com/owera/owera-cloud/api/internal/catalog"
	"github.com/owera/owera-cloud/api/internal/dispatcher"
	"github.com/owera/owera-cloud/api/internal/erasure"
	"github.com/owera/owera-cloud/api/internal/identity"
	"github.com/owera/owera-cloud/api/internal/jobs"
	"github.com/owera/owera-cloud/api/internal/queue"
	"github.com/owera/owera-cloud/api/internal/server"
	"github.com/owera/owera-cloud/api/internal/status"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "./owera-cloud.db", "SQLite database path")
	flag.Parse()

	if err := run(*addr, *dbPath); err != nil {
		log.Fatalf("apiserver: %v", err)
	}
}

// wiring captures the env-driven backend choices so run() and the tests
// can describe them with one log line.
type wiring struct {
	billing       billing.Backend
	billingLabel  string                 // "stripe" | "fake"
	stripeBackend *billing.StripeBackend // non-nil only when billingLabel=="stripe"; used as StripeUsageReporter for the drift reconciler
	portal        server.BillingPortalMinter
	ledger        dispatcher.LedgerPoller
	ledgerLabel   string // "tunnel (<url>)" | "synthetic"
	transport     dispatcher.Transport
	statusFetcher status.Fetcher // nil → fall back to transport-decode path
	rpcLabel      string         // "tunnel (<url>)" | "in-memory"
	clerk         auth.ClerkAuthenticator
	clerkLabel    string // "clerk (<issuer>)" | "disabled"
	defaultCap    int64  // monthly cost cap when tenant has no override
}

// chooseWiring resolves env-driven production vs dev backends. idStore
// is needed by the Stripe resolver path. Returns an error only when env
// is half-configured in a way that would fail later anyway (e.g.
// STRIPE_SECRET_KEY set but resolver construction fails).
//
// Env vars consumed:
//
//	STRIPE_SECRET_KEY     real Stripe backend; the same backend doubles
//	                      as the BillingPortalMinter
//	OPERATOR_RPC_URL      real LedgerTailClient against the Cloudflare-
//	                      tunnel JSON-RPC endpoint
//	CLERK_JWT_ISSUER      real Clerk verifier for the dashboard JWT path
//	                      (auth middleware accepts both API keys and
//	                      Clerk JWTs)
//	OWERA_DEFAULT_CAP_CENTS  monthly cost-cap default (cents) for
//	                         tenants that haven't set their own;
//	                         defaults to 20000 ($200/mo)
func chooseWiring(idStore *identity.Store) (wiring, error) {
	w := wiring{
		billing:      &billing.FakeBackend{},
		billingLabel: "fake",
		ledger:       dispatcher.NewSyntheticLedgerPoller(),
		ledgerLabel:  "synthetic",
		transport:    dispatcher.NewInMemoryTransport(),
		rpcLabel:     "in-memory",
		clerkLabel:   "disabled",
		defaultCap:   defaultCapCentsFromEnv(),
	}

	if os.Getenv("STRIPE_SECRET_KEY") != "" {
		resolver, err := billing.NewIdentityCustomerResolver(idStore)
		if err != nil {
			return wiring{}, err
		}
		sb, err := billing.NewStripeBackend(resolver)
		if err != nil {
			return wiring{}, err
		}
		w.billing = sb
		w.billingLabel = "stripe"
		w.stripeBackend = sb
		w.portal = sb
	}

	if url := os.Getenv("OPERATOR_RPC_URL"); url != "" {
		w.ledger = dispatcher.NewLedgerTailClient(url, nil)
		w.ledgerLabel = "tunnel (" + url + ")"
		httpTransport := dispatcher.NewHTTPTransport(url, nil)
		w.transport = httpTransport
		w.statusFetcher = status.NewOperatorFetcher(httpTransport)
		w.rpcLabel = "tunnel (" + url + ")"
	}

	if iss := os.Getenv("CLERK_JWT_ISSUER"); iss != "" {
		verifier, err := auth.NewClerkVerifier(context.Background(), iss, nil)
		if err != nil {
			return wiring{}, err
		}
		w.clerk = verifier
		w.clerkLabel = "clerk (" + iss + ")"
	}

	return w, nil
}

// defaultCapCentsFromEnv parses OWERA_DEFAULT_CAP_CENTS. Unset or bad
// input returns the docs/pricing.md baseline of $200/mo (20000 cents).
func defaultCapCentsFromEnv() int64 {
	const fallback int64 = 20000
	v := os.Getenv("OWERA_DEFAULT_CAP_CENTS")
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func run(addr, dbPath string) error {
	idStore, err := identity.Open(dbPath)
	if err != nil {
		return err
	}
	defer idStore.Close()

	jobsStore, err := jobs.New(idStore.DB())
	if err != nil {
		return err
	}
	q, err := queue.NewSQLite(idStore.DB())
	if err != nil {
		return err
	}
	auditLog, err := audit.New(idStore.DB())
	if err != nil {
		return err
	}

	w, err := chooseWiring(idStore)
	if err != nil {
		return err
	}
	log.Printf("apiserver: billing=%s, ledger=%s, rpc=%s, auth=%s, default_cap_cents=%d",
		w.billingLabel, w.ledgerLabel, w.rpcLabel, w.clerkLabel, w.defaultCap)

	billingSvc, err := billing.New(idStore.DB(), w.billing)
	if err != nil {
		return err
	}
	// Route metered vs per-job-fixed SKUs through their respective
	// emit paths (EmitUsage vs EmitOneShot) per catalog.Pricing.Model.
	billingSvc.SetDispatcher(catalog.NewDispatcher())

	// Cost cap is wired against identity.Store (cap source) + catalog
	// pricer. Returns 402 + Retry-After at /v1/jobs when a submission
	// would push the tenant over their monthly cap.
	costCap, err := billing.NewCostCap(billingSvc, idStore, catalog.NewPricer(), w.defaultCap, nil)
	if err != nil {
		return err
	}

	erasureSvc, err := erasure.New(idStore.DB(), erasure.AdaptQueue(q), auditLog)
	if err != nil {
		return err
	}
	disp := dispatcher.New(w.transport)
	var statusSvc *status.Service
	if w.statusFetcher != nil {
		statusSvc = status.NewWithFetcher(w.statusFetcher, 30*time.Second)
	} else {
		statusSvc = status.New(w.transport, 30*time.Second)
	}

	deps := server.Deps{
		Identity:    idStore,
		Jobs:        jobsStore,
		Queue:       q,
		Dispatcher:  disp,
		Audit:       auditLog,
		Billing:     billingSvc,
		CostCap:     costCap,
		BillPortal:  w.portal, // nil unless STRIPE_SECRET_KEY is set
		BillCustLkp: idStore,  // identity.Store satisfies TenantCustomerLookup directly
		ClerkAuth:   w.clerk,  // nil unless CLERK_JWT_ISSUER is set
		Status:      statusSvc,
		Erasure:     erasureSvc,
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	worker := dispatcher.NewWorker(q, disp, jobsStore, w.ledger, dispatcher.DefaultWorkerConfig())
	go worker.Run(workerCtx)

	// Outbox flusher: ticks the billing service's Reconcile, which drains
	// the per-tenant outbox into the configured Backend (Stripe in prod,
	// FakeBackend in dev). The cron-style daily reconciler binary in
	// cmd/reconciler/ does drift detection only; without this in-process
	// ticker the outbox accumulates and Stripe never sees the events.
	reconcileInterval := reconcileIntervalFromEnv()
	go runOutboxFlusher(workerCtx, billingSvc, reconcileInterval)

	// Daily drift detector: compares ledger sums vs Stripe-reported usage
	// per tenant for yesterday's window and alerts on drift > 0.5%. Wired
	// only when Stripe + ledger are both real — synthetic ledger + fake
	// Stripe would always read drift=0 and add no signal.
	reconcilerOn := w.billingLabel == "stripe" && w.ledgerLabel != "synthetic" && w.stripeBackend != nil
	if reconcilerOn {
		rec, err := billing.NewReconciler(billingSvc, w.stripeBackend, &logAlerter{})
		if err != nil {
			return err
		}
		go runDriftReconciler(workerCtx, rec)
		log.Printf("apiserver: reconciler=on (drift detector, daily)")
	} else {
		log.Printf("apiserver: reconciler=off (requires billing=stripe + ledger!=synthetic)")
	}

	h := server.New(deps)
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("apiserver: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		log.Printf("apiserver: received %s, shutting down", sig)
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}

// reconcileIntervalFromEnv parses OWERA_RECONCILE_INTERVAL as a Go
// duration (e.g. "30s", "2m"). Unset or invalid input returns 60s, which
// matches the V0 expectation of "outbox→Stripe latency under a minute"
// without hammering the Stripe API.
func reconcileIntervalFromEnv() time.Duration {
	const fallback = 60 * time.Second
	v := os.Getenv("OWERA_RECONCILE_INTERVAL")
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("apiserver: bad OWERA_RECONCILE_INTERVAL=%q, defaulting to %s", v, fallback)
		return fallback
	}
	return d
}

// runOutboxFlusher ticks billing.Service.Reconcile every interval until
// ctx is cancelled. Each tick flushes any unbilled rows in the outbox
// to the configured Backend. Errors are logged and the loop continues —
// a transient Stripe failure shouldn't take the apiserver down.
func runOutboxFlusher(ctx context.Context, svc *billing.Service, interval time.Duration) {
	log.Printf("apiserver: outbox flusher starting, interval=%s", interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("apiserver: outbox flusher stopping")
			return
		case <-t.C:
			n, err := svc.Reconcile(ctx)
			log.Printf("apiserver: reconciler flushed=%d err=%v", n, err)
		}
	}
}

// runDriftReconciler ticks the daily drift detector. billing.Reconciler.Run
// does one cycle per call (not self-ticking), so wrap in a 24h ticker.
// Each cycle compares the previous-UTC-day window: [yesterday 00:00,
// today 00:00). One reconcile-tenant failure is already swallowed
// internally by the reconciler — only fatal errors propagate up here.
func runDriftReconciler(ctx context.Context, rec *billing.Reconciler) {
	log.Printf("apiserver: drift reconciler starting, interval=24h")
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("apiserver: drift reconciler stopping")
			return
		case <-t.C:
			now := time.Now().UTC()
			yStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
			yEnd := yStart.AddDate(0, 0, 1)
			reports, err := rec.Run(ctx, yStart, yEnd)
			if err != nil {
				log.Printf("apiserver: drift reconciler err=%v", err)
				continue
			}
			alerted := 0
			for _, r := range reports {
				if r.Alerted {
					alerted++
				}
			}
			log.Printf("apiserver: drift reconciler tenants=%d alerted=%d window=%s..%s",
				len(reports), alerted, yStart.Format(time.RFC3339), yEnd.Format(time.RFC3339))
		}
	}
}

// logAlerter is the in-process Alerter used by the daily reconciler.
// It marshals (kind, fields, ts) as one JSONL line on stderr so the
// existing log-rotation/ingestion pipeline picks the alerts up
// unchanged. The cmd/reconciler binary has its own HTTP-POST alerter
// for batch-cron deployments; this one is for the in-apiserver path.
type logAlerter struct{}

// Alert implements billing.Alerter. Errors from json.Marshal would only
// happen for non-marshallable payload values; we surface them rather
// than swallowing so the reconciler can fail-loud and a fix can land.
func (logAlerter) Alert(_ context.Context, kind string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["kind"] = kind
	payload["ts"] = time.Now().UTC().Format(time.RFC3339)
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("logAlerter: marshal: %w", err)
	}
	fmt.Fprintln(os.Stderr, string(buf))
	return nil
}
