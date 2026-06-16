package futu

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/internal/trading"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

type fakeOrderUpdateExchange struct {
	connectCalls   int
	subscribeCalls int
	accountIDs     []uint64
	orderHandler   func(*trdcommonpb.TrdHeader, *trdcommonpb.Order)
	fillHandler    func(*trdcommonpb.TrdHeader, *trdcommonpb.OrderFill)
	orderRegisters int
	fillRegisters  int
	orderStops     int
	fillStops      int
}

func (f *fakeOrderUpdateExchange) Connect(context.Context) error {
	f.connectCalls++
	return nil
}

func (f *fakeOrderUpdateExchange) SubscribeTradeAccountPush(_ context.Context, ids []uint64) error {
	f.subscribeCalls++
	f.accountIDs = append([]uint64(nil), ids...)
	return nil
}

func (f *fakeOrderUpdateExchange) OnOrderUpdate(handler func(*trdcommonpb.TrdHeader, *trdcommonpb.Order)) func() {
	f.orderRegisters++
	f.orderHandler = handler
	return func() { f.orderStops++ }
}

func (f *fakeOrderUpdateExchange) OnOrderFillUpdate(handler func(*trdcommonpb.TrdHeader, *trdcommonpb.OrderFill)) func() {
	f.fillRegisters++
	f.fillHandler = handler
	return func() { f.fillStops++ }
}

type captureOrderUpdates struct {
	orders []trading.Order
	fills  []trading.Fill
}

func (c *captureOrderUpdates) HandleOrderUpdate(order trading.Order) {
	c.orders = append(c.orders, order)
}
func (c *captureOrderUpdates) HandleFillUpdate(fill trading.Fill) { c.fills = append(c.fills, fill) }

func TestOrderUpdatesAdapterConvertsPushesAndStopsOnce(t *testing.T) {
	exchange := &fakeOrderUpdateExchange{}
	adapter := NewOrderUpdatesAdapter(exchange)
	capture := &captureOrderUpdates{}
	subscription, err := adapter.Subscribe(context.Background(), []trading.Account{
		{ID: "1001"}, {ID: "1001"}, {ID: "bad"},
	}, capture)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	header := &trdcommonpb.TrdHeader{
		AccID: new(uint64(1001)), TrdEnv: new(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		TrdMarket: new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}
	exchange.orderHandler(header, &trdcommonpb.Order{
		OrderID: new(uint64(9)), Code: new("HK.00700"),
		OrderStatus: new(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		TrdMarket:   new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	})
	exchange.fillHandler(header, &trdcommonpb.OrderFill{
		OrderID: new(uint64(9)), FillID: new(uint64(10)), Code: new("HK.00700"), Qty: new(float64(5)),
		TrdMarket: new(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	})
	if len(capture.orders) != 1 || capture.orders[0].BrokerOrderID != "9" || capture.orders[0].Market != "HK" {
		t.Fatalf("orders = %#v", capture.orders)
	}
	if len(capture.fills) != 1 || capture.fills[0].BrokerFillID != "10" {
		t.Fatalf("fills = %#v", capture.fills)
	}
	if exchange.connectCalls != 1 || exchange.subscribeCalls != 1 || len(exchange.accountIDs) != 1 || exchange.accountIDs[0] != 1001 {
		t.Fatalf("subscription state = %+v", exchange)
	}
	_ = subscription.Stop()
	_ = subscription.Stop()
	if exchange.orderStops != 1 || exchange.fillStops != 1 {
		t.Fatalf("stop calls order=%d fill=%d", exchange.orderStops, exchange.fillStops)
	}
}

func TestOrderUpdatesAdapterRefreshRepeatsAccountPushWithoutReregisteringHandlers(t *testing.T) {
	exchange := &fakeOrderUpdateExchange{}
	adapter := NewOrderUpdatesAdapter(exchange)
	capture := &captureOrderUpdates{}
	subscription, err := adapter.Subscribe(context.Background(), []trading.Account{{ID: "1001"}}, capture)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	refresher, ok := subscription.(interface {
		Refresh(context.Context, []trading.Account, []trading.OrderQuery) error
	})
	if !ok {
		t.Fatal("subscription does not implement Refresh")
	}
	if err := refresher.Refresh(context.Background(), []trading.Account{{ID: "1002"}, {ID: "1002"}}, nil); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if exchange.connectCalls != 2 {
		t.Fatalf("connect calls = %d, want 2", exchange.connectCalls)
	}
	if exchange.subscribeCalls != 2 {
		t.Fatalf("subscribe calls = %d, want 2", exchange.subscribeCalls)
	}
	if got := exchange.accountIDs; len(got) != 1 || got[0] != 1002 {
		t.Fatalf("account ids = %#v, want [1002]", got)
	}
	if exchange.orderRegisters != 1 || exchange.fillRegisters != 1 {
		t.Fatalf("handler registrations order=%d fill=%d, want 1 each", exchange.orderRegisters, exchange.fillRegisters)
	}
}
