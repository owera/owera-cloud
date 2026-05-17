package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// StripeUsageReporter reports the per-tenant usage Stripe believes it has
// recorded for a billing window. Production: a thin wrapper over Stripe's
// /v1/subscription_items/{ID}/usage_record_summaries listing API. Tests
// inject a fake.
type StripeUsageReporter interface {
	UsageForTenant(ctx context.Context, tenantID string, periodStart, periodEnd time.Time) (int64, error)
}

// Alerter receives reconciliation drift alerts. The operator plane's
// internal/alerting package is the eventual sink — for now Alerter is
// implemented by a ledger-emitting stub (see CmdReconciler) that writes
// `Result == ResultAlert` rows.
type Alerter interface {
	Alert(ctx context.Context, kind string, payload map[string]any) error
}

// DriftThreshold is the fractional difference between ledger sum and
// Stripe usage above which the reconciler raises an alert. Per the plan,
// the cutoff is 0.5%.
const DriftThreshold = 0.005

// Reconciler is the daily ledger-vs-Stripe drift detector.
type Reconciler struct {
	svc     *Service
	report  StripeUsageReporter
	alerter Alerter
}

// NewReconciler wires the daily job.
func NewReconciler(svc *Service, report StripeUsageReporter, alerter Alerter) (*Reconciler, error) {
	if svc == nil {
		return nil, errors.New("billing: reconciler nil service")
	}
	if report == nil {
		return nil, errors.New("billing: reconciler nil reporter")
	}
	if alerter == nil {
		return nil, errors.New("billing: reconciler nil alerter")
	}
	return &Reconciler{svc: svc, report: report, alerter: alerter}, nil
}

// ReconcileReport captures one tenant's drift result.
type ReconcileReport struct {
	TenantID    string
	LedgerUnits int64
	StripeUnits int64
	DriftFrac   float64
	Alerted     bool
}

// Run iterates every tenant with outbox rows in [periodStart, periodEnd),
// compares ledger sum vs Stripe reporter, and alerts on drift > 0.5%. A
// run with zero drift is silent (no alert).
func (r *Reconciler) Run(ctx context.Context, periodStart, periodEnd time.Time) ([]ReconcileReport, error) {
	tenants, err := r.svc.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ReconcileReport, 0, len(tenants))
	for _, tenantID := range tenants {
		ledger, err := r.svc.TenantPeriodSum(ctx, tenantID, periodStart, periodEnd)
		if err != nil {
			return out, fmt.Errorf("billing: reconcile tenant=%s ledger sum: %w", tenantID, err)
		}
		stripeUnits, err := r.report.UsageForTenant(ctx, tenantID, periodStart, periodEnd)
		if err != nil {
			// One tenant failure shouldn't abort the whole run; alert and
			// continue so other tenants still get reconciled.
			_ = r.alerter.Alert(ctx, "billing.reconcile.report_error", map[string]any{
				"tenant_id": tenantID,
				"error":     err.Error(),
			})
			continue
		}
		drift := driftFraction(ledger, stripeUnits)
		rep := ReconcileReport{
			TenantID:    tenantID,
			LedgerUnits: ledger,
			StripeUnits: stripeUnits,
			DriftFrac:   drift,
		}
		if drift > DriftThreshold {
			if err := r.alerter.Alert(ctx, "billing.reconcile.drift", map[string]any{
				"tenant_id":    tenantID,
				"ledger_units": ledger,
				"stripe_units": stripeUnits,
				"drift_frac":   drift,
				"threshold":    DriftThreshold,
				"period_start": periodStart.UTC().Format(time.RFC3339),
				"period_end":   periodEnd.UTC().Format(time.RFC3339),
			}); err != nil {
				return out, fmt.Errorf("billing: alert drift tenant=%s: %w", tenantID, err)
			}
			rep.Alerted = true
		}
		out = append(out, rep)
	}
	return out, nil
}

// driftFraction returns |ledger - stripe| / max(ledger, 1) — using the
// ledger as the denominator because the ledger is the source of truth.
// Result is 0 when both are 0; the reconciler treats divide-by-zero as a
// no-drift case.
func driftFraction(ledger, stripeUnits int64) float64 {
	if ledger == 0 && stripeUnits == 0 {
		return 0
	}
	denom := float64(ledger)
	if denom < 1 {
		denom = 1
	}
	return math.Abs(float64(ledger-stripeUnits)) / denom
}
