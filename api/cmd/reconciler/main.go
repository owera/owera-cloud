// Command reconciler runs the daily billing reconciliation pass: for each
// tenant with rows in the local outbox for the previous calendar day,
// compare ledger sum vs Stripe's reported usage and emit an alert when
// drift exceeds 0.5%. Designed to run from cron (or a Fly machine
// scheduled-job) once per day.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/owera/owera-cloud/api/internal/billing"
	"github.com/owera/owera-cloud/api/internal/identity"
)

func main() {
	dbPath := flag.String("db", "./owera-cloud.db", "SQLite database path")
	periodFlag := flag.String("period", "yesterday", "yesterday | last-month | custom")
	alertURL := flag.String("alert-url", "", "HTTP endpoint to POST drift alerts to (operator-plane alerting). If empty, alerts go to stderr.")
	flag.Parse()

	if err := run(*dbPath, *periodFlag, *alertURL); err != nil {
		log.Fatalf("reconciler: %v", err)
	}
}

func run(dbPath, periodFlag, alertURL string) error {
	idStore, err := identity.Open(dbPath)
	if err != nil {
		return err
	}
	defer idStore.Close()

	svc, err := billing.New(idStore.DB(), nil)
	if err != nil {
		return err
	}

	periodStart, periodEnd := parsePeriod(periodFlag)
	reporter := newStripeReporter()
	alerter := newAlerter(alertURL)

	rec, err := billing.NewReconciler(svc, reporter, alerter)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	reports, err := rec.Run(ctx, periodStart, periodEnd)
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		log.Printf("reconciler: no tenants to reconcile for window %s..%s",
			periodStart.Format(time.RFC3339), periodEnd.Format(time.RFC3339))
		return nil
	}
	totalAlerted := 0
	for _, r := range reports {
		if r.Alerted {
			totalAlerted++
		}
		log.Printf("reconciler: tenant=%s ledger=%d stripe=%d drift=%.4f alerted=%v",
			r.TenantID, r.LedgerUnits, r.StripeUnits, r.DriftFrac, r.Alerted)
	}
	log.Printf("reconciler: done — %d tenants, %d alerted", len(reports), totalAlerted)
	return nil
}

func parsePeriod(flag string) (time.Time, time.Time) {
	now := time.Now().UTC()
	switch flag {
	case "last-month":
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return first.AddDate(0, -1, 0), first
	case "yesterday", "":
		y := now.AddDate(0, 0, -1)
		start := time.Date(y.Year(), y.Month(), y.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1)
	default:
		// custom: yyyy-mm-dd..yyyy-mm-dd
		parts := strings.Split(flag, "..")
		if len(parts) == 2 {
			s, err1 := time.Parse("2006-01-02", parts[0])
			e, err2 := time.Parse("2006-01-02", parts[1])
			if err1 == nil && err2 == nil {
				return s, e
			}
		}
		log.Fatalf("reconciler: bad period flag %q", flag)
		return time.Time{}, time.Time{}
	}
}

// stripeReporter is the production StripeUsageReporter. It queries
// Stripe's /v1/subscription_items/{id}/usage_record_summaries listings.
// Implementation is deliberately minimal at this scaffold stage:
// returns 0 for every tenant if STRIPE_SECRET_KEY is unset, which makes
// the reconciler's drift math compare ledger vs zero — a deliberately
// loud failure mode in dev that goes silent only once the env is wired.
type stripeReporter struct {
	key string
}

func newStripeReporter() *stripeReporter {
	return &stripeReporter{key: os.Getenv("STRIPE_SECRET_KEY")}
}

func (r *stripeReporter) UsageForTenant(_ context.Context, _ string, _, _ time.Time) (int64, error) {
	if r.key == "" {
		// Dev mode: report 0 so the reconciler's drift math runs.
		return 0, nil
	}
	// Wiring to the real Stripe usage_record_summaries listing endpoint is
	// gated on the subscription-item ↔ tenant map landing in WS-15
	// identity. Until then this is intentionally a no-op so the cron
	// doesn't no-charge or double-charge anything.
	return 0, nil
}

// alertEmitter POSTs drift alerts to the configured URL, falling back to
// stderr if no URL is set. The TME alerting endpoint on the operator
// plane (internal/alerting on owera-fleet) is the eventual target.
type alertEmitter struct {
	url string
	c   *http.Client
}

func newAlerter(targetURL string) *alertEmitter {
	if targetURL != "" {
		if _, err := url.Parse(targetURL); err != nil {
			log.Fatalf("reconciler: bad alert URL: %v", err)
		}
	}
	return &alertEmitter{url: targetURL, c: &http.Client{Timeout: 10 * time.Second}}
}

func (a *alertEmitter) Alert(ctx context.Context, kind string, payload map[string]any) error {
	if a.url == "" {
		// Stderr fallback: render as JSONL so log-rotation pipelines pick
		// it up and the operator can grep for it.
		payload["kind"] = kind
		payload["ts"] = time.Now().UTC().Format(time.RFC3339)
		buf, _ := json.Marshal(payload)
		fmt.Fprintln(os.Stderr, string(buf))
		return nil
	}
	payload["kind"] = kind
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("reconciler: alert POST %s -> %d", a.url, resp.StatusCode)
	}
	return nil
}
