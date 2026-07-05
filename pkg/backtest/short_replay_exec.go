package backtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

var pineWorkerSyntheticOrderID uint64 = 900_000_000
var pineWorkerSyntheticTradeID uint64 = 900_000_000

type pineWorkerShortReplayExecutor struct {
	delegate PineWorkerOrderExecutor
	account  *types.Account
	stream   types.StandardStreamEmitter

	mu        sync.RWMutex
	lastPrice map[string]fixedpoint.Value
	lastTime  map[string]time.Time
}

func newPineWorkerShortReplayExecutor(delegate PineWorkerOrderExecutor, account *types.Account, stream types.StandardStreamEmitter) *pineWorkerShortReplayExecutor {
	return &pineWorkerShortReplayExecutor{
		delegate:  delegate,
		account:   account,
		stream:    stream,
		lastPrice: map[string]fixedpoint.Value{},
		lastTime:  map[string]time.Time{},
	}
}

func (executor *pineWorkerShortReplayExecutor) onKLineClosed(kline types.KLine) {
	executor.mu.Lock()
	executor.lastPrice[kline.Symbol] = kline.Close
	executor.lastTime[kline.Symbol] = kline.EndTime.Time()
	executor.mu.Unlock()
}

func (executor *pineWorkerShortReplayExecutor) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	created := make(types.OrderSlice, 0, len(orders))
	for _, order := range orders {
		if order.Tag != pineWorkerShortReplayOrderTag {
			if executor.delegate == nil {
				return nil, fmt.Errorf("pine worker short replay delegate order executor is required")
			}
			delegateCreated, err := executor.delegate.SubmitOrders(ctx, order)
			if err != nil {
				return created, err
			}
			created = append(created, delegateCreated...)
			continue
		}
		filled, err := executor.submitSyntheticShortReplay(order)
		if err != nil {
			return created, err
		}
		created = append(created, filled)
	}
	return created, nil
}

func (executor *pineWorkerShortReplayExecutor) CancelOrders(ctx context.Context, orders ...types.Order) error {
	var delegateOrders []types.Order
	for _, order := range orders {
		if order.Tag == pineWorkerShortReplayOrderTag {
			continue
		}
		delegateOrders = append(delegateOrders, order)
	}
	if len(delegateOrders) == 0 {
		return nil
	}
	if executor.delegate == nil {
		return fmt.Errorf("pine worker short replay delegate order executor is required")
	}
	return executor.delegate.CancelOrders(ctx, delegateOrders...)
}

func (executor *pineWorkerShortReplayExecutor) submitSyntheticShortReplay(order types.SubmitOrder) (types.Order, error) {
	if executor.account == nil {
		return types.Order{}, fmt.Errorf("pine worker short replay account is required")
	}
	if executor.stream == nil {
		return types.Order{}, fmt.Errorf("pine worker short replay stream is required")
	}
	price, eventTime, err := executor.syntheticFillPrice(order)
	if err != nil {
		return types.Order{}, err
	}
	if order.Quantity.Sign() <= 0 {
		return types.Order{}, fmt.Errorf("pine worker short replay quantity must be positive")
	}
	quoteQuantity := order.Quantity.Mul(price)
	fee := quoteQuantity.Mul(executor.account.TakerFeeRate)
	switch order.Side {
	case types.SideTypeSell:
		executor.account.AddBalance(order.Market.QuoteCurrency, quoteQuantity.Sub(fee))
	case types.SideTypeBuy:
		executor.account.AddBalance(order.Market.QuoteCurrency, quoteQuantity.Add(fee).Neg())
	default:
		return types.Order{}, fmt.Errorf("pine worker short replay side is required")
	}

	order.Price = price
	order.AveragePrice = price
	orderID := atomic.AddUint64(&pineWorkerSyntheticOrderID, 1)
	tradeID := atomic.AddUint64(&pineWorkerSyntheticTradeID, 1)
	created := types.Order{
		SubmitOrder:  order,
		Exchange:     types.ExchangeBacktest,
		OrderID:      orderID,
		Status:       types.OrderStatusNew,
		CreationTime: types.Time(eventTime),
		UpdateTime:   types.Time(eventTime),
		IsWorking:    true,
		IsMargin:     true,
	}
	executor.stream.EmitOrderUpdate(created)

	filled := created
	filled.Status = types.OrderStatusFilled
	filled.ExecutedQuantity = order.Quantity
	filled.IsWorking = false
	executor.stream.EmitTradeUpdate(types.Trade{
		ID:            tradeID,
		OrderID:       orderID,
		Exchange:      types.ExchangeBacktest,
		Price:         price,
		Quantity:      order.Quantity,
		QuoteQuantity: quoteQuantity,
		Symbol:        order.Symbol,
		Side:          order.Side,
		IsBuyer:       order.Side == types.SideTypeBuy,
		IsMaker:       false,
		Time:          types.Time(eventTime),
		Fee:           fee,
		FeeCurrency:   order.Market.QuoteCurrency,
		IsMargin:      true,
	})
	executor.stream.EmitOrderUpdate(filled)
	return filled, nil
}

func (executor *pineWorkerShortReplayExecutor) syntheticFillPrice(order types.SubmitOrder) (fixedpoint.Value, time.Time, error) {
	price := order.Price
	if price.IsZero() {
		price = order.AveragePrice
	}
	executor.mu.RLock()
	lastPrice := executor.lastPrice[order.Symbol]
	eventTime := executor.lastTime[order.Symbol]
	executor.mu.RUnlock()
	if price.IsZero() {
		price = lastPrice
	}
	if !order.Market.TickSize.IsZero() && !price.IsZero() {
		price = order.Market.TruncatePrice(price)
	}
	if price.Sign() <= 0 {
		return fixedpoint.Zero, time.Time{}, fmt.Errorf("pine worker short replay price is unavailable for %s", order.Symbol)
	}
	if eventTime.IsZero() {
		eventTime = time.Now().UTC()
	}
	return price, eventTime, nil
}
