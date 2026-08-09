package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
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

func TestLiveWebSocketUsesActivePollOnlyProviderBehindLegacyFutuSelection(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	sidecar := testkit.New(t)
	if err := marketdataapp.RuntimeFromService(server.marketdataSvc).Activate(t.Context(), marketdataapp.Activation{
		ProviderID:       marketdataapp.ProviderYFinance,
		YFinanceEndpoint: sidecar.URL(),
	}); err != nil {
		t.Fatalf("activate yfinance: %v", err)
	}
	server.marketdataSvc.NotifyProviderChanged()
	server.marketdataSvc.Seed(mdsrv.Tick{
		InstrumentID: "US.AAPL",
		Market:       "US",
		Symbol:       "AAPL",
		Price:        decimal.NewFromInt(190),
		ObservedAt:   time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339Nano),
		QuoteAt:      time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339Nano),
		Source:       "yfinance",
	})

	backend := newLiveWebSocketBackend(server)
	heartbeat := backend.Heartbeat(
		time.Second,
		apilive.ClientStats{},
		[]string{"US.AAPL"},
		"futu",
	)
	transport := heartbeat["transport"].(map[string]any)
	if heartbeat["providerBrokerId"] != "futu" ||
		heartbeat["marketDataProviderId"] != marketdataapp.ProviderYFinance ||
		transport["mode"] != "snapshot-poll-delayed" ||
		transport["sampleFreshnessMs"].(int64) <= liveHeartbeatStaleThreshold.Milliseconds() {
		t.Fatalf("yfinance heartbeat = %#v", heartbeat)
	}
	if reasons := heartbeat["staleReasons"].([]any); len(reasons) != 0 {
		t.Fatalf("yfinance heartbeat stale reasons = %#v", reasons)
	}
	ticks, err := backend.MarketTicks(t.Context(), "futu", []string{"US.AAPL"}, "")
	if err != nil || len(ticks) != 1 ||
		ticks[0].Payload["source"] != "yfinance" ||
		ticks[0].Payload["brokerId"] != "futu" ||
		ticks[0].Payload["marketDataProviderId"] != marketdataapp.ProviderYFinance {
		t.Fatalf("yfinance live ticks = %#v, err=%v", ticks, err)
	}
	details, err := backend.SecurityDetails(t.Context(), "futu", "US", "AAPL")
	if err != nil || details["meta"].(map[string]any)["source"] != "yfinance" {
		t.Fatalf("yfinance live security details = %#v, err=%v", details, err)
	}
	if _, err := backend.Depth(t.Context(), "futu", "US", "AAPL", 10); !errors.Is(
		err,
		mdsrv.ErrCapabilityUnsupported,
	) {
		t.Fatalf("yfinance live depth error = %v", err)
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
	nilBackend := newLiveWebSocketBackend(nil)
	if got := nilBackend.ConnectionLimit(); got != defaultMaxWebSocketClients {
		t.Fatalf("nil backend connection limit = %d", got)
	}
	if _, err := nilBackend.MarketTicks(t.Context(), "ibkr", []string{"US.AAPL"}, ""); err == nil {
		t.Fatal("nil backend polled broker ticks")
	}
	if _, err := nilBackend.SecurityDetails(t.Context(), "ibkr", "US", "AAPL"); err == nil {
		t.Fatal("nil backend read broker security details")
	}
	if _, err := nilBackend.Depth(t.Context(), "ibkr", "US", "AAPL", 10); err == nil {
		t.Fatal("nil backend read broker depth")
	}
	if count, limit, atLimit := liveStreamStats((*serverApplication)(nil)); count != 0 || limit != defaultMaxWebSocketClients || atLimit {
		t.Fatalf("nil live stats = %d/%d/%v", count, limit, atLimit)
	}

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	backend := newLiveWebSocketBackend(server)
	if got := backend.ConnectionLimit(); got != defaultMaxWebSocketClients {
		t.Fatalf("default connection limit = %d", got)
	}
	native := backend.Heartbeat(time.Second, apilive.ClientStats{}, nil, " FUTU ")
	if native["providerBrokerId"] != "futu" {
		t.Fatalf("native heartbeat = %#v", native)
	}
	broker := backend.Heartbeat(time.Second, apilive.ClientStats{}, nil, " IBKR ")
	transport, _ := broker["transport"].(map[string]any)
	if broker["providerBrokerId"] != "ibkr" || transport["mode"] != "snapshot-poll-fallback" {
		t.Fatalf("broker heartbeat = %#v", broker)
	}
	if ticks, err := backend.MarketTicks(t.Context(), "futu", nil, ""); err != nil || len(ticks) != 0 {
		t.Fatalf("empty native ticks = %#v, err=%v", ticks, err)
	}
	if _, err := backend.SecurityDetails(t.Context(), "futu", "US", "MISSING"); err == nil {
		t.Fatal("missing native security details returned no error")
	}
	if _, err := backend.Depth(t.Context(), "futu", "US", "MISSING", 10); err == nil {
		t.Fatal("missing native depth returned no error")
	}
	if _, err := backend.MarketTicks(t.Context(), "", nil, ""); err == nil {
		t.Fatal("missing provider was accepted")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	backend.EnsureNotificationBridge(cancelled)
	_ = backend.NotificationsAfter(0)
	unsubscribe := backend.SubscribeDepthUpdates(func(string) {})
	unsubscribe()
	liveStreamStats(&server.serverApplication)

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
	server := &Server{}
	server.runtimes.SetBrokerRegistry(registry)
	server.productFeaturesSvc = productsrv.NewService(registry, "ibkr", nil, nil)
	backend := newLiveWebSocketBackend(server)

	ticks, err := backend.MarketTicks(t.Context(), "ibkr", []string{"invalid", "US.AAPL"}, "")
	if err != nil || len(ticks) != 1 || ticks[0].InstrumentID != "US.AAPL" || ticks[0].ObservedAt == "" {
		t.Fatalf("broker snapshot ticks = %#v, err=%v", ticks, err)
	}
	if ticks[0].Payload["brokerId"] != "ibkr" || ticks[0].Payload["snapshot"] == nil {
		t.Fatalf("broker tick payload = %#v", ticks[0].Payload)
	}

	reader.err = errors.New("snapshot provider failed")
	if _, err := backend.MarketTicks(t.Context(), "ibkr", []string{"US.MSFT"}, ""); !errors.Is(err, reader.err) {
		t.Fatalf("broker snapshot error = %v", err)
	}
}
