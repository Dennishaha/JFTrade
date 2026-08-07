package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type marketDataBoundaryBroker struct{ reader broker.MarketDataReader }

func (b marketDataBoundaryBroker) ID() string { return "futu" }
func (b marketDataBoundaryBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: "futu", Capabilities: []broker.MarketCapability{{
		Market: "US", Features: []broker.FeatureCapability{{
			ID: broker.FeatureMarketCandles, Markets: []string{"US"},
			Access: broker.FeatureAccessRead, State: broker.CapabilityAvailable,
		}},
	}}}
}
func (b marketDataBoundaryBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b marketDataBoundaryBroker) Trading() broker.TradingService      { return nil }
func (b marketDataBoundaryBroker) MarketData() broker.MarketDataReader { return b.reader }

type marketDataBoundaryReader struct {
	servercoreFakeBrokerReader
	info       *broker.SecurityInfoSnapshot
	infoErr    error
	search     *broker.SecuritySearchSnapshot
	searchErr  error
	kline      *broker.KLineSnapshot
	klineErr   error
	klineQuery broker.KLineQuery
}

func (r *marketDataBoundaryReader) QuerySecurityInfo(context.Context, broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error) {
	return r.info, r.infoErr
}

func (r *marketDataBoundaryReader) QuerySecuritySearch(context.Context, broker.SecuritySearchQuery) (*broker.SecuritySearchSnapshot, error) {
	return r.search, r.searchErr
}

func (r *marketDataBoundaryReader) QueryKLines(
	_ context.Context,
	query broker.KLineQuery,
) (*broker.KLineSnapshot, error) {
	r.klineQuery = query
	return r.kline, r.klineErr
}

func newMarketDataProviderBoundaryServer(t *testing.T, reader broker.MarketDataReader) *Server {
	t.Helper()
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := settings.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{Type: "futu", Host: "127.0.0.1", APIPort: 1})}); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	registry := broker.NewRegistry()
	registry.Register(marketDataBoundaryBroker{reader: reader})
	server := &Server{
		serverApplication: serverApplication{
			store: settings,
		},
	}
	server.runtimes.SetBrokerRegistry(registry)
	server.productFeaturesSvc = productsrv.NewService(registry, "futu", nil, nil)
	return server
}

func TestMarketDataProviderClosureAndOptionalCapabilityBoundaries(t *testing.T) {
	disabledSettings, err := NewSettingsStore(filepath.Join(t.TempDir(), "disabled.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	provider := newMarketdataProvider(&Server{serverApplication: serverApplication{
		store: disabledSettings,
	}})
	if _, err := provider.GetSecurityDetails(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected security details integration error")
	}
	if health, err := provider.Health(context.Background()); err != nil || health.Connected || health.LastError == "" {
		t.Fatalf("disabled provider health = %#v, %v", health, err)
	}

	empty := &marketdataProvider{}
	if _, err := empty.LookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected unavailable lookup error")
	}
	if _, err := empty.SearchInstruments(context.Background(), "AAPL", 10); err == nil {
		t.Fatal("expected unavailable search error")
	}
}

func TestMarketDataProviderLookupFailureAndFilteringBoundaries(t *testing.T) {
	server := newMarketDataProviderBoundaryServer(t, nil)
	if _, err := server.marketdataProviderLookupInstrument(context.Background(), "invalid", ""); err == nil {
		t.Fatal("expected invalid instrument error")
	}

	disabledSettings, err := NewSettingsStore(filepath.Join(t.TempDir(), "disabled.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	disabled := &Server{serverApplication: serverApplication{
		store: disabledSettings,
	}}
	if _, err := disabled.marketdataProviderLookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected disabled broker error")
	}

	if _, err := server.marketdataProviderLookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected missing reader error")
	}
	reader := &marketDataBoundaryReader{infoErr: errors.New("forced info error")}
	server.runtimes.Brokers().Replace(marketDataBoundaryBroker{reader: reader})
	if _, err := server.marketdataProviderLookupInstrument(context.Background(), "US", "AAPL"); !errors.Is(err, reader.infoErr) {
		t.Fatalf("info query error = %v", err)
	}
	reader.infoErr = nil
	if got, err := server.marketdataProviderLookupInstrument(context.Background(), "US", "AAPL"); err != nil || len(got) != 0 {
		t.Fatalf("nil info snapshot = %#v, %v", got, err)
	}
	reader.info = &broker.SecurityInfoSnapshot{Securities: []broker.SecurityInfoItem{
		{Symbol: "not-qualified"},
		{Symbol: "HK.AAPL"},
		{Symbol: "US.MSFT"},
	}}
	if got, err := server.marketdataProviderLookupInstrument(context.Background(), "US", "AAPL"); err != nil || len(got) != 0 {
		t.Fatalf("filtered info candidates = %#v, %v", got, err)
	}
}

func TestMarketDataProviderSearchFailureAndNormalizationBoundaries(t *testing.T) {
	server := newMarketDataProviderBoundaryServer(t, nil)
	if _, err := server.marketdataProviderSearchInstruments(context.Background(), "AAPL", 10); err == nil {
		t.Fatal("expected missing reader search error")
	}
	reader := &marketDataBoundaryReader{searchErr: errors.New("forced search error")}
	server.runtimes.Brokers().Replace(marketDataBoundaryBroker{reader: reader})
	if _, err := server.marketdataProviderSearchInstruments(context.Background(), "AAPL", 10); !errors.Is(err, reader.searchErr) {
		t.Fatalf("search query error = %v", err)
	}
	reader.searchErr = nil
	if got, err := server.marketdataProviderSearchInstruments(context.Background(), "AAPL", 10); err != nil || len(got) != 0 {
		t.Fatalf("nil search snapshot = %#v, %v", got, err)
	}
	reader.search = &broker.SecuritySearchSnapshot{Entries: []broker.SecuritySearchItem{
		{Market: "", Symbol: ""},
		{Market: "UNKNOWN", Symbol: "UNKNOWN.CODE"},
	}}
	if got, err := server.marketdataProviderSearchInstruments(context.Background(), "x", 10); err != nil || len(got) != 1 || got[0].UnavailableReason == "" {
		t.Fatalf("search candidates = %#v, %v", got, err)
	}

	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "CNSH", want: "SH"},
		{input: "CNSZ", want: "SZ"},
		{input: "HKFUTURE", want: "HK_FUTURE"},
		{input: "CC", want: "CRYPTO"},
		{input: "US", want: "US"},
		{input: "bad", want: ""},
	} {
		if got := canonicalBrokerSearchMarketPrefix(tc.input); got != tc.want {
			t.Fatalf("canonical prefix %q = %q", tc.input, got)
		}
	}
	if marketCode, code := brokerSearchInstrumentParts("", "CNSH.600000"); marketCode != "SH" || code != "600000" {
		t.Fatalf("inferred search parts = %q/%q", marketCode, code)
	}
	if marketCode, code := brokerSearchInstrumentParts("US", "HK.00700"); marketCode != "US" || code != "HK.00700" {
		t.Fatalf("mismatched search parts = %q/%q", marketCode, code)
	}
}

func TestMarketDataProviderCandleParsingRemainingBoundaries(t *testing.T) {
	open, high, low, volume := 100.0, 102.0, 99.0, 1000.0
	closePrice := 101.5
	reader := &marketDataBoundaryReader{kline: &broker.KLineSnapshot{
		Symbol: "US.AAPL", Period: "5m",
		Pagination: broker.KLinePagination{
			HasMore: true, NextBefore: "2026-07-15T01:00:00Z",
		},
		KLines: []broker.KLineItem{{
			Time: "2026-07-15T01:00:00Z", Open: &open, High: &high, Low: &low,
			Close: &closePrice, Volume: &volume,
		}},
	}}
	server := newMarketDataProviderBoundaryServer(t, reader)
	response, err := server.marketdataProviderHistoricalCandles(context.Background(), mdsrv.HistoricalCandlesQuery{
		Market: "US", Symbol: "AAPL", Period: "5m", Limit: 1000,
		BeforeTime: "2026-07-15T01:02:03.123456789Z",
	})
	if err != nil {
		t.Fatalf("default Futu candles: %v", err)
	}
	if reader.klineQuery.BrokerID != "futu" || reader.klineQuery.Symbol != "US.AAPL" ||
		reader.klineQuery.BeforeTime != "2026-07-15T01:02:03.123456789Z" || reader.klineQuery.Limit != 1000 {
		t.Fatalf("broker candle query = %#v", reader.klineQuery)
	}
	pagination, ok := response["pagination"].(map[string]any)
	if !ok || pagination["hasMore"] != true || pagination["nextBefore"] != "2026-07-15T01:00:00Z" {
		t.Fatalf("default Futu pagination = %#v", response["pagination"])
	}
}
