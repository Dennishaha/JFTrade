package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestProviderDelegatesApplicationCallbacks(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("details failed")
	provider := NewProvider(ProviderOptions{
		Descriptor: func(context.Context) (mdsrv.ProviderDescriptor, error) {
			return mdsrv.ProviderDescriptor{ProviderID: "futu-opend"}, nil
		},
		Markets: func(context.Context) ([]mdsrv.MarketProfile, error) {
			return []mdsrv.MarketProfile{{"code": "US"}}, nil
		},
		NormalizeInstrument: func(_ context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"instrumentId": strings.ToUpper(input["instrumentId"].(string))}, nil
		},
		SecurityDetails: func(context.Context, string, string) (mdsrv.SecurityDetails, error) {
			return nil, expectedErr
		},
		LookupInstrument: func(_ context.Context, market, code string) ([]mdsrv.InstrumentCandidate, error) {
			return []mdsrv.InstrumentCandidate{{Market: market, Code: code, InstrumentID: market + "." + code}}, nil
		},
		SearchInstruments: func(_ context.Context, query string, limit int) ([]mdsrv.InstrumentCandidate, error) {
			return []mdsrv.InstrumentCandidate{{Name: query, LotSize: int32(limit)}}, nil
		},
		QuerySnapshot: func(context.Context, string) (*mdsrv.Tick, error) {
			return &mdsrv.Tick{InstrumentID: "US.AAPL"}, nil
		},
		QueryTicker: func(context.Context, string) (*mdsrv.Tick, error) {
			return &mdsrv.Tick{Kind: mdsrv.TickKindQuote}, nil
		},
		HistoricalCandles: func(context.Context, mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error) {
			return mdsrv.CandlesResponse{"period": "1m"}, nil
		},
		Depth: func(context.Context, string, string, int) (mdsrv.DepthResponse, error) {
			return mdsrv.DepthResponse{"asks": 1}, nil
		},
		Health: func(context.Context) (mdsrv.HealthStatus, error) {
			return mdsrv.HealthStatus{Connected: true, ActiveCount: 2}, nil
		},
	})

	if markets, err := provider.GetMarkets(ctx); err != nil || markets[0]["code"] != "US" {
		t.Fatalf("GetMarkets() = %#v err=%v", markets, err)
	}
	if descriptor, err := provider.Descriptor(ctx); err != nil || descriptor.ProviderID != "futu-opend" {
		t.Fatalf("Descriptor() = %+v err=%v", descriptor, err)
	}
	if normalized, err := provider.NormalizeInstrument(ctx, map[string]any{"instrumentId": "us.aapl"}); err != nil || normalized["instrumentId"] != "US.AAPL" {
		t.Fatalf("NormalizeInstrument() = %#v err=%v", normalized, err)
	}
	if _, err := provider.GetSecurityDetails(ctx, "US", "AAPL"); !errors.Is(err, expectedErr) {
		t.Fatalf("GetSecurityDetails() err=%v", err)
	}
	if candidates, err := provider.LookupInstrument(ctx, "US", "AAPL"); err != nil || candidates[0].InstrumentID != "US.AAPL" {
		t.Fatalf("LookupInstrument() = %#v err=%v", candidates, err)
	}
	if candidates, err := provider.SearchInstruments(ctx, "Apple", 100); err != nil || candidates[0].Name != "Apple" || candidates[0].LotSize != 100 {
		t.Fatalf("SearchInstruments() = %#v err=%v", candidates, err)
	}
	if tick, err := provider.QuerySnapshot(ctx, "US.AAPL"); err != nil || tick.InstrumentID != "US.AAPL" {
		t.Fatalf("QuerySnapshot() = %#v err=%v", tick, err)
	}
	if tick, err := provider.QueryTicker(ctx, "US.MSFT"); err != nil || tick.Kind != mdsrv.TickKindQuote {
		t.Fatalf("QueryTicker() = %#v err=%v", tick, err)
	}
	if candles, err := provider.GetHistoricalCandles(ctx, mdsrv.HistoricalCandlesQuery{}); err != nil || candles["period"] != "1m" {
		t.Fatalf("GetHistoricalCandles() = %#v err=%v", candles, err)
	}
	if depth, err := provider.GetDepth(ctx, "US", "AAPL", 5); err != nil || depth["asks"] != 1 {
		t.Fatalf("GetDepth() = %#v err=%v", depth, err)
	}
	if health, err := provider.Health(ctx); err != nil || !health.Connected || health.ActiveCount != 2 {
		t.Fatalf("Health() = %#v err=%v", health, err)
	}
}

func TestBrokerSearchInstrumentPartsNormalizesKnownPrefixes(t *testing.T) {
	for _, test := range []struct {
		market string
		symbol string
		want   []string
	}{
		{market: "", symbol: "CNSH.600000", want: []string{"SH", "600000"}},
		{market: "", symbol: "CNSZ.000001", want: []string{"SZ", "000001"}},
		{market: "", symbol: "HKFUTURE.MHI", want: []string{"HK_FUTURE", "MHI"}},
		{market: "", symbol: "CC.BTC", want: []string{"CRYPTO", "BTC"}},
		{market: "US", symbol: "US.BRK.B", want: []string{"US", "BRK.B"}},
		{market: "US", symbol: "HK.00700", want: []string{"US", "HK.00700"}},
	} {
		marketCode, code := BrokerSearchInstrumentParts(test.market, test.symbol)
		if marketCode != test.want[0] || code != test.want[1] {
			t.Errorf("BrokerSearchInstrumentParts(%q, %q) = %q/%q, want %q/%q",
				test.market, test.symbol, marketCode, code, test.want[0], test.want[1])
		}
	}
}

func TestBrokerSearchInstrumentPartsRejectsUnknownPrefixInference(t *testing.T) {
	marketCode, code := BrokerSearchInstrumentParts("", "bad.CODE")
	if marketCode != "" || code != "bad.CODE" {
		t.Fatalf("unknown prefix result = %q/%q", marketCode, code)
	}
}
