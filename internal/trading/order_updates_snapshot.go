package trading

import (
	"sort"
	"time"
)

func (w *OrderUpdatesWorker) SnapshotResponse() map[string]any {
	if w == nil {
		return map[string]any{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	keys := make([]string, 0, len(w.subscriptions))
	for key := range w.subscriptions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	subscriptions := make([]any, 0, len(keys))
	latestAction := "idle"
	latestActionAt := w.now().Format(time.RFC3339Nano)
	active, inactive := 0, 0
	for _, key := range keys {
		state := w.subscriptions[key]
		if state == nil {
			continue
		}
		if state.Status == "active" {
			active++
		} else {
			inactive++
		}
		if state.LastActionAt > latestActionAt || latestAction == "idle" {
			latestAction = state.LastAction
			latestActionAt = state.LastActionAt
		}
		subscriptions = append(subscriptions, map[string]any{
			"subscriptionKey": state.SubscriptionKey, "brokerId": state.BrokerID,
			"tradingEnvironment": state.TradingEnvironment, "accountId": state.AccountID, "market": state.Market,
			"status": state.Status, "lastAction": state.LastAction, "lastActionAt": state.LastActionAt,
			"lastError": state.LastError, "lastErrorContext": nil, "consecutiveFailures": state.ConsecutiveFailures,
			"retryDelayMs": state.RetryDelayMs, "backoffUntil": state.BackoffUntil,
		})
	}
	invalidations := make([]any, 0, len(w.recentInvalidations))
	for _, invalidation := range w.recentInvalidations {
		invalidations = append(invalidations, map[string]any{
			"subscriptionKey": invalidation.SubscriptionKey, "brokerId": invalidation.BrokerID,
			"tradingEnvironment": invalidation.TradingEnvironment, "accountId": invalidation.AccountID,
			"market": invalidation.Market, "kind": invalidation.Kind, "message": invalidation.Message,
			"errorContext": nil, "consecutiveFailures": nil, "retryDelayMs": nil, "backoffUntil": nil,
			"createdAt": invalidation.CreatedAt,
		})
	}
	brokers := []any{}
	if len(keys) > 0 || w.accountsDiscovered > 0 || w.connectivity != nil {
		brokers = append(brokers, map[string]any{
			"brokerId": w.config.BrokerID, "lastAction": latestAction, "lastActionAt": latestActionAt,
			"connectivity": w.connectivity, "lastError": latestSubscriptionError(w.subscriptions),
			"accountsDiscovered": nullablePositiveInt(w.accountsDiscovered), "activeSubscriptions": active,
			"retryingSubscriptions": 0, "inactiveSubscriptions": inactive, "backoffSubscriptions": 0,
			"disconnectedBackoffSubscriptions": 0, "subscribeFailedBackoffSubscriptions": 0,
			"errorBackoffSubscriptions": 0, "dominantBackoffSource": nil, "dominantBackoffCount": 0,
			"longestBackoffSource": nil, "longestBackoffRemainingMs": nil,
			"longestBackoffSubscriptionKey": nil, "longestBackoffMarket": nil,
			"longestBackoffTradingEnvironment": nil, "longestBackoffAccountId": nil,
			"topBackoffHotspots": []any{}, "layeredBackoffSummaries": []any{},
			"recentInvalidationCount": len(w.recentInvalidations),
			"lastInvalidationKind":    latestInvalidationKind(w.recentInvalidations),
			"lastInvalidationAt":      latestInvalidationAt(w.recentInvalidations),
			"backoffActive":           false, "backoffSource": nil, "backoffUntil": nil, "backoffRemainingMs": nil,
		})
	}
	return map[string]any{
		"subscriptions": subscriptions, "recentInvalidations": invalidations, "brokers": brokers,
		"runtime": map[string]any{"lastStoppedAt": w.lastStoppedAt, "stoppedSubscriptions": w.stoppedSubscriptions},
	}
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func latestSubscriptionError(states map[string]*orderUpdateSubscriptionState) *string {
	var latest *string
	latestAt := ""
	for _, state := range states {
		if state != nil && state.LastError != nil && state.LastActionAt >= latestAt {
			latestAt, latest = state.LastActionAt, state.LastError
		}
	}
	return latest
}

func latestInvalidationKind(invalidations []orderUpdateInvalidation) any {
	if len(invalidations) == 0 {
		return nil
	}
	return invalidations[len(invalidations)-1].Kind
}

func latestInvalidationAt(invalidations []orderUpdateInvalidation) any {
	if len(invalidations) == 0 {
		return nil
	}
	return invalidations[len(invalidations)-1].CreatedAt
}
