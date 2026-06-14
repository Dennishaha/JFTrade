package servercore

import (
	"context"
	"strings"
	"time"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	livecore "github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/pkg/broker"
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

func (b liveWebSocketBackend) Heartbeat(interval time.Duration, stats apilive.ClientStats, webSocketInstrumentIDs []string) map[string]any {
	return b.server.liveHeartbeatEvent(interval, stats, webSocketInstrumentIDs)
}

func (b liveWebSocketBackend) MarketTicks(ctx context.Context, instrumentIDs []string, initialObservedAt string) ([]apilive.TickEvent, error) {
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

func (b liveWebSocketBackend) SecurityDetails(ctx context.Context, market, symbol string) (map[string]any, error) {
	return b.server.marketSecurityDetailsResponseForInstrument(ctx, market, symbol)
}

func (b liveWebSocketBackend) SubscribeDepth(ctx context.Context, instrumentID string, num int32) {
	if subscriber, ok := b.server.futuBroker().(broker.OrderBookSubscriber); ok {
		_ = subscriber.SubscribeOrderBook(ctx, broker.OrderBookSubscribeRequest{
			ReadQuery: brokerReadQuery(instrumentID),
			Symbols:   []string{instrumentID},
			Num:       num,
		})
	}
}

func (b liveWebSocketBackend) Depth(ctx context.Context, market, symbol string, num int32) (map[string]any, error) {
	return b.server.marketDepthResponseForInstrument(ctx, market, symbol, marketDepthQuery{
		Num: newOptionalIntValue(int(num)),
	})
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
