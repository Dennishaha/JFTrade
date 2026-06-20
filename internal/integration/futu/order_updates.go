package futu

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/jftrade/jftrade-main/internal/trading"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

type orderUpdateExchange interface {
	Connect(context.Context) error
	SubscribeTradeAccountPush(context.Context, []uint64) error
	OnOrderUpdate(func(*trdcommonpb.TrdHeader, *trdcommonpb.Order)) func()
	OnOrderFillUpdate(func(*trdcommonpb.TrdHeader, *trdcommonpb.OrderFill)) func()
}

type OrderUpdatesAdapter struct {
	exchange orderUpdateExchange
}

func NewOrderUpdatesAdapter(exchange orderUpdateExchange) *OrderUpdatesAdapter {
	return &OrderUpdatesAdapter{exchange: exchange}
}

func (a *OrderUpdatesAdapter) Subscribe(ctx context.Context, accounts []trading.Account, handler trading.OrderUpdateHandler) (trading.OrderUpdateSubscription, error) {
	if a == nil || a.exchange == nil || handler == nil {
		return noOpOrderUpdateSubscription{}, nil
	}
	stopOrder := a.exchange.OnOrderUpdate(func(header *trdcommonpb.TrdHeader, order *trdcommonpb.Order) {
		handler.HandleOrderUpdate(orderFromPush(header, order))
	})
	stopFill := a.exchange.OnOrderFillUpdate(func(header *trdcommonpb.TrdHeader, fill *trdcommonpb.OrderFill) {
		handler.HandleFillUpdate(fillFromPush(header, fill))
	})
	stop := &orderUpdateSubscription{exchange: a.exchange, stopOrder: stopOrder, stopFill: stopFill}
	if err := stop.Refresh(ctx, accounts, nil); err != nil {
		jftradeErr1 := stop.Stop()
		jftradeLogError(jftradeErr1)
		return nil, err
	}
	return stop, nil
}

type orderUpdateSubscription struct {
	once      sync.Once
	exchange  orderUpdateExchange
	stopOrder func()
	stopFill  func()
}

func (s *orderUpdateSubscription) Refresh(ctx context.Context, accounts []trading.Account, _ []trading.OrderQuery) error {
	if s == nil || s.exchange == nil {
		return nil
	}
	if err := s.exchange.Connect(ctx); err != nil {
		return err
	}
	accountIDs := accountPushIDs(accounts)
	if len(accountIDs) == 0 {
		return nil
	}
	return s.exchange.SubscribeTradeAccountPush(ctx, accountIDs)
}

func (s *orderUpdateSubscription) Stop() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		if s.stopOrder != nil {
			s.stopOrder()
		}
		if s.stopFill != nil {
			s.stopFill()
		}
	})
	return nil
}

type noOpOrderUpdateSubscription struct{}

func (noOpOrderUpdateSubscription) Stop() error { return nil }

func accountPushIDs(accounts []trading.Account) []uint64 {
	ids := make([]uint64, 0, len(accounts))
	seen := make(map[uint64]struct{})
	for _, account := range accounts {
		id, err := strconv.ParseUint(strings.TrimSpace(account.ID), 10, 64)
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func orderFromPush(header *trdcommonpb.TrdHeader, order *trdcommonpb.Order) trading.Order {
	snapshot := pkgfutu.BrokerOrderSnapshotFromPush(header, order)
	return trading.Order{
		AccountID: snapshot.AccountID, TradingEnvironment: snapshot.TradingEnvironment, Market: snapshot.Market,
		BrokerOrderID: snapshot.BrokerOrderID, BrokerOrderIDEx: snapshot.BrokerOrderIDEx,
		Symbol: snapshot.Symbol, SymbolName: snapshot.SymbolName, Side: snapshot.Side, OrderType: snapshot.OrderType,
		Status: snapshot.Status, Quantity: snapshot.Quantity, FilledQuantity: snapshot.FilledQuantity,
		Price: snapshot.Price, FilledAveragePrice: snapshot.FilledAveragePrice, SubmittedAt: snapshot.SubmittedAt,
		UpdatedAt: snapshot.UpdatedAt, Remark: snapshot.Remark, LastError: snapshot.LastError,
		TimeInForce: snapshot.TimeInForce, Currency: snapshot.Currency,
	}
}

func fillFromPush(header *trdcommonpb.TrdHeader, fill *trdcommonpb.OrderFill) trading.Fill {
	snapshot := pkgfutu.BrokerOrderFillSnapshotFromPush(header, fill)
	return trading.Fill{
		AccountID: snapshot.AccountID, TradingEnvironment: snapshot.TradingEnvironment, Market: snapshot.Market,
		BrokerOrderID: snapshot.BrokerOrderID, BrokerOrderIDEx: snapshot.BrokerOrderIDEx,
		BrokerFillID: snapshot.BrokerFillID, BrokerFillIDEx: snapshot.BrokerFillIDEx,
		Symbol: snapshot.Symbol, SymbolName: snapshot.SymbolName, Side: snapshot.Side,
		FilledQuantity: snapshot.FilledQuantity, FillPrice: snapshot.FillPrice, FilledAt: snapshot.FilledAt,
		Status: snapshot.Status,
	}
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
