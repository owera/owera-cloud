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
	"errors"
	"flag"
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
	billingLabel  string // "stripe" | "fake"
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
