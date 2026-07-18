package trading

import (
	"context"
	"strings"
	"time"
)

func (w *OrderUpdatesWorker) shouldSync(force bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if force || w.lastSyncAt.IsZero() || now.Sub(w.lastSyncAt) >= w.config.SyncMinInterval {
		w.lastSyncAt = now
		return true
	}
	return false
}

func (w *OrderUpdatesWorker) applyOrders(ctx context.Context, brokerID string, orders []Order, metadata OrderWriteMetadata) {
	for _, order := range orders {
		actualBrokerID := strings.TrimSpace(order.BrokerID)
		if actualBrokerID == "" {
			actualBrokerID = brokerID
		}
		w.execution.ApplyOrder(ctx, actualBrokerID, cloneOrder(order), metadata)
	}
}

func (w *OrderUpdatesWorker) markSubscriptions(queries []OrderQuery, status, action string, err error) {
	now := w.now().Format(time.RFC3339Nano)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, query := range queries {
		key := OrderUpdateSubscriptionKey(query)
		state := w.subscriptions[key]
		if state == nil {
			state = &orderUpdateSubscriptionState{
				SubscriptionKey: key, BrokerID: query.BrokerID,
				TradingEnvironment: stringPointer(query.TradingEnvironment), AccountID: stringPointer(query.AccountID),
				Market: stringPointer(query.Market),
			}
			w.subscriptions[key] = state
		}
		state.Status, state.LastAction, state.LastActionAt = status, action, now
		if err == nil {
			state.LastError, state.ConsecutiveFailures, state.RetryDelayMs, state.BackoffUntil = nil, nil, nil, nil
			continue
		}
		message := strings.TrimSpace(err.Error())
		state.LastError = stringPointer(message)
		failures := 1
		if state.ConsecutiveFailures != nil {
			failures = *state.ConsecutiveFailures + 1
		}
		state.ConsecutiveFailures = &failures
		kind := "ERROR"
		lower := strings.ToLower(message)
		if strings.Contains(lower, "dial") || strings.Contains(lower, "closed") || strings.Contains(lower, "timeout") {
			kind = "DISCONNECTED"
		}
		w.recentInvalidations = append(w.recentInvalidations, orderUpdateInvalidation{
			SubscriptionKey: key, BrokerID: query.BrokerID,
			TradingEnvironment: stringPointer(query.TradingEnvironment), AccountID: stringPointer(query.AccountID),
			Market: stringPointer(query.Market), Kind: kind, Message: stringPointer(message), CreatedAt: now,
		})
		if len(w.recentInvalidations) > maxOrderUpdateInvalidations {
			w.recentInvalidations = append([]orderUpdateInvalidation(nil), w.recentInvalidations[len(w.recentInvalidations)-maxOrderUpdateInvalidations:]...)
		}
	}
}

func (w *OrderUpdatesWorker) markDiscoveredAccounts(count int, connectivity string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accountsDiscovered = count
	w.connectivity = stringPointer(connectivity)
}

func (w *OrderUpdatesWorker) storeActiveOrders(key string, orders []Order) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.activeOrdersCache[key] = cloneOrders(orders)
	w.activeOrdersCachedAt[key] = w.now()
}

func (w *OrderUpdatesWorker) cachedActiveOrders(key string) ([]Order, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cachedAt, ok := w.activeOrdersCachedAt[key]
	if !ok || w.now().Sub(cachedAt) > w.config.CacheTTL {
		return nil, false
	}
	orders, ok := w.activeOrdersCache[key]
	if !ok {
		return nil, false
	}
	return cloneOrders(orders), true
}

func (w *OrderUpdatesWorker) upsertActiveOrder(key string, order Order) {
	w.mu.Lock()
	defer w.mu.Unlock()
	orders := w.activeOrdersCache[key]
	for i, existing := range orders {
		if sameOrder(existing, order.BrokerOrderID, order.BrokerOrderIDEx) {
			orders[i] = cloneOrder(order)
			w.activeOrdersCache[key] = orders
			return
		}
	}
	w.activeOrdersCache[key] = append(orders, cloneOrder(order))
	w.activeOrdersCachedAt[key] = w.now()
}

func (w *OrderUpdatesWorker) removeActiveOrder(key, orderID string, orderIDEx *string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	orders, ok := w.activeOrdersCache[key]
	if !ok {
		return
	}
	filtered := make([]Order, 0, len(orders))
	for _, order := range orders {
		if !sameOrder(order, orderID, orderIDEx) {
			filtered = append(filtered, cloneOrder(order))
		}
	}
	w.activeOrdersCache[key] = filtered
}
