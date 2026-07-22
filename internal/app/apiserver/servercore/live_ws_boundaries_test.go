package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type liveSnapshotBroker struct{ reader *liveSnapshotReader }

func (b liveSnapshotBroker) ID() string { return "ibkr" }
func (b liveSnapshotBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{
		ID: "ibkr", Capabilities: []broker.MarketCapability{{
			Market: "US", SupportsQuote: true,
			Features: []broker.FeatureCapability{{
				ID: broker.FeatureMarketSnapshot, Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable,
			}, {
				ID: broker.FeatureMarketSnapshots, Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable,
			}},
		}},
	}
}
func (b liveSnapshotBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b liveSnapshotBroker) Trading() broker.TradingService      { return nil }
func (b liveSnapshotBroker) MarketData() broker.MarketDataReader { return b.reader }
func (b liveSnapshotBroker) QuerySecuritySnapshot(ctx context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	return b.reader.QuerySecuritySnapshot(ctx, query)
}

type liveSnapshotReader struct {
	servercoreFakeBrokerReader
	result *broker.SecuritySnapshotResult
	err    error
}

func (reader *liveSnapshotReader) QuerySecuritySnapshot(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	return reader.result, reader.err
}

func TestLiveWebSocketBackendProviderAndNilBoundaries(t *testing.T) {
	nilBackend := liveWebSocketBackend{}
	if got := nilBackend.ConnectionLimit(); got != defaultMaxWebSocketClients {
		t.Fatalf("nil backend connection limit = %d", got)
	}
	if _, err := nilBackend.MarketTicksForProvider(t.Context(), "ibkr", []string{"US.AAPL"}, ""); err == nil {
		t.Fatal("nil backend polled broker ticks")
	}
	if _, err := nilBackend.SecurityDetailsForProvider(t.Context(), "ibkr", "US", "AAPL"); err == nil {
		t.Fatal("nil backend read broker security details")
	}
	if _, err := nilBackend.DepthForProvider(t.Context(), "ibkr", "US", "AAPL", 10); err == nil {
		t.Fatal("nil backend read broker depth")
	}
	if count, limit, atLimit := (*Server)(nil).liveStreamStats(); count != 0 || limit != defaultMaxWebSocketClients || atLimit {
		t.Fatalf("nil live stats = %d/%d/%v", count, limit, atLimit)
	}

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	backend := liveWebSocketBackend{server: server}
	if got := backend.ConnectionLimit(); got != defaultMaxWebSocketClients {
		t.Fatalf("default connection limit = %d", got)
	}
	legacy := backend.Heartbeat(time.Second, apilive.ClientStats{}, nil)
	if legacy["providerBrokerId"] != "futu" {
		t.Fatalf("legacy heartbeat = %#v", legacy)
	}
	broker := backend.HeartbeatForProvider(time.Second, apilive.ClientStats{}, nil, " IBKR ")
	transport, _ := broker["transport"].(map[string]any)
	if broker["providerBrokerId"] != "ibkr" || transport["mode"] != "snapshot-poll-fallback" {
		t.Fatalf("broker heartbeat = %#v", broker)
	}
	if ticks, err := backend.MarketTicks(t.Context(), nil, ""); err != nil || len(ticks) != 0 {
		t.Fatalf("empty legacy ticks = %#v, err=%v", ticks, err)
	}
	if _, err := backend.SecurityDetails(t.Context(), "US", "MISSING"); err == nil {
		t.Fatal("missing legacy security details returned no error")
	}
	if _, err := backend.Depth(t.Context(), "US", "MISSING", 10); err == nil {
		t.Fatal("missing legacy depth returned no error")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	backend.EnsureNotificationBridge(cancelled)
	_ = backend.NotificationsAfter(0)
	unsubscribe := backend.SubscribeDepthUpdates(func(string) {})
	unsubscribe()
	server.liveStreamStats()

	if normalizeLiveProviderBrokerID(" ") != "futu" || normalizeLiveProviderBrokerID(" IBKR ") != "ibkr" {
		t.Fatal("provider normalization mismatch")
	}
	if !usesLegacyLiveProvider(" FUTU ") || usesLegacyLiveProvider("ibkr") {
		t.Fatal("legacy provider classification mismatch")
	}
	if got := stringMapValue(map[string]any{"value": " trimmed "}, "value"); got != "trimmed" {
		t.Fatalf("string map value = %q", got)
	}
}

func TestLiveWebSocketBackendPollsExplicitBrokerSnapshots(t *testing.T) {
	observedAt := time.Date(2026, time.July, 22, 6, 30, 0, 0, time.UTC)
	lastPrice := 213.25
	reader := &liveSnapshotReader{result: &broker.SecuritySnapshotResult{
		Snapshots: []broker.SecuritySnapshotItem{{
			Symbol: "US.AAPL", LastPrice: &lastPrice, ObservedAt: observedAt,
		}},
	}}
	registry := broker.NewRegistry()
	registry.Register(liveSnapshotBroker{reader: reader})
	server := &Server{serverRuntimes: serverRuntimes{brokers: registry}}
	server.productFeaturesSvc = productsrv.NewService(registry, "ibkr", nil, nil)
	backend := liveWebSocketBackend{server: server}

	ticks, err := backend.pollBrokerMarketTicks(t.Context(), "ibkr", []string{"invalid", "US.AAPL"})
	if err != nil || len(ticks) != 1 || ticks[0].InstrumentID != "US.AAPL" || ticks[0].ObservedAt == "" {
		t.Fatalf("broker snapshot ticks = %#v, err=%v", ticks, err)
	}
	if ticks[0].Payload["brokerId"] != "ibkr" || ticks[0].Payload["snapshot"] == nil {
		t.Fatalf("broker tick payload = %#v", ticks[0].Payload)
	}

	reader.err = errors.New("snapshot provider failed")
	if _, err := backend.MarketTicksForProvider(t.Context(), "ibkr", []string{"US.MSFT"}, ""); !errors.Is(err, reader.err) {
		t.Fatalf("broker snapshot error = %v", err)
	}
}
