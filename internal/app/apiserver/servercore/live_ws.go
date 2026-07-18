package servercore

import (
	"context"
	"fmt"
	"strings"
	"time"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	livecore "github.com/jftrade/jftrade-main/internal/live"
)

type liveWebSocketBackend struct {
	server *Server
}

func (b liveWebSocketBackend) ConnectionLimit() int {
	if b.server == nil {
		return defaultMaxWebSocketClients
	}
	limit := b.server.store.Integration().Config.MaxWebSocketConnections
	if limit <= 0 {
		return defaultMaxWebSocketClients
	}
	return limit
}

func (b liveWebSocketBackend) Heartbeat(
	interval time.Duration,
	stats apilive.ClientStats,
	webSocketInstrumentIDs []string,
) map[string]any {
	return b.HeartbeatForProvider(interval, stats, webSocketInstrumentIDs, "")
}

func (b liveWebSocketBackend) HeartbeatForProvider(
	interval time.Duration,
	stats apilive.ClientStats,
	webSocketInstrumentIDs []string,
	providerBrokerID string,
) map[string]any {
	payload := b.server.liveHeartbeatEvent(interval, stats, webSocketInstrumentIDs)
	providerBrokerID = normalizeLiveProviderBrokerID(providerBrokerID)
	payload["providerBrokerId"] = providerBrokerID
	if !usesLegacyLiveProvider(providerBrokerID) {
		transport, _ := payload["transport"].(map[string]any)
		if transport == nil {
			transport = map[string]any{}
			payload["transport"] = transport
		}
		transport["mode"] = "snapshot-poll-fallback"
	}
	return payload
}

func (b liveWebSocketBackend) MarketTicks(
	ctx context.Context,
	instrumentIDs []string,
	initialObservedAt string,
) ([]apilive.TickEvent, error) {
	return b.MarketTicksForProvider(ctx, "", instrumentIDs, initialObservedAt)
}

func (b liveWebSocketBackend) MarketTicksForProvider(
	ctx context.Context,
	providerBrokerID string,
	instrumentIDs []string,
	initialObservedAt string,
) ([]apilive.TickEvent, error) {
	providerBrokerID = normalizeLiveProviderBrokerID(providerBrokerID)
	if !usesLegacyLiveProvider(providerBrokerID) {
		return b.pollBrokerMarketTicks(ctx, providerBrokerID, instrumentIDs)
	}
	b.server.marketdataSvc.WakeCollector()
	result := make([]apilive.TickEvent, 0, len(instrumentIDs))
	for _, sample := range b.server.marketdataSvc.LatestMany(instrumentIDs, liveTickSampleFreshness) {
		if sample == nil {
			continue
		}
		event := b.server.marketdataSvc.LiveTick(sample, initialObservedAt)
		if event == nil {
			continue
		}
		result = append(result, apilive.TickEvent{
			InstrumentID: sample.InstrumentID,
			ObservedAt:   sample.ObservedAt,
			Payload:      event,
		})
	}
	return result, nil
}

func (b liveWebSocketBackend) NotificationsAfter(sequence uint64) []livecore.Event {
	return b.server.liveNotificationsAfter(sequence)
}

func (b liveWebSocketBackend) EnsureNotificationBridge(ctx context.Context) {
	b.server.ensureLiveNotificationBridge(ctx)
}

func (b liveWebSocketBackend) SecurityDetails(
	ctx context.Context,
	market string,
	symbol string,
) (map[string]any, error) {
	return b.SecurityDetailsForProvider(ctx, "", market, symbol)
}

func (b liveWebSocketBackend) SecurityDetailsForProvider(
	ctx context.Context,
	providerBrokerID string,
	market string,
	symbol string,
) (map[string]any, error) {
	providerBrokerID = normalizeLiveProviderBrokerID(providerBrokerID)
	if !usesLegacyLiveProvider(providerBrokerID) {
		if b.server == nil || b.server.productFeaturesSvc == nil {
			return nil, fmt.Errorf("broker market-data reader is unavailable")
		}
		return b.server.productFeaturesSvc.ReadMarketSecurityDetails(
			ctx, providerBrokerID, market, symbol,
		)
	}
	return b.server.marketSecurityDetailsResponseForInstrument(ctx, market, symbol)
}

func (b liveWebSocketBackend) Depth(
	ctx context.Context,
	market string,
	symbol string,
	num int32,
) (map[string]any, error) {
	return b.DepthForProvider(ctx, "", market, symbol, num)
}

func (b liveWebSocketBackend) DepthForProvider(
	ctx context.Context,
	providerBrokerID string,
	market string,
	symbol string,
	num int32,
) (map[string]any, error) {
	providerBrokerID = normalizeLiveProviderBrokerID(providerBrokerID)
	if !usesLegacyLiveProvider(providerBrokerID) {
		if b.server == nil || b.server.productFeaturesSvc == nil {
			return nil, fmt.Errorf("broker market-data reader is unavailable")
		}
		return b.server.productFeaturesSvc.ReadMarketDepth(
			ctx, providerBrokerID, market, symbol, int(num),
		)
	}
	return b.server.marketDepthResponseForInstrument(ctx, market, symbol, marketDepthQuery{
		Num: newOptionalIntValue(int(num)),
	})
}

func (b liveWebSocketBackend) pollBrokerMarketTicks(
	ctx context.Context,
	providerBrokerID string,
	instrumentIDs []string,
) ([]apilive.TickEvent, error) {
	if b.server == nil || b.server.productFeaturesSvc == nil {
		return nil, fmt.Errorf("broker market-data reader is unavailable")
	}
	result := make([]apilive.TickEvent, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		market, symbol, ok := strings.Cut(strings.ToUpper(strings.TrimSpace(instrumentID)), ".")
		if !ok || market == "" || symbol == "" {
			continue
		}
		response, err := b.server.productFeaturesSvc.ReadMarketSnapshot(
			ctx, providerBrokerID, market, symbol, false,
		)
		if err != nil {
			return nil, err
		}
		snapshot, _ := response["snapshot"].(map[string]any)
		meta, _ := response["meta"].(map[string]any)
		observedAt := stringMapValue(snapshot, "observedAt")
		if observedAt == "" {
			observedAt = stringMapValue(meta, "resolvedAt")
		}
		payload := map[string]any{
			"type":       "market-data.tick",
			"at":         observedAt,
			"brokerId":   providerBrokerID,
			"instrument": response["request"],
			"snapshot":   snapshot,
			"source":     stringMapValue(meta, "source"),
		}
		result = append(result, apilive.TickEvent{
			InstrumentID: instrumentID,
			ObservedAt:   observedAt,
			Payload:      payload,
		})
	}
	return result, nil
}

func normalizeLiveProviderBrokerID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "futu"
	}
	return value
}

func usesLegacyLiveProvider(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "futu")
}

func stringMapValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func (b liveWebSocketBackend) SubscribeDepthUpdates(onUpdate func(string)) func() {
	exchange := b.server.futuExchange()
	if exchange == nil {
		return func() {}
	}
	return exchange.OnOrderBookUpdate(func(updatedSymbol string) {
		onUpdate(strings.ToUpper(strings.TrimSpace(updatedSymbol)))
	})
}

func (s *Server) liveStreamStats() (count int, limit int, atLimit bool) {
	if s == nil || s.liveWebSocket == nil {
		return 0, defaultMaxWebSocketClients, false
	}
	stats := s.liveWebSocket.Stats()
	return stats.Connected, stats.Limit, stats.AtLimit
}
