package jftradeapi

import (
	"errors"
	"strings"
	"sync"
	"time"
)

var errMarketSubscriptionTargetRequired = errors.New("market and symbol are required")

type marketSubscription struct {
	Key          string
	Channel      string
	Market       string
	Symbol       string
	InstrumentID string
	Interval     string
	Consumers    map[string]time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type marketSubscriptionInput struct {
	Key        string
	Channel    string
	Market     string
	Symbol     string
	Interval   string
	ConsumerID string
}

type marketSubscriptionManager struct {
	mu            sync.Mutex
	subscriptions map[string]*marketSubscription
}

func newMarketSubscriptionManager() marketSubscriptionManager {
	return marketSubscriptionManager{subscriptions: map[string]*marketSubscription{}}
}

func (s *Server) acquireMarketSubscription(payload marketSubscriptionPayload) (map[string]any, error) {
	input := normalizeMarketSubscriptionPayload(payload)
	if input.Market == "" || input.Symbol == "" {
		return nil, errMarketSubscriptionTargetRequired
	}
	return s.marketSubscriptions.acquire(input), nil
}

func (manager *marketSubscriptionManager) acquire(input marketSubscriptionInput) map[string]any {

	now := time.Now().UTC()

	manager.mu.Lock()
	defer manager.mu.Unlock()

	entry := manager.subscriptions[input.Key]
	if entry == nil {
		entry = &marketSubscription{
			Key:          input.Key,
			Channel:      input.Channel,
			Market:       input.Market,
			Symbol:       input.Symbol,
			InstrumentID: input.Market + "." + input.Symbol,
			Interval:     input.Interval,
			Consumers:    map[string]time.Time{},
			CreatedAt:    now,
		}
		manager.subscriptions[input.Key] = entry
	}
	entry.Consumers[input.ConsumerID] = now
	entry.UpdatedAt = now

	return manager.responseLocked()
}

func (s *Server) releaseMarketSubscription(payload marketSubscriptionPayload) map[string]any {
	input := normalizeMarketSubscriptionPayload(payload)
	return s.marketSubscriptions.release(input)
}

func (manager *marketSubscriptionManager) release(input marketSubscriptionInput) map[string]any {
	now := time.Now().UTC()

	manager.mu.Lock()
	defer manager.mu.Unlock()

	if entry := manager.subscriptions[input.Key]; entry != nil {
		delete(entry.Consumers, input.ConsumerID)
		entry.UpdatedAt = now
		if len(entry.Consumers) == 0 {
			delete(manager.subscriptions, input.Key)
		}
	}

	return manager.responseLocked()
}

func (s *Server) heartbeatMarketSubscriptions(consumerID string) map[string]any {
	return s.marketSubscriptions.heartbeat(consumerID)
}

func (manager *marketSubscriptionManager) heartbeat(consumerID string) map[string]any {
	consumerID = normalizeConsumerID(consumerID)
	now := time.Now().UTC()

	manager.mu.Lock()
	defer manager.mu.Unlock()

	for _, entry := range manager.subscriptions {
		if _, exists := entry.Consumers[consumerID]; exists {
			entry.Consumers[consumerID] = now
			entry.UpdatedAt = now
		}
	}

	return manager.responseLocked()
}

func (s *Server) clearMarketSubscriptions(rawConsumerID string) map[string]any {
	return s.marketSubscriptions.clear(rawConsumerID)
}

func (manager *marketSubscriptionManager) clear(rawConsumerID string) map[string]any {
	consumerID := normalizeConsumerID(rawConsumerID)
	now := time.Now().UTC()

	manager.mu.Lock()
	defer manager.mu.Unlock()

	if consumerID == "web" && rawConsumerID == "" {
		manager.subscriptions = map[string]*marketSubscription{}
		return manager.responseLocked()
	}

	for key, entry := range manager.subscriptions {
		delete(entry.Consumers, consumerID)
		entry.UpdatedAt = now
		if len(entry.Consumers) == 0 {
			delete(manager.subscriptions, key)
		}
	}

	return manager.responseLocked()
}

func (s *Server) marketSubscriptionsResponse() map[string]any {
	return s.marketSubscriptions.response()
}

func (manager *marketSubscriptionManager) response() map[string]any {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.responseLocked()
}

func (manager *marketSubscriptionManager) responseLocked() map[string]any {
	entries := make([]map[string]any, 0, len(manager.subscriptions))
	byMarket := map[string]int{}
	for _, entry := range manager.subscriptions {
		consumers := make([]string, 0, len(entry.Consumers))
		for consumerID := range entry.Consumers {
			consumers = append(consumers, consumerID)
		}
		byMarket[entry.Market]++
		var interval any
		if entry.Interval != "" {
			interval = entry.Interval
		}
		entries = append(entries, map[string]any{
			"key":          entry.Key,
			"channel":      entry.Channel,
			"market":       entry.Market,
			"symbol":       entry.Symbol,
			"instrumentId": entry.InstrumentID,
			"interval":     interval,
			"depthLevel":   nil,
			"consumers":    consumers,
			"refCount":     len(consumers),
			"createdAt":    entry.CreatedAt.Format(time.RFC3339Nano),
			"updatedAt":    entry.UpdatedAt.Format(time.RFC3339Nano),
		})
	}

	quotaBuckets := make([]map[string]any, 0, len(byMarket))
	for market, used := range byMarket {
		quotaBuckets = append(quotaBuckets, map[string]any{"market": market, "used": used, "limit": nil, "remaining": nil})
	}

	return map[string]any{
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

func (manager *marketSubscriptionManager) activeInstrumentIDs() []string {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	ids := make([]string, 0, len(manager.subscriptions))
	seen := make(map[string]struct{}, len(manager.subscriptions))
	for _, entry := range manager.subscriptions {
		if entry.Market == "" || entry.Symbol == "" {
			continue
		}
		instrumentID := entry.Market + "." + entry.Symbol
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		ids = append(ids, instrumentID)
	}
	return ids
}

func (manager *marketSubscriptionManager) seed(entry *marketSubscription) {
	if entry == nil {
		return
	}
	manager.mu.Lock()
	manager.subscriptions[entry.Key] = entry
	manager.mu.Unlock()
}

func normalizeMarketSubscriptionPayload(payload marketSubscriptionPayload) marketSubscriptionInput {
	market, symbol, channel := normalizeMarketDataSubscription(payload.Market, payload.Symbol, payload.Channel)
	interval := normalizeMarketSubscriptionInterval(payload.Interval)
	return marketSubscriptionInput{
		Key:        marketSubscriptionKey(channel, market, symbol, interval),
		Channel:    channel,
		Market:     market,
		Symbol:     symbol,
		Interval:   interval,
		ConsumerID: normalizeConsumerID(payload.ConsumerID),
	}
}

func normalizeMarketDataSubscription(market string, symbol string, channel string) (string, string, string) {
	market = strings.ToUpper(strings.TrimSpace(market))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	channel = strings.ToUpper(strings.TrimSpace(channel))
	if channel == "" {
		channel = "SNAPSHOT"
	}
	if strings.Contains(symbol, ".") {
		parts := strings.SplitN(symbol, ".", 2)
		if market == "" {
			market = strings.ToUpper(parts[0])
		}
		symbol = strings.ToUpper(parts[1])
	}
	return market, symbol, channel
}

func normalizeMarketSubscriptionInterval(interval string) string {
	return strings.TrimSpace(strings.ToLower(interval))
}

func normalizeConsumerID(consumerID string) string {
	consumerID = strings.TrimSpace(consumerID)
	if consumerID == "" {
		return "web"
	}
	return consumerID
}

func marketSubscriptionKey(channel string, market string, symbol string, interval string) string {
	if interval == "" {
		return channel + ":" + market + ":" + symbol
	}
	return channel + ":" + market + ":" + symbol + ":" + interval
}
