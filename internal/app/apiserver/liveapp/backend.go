package liveapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	livecore "github.com/jftrade/jftrade-main/internal/live"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
)

// BackendOptions supplies the application-owned services and event callbacks
// behind the live WebSocket transport.
type BackendOptions struct {
	DefaultConnectionLimit   int
	ConnectionLimit          func() int
	Heartbeat                func(time.Duration, apilive.ClientStats, []string) map[string]any
	MarketData               func() *mdsrv.Service
	ProductFeatures          func() *productsrv.Service
	SampleFreshnessThreshold time.Duration
	NotificationsAfter       func(uint64) []livecore.Event
	EnsureNotificationBridge func(context.Context)
	SubscribeNativeDepth     func(func(string)) func()
}

// Backend adapts application services to the live transport contract.
type Backend struct {
	options BackendOptions
}

var _ apilive.Backend = (*Backend)(nil)

func NewBackend(options BackendOptions) *Backend {
	return &Backend{options: options}
}

func (b *Backend) ConnectionLimit() int {
	if b != nil && b.options.ConnectionLimit != nil {
		if limit := b.options.ConnectionLimit(); limit > 0 {
			return limit
		}
	}
	if b != nil && b.options.DefaultConnectionLimit > 0 {
		return b.options.DefaultConnectionLimit
	}
	return 20
}

func (b *Backend) Heartbeat(
	interval time.Duration,
	stats apilive.ClientStats,
	webSocketInstrumentIDs []string,
	providerBrokerID string,
) map[string]any {
	payload := map[string]any{}
	if b != nil && b.options.Heartbeat != nil {
		payload = b.options.Heartbeat(interval, stats, webSocketInstrumentIDs)
		if payload == nil {
			payload = map[string]any{}
		}
	}
	providerBrokerID = normalizeProviderBrokerID(providerBrokerID)
	nativeProviderID, native := b.nativeMarketDataProvider(providerBrokerID)
	payload["providerBrokerId"] = providerBrokerID
	if native && nativeProviderID != "" {
		payload["marketDataProviderId"] = nativeProviderID
	}
	if providerBrokerID != "" && !native {
		transport, _ := payload["transport"].(map[string]any)
		if transport == nil {
			transport = map[string]any{}
			payload["transport"] = transport
		}
		transport["mode"] = "snapshot-poll-fallback"
	}
	return payload
}

func (b *Backend) MarketTicks(
	ctx context.Context,
	providerBrokerID string,
	instrumentIDs []string,
	initialObservedAt string,
) ([]apilive.TickEvent, error) {
	providerBrokerID, err := requireProviderBrokerID(providerBrokerID)
	if err != nil {
		return nil, err
	}
	nativeProviderID, native := b.nativeMarketDataProvider(providerBrokerID)
	if !native {
		return b.pollBrokerMarketTicks(ctx, providerBrokerID, instrumentIDs)
	}
	marketData := b.marketData()
	if marketData == nil {
		return nil, fmt.Errorf("active market-data service is unavailable")
	}
	marketData.WakeCollector()
	result := make([]apilive.TickEvent, 0, len(instrumentIDs))
	for _, sample := range marketData.LatestMany(
		instrumentIDs,
		marketdataapp.SampleFreshness(marketData, b.options.SampleFreshnessThreshold),
	) {
		if sample == nil {
			continue
		}
		event := marketData.LiveTick(sample, initialObservedAt)
		if event == nil {
			continue
		}
		event["brokerId"] = providerBrokerID
		event["marketDataProviderId"] = nativeProviderID
		result = append(result, apilive.TickEvent{
			InstrumentID: sample.InstrumentID, ObservedAt: sample.ObservedAt, Payload: event,
		})
	}
	return result, nil
}

func (b *Backend) NotificationsAfter(sequence uint64) []livecore.Event {
	if b == nil || b.options.NotificationsAfter == nil {
		return nil
	}
	return b.options.NotificationsAfter(sequence)
}

func (b *Backend) EnsureNotificationBridge(ctx context.Context) {
	if b != nil && b.options.EnsureNotificationBridge != nil {
		b.options.EnsureNotificationBridge(ctx)
	}
}

func (b *Backend) SecurityDetails(
	ctx context.Context,
	providerBrokerID string,
	marketCode string,
	symbol string,
) (map[string]any, error) {
	providerBrokerID, err := requireProviderBrokerID(providerBrokerID)
	if err != nil {
		return nil, err
	}
	if _, native := b.nativeMarketDataProvider(providerBrokerID); !native {
		productFeatures := b.productFeatures()
		if productFeatures == nil {
			return nil, fmt.Errorf("broker market-data reader is unavailable")
		}
		return productFeatures.ReadMarketSecurityDetails(
			ctx, providerBrokerID, marketCode, symbol,
		)
	}
	marketData := b.marketData()
	if marketData == nil {
		return nil, fmt.Errorf("active market-data service is unavailable")
	}
	details, err := marketData.GetSecurityDetails(ctx, marketCode, symbol)
	return map[string]any(details), err
}

func (b *Backend) Depth(
	ctx context.Context,
	providerBrokerID string,
	marketCode string,
	symbol string,
	num int32,
) (map[string]any, error) {
	providerBrokerID, err := requireProviderBrokerID(providerBrokerID)
	if err != nil {
		return nil, err
	}
	if _, native := b.nativeMarketDataProvider(providerBrokerID); !native {
		productFeatures := b.productFeatures()
		if productFeatures == nil {
			return nil, fmt.Errorf("broker market-data reader is unavailable")
		}
		return productFeatures.ReadMarketDepth(
			ctx, providerBrokerID, marketCode, symbol, int(num),
		)
	}
	marketData := b.marketData()
	if marketData == nil {
		return nil, fmt.Errorf("active market-data service is unavailable")
	}
	depth, err := marketData.GetDepth(ctx, marketCode, symbol, int(num))
	return map[string]any(depth), err
}

func (b *Backend) SubscribeDepthUpdates(onUpdate func(string)) func() {
	if b == nil || b.options.SubscribeNativeDepth == nil {
		return func() {}
	}
	unsubscribe := b.options.SubscribeNativeDepth(func(updatedSymbol string) {
		onUpdate(strings.ToUpper(strings.TrimSpace(updatedSymbol)))
	})
	if unsubscribe == nil {
		return func() {}
	}
	return unsubscribe
}

func (b *Backend) pollBrokerMarketTicks(
	ctx context.Context,
	providerBrokerID string,
	instrumentIDs []string,
) ([]apilive.TickEvent, error) {
	productFeatures := b.productFeatures()
	if productFeatures == nil {
		return nil, fmt.Errorf("broker market-data reader is unavailable")
	}
	result := make([]apilive.TickEvent, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		marketCode, symbol, ok := strings.Cut(strings.ToUpper(strings.TrimSpace(instrumentID)), ".")
		if !ok || marketCode == "" || symbol == "" {
			continue
		}
		response, err := productFeatures.ReadMarketSnapshot(
			ctx, providerBrokerID, marketCode, symbol, false,
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
		result = append(result, apilive.TickEvent{
			InstrumentID: instrumentID,
			ObservedAt:   observedAt,
			Payload: map[string]any{
				"type": "market-data.tick", "at": observedAt, "brokerId": providerBrokerID,
				"instrument": response["request"], "snapshot": snapshot, "source": stringMapValue(meta, "source"),
			},
		})
	}
	return result, nil
}

func (b *Backend) nativeMarketDataProvider(value string) (string, bool) {
	value = normalizeProviderBrokerID(value)
	if value == "" {
		return "", false
	}
	if b != nil {
		runtime := marketdataapp.RuntimeFromService(b.marketData())
		if runtime != nil {
			active := normalizeProviderBrokerID(runtime.ActiveProviderID())
			if usesNativeFutuLiveProvider(value) || value == active {
				return active, true
			}
		}
	}
	if usesNativeFutuLiveProvider(value) {
		return "futu", true
	}
	return value, false
}

func normalizeProviderBrokerID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func requireProviderBrokerID(value string) (string, error) {
	value = normalizeProviderBrokerID(value)
	if value == "" {
		return "", fmt.Errorf("provider broker id is required")
	}
	return value, nil
}

func usesNativeFutuLiveProvider(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "futu")
}

func stringMapValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func (b *Backend) marketData() *mdsrv.Service {
	if b == nil || b.options.MarketData == nil {
		return nil
	}
	return b.options.MarketData()
}

func (b *Backend) productFeatures() *productsrv.Service {
	if b == nil || b.options.ProductFeatures == nil {
		return nil
	}
	return b.options.ProductFeatures()
}
