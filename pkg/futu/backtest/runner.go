// Package backtest provides a standalone backtest runner for Futu/QuickJS
// strategies using bbgo's backtest engine with a local SQLite K-line store.
package backtest

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bt "github.com/c9s/bbgo/pkg/backtest"
	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/strategy/quickjs"
)

// RunConfig describes a single backtest run.
type RunConfig struct {
	DBPath         string    `json:"dbPath"`
	Symbol         string    `json:"symbol"`
	Interval       string    `json:"interval"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime"`
	StrategyScript string    `json:"strategyScript"`
	InitialBalance float64   `json:"initialBalance"`
	WarmupCandles  int       `json:"warmupCandles"` // extra candles to load before StartTime
	QuoteCurrency  string    `json:"quoteCurrency"` // e.g. HKD, USD — auto-detected if empty
	RehabType      string    `json:"rehabType"`     // "forward" | "backward" | "none"
}

// TradeEvent is a single filled trade for chart rendering.
type TradeEvent struct {
	Time  string  `json:"time"`
	Side  string  `json:"side"`
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
	PnL   float64 `json:"pnl,omitempty"`
}

// OrderBookEntry captures a single backtest order and its latest fill outcome.
// A submitted order that later fills is merged into the same row.
type OrderBookEntry struct {
	OrderID        string  `json:"orderId"`
	ClientOrderID  string  `json:"clientOrderId,omitempty"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	Quantity       float64 `json:"quantity"`
	OrderType      string  `json:"orderType,omitempty"`
	OrderPrice     float64 `json:"orderPrice,omitempty"`
	SubmittedAt    string  `json:"submittedAt,omitempty"`
	Status         string  `json:"status"`
	FilledQuantity float64 `json:"filledQuantity,omitempty"`
	FilledPrice    float64 `json:"filledPrice,omitempty"`
	FilledAt       string  `json:"filledAt,omitempty"`
}

// PnLPoint is a single point on the equity curve.
type PnLPoint struct {
	Time   string  `json:"time"`
	Equity float64 `json:"equity"`
}

// Candle is a single OHLCV bar for chart rendering.
type Candle struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// RunResult holds the output of a backtest run.
type RunResult struct {
	Symbol        string           `json:"symbol"`
	Interval      string           `json:"interval"`
	StartTime     string           `json:"startTime"`
	EndTime       string           `json:"endTime"`
	QuoteCurrency string           `json:"quoteCurrency"`
	FinalBalance  float64          `json:"finalBalance"`
	PnL           float64          `json:"pnl"`
	TotalTrades   int              `json:"totalTrades"`
	WinRate       float64          `json:"winRate"`
	Trades        []TradeEvent     `json:"trades,omitempty"`
	OrderBook     []OrderBookEntry `json:"orderBook,omitempty"`
	PnLCurve      []PnLPoint       `json:"pnlCurve,omitempty"`
	Candles       []Candle         `json:"candles,omitempty"`
	Logs          []string         `json:"logs"`
	Error         string           `json:"error,omitempty"`

	mu            sync.Mutex
	RuntimeErrors []string `json:"runtimeErrors,omitempty"`
}

// Run executes a backtest with the given configuration.
//
// It opens the SQLite K-line store, creates a bbgo backtest.Exchange,
// instantiates a QuickJS strategy, and replays historical K-lines one by one
// through bbgo's matching engine. The strategy's onKLineClosed hook receives
// each candle and may place/cancel orders via the host API.
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

	// Validate that we have data.
	klines, err := store.QueryKLinesBackward(nil, cfg.Symbol, types.Interval(cfg.Interval), cfg.EndTime, 1)
	if err != nil || len(klines) == 0 {
		result.Error = fmt.Sprintf("no K-line data for %s %s (run 'jftrade kline-sync' first)", cfg.Symbol, cfg.Interval)
		return result
	}

	// ── Warmup ──────────────────────────────────────────────────────
	// Extend the kline query start time backward so indicators have
	// enough history before the actual backtest start.
	warmupUntil := cfg.StartTime
	queryStartTime := cfg.StartTime
	if cfg.WarmupCandles > 0 {
		intervalDur := types.Interval(cfg.Interval).Duration()
		warmupDur := intervalDur * time.Duration(cfg.WarmupCandles)
		queryStartTime = cfg.StartTime.Add(-warmupDur)
		log.Printf("backtest: warmup %d candles (%v), query from %s",
			cfg.WarmupCandles, warmupDur, queryStartTime.Format(time.RFC3339))
	}

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
				Balances: bbgo2.BacktestAccountBalanceMap{
					quoteCurrency: fixedpoint.NewFromFloat(cfg.InitialBalance),
				},
			},
		},
	}

	btExchange, err := bt.NewExchange(
		sourceExchange.Name(),
		sourceExchange,
		store,
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
	// Account object.  NewExchangeSession defaults CanTrade to false,
	// which causes the QuickJS risk check to block PLACE operations.
	session.Account.CanTrade = true
	session.Account.CanDeposit = true
	session.Account.CanWithdraw = true

	if err := btExchange.Prepare(&bbgo2.Config{Backtest: backtestCfg}); err != nil {
		log.Printf("backtest prepare warning: %v", err)
	}
	btExchange.BindUserData(session.UserDataStream.(types.StandardStreamEmitter))

	// ── 4. Instantiate and run QuickJS strategy ─────────────────────
	// Wire before Subscribe so SubscribeMarketData sees strategy interval.
	btExchange.MarketDataStream = session.MarketDataStream.(types.StandardStreamEmitter)

	bbgo2.IsBackTesting = true
	defer func() { bbgo2.IsBackTesting = false }()

	strategy := &quickjs.Strategy{
		Name:        "backtest-strategy",
		Symbol:      cfg.Symbol,
		Interval:    types.Interval(cfg.Interval),
		Script:      cfg.StrategyScript,
		WarmupUntil: warmupUntil,
		// Collect runtime errors (e.g. insufficient balance) so the
		// frontend can surface them alongside the backtest result.
		OnError: func(errMsg string) {
			result.addRuntimeError(errMsg)
			log.Printf("backtest runtime error: %s", errMsg)
		},
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
	if err := strategy.Run(ctx, session.OrderExecutor, session); err != nil {
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

	collector := newResultCollector(cfg.Symbol, strategyInterval, quoteCurrency, warmupUntil, result)

	session.UserDataStream.OnOrderUpdate(collector.onOrderUpdate)

	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		collector.onKLineClosed(ctx, btExchange, kline)
	})

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
