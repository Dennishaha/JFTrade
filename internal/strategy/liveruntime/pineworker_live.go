package liveruntime

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/pineruntime"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/chart"
	strategyindicatorwarmup "github.com/jftrade/jftrade-main/pkg/strategy/indicatorwarmup"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type PineWorker interface {
	RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
}

type pineWorkerLive struct {
	runner          PineWorker
	instance        stratsrv.ManagedInstance
	symbol          string
	interval        bbgotypes.Interval
	source          string
	sizer           *strategyRuntimeLiveSizer
	commandExecutor *stratsrv.LiveCommandExecutor

	mu      sync.Mutex
	candles []pineworker.Candle
	session pineruntime.LiveSession
}

func newStrategyRuntimePineWorkerLive(
	runner PineWorker,
	instance stratsrv.ManagedInstance,
	symbol string,
	interval bbgotypes.Interval,
	source string,
	executor bbgo.OrderExecutor,
	symbolRuntime *symbolRuntime,
	recordWarning func(string),
) (*pineWorkerLive, error) {
	if runner == nil {
		return nil, fmt.Errorf("pine worker manager is required for live strategy runtime")
	}
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("pine worker live strategy source is required")
	}
	if executor == nil {
		return nil, fmt.Errorf("pine worker live order executor is required")
	}
	if symbolRuntime == nil {
		return nil, fmt.Errorf("pine worker live symbol runtime is required")
	}
	liveSizer := &strategyRuntimeLiveSizer{runner: symbolRuntime}
	live := &pineWorkerLive{
		runner:   runner,
		instance: instance,
		symbol:   symbol,
		interval: interval,
		source:   source,
		sizer:    liveSizer,
	}
	live.commandExecutor = stratsrv.NewLiveCommandExecutor(stratsrv.LiveCommandExecutorOptions{
		Symbol:              symbol,
		OrderExecutor:       executor,
		MarketResolver:      strategyRuntimeLiveMarketResolver{market: symbolRuntime.market},
		PositionSizer:       liveSizer,
		WarningSink:         strategyRuntimeLiveWarningSink{record: recordWarning},
		ClientOrderIDPrefix: fmt.Sprintf("strategy-live-%s", instance.ID),
	})
	return live, nil
}

func (live *pineWorkerLive) loadWarmup(ctx context.Context, source MarketDataSource) ([]bbgotypes.KLine, error) {
	warmupBars, err := strategyindicatorwarmup.WarmupBarsFromScriptForSymbol(live.source, live.interval, live.symbol)
	if err != nil {
		return nil, fmt.Errorf("analyze strategy warmup for %s: %w", live.symbol, err)
	}
	queryLimit := strategyRuntimeMaxInt(warmupBars+2, 2)
	klines, err := source.QueryKLines(ctx, live.symbol, live.interval, bbgotypes.KLineQueryOptions{Limit: queryLimit})
	if err != nil {
		return nil, fmt.Errorf("load warmup klines for %s: %w", live.symbol, err)
	}
	return klines, nil
}

func (live *pineWorkerLive) recordWarmupClosed(closed bbgotypes.KLine) {
	live.mu.Lock()
	defer live.mu.Unlock()
	live.candles = append(live.candles, stratsrv.CandleFromKLine(closed))
	live.sizer.onKLineClosed(closed)
}

func (live *pineWorkerLive) onClosedKLine(ctx context.Context, closed bbgotypes.KLine) error {
	live.mu.Lock()
	defer live.mu.Unlock()
	candle := stratsrv.CandleFromKLine(closed)
	live.candles = append(live.candles, candle)
	live.sizer.onKLineClosed(closed)
	currentBarIndex := len(live.candles) - 1
	request := live.requestLocked()
	var response pineworker.RunScriptResponse
	var err error
	if live.session != nil {
		request.Candles = []pineworker.Candle{candle}
		response, err = live.session.Append(ctx, request)
	} else {
		response, err = live.runner.RunScript(ctx, request)
	}
	if err != nil {
		return fmt.Errorf("run live pine worker for %s: %w", live.symbol, err)
	}
	commands, err := stratsrv.CommandsFromOrderIntents(
		strategyRuntimeCurrentBarIntents(response.OrderIntents, currentBarIndex, closed.StartTime.Time()),
	)
	if err != nil {
		return fmt.Errorf("map live pine worker intents for %s: %w", live.symbol, err)
	}
	return live.commandExecutor.ExecuteBarCommands(ctx, commands)
}

func (live *pineWorkerLive) openSession(ctx context.Context) error {
	opener, ok := live.runner.(pineruntime.LiveSessionOpener)
	if !ok {
		return nil
	}
	live.mu.Lock()
	defer live.mu.Unlock()
	if live.session != nil {
		return nil
	}
	request := live.requestLocked()
	request.SessionID = fmt.Sprintf("strategy:%s:%s", live.instance.ID, live.symbol)
	session, response, err := opener.OpenLiveSession(ctx, request)
	if err != nil {
		return fmt.Errorf("open stateful live pine worker for %s: %w", live.symbol, err)
	}
	if session == nil || response.SessionRevision != 1 {
		if session != nil {
			_ = session.Close(context.Background())
		}
		return fmt.Errorf("open stateful live pine worker for %s returned an invalid session", live.symbol)
	}
	live.session = session
	return nil
}

func (live *pineWorkerLive) closeSession(ctx context.Context) error {
	if live == nil {
		return nil
	}
	live.mu.Lock()
	session := live.session
	live.session = nil
	live.mu.Unlock()
	if session == nil {
		return nil
	}
	return session.Close(ctx)
}

func (live *pineWorkerLive) requestLocked() pineworker.RunScriptRequest {
	return pineworker.RunScriptRequest{
		JobID:     fmt.Sprintf("live:%s:%s:%d", live.instance.ID, live.symbol, time.Now().UnixNano()),
		ScriptID:  strategyRuntimeDefinitionID(live.instance),
		Source:    live.source,
		Symbol:    live.symbol,
		Timeframe: string(live.interval),
		ChartType: string(chart.NormalizeChartType(string(live.instance.Binding.ChartType))),
		Mode:      pineworker.ModeLive,
		Candles:   append([]pineworker.Candle(nil), live.candles...),
		Params:    PineWorkerParams(live.instance),
	}
}

type strategyRuntimeLiveMarketResolver struct {
	market bbgotypes.Market
}

func (resolver strategyRuntimeLiveMarketResolver) Market(symbol string) (bbgotypes.Market, bool) {
	if strings.EqualFold(strings.TrimSpace(symbol), strings.TrimSpace(resolver.market.Symbol)) {
		return resolver.market, true
	}
	return bbgotypes.Market{}, false
}

type strategyRuntimeLiveSizer struct {
	runner *symbolRuntime

	mu        sync.RWMutex
	lastPrice fixedpoint.Value
}

func (sizer *strategyRuntimeLiveSizer) onKLineClosed(kline bbgotypes.KLine) {
	if sizer == nil {
		return
	}
	if sizer.runner != nil && !strings.EqualFold(strings.TrimSpace(kline.Symbol), strings.TrimSpace(sizer.runner.symbol)) {
		return
	}
	sizer.mu.Lock()
	sizer.lastPrice = kline.Close
	sizer.mu.Unlock()
}

func (sizer *strategyRuntimeLiveSizer) QuantityForCommand(
	command stratsrv.WorkerOrderCommand,
	market bbgotypes.Market,
) (fixedpoint.Value, error) {
	if sizer == nil || sizer.runner == nil {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires live position sizing", command.ID)
	}
	if math.IsNaN(command.QuantityPct) || math.IsInf(command.QuantityPct, 0) || command.QuantityPct <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct must be positive", command.ID)
	}
	percent := fixedpoint.NewFromFloat(command.QuantityPct / 100)
	switch normalizeStrategyRuntimeWorkerIntentKind(command.Kind) {
	case "entry", "order":
		return sizer.entryQuantity(command, market, percent)
	case "exit", "close", "close_all":
		return sizer.closeQuantity(command, percent)
	default:
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s does not support quantity pct", command.ID)
	}
}

func (sizer *strategyRuntimeLiveSizer) NetPosition() fixedpoint.Value {
	if sizer == nil || sizer.runner == nil {
		return fixedpoint.Zero
	}
	quantity := 0.0
	for _, position := range sizer.runner.brokerPositionsSnapshot() {
		if strategyRuntimePositionMatchesSymbol(position, sizer.runner.symbol) {
			quantity += position.Quantity
		}
	}
	return fixedpoint.NewFromFloat(quantity)
}

func (sizer *strategyRuntimeLiveSizer) entryQuantity(
	command stratsrv.WorkerOrderCommand,
	market bbgotypes.Market,
	percent fixedpoint.Value,
) (fixedpoint.Value, error) {
	price := sizer.priceForCommand(command, market)
	if price.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires a positive price", command.ID)
	}
	equity, err := sizer.equity(market)
	if err != nil {
		return fixedpoint.Zero, err
	}
	if equity.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires positive equity", command.ID)
	}
	return equity.Mul(percent).Div(price), nil
}

func (sizer *strategyRuntimeLiveSizer) closeQuantity(
	command stratsrv.WorkerOrderCommand,
	percent fixedpoint.Value,
) (fixedpoint.Value, error) {
	position := sizer.NetPosition().Abs()
	if position.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires an open position", command.ID)
	}
	quantity := position.Mul(percent)
	if quantity.Compare(position) > 0 {
		quantity = position
	}
	return quantity, nil
}

func (sizer *strategyRuntimeLiveSizer) equity(market bbgotypes.Market) (fixedpoint.Value, error) {
	account := sizer.runner.brokerAccountSnapshot()
	if account == nil {
		return fixedpoint.Zero, fmt.Errorf("pine worker quantity pct account is required")
	}
	if account.TotalAccountValue.Sign() > 0 {
		return account.TotalAccountValue, nil
	}
	quoteCurrency := strings.TrimSpace(market.QuoteCurrency)
	if quoteCurrency == "" {
		return fixedpoint.Zero, fmt.Errorf("pine worker quantity pct quote currency is required")
	}
	balance, _ := account.Balance(quoteCurrency)
	return balance.Total(), nil
}

func (sizer *strategyRuntimeLiveSizer) priceForCommand(
	command stratsrv.WorkerOrderCommand,
	market bbgotypes.Market,
) fixedpoint.Value {
	if command.LimitPrice > 0 {
		return fixedpoint.NewFromFloat(command.LimitPrice)
	}
	if command.StopPrice > 0 {
		return fixedpoint.NewFromFloat(command.StopPrice)
	}
	sizer.mu.RLock()
	price := sizer.lastPrice
	sizer.mu.RUnlock()
	if price.Sign() <= 0 {
		price = fixedpoint.NewFromFloat(sizer.runner.currentPrice())
	}
	if !market.TickSize.IsZero() && price.Sign() > 0 {
		return market.TruncatePrice(price)
	}
	return price
}

type strategyRuntimeLiveWarningSink struct {
	record func(string)
}

func (sink strategyRuntimeLiveWarningSink) AddIgnoredOrderWarning(message string) {
	if sink.record != nil && strings.TrimSpace(message) != "" {
		sink.record(message)
	}
}

func (sink strategyRuntimeLiveWarningSink) AddIgnoredOrderWarningGroup(_ string, message string) {
	sink.AddIgnoredOrderWarning(message)
}

func normalizeStrategyRuntimeWorkerIntentKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func strategyRuntimeCurrentBarIntents(intents []pineworker.OrderIntent, barIndex int, openTime time.Time) []pineworker.OrderIntent {
	filtered := make([]pineworker.OrderIntent, 0, len(intents))
	openMillis := openTime.UTC().UnixMilli()
	for _, intent := range intents {
		if intent.BarIndex == barIndex || (intent.Time > 0 && intent.Time == openMillis) {
			filtered = append(filtered, intent)
		}
	}
	return filtered
}

func PineWorkerParams(instance stratsrv.ManagedInstance) map[string]string {
	params := make(map[string]string, len(instance.Params))
	for key, value := range instance.Params {
		switch typed := value.(type) {
		case string:
			params[key] = typed
		case fmt.Stringer:
			params[key] = typed.String()
		case float64, float32, int, int64, int32, bool:
			params[key] = fmt.Sprint(typed)
		}
	}
	return params
}
