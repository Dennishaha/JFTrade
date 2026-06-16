package futu

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
)

func TestMarketDataRuntimeCloseWaitsForEnsureAndDoesNotRevive(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var creates atomic.Int64
	runtime := NewMarketDataRuntime(MarketDataRuntimeOptions{
		ConfigSource: func() MarketDataConfig {
			return MarketDataConfig{Enabled: true, Host: "127.0.0.1", APIPort: 11110}
		},
		NewExchange: func(MarketDataConfig) *pkgfutu.Exchange {
			creates.Add(1)
			close(started)
			<-release
			return pkgfutu.NewExchange("127.0.0.1:1")
		},
	})

	ensureDone := make(chan *pkgfutu.Exchange, 1)
	go func() { ensureDone <- runtime.Ensure() }()
	<-started
	closeDone := make(chan struct{})
	go func() {
		_ = runtime.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned before in-flight Ensure completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if exchange := <-ensureDone; exchange != nil {
		t.Fatal("stale Ensure result revived runtime after Close")
	}
	<-closeDone

	if exchange := runtime.Ensure(); exchange != nil {
		t.Fatal("Ensure created exchange after Close")
	}
	if _, err := runtime.QueryTickers(context.Background(), []string{"HK.00700"}); err == nil {
		t.Fatal("QueryTickers succeeded after Close")
	}
	if _, err := runtime.NewStream([]string{"HK.00700"}, nil); err == nil {
		t.Fatal("NewStream succeeded after Close")
	}
	if got := creates.Load(); got != 1 {
		t.Fatalf("exchange creations = %d", got)
	}
}

func TestTickFromTradeProducesBrokerNeutralPushTick(t *testing.T) {
	at := time.Date(2026, time.June, 14, 1, 2, 3, 0, time.UTC)
	tick := tickFromTrade(bbgotypes.Trade{
		Symbol:   "hk.00700",
		Price:    fixedpointValue(t, "321.5"),
		Quantity: fixedpointValue(t, "200"),
		Time:     bbgotypes.Time(at),
	}, at.Add(time.Second))
	if tick == nil {
		t.Fatal("tickFromTrade returned nil")
	}
	if tick.InstrumentID != "HK.00700" || tick.Kind != marketdata.TickKindTrade ||
		tick.Source != "bbgo:futu:stream" || !tick.Price.Equal(decimal.RequireFromString("321.5")) {
		t.Fatalf("tick = %#v", tick)
	}
}

func TestTickFromTickerPreservesHKPreviousCloseDuringLunchBreak(t *testing.T) {
	cache := marketdata.NewCache()
	previousClose := decimal.RequireFromString("698.9")
	cache.Seed(marketdata.Tick{
		InstrumentID:       "HK.00700",
		Market:             "HK",
		Symbol:             "00700",
		Price:              decimal.RequireFromString("700.1"),
		PreviousClosePrice: &previousClose,
		LastClosePrice:     new(decimal.RequireFromString("698.1")),
		QuoteAt:            time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		ObservedAt:         time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		Source:             "bbgo:futu",
		Session:            "unknown",
		ExtendedHours:      false,
	})

	hkt := time.FixedZone("HKT", 8*60*60)
	lunchAt := time.Date(2026, time.June, 12, 12, 30, 0, 0, hkt).UTC()
	tick := tickFromTicker("HK.00700", &bbgotypes.Ticker{
		Time:   lunchAt,
		Last:   fixedpointValue(t, "701.1"),
		Buy:    fixedpointValue(t, "701.0"),
		Sell:   fixedpointValue(t, "701.2"),
		Volume: fixedpointValue(t, "22222"),
	}, lunchAt)
	if tick == nil {
		t.Fatal("tickFromTicker returned nil")
	}
	stored := cache.Store(*tick)
	if stored == nil {
		t.Fatal("cache.Store returned nil")
	}
	if stored.PreviousClosePrice == nil || !stored.PreviousClosePrice.Equal(previousClose) {
		t.Fatalf("PreviousClosePrice = %#v, want %s", stored.PreviousClosePrice, previousClose)
	}
	if stored.PreviousClosePrice.Equal(stored.Price) {
		t.Fatalf("PreviousClosePrice was overwritten by current price %s", stored.Price)
	}
	if stored.ExtendedHours {
		t.Fatal("HK lunch tick should not be marked extended hours")
	}
}

func TestTickFromTradeInheritsLatestQuoteFieldsThroughCache(t *testing.T) {
	cache := marketdata.NewCache()
	openPrice := decimal.RequireFromString("698.0")
	highPrice := decimal.RequireFromString("705.0")
	lowPrice := decimal.RequireFromString("697.2")
	previousClose := decimal.RequireFromString("697.0")
	lastClose := decimal.RequireFromString("696.8")
	cache.Seed(marketdata.Tick{
		InstrumentID:       "HK.00700",
		Market:             "HK",
		Symbol:             "00700",
		Price:              decimal.RequireFromString("700.1"),
		Bid:                decimal.RequireFromString("700.0"),
		Ask:                decimal.RequireFromString("700.2"),
		OpenPrice:          &openPrice,
		HighPrice:          &highPrice,
		LowPrice:           &lowPrice,
		PreviousClosePrice: &previousClose,
		LastClosePrice:     &lastClose,
		Volume:             43210,
		Turnover:           decimal.RequireFromString("1234567.8"),
		QuoteAt:            time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		ObservedAt:         time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		Source:             "bbgo:futu",
		Session:            "regular",
	})

	hkt := time.FixedZone("HKT", 8*60*60)
	at := time.Date(2026, time.June, 12, 10, 1, 0, 0, hkt).UTC()
	tick := tickFromTrade(bbgotypes.Trade{
		Symbol:   "HK.00700",
		Price:    fixedpointValue(t, "702.3"),
		Quantity: fixedpointValue(t, "0"),
		Time:     bbgotypes.Time(at),
	}, at)
	if tick == nil {
		t.Fatal("tickFromTrade returned nil")
	}
	stored := cache.Store(*tick)
	if stored == nil {
		t.Fatal("cache.Store returned nil")
	}
	if !stored.Bid.Equal(decimal.RequireFromString("700.0")) || !stored.Ask.Equal(decimal.RequireFromString("700.2")) {
		t.Fatalf("bid/ask = %s/%s", stored.Bid, stored.Ask)
	}
	if stored.OpenPrice == nil || !stored.OpenPrice.Equal(openPrice) {
		t.Fatalf("OpenPrice = %#v", stored.OpenPrice)
	}
	if stored.HighPrice == nil || !stored.HighPrice.Equal(highPrice) {
		t.Fatalf("HighPrice = %#v", stored.HighPrice)
	}
	if stored.LowPrice == nil || !stored.LowPrice.Equal(lowPrice) {
		t.Fatalf("LowPrice = %#v", stored.LowPrice)
	}
	if stored.PreviousClosePrice == nil || !stored.PreviousClosePrice.Equal(previousClose) {
		t.Fatalf("PreviousClosePrice = %#v", stored.PreviousClosePrice)
	}
	if stored.LastClosePrice == nil || !stored.LastClosePrice.Equal(lastClose) {
		t.Fatalf("LastClosePrice = %#v", stored.LastClosePrice)
	}
	if !stored.Turnover.Equal(decimal.RequireFromString("1234567.8")) || stored.Volume != 43210 {
		t.Fatalf("turnover/volume = %s/%v", stored.Turnover, stored.Volume)
	}
}

func TestTickFromTickerReclassifiesUSRegularBoundary(t *testing.T) {
	regularClock := time.Date(2026, time.January, 7, 16, 0, 0, 0, time.UTC)
	tick := tickFromTicker("US.AAPL", &bbgotypes.Ticker{
		Time: regularClock,
		Last: fixedpointValue(t, "189.5"),
		Buy:  fixedpointValue(t, "189.4"),
		Sell: fixedpointValue(t, "189.6"),
	}, regularClock)
	if tick == nil {
		t.Fatal("tickFromTicker returned nil")
	}
	if tick.Session != "regular" {
		t.Fatalf("session = %s", tick.Session)
	}
	if tick.ExtendedHours {
		t.Fatal("regular US tick should clear extended hours")
	}
}

func fixedpointValue(t *testing.T, value string) fixedpoint.Value {
	t.Helper()
	result, err := fixedpoint.NewFromString(value)
	if err != nil {
		t.Fatalf("fixedpoint.NewFromString: %v", err)
	}
	return result
}
