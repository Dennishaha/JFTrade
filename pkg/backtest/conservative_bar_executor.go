package backtest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

const (
	conservativeBarInitialOrderID = uint64(1_100_000_000)
	conservativeBarInitialTradeID = uint64(1_200_000_000)
)

type conservativeBarWarningSink interface {
	AddWarning(string)
}

type conservativeBarExecutorOptions struct {
	ProcessOrdersOnClose bool
	SlippageTicks        int
	WarningSink          conservativeBarWarningSink
}

type conservativeBarExecutor struct {
	account *types.Account
	stream  types.StandardStreamEmitter
	options conservativeBarExecutorOptions

	mu                     sync.Mutex
	nextOrderID            uint64
	nextTradeID            uint64
	pending                []*conservativeBarPendingOrder
	currentBar             types.KLine
	currentBarReady        bool
	currentBarBudget       fixedpoint.Value
	currentBarBudgetSymbol string
	warned                 map[string]struct{}
}

type conservativeBarPendingOrder struct {
	order          types.Order
	remaining      fixedpoint.Value
	filled         fixedpoint.Value
	filledNotional fixedpoint.Value
	stopTriggered  bool
}

func newConservativeBarExecutor(account *types.Account, stream types.StandardStreamEmitter, options conservativeBarExecutorOptions) *conservativeBarExecutor {
	return &conservativeBarExecutor{
		account:     account,
		stream:      stream,
		options:     options,
		nextOrderID: conservativeBarInitialOrderID,
		nextTradeID: conservativeBarInitialTradeID,
		warned:      map[string]struct{}{},
	}
}

func (executor *conservativeBarExecutor) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	if executor == nil {
		return nil, fmt.Errorf("conservative bar executor is required")
	}
	if executor.account == nil {
		return nil, fmt.Errorf("conservative bar executor account is required")
	}
	if executor.stream == nil {
		return nil, fmt.Errorf("conservative bar executor stream is required")
	}

	executor.mu.Lock()
	defer executor.mu.Unlock()

	created := make(types.OrderSlice, 0, len(orders))
	for _, submit := range orders {
		order := executor.newOrderLocked(submit)
		pending := &conservativeBarPendingOrder{
			order:     order,
			remaining: order.Quantity,
		}
		executor.pending = append(executor.pending, pending)
		executor.stream.EmitOrderUpdate(order)
		created = append(created, order)

		if executor.options.ProcessOrdersOnClose && executor.currentBarReady && executor.currentBar.Symbol == order.Symbol {
			executor.fillPendingOrderLocked(pending, executor.currentBar, conservativeBarClosePoint)
			executor.compactPendingLocked()
		}
	}
	return created, nil
}

func (executor *conservativeBarExecutor) CancelOrders(ctx context.Context, orders ...types.Order) error {
	if executor == nil {
		return fmt.Errorf("conservative bar executor is required")
	}
	if executor.stream == nil {
		return fmt.Errorf("conservative bar executor stream is required")
	}

	executor.mu.Lock()
	defer executor.mu.Unlock()

	eventTime := executor.eventTimeLocked()
	for _, requested := range orders {
		for _, pending := range executor.pending {
			if pending == nil || pending.remaining.Sign() <= 0 || !sameConservativeBarOrder(pending.order, requested) {
				continue
			}
			pending.remaining = fixedpoint.Zero
			pending.order.Status = types.OrderStatusCanceled
			pending.order.IsWorking = false
			pending.order.UpdateTime = types.Time(eventTime)
			executor.stream.EmitOrderUpdate(pending.order)
			break
		}
	}
	executor.compactPendingLocked()
	return nil
}

func (executor *conservativeBarExecutor) onKLineClosed(kline types.KLine) {
	if executor == nil {
		return
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()

	executor.currentBar = kline
	executor.currentBarReady = true
	executor.currentBarBudget = conservativeBarLiquidityBudget(kline)
	executor.currentBarBudgetSymbol = kline.Symbol
	if kline.Volume.Sign() <= 0 && executor.hasPendingSymbolLocked(kline.Symbol) {
		executor.warnOnceLocked(
			"zero-volume|"+kline.Symbol,
			fmt.Sprintf("conservative-bar-v1: %s bar ending %s has no positive volume; pending orders cannot fill on this bar", kline.Symbol, kline.EndTime.Time().UTC().Format(time.RFC3339Nano)),
		)
		return
	}

	for _, pending := range executor.pending {
		if pending == nil || pending.order.Symbol != kline.Symbol {
			continue
		}
		executor.fillPendingOrderLocked(pending, kline, conservativeBarFullBar)
		if executor.currentBarBudget.Sign() <= 0 {
			break
		}
	}
	executor.compactPendingLocked()
}

func (executor *conservativeBarExecutor) newOrderLocked(submit types.SubmitOrder) types.Order {
	if submit.Type == "" {
		submit.Type = types.OrderTypeMarket
	}
	executor.nextOrderID++
	eventTime := executor.eventTimeLocked()
	return types.Order{
		SubmitOrder:  submit,
		Exchange:     types.ExchangeBacktest,
		OrderID:      executor.nextOrderID,
		Status:       types.OrderStatusNew,
		CreationTime: types.Time(eventTime),
		UpdateTime:   types.Time(eventTime),
		IsWorking:    true,
		IsMargin:     true,
	}
}

type conservativeBarMatchMode int

const (
	conservativeBarFullBar conservativeBarMatchMode = iota
	conservativeBarClosePoint
)

func (executor *conservativeBarExecutor) fillPendingOrderLocked(
	pending *conservativeBarPendingOrder,
	kline types.KLine,
	mode conservativeBarMatchMode,
) {
	if pending == nil || pending.remaining.Sign() <= 0 {
		return
	}
	if executor.currentBarBudgetSymbol != kline.Symbol {
		executor.currentBarBudget = conservativeBarLiquidityBudget(kline)
		executor.currentBarBudgetSymbol = kline.Symbol
	}
	if executor.currentBarBudget.Sign() <= 0 {
		return
	}

	price, ok := executor.matchPriceLocked(pending, kline, mode)
	if !ok || price.Sign() <= 0 {
		return
	}
	quantity := pending.remaining
	if quantity.Compare(executor.currentBarBudget) > 0 {
		quantity = executor.currentBarBudget
	}
	quantity = normalizePineWorkerOrderQuantity(pending.order.Market, quantity)
	if quantity.Sign() <= 0 {
		executor.warnOnceLocked(
			"liquidity-step|"+pending.order.Symbol,
			fmt.Sprintf("conservative-bar-v1: liquidity budget for %s is below tradable quantity step; order %s remains pending", pending.order.Symbol, pending.order.ClientOrderID),
		)
		return
	}
	if pending.order.Market.MinQuantity.Sign() > 0 && quantity.Compare(pending.order.Market.MinQuantity) < 0 {
		executor.warnOnceLocked(
			"liquidity-min|"+pending.order.Symbol,
			fmt.Sprintf("conservative-bar-v1: liquidity budget for %s is below min quantity %s; order %s remains pending", pending.order.Symbol, pending.order.Market.MinQuantity.String(), pending.order.ClientOrderID),
		)
		return
	}

	executor.applyFillLocked(pending, quantity, price, conservativeBarEventTime(kline, mode))
	executor.currentBarBudget = executor.currentBarBudget.Sub(quantity)
}

func (executor *conservativeBarExecutor) applyFillLocked(
	pending *conservativeBarPendingOrder,
	quantity fixedpoint.Value,
	price fixedpoint.Value,
	eventTime time.Time,
) {
	quoteQuantity := quantity.Mul(price)
	switch pending.order.Side {
	case types.SideTypeBuy:
		executor.account.AddBalance(pending.order.Market.QuoteCurrency, quoteQuantity.Neg())
		executor.account.AddBalance(pending.order.Market.BaseCurrency, quantity)
	case types.SideTypeSell:
		executor.account.AddBalance(pending.order.Market.QuoteCurrency, quoteQuantity)
		executor.account.AddBalance(pending.order.Market.BaseCurrency, quantity.Neg())
	default:
		return
	}

	executor.nextTradeID++
	pending.filled = pending.filled.Add(quantity)
	pending.remaining = pending.remaining.Sub(quantity)
	pending.filledNotional = pending.filledNotional.Add(quoteQuantity)
	averagePrice := price
	if pending.filled.Sign() > 0 {
		averagePrice = pending.filledNotional.Div(pending.filled)
	}

	pending.order.ExecutedQuantity = pending.filled
	pending.order.AveragePrice = averagePrice
	pending.order.UpdateTime = types.Time(eventTime)
	pending.order.IsWorking = pending.remaining.Sign() > 0
	if pending.remaining.Sign() > 0 {
		pending.order.Status = types.OrderStatusPartiallyFilled
	} else {
		pending.order.Status = types.OrderStatusFilled
	}

	executor.stream.EmitTradeUpdate(types.Trade{
		ID:            executor.nextTradeID,
		OrderID:       pending.order.OrderID,
		Exchange:      types.ExchangeBacktest,
		Price:         price,
		Quantity:      quantity,
		QuoteQuantity: quoteQuantity,
		Symbol:        pending.order.Symbol,
		Side:          pending.order.Side,
		IsBuyer:       pending.order.Side == types.SideTypeBuy,
		IsMaker:       pending.order.Type == types.OrderTypeLimit || pending.order.Type == types.OrderTypeStopLimit,
		Time:          types.Time(eventTime),
		FeeCurrency:   pending.order.Market.QuoteCurrency,
		IsMargin:      true,
	})
	executor.stream.EmitOrderUpdate(pending.order)
}

func (executor *conservativeBarExecutor) matchPriceLocked(
	pending *conservativeBarPendingOrder,
	kline types.KLine,
	mode conservativeBarMatchMode,
) (fixedpoint.Value, bool) {
	order := pending.order
	switch order.Type {
	case "", types.OrderTypeMarket:
		price := kline.Open
		if mode == conservativeBarClosePoint {
			price = kline.Close
		}
		return executor.applyMarketSlippage(order, price), price.Sign() > 0
	case types.OrderTypeLimit, types.OrderTypeLimitMaker:
		return conservativeBarLimitPrice(order, kline, mode)
	case types.OrderTypeStopMarket:
		price, ok := conservativeBarStopMarketPrice(order, kline, mode)
		if !ok {
			return fixedpoint.Zero, false
		}
		return executor.applyMarketSlippage(order, price), true
	case types.OrderTypeStopLimit:
		if !pending.stopTriggered {
			if !conservativeBarStopTriggered(order, kline, mode) {
				return fixedpoint.Zero, false
			}
			pending.stopTriggered = true
			return fixedpoint.Zero, false
		}
		return conservativeBarLimitPrice(order, kline, mode)
	default:
		executor.warnOnceLocked(
			"unsupported-order-type|"+string(order.Type),
			fmt.Sprintf("conservative-bar-v1: unsupported order type %s remains pending", order.Type),
		)
		return fixedpoint.Zero, false
	}
}

func conservativeBarLimitPrice(order types.Order, kline types.KLine, mode conservativeBarMatchMode) (fixedpoint.Value, bool) {
	limit := order.Price
	if limit.Sign() <= 0 {
		return fixedpoint.Zero, false
	}
	if mode == conservativeBarClosePoint {
		switch order.Side {
		case types.SideTypeBuy:
			return kline.Close, kline.Close.Sign() > 0 && kline.Close.Compare(limit) <= 0
		case types.SideTypeSell:
			return kline.Close, kline.Close.Sign() > 0 && kline.Close.Compare(limit) >= 0
		default:
			return fixedpoint.Zero, false
		}
	}

	switch order.Side {
	case types.SideTypeBuy:
		if kline.Open.Sign() > 0 && kline.Open.Compare(limit) <= 0 {
			return kline.Open, true
		}
		if kline.Low.Sign() > 0 && kline.Low.Compare(limit) <= 0 {
			return limit, true
		}
	case types.SideTypeSell:
		if kline.Open.Sign() > 0 && kline.Open.Compare(limit) >= 0 {
			return kline.Open, true
		}
		if kline.High.Sign() > 0 && kline.High.Compare(limit) >= 0 {
			return limit, true
		}
	}
	return fixedpoint.Zero, false
}

func conservativeBarStopMarketPrice(order types.Order, kline types.KLine, mode conservativeBarMatchMode) (fixedpoint.Value, bool) {
	stop := order.StopPrice
	if stop.Sign() <= 0 {
		return fixedpoint.Zero, false
	}
	if mode == conservativeBarClosePoint {
		switch order.Side {
		case types.SideTypeBuy:
			return kline.Close, kline.Close.Sign() > 0 && kline.Close.Compare(stop) >= 0
		case types.SideTypeSell:
			return kline.Close, kline.Close.Sign() > 0 && kline.Close.Compare(stop) <= 0
		default:
			return fixedpoint.Zero, false
		}
	}
	switch order.Side {
	case types.SideTypeBuy:
		if kline.Open.Sign() > 0 && kline.Open.Compare(stop) >= 0 {
			return kline.Open, true
		}
		if kline.High.Sign() > 0 && kline.High.Compare(stop) >= 0 {
			return stop, true
		}
	case types.SideTypeSell:
		if kline.Open.Sign() > 0 && kline.Open.Compare(stop) <= 0 {
			return kline.Open, true
		}
		if kline.Low.Sign() > 0 && kline.Low.Compare(stop) <= 0 {
			return stop, true
		}
	}
	return fixedpoint.Zero, false
}

func conservativeBarStopTriggered(order types.Order, kline types.KLine, mode conservativeBarMatchMode) bool {
	_, ok := conservativeBarStopMarketPrice(order, kline, mode)
	return ok
}

func (executor *conservativeBarExecutor) applyMarketSlippage(order types.Order, price fixedpoint.Value) fixedpoint.Value {
	if executor.options.SlippageTicks <= 0 || price.Sign() <= 0 || order.Market.TickSize.Sign() <= 0 {
		return price
	}
	offset := order.Market.TickSize.Mul(fixedpoint.NewFromInt(int64(executor.options.SlippageTicks)))
	switch order.Side {
	case types.SideTypeBuy:
		price = price.Add(offset)
	case types.SideTypeSell:
		price = price.Sub(offset)
	}
	if price.Sign() <= 0 {
		return fixedpoint.Zero
	}
	return order.Market.TruncatePrice(price)
}

func conservativeBarLiquidityBudget(kline types.KLine) fixedpoint.Value {
	if kline.Volume.Sign() <= 0 {
		return fixedpoint.Zero
	}
	return kline.Volume.Mul(fixedpoint.NewFromFloat(0.1))
}

func conservativeBarEventTime(kline types.KLine, mode conservativeBarMatchMode) time.Time {
	if mode == conservativeBarClosePoint {
		return kline.EndTime.Time()
	}
	return kline.StartTime.Time()
}

func sameConservativeBarOrder(a types.Order, b types.Order) bool {
	if a.OrderID != 0 && b.OrderID != 0 {
		return a.OrderID == b.OrderID
	}
	return a.ClientOrderID != "" && a.ClientOrderID == b.ClientOrderID
}

func (executor *conservativeBarExecutor) eventTimeLocked() time.Time {
	if executor.currentBarReady {
		return executor.currentBar.EndTime.Time()
	}
	return time.Now().UTC()
}

func (executor *conservativeBarExecutor) hasPendingSymbolLocked(symbol string) bool {
	for _, pending := range executor.pending {
		if pending != nil && pending.order.Symbol == symbol && pending.remaining.Sign() > 0 {
			return true
		}
	}
	return false
}

func (executor *conservativeBarExecutor) compactPendingLocked() {
	if len(executor.pending) == 0 {
		return
	}
	kept := executor.pending[:0]
	for _, pending := range executor.pending {
		if pending != nil && pending.remaining.Sign() > 0 {
			kept = append(kept, pending)
		}
	}
	executor.pending = kept
}

func (executor *conservativeBarExecutor) warnOnceLocked(key string, message string) {
	if executor.options.WarningSink == nil {
		return
	}
	if _, exists := executor.warned[key]; exists {
		return
	}
	executor.warned[key] = struct{}{}
	executor.options.WarningSink.AddWarning(message)
}
