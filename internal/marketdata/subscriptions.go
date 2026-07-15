package marketdata

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const WebSubscriptionTTL = 5 * time.Minute

type subscriptionConsumer struct {
	seenAt  time.Time
	managed bool
}

type subscription struct {
	key          string
	channel      string
	market       string
	symbol       string
	instrumentID string
	interval     string
	consumers    map[string]subscriptionConsumer
	createdAt    time.Time
	updatedAt    time.Time
}

type subscriptionRegistry struct {
	mu            sync.Mutex
	subscriptions map[string]*subscription
	now           func() time.Time
	externalTTL   time.Duration
}

func newSubscriptionRegistry() *subscriptionRegistry {
	return &subscriptionRegistry{
		subscriptions: map[string]*subscription{},
		now:           time.Now,
		externalTTL:   WebSubscriptionTTL,
	}
}

func (r *subscriptionRegistry) acquire(consumerID string, instruments []InstrumentRef) SubscriptionResult {
	return r.acquireWithMode(consumerID, instruments, false)
}

func (r *subscriptionRegistry) acquireManaged(consumerID string, instruments []InstrumentRef) SubscriptionResult {
	return r.acquireWithMode(consumerID, instruments, true)
}

func (r *subscriptionRegistry) acquireWithMode(consumerID string, instruments []InstrumentRef, managed bool) SubscriptionResult {
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
				consumers:    map[string]subscriptionConsumer{},
				createdAt:    now,
			}
			r.subscriptions[key] = entry
		}
		entry.consumers[consumerID] = subscriptionConsumer{seenAt: now, managed: managed}
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
		consumer, exists := entry.consumers[consumerID]
		if !exists || consumer.managed {
			continue
		}
		consumer.seenAt = now
		entry.consumers[consumerID] = consumer
		entry.updatedAt = now
	}

	return HeartbeatResult(r.snapshotLocked())
}

func (r *subscriptionRegistry) clear(rawConsumerID string) {
	now := r.now().UTC()

	r.mu.Lock()
	defer r.mu.Unlock()

	if strings.TrimSpace(rawConsumerID) == "" {
		for key, entry := range r.subscriptions {
			for consumerID, consumer := range entry.consumers {
				if !consumer.managed {
					delete(entry.consumers, consumerID)
				}
			}
			entry.updatedAt = now
			if len(entry.consumers) == 0 {
				delete(r.subscriptions, key)
			}
		}
		return
	}

	consumerID := normalizeConsumerID(rawConsumerID)
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
	r.expireLocked(r.now().UTC())
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
		sort.Strings(consumers)
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
	r.expireLocked(r.now().UTC())

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

func (r *subscriptionRegistry) activeSubscriptions() []InstrumentRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(r.now().UTC())

	refs := make([]InstrumentRef, 0, len(r.subscriptions))
	for _, entry := range r.subscriptions {
		refs = append(refs, InstrumentRef{
			Channel: entry.channel, Market: entry.market, Symbol: entry.symbol, Interval: entry.interval,
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		_, _, _, _, left := normalizeSubscription(refs[i])
		_, _, _, _, right := normalizeSubscription(refs[j])
		return left < right
	})
	return refs
}

func (r *subscriptionRegistry) expireLocked(now time.Time) {
	if r.externalTTL <= 0 {
		return
	}
	cutoff := now.Add(-r.externalTTL)
	for key, entry := range r.subscriptions {
		changed := false
		for consumerID, consumer := range entry.consumers {
			if consumer.managed || consumer.seenAt.After(cutoff) {
				continue
			}
			delete(entry.consumers, consumerID)
			changed = true
		}
		if changed {
			entry.updatedAt = now
		}
		if len(entry.consumers) == 0 {
			delete(r.subscriptions, key)
		}
	}
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
