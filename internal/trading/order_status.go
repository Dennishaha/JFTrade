package trading

import (
	"slices"
	"strings"
)

const (
	OrderStatusCreated         = "CREATED"
	OrderStatusPrecheckReject  = "PRECHECK_REJECTED"
	OrderStatusSubmitting      = "SUBMITTING"
	OrderStatusSubmitted       = "SUBMITTED"
	OrderStatusBrokerAccepted  = "BROKER_ACCEPTED"
	OrderStatusPartiallyFilled = "PARTIALLY_FILLED"
	OrderStatusFilled          = "FILLED"
	OrderStatusCancelRequested = "CANCEL_REQUESTED"
	OrderStatusCancelled       = "CANCELLED"
	OrderStatusRejected        = "REJECTED"
	OrderStatusExpired         = "EXPIRED"
	OrderStatusUnknown         = "UNKNOWN"
)

// CanonicalBrokerOrderStatus maps broker-specific lifecycle values to the
// stable JFTrade order lifecycle exposed by the execution ledger.
func CanonicalBrokerOrderStatus(raw string) string {
	switch normalizeOrderStatus(raw) {
	case "CREATED":
		return OrderStatusCreated
	case "PRECHECK_REJECTED":
		return OrderStatusPrecheckReject
	case "UNSUBMITTED", "WAITING_SUBMIT", "SUBMITTING":
		return OrderStatusSubmitting
	case "SUBMITTED", "NEW", "ACCEPTED", "BROKER_ACCEPTED":
		return OrderStatusBrokerAccepted
	case "FILLED_PART", "PARTIAL_FILLED", "PARTIALLY_FILLED":
		return OrderStatusPartiallyFilled
	case "FILLED_ALL", "FILLED":
		return OrderStatusFilled
	case "CANCELLING_PART", "CANCELLING_ALL", "CANCELING", "CANCEL_REQUESTED", "PENDING_CANCEL":
		return OrderStatusCancelRequested
	case "CANCELLED_PART", "CANCELLED_ALL", "CANCELLED", "CANCELED_PART", "CANCELED_ALL", "CANCELED", "DELETED":
		return OrderStatusCancelled
	case "SUBMIT_FAILED", "SUBMITFAILED", "FAILED", "REJECTED", "DISABLED", "FILL_CANCELLED", "FILLCANCELLED":
		return OrderStatusRejected
	case "EXPIRED":
		return OrderStatusExpired
	default:
		return OrderStatusUnknown
	}
}

// CanonicalStoredOrderStatus also understands JFTrade command-side states.
// It is used when loading legacy ledger rows that may still contain raw broker
// values from before canonical statuses were introduced.
func CanonicalStoredOrderStatus(status string) string {
	switch normalizeOrderStatus(status) {
	case "CREATED":
		return OrderStatusCreated
	case "PRECHECK_REJECTED":
		return OrderStatusPrecheckReject
	case "SUBMITTING":
		return OrderStatusSubmitting
	case "SUBMITTED":
		return OrderStatusSubmitted
	case "BROKER_ACCEPTED":
		return OrderStatusBrokerAccepted
	case "PARTIALLY_FILLED":
		return OrderStatusPartiallyFilled
	case "FILLED":
		return OrderStatusFilled
	case "CANCEL_REQUESTED":
		return OrderStatusCancelRequested
	case "CANCELLED":
		return OrderStatusCancelled
	case "REJECTED":
		return OrderStatusRejected
	case "EXPIRED":
		return OrderStatusExpired
	case "UNKNOWN":
		return OrderStatusUnknown
	default:
		return CanonicalBrokerOrderStatus(status)
	}
}

func IsCanonicalTerminalOrderStatus(status string) bool {
	switch CanonicalStoredOrderStatus(status) {
	case OrderStatusPrecheckReject, OrderStatusFilled, OrderStatusCancelled, OrderStatusRejected, OrderStatusExpired:
		return true
	default:
		return false
	}
}

// ReconcileCanonicalOrderStatus rejects status regressions caused by delayed
// broker queries or out-of-order push delivery.
func ReconcileCanonicalOrderStatus(current, incoming string) (string, bool) {
	current = CanonicalStoredOrderStatus(current)
	incoming = CanonicalStoredOrderStatus(incoming)
	if current == incoming {
		return current, true
	}
	if incoming == OrderStatusUnknown {
		return current, false
	}
	if current == "" || current == OrderStatusUnknown {
		return incoming, true
	}
	if IsCanonicalTerminalOrderStatus(current) {
		return current, false
	}
	if slices.Contains(canonicalOrderStatusTransitions[current], incoming) {
		return incoming, true
	}
	return current, false
}

var canonicalOrderStatusTransitions = map[string][]string{
	OrderStatusCreated: {
		OrderStatusSubmitting, OrderStatusSubmitted, OrderStatusBrokerAccepted,
		OrderStatusPartiallyFilled, OrderStatusFilled, OrderStatusCancelRequested,
		OrderStatusCancelled, OrderStatusRejected, OrderStatusExpired,
	},
	OrderStatusSubmitting: {
		OrderStatusSubmitted, OrderStatusBrokerAccepted, OrderStatusPartiallyFilled,
		OrderStatusFilled, OrderStatusCancelRequested, OrderStatusCancelled,
		OrderStatusRejected, OrderStatusExpired,
	},
	OrderStatusSubmitted: {
		OrderStatusBrokerAccepted, OrderStatusPartiallyFilled, OrderStatusFilled,
		OrderStatusCancelRequested, OrderStatusCancelled, OrderStatusRejected,
		OrderStatusExpired,
	},
	OrderStatusBrokerAccepted: {
		OrderStatusPartiallyFilled, OrderStatusFilled, OrderStatusCancelRequested,
		OrderStatusCancelled, OrderStatusRejected, OrderStatusExpired,
	},
	OrderStatusPartiallyFilled: {
		OrderStatusFilled, OrderStatusCancelRequested, OrderStatusCancelled,
		OrderStatusRejected, OrderStatusExpired,
	},
	OrderStatusCancelRequested: {
		OrderStatusFilled, OrderStatusCancelled, OrderStatusRejected, OrderStatusExpired,
	},
}

func normalizeOrderStatus(status string) string {
	normalized := strings.ToUpper(strings.TrimSpace(status))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return strings.TrimPrefix(normalized, "ORDER_STATUS_")
}
