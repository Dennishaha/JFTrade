package backtest

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	bt "github.com/c9s/bbgo/pkg/backtest"
	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func RunWithPineWorker(ctx context.Context, cfg RunConfig, runner PineWorkerRunner) *RunResult {
	result := newRunResult(cfg)
	if runner == nil {
		result.Error = "pine worker runner is required"
		return result
	}

	if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
		result.Error = fmt.Sprintf("backtest database not found: %s (run 'jftrade kline-sync' first)", cfg.DBPath)
		return result
	}
	store, err := NewFutuKLineStore(cfg.DBPath)
	if err != nil {
		result.Error = fmt.Sprintf("open backtest store: %v", err)
		return result
	}
	defer func() { jftradeLogError(store.Close()) }()

	if cfg.RehabType == "" {
		cfg.RehabType = "forward"
	}
	store.SetRehabType(cfg.RehabType)
	store.SetReadSessionScope(resolveBacktestReadSessionScope(cfg.UseExtendedHours))

	sourceFormat := strategydefinition.NormalizeSourceFormat(cfg.SourceFormat)
	if sourceFormat != strategydefinition.SourceFormatPineV6 {
		result.Error = fmt.Sprintf("unsupported strategy source format: %s", sourceFormat)
		return result
	}
	compilation, err := strategypine.Compile(cfg.StrategyScript)
	if err != nil {
		result.Error = fmt.Sprintf("compile pine strategy metadata: %v", err)
		return result
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
		return result
	}
	warmupUntil := cfg.StartTime
	queryStartTime := cfg.StartTime
	if warmupCandles := max(derivedWarmupCandles, cfg.WarmupCandles); warmupCandles > 0 {
		queryStartTime = cfg.StartTime.Add(-strategyInterval.Duration() * time.Duration(warmupCandles))
	}
	if err := store.EnsureCoverage(cfg.Symbol, strategyInterval, queryStartTime, cfg.EndTime); err != nil {
		result.Error = err.Error()
		return result
	}

	replayStore := newBacktestReplayStore(store, cfg.UseExtendedHours)
	streamer, ok := replayStore.(klineRangeStreamer)
	if !ok {
		result.Error = "pine worker backtest replay store does not support streaming"
		return result
	}

	sourceExchange := futu.NewExchange("127.0.0.1:11110")
	sourceExchange.EnsureMarket(cfg.Symbol)
	removeFutuMarketCache()

	quoteCurrency := resolveBacktestQuoteCurrency(cfg.Symbol, cfg.QuoteCurrency)
	result.QuoteCurrency = quoteCurrency
	cfg.TradingCosts = resolveBacktestTradingCosts(cfg, quoteCurrency, compilation.Program.Metadata)
	result.TradingCosts = cfg.TradingCosts
	backtestCfg := pineWorkerBacktestConfig(cfg, quoteCurrency, compilation.Program.Metadata)
	disableBacktestNativeFeeRates(backtestCfg)
	btExchange, err := bt.NewExchange(sourceExchange.Name(), sourceExchange, replayStore, backtestCfg)
	if err != nil {
		result.Error = fmt.Sprintf("create backtest exchange: %v", err)
		return result
	}

	environ := bbgo2.NewEnvironment()
	session := environ.AddExchange("futu", btExchange)
	environ.SetStartTime(cfg.StartTime)
	if markets, err := btExchange.QueryMarkets(ctx); err == nil {
		session.SetMarkets(markets)
	}
	if err := environ.Init(ctx); err != nil {
		result.Error = fmt.Sprintf("init environment: %v", err)
		return result
	}
	session.Account.CanTrade = true
	session.Account.CanDeposit = true
	session.Account.CanWithdraw = true
	if err := btExchange.Prepare(&bbgo2.Config{Backtest: backtestCfg}); err != nil && !isMissingPrepareKLineError(err) {
		log.Printf("backtest prepare warning: %v", err)
	}
	btExchange.BindUserData(jftradeCheckedTypeAssertion[types.StandardStreamEmitter](session.UserDataStream))
	btExchange.MarketDataStream = jftradeCheckedTypeAssertion[types.StandardStreamEmitter](session.MarketDataStream)

	replayKLines, err := CollectPineWorkerReplayKLines(streamer, queryStartTime, cfg.EndTime, btExchange, cfg.Symbol, strategyInterval)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	plan, err := (PineWorkerReplayPlanner{Adapter: PineWorkerBacktestAdapter{Runner: runner}}).Plan(ctx, PineWorkerReplayPlanRequest{
		JobID:     defaultPineWorkerReplayJobID(cfg.Symbol, cfg.Interval),
		Source:    cfg.StrategyScript,
		Symbol:    cfg.Symbol,
		Timeframe: cfg.Interval,
		KLines:    replayKLines,
	})
	if err != nil {
		result.Error = fmt.Sprintf("plan pine worker replay: %v", err)
		return result
	}

	defaultExecutor := sessionDefaultOrderExecutor(session)
	if defaultExecutor == nil {
		result.Error = "session order executor is nil"
		return result
	}
	shortReplayExecutor := newPineWorkerShortReplayExecutor(
		defaultExecutor,
		session.Account,
		jftradeCheckedTypeAssertion[types.StandardStreamEmitter](session.UserDataStream),
	)
	session.MarketDataStream.OnKLineClosed(shortReplayExecutor.onKLineClosed)
	replaySizer := newPineWorkerReplaySizer(cfg.Symbol, quoteCurrency, session.Account)
	session.MarketDataStream.OnKLineClosed(replaySizer.onKLineClosed)
	session.UserDataStream.OnOrderUpdate(replaySizer.onOrderUpdate)
	var executor bbgo2.OrderExecutor = shortReplayExecutor
	if compilation.Program.Metadata.Slippage > 0 {
		slippageExecutor := newBacktestSlippageExecutor(executor, session, compilation.Program.Metadata.Slippage)
		session.MarketDataStream.OnKLineClosed(slippageExecutor.onKLineClosed)
		executor = slippageExecutor
	}

	collector := newResultCollector(cfg.Symbol, strategyInterval, quoteCurrency, warmupUntil, result)
	if estimatedBars := estimateReplayBarCapacity(warmupUntil, cfg.EndTime, strategyInterval); estimatedBars > 0 {
		collector.candles = make([]Candle, 0, estimatedBars)
		collector.pnlCurve = make([]PnLPoint, 0, estimatedBars)
	}
	feeEngine := newBacktestFeeEngine(session.Account, quoteCurrency, cfg.InstrumentType, cfg.TradingCosts, result, collector.recordTradeFees)
	session.UserDataStream.OnTradeUpdate(feeEngine.onTradeUpdate)
	session.UserDataStream.OnOrderUpdate(collector.onOrderUpdate)
	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		collector.onKLineClosed(ctx, btExchange, kline)
	})

	btExchange.Src = &bt.ExchangeDataSource{Exchange: btExchange, Session: session}
	commandExecutor := &PineWorkerCommandExecutor{
		Symbol:         cfg.Symbol,
		OrderExecutor:  executor,
		MarketResolver: session,
		PositionSizer:  replaySizer,
	}
	replayState := newPineWorkerBacktestReplayState(plan, commandExecutor)
	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		session.LastPrices()[kline.Symbol] = kline.Close
		if err := replayState.onKLineClosed(ctx, kline); err != nil {
			result.Error = fmt.Sprintf("pine worker replay command: %v", err)
		}
	})
	for _, kline := range replayKLines {
		if result.Error != "" {
			return result
		}
		btExchange.ConsumeKLine(kline, strategyInterval)
	}
	if err := btExchange.CloseMarketData(); err != nil {
		log.Printf("backtest close market data: %v", err)
	}
	if result.Error != "" {
		return result
	}

	feeEngine.finalize()
	totalOrders, filledOrders := collector.finalize(ctx, btExchange, cfg.InitialBalance)
	log.Printf("pine worker backtest: done totalOrders=%d filledOrders=%d finalBalance=%.2f metadata=%+v", totalOrders, filledOrders, result.FinalBalance, plan.Metadata)
	return result
}

func newRunResult(cfg RunConfig) *RunResult {
	return &RunResult{
		Symbol:    cfg.Symbol,
		Interval:  cfg.Interval,
		StartTime: cfg.StartTime.UTC().Format(time.RFC3339Nano),
		EndTime:   cfg.EndTime.UTC().Format(time.RFC3339Nano),
	}
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
	jftradeLogError(os.Remove(filepath.Join(home, ".bbgo", "cache", "futu-markets.json")))
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
	plan            PineWorkerReplayPlan
	commandExecutor *PineWorkerCommandExecutor
	nextBarIndex    int
}

func newPineWorkerBacktestReplayState(plan PineWorkerReplayPlan, commandExecutor *PineWorkerCommandExecutor) *pineWorkerBacktestReplayState {
	return &pineWorkerBacktestReplayState{plan: plan, commandExecutor: commandExecutor}
}

func (state *pineWorkerBacktestReplayState) onKLineClosed(ctx context.Context, kline types.KLine) error {
	if state.nextBarIndex >= state.plan.CandleCount {
		return fmt.Errorf("received extra closed kline at index %d", state.nextBarIndex)
	}
	expected := state.plan.Request.Candles[state.nextBarIndex]
	if openTime := kline.StartTime.Time().UnixMilli(); expected.OpenTime > 0 && openTime != expected.OpenTime {
		return fmt.Errorf("closed kline %d open time %d does not match planned candle %d", state.nextBarIndex, openTime, expected.OpenTime)
	}
	barIndex := state.nextBarIndex
	state.nextBarIndex++
	if commands := state.plan.ByBarIndex[barIndex]; len(commands) > 0 {
		if err := state.commandExecutor.ExecuteBarCommands(ctx, commands); err != nil {
			return fmt.Errorf("execute commands for bar %d: %w", barIndex, err)
		}
	}
	return nil
}
