package futu

import (
	"testing"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestTradeUpdateHandlersCanUnsubscribe(t *testing.T) {
	exchange := NewExchange("")
	orderCalls := 0
	fillCalls := 0
	stopOrder := exchange.OnOrderUpdate(func(*trdcommonpb.TrdHeader, *trdcommonpb.Order) {
		orderCalls++
	})
	stopFill := exchange.OnOrderFillUpdate(func(*trdcommonpb.TrdHeader, *trdcommonpb.OrderFill) {
		fillCalls++
	})

	exchange.dispatchOrderUpdateNotify(nil, &trdcommonpb.Order{})
	exchange.dispatchOrderFillUpdateNotify(nil, &trdcommonpb.OrderFill{})
	stopOrder()
	stopFill()
	stopOrder()
	stopFill()
	exchange.dispatchOrderUpdateNotify(nil, &trdcommonpb.Order{})
	exchange.dispatchOrderFillUpdateNotify(nil, &trdcommonpb.OrderFill{})

	if orderCalls != 1 || fillCalls != 1 {
		t.Fatalf("calls after unsubscribe: order=%d fill=%d", orderCalls, fillCalls)
	}
}
