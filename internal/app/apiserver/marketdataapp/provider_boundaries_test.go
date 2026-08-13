package marketdataapp

import (
	"context"
	"errors"
	"slices"
	"testing"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
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

type marketDataReaderStub struct{}

func (marketDataReaderStub) QueryFunds(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryPositions(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryOrders(context.Context, broker.ReadQuery, string) ([]broker.OrderSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryHistoryOrders(context.Context, broker.OrderHistoryQuery) ([]broker.OrderSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryOrderFills(context.Context, broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryHistoryOrderFills(context.Context, broker.OrderFillHistoryQuery) ([]broker.OrderFillSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryOrderFees(context.Context, broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryMarginRatios(context.Context, broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryCashFlows(context.Context, broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryMaxTradeQuantity(context.Context, broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryQuote(context.Context, broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryKLines(context.Context, broker.KLineQuery) (*broker.KLineSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QuerySecurityInfo(context.Context, broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QuerySecuritySearch(context.Context, broker.SecuritySearchQuery) (*broker.SecuritySearchSnapshot, error) {
	return nil, nil
}
func (marketDataReaderStub) QuerySecuritySnapshot(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	return nil, nil
}
func (marketDataReaderStub) QueryOrderBook(context.Context, broker.OrderBookQuery) (*broker.OrderBookSnapshot, error) {
	return nil, nil
}

type marketDataBoundaryReader struct {
	marketDataReaderStub
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

func newMarketDataProviderBoundaryProvider(reader broker.MarketDataReader) mdsrv.Provider {
	selected := marketDataBoundaryBroker{reader: reader}
	return NewFutuProvider(FutuProviderDependencies{
		SecurityDetails: func(context.Context, string, string) (mdsrv.SecurityDetails, error) {
			if reader == nil {
				return nil, errFutuIntegrationNotEnabled
			}
			return nil, nil
		},
		LookupInstrument: func(ctx context.Context, marketCode, code string) ([]mdsrv.InstrumentCandidate, error) {
			if reader == nil {
				return nil, errFutuIntegrationNotEnabled
			}
			return LookupInstrument(ctx, selected, marketCode, code, "bbgo:futu")
		},
		SearchInstruments: func(ctx context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error) {
			if reader == nil {
				return nil, errFutuIntegrationNotEnabled
			}
			return SearchInstruments(ctx, selected, query, limit, "bbgo:futu-search")
		},
		HistoricalCandles: func(ctx context.Context, request mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error) {
			if reader == nil {
				return nil, errFutuIntegrationNotEnabled
			}
			return HistoricalCandles(ctx, selected, futuintegration.BrokerID, request, false, "bbgo:futu")
		},
		Health: func(context.Context) (mdsrv.HealthStatus, error) {
			if reader == nil {
				return mdsrv.HealthStatus{Connected: false, LastError: "futu integration is not enabled"}, nil
			}
			return mdsrv.HealthStatus{Connected: true}, nil
		},
	})
}

func TestMarketDataProviderClosureAndOptionalCapabilityBoundaries(t *testing.T) {
	provider := newMarketDataProviderBoundaryProvider(nil)
	if _, err := provider.GetSecurityDetails(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected security details integration error")
	}
	if health, err := provider.Health(context.Background()); err != nil || health.Connected || health.LastError == "" {
		t.Fatalf("disabled provider health = %#v, %v", health, err)
	}

	empty := NewProvider(ProviderOptions{})
	if _, err := empty.LookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected unavailable lookup error")
	}
	if _, err := empty.SearchInstruments(context.Background(), "AAPL", 10); err == nil {
		t.Fatal("expected unavailable search error")
	}
}

func TestMarketDataProviderLookupFailureAndFilteringBoundaries(t *testing.T) {
	if _, err := newMarketDataProviderBoundaryProvider(nil).LookupInstrument(context.Background(), "invalid", ""); err == nil {
		t.Fatal("expected invalid instrument error")
	}

	disabled := newMarketDataProviderBoundaryProvider(nil)
	if _, err := disabled.LookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected disabled broker error")
	}

	provider := newMarketDataProviderBoundaryProvider(nil)
	if _, err := provider.LookupInstrument(context.Background(), "US", "AAPL"); err == nil {
		t.Fatal("expected missing reader error")
	}

	reader := &marketDataBoundaryReader{infoErr: errors.New("forced info error")}
	provider = newMarketDataProviderBoundaryProvider(reader)
	if _, err := provider.LookupInstrument(context.Background(), "US", "AAPL"); !errors.Is(err, reader.infoErr) {
		t.Fatalf("info query error = %v", err)
	}
	reader.infoErr = nil
	if got, err := provider.LookupInstrument(context.Background(), "US", "AAPL"); err != nil || len(got) != 0 {
		t.Fatalf("nil info snapshot = %#v, %v", got, err)
	}
	reader.info = &broker.SecurityInfoSnapshot{Securities: []broker.SecurityInfoItem{
		{Symbol: "not-qualified"},
		{Symbol: "HK.AAPL"},
		{Symbol: "US.MSFT"},
	}}
	if got, err := provider.LookupInstrument(context.Background(), "US", "AAPL"); err != nil || len(got) != 0 {
		t.Fatalf("filtered info candidates = %#v, %v", got, err)
	}
}

func TestMarketDataProviderSearchFailureAndNormalizationBoundaries(t *testing.T) {
	provider := newMarketDataProviderBoundaryProvider(nil)
	if _, err := provider.SearchInstruments(context.Background(), "AAPL", 10); err == nil {
		t.Fatal("expected missing reader search error")
	}
	reader := &marketDataBoundaryReader{searchErr: errors.New("forced search error")}
	provider = newMarketDataProviderBoundaryProvider(reader)
	if _, err := provider.SearchInstruments(context.Background(), "AAPL", 10); !errors.Is(err, reader.searchErr) {
		t.Fatalf("search query error = %v", err)
	}
	reader.searchErr = nil
	if got, err := provider.SearchInstruments(context.Background(), "AAPL", 10); err != nil || len(got) != 0 {
		t.Fatalf("nil search snapshot = %#v, %v", got, err)
	}
	reader.search = &broker.SecuritySearchSnapshot{Entries: []broker.SecuritySearchItem{
		{Market: "", Symbol: ""},
		{Market: "UNKNOWN", Symbol: "UNKNOWN.CODE"},
	}}
	if got, err := provider.SearchInstruments(context.Background(), "x", 10); err != nil || len(got) != 1 || got[0].UnavailableReason == "" {
		t.Fatalf("search candidates = %#v, %v", got, err)
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
	provider := newMarketDataProviderBoundaryProvider(reader)
	response, err := provider.GetHistoricalCandles(context.Background(), mdsrv.HistoricalCandlesQuery{
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

	response, err = provider.GetHistoricalCandles(context.Background(), mdsrv.HistoricalCandlesQuery{
		Market: "US", Symbol: "US.BABA", Period: "1d", Adjustment: "none", Limit: 1000,
		BeforeTime: "2026-08-13T04:00:00Z", Sessions: []mdsrv.CandleSession{mdsrv.CandleSessionRegular},
		SessionsSpecified: true,
	})
	if err != nil {
		t.Fatalf("canonical Futu candles: %v", err)
	}
	if reader.klineQuery.Symbol != "US.BABA" || reader.klineQuery.Market != "US" ||
		reader.klineQuery.Adjustment != "none" {
		t.Fatalf("canonical broker candle query = %#v", reader.klineQuery)
	}
	meta, _ := response["meta"].(map[string]any)
	requestMeta, _ := response["request"].(map[string]any)
	requestInstrument, _ := requestMeta["instrument"].(map[string]any)
	if meta["instrumentId"] != "US.BABA" || requestInstrument["market"] != "US" ||
		requestInstrument["symbol"] != "BABA" || requestInstrument["instrumentId"] != "US.BABA" {
		t.Fatalf("canonical Futu candle response identity = %#v", response)
	}
}

func TestBrokerSearchInstrumentPartsPreservesDottedCodes(t *testing.T) {
	for _, test := range []struct {
		market string
		symbol string
		want   []string
	}{
		{market: "US", symbol: "US.BRK.B", want: []string{"US", "BRK.B"}},
		{market: "US", symbol: "BRK.B", want: []string{"US", "BRK.B"}},
		{market: "SH", symbol: "CNSH.600519", want: []string{"SH", "600519"}},
	} {
		marketCode, code := BrokerSearchInstrumentParts(test.market, test.symbol)
		if got := []string{marketCode, code}; !slices.Equal(got, test.want) {
			t.Errorf("brokerSearchInstrumentParts(%q, %q) = %#v, want %#v", test.market, test.symbol, got, test.want)
		}
	}
}
