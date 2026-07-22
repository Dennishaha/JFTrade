package servercore

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestPineWorkerLiveUsesStatefulSessionAfterWarmup(t *testing.T) {
	runner := &fakeStatefulPineWorkerRunner{session: &fakeStatefulPineWorkerSession{}}
	symbolRuntime := &strategySymbolRuntime{
		symbol: "US.AAPL",
		market: bbgotypes.Market{Symbol: "US.AAPL", StepSize: fixedpoint.One, MinQuantity: fixedpoint.One},
	}
	executor := &strategyNotifyOnlyOrderExecutor{runner: symbolRuntime}
	live, err := newStrategyRuntimePineWorkerLive(
		runner,
		managedStrategyInstance{ID: "stateful-instance"},
		"US.AAPL",
		bbgotypes.Interval1m,
		"//@version=6\nstrategy(\"Stateful\")",
		executor,
		symbolRuntime,
		nil,
	)
	if err != nil {
		t.Fatalf("newStrategyRuntimePineWorkerLive: %v", err)
	}
	live.recordWarmupClosed(strategyRuntimeHistoricalKLine("US.AAPL", "1m", 100, strategyRuntimeTestTime(9, 58, 0)))
	live.recordWarmupClosed(strategyRuntimeHistoricalKLine("US.AAPL", "1m", 101, strategyRuntimeTestTime(9, 59, 0)))
	if err := live.openSession(t.Context()); err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if len(runner.openRequest.Candles) != 2 || runner.openRequest.SessionID != "strategy:stateful-instance:US.AAPL" {
		t.Fatalf("open request = %#v", runner.openRequest)
	}
	closed := strategyRuntimeHistoricalKLine("US.AAPL", "1m", 102, strategyRuntimeTestTime(10, 0, 0))
	if err := live.onClosedKLine(t.Context(), closed); err != nil {
		t.Fatalf("onClosedKLine: %v", err)
	}
	if runner.runCalls != 0 {
		t.Fatalf("full-history RunScript calls = %d, want 0", runner.runCalls)
	}
	if len(runner.session.appendRequests) != 1 || len(runner.session.appendRequests[0].Candles) != 1 {
		t.Fatalf("incremental append requests = %#v", runner.session.appendRequests)
	}
	if runner.session.appendRequests[0].Candles[0].OpenTime != bt.CandleFromKLine(closed).OpenTime {
		t.Fatalf("incremental candle = %#v", runner.session.appendRequests[0].Candles)
	}
	if err := live.closeSession(t.Context()); err != nil || runner.session.closeCalls != 1 {
		t.Fatalf("closeSession err=%v calls=%d", err, runner.session.closeCalls)
	}
}

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

type fakeStatefulPineWorkerRunner struct {
	openRequest pineworker.RunScriptRequest
	session     *fakeStatefulPineWorkerSession
	runCalls    int
}

func (runner *fakeStatefulPineWorkerRunner) RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	runner.runCalls++
	return pineworker.RunScriptResponse{}, nil
}

func (runner *fakeStatefulPineWorkerRunner) OpenLiveSession(
	_ context.Context,
	request pineworker.RunScriptRequest,
) (pineWorkerLiveSession, pineworker.RunScriptResponse, error) {
	runner.openRequest = request
	return runner.session, pineworker.RunScriptResponse{SessionID: request.SessionID, SessionRevision: 1}, nil
}

type fakeStatefulPineWorkerSession struct {
	appendRequests []pineworker.RunScriptRequest
	closeCalls     int
}

func (session *fakeStatefulPineWorkerSession) Append(
	_ context.Context,
	request pineworker.RunScriptRequest,
) (pineworker.RunScriptResponse, error) {
	session.appendRequests = append(session.appendRequests, request)
	return pineworker.RunScriptResponse{SessionRevision: uint64(len(session.appendRequests) + 1)}, nil
}

func (session *fakeStatefulPineWorkerSession) Close(context.Context) error {
	session.closeCalls++
	return nil
}
