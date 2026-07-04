package trading

import "testing"

func TestCanonicalBrokerOrderStatusCoversFutuLifecycle(t *testing.T) {
	cases := map[string]string{
		"Unsubmitted":     OrderStatusSubmitting,
		"WAITING_SUBMIT":  OrderStatusSubmitting,
		"Submitting":      OrderStatusSubmitting,
		"NEW":             OrderStatusBrokerAccepted,
		"Submitted":       OrderStatusBrokerAccepted,
		"Filled_Part":     OrderStatusPartiallyFilled,
		"Filled_All":      OrderStatusFilled,
		"Cancelling_Part": OrderStatusCancelRequested,
		"Cancelling_All":  OrderStatusCancelRequested,
		"Cancelled_Part":  OrderStatusCancelled,
		"Cancelled_All":   OrderStatusCancelled,
		"SubmitFailed":    OrderStatusRejected,
		"Failed":          OrderStatusRejected,
		"Disabled":        OrderStatusRejected,
		"Deleted":         OrderStatusCancelled,
		"FillCancelled":   OrderStatusRejected,
		"TimeOut":         OrderStatusUnknown,
		"unexpected":      OrderStatusUnknown,
	}
	for raw, want := range cases {
		if got := CanonicalBrokerOrderStatus(raw); got != want {
			t.Fatalf("CanonicalBrokerOrderStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestReconcileCanonicalOrderStatusPreventsBrokerRegressions(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		incoming string
		want     string
		accepted bool
	}{
		{name: "accepted to partial", current: OrderStatusBrokerAccepted, incoming: OrderStatusPartiallyFilled, want: OrderStatusPartiallyFilled, accepted: true},
		{name: "partial to submitted regression", current: OrderStatusPartiallyFilled, incoming: OrderStatusBrokerAccepted, want: OrderStatusPartiallyFilled, accepted: false},
		{name: "cancel race fills", current: OrderStatusCancelRequested, incoming: OrderStatusFilled, want: OrderStatusFilled, accepted: true},
		{name: "cancel request ignores partial regression", current: OrderStatusCancelRequested, incoming: OrderStatusPartiallyFilled, want: OrderStatusCancelRequested, accepted: false},
		{name: "filled is terminal", current: OrderStatusFilled, incoming: OrderStatusBrokerAccepted, want: OrderStatusFilled, accepted: false},
		{name: "unknown recovers", current: OrderStatusUnknown, incoming: OrderStatusBrokerAccepted, want: OrderStatusBrokerAccepted, accepted: true},
		{name: "known ignores unknown", current: OrderStatusBrokerAccepted, incoming: OrderStatusUnknown, want: OrderStatusBrokerAccepted, accepted: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, accepted := ReconcileCanonicalOrderStatus(test.current, test.incoming)
			if got != test.want || accepted != test.accepted {
				t.Fatalf("ReconcileCanonicalOrderStatus(%q, %q) = (%q, %v), want (%q, %v)", test.current, test.incoming, got, accepted, test.want, test.accepted)
			}
		})
	}
}

func TestCanonicalTerminalOrderStatus(t *testing.T) {
	for _, status := range []string{OrderStatusPrecheckReject, OrderStatusFilled, OrderStatusCancelled, OrderStatusRejected, OrderStatusExpired} {
		if !IsCanonicalTerminalOrderStatus(status) {
			t.Fatalf("status %q should be terminal", status)
		}
	}
	for _, status := range []string{OrderStatusCreated, OrderStatusSubmitting, OrderStatusSubmitted, OrderStatusBrokerAccepted, OrderStatusPartiallyFilled, OrderStatusCancelRequested, OrderStatusUnknown} {
		if IsCanonicalTerminalOrderStatus(status) {
			t.Fatalf("status %q should not be terminal", status)
		}
	}
}
