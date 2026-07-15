package servercore

import (
	"errors"
	"math"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestPineWorkerLiveRemainingConstructorAndWarmupErrors(t *testing.T) {
	worker := newFakeStrategyRuntimePineWorker()
	runtime := &strategySymbolRuntime{symbol: "US.AAPL", market: bbgotypes.Market{Symbol: "US.AAPL"}}
	executor := &strategyNotifyOnlyOrderExecutor{runner: runtime}
	if _, err := newStrategyRuntimePineWorkerLive(worker, managedStrategyInstance{}, "US.AAPL", bbgotypes.Interval1m, " ", executor, runtime, nil); err == nil {
		t.Fatal("blank live source error = nil")
	}
	if _, err := newStrategyRuntimePineWorkerLive(worker, managedStrategyInstance{}, "US.AAPL", bbgotypes.Interval1m, "strategy(\"x\")", nil, runtime, nil); err == nil {
		t.Fatal("nil live executor error = nil")
	}
	if _, err := newStrategyRuntimePineWorkerLive(worker, managedStrategyInstance{}, "US.AAPL", bbgotypes.Interval1m, "strategy(\"x\")", executor, nil, nil); err == nil {
		t.Fatal("nil symbol runtime error = nil")
	}

	live := &strategyRuntimePineWorkerLive{source: "not pine", interval: bbgotypes.Interval1m, symbol: "US.AAPL"}
	if _, err := live.loadWarmup(t.Context(), newStrategyRuntimeStubExchange()); err == nil {
		t.Fatal("invalid warmup source error = nil")
	}
	stub := newStrategyRuntimeStubExchange()
	stub.queryKLinesErr = errors.New("klines failed")
	live.source = "//@version=6\nstrategy(\"Coverage\")"
	if _, err := live.loadWarmup(t.Context(), stub); err == nil {
		t.Fatal("warmup query error = nil")
	}

	worker.err = errors.New("worker failed")
	live, err := newStrategyRuntimePineWorkerLive(worker, managedStrategyInstance{ID: "instance"}, "US.AAPL", bbgotypes.Interval1m, "//@version=6\nstrategy(\"Coverage\")", executor, runtime, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := live.onClosedKLine(t.Context(), strategyRuntimeHistoricalKLine("US.AAPL", "1m", 100, strategyRuntimeTestTime(10, 0, 0))); err == nil {
		t.Fatal("live worker execution error = nil")
	}
}

func TestPineWorkerLiveRemainingMarketAndSizerBoundaries(t *testing.T) {
	market := bbgotypes.Market{Symbol: "US.AAPL", QuoteCurrency: "USD", TickSize: fixedpoint.NewFromFloat(0.05)}
	resolver := strategyRuntimeLiveMarketResolver{market: market}
	if _, ok := resolver.Market("US.MSFT"); ok {
		t.Fatal("mismatched live market resolved")
	}

	var nilSizer *strategyRuntimeLiveSizer
	nilSizer.onKLineClosed(bbgotypes.KLine{})
	if _, err := nilSizer.QuantityForCommand(bt.WorkerOrderCommand{ID: "nil", QuantityPct: 10}, market); err == nil {
		t.Fatal("nil sizer quantity error = nil")
	}
	if nilSizer.NetPosition().Sign() != 0 {
		t.Fatal("nil sizer position was nonzero")
	}

	account := bbgotypes.NewAccount()
	runner := &strategySymbolRuntime{
		symbol: "US.AAPL", session: &bbgo.ExchangeSession{Account: account},
		cachedPositions: []broker.PositionSnapshot{{Market: "US", Symbol: "AAPL", Quantity: -4}},
	}
	sizer := &strategyRuntimeLiveSizer{runner: runner}
	sizer.onKLineClosed(bbgotypes.KLine{Symbol: "US.MSFT", Close: fixedpoint.NewFromFloat(200)})
	if sizer.lastPrice.Sign() != 0 {
		t.Fatal("mismatched kline changed last price")
	}
	for _, pct := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := sizer.QuantityForCommand(bt.WorkerOrderCommand{ID: "invalid", Kind: "entry", QuantityPct: pct}, market); err == nil {
			t.Fatalf("invalid quantity pct %v error = nil", pct)
		}
	}
	if _, err := sizer.QuantityForCommand(bt.WorkerOrderCommand{ID: "unsupported", Kind: "cancel", QuantityPct: 10}, market); err == nil {
		t.Fatal("unsupported quantity kind error = nil")
	}
	if got := sizer.NetPosition(); got.Float64() != -4 {
		t.Fatalf("net position = %v", got)
	}

	entry := bt.WorkerOrderCommand{ID: "entry", Kind: "entry", QuantityPct: 10}
	if _, err := sizer.QuantityForCommand(entry, market); err == nil {
		t.Fatal("entry without price error = nil")
	}
	sizer.lastPrice = fixedpoint.NewFromFloat(100)
	if _, err := sizer.QuantityForCommand(entry, market); err == nil {
		t.Fatal("entry without equity error = nil")
	}
	account.TotalAccountValue = fixedpoint.NewFromFloat(1000)
	quantity, err := sizer.QuantityForCommand(entry, market)
	if err != nil || quantity.Float64() != 1 {
		t.Fatalf("entry quantity = %v, %v", quantity, err)
	}

	closeCommand := bt.WorkerOrderCommand{ID: "close", Kind: "close", QuantityPct: 200}
	quantity, err = sizer.QuantityForCommand(closeCommand, market)
	if err != nil || quantity.Float64() != 4 {
		t.Fatalf("capped close quantity = %v, %v", quantity, err)
	}
	runner.cachedPositions = nil
	if _, err := sizer.QuantityForCommand(closeCommand, market); err == nil {
		t.Fatal("close without position error = nil")
	}
}

func TestPineWorkerLiveRemainingEquityPriceAndParamBoundaries(t *testing.T) {
	runner := &strategySymbolRuntime{symbol: "US.AAPL", session: &bbgo.ExchangeSession{}}
	sizer := &strategyRuntimeLiveSizer{runner: runner}
	if _, err := sizer.equity(bbgotypes.Market{QuoteCurrency: "USD"}); err == nil {
		t.Fatal("nil account equity error = nil")
	}
	runner.session.Account = bbgotypes.NewAccount()
	if _, err := sizer.equity(bbgotypes.Market{}); err == nil {
		t.Fatal("blank quote currency error = nil")
	}
	runner.session.Account.SetBalance("USD", bbgotypes.Balance{Available: fixedpoint.NewFromFloat(25), Locked: fixedpoint.NewFromFloat(5)})
	if equity, err := sizer.equity(bbgotypes.Market{QuoteCurrency: "USD"}); err != nil || equity.Float64() != 30 {
		t.Fatalf("balance equity = %v, %v", equity, err)
	}

	market := bbgotypes.Market{TickSize: fixedpoint.NewFromFloat(0.1)}
	if price := sizer.priceForCommand(bt.WorkerOrderCommand{LimitPrice: 10.12}, market); price.Float64() <= 10 {
		t.Fatalf("limit command price = %v", price)
	}
	if price := sizer.priceForCommand(bt.WorkerOrderCommand{StopPrice: 11.2}, market); price.Float64() != 11.2 {
		t.Fatalf("stop command price = %v", price)
	}
	runner.currentBucket = &bbgotypes.KLine{Close: fixedpoint.NewFromFloat(10.19)}
	sizer.lastPrice = fixedpoint.Zero
	if price := sizer.priceForCommand(bt.WorkerOrderCommand{}, market); price.Float64() != 10.1 {
		t.Fatalf("truncated current price = %v", price)
	}

	params := strategyRuntimePineWorkerParams(managedStrategyInstance{Params: map[string]any{
		"duration": time.Second, "unsupported": []string{"x"},
	}})
	if params["duration"] != "1s" {
		t.Fatalf("worker params = %#v", params)
	}
}
