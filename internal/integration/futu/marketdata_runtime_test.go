package futu

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	"github.com/jftrade/jftrade-main/pkg/market"
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
	waitingEnsure := make(chan *pkgfutu.Exchange, 1)
	go func() { waitingEnsure <- runtime.Ensure() }()
	time.Sleep(10 * time.Millisecond)
	closeDone := make(chan struct{})
	go func() {
		func() {
			jftradeErr1 := runtime.Close()
			jftradeCheckTestError(t, jftradeErr1)
			close(closeDone)
		}()
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
	if exchange := <-waitingEnsure; exchange != nil {
		t.Fatal("waiting Ensure result revived runtime after Close")
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

func TestMarketDataRuntimeDoesNotPublishExchangeWhenConfigChangesDuringCreate(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var enabled atomic.Bool
	enabled.Store(true)
	runtime := NewMarketDataRuntime(MarketDataRuntimeOptions{
		ConfigSource: func() MarketDataConfig {
			if !enabled.Load() {
				return MarketDataConfig{}
			}
			return MarketDataConfig{Enabled: true, Host: "127.0.0.1", APIPort: 11110}
		},
		NewExchange: func(MarketDataConfig) *pkgfutu.Exchange {
			close(started)
			<-release
			return pkgfutu.NewExchange("127.0.0.1:1")
		},
	})

	result := make(chan *pkgfutu.Exchange, 1)
	go func() { result <- runtime.Ensure() }()
	<-started
	enabled.Store(false)
	close(release)
	if exchange := <-result; exchange != nil {
		t.Fatalf("Ensure() = %#v after config was disabled, want nil", exchange)
	}
	if exchange := runtime.Exchange(); exchange != nil {
		t.Fatalf("Exchange() = %#v after disabled config, want nil", exchange)
	}
	jftradeCheckTestError(t, runtime.Close())
}

func TestMarketDataRuntimeNilAndClosedLifecycleBoundaries(t *testing.T) {
	var nilRuntime *MarketDataRuntime
	nilRuntime.Reset()
	if err := nilRuntime.Close(); err != nil {
		t.Fatalf("nil Close() = %v", err)
	}
	if exchange := nilRuntime.Ensure(); exchange != nil {
		t.Fatalf("nil Ensure() = %#v", exchange)
	}

	runtime := NewMarketDataRuntime(MarketDataRuntimeOptions{})
	if exchange := runtime.Ensure(); exchange != nil {
		t.Fatalf("Ensure() without config source = %#v", exchange)
	}
	jftradeCheckTestError(t, runtime.Close())
	runtime.Reset()
	if err := runtime.Close(); err != nil {
		t.Fatalf("closed Close() = %v", err)
	}
}

func TestTickFromTradeProducesBrokerNeutralPushTick(t *testing.T) {
	at := time.Date(2026, time.June, 14, 1, 2, 3, 0, time.UTC)
	cumulativeVolume := fixedpointValue(t, "1200")
	tick := tickFromTrade(bbgotypes.Trade{
		Symbol:           "hk.00700",
		Price:            fixedpointValue(t, "321.5"),
		Quantity:         fixedpointValue(t, "200"),
		CumulativeVolume: &cumulativeVolume,
		Time:             bbgotypes.Time(at),
	}, at.Add(time.Second))
	if tick == nil {
		t.Fatal("tickFromTrade returned nil")
		return
	}
	if tick.InstrumentID != "HK.00700" || tick.Kind != marketdata.TickKindTrade ||
		tick.Source != "bbgo:futu:stream" || !tick.Price.Equal(decimal.RequireFromString("321.5")) {
		t.Fatalf("tick = %#v", tick)
	}
	if tick.Volume != 1200 || tick.VolumeDelta != 200 {
		t.Fatalf("tick volume contract = cumulative:%v delta:%v", tick.Volume, tick.VolumeDelta)
	}

	withoutCounter := tickFromTrade(bbgotypes.Trade{
		Symbol:   "HK.00700",
		Price:    fixedpointValue(t, "321.6"),
		Quantity: fixedpointValue(t, "5"),
		Time:     bbgotypes.Time(at),
	}, at.Add(time.Second))
	if withoutCounter == nil || withoutCounter.Volume != 0 || withoutCounter.VolumeDelta != 5 {
		t.Fatalf("optional cumulative volume contract = %#v", withoutCounter)
	}
}

func TestTickConversionRejectsUnusablePricesAndUsesQuoteFallbacks(t *testing.T) {
	at := time.Date(2026, time.June, 14, 1, 2, 3, 0, time.UTC)
	if got := tickFromTicker("HK.00700", nil, at); got != nil {
		t.Fatalf("tickFromTicker(nil) = %#v", got)
	}
	if got := tickFromTicker("HK.00700", &bbgotypes.Ticker{}, at); got != nil {
		t.Fatalf("tickFromTicker(zero price) = %#v", got)
	}
	if got := tickFromTicker("bad-symbol", &bbgotypes.Ticker{Last: fixedpointValue(t, "1")}, at); got != nil {
		t.Fatalf("tickFromTicker(invalid symbol) = %#v", got)
	}

	tick := tickFromTicker("US.MSFT", &bbgotypes.Ticker{
		Buy:    fixedpointValue(t, "401.10"),
		Sell:   fixedpointValue(t, "401.20"),
		Open:   fixedpointValue(t, "399.00"),
		High:   fixedpointValue(t, "402.00"),
		Low:    fixedpointValue(t, "398.50"),
		Volume: fixedpointValue(t, "900"),
	}, at)
	if tick == nil {
		t.Fatal("tickFromTicker should use bid/ask as fallback when last is missing")
		return
	}
	if !tick.Price.Equal(decimal.RequireFromString("401.10")) ||
		!tick.Bid.Equal(decimal.RequireFromString("401.10")) ||
		!tick.Ask.Equal(decimal.RequireFromString("401.20")) {
		t.Fatalf("fallback price/bid/ask = %s/%s/%s", tick.Price, tick.Bid, tick.Ask)
	}
	if tick.OpenPrice == nil || tick.HighPrice == nil || tick.LowPrice == nil {
		t.Fatalf("quote range fields were not preserved: %#v", tick)
	}

	if got := tickFromTrade(bbgotypes.Trade{Symbol: "US.MSFT", Price: fixedpointValue(t, "0")}, at); got != nil {
		t.Fatalf("tickFromTrade(zero price) = %#v", got)
	}
	if got := tickFromTrade(bbgotypes.Trade{Symbol: "bad", Price: fixedpointValue(t, "1")}, at); got != nil {
		t.Fatalf("tickFromTrade(invalid symbol) = %#v", got)
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
		return
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
	at := time.Now().UTC().Truncate(time.Second)
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
		QuoteAt:            at.Format(time.RFC3339Nano),
		ObservedAt:         at.Format(time.RFC3339Nano),
		Source:             "bbgo:futu",
		Session:            "regular",
	})

	cumulativeVolume := fixedpointValue(t, "43210")
	tick := tickFromTrade(bbgotypes.Trade{
		Symbol:           "HK.00700",
		Price:            fixedpointValue(t, "702.3"),
		Quantity:         fixedpointValue(t, "0"),
		CumulativeVolume: &cumulativeVolume,
		Time:             bbgotypes.Time(at),
	}, at)
	if tick == nil {
		t.Fatal("tickFromTrade returned nil")
	}
	stored := cache.Store(*tick)
	if stored == nil {
		t.Fatal("cache.Store returned nil")
		return
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
	if !stored.Turnover.Equal(decimal.RequireFromString("1234567.8")) || stored.Volume != 43210 || stored.VolumeDelta != 0 {
		t.Fatalf("turnover/volume/delta = %s/%v/%v", stored.Turnover, stored.Volume, stored.VolumeDelta)
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
		return
	}
	if tick.Session != "regular" {
		t.Fatalf("session = %s", tick.Session)
	}
	if tick.ExtendedHours {
		t.Fatal("regular US tick should clear extended hours")
	}
}

func TestMarketDataRuntimeExchangeResetAndStreamLifecycle(t *testing.T) {
	var created atomic.Int64
	var onExchangeCalls atomic.Int64
	runtime := NewMarketDataRuntime(MarketDataRuntimeOptions{
		ConfigSource: func() MarketDataConfig {
			return MarketDataConfig{Enabled: true, Host: "127.0.0.1", APIPort: 11110, WebSocketKey: "secret"}
		},
		NewExchange: func(config MarketDataConfig) *pkgfutu.Exchange {
			created.Add(1)
			return pkgfutu.NewExchangeWithConfig(opend.Config{
				Addr:         "127.0.0.1:1",
				WebSocketKey: config.WebSocketKey,
			})
		},
		OnExchange: func(*pkgfutu.Exchange) {
			onExchangeCalls.Add(1)
		},
		Now: func() time.Time {
			return time.Date(2026, time.June, 23, 1, 2, 3, 0, time.UTC)
		},
	})

	first := runtime.Exchange()
	if first == nil {
		t.Fatal("Exchange() = nil")
	}
	if runtime.Ensure() != first {
		t.Fatal("Ensure() did not reuse exchange from Exchange()")
	}
	if created.Load() != 1 || onExchangeCalls.Load() != 1 {
		t.Fatalf("creations/onExchange = %d/%d", created.Load(), onExchangeCalls.Load())
	}

	stream, err := runtime.NewStream([]string{"HK.00700", "US.AAPL"}, nil)
	if err != nil {
		t.Fatalf("NewStream(nil handler): %v", err)
	}
	futuStream, ok := stream.(*pkgfutu.Stream)
	if !ok {
		t.Fatalf("stream type = %T, want *pkgfutu.Stream", stream)
	}
	if !futuStream.GetPublicOnly() {
		t.Fatal("stream should be public-only")
	}
	subs := futuStream.GetSubscriptions()
	if len(subs) != 2 {
		t.Fatalf("subscriptions = %#v", subs)
	}
	if subs[0].Symbol != "HK.00700" || subs[1].Symbol != "US.AAPL" {
		t.Fatalf("subscription symbols = %#v", subs)
	}
	futuStream.EmitMarketTrade(bbgotypes.Trade{
		Symbol: "HK.00700",
		Price:  fixedpointValue(t, "1"),
	})

	pushTicks := make(chan marketdata.Tick, 1)
	stream, err = runtime.NewStream([]string{"HK.00700"}, func(tick marketdata.Tick) {
		pushTicks <- tick
	})
	if err != nil {
		t.Fatalf("NewStream(handler): %v", err)
	}
	futuStream = stream.(*pkgfutu.Stream)
	tradeAt := time.Date(2026, time.June, 23, 9, 31, 0, 0, time.UTC)
	cumulativeVolume := fixedpointValue(t, "50120")
	futuStream.EmitMarketTrade(bbgotypes.Trade{
		Symbol:           "HK.00700",
		Price:            fixedpointValue(t, "323.5"),
		Quantity:         fixedpointValue(t, "120"),
		CumulativeVolume: &cumulativeVolume,
		Time:             bbgotypes.Time(tradeAt),
	})
	select {
	case tick := <-pushTicks:
		if tick.InstrumentID != "HK.00700" || tick.Kind != marketdata.TickKindTrade || !tick.Price.Equal(decimal.RequireFromString("323.5")) || tick.Volume != 50120 || tick.VolumeDelta != 120 {
			t.Fatalf("push tick = %#v", tick)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for push tick")
	}

	runtime.Reset()
	second := runtime.Exchange()
	if second == nil {
		t.Fatal("Exchange() after Reset = nil")
	}
	if second == first {
		t.Fatal("Reset() should force a fresh exchange")
	}
	if created.Load() != 2 || onExchangeCalls.Load() != 2 {
		t.Fatalf("creations/onExchange after reset = %d/%d", created.Load(), onExchangeCalls.Load())
	}
}

func TestMarketDataRuntimeReplacesAnExchangeWhenItsConfigKeyChanges(t *testing.T) {
	var port atomic.Int64
	port.Store(11110)
	runtime := NewMarketDataRuntime(MarketDataRuntimeOptions{
		ConfigSource: func() MarketDataConfig {
			return MarketDataConfig{Enabled: true, Host: "127.0.0.1", APIPort: int(port.Load())}
		},
		NewExchange: func(config MarketDataConfig) *pkgfutu.Exchange {
			return pkgfutu.NewExchange("127.0.0.1:" + fmt.Sprint(config.APIPort))
		},
	})
	first := runtime.Ensure()
	port.Store(11111)
	second := runtime.Ensure()
	if first == nil || second == nil || first == second {
		t.Fatalf("exchange replacement = %p -> %p", first, second)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
}

func TestMarketDataRuntimeUnavailableQueryHelpers(t *testing.T) {
	runtime := NewMarketDataRuntime(MarketDataRuntimeOptions{
		ConfigSource: func() MarketDataConfig { return MarketDataConfig{} },
	})

	if exchange := runtime.Exchange(); exchange != nil {
		t.Fatalf("Exchange() = %#v, want nil when config disabled", exchange)
	}
	if _, err := runtime.QueryTicker(context.Background(), "HK.00700"); err == nil {
		t.Fatal("QueryTicker() error = nil")
	}
	if _, err := runtime.QuerySnapshot(context.Background(), "HK.00700"); err == nil {
		t.Fatal("QuerySnapshot() error = nil")
	}
	if _, err := runtime.NewStream([]string{"HK.00700"}, func(marketdata.Tick) {}); err == nil {
		t.Fatal("NewStream() error = nil when config disabled")
	}
	if err := runtime.ReconcileSubscriptions(context.Background(), []marketdata.InstrumentRef{{Market: "US", Symbol: "AAPL"}}); err == nil {
		t.Fatal("ReconcileSubscriptions() error = nil when config disabled")
	}
	besteffort.LogError(errors.New("expected best-effort test error"))
}

func TestTickFromSnapshotMapsExtendedQuoteFields(t *testing.T) {
	quoteAt := time.Date(2026, time.June, 23, 20, 15, 0, 0, time.UTC)
	prePrice := decimal.RequireFromString("320.1")
	preHigh := decimal.RequireFromString("321.8")
	preLow := decimal.RequireFromString("319.5")
	preTurnover := decimal.RequireFromString("12345.6")
	preChangeVal := decimal.RequireFromString("2.3")
	preChangeRate := decimal.RequireFromString("0.72")
	preAmplitude := decimal.RequireFromString("1.1")
	preVolume := 4567.0

	afterPrice := decimal.RequireFromString("322.4")
	overnightPrice := decimal.RequireFromString("323.7")
	openPrice := decimal.RequireFromString("318.0")
	highPrice := decimal.RequireFromString("322.0")
	lowPrice := decimal.RequireFromString("317.8")
	previousClose := decimal.RequireFromString("316.4")
	lastClose := decimal.RequireFromString("316.2")
	snapshot := &pkgfutu.QuoteSnapshot{
		Symbol:             "US.AAPL",
		Price:              decimal.RequireFromString("321.2"),
		Bid:                decimal.RequireFromString("321.1"),
		Ask:                decimal.RequireFromString("321.3"),
		OpenPrice:          &openPrice,
		HighPrice:          &highPrice,
		LowPrice:           &lowPrice,
		PreviousClosePrice: &previousClose,
		LastClosePrice:     &lastClose,
		Volume:             6789,
		Turnover:           decimal.RequireFromString("999999.9"),
		QuoteAt:            quoteAt,
		Session:            market.SessionAfter,
		ExtendedHours:      true,
		PreMarket: &pkgfutu.ExtendedMarketQuote{
			Price:      &prePrice,
			HighPrice:  &preHigh,
			LowPrice:   &preLow,
			Volume:     &preVolume,
			Turnover:   &preTurnover,
			ChangeVal:  &preChangeVal,
			ChangeRate: &preChangeRate,
			Amplitude:  &preAmplitude,
			QuoteTime:  "2026-06-23T08:15:00Z",
		},
		AfterMarket: &pkgfutu.ExtendedMarketQuote{
			Price:     &afterPrice,
			QuoteTime: "2026-06-23T20:15:00Z",
		},
		Overnight: &pkgfutu.ExtendedMarketQuote{
			Price:     &overnightPrice,
			QuoteTime: "2026-06-24T01:15:00Z",
		},
	}

	observedAt := quoteAt.Add(time.Second)
	tick := tickFromSnapshot("US.AAPL", snapshot, observedAt)
	if tick == nil {
		t.Fatal("tickFromSnapshot() = nil")
		return
	}
	if tick.InstrumentID != "US.AAPL" || tick.Market != "US" || tick.Symbol != "AAPL" {
		t.Fatalf("tick identity = %#v", tick)
	}
	if tick.Kind != marketdata.TickKindQuote || tick.Source != "bbgo:futu" || !tick.ExtendedHours {
		t.Fatalf("tick kind/source/session = %#v", tick)
	}
	if tick.Session != string(market.SessionAfter) {
		t.Fatalf("tick session = %s", tick.Session)
	}
	if tick.PreMarket == nil || tick.PreMarket.Price == nil || !tick.PreMarket.Price.Equal(prePrice) {
		t.Fatalf("tick.PreMarket = %#v", tick.PreMarket)
	}
	if tick.AfterMarket == nil || tick.AfterMarket.Price == nil || !tick.AfterMarket.Price.Equal(afterPrice) {
		t.Fatalf("tick.AfterMarket = %#v", tick.AfterMarket)
	}
	if tick.Overnight == nil || tick.Overnight.Price == nil || !tick.Overnight.Price.Equal(overnightPrice) {
		t.Fatalf("tick.Overnight = %#v", tick.Overnight)
	}
	if tick.QuoteAt != quoteAt.Format(time.RFC3339Nano) || tick.ObservedAt != observedAt.Format(time.RFC3339Nano) {
		t.Fatalf("tick times = quote:%s observed:%s", tick.QuoteAt, tick.ObservedAt)
	}

	if got := tickFromSnapshot("US.AAPL", &pkgfutu.QuoteSnapshot{}, observedAt); got != nil {
		t.Fatalf("tickFromSnapshot(zero price) = %#v", got)
	}
	if got := tickFromSnapshot("bad-symbol", snapshot, observedAt); got != nil {
		t.Fatalf("tickFromSnapshot(invalid symbol) = %#v", got)
	}
	if got := extendedQuote(nil); got != nil {
		t.Fatalf("extendedQuote(nil) = %#v", got)
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

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
