package futu

import (
	"errors"
	"fmt"
	"strings"
)

// ErrSubscriptionRequired identifies live market-data reads that do not have
// the physical OpenD subscription required by the requested channel.
var ErrSubscriptionRequired = errors.New("futu market-data subscription required")

// SubscriptionRequiredError describes the exact missing physical lease.
type SubscriptionRequiredError struct {
	Channel  string
	Symbol   string
	Interval string
}

func (e *SubscriptionRequiredError) Error() string {
	if e == nil {
		return ErrSubscriptionRequired.Error()
	}
	channel := strings.ToUpper(strings.TrimSpace(e.Channel))
	if channel == "" {
		channel = "market-data"
	}
	detail := strings.TrimSpace(e.Symbol)
	if interval := strings.TrimSpace(e.Interval); interval != "" {
		detail += ":" + interval
	}
	if detail == "" {
		return fmt.Sprintf("%s: acquire a %s lease before reading live data", ErrSubscriptionRequired, channel)
	}
	return fmt.Sprintf("%s: acquire a %s lease for %s before reading live data", ErrSubscriptionRequired, channel, detail)
}

func (e *SubscriptionRequiredError) Unwrap() error {
	return ErrSubscriptionRequired
}
