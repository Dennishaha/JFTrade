package marketdata

import (
	"strings"
	"sync"
	"time"
)

type subscription struct {
	key          string
	channel      string
	market       string
	symbol       string
	instrumentID string
	interval     string
	consumers    map[string]time.Time
	createdAt    time.Time
	updatedAt    time.Time
}

type subscriptionRegistry struct {
	mu            sync.Mutex
	subscriptions map[string]*subscription
	now           func() time.Time
}

func newSubscriptionRegistry() *subscriptionRegistry {
	return &subscriptionRegistry{
		subscriptions: map[string]*subscription{},
		now:           time.Now,
	}
}

func (r *subscriptionRegistry) acquire(consumerID string, instruments []InstrumentRef) SubscriptionResult {
	consumerID = normalizeConsumerID(consumerID)
	now := r.now().UTC()

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, instrument := range instruments {
		channel, market, symbol, interval, key := normalizeSubscription(instrument)
		entry := r.subscriptions[key]
		if entry == nil {
			entry = &subscription{
				key:          key,
				channel:      channel,
				market:       market,
				symbol:       symbol,
				instrumentID: market + "." + symbol,
				interval:     interval,
				consumers:    map[string]time.Time{},
				createdAt:    now,
			}
			r.subscriptions[key] = entry
		}
		entry.consumers[consumerID] = now
		entry.updatedAt = now
	}

	return SubscriptionResult(r.snapshotLocked())
}

func (r *subscriptionRegistry) release(consumerID string, instrument InstrumentRef) {
	consumerID = normalizeConsumerID(consumerID)
	_, _, _, _, key := normalizeSubscription(instrument)
	now := r.now().UTC()

	r.mu.Lock()
	defer r.mu.Unlock()

	entry := r.subscriptions[key]
	if entry == nil {
		return
	}
	delete(entry.consumers, consumerID)
	entry.updatedAt = now
	if len(entry.consumers) == 0 {
		delete(r.subscriptions, key)
	}
}

func (r *subscriptionRegistry) heartbeat(consumerID string) HeartbeatResult {
	consumerID = normalizeConsumerID(consumerID)
	now := r.now().UTC()

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.subscriptions {
		if _, exists := entry.consumers[consumerID]; !exists {
			continue
		}
		entry.consumers[consumerID] = now
		entry.updatedAt = now
	}

	return HeartbeatResult(r.snapshotLocked())
}

func (r *subscriptionRegistry) clear(rawConsumerID string) {
	consumerID := normalizeConsumerID(rawConsumerID)
	now := r.now().UTC()

	r.mu.Lock()
	defer r.mu.Unlock()

	if consumerID == "web" && rawConsumerID == "" {
		r.subscriptions = map[string]*subscription{}
		return
	}

	for key, entry := range r.subscriptions {
		delete(entry.consumers, consumerID)
		entry.updatedAt = now
		if len(entry.consumers) == 0 {
			delete(r.subscriptions, key)
		}
	}
}

func (r *subscriptionRegistry) snapshot() SubscriptionsSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *subscriptionRegistry) snapshotLocked() SubscriptionsSnapshot {
	entries := make([]map[string]any, 0, len(r.subscriptions))
	byMarket := map[string]int{}
	for _, entry := range r.subscriptions {
		consumers := make([]string, 0, len(entry.consumers))
		for consumerID := range entry.consumers {
			consumers = append(consumers, consumerID)
		}
		byMarket[entry.market]++
		var interval any
		if entry.interval != "" {
			interval = entry.interval
		}
		entries = append(entries, map[string]any{
			"key":          entry.key,
			"channel":      entry.channel,
			"market":       entry.market,
			"symbol":       entry.symbol,
			"instrumentId": entry.instrumentID,
			"interval":     interval,
			"depthLevel":   nil,
			"consumers":    consumers,
			"refCount":     len(consumers),
			"createdAt":    entry.createdAt.Format(time.RFC3339Nano),
			"updatedAt":    entry.updatedAt.Format(time.RFC3339Nano),
		})
	}

	quotaBuckets := make([]map[string]any, 0, len(byMarket))
	for market, used := range byMarket {
		quotaBuckets = append(quotaBuckets, map[string]any{
			"market": market, "used": used, "limit": nil, "remaining": nil,
		})
	}

	return SubscriptionsSnapshot{
		"totalActiveSubscriptions": len(entries),
		"quota": map[string]any{
			"totalUsed":      len(entries),
			"totalLimit":     nil,
			"totalRemaining": nil,
			"byMarket":       quotaBuckets,
		},
		"entries": entries,
	}
}

func (r *subscriptionRegistry) activeInstruments() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.subscriptions))
	seen := make(map[string]struct{}, len(r.subscriptions))
	for _, entry := range r.subscriptions {
		if entry.market == "" || entry.symbol == "" {
			continue
		}
		instrumentID := entry.market + "." + entry.symbol
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		ids = append(ids, instrumentID)
	}
	return ids
}

func normalizeSubscriptionInstrument(market, symbol string) (string, string) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if strings.Contains(symbol, ".") {
		parts := strings.SplitN(symbol, ".", 2)
		if market == "" {
			market = strings.ToUpper(parts[0])
		}
		symbol = strings.ToUpper(parts[1])
	}
	return market, symbol
}

func normalizeSubscription(instrument InstrumentRef) (string, string, string, string, string) {
	market, symbol := normalizeSubscriptionInstrument(instrument.Market, instrument.Symbol)
	channel := strings.ToUpper(strings.TrimSpace(instrument.Channel))
	if channel == "" {
		channel = "SNAPSHOT"
	}
	interval := normalizeSubscriptionInterval(instrument.Interval)
	return channel, market, symbol, interval, subscriptionKey(channel, market, symbol, interval)
}

func normalizeSubscriptionInterval(interval string) string {
	return strings.TrimSpace(strings.ToLower(interval))
}

func normalizeConsumerID(consumerID string) string {
	consumerID = strings.TrimSpace(consumerID)
	if consumerID == "" {
		return "web"
	}
	return consumerID
}

func subscriptionKey(channel, market, symbol, interval string) string {
	if interval == "" {
		return channel + ":" + market + ":" + symbol
	}
	return channel + ":" + market + ":" + symbol + ":" + interval
}
