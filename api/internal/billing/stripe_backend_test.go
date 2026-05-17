package billing

import (
	"strconv"
	"testing"
	"time"
)

// TestBuildMeterEventParams verifies the BillingMeterEventParams shape
// the new EmitUsage builds. The earlier (legacy) usage_records API took
// SubscriptionItem + Quantity + Idempotency-Key; the new meter_events
// API takes EventName + Identifier + Payload{stripe_customer_id, value}.
//
// Coverage:
//   - Required fields all set (event_name, identifier, payload keys)
//   - Numeric value rendered as decimal string (Payload is map[string]string)
//   - Customer id passed through verbatim
//   - Explicit Ts honored
//   - Zero Ts → now() fallback
func TestBuildMeterEventParams(t *testing.T) {
	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	t.Run("explicit Ts honored", func(t *testing.T) {
		ev := UsageEmit{
			TenantID: "ten_acme",
			SKU:      "triage-watch@v1",
			Meter:    "tickets_processed",
			Units:    42,
			Ts:       fixed,
			IdemKey:  "usage:ten_acme:task-1:5",
		}
		p := buildMeterEventParams(ev, "cus_test_xyz", func() time.Time {
			t.Fatal("now() must not be called when Ts is explicit")
			return time.Time{}
		})
		if p.EventName == nil || *p.EventName != "tickets_processed" {
			t.Errorf("EventName: got %v want tickets_processed", p.EventName)
		}
		if p.Identifier == nil || *p.Identifier != "usage:ten_acme:task-1:5" {
			t.Errorf("Identifier: got %v", p.Identifier)
		}
		if p.Timestamp == nil || *p.Timestamp != fixed.Unix() {
			t.Errorf("Timestamp: got %v want %d", p.Timestamp, fixed.Unix())
		}
		if got := p.Payload["stripe_customer_id"]; got != "cus_test_xyz" {
			t.Errorf("payload.stripe_customer_id: %q want cus_test_xyz", got)
		}
		if got := p.Payload["value"]; got != "42" {
			t.Errorf("payload.value: %q want \"42\"", got)
		}
	})

	t.Run("zero Ts falls back to now()", func(t *testing.T) {
		nowCalls := 0
		ev := UsageEmit{Meter: "x", IdemKey: "k", Units: 1}
		p := buildMeterEventParams(ev, "cus_x", func() time.Time {
			nowCalls++
			return fixed
		})
		if nowCalls != 1 {
			t.Errorf("now() called %d times, want 1", nowCalls)
		}
		if *p.Timestamp != fixed.Unix() {
			t.Errorf("Timestamp from fallback: got %d want %d", *p.Timestamp, fixed.Unix())
		}
	})

	t.Run("large quantity renders as decimal", func(t *testing.T) {
		ev := UsageEmit{
			Meter:   "y",
			IdemKey: "k",
			Units:   1_234_567,
			Ts:      fixed,
		}
		p := buildMeterEventParams(ev, "cus_a", time.Now)
		got := p.Payload["value"]
		want := strconv.FormatInt(1_234_567, 10)
		if got != want {
			t.Errorf("value: %q want %q", got, want)
		}
	})
}
