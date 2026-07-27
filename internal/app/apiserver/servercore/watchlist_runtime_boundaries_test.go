package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jftrade/jftrade-main/pkg/broker"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type futuWatchlistBoundaryBroker struct {
	reader broker.MarketDataReader
	groups bool
}

func (b futuWatchlistBoundaryBroker) ID() string { return "futu" }
func (b futuWatchlistBoundaryBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: "futu"}
}
func (b futuWatchlistBoundaryBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b futuWatchlistBoundaryBroker) Trading() broker.TradingService      { return nil }
func (b futuWatchlistBoundaryBroker) MarketData() broker.MarketDataReader { return b.reader }
func (b futuWatchlistBoundaryBroker) ListWatchlistGroups(context.Context) ([]broker.WatchlistGroup, error) {
	if !b.groups {
		return nil, errors.New("disabled")
	}
	return nil, nil
}
func (b futuWatchlistBoundaryBroker) ListWatchlistGroupSecurities(context.Context, string) ([]broker.WatchlistSecurity, error) {
	return nil, nil
}

type futuBrokerWithoutWatchlists struct{ reader broker.MarketDataReader }

func (b futuBrokerWithoutWatchlists) ID() string { return "futu" }
func (b futuBrokerWithoutWatchlists) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: "futu"}
}
func (b futuBrokerWithoutWatchlists) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b futuBrokerWithoutWatchlists) Trading() broker.TradingService      { return nil }
func (b futuBrokerWithoutWatchlists) MarketData() broker.MarketDataReader { return b.reader }

func TestFutuWatchlistProbeErrorRemainingStates(t *testing.T) {
	detail := "connection refused"
	quoteLoggedOut := false
	for _, test := range []struct {
		probe opendProbe
		want  string
	}{
		{probe: opendProbe{Connectivity: "disconnected", LastError: &detail}, want: detail},
		{probe: opendProbe{Connectivity: "disconnected"}, want: "not connected"},
		{probe: opendProbe{Connectivity: "connected", QuoteLoggedIn: &quoteLoggedOut}, want: "not logged in"},
	} {
		if err := futuWatchlistProbeError(test.probe); err == nil || !strings.Contains(err.Error(), test.want) || !errors.Is(err, watchlist.ErrUnavailable) {
			t.Fatalf("probe %+v error = %v", test.probe, err)
		}
	}
	if err := futuWatchlistProbeError(opendProbe{Connectivity: "connected"}); err != nil {
		t.Fatalf("connected probe error = %v", err)
	}
}

func TestFutuWatchlistBrokerRemainingAvailabilityAndCapabilities(t *testing.T) {
	var nilServer *serverApplication
	if _, err := nilServer.futuWatchlistBroker(); !errors.Is(err, watchlist.ErrUnavailable) {
		t.Fatalf("nil broker error = %v", err)
	}

	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{Type: "futu", Host: "127.0.0.1", APIPort: 11110})}); err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	server.runtimes.SetMarketData(nil)
	if _, err := server.futuWatchlistBroker(); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil runtime broker error = %v", err)
	}

	server.runtimes.SetMarketData(futuintegration.NewMarketDataRuntime(futuintegration.MarketDataRuntimeOptions{
		ConfigSource: func() futuintegration.MarketDataConfig {
			return futuintegration.MarketDataConfig{Enabled: true, Host: "127.0.0.1", APIPort: 11110}
		},
	}))
	t.Cleanup(func() { _ = server.runtimes.MarketData().Close() })
	server.runtimes.SetBrokerRegistry(nil)
	if _, err := server.futuWatchlistBroker(); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil registry broker error = %v", err)
	}
	server.runtimes.SetBrokerRegistry(broker.NewRegistry())
	if _, err := server.futuWatchlistBroker(); err == nil || !strings.Contains(err.Error(), "adapter") {
		t.Fatalf("missing adapter broker error = %v", err)
	}

	server.runtimes.Brokers().Replace(futuBrokerWithoutWatchlists{})
	if value, err := server.futuWatchlistBroker(); err != nil || value == nil {
		t.Fatalf("resolved broker = %#v, %v", value, err)
	}
	if _, err := server.futuWatchlistGroupReader(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported watchlist reader error = %v", err)
	}
	if _, err := server.futuWatchlistBatchSnapshotSource(); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil snapshot reader error = %v", err)
	}

	reader := &servercoreFakeBrokerReader{}
	server.runtimes.Brokers().Replace(futuWatchlistBoundaryBroker{reader: reader, groups: true})
	if groupReader, err := server.futuWatchlistGroupReader(); err != nil || groupReader == nil {
		t.Fatalf("watchlist group reader = %#v, %v", groupReader, err)
	}
	if snapshots, err := server.futuWatchlistBatchSnapshotSource(); err != nil || snapshots == nil {
		t.Fatalf("snapshot source = %#v, %v", snapshots, err)
	}
}

func TestInitializeWatchlistServiceNilBoundaries(t *testing.T) {
	var nilServer *serverApplication
	nilServer.initializeWatchlistService()

	server := &Server{}
	server.initializeWatchlistService()
	if server.watchlistSvc != nil {
		t.Fatalf("initializeWatchlistService without watchlistStore created service = %#v", server.watchlistSvc)
	}
}
