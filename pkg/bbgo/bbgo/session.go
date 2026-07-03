package bbgo

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type ExchangeSession struct {
	Name             string
	Exchange         types.Exchange
	Account          *types.Account
	UserDataStream   types.Stream
	MarketDataStream types.Stream
	OrderExecutor    *ExchangeOrderExecutor

	markets         types.MarketMap
	marketMutex     sync.RWMutex
	lastPrices      map[string]fixedpoint.Value
	lastPricesMutex sync.RWMutex
}

func NewExchangeSession(name string, exchange types.Exchange) *ExchangeSession {
	userDataStream := exchange.NewStream()
	marketDataStream := exchange.NewStream()
	marketDataStream.SetPublicOnly()

	session := &ExchangeSession{
		Name:             name,
		Exchange:         exchange,
		Account:          types.NewAccount(),
		UserDataStream:   userDataStream,
		MarketDataStream: marketDataStream,
		markets:          types.MarketMap{},
		lastPrices:       map[string]fixedpoint.Value{},
	}
	session.OrderExecutor = &ExchangeOrderExecutor{Session: session}
	return session
}

func (s *ExchangeSession) SetMarkets(markets types.MarketMap) {
	s.marketMutex.Lock()
	defer s.marketMutex.Unlock()
	s.markets = types.MarketMap{}
	maps.Copy(s.markets, markets)
}

func (s *ExchangeSession) Markets() types.MarketMap {
	s.marketMutex.RLock()
	defer s.marketMutex.RUnlock()
	markets := types.MarketMap{}
	maps.Copy(markets, s.markets)
	return markets
}

func (s *ExchangeSession) Market(symbol string) (types.Market, bool) {
	s.marketMutex.RLock()
	defer s.marketMutex.RUnlock()
	market, ok := s.markets[symbol]
	return market, ok
}

func (s *ExchangeSession) LastPrices() map[string]fixedpoint.Value {
	return s.lastPrices
}

func (s *ExchangeSession) LastPrice(symbol string) (fixedpoint.Value, bool) {
	s.lastPricesMutex.RLock()
	defer s.lastPricesMutex.RUnlock()
	price, ok := s.lastPrices[symbol]
	return price, ok
}

func (s *ExchangeSession) FormatOrders(orders []types.SubmitOrder) ([]types.SubmitOrder, error) {
	formatted := make([]types.SubmitOrder, len(orders))
	copy(formatted, orders)
	for i := range formatted {
		order := &formatted[i]
		if order.Market.Symbol == "" {
			market, ok := s.Market(order.Symbol)
			if !ok {
				return nil, fmt.Errorf("market config of symbol %q is not found", order.Symbol)
			}
			order.Market = market
		}
	}
	return formatted, nil
}

type OrderExecutor interface {
	SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error)
	CancelOrders(ctx context.Context, orders ...types.Order) error
}

type ExchangeOrderExecutor struct {
	Session *ExchangeSession
}

func (e *ExchangeOrderExecutor) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	if e == nil || e.Session == nil || e.Session.Exchange == nil {
		return nil, fmt.Errorf("exchange order executor session is required")
	}
	formatted, err := e.Session.FormatOrders(orders)
	if err != nil {
		return nil, err
	}
	created := make(types.OrderSlice, 0, len(formatted))
	for _, order := range formatted {
		createdOrder, err := e.Session.Exchange.SubmitOrder(ctx, order)
		if err != nil {
			return created, err
		}
		if createdOrder != nil {
			created = append(created, *createdOrder)
		}
	}
	return created, nil
}

func (e *ExchangeOrderExecutor) CancelOrders(ctx context.Context, orders ...types.Order) error {
	if e == nil || e.Session == nil || e.Session.Exchange == nil {
		return fmt.Errorf("exchange order executor session is required")
	}
	return e.Session.Exchange.CancelOrders(ctx, orders...)
}
