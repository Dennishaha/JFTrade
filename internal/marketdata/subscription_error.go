package marketdata

import (
	"errors"
	"fmt"
	"strings"
)

// ErrSubscriptionRequired identifies a live read attempted without an active
// logical subscription lease.
var ErrSubscriptionRequired = errors.New("market-data subscription required")

// SubscriptionRequiredError keeps broker-neutral lease details available to
// transports without leaking provider SDK errors.
type SubscriptionRequiredError struct {
	Channel  string
	Market   string
	Symbol   string
	Interval string
}

func NewSubscriptionRequiredError(channel, market, symbol, interval string) *SubscriptionRequiredError {
	return &SubscriptionRequiredError{
		Channel:  strings.ToUpper(strings.TrimSpace(channel)),
		Market:   strings.ToUpper(strings.TrimSpace(market)),
		Symbol:   strings.ToUpper(strings.TrimSpace(symbol)),
		Interval: normalizeSubscriptionInterval(interval),
	}
}

func (e *SubscriptionRequiredError) Error() string {
	if e == nil {
		return ErrSubscriptionRequired.Error()
	}
	channel := e.Channel
	if channel == "" {
		channel = "SNAPSHOT"
	}
	instrumentID := strings.Trim(strings.TrimSpace(e.Market)+"."+strings.TrimSpace(e.Symbol), ".")
	if instrumentID == "" {
		return fmt.Sprintf("%s: acquire a %s lease before reading live data", ErrSubscriptionRequired, channel)
	}
	if e.Interval != "" {
		return fmt.Sprintf("%s: acquire a %s lease for %s:%s before reading live data", ErrSubscriptionRequired, channel, instrumentID, e.Interval)
	}
	return fmt.Sprintf("%s: acquire a %s lease for %s before reading live data", ErrSubscriptionRequired, channel, instrumentID)
}

func (e *SubscriptionRequiredError) Unwrap() error {
	return ErrSubscriptionRequired
}
