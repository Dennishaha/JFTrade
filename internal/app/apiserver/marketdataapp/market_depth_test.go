package marketdataapp

import (
	"encoding/json"
	"testing"

	httpserver "github.com/jftrade/jftrade-main/internal/api/httpserver"
	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func marketDataDepthOrderBookFixture(price float64, volume int64, orderCount int32) fututestkit.OrderBookEntry {
	return fututestkit.OrderBookEntry{Price: price, Volume: volume, OrderCount: orderCount}
}

func acquireMarketDataDepthSubscription(t *testing.T, harness *marketDataTestHarness, market, symbol string) {
	t.Helper()
	harness.Service.SetSubscriptionReconciler(harness.Runtime)
	if _, err := harness.Service.AcquireSubscription(t.Context(), "test-depth", []mdsrv.InstrumentRef{{
		Channel: "ORDER_BOOK",
		Market:  market,
		Symbol:  symbol,
	}}); err != nil {
		t.Fatalf("acquire depth subscription: %v", err)
	}
	if err := harness.Runtime.ReconcileSubscriptions(t.Context(), []mdsrv.InstrumentRef{{
		Channel: "ORDER_BOOK",
		Market:  market,
		Symbol:  symbol,
	}}); err != nil {
		t.Fatalf("reconcile depth subscription: %v", err)
	}
}

func TestMarketDepthResponseWithMockOpenD(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetOrderBook(
		[]fututestkit.OrderBookEntry{
			marketDataDepthOrderBookFixture(155.0, 1000, 5),
			marketDataDepthOrderBookFixture(154.5, 500, 3),
		},
		[]fututestkit.OrderBookEntry{
			marketDataDepthOrderBookFixture(155.5, 800, 4),
			marketDataDepthOrderBookFixture(156.0, 1200, 6),
		},
	)

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataDepthSubscription(t, harness, "US", "NVDA")
	response, err := harness.Adapters.DepthResponseForInstrument(
		t.Context(), "US", "NVDA", DepthQuery{
			Num: httpserver.OptionalIntValue{Value: 10, Set: true, Valid: true},
		},
	)
	if err != nil {
		t.Fatalf("depth response: %v", err)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal depth response: %v", err)
	}

	var envelope struct {
		Request struct {
			Market       string `json:"market"`
			Symbol       string `json:"symbol"`
			InstrumentID string `json:"instrumentId"`
			Num          int    `json:"num"`
		} `json:"request"`
		Depth struct {
			Symbol     string `json:"symbol"`
			SymbolName string `json:"symbolName"`
			Bids       []struct {
				Price      float64 `json:"price"`
				Volume     float64 `json:"volume"`
				OrderCount int32   `json:"orderCount"`
			} `json:"bids"`
			Asks []struct {
				Price      float64 `json:"price"`
				Volume     float64 `json:"volume"`
				OrderCount int32   `json:"orderCount"`
			} `json:"asks"`
		} `json:"depth"`
		Meta struct {
			InstrumentID string `json:"instrumentId"`
			Source       string `json:"source"`
			FromCache    bool   `json:"fromCache"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode depth response: %v", err)
	}

	if envelope.Request.Market != "US" {
		t.Errorf("request market = %q, want US", envelope.Request.Market)
	}
	if envelope.Request.Symbol != "NVDA" {
		t.Errorf("request symbol = %q, want NVDA", envelope.Request.Symbol)
	}
	if envelope.Request.InstrumentID != "US.NVDA" {
		t.Errorf("request instrumentId = %q, want US.NVDA", envelope.Request.InstrumentID)
	}
	if envelope.Request.Num != 10 {
		t.Errorf("request num = %d, want 10", envelope.Request.Num)
	}
	if envelope.Depth.Symbol != "US.NVDA" {
		t.Errorf("depth symbol = %q, want US.NVDA", envelope.Depth.Symbol)
	}
	if len(envelope.Depth.Bids) != 2 {
		t.Fatalf("bids len = %d, want 2", len(envelope.Depth.Bids))
	}
	if envelope.Depth.Bids[0].Price != 155.0 {
		t.Errorf("bids[0].price = %f, want 155.0", envelope.Depth.Bids[0].Price)
	}
	if envelope.Depth.Bids[0].Volume != 1000 {
		t.Errorf("bids[0].volume = %f, want 1000", envelope.Depth.Bids[0].Volume)
	}
	if envelope.Depth.Bids[0].OrderCount != 5 {
		t.Errorf("bids[0].orderCount = %d, want 5", envelope.Depth.Bids[0].OrderCount)
	}
	if len(envelope.Depth.Asks) != 2 {
		t.Fatalf("asks len = %d, want 2", len(envelope.Depth.Asks))
	}
	if envelope.Depth.Asks[0].Price != 155.5 {
		t.Errorf("asks[0].price = %f, want 155.5", envelope.Depth.Asks[0].Price)
	}
	if envelope.Meta.InstrumentID != "US.NVDA" {
		t.Errorf("meta instrumentId = %q, want US.NVDA", envelope.Meta.InstrumentID)
	}
	if envelope.Meta.Source != "bbgo:futu" {
		t.Errorf("meta source = %q, want bbgo:futu", envelope.Meta.Source)
	}
	if envelope.Meta.FromCache {
		t.Error("meta fromCache should be false for direct depth query")
	}
	if got := quoteServer.OrderBookCallCount(); got != 1 {
		t.Errorf("orderBook OpenD calls = %d, want 1", got)
	}
}

func TestMarketDepthNumClamping(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetOrderBook(
		[]fututestkit.OrderBookEntry{marketDataDepthOrderBookFixture(100, 10, 1)},
		[]fututestkit.OrderBookEntry{marketDataDepthOrderBookFixture(101, 10, 1)},
	)

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataDepthSubscription(t, harness, "HK", "00700")

	tests := []struct {
		name      string
		queryNum  httpserver.OptionalIntValue
		expectNum int
	}{
		{"num=0 clamps to 1", httpserver.OptionalIntValue{Value: 0, Set: true, Valid: true}, 1},
		{"num=-5 clamps to 1", httpserver.OptionalIntValue{Value: -5, Set: true, Valid: true}, 1},
		{"num=100 clamps to 50", httpserver.OptionalIntValue{Value: 100, Set: true, Valid: true}, 50},
		{"num=50 is max valid", httpserver.OptionalIntValue{Value: 50, Set: true, Valid: true}, 50},
		{"no num defaults to 10", httpserver.OptionalIntValue{}, 10},
		{"num=5 is valid", httpserver.OptionalIntValue{Value: 5, Set: true, Valid: true}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := harness.Adapters.DepthResponseForInstrument(
				t.Context(), "HK", "00700", DepthQuery{Num: tt.queryNum},
			)
			if err != nil {
				t.Fatalf("depth response: %v", err)
			}
			payload, err := json.Marshal(response)
			if err != nil {
				t.Fatalf("marshal depth response: %v", err)
			}
			var envelope struct {
				Request struct {
					Num int `json:"num"`
				} `json:"request"`
			}
			if err := json.Unmarshal(payload, &envelope); err != nil {
				t.Fatalf("decode depth response: %v", err)
			}
			if envelope.Request.Num != tt.expectNum {
				t.Errorf("response request.num = %d, want %d", envelope.Request.Num, tt.expectNum)
			}
			if got := quoteServer.OrderBookLastNum(); got != int32(tt.expectNum) {
				t.Errorf("OpenD order book num = %d, want %d", got, tt.expectNum)
			}
		})
	}
}

func TestMarketDepthSymbolCasing(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetOrderBook(
		[]fututestkit.OrderBookEntry{marketDataDepthOrderBookFixture(100, 10, 1)},
		[]fututestkit.OrderBookEntry{marketDataDepthOrderBookFixture(101, 10, 1)},
	)

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataDepthSubscription(t, harness, "US", "NVDA")
	response, err := harness.Adapters.DepthResponseForInstrument(
		t.Context(), "us", "nvda", DepthQuery{
			Num: httpserver.OptionalIntValue{Value: 5, Set: true, Valid: true},
		},
	)
	if err != nil {
		t.Fatalf("depth response: %v", err)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal depth response: %v", err)
	}

	var envelope struct {
		Request struct {
			Market       string `json:"market"`
			Symbol       string `json:"symbol"`
			InstrumentID string `json:"instrumentId"`
		} `json:"request"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if envelope.Request.Market != "US" {
		t.Errorf("market = %q, want US (upper-cased)", envelope.Request.Market)
	}
	if envelope.Request.Symbol != "NVDA" {
		t.Errorf("symbol = %q, want NVDA (upper-cased)", envelope.Request.Symbol)
	}
	if envelope.Request.InstrumentID != "US.NVDA" {
		t.Errorf("instrumentId = %q, want US.NVDA", envelope.Request.InstrumentID)
	}
}

func TestMarketDepthHKMarket(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetOrderBook(
		[]fututestkit.OrderBookEntry{
			marketDataDepthOrderBookFixture(320.0, 5000, 10),
			marketDataDepthOrderBookFixture(319.8, 3000, 8),
		},
		[]fututestkit.OrderBookEntry{
			marketDataDepthOrderBookFixture(320.2, 4000, 6),
			marketDataDepthOrderBookFixture(320.4, 2000, 3),
		},
	)

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataDepthSubscription(t, harness, "HK", "00700")
	response, err := harness.Adapters.DepthResponseForInstrument(
		t.Context(), "HK", "00700", DepthQuery{
			Num: httpserver.OptionalIntValue{Value: 5, Set: true, Valid: true},
		},
	)
	if err != nil {
		t.Fatalf("depth response: %v", err)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal depth response: %v", err)
	}

	var envelope struct {
		Request struct {
			Market       string `json:"market"`
			Symbol       string `json:"symbol"`
			InstrumentID string `json:"instrumentId"`
		} `json:"request"`
		Depth struct {
			Symbol string `json:"symbol"`
			Bids   []struct {
				Price      float64 `json:"price"`
				Volume     float64 `json:"volume"`
				OrderCount int32   `json:"orderCount"`
			} `json:"bids"`
			Asks []struct {
				Price      float64 `json:"price"`
				Volume     float64 `json:"volume"`
				OrderCount int32   `json:"orderCount"`
			} `json:"asks"`
		} `json:"depth"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if envelope.Request.InstrumentID != "HK.00700" {
		t.Errorf("instrumentId = %q, want HK.00700", envelope.Request.InstrumentID)
	}
	if envelope.Depth.Symbol != "HK.00700" {
		t.Errorf("depth symbol = %q, want HK.00700", envelope.Depth.Symbol)
	}
}

func TestMarketDepthEmptyOrderBook(t *testing.T) {
	quoteServer := fututestkit.StartQuoteServer(t)
	defer quoteServer.Close()

	quoteServer.SetOrderBook(nil, nil)

	harness := newMarketDataQuoteHarness(t, quoteServer.Addr())
	acquireMarketDataDepthSubscription(t, harness, "US", "AAPL")
	response, err := harness.Adapters.DepthResponseForInstrument(
		t.Context(), "US", "AAPL", DepthQuery{
			Num: httpserver.OptionalIntValue{Value: 10, Set: true, Valid: true},
		},
	)
	if err != nil {
		t.Fatalf("depth response: %v", err)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal depth response: %v", err)
	}

	var envelope struct {
		Depth struct {
			Bids []any `json:"bids"`
			Asks []any `json:"asks"`
		} `json:"depth"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(envelope.Depth.Bids) != 0 {
		t.Errorf("expected 0 bids, got %d", len(envelope.Depth.Bids))
	}
	if len(envelope.Depth.Asks) != 0 {
		t.Errorf("expected 0 asks, got %d", len(envelope.Depth.Asks))
	}
}
