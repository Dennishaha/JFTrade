package marketdata

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// SubscriptionReconciler applies the complete desired subscription set to a
// broker connection. Implementations must be idempotent and safe for concurrent
// calls from HTTP handlers and the collector refresh loop.
type SubscriptionReconciler interface {
	ReconcileSubscriptions(context.Context, []InstrumentRef) error
	SubscriptionState() map[string]any
}

// ManagedSubscription is a non-expiring, process-owned subscription lease.
// Strategy runtimes keep one for their whole RUNNING lifetime.
type ManagedSubscription struct {
	once    sync.Once
	release func()
}

func newManagedSubscription(release func()) *ManagedSubscription {
	return &ManagedSubscription{release: release}
}

// Release relinquishes the lease exactly once.
func (s *ManagedSubscription) Release() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.release != nil {
			s.release()
		}
	})
}

func decorateSubscriptionSnapshot(snapshot SubscriptionsSnapshot, broker map[string]any) SubscriptionsSnapshot {
	defaultEntryState := "pending_subscribe"
	if broker == nil {
		defaultEntryState = "unmanaged"
		broker = map[string]any{
			"desiredCount":        snapshot["totalActiveSubscriptions"],
			"ownActiveCount":      0,
			"pendingReleaseCount": 0,
			"totalUsedQuota":      nil,
			"remainQuota":         nil,
			"entries":             []map[string]any{},
		}
	}
	for _, field := range []string{"desiredCount", "ownActiveCount", "pendingReleaseCount", "totalUsedQuota", "remainQuota"} {
		snapshot[field] = broker[field]
	}
	physicalEntries := map[string]map[string]any{}
	if entries, ok := broker["entries"].([]map[string]any); ok {
		for _, entry := range entries {
			if key, ok := entry["key"].(string); ok {
				physicalEntries[key] = entry
			}
		}
	}
	if entries, ok := snapshot["entries"].([]map[string]any); ok {
		for _, entry := range entries {
			instrumentID, _ := entry["instrumentId"].(string)
			channel, _ := entry["channel"].(string)
			physicalKey := "BASIC:" + instrumentID
			if channel == "KLINE" {
				if interval, ok := entry["interval"].(string); ok && interval != "" {
					physicalKey = "KLINE:" + instrumentID + ":" + interval
				}
			}
			physical := physicalEntries[physicalKey]
			entry["brokerState"] = defaultEntryState
			entry["subscribedAt"] = nil
			entry["unsubscribeEligibleAt"] = nil
			entry["lastError"] = nil
			if physical != nil {
				for _, field := range []string{"brokerState", "subscribedAt", "unsubscribeEligibleAt", "lastError"} {
					entry[field] = physical[field]
				}
			}
		}
	}
	snapshot["brokerState"] = broker
	return snapshot
}

func validateSubscriptionRefs(refs []InstrumentRef) error {
	if len(refs) == 0 {
		return fmt.Errorf("at least one subscription is required")
	}
	for _, ref := range refs {
		market, symbol := normalizeSubscriptionInstrument(ref.Market, ref.Symbol)
		if market == "" || symbol == "" {
			return fmt.Errorf("subscription market and symbol are required")
		}
		channel := strings.ToUpper(strings.TrimSpace(ref.Channel))
		if channel == "" {
			channel = "SNAPSHOT"
		}
		switch channel {
		case "SNAPSHOT", "TICK":
			if strings.TrimSpace(ref.Interval) != "" {
				return fmt.Errorf("subscription interval is only valid for KLINE")
			}
		case "KLINE":
			interval := normalizeSubscriptionInterval(ref.Interval)
			if !supportedKLineSubscriptionInterval(interval) {
				return fmt.Errorf("unsupported KLINE subscription interval %q", ref.Interval)
			}
		default:
			return fmt.Errorf("unsupported subscription channel %q", ref.Channel)
		}
	}
	return nil
}

// ValidateSubscriptionRefs validates the public subscription wire contract.
func ValidateSubscriptionRefs(refs []InstrumentRef) error {
	return validateSubscriptionRefs(refs)
}

func supportedKLineSubscriptionInterval(interval string) bool {
	switch interval {
	case "1m", "3m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo":
		return true
	default:
		return false
	}
}
