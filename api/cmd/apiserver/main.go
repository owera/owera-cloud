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

	"github.com/owera/owera-cloud/api/internal/alerting"
	"github.com/owera/owera-cloud/api/internal/audit"
	"github.com/owera/owera-cloud/api/internal/audit/tamperdetect"
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
	auditStreamer audit.WORMStreamer
	auditLabel    string // "s3 (<bucket>@<region>, <retention>d)" | "sqlite-only"
	auditS3Reader *tamperdetect.S3EntryReader
	tamperFlag    time.Duration // 0 disables; default 24h. Set via AUDIT_TAMPER_DETECT_INTERVAL.
	tamperLabel   string        // "on (24h, full)" | "on (24h, continuity-only)" | "off"
	alertRouter   *alerting.Router
	alertLabel    string // "log-only" | "log+pagerduty"
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
//	OWERA_AUDIT_S3_BUCKET    enables WORM audit shipping to S3 with
//	                         Object Lock (Governance mode). When set,
//	                         each audit_log row is PUT to
//	                         s3://<bucket>/audit/<tenant>/<YYYY-MM-DD>/<hash>.json
//	                         with x-amz-object-lock-mode=GOVERNANCE and
//	                         a retain-until-date of now+RetentionDays.
//	                         Unset → SQLite-only (the WORM triggers
//	                         still enforce append-only at the DB).
//	OWERA_AUDIT_S3_REGION    default "us-east-1"
//	OWERA_AUDIT_S3_ENDPOINT  default "https://s3.<region>.amazonaws.com";
//	                         override for MinIO or other S3-compatible
//	                         stores
//	OWERA_AUDIT_S3_RETENTION_DAYS  int, default 2555 (7y per SOC2/HIPAA)
//	AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY (or IMDS / SSO chain)
//	                         supply the SigV4 credentials
//	PAGERDUTY_INTEGRATION_KEY  enables PagerDuty Events API v2 as a
//	                         parallel alert channel behind the always-on
//	                         logAlerter. When set, the drift reconciler's
//	                         multi-alerter fans out CRITICAL alerts to
//	                         both stderr JSONL (audit stream) and
//	                         PagerDuty. PagerDuty outages never silence
//	                         the local emission. See
//	                         compliance/runbooks/pagerduty-setup.md §5.
//	AUDIT_TAMPER_DETECT_INTERVAL  Go duration (e.g. "24h", "1h"); empty
//	                         defaults to 24h, "0" disables the cron
//	                         entirely. When enabled, the apiserver runs
//	                         a daily tamper-detection pass that checks
//	                         completeness (every SQLite audit row has a
//	                         matching S3 WORM object), integrity
//	                         (sha256(object) == row.hash), and
//	                         continuity (no rowid gaps). Findings fan
//	                         out as CRITICAL alerts through the same
//	                         multiAlerter the drift reconciler uses. If
//	                         OWERA_AUDIT_S3_BUCKET is unset, the cron
//	                         runs in continuity-only mode.
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
		auditLabel:   "sqlite-only",
		tamperFlag:   tamperIntervalFromEnv(),
		tamperLabel:  "off",
		alertLabel:   "log-only",
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

	if bucket := os.Getenv("OWERA_AUDIT_S3_BUCKET"); bucket != "" {
		region := os.Getenv("OWERA_AUDIT_S3_REGION")
		if region == "" {
			region = "us-east-1"
		}
		endpoint := os.Getenv("OWERA_AUDIT_S3_ENDPOINT")
		if endpoint == "" {
			endpoint = "https://s3." + region + ".amazonaws.com"
		}
		retention := auditRetentionDaysFromEnv()
		httpClient, err := audit.NewSigV4HTTPClient(context.Background(), region, "s3")
		if err != nil {
			return wiring{}, fmt.Errorf("audit s3: %w", err)
		}
		w.auditStreamer = &audit.S3WORMStreamer{
			HTTPClient:    httpClient,
			Endpoint:      endpoint,
			Bucket:        bucket,
			Region:        region,
			RetentionDays: retention,
		}
		w.auditLabel = fmt.Sprintf("s3 (%s@%s, %dd)", bucket, region, retention)
		// The same SigV4-signing http.Client doubles as the tamper-
		// detect reader's transport — connection pooling stays
		// effective and we don't have to re-resolve AWS creds.
		w.auditS3Reader = &tamperdetect.S3EntryReader{
			HTTPClient: httpClient,
			Endpoint:   endpoint,
			Bucket:     bucket,
		}
	}

	// Compute the tamper-detect label after the S3 block so we know
	// whether the cron will run in full or continuity-only mode.
	if w.tamperFlag > 0 {
		mode := "continuity-only"
		if w.auditS3Reader != nil {
			mode = "full"
		}
		w.tamperLabel = fmt.Sprintf("on (%s, %s)", w.tamperFlag, mode)
	}

	if key := os.Getenv("PAGERDUTY_INTEGRATION_KEY"); key != "" {
		pd, err := alerting.NewPagerDuty(key)
		if err != nil {
			return wiring{}, fmt.Errorf("alerting: %w", err)
		}
		r := alerting.NewRouter()
		r.AddBackend(pd)
		w.alertRouter = r
		w.alertLabel = "log+pagerduty"
	}

	return w, nil
}

// tamperIntervalFromEnv parses AUDIT_TAMPER_DETECT_INTERVAL as a Go
// duration. Empty defaults to 24h; "0" (or any non-positive value)
// disables the cron entirely. Invalid input also defaults to 24h with
// a warning so a typo doesn't silently disable G4.
func tamperIntervalFromEnv() time.Duration {
	const fallback = 24 * time.Hour
	v := os.Getenv("AUDIT_TAMPER_DETECT_INTERVAL")
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("apiserver: bad AUDIT_TAMPER_DETECT_INTERVAL=%q, defaulting to %s", v, fallback)
		return fallback
	}
	if d <= 0 {
		return 0
	}
	return d
}

// auditRetentionDaysFromEnv parses OWERA_AUDIT_S3_RETENTION_DAYS. Unset
// or invalid input returns 2555 (7 y) per common SOC2/HIPAA defaults.
// Compliance reduction below 365 d should be a deliberate decision and
// is rejected at the env layer.
func auditRetentionDaysFromEnv() int {
	const fallback = 2555
	v := os.Getenv("OWERA_AUDIT_S3_RETENTION_DAYS")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 365 {
		log.Printf("apiserver: bad OWERA_AUDIT_S3_RETENTION_DAYS=%q (need int >= 365), defaulting to %d", v, fallback)
		return fallback
	}
	return n
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
	w, err := chooseWiring(idStore)
	if err != nil {
		return err
	}

	var auditOpts []audit.Option
	if w.auditStreamer != nil {
		auditOpts = append(auditOpts, audit.WithStreamer(w.auditStreamer))
	}
	auditLog, err := audit.New(idStore.DB(), auditOpts...)
	if err != nil {
		return err
	}

	log.Printf("apiserver: billing=%s, ledger=%s, rpc=%s, auth=%s, audit=%s, tamper_detect=%s, alerting=%s, default_cap_cents=%d",
		w.billingLabel, w.ledgerLabel, w.rpcLabel, w.clerkLabel, w.auditLabel, w.tamperLabel, w.alertLabel, w.defaultCap)

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
	// billingSvc doubles as the dispatcher.BillRecorder: the worker hands
	// every `phase: "bill"` ledger entry to billing.Service.Record so the
	// outbox flusher below can ship it to Stripe on the next tick. This
	// closes the WS-A.1 gap where bill markers existed in the operator
	// plane ledger but never landed in billing_outbox.
	worker := dispatcher.NewWorker(q, disp, jobsStore, w.ledger, billingSvc, dispatcher.DefaultWorkerConfig())
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
		ma := &multiAlerter{
			log:    logAlerter{},
			router: w.alertRouter, // nil unless PAGERDUTY_INTEGRATION_KEY is set
			source: "owera-agentic-api",
		}
		rec, err := billing.NewReconciler(billingSvc, w.stripeBackend, ma)
		if err != nil {
			return err
		}
		go runDriftReconciler(workerCtx, rec)
		log.Printf("apiserver: reconciler=on (drift detector, daily)")
	} else {
		log.Printf("apiserver: reconciler=off (requires billing=stripe + ledger!=synthetic)")
	}

	// Daily WORM audit tamper detector (launch gate G4). Runs whether
	// or not S3 is wired — without a reader it checks continuity only.
	// AUDIT_TAMPER_DETECT_INTERVAL=0 disables; default 24h matches the
	// drift reconciler cadence so the two cron lines stay visually
	// aligned in the operator's stderr stream.
	if w.tamperFlag > 0 {
		ma := &multiAlerter{
			log:    logAlerter{},
			router: w.alertRouter,
			source: "owera-agentic-api",
		}
		// Note: tamperdetect's keyFn matches audit.WormKey by
		// construction (same format string); we wire through
		// audit.WormKey explicitly so a future change to the WORM
		// layout in one place doesn't silently drift the other.
		var reader tamperdetect.EntryReader
		if w.auditS3Reader != nil {
			reader = w.auditS3Reader
		}
		td, err := tamperdetect.New(idStore.DB(), reader, ma, w.tamperFlag,
			tamperdetect.WithKeyFunc(audit.WormKey),
			tamperdetect.WithSource("owera-agentic-api"))
		if err != nil {
			return err
		}
		go td.Run(workerCtx)
		log.Printf("apiserver: tamper_detect=%s", w.tamperLabel)
	} else {
		log.Printf("apiserver: tamper_detect=off (AUDIT_TAMPER_DETECT_INTERVAL=0)")
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

// multiAlerter is the billing.Alerter the apiserver hands to the daily
// drift reconciler. It always fires the local logAlerter first (so a
// PagerDuty outage never silences the stderr audit emission), then
// optionally fans out to an internal/alerting Router for remote
// channels (PagerDuty today; Slack + OpsGenie in the future).
//
// The fields are intentionally exported in shape (lowercase but
// straightforward) so any test driving the reconciler can substitute
// a fake billing.Alerter without touching this type.
type multiAlerter struct {
	log    billing.Alerter  // always invoked; the stderr JSONL audit
	router *alerting.Router // optional; nil = log-only mode
	source string           // alerting.Alert.Source for remote backends
}

// Alert satisfies billing.Alerter. The local logAlerter always fires
// first; its error is preserved as the primary return when both legs
// error so the audit-stream failure (rarer) dominates over the remote-
// network failure (more common).
func (m *multiAlerter) Alert(ctx context.Context, kind string, payload map[string]any) error {
	logErr := m.log.Alert(ctx, kind, payload)
	if m.router == nil {
		return logErr
	}
	if err := m.router.Fire(ctx, alertingFromBilling(kind, payload, m.source)); err != nil && logErr == nil {
		return err
	}
	return logErr
}

// alertingFromBilling maps the billing.Alerter (kind, payload) shape to
// the internal/alerting Alert shape. Every drift alert from the
// reconciler is Critical for V0 — we never want a drift signal to
// land somewhere other than the pager. tenant_id (when present in the
// payload) anchors the dedup key so repeat drift on the same tenant
// collapses into one open incident.
func alertingFromBilling(kind string, payload map[string]any, source string) alerting.Alert {
	labels := make(map[string]string, len(payload))
	for k, v := range payload {
		labels[k] = fmt.Sprintf("%v", v)
	}
	dedup := kind
	if t, ok := payload["tenant_id"].(string); ok && t != "" {
		dedup = kind + ":" + t
	}
	body := ""
	if d, ok := payload["drift_frac"]; ok {
		body = fmt.Sprintf("drift_frac=%v exceeded reconciler threshold", d)
	}
	return alerting.Alert{
		Severity:   alerting.SeverityCritical,
		Title:      kind,
		Body:       body,
		Source:     source,
		Dedup:      dedup,
		Labels:     labels,
		OccurredAt: time.Now().UTC(),
	}
}
