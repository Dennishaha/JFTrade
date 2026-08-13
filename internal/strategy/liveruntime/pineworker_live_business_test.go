package liveruntime

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/pineruntime"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type pineWorkerRunner = pineruntime.Runner
type pineWorkerLiveSession = pineruntime.LiveSession

type fakeStrategyRuntimePineWorker struct {
	err error
}

func newFakeStrategyRuntimePineWorker() *fakeStrategyRuntimePineWorker {
	return &fakeStrategyRuntimePineWorker{}
}

func (worker *fakeStrategyRuntimePineWorker) RunScript(
	_ context.Context,
	request pineworker.RunScriptRequest,
) (pineworker.RunScriptResponse, error) {
	if worker.err != nil {
		return pineworker.RunScriptResponse{}, worker.err
	}
	return pineworker.RunScriptResponse{JobID: request.JobID}, nil
}

type strategyRuntimeStubExchange struct {
	markets           bbgotypes.MarketMap
	history           map[string][]bbgotypes.KLine
	funds             *broker.FundsSnapshot
	positions         []broker.PositionSnapshot
	queryMarketsErr   error
	queryKLinesErr    error
	queryFundsErr     error
	queryPositionsErr error
	stream            bbgotypes.Stream
	nilStream         bool
}

func newStrategyRuntimeStubExchange() *strategyRuntimeStubExchange {
	availableFunds := 10_000.0
	return &strategyRuntimeStubExchange{
		markets: bbgotypes.MarketMap{
			"US.AAPL": {
				Symbol:        "US.AAPL",
				BaseCurrency:  "AAPL",
				QuoteCurrency: "USD",
			},
		},
		history: map[string][]bbgotypes.KLine{},
		funds: &broker.FundsSnapshot{
			Currency:       new("USD"),
			AvailableFunds: &availableFunds,
		},
	}
}

func (e *strategyRuntimeStubExchange) Name() bbgotypes.ExchangeName {
	return bbgotypes.ExchangeFutu
}

func (e *strategyRuntimeStubExchange) PlatformFeeCurrency() string {
	return "USD"
}

func (e *strategyRuntimeStubExchange) NewStream() bbgotypes.Stream {
	if e.nilStream {
		return nil
	}
	if e.stream != nil {
		return e.stream
	}
	stream := bbgotypes.NewStandardStream()
	return &stream
}

func (e *strategyRuntimeStubExchange) QueryMarkets(context.Context) (bbgotypes.MarketMap, error) {
	if e.queryMarketsErr != nil {
		return nil, e.queryMarketsErr
	}
	return e.markets, nil
}

func (e *strategyRuntimeStubExchange) QueryTicker(
	context.Context,
	string,
) (*bbgotypes.Ticker, error) {
	return &bbgotypes.Ticker{}, nil
}

func (e *strategyRuntimeStubExchange) QueryTickers(
	context.Context,
	...string,
) (map[string]bbgotypes.Ticker, error) {
	return map[string]bbgotypes.Ticker{}, nil
}

func (e *strategyRuntimeStubExchange) QueryKLines(
	_ context.Context,
	symbol string,
	interval bbgotypes.Interval,
	_ bbgotypes.KLineQueryOptions,
) ([]bbgotypes.KLine, error) {
	if e.queryKLinesErr != nil {
		return nil, e.queryKLinesErr
	}
	if history := e.history[symbol]; len(history) > 0 {
		return append([]bbgotypes.KLine(nil), history...), nil
	}
	return []bbgotypes.KLine{
		strategyRuntimeHistoricalKLine(symbol, interval, 100, strategyRuntimeTestTime(9, 59, 0)),
	}, nil
}

func (e *strategyRuntimeStubExchange) QueryAccount(context.Context) (*bbgotypes.Account, error) {
	return bbgotypes.NewAccount(), nil
}

func (e *strategyRuntimeStubExchange) QueryAccountBalances(context.Context) (bbgotypes.BalanceMap, error) {
	return bbgotypes.BalanceMap{}, nil
}

func (e *strategyRuntimeStubExchange) SubmitOrder(
	context.Context,
	bbgotypes.SubmitOrder,
) (*bbgotypes.Order, error) {
	return nil, nil
}

func (e *strategyRuntimeStubExchange) QueryOpenOrders(
	context.Context,
	string,
) ([]bbgotypes.Order, error) {
	return nil, nil
}

func (e *strategyRuntimeStubExchange) CancelOrders(context.Context, ...bbgotypes.Order) error {
	return nil
}

func (e *strategyRuntimeStubExchange) QueryBrokerFunds(
	context.Context,
	broker.ReadQuery,
) (*broker.FundsSnapshot, error) {
	if e.queryFundsErr != nil {
		return nil, e.queryFundsErr
	}
	return cloneStrategyRuntimeFundsSnapshot(e.funds), nil
}

func (e *strategyRuntimeStubExchange) QueryBrokerPositions(
	context.Context,
	broker.ReadQuery,
) ([]broker.PositionSnapshot, error) {
	if e.queryPositionsErr != nil {
		return nil, e.queryPositionsErr
	}
	return append([]broker.PositionSnapshot(nil), e.positions...), nil
}

func (e *strategyRuntimeStubExchange) PlaceBrokerOrder(
	context.Context,
	broker.PlaceOrderQuery,
) (*broker.PlaceOrderResult, error) {
	return &broker.PlaceOrderResult{}, nil
}

func (e *strategyRuntimeStubExchange) CancelBrokerOrder(
	context.Context,
	broker.ReadQuery,
	broker.CancelOrder,
) error {
	return nil
}

func (e *strategyRuntimeStubExchange) appendHistory(symbol string, klines ...bbgotypes.KLine) {
	e.history[symbol] = append(e.history[symbol], klines...)
}

func strategyRuntimeHistoricalKLine(
	symbol string,
	interval bbgotypes.Interval,
	closePrice float64,
	start time.Time,
) bbgotypes.KLine {
	end := start.Add(interval.Duration()).Add(-time.Millisecond)
	price := fixedpoint.NewFromFloat(closePrice)
	return bbgotypes.KLine{
		Exchange: bbgotypes.ExchangeFutu, Symbol: symbol,
		StartTime: bbgotypes.Time(start), EndTime: bbgotypes.Time(end), Interval: interval,
		Open: price, Close: price, High: price, Low: price,
		Volume: fixedpoint.NewFromFloat(100), QuoteVolume: fixedpoint.NewFromFloat(closePrice * 100),
		Closed: true,
	}
}

func strategyRuntimeTestTime(hour, minute, second int) time.Time {
	return time.Date(2026, time.May, 28, hour, minute, second, 0, time.UTC)
}

func strategyRuntimeTestTrade(symbol string, price float64, at time.Time) bbgotypes.Trade {
	return bbgotypes.Trade{
		ID:            uint64(at.Unix()),
		Symbol:        symbol,
		Side:          bbgotypes.SideTypeBuy,
		Price:         fixedpoint.NewFromFloat(price),
		Quantity:      fixedpoint.NewFromFloat(1),
		QuoteQuantity: fixedpoint.NewFromFloat(price),
		Time:          bbgotypes.Time(at),
	}
}

func TestPineWorkerLiveUsesStatefulSessionAfterWarmup(t *testing.T) {
	runner := &fakeStatefulPineWorkerRunner{session: &fakeStatefulPineWorkerSession{}}
	symbolRuntime := &symbolRuntime{
		symbol: "US.AAPL",
		market: bbgotypes.Market{Symbol: "US.AAPL", StepSize: fixedpoint.One, MinQuantity: fixedpoint.One},
	}
	executor := &strategyNotifyOnlyOrderExecutor{runner: symbolRuntime}
	live, err := newStrategyRuntimePineWorkerLive(
		runner, stratsrv.ManagedInstance{ID: "stateful-instance"}, "US.AAPL",
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
	if runner.session.appendRequests[0].Candles[0].OpenTime != stratsrv.CandleFromKLine(closed).OpenTime {
		t.Fatalf("incremental candle = %#v", runner.session.appendRequests[0].Candles)
	}
	if err := live.closeSession(t.Context()); err != nil || runner.session.closeCalls != 1 {
		t.Fatalf("closeSession err=%v calls=%d", err, runner.session.closeCalls)
	}
}

func TestPineWorkerLiveRemainingConstructorAndWarmupErrors(t *testing.T) {
	worker := newFakeStrategyRuntimePineWorker()
	runtime := &symbolRuntime{symbol: "US.AAPL", market: bbgotypes.Market{Symbol: "US.AAPL"}}
	executor := &strategyNotifyOnlyOrderExecutor{runner: runtime}
	if _, err := newStrategyRuntimePineWorkerLive(worker, stratsrv.ManagedInstance{}, "US.AAPL", bbgotypes.Interval1m, " ", executor, runtime, nil); err == nil {
		t.Fatal("blank live source error = nil")
	}
	if _, err := newStrategyRuntimePineWorkerLive(worker, stratsrv.ManagedInstance{}, "US.AAPL", bbgotypes.Interval1m, "strategy(\"x\")", nil, runtime, nil); err == nil {
		t.Fatal("nil live executor error = nil")
	}
	if _, err := newStrategyRuntimePineWorkerLive(worker, stratsrv.ManagedInstance{}, "US.AAPL", bbgotypes.Interval1m, "strategy(\"x\")", executor, nil, nil); err == nil {
		t.Fatal("nil symbol runtime error = nil")
	}

	live := &pineWorkerLive{source: "not pine", interval: bbgotypes.Interval1m, symbol: "US.AAPL"}
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
	live, err := newStrategyRuntimePineWorkerLive(worker, stratsrv.ManagedInstance{ID: "instance"}, "US.AAPL", bbgotypes.Interval1m, "//@version=6\nstrategy(\"Coverage\")", executor, runtime, nil)
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
	if _, err := nilSizer.QuantityForCommand(stratsrv.WorkerOrderCommand{ID: "nil", QuantityPct: 10}, market); err == nil {
		t.Fatal("nil sizer quantity error = nil")
	}
	if nilSizer.NetPosition().Sign() != 0 {
		t.Fatal("nil sizer position was nonzero")
	}

	account := bbgotypes.NewAccount()
	runner := &symbolRuntime{
		symbol: "US.AAPL", session: &bbgo.ExchangeSession{Account: account},
		cachedPositions: []broker.PositionSnapshot{{Market: "US", Symbol: "AAPL", Quantity: -4}},
	}
	sizer := &strategyRuntimeLiveSizer{runner: runner}
	sizer.onKLineClosed(bbgotypes.KLine{Symbol: "US.MSFT", Close: fixedpoint.NewFromFloat(200)})
	if sizer.lastPrice.Sign() != 0 {
		t.Fatal("mismatched kline changed last price")
	}
	for _, pct := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := sizer.QuantityForCommand(stratsrv.WorkerOrderCommand{ID: "invalid", Kind: "entry", QuantityPct: pct}, market); err == nil {
			t.Fatalf("invalid quantity pct %v error = nil", pct)
		}
	}
	if _, err := sizer.QuantityForCommand(stratsrv.WorkerOrderCommand{ID: "unsupported", Kind: "cancel", QuantityPct: 10}, market); err == nil {
		t.Fatal("unsupported quantity kind error = nil")
	}
	if got := sizer.NetPosition(); got.Float64() != -4 {
		t.Fatalf("net position = %v", got)
	}

	entry := stratsrv.WorkerOrderCommand{ID: "entry", Kind: "entry", QuantityPct: 10}
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

	closeCommand := stratsrv.WorkerOrderCommand{ID: "close", Kind: "close", QuantityPct: 200}
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
	runner := &symbolRuntime{symbol: "US.AAPL", session: &bbgo.ExchangeSession{}}
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
	if price := sizer.priceForCommand(stratsrv.WorkerOrderCommand{LimitPrice: 10.12}, market); price.Float64() <= 10 {
		t.Fatalf("limit command price = %v", price)
	}
	if price := sizer.priceForCommand(stratsrv.WorkerOrderCommand{StopPrice: 11.2}, market); price.Float64() != 11.2 {
		t.Fatalf("stop command price = %v", price)
	}
	runner.currentBucket = &bbgotypes.KLine{Close: fixedpoint.NewFromFloat(10.19)}
	sizer.lastPrice = fixedpoint.Zero
	if price := sizer.priceForCommand(stratsrv.WorkerOrderCommand{}, market); price.Float64() != 10.1 {
		t.Fatalf("truncated current price = %v", price)
	}

	params := PineWorkerParams(stratsrv.ManagedInstance{Params: map[string]any{
		"duration": time.Second, "unsupported": []string{"x"},
	}})
	if params["duration"] != "1s" {
		t.Fatalf("worker params = %#v", params)
	}
}

func TestPineWorkerLiveSessionFailureBoundaries(t *testing.T) {
	var nilLive *pineWorkerLive
	if err := nilLive.closeSession(t.Context()); err != nil {
		t.Fatalf("nil live close: %v", err)
	}

	newLive := func(t *testing.T, runner pineWorkerRunner) *pineWorkerLive {
		t.Helper()
		symbolRuntime := &symbolRuntime{
			symbol: "US.AAPL", market: bbgotypes.Market{Symbol: "US.AAPL", StepSize: fixedpoint.One, MinQuantity: fixedpoint.One},
		}
		live, err := newStrategyRuntimePineWorkerLive(
			runner, stratsrv.ManagedInstance{ID: "failure-instance"}, "US.AAPL", bbgotypes.Interval1m,
			"//@version=6\nstrategy(\"Failure\")", &strategyNotifyOnlyOrderExecutor{runner: symbolRuntime}, symbolRuntime, nil,
		)
		if err != nil {
			t.Fatalf("new live runtime: %v", err)
		}
		return live
	}

	openErr := errors.New("stateful open failed")
	runner := &fakeStatefulPineWorkerRunner{openErr: openErr}
	if err := newLive(t, runner).openSession(t.Context()); !errors.Is(err, openErr) {
		t.Fatalf("open error = %v", err)
	}

	runner = &fakeStatefulPineWorkerRunner{nilSession: true, openResponse: &pineworker.RunScriptResponse{SessionRevision: 1}}
	if err := newLive(t, runner).openSession(t.Context()); err == nil {
		t.Fatal("nil stateful session was accepted")
	}

	invalidSession := &fakeStatefulPineWorkerSession{}
	runner = &fakeStatefulPineWorkerRunner{
		session: invalidSession, openResponse: &pineworker.RunScriptResponse{SessionRevision: 2},
	}
	if err := newLive(t, runner).openSession(t.Context()); err == nil || invalidSession.closeCalls != 1 {
		t.Fatalf("invalid revision error = %v, close calls=%d", err, invalidSession.closeCalls)
	}

	closeErr := errors.New("stateful close failed")
	session := &fakeStatefulPineWorkerSession{closeErr: closeErr}
	runner = &fakeStatefulPineWorkerRunner{session: session}
	live := newLive(t, runner)
	if err := live.openSession(t.Context()); err != nil {
		t.Fatalf("open before close failure: %v", err)
	}
	if err := live.openSession(t.Context()); err != nil {
		t.Fatalf("idempotent live open: %v", err)
	}
	if err := live.closeSession(t.Context()); !errors.Is(err, closeErr) {
		t.Fatalf("close error = %v", err)
	}
	if err := live.closeSession(t.Context()); err != nil {
		t.Fatalf("empty close: %v", err)
	}

	appendErr := errors.New("stateful append failed")
	session = &fakeStatefulPineWorkerSession{appendErr: appendErr}
	runner = &fakeStatefulPineWorkerRunner{session: session}
	live = newLive(t, runner)
	if err := live.openSession(t.Context()); err != nil {
		t.Fatalf("open before append failure: %v", err)
	}
	if err := live.onClosedKLine(t.Context(), strategyRuntimeHistoricalKLine("US.AAPL", "1m", 100, strategyRuntimeTestTime(10, 0, 0))); !errors.Is(err, appendErr) {
		t.Fatalf("append error = %v", err)
	}
}

type fakeStatefulPineWorkerRunner struct {
	openRequest  pineworker.RunScriptRequest
	session      *fakeStatefulPineWorkerSession
	openResponse *pineworker.RunScriptResponse
	openErr      error
	nilSession   bool
	runCalls     int
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
	if runner.openErr != nil {
		return nil, pineworker.RunScriptResponse{}, runner.openErr
	}
	if runner.openResponse != nil {
		if runner.nilSession {
			return nil, *runner.openResponse, nil
		}
		return runner.session, *runner.openResponse, nil
	}
	return runner.session, pineworker.RunScriptResponse{SessionID: request.SessionID, SessionRevision: 1}, nil
}

type fakeStatefulPineWorkerSession struct {
	appendRequests []pineworker.RunScriptRequest
	appendErr      error
	closeErr       error
	closeCalls     int
}

func (session *fakeStatefulPineWorkerSession) Append(
	_ context.Context,
	request pineworker.RunScriptRequest,
) (pineworker.RunScriptResponse, error) {
	session.appendRequests = append(session.appendRequests, request)
	if session.appendErr != nil {
		return pineworker.RunScriptResponse{}, session.appendErr
	}
	return pineworker.RunScriptResponse{SessionRevision: uint64(len(session.appendRequests) + 1)}, nil
}

func (session *fakeStatefulPineWorkerSession) Close(context.Context) error {
	session.closeCalls++
	return session.closeErr
}

func TestLiveWarningSinkRecordsOnlyActionableOrderWarnings(t *testing.T) {
	var recorded []string
	sink := strategyRuntimeLiveWarningSink{record: func(message string) {
		recorded = append(recorded, message)
	}}

	sink.AddIgnoredOrderWarning(" ")
	sink.AddIgnoredOrderWarning("order quantity was rounded")
	sink.AddIgnoredOrderWarningGroup("risk", "order was rejected by risk policy")
	strategyRuntimeLiveWarningSink{}.AddIgnoredOrderWarning("no recorder")

	if len(recorded) != 2 {
		t.Fatalf("recorded warnings = %#v, want two actionable warnings", recorded)
	}
	if recorded[0] != "order quantity was rounded" || recorded[1] != "order was rejected by risk policy" {
		t.Fatalf("recorded warnings = %#v", recorded)
	}
}
