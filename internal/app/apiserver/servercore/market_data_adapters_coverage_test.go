package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type marketDataCoverageBroker struct{ reader broker.MarketDataReader }

func (b marketDataCoverageBroker) ID() string { return "futu" }
func (b marketDataCoverageBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: "futu"}
}
func (b marketDataCoverageBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b marketDataCoverageBroker) Trading() broker.TradingService      { return nil }
func (b marketDataCoverageBroker) MarketData() broker.MarketDataReader { return b.reader }

type marketDataCoverageReader struct {
	servercoreFakeBrokerReader
	info      *broker.SecurityInfoSnapshot
	infoErr   error
	search    *broker.SecuritySearchSnapshot
	searchErr error
}

func (r *marketDataCoverageReader) QuerySecurityInfo(context.Context, broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error) {
	return r.info, r.infoErr
}

func (r *marketDataCoverageReader) QuerySecuritySearch(context.Context, broker.SecuritySearchQuery) (*broker.SecuritySearchSnapshot, error) {
	return r.search, r.searchErr
}

func newMarketDataAdapterCoverageServer(t *testing.T, reader broker.MarketDataReader) *Server {
	t.Helper()
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if _, err := settings.SaveIntegration(BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(FutuIntegrationConfig{Type: "futu", Host: "127.0.0.1", APIPort: 1})}); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	registry := broker.NewRegistry()
	registry.Register(marketDataCoverageBroker{reader: reader})
	return &Server{store: settings, brokers: registry}
}

func TestMarketDataProviderClosureAndOptionalCapabilityBoundaries(t *testing.T) {
	disabledSettings, err := NewSettingsStore(filepath.Join(t.TempDir(), "disabled.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	provider := newMarketdataProvider(&Server{store: disabledSettings})
	if _, err := provider.GetSecurityDetails(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected security details integration error")
	}
	if health, err := provider.Health(context.Background()); err != nil || health != (mdsrv.HealthStatus{}) {
		t.Fatalf("provider health = %#v, %v", health, err)
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
	server := newMarketDataAdapterCoverageServer(t, nil)
	if _, err := server.marketdataProviderLookupInstrument(context.Background(), "invalid", ""); err == nil {
		t.Fatal("expected invalid instrument error")
	}

	disabledSettings, err := NewSettingsStore(filepath.Join(t.TempDir(), "disabled.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	disabled := &Server{store: disabledSettings}
	if _, err := disabled.marketdataProviderLookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected disabled broker error")
	}

	if _, err := server.marketdataProviderLookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected missing reader error")
	}
	reader := &marketDataCoverageReader{infoErr: errors.New("forced info error")}
	server.brokers.Replace(marketDataCoverageBroker{reader: reader})
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
	server := newMarketDataAdapterCoverageServer(t, nil)
	if _, err := server.marketdataProviderSearchInstruments(context.Background(), "AAPL", 10); err == nil {
		t.Fatal("expected missing reader search error")
	}
	reader := &marketDataCoverageReader{searchErr: errors.New("forced search error")}
	server.brokers.Replace(marketDataCoverageBroker{reader: reader})
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
	query := marketdataProviderCandlesQuery("", 0, "", "invalid")
	if query.Period != "" || query.Limit.Set || !query.FromTime.IsZero() || !query.ToTime.IsZero() {
		t.Fatalf("empty/invalid candle query = %#v", query)
	}
	valid := marketdataProviderOptionalTime("2026-07-15T01:02:03.123456789Z")
	if valid.IsZero() {
		t.Fatal("nanosecond timestamp was not parsed")
	}

	server := newMarketDataTestServerWithQuoteRuntime(t, "127.0.0.1:1")
	if _, err := server.marketdataProviderHistoricalCandles(context.Background(), "US", "AAPL", "tick", 1, "", ""); !errors.Is(err, mdsrv.ErrSubscriptionRequired) {
		t.Fatalf("current K-line without lease error = %v", err)
	}
}
