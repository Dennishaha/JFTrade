// Package backtest provides a standalone backtest runner for Futu strategies
// using bbgo's backtest engine with a local SQLite K-line store.
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
	"github.com/jftrade/jftrade-main/pkg/strategy/pineruntime"
)

// Run executes a backtest with the given configuration.
//
// It opens the SQLite K-line store, creates a bbgo backtest.Exchange,
// instantiates the configured strategy runtime, and replays historical K-lines
// one by one through bbgo's matching engine.
func Run(ctx context.Context, cfg RunConfig) *RunResult {
	result := &RunResult{
		Symbol:    cfg.Symbol,
		Interval:  cfg.Interval,
		StartTime: cfg.StartTime.Format(time.RFC3339),
		EndTime:   cfg.EndTime.Format(time.RFC3339),
	}

	// ── 1. Open K-line store ────────────────────────────────────────
	if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
		result.Error = fmt.Sprintf("backtest database not found: %s (run 'jftrade kline-sync' first)", cfg.DBPath)
		return result
	}
	store, err := NewFutuKLineStore(cfg.DBPath)
	if err != nil {
		result.Error = fmt.Sprintf("open backtest store: %v", err)
		return result
	}
	defer store.Close()

	// Configure price-adjustment mode for all subsequent K-line queries.
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
		result.Error = fmt.Sprintf("compile pine strategy: %v", err)
		return result
	}
	cfg.InitialBalance = resolvePineInitialBalance(cfg.InitialBalance, compilation.Program.Metadata)

	derivedWarmupCandles, err := indicatorruntime.WarmupBarsFromPlanForSymbolWithOptions(
		compilation.Requirements,
		types.Interval(cfg.Interval),
		cfg.Symbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: cfg.UseExtendedHours != nil && *cfg.UseExtendedHours},
	)
	if err != nil {
		result.Error = fmt.Sprintf("derive strategy warmup: %v", err)
		return result
	}
	warmupCandles := cfg.WarmupCandles
	if derivedWarmupCandles > warmupCandles {
		warmupCandles = derivedWarmupCandles
	}

	// ── Warmup ──────────────────────────────────────────────────────
	// Extend the kline query start time backward so indicators have
	// enough history before the actual backtest start.
	warmupUntil := cfg.StartTime
	queryStartTime := cfg.StartTime
	if warmupCandles > 0 {
		intervalDur := types.Interval(cfg.Interval).Duration()
		warmupDur := intervalDur * time.Duration(warmupCandles)
		queryStartTime = cfg.StartTime.Add(-warmupDur)
		log.Printf("backtest: warmup %d candles (configured=%d derived=%d, %v), query from %s",
			warmupCandles, cfg.WarmupCandles, derivedWarmupCandles, warmupDur, queryStartTime.Format(time.RFC3339))
	}
	if err := store.EnsureCoverage(cfg.Symbol, types.Interval(cfg.Interval), queryStartTime, cfg.EndTime); err != nil {
		result.Error = err.Error()
		return result
	}
	replayStore := newBacktestReplayStore(store, cfg.UseExtendedHours)

	// ── 2. Build backtest exchange ──────────────────────────────────
	sourceExchange := futu.NewExchange("127.0.0.1:11110")

	// Ensure the backtest symbol's market info is available so the
	// backtest matching engine can be initialized for this symbol.
	// The futu exchange natively only knows HK.00700; arbitrary symbols
	// (e.g. US.TME) need a minimal Market record injected here.
	sourceExchange.EnsureMarket(cfg.Symbol)

	// bbgo caches QueryMarkets results to ~/.bbgo/cache/futu-markets.json
	// with a 24-hour TTL.  If a previous backtest ran with a different
	// symbol (e.g. HK.00700) the stale cache would exclude the current
	// symbol, causing the matching book to never be created.  Delete the
	// cache so LoadExchangeMarketsWithCache inside NewExchange calls
	// QueryMarkets fresh and picks up our EnsureMarket injection.
	if home, err := os.UserHomeDir(); err == nil {
		cacheFile := filepath.Join(home, ".bbgo", "cache", "futu-markets.json")
		_ = os.Remove(cacheFile)
	}

	// Determine quote currency from the symbol's market.
	quoteCurrency := cfg.QuoteCurrency
	if quoteCurrency == "" {
		switch {
		case strings.HasPrefix(strings.ToUpper(cfg.Symbol), "HK."):
			quoteCurrency = "HKD"
		case strings.HasPrefix(strings.ToUpper(cfg.Symbol), "US."):
			quoteCurrency = "USD"
		default:
			quoteCurrency = "HKD"
		}
	}
	result.QuoteCurrency = quoteCurrency

	backtestCfg := &bbgo2.Backtest{
		StartTime: types.LooseFormatTime(cfg.StartTime),
		EndTime:   (*types.LooseFormatTime)(&cfg.EndTime),
		Symbols:   []string{cfg.Symbol},
		Sessions:  []string{"futu"},
		Accounts: map[string]bbgo2.BacktestAccount{
			"futu": {
				MakerFeeRate: pineCommissionRate(compilation.Program.Metadata),
				TakerFeeRate: pineCommissionRate(compilation.Program.Metadata),
				Balances: bbgo2.BacktestAccountBalanceMap{
					quoteCurrency: fixedpoint.NewFromFloat(cfg.InitialBalance),
				},
			},
		},
	}

	btExchange, err := bt.NewExchange(
		sourceExchange.Name(),
		sourceExchange,
		replayStore,
		backtestCfg,
	)
	if err != nil {
		result.Error = fmt.Sprintf("create backtest exchange: %v", err)
		return result
	}

	// ── 3. Set up bbgo environment ──────────────────────────────────
	environ := bbgo2.NewEnvironment()
	session := environ.AddExchange("futu", btExchange)
	// session.MarketDataStream wired via btExchange.MarketDataStream below
	environ.SetStartTime(cfg.StartTime)

	// Populate session markets from the backtest exchange so that
	// Market() lookups (needed by order placement) succeed.
	if markets, err := btExchange.QueryMarkets(ctx); err == nil {
		session.SetMarkets(markets)
	}

	if err := environ.Init(ctx); err != nil {
		result.Error = fmt.Sprintf("init environment: %v", err)
		return result
	}

	// Enable trading on the session account AFTER Init, because
	// session.Init() calls session.setAccount() which replaces the
	// Account object. NewExchangeSession defaults CanTrade to false,
	// which causes runtime-side risk checks to block PLACE operations.
	session.Account.CanTrade = true
	session.Account.CanDeposit = true
	session.Account.CanWithdraw = true

	if err := btExchange.Prepare(&bbgo2.Config{Backtest: backtestCfg}); err != nil {
		if !isMissingPrepareKLineError(err) {
			log.Printf("backtest prepare warning: %v", err)
		}
	}
	btExchange.BindUserData(session.UserDataStream.(types.StandardStreamEmitter))

	// ── 4. Instantiate and run strategy runtime ─────────────────────
	// Wire before Subscribe so SubscribeMarketData sees strategy interval.
	btExchange.MarketDataStream = session.MarketDataStream.(types.StandardStreamEmitter)

	bbgo2.IsBackTesting = true
	defer func() { bbgo2.IsBackTesting = false }()

	type runnableStrategy interface {
		Subscribe(session *bbgo2.ExchangeSession)
		Run(ctx context.Context, orderExecutor bbgo2.OrderExecutor, session *bbgo2.ExchangeSession) error
	}

	strategy := &pineruntime.Strategy{
		Name:             "backtest-strategy",
		Symbol:           cfg.Symbol,
		Interval:         types.Interval(cfg.Interval),
		Script:           cfg.StrategyScript,
		Program:          compilation.Program,
		Requirements:     &compilation.Requirements,
		UseExtendedHours: cfg.UseExtendedHours != nil && *cfg.UseExtendedHours,
		WarmupUntil:      warmupUntil,
		OnError: func(errMsg string) {
			result.AddRuntimeError(errMsg)
			log.Printf("backtest runtime error: %s", errMsg)
		},
	}
	executor := bbgo2.OrderExecutor(session.OrderExecutor)
	if compilation.Program.Metadata.Slippage > 0 {
		slippageExecutor := newBacktestSlippageExecutor(
			session.OrderExecutor,
			session,
			compilation.Program.Metadata.Slippage,
		)
		session.MarketDataStream.OnKLineClosed(slippageExecutor.onKLineClosed)
		executor = slippageExecutor
	}
	strategy.Subscribe(session)

	// Sync session subscriptions to MarketDataStream so that
	// SubscribeMarketData can discover the strategy's kline interval.
	// In live mode environ.Connect() does this; in backtest we must do
	// it manually before InitializeExchangeSources.
	for _, sub := range session.Subscriptions {
		session.MarketDataStream.Subscribe(sub.Channel, sub.Symbol, sub.Options)
	}

	if session.OrderExecutor == nil {
		result.Error = "session.OrderExecutor is nil"
		return result
	}
	log.Printf("backtest: strategy starting, executor=%T symbol=%s interval=%s", session.OrderExecutor, cfg.Symbol, cfg.Interval)
	if err := strategy.Run(ctx, executor, session); err != nil {
		result.Error = fmt.Sprintf("strategy run: %v", err)
		return result
	}

	// ── 5. K-line replay pump ───────────────────────────────────────
	// Use the strategy's interval as the required kline interval.
	// bbgo's backtest engine normally requires 1m klines for accurate
	// order matching, but if 1m data is unavailable in the store the
	// ConsumeKLine pump panics when it receives higher-interval klines
	// (e.g. 5m) without interleaved 1m bars.  Using the strategy
	// interval avoids this while still delivering correct matching at
	// the strategy's own granularity.
	strategyInterval := types.Interval(cfg.Interval)
	collector := newResultCollector(cfg.Symbol, strategyInterval, quoteCurrency, warmupUntil, result)
	if estimatedBars := estimateReplayBarCapacity(warmupUntil, cfg.EndTime, strategyInterval); estimatedBars > 0 {
		collector.candles = make([]Candle, 0, estimatedBars)
		collector.pnlCurve = make([]PnLPoint, 0, estimatedBars)
	}

	session.UserDataStream.OnOrderUpdate(collector.onOrderUpdate)
	bindCashCommission(session, quoteCurrency, compilation.Program.Metadata)

	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		collector.onKLineClosed(ctx, btExchange, kline)
	})

	if streamer, ok := replayStore.(klineRangeStreamer); ok {
		btExchange.Src = &bt.ExchangeDataSource{Exchange: btExchange, Session: session}
		if err := streamer.StreamKLines(queryStartTime, cfg.EndTime, btExchange, []string{cfg.Symbol}, []types.Interval{strategyInterval}, func(kline types.KLine) {
			btExchange.ConsumeKLine(kline, strategyInterval)
		}); err != nil {
			result.Error = fmt.Sprintf("stream replay klines: %v", err)
			return result
		}
		if err := btExchange.CloseMarketData(); err != nil {
			log.Printf("backtest close market data: %v", err)
		}

		totalOrders, filledOrders := collector.finalize(ctx, btExchange, cfg.InitialBalance)
		log.Printf("backtest: done totalOrders=%d filledOrders=%d finalBalance=%.2f", totalOrders, filledOrders, result.FinalBalance)
		return result
	}

	exchangeSources, err := bt.InitializeExchangeSources(
		environ.Sessions(),
		queryStartTime, cfg.EndTime,
		backtestCfg.Symbols,
		strategyInterval,
	)
	if err != nil {
		result.Error = fmt.Sprintf("initialize exchange sources: %v", err)
		return result
	}

	// Pump klines one by one. Each call to ConsumeKLine drives the
	// matching engine: orders are matched against the candle's OHLCV,
	// trade/order updates flow to the user data stream, and after the
	// matching the kline is emitted via MarketDataStream.EmitKLineClosed
	// — which triggers the strategy's onKLineClosed hook.
	if len(exchangeSources) == 1 {
		exSource := exchangeSources[0]
		for k := range exSource.C {
			exSource.Exchange.ConsumeKLine(k, strategyInterval)
		}
		if err := exSource.Exchange.CloseMarketData(); err != nil {
			log.Printf("backtest close market data: %v", err)
		}
	} else {
		// Multi-exchange path (not expected for single-session backtests).
		result.Error = "unexpected multi-exchange backtest configuration"
		return result
	}

	totalOrders, filledOrders := collector.finalize(ctx, btExchange, cfg.InitialBalance)
	log.Printf("backtest: done totalOrders=%d filledOrders=%d finalBalance=%.2f", totalOrders, filledOrders, result.FinalBalance)

	return result
}

func isMissingPrepareKLineError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "no kline data found for symbol") &&
		strings.Contains(message, "1m before start time")
}

func resolvePineInitialBalance(requested float64, metadata strategyir.StrategyMetadata) float64 {
	if requested > 0 {
		return requested
	}
	if metadata.InitialCapital > 0 {
		return metadata.InitialCapital
	}
	return 100000
}

func deriveStrategyWarmupCandles(script string, interval types.Interval, symbol string, useExtendedHours *bool) (int, error) {
	return indicatorruntime.WarmupBarsFromScriptForSymbolWithOptions(
		script,
		interval,
		symbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: useExtendedHours != nil && *useExtendedHours},
	)
}

func resolveBacktestReadSessionScope(useExtendedHours *bool) string {
	if useExtendedHours == nil {
		return "auto"
	}
	if *useExtendedHours {
		return "extended"
	}
	return "regular"
}

func estimateReplayBarCapacity(start, end time.Time, interval types.Interval) int {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return 0
	}
	intervalDuration := interval.Duration()
	if intervalDuration <= 0 {
		return 0
	}
	return int(end.Sub(start)/intervalDuration) + 1
}
