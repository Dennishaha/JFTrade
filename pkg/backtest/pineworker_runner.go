package backtest

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/bbgo/backtest"
	bbgo2 "github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/service"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

type pineWorkerBacktestPreparation struct {
	cfg                            RunConfig
	result                         *RunResult
	store                          *FutuKLineStore
	replayStore                    service.BackTestable
	streamer                       klineRangeStreamer
	sourceExchange                 *futu.Exchange
	compilation                    strategypine.Compilation
	strategyInterval               types.Interval
	warmupUntil                    time.Time
	queryStartTime                 time.Time
	quoteCurrency                  string
	backtestCfg                    *bbgo2.Backtest
	rejectOrdersWithoutMarketRules bool
}

type pineWorkerBacktestExecution struct {
	exchange     *bt.Exchange
	session      *bbgo2.ExchangeSession
	replayKLines *pineWorkerReplayKLineBatch
	plan         pineWorkerCompactReplayPlan
}

func RunWithPineWorker(ctx context.Context, cfg RunConfig, runner PineWorkerRunner) *RunResult {
	result := newRunResult(cfg)
	cfg, ok := normalizePineWorkerRunConfig(cfg, runner, result)
	if !ok {
		return result
	}
	store, ok := openPineWorkerBacktestStore(cfg, result)
	if !ok {
		return result
	}
	defer func() { besteffort.LogError(store.Close()) }()
	preparation, ok := preparePineWorkerBacktest(ctx, cfg, result, store)
	if !ok {
		return result
	}
	execution, ok := setupPineWorkerBacktestExecution(ctx, runner, preparation)
	if !ok {
		return result
	}
	executePineWorkerBacktest(ctx, preparation, execution)
	return result
}

func normalizePineWorkerRunConfig(cfg RunConfig, runner PineWorkerRunner, result *RunResult) (RunConfig, bool) {
	executionModel, err := NormalizeExecutionModelName(cfg.ExecutionModel)
	if err != nil {
		result.Error = err.Error()
		return cfg, false
	}
	cfg.ExecutionModel = executionModel
	result.ExecutionModel = executionModel
	if runner == nil {
		result.Error = "pine worker runner is required"
		return cfg, false
	}
	return cfg, true
}

func openPineWorkerBacktestStore(cfg RunConfig, result *RunResult) (*FutuKLineStore, bool) {
	if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
		result.Error = fmt.Sprintf("backtest database not found: %s (run 'jftrade kline-sync' first)", cfg.DBPath)
		return nil, false
	}
	store, err := NewFutuKLineStore(cfg.DBPath)
	if err != nil {
		result.Error = fmt.Sprintf("open backtest store: %v", err)
		return nil, false
	}
	if cfg.RehabType == "" {
		cfg.RehabType = "forward"
	}
	store.SetRehabType(cfg.RehabType)
	store.SetReadSessionScope(resolveBacktestReadSessionScope(cfg.UseExtendedHours))
	return store, true
}

func preparePineWorkerBacktest(ctx context.Context, cfg RunConfig, result *RunResult, store *FutuKLineStore) (*pineWorkerBacktestPreparation, bool) {
	cfg, compilation, strategyInterval, warmupUntil, queryStartTime, ok := preparePineWorkerStrategy(ctx, cfg, result, store)
	if !ok {
		return nil, false
	}
	replayStore := newBacktestReplayStore(store, cfg.UseExtendedHours)
	streamer, ok := replayStore.(klineRangeStreamer)
	if !ok {
		result.Error = "pine worker backtest replay store does not support streaming"
		return nil, false
	}
	sourceExchange := newBacktestFutuSourceExchange()
	rejectOrdersWithoutMarketRules := ensureBacktestSourceMarket(ctx, result, sourceExchange, cfg.Symbol)
	removeFutuMarketCache()
	quoteCurrency := resolveBacktestQuoteCurrency(cfg.Symbol, cfg.QuoteCurrency)
	result.QuoteCurrency = quoteCurrency
	cfg.TradingCosts = resolveBacktestTradingCosts(cfg, quoteCurrency, compilation.Program.Metadata)
	result.TradingCosts = cfg.TradingCosts
	backtestCfg := pineWorkerBacktestConfig(cfg, quoteCurrency, compilation.Program.Metadata)
	disableBacktestNativeFeeRates(backtestCfg)
	return &pineWorkerBacktestPreparation{
		cfg:                            cfg,
		result:                         result,
		store:                          store,
		replayStore:                    replayStore,
		streamer:                       streamer,
		sourceExchange:                 sourceExchange,
		compilation:                    compilation,
		strategyInterval:               strategyInterval,
		warmupUntil:                    warmupUntil,
		queryStartTime:                 queryStartTime,
		quoteCurrency:                  quoteCurrency,
		backtestCfg:                    backtestCfg,
		rejectOrdersWithoutMarketRules: rejectOrdersWithoutMarketRules,
	}, true
}

func preparePineWorkerStrategy(ctx context.Context, cfg RunConfig, result *RunResult, store *FutuKLineStore) (RunConfig, strategypine.Compilation, types.Interval, time.Time, time.Time, bool) {
	sourceFormat := strategydefinition.NormalizeSourceFormat(cfg.SourceFormat)
	if sourceFormat != strategydefinition.SourceFormatPineV6 {
		result.Error = fmt.Sprintf("unsupported strategy source format: %s", sourceFormat)
		return cfg, strategypine.Compilation{}, "", time.Time{}, time.Time{}, false
	}
	compilation, err := strategypine.Compile(cfg.StrategyScript)
	if err != nil {
		result.Error = fmt.Sprintf("compile pine strategy metadata: %v", err)
		return cfg, strategypine.Compilation{}, "", time.Time{}, time.Time{}, false
	}
	cfg.InitialBalance = resolvePineInitialBalance(cfg.InitialBalance, compilation.Program.Metadata)
	strategyInterval := types.Interval(cfg.Interval)
	derivedWarmupCandles, err := indicatorruntime.WarmupBarsFromPlanForSymbolWithOptions(
		compilation.Requirements,
		strategyInterval,
		cfg.Symbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: cfg.UseExtendedHours != nil && *cfg.UseExtendedHours},
	)
	if err != nil {
		result.Error = fmt.Sprintf("derive strategy warmup: %v", err)
		return cfg, strategypine.Compilation{}, "", time.Time{}, time.Time{}, false
	}
	warmupUntil, queryStartTime := pineWorkerWarmupRange(cfg, strategyInterval, derivedWarmupCandles)
	if err := store.EnsureCoverage(cfg.Symbol, strategyInterval, queryStartTime, cfg.EndTime); err != nil {
		result.Error = err.Error()
		return cfg, strategypine.Compilation{}, "", time.Time{}, time.Time{}, false
	}
	return cfg, compilation, strategyInterval, warmupUntil, queryStartTime, true
}

func pineWorkerWarmupRange(cfg RunConfig, strategyInterval types.Interval, derivedWarmupCandles int) (time.Time, time.Time) {
	warmupUntil := cfg.StartTime
	queryStartTime := cfg.StartTime
	if warmupCandles := max(derivedWarmupCandles, cfg.WarmupCandles); warmupCandles > 0 {
		queryStartTime = cfg.StartTime.Add(-strategyInterval.Duration() * time.Duration(warmupCandles))
	}
	return warmupUntil, queryStartTime
}

func setupPineWorkerBacktestExecution(ctx context.Context, runner PineWorkerRunner, prep *pineWorkerBacktestPreparation) (*pineWorkerBacktestExecution, bool) {
	btExchange, session, ok := newPineWorkerBacktestSession(ctx, prep)
	if !ok {
		return nil, false
	}
	replayKLines, err := collectPineWorkerReplayKLineBatch(prep.streamer, prep.queryStartTime, prep.cfg.EndTime, btExchange, prep.cfg.Symbol, prep.strategyInterval)
	if err != nil {
		prep.result.Error = err.Error()
		return nil, false
	}
	plan, err := planPineWorkerCompactReplay(ctx, PineWorkerBacktestAdapter{Runner: runner}, PineWorkerReplayPlanRequest{
		JobID:     defaultPineWorkerReplayJobID(prep.cfg.Symbol, prep.cfg.Interval),
		Source:    prep.cfg.StrategyScript,
		Symbol:    prep.cfg.Symbol,
		Timeframe: prep.cfg.Interval,
	}, replayKLines)
	if err != nil {
		prep.result.Error = fmt.Sprintf("plan pine worker replay: %v", err)
		return nil, false
	}
	return &pineWorkerBacktestExecution{
		exchange:     btExchange,
		session:      session,
		replayKLines: replayKLines,
		plan:         plan,
	}, true
}

func newPineWorkerBacktestSession(ctx context.Context, prep *pineWorkerBacktestPreparation) (*bt.Exchange, *bbgo2.ExchangeSession, bool) {
	btExchange, err := bt.NewExchange(prep.sourceExchange.Name(), prep.sourceExchange, prep.replayStore, prep.backtestCfg)
	if err != nil {
		prep.result.Error = fmt.Sprintf("create backtest exchange: %v", err)
		return nil, nil, false
	}
	environ := bbgo2.NewEnvironment()
	session := environ.AddExchange("futu", btExchange)
	environ.SetStartTime(prep.cfg.StartTime)
	if markets, err := btExchange.QueryMarkets(ctx); err == nil {
		session.SetMarkets(markets)
	}
	if err := environ.Init(ctx); err != nil {
		prep.result.Error = fmt.Sprintf("init environment: %v", err)
		return nil, nil, false
	}
	session.Account.CanTrade = true
	session.Account.CanDeposit = true
	session.Account.CanWithdraw = true
	if err := btExchange.Prepare(&bbgo2.Config{Backtest: prep.backtestCfg}); err != nil && !isMissingPrepareKLineError(err) {
		log.Printf("backtest prepare warning: %v", err)
	}
	btExchange.BindUserData(jftradeCheckedTypeAssertion[types.StandardStreamEmitter](session.UserDataStream))
	btExchange.MarketDataStream = jftradeCheckedTypeAssertion[types.StandardStreamEmitter](session.MarketDataStream)
	return btExchange, session, true
}

func executePineWorkerBacktest(ctx context.Context, prep *pineWorkerBacktestPreparation, execution *pineWorkerBacktestExecution) {
	replaySizer := newPineWorkerReplaySizer(prep.cfg.Symbol, prep.quoteCurrency, execution.session.Account)
	execution.session.UserDataStream.OnOrderUpdate(replaySizer.onOrderUpdate)
	executor := newPineWorkerBacktestOrderExecutor(
		execution.session,
		replaySizer,
		prep.result,
		prep.compilation.Program.Metadata,
	)
	collector := newPineWorkerResultCollector(prep, execution)
	feeEngine := newBacktestFeeEngine(execution.session.Account, prep.quoteCurrency, prep.cfg.InstrumentType, prep.cfg.TradingCosts, prep.result, collector.recordTradeFees)
	execution.session.UserDataStream.OnTradeUpdate(feeEngine.onTradeUpdate)
	execution.session.UserDataStream.OnOrderUpdate(collector.onOrderUpdate)
	bindPineWorkerReplayHandlers(ctx, prep, execution, replaySizer, executor, collector)
	consumePineWorkerReplay(prep, execution)
	if prep.result.Error != "" {
		return
	}
	feeEngine.finalize()
	totalOrders, filledOrders := collector.finalize(ctx, execution.exchange, prep.cfg.InitialBalance)
	log.Printf("pine worker backtest: done totalOrders=%d filledOrders=%d finalBalance=%.2f metadata=%+v", totalOrders, filledOrders, prep.result.FinalBalance, execution.plan.Metadata)
}

func newPineWorkerResultCollector(prep *pineWorkerBacktestPreparation, execution *pineWorkerBacktestExecution) *resultCollector {
	collector := newResultCollector(prep.cfg.Symbol, prep.strategyInterval, prep.quoteCurrency, prep.warmupUntil, prep.result)
	if resultCapacity := execution.replayKLines.resultCapacity(prep.warmupUntil); resultCapacity > 0 {
		collector.candles = make([]Candle, 0, resultCapacity)
		collector.pnlCurve = make([]PnLPoint, 0, resultCapacity)
	}
	return collector
}

func bindPineWorkerReplayHandlers(
	ctx context.Context,
	prep *pineWorkerBacktestPreparation,
	execution *pineWorkerBacktestExecution,
	replaySizer *pineWorkerReplaySizer,
	executor bbgo2.OrderExecutor,
	collector *resultCollector,
) {
	execution.exchange.Src = &bt.ExchangeDataSource{Exchange: execution.exchange, Session: execution.session}
	commandExecutor := &PineWorkerCommandExecutor{
		Symbol:                         prep.cfg.Symbol,
		OrderExecutor:                  executor,
		MarketResolver:                 execution.session,
		PositionSizer:                  replaySizer,
		WarningSink:                    prep.result,
		RejectOrdersWithoutMarketRules: prep.rejectOrdersWithoutMarketRules,
	}
	replayState := newPineWorkerBacktestReplayState(execution.replayKLines, execution.plan.Commands, commandExecutor)
	execution.session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		execution.session.LastPrices()[kline.Symbol] = kline.Close
		if err := replayState.onKLineClosed(ctx, kline); err != nil {
			prep.result.Error = fmt.Sprintf("pine worker replay command: %v", err)
		}
	})
	execution.session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		collector.onKLineClosed(ctx, execution.exchange, kline)
	})
}

func consumePineWorkerReplay(prep *pineWorkerBacktestPreparation, execution *pineWorkerBacktestExecution) {
	execution.replayKLines.forEach(func(kline types.KLine) bool {
		if prep.result.Error != "" {
			return false
		}
		execution.exchange.ConsumeKLine(kline, prep.strategyInterval)
		return true
	})
	if prep.result.Error != "" {
		return
	}
	if err := execution.exchange.CloseMarketData(); err != nil {
		log.Printf("backtest close market data: %v", err)
	}
}

func newBacktestFutuSourceExchange() *futu.Exchange {
	addr := strings.TrimSpace(os.Getenv(futu.EnvOpenDAddr))
	if addr == "" {
		addr = futu.DefaultOpenDAddr
	}
	webSocketKey := strings.TrimSpace(os.Getenv(futu.EnvOpenDWebSocketKey))
	if webSocketKey == "" {
		webSocketKey = strings.TrimSpace(os.Getenv("JFTRADE_FUTU_WEBSOCKET_KEY"))
	}
	return futu.NewExchangeWithConfig(opend.Config{Addr: addr, WebSocketKey: webSocketKey})
}

type backtestSourceMarketEnsurer interface {
	EnsureMarket(symbol string)
	EnsureMarketWithDiagnostics(ctx context.Context, symbol string) (types.Market, []string, error)
}

func ensureBacktestSourceMarket(ctx context.Context, result *RunResult, exchange backtestSourceMarketEnsurer, symbol string) bool {
	marketRuleCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	_, warnings, err := exchange.EnsureMarketWithDiagnostics(marketRuleCtx, symbol)
	for _, warning := range warnings {
		result.AddWarning(fmt.Sprintf("market rule warning for %s: %s", symbol, warning))
	}
	if err != nil && isHKBacktestSymbol(symbol) {
		exchange.EnsureMarket(symbol)
		result.AddWarning(fmt.Sprintf("lot size unavailable for %s; orders will be ignored until market quantity rules are available: %v", symbol, err))
		return true
	}
	return false
}

func isHKBacktestSymbol(symbol string) bool {
	upper := strings.ToUpper(strings.TrimSpace(symbol))
	return strings.HasPrefix(upper, "HK.") || strings.HasPrefix(upper, "HK:")
}

func newRunResult(cfg RunConfig) *RunResult {
	executionModel, _ := NormalizeExecutionModelName(cfg.ExecutionModel)
	return &RunResult{
		Symbol:         cfg.Symbol,
		Interval:       cfg.Interval,
		StartTime:      cfg.StartTime.UTC().Format(time.RFC3339Nano),
		EndTime:        cfg.EndTime.UTC().Format(time.RFC3339Nano),
		ExecutionModel: executionModel,
	}
}

func newPineWorkerBacktestOrderExecutor(
	session *bbgo2.ExchangeSession,
	replaySizer *pineWorkerReplaySizer,
	result *RunResult,
	metadata strategyir.StrategyMetadata,
) bbgo2.OrderExecutor {
	executor := newConservativeBarExecutor(
		session.Account,
		jftradeCheckedTypeAssertion[types.StandardStreamEmitter](session.UserDataStream),
		conservativeBarExecutorOptions{
			ProcessOrdersOnClose: metadata.ProcessOnClose,
			SlippageTicks:        metadata.Slippage,
			WarningSink:          result,
		},
	)
	session.MarketDataStream.OnKLineClosed(executor.onKLineClosed)
	session.MarketDataStream.OnKLineClosed(replaySizer.onKLineClosed)
	return executor
}

func resolveBacktestQuoteCurrency(symbol string, requested string) string {
	if requested != "" {
		return requested
	}
	upperSymbol := strings.ToUpper(symbol)
	if strings.HasPrefix(upperSymbol, "US.") {
		return "USD"
	}
	if strings.HasPrefix(upperSymbol, "SH.") || strings.HasPrefix(upperSymbol, "SZ.") || strings.HasPrefix(upperSymbol, "CN.") {
		return "CNY"
	}
	return "HKD"
}

func removeFutuMarketCache() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	besteffort.LogError(os.Remove(filepath.Join(home, ".bbgo", "cache", "futu-markets.json")))
}

func pineWorkerBacktestConfig(cfg RunConfig, quoteCurrency string, metadata strategyir.StrategyMetadata) *bbgo2.Backtest {
	return &bbgo2.Backtest{
		StartTime: types.LooseFormatTime(cfg.StartTime),
		EndTime:   (*types.LooseFormatTime)(&cfg.EndTime),
		Symbols:   []string{cfg.Symbol},
		Sessions:  []string{"futu"},
		Accounts: map[string]bbgo2.BacktestAccount{
			"futu": {
				MakerFeeRate: pineCommissionRate(metadata),
				TakerFeeRate: pineCommissionRate(metadata),
				Balances: bbgo2.BacktestAccountBalanceMap{
					quoteCurrency: fixedpoint.NewFromFloat(cfg.InitialBalance),
				},
			},
		},
	}
}

type pineWorkerBacktestReplayState struct {
	klines          *pineWorkerReplayKLineBatch
	commands        []WorkerOrderCommand
	commandExecutor *PineWorkerCommandExecutor
	nextBarIndex    int
	nextCommand     int
}

func newPineWorkerBacktestReplayState(
	klines *pineWorkerReplayKLineBatch,
	commands []WorkerOrderCommand,
	commandExecutor *PineWorkerCommandExecutor,
) *pineWorkerBacktestReplayState {
	return &pineWorkerBacktestReplayState{klines: klines, commands: commands, commandExecutor: commandExecutor}
}

func (state *pineWorkerBacktestReplayState) onKLineClosed(ctx context.Context, kline types.KLine) error {
	if state.nextBarIndex >= state.klines.Len() {
		return fmt.Errorf("received extra closed kline at index %d", state.nextBarIndex)
	}
	expected, ok := state.klines.At(state.nextBarIndex)
	if !ok {
		return fmt.Errorf("missing planned kline at index %d", state.nextBarIndex)
	}
	if openTime, expectedOpenTime := kline.StartTime.Time().UnixMilli(), expected.StartTime.Time().UnixMilli(); expectedOpenTime > 0 && openTime != expectedOpenTime {
		return fmt.Errorf("closed kline %d open time %d does not match planned candle %d", state.nextBarIndex, openTime, expectedOpenTime)
	}
	barIndex := state.nextBarIndex
	state.nextBarIndex++
	commandStart := state.nextCommand
	for state.nextCommand < len(state.commands) && state.commands[state.nextCommand].BarIndex == barIndex {
		state.nextCommand++
	}
	if state.nextCommand > commandStart {
		if err := state.commandExecutor.ExecuteBarCommands(ctx, state.commands[commandStart:state.nextCommand]); err != nil {
			return fmt.Errorf("execute commands for bar %d: %w", barIndex, err)
		}
	}
	return nil
}
