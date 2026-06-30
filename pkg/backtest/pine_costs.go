package backtest

import (
	"context"
	"sync"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func pineCommissionRate(metadata strategyir.StrategyMetadata) fixedpoint.Value {
	if metadata.CommissionType != "percent" || metadata.CommissionValue <= 0 {
		return fixedpoint.Zero
	}
	return fixedpoint.NewFromFloat(metadata.CommissionValue / 100)
}

type backtestSlippageExecutor struct {
	delegate bbgo2.OrderExecutor
	session  *bbgo2.ExchangeSession
	ticks    int

	mu         sync.RWMutex
	lastPrices map[string]fixedpoint.Value
}

func newBacktestSlippageExecutor(delegate bbgo2.OrderExecutor, session *bbgo2.ExchangeSession, ticks int) *backtestSlippageExecutor {
	return &backtestSlippageExecutor{
		delegate:   delegate,
		session:    session,
		ticks:      ticks,
		lastPrices: map[string]fixedpoint.Value{},
	}
}

func (e *backtestSlippageExecutor) onKLineClosed(kline types.KLine) {
	e.mu.Lock()
	e.lastPrices[kline.Symbol] = kline.Close
	e.mu.Unlock()
}

func (e *backtestSlippageExecutor) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	adjusted := make([]types.SubmitOrder, len(orders))
	copy(adjusted, orders)
	for index := range adjusted {
		order := &adjusted[index]
		if order.Type != types.OrderTypeMarket {
			continue
		}
		price, tickSize, ok := e.slippagePrice(*order)
		if !ok {
			continue
		}
		order.Type = types.OrderTypeLimit
		order.Price = price
		if order.Market.TickSize.IsZero() {
			order.Market.TickSize = tickSize
		}
	}
	return e.delegate.SubmitOrders(ctx, adjusted...)
}

func (e *backtestSlippageExecutor) CancelOrders(ctx context.Context, orders ...types.Order) error {
	return e.delegate.CancelOrders(ctx, orders...)
}

func (e *backtestSlippageExecutor) slippagePrice(order types.SubmitOrder) (fixedpoint.Value, fixedpoint.Value, bool) {
	e.mu.RLock()
	lastPrice, ok := e.lastPrices[order.Symbol]
	e.mu.RUnlock()
	if !ok || lastPrice.IsZero() || e.session == nil || e.ticks <= 0 {
		return fixedpoint.Zero, fixedpoint.Zero, false
	}
	market, ok := e.session.Market(order.Symbol)
	if !ok || market.TickSize.IsZero() {
		return fixedpoint.Zero, fixedpoint.Zero, false
	}
	offset := market.TickSize.Mul(fixedpoint.NewFromInt(int64(e.ticks)))
	if order.Side == types.SideTypeSell {
		return market.TruncatePrice(lastPrice.Sub(offset)), market.TickSize, true
	}
	return market.TruncatePrice(lastPrice.Add(offset)), market.TickSize, true
}
