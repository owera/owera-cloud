// Command apiserver runs the Owera Cloud customer API on :8080. It wires
// the SQLite-backed identity/jobs/queue/audit/billing stores, a fake
// dispatcher transport (production swap-in is the Cloudflare-tunnel
// client), the status service, and the chi router; then serves HTTP with
// graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/billing"
	_ "github.com/owera/owera-cloud/api/internal/catalog" // registers SKUs
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
	billingSvc, err := billing.New(idStore.DB(), &billing.FakeBackend{})
	if err != nil {
		return err
	}
	erasureSvc, err := erasure.New(idStore.DB(), erasure.AdaptQueue(q), auditLog)
	if err != nil {
		return err
	}
	// Operator-plane transport — scaffold uses the fake; production wires
	// the Cloudflare-tunnel client here. The seam is the Transport
	// interface in internal/dispatcher.
	transport := dispatcher.NewInMemoryTransport()
	disp := dispatcher.New(transport)
	statusSvc := status.New(transport, 30*time.Second)

	deps := server.Deps{
		Identity:   idStore,
		Jobs:       jobsStore,
		Queue:      q,
		Dispatcher: disp,
		Audit:      auditLog,
		Billing:    billingSvc,
		// CostCap / BillPortal / BillCustLkp are intentionally nil in dev
		// — production wires them once the Stripe key + WS-15 identity
		// surfaces (StripeCustomerID + GetMonthlyCapCents) are available.
		Status:  statusSvc,
		Erasure: erasureSvc,
	}

	// Background dispatcher worker. The synthetic ledger poller is the
	// stand-in until the operator plane exposes fleet.LedgerTail; see
	// PR open question.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	worker := dispatcher.NewWorker(q, disp, jobsStore, dispatcher.NewSyntheticLedgerPoller(), dispatcher.DefaultWorkerConfig())
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
