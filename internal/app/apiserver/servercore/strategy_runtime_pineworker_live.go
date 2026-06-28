package servercore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	strategyindicatorruntime "github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type strategyRuntimePineWorker interface {
	RunScript(context.Context, pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error)
}

type strategyRuntimePineWorkerLive struct {
	runner   strategyRuntimePineWorker
	instance managedStrategyInstance
	symbol   string
	interval bbgotypes.Interval
	source   string
	executor bbgo.OrderExecutor

	mu      sync.Mutex
	candles []pineworker.Candle
}

func newStrategyRuntimePineWorkerLive(
	runner strategyRuntimePineWorker,
	instance managedStrategyInstance,
	symbol string,
	interval bbgotypes.Interval,
	source string,
	executor bbgo.OrderExecutor,
) (*strategyRuntimePineWorkerLive, error) {
	if runner == nil {
		return nil, fmt.Errorf("pine worker manager is required for live strategy runtime")
	}
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("pine worker live strategy source is required")
	}
	if executor == nil {
		return nil, fmt.Errorf("pine worker live order executor is required")
	}
	return &strategyRuntimePineWorkerLive{
		runner:   runner,
		instance: instance,
		symbol:   symbol,
		interval: interval,
		source:   source,
		executor: executor,
	}, nil
}

func (live *strategyRuntimePineWorkerLive) loadWarmup(ctx context.Context, exchange strategyRuntimeExchange) ([]bbgotypes.KLine, error) {
	warmupBars, err := strategyindicatorruntime.WarmupBarsFromScriptForSymbol(live.source, live.interval, live.symbol)
	if err != nil {
		return nil, fmt.Errorf("analyze strategy warmup for %s: %w", live.symbol, err)
	}
	queryLimit := strategyRuntimeMaxInt(warmupBars+2, 2)
	klines, err := exchange.QueryKLines(ctx, live.symbol, live.interval, bbgotypes.KLineQueryOptions{Limit: queryLimit})
	if err != nil {
		return nil, fmt.Errorf("load warmup klines for %s: %w", live.symbol, err)
	}
	return klines, nil
}

func (live *strategyRuntimePineWorkerLive) recordWarmupClosed(closed bbgotypes.KLine) {
	live.mu.Lock()
	defer live.mu.Unlock()
	live.candles = append(live.candles, strategyRuntimePineWorkerCandle(closed))
}

func (live *strategyRuntimePineWorkerLive) onClosedKLine(ctx context.Context, closed bbgotypes.KLine) error {
	live.mu.Lock()
	defer live.mu.Unlock()
	live.candles = append(live.candles, strategyRuntimePineWorkerCandle(closed))
	request := live.requestLocked()
	response, err := live.runner.RunScript(ctx, request)
	if err != nil {
		return fmt.Errorf("run live pine worker for %s: %w", live.symbol, err)
	}
	commands, err := bt.CommandsFromOrderIntents(strategyRuntimeCurrentBarIntents(response.OrderIntents, len(request.Candles)-1, closed.StartTime.Time()))
	if err != nil {
		return fmt.Errorf("map live pine worker intents for %s: %w", live.symbol, err)
	}
	for _, command := range commands {
		if err := live.executeCommand(ctx, command); err != nil {
			return err
		}
	}
	return nil
}

func (live *strategyRuntimePineWorkerLive) requestLocked() pineworker.RunScriptRequest {
	return pineworker.RunScriptRequest{
		JobID:     fmt.Sprintf("live:%s:%s:%d", live.instance.ID, live.symbol, time.Now().UnixNano()),
		ScriptID:  strategyRuntimeDefinitionID(live.instance),
		Source:    live.source,
		Symbol:    live.symbol,
		Timeframe: string(live.interval),
		Mode:      pineworker.ModeLive,
		Candles:   append([]pineworker.Candle(nil), live.candles...),
		Params:    strategyRuntimePineWorkerParams(live.instance),
	}
}

func (live *strategyRuntimePineWorkerLive) executeCommand(ctx context.Context, command bt.WorkerOrderCommand) error {
	switch strings.TrimSpace(strings.ToLower(command.Kind)) {
	case "entry", "order", "exit", "close", "close_all":
		order, err := live.submitOrderFromCommand(command)
		if err != nil {
			return err
		}
		_, err = live.executor.SubmitOrders(ctx, order)
		return err
	case "cancel", "cancel_all":
		return nil
	default:
		return fmt.Errorf("unsupported live pine worker command kind: %s", command.Kind)
	}
}

func (live *strategyRuntimePineWorkerLive) submitOrderFromCommand(command bt.WorkerOrderCommand) (bbgotypes.SubmitOrder, error) {
	if command.Side == "" {
		return bbgotypes.SubmitOrder{}, fmt.Errorf("pine worker command %s side is required", command.Kind)
	}
	if command.QuantityPct > 0 {
		return bbgotypes.SubmitOrder{}, fmt.Errorf("pine worker command %s quantity pct requires worker-side sizing for live runtime", command.ID)
	}
	if command.Quantity <= 0 {
		return bbgotypes.SubmitOrder{}, fmt.Errorf("pine worker command %s quantity must be positive", command.ID)
	}
	orderType := command.OrderType
	if orderType == "" {
		orderType = bbgotypes.OrderTypeMarket
	}
	order := bbgotypes.SubmitOrder{
		ClientOrderID: command.ID,
		Symbol:        live.symbol,
		Side:          command.Side,
		Type:          orderType,
		Quantity:      fixedpoint.NewFromFloat(command.Quantity),
	}
	if command.LimitPrice > 0 {
		order.Price = fixedpoint.NewFromFloat(command.LimitPrice)
		order.TimeInForce = bbgotypes.TimeInForceGTC
	}
	if command.StopPrice > 0 {
		order.StopPrice = fixedpoint.NewFromFloat(command.StopPrice)
	}
	return order, nil
}

func strategyRuntimePineWorkerCandle(kline bbgotypes.KLine) pineworker.Candle {
	return pineworker.Candle{
		OpenTime:  kline.StartTime.Time().UnixMilli(),
		CloseTime: kline.EndTime.Time().UnixMilli(),
		Open:      kline.Open.Float64(),
		High:      kline.High.Float64(),
		Low:       kline.Low.Float64(),
		Close:     kline.Close.Float64(),
		Volume:    kline.Volume.Float64(),
	}
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

func strategyRuntimePineWorkerParams(instance managedStrategyInstance) map[string]string {
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
