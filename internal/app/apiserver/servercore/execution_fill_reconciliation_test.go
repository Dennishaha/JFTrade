package servercore

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionOrderStoreReconcilesFillBeforeOrderSnapshot(t *testing.T) {
	store := newExecutionOrderStore()
	fillPrice := 191.25
	fillIDEx := "fill-before-order-ex"
	orderIDEx := "order-before-sync-ex"

	filled, fillEvent, changed := store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID:          "SIM-OUT-OF-ORDER",
		TradingEnvironment: "SIMULATE",
		Market:             "US",
		BrokerOrderID:      "order-before-sync",
		BrokerOrderIDEx:    &orderIDEx,
		BrokerFillID:       "fill-before-order",
		BrokerFillIDEx:     &fillIDEx,
		Symbol:             "US.AAPL",
		Side:               "BUY",
		FilledQuantity:     2,
		FillPrice:          &fillPrice,
		FilledAt:           "2026-07-02T01:30:00Z",
	})
	if !changed || fillEvent == nil {
		t.Fatalf("fill discovery changed=%v event=%#v", changed, fillEvent)
	}
	if filled.Status != "FILLED_PART" || filled.Source != "broker" || filled.SourceDetail != "broker.fill" {
		t.Fatalf("fill-discovered order = %#v", filled)
	}
	if filled.FilledQuantity == nil || *filled.FilledQuantity != 2 || filled.FilledAveragePrice == nil || *filled.FilledAveragePrice != fillPrice {
		t.Fatalf("fill economics = %#v", filled)
	}
	if fillEvent.InternalOrderID != filled.InternalOrderID || fillEvent.EventType != "BROKER_FILL_RECEIVED" || fillEvent.PreviousStatus != nil {
		t.Fatalf("fill event = %#v", fillEvent)
	}

	requestedPrice := 190.50
	filledQuantity := 2.0
	averagePrice := 191.25
	remark := "snapshot arrived after push"
	synced, syncEvent, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID:          "SIM-OUT-OF-ORDER",
		TradingEnvironment: "SIMULATE",
		Market:             "US",
		BrokerOrderID:      "order-before-sync",
		BrokerOrderIDEx:    &orderIDEx,
		Symbol:             "US.AAPL",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "FILLED_ALL",
		Quantity:           2,
		Price:              &requestedPrice,
		FilledQuantity:     &filledQuantity,
		FilledAveragePrice: &averagePrice,
		Remark:             &remark,
		SubmittedAt:        "2026-07-02T01:29:58Z",
		UpdatedAt:          "2026-07-02T01:30:01Z",
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "broker", "broker.current")
	if !changed || syncEvent == nil {
		t.Fatalf("late snapshot changed=%v event=%#v", changed, syncEvent)
	}
	if synced.InternalOrderID != filled.InternalOrderID {
		t.Fatalf("late snapshot created %q, want merge into %q", synced.InternalOrderID, filled.InternalOrderID)
	}
	if synced.Status != "FILLED_ALL" || synced.OrderType == nil || *synced.OrderType != "LIMIT" {
		t.Fatalf("reconciled order state = %#v", synced)
	}
	if synced.RequestedQuantity == nil || *synced.RequestedQuantity != 2 || synced.RequestedPrice == nil || *synced.RequestedPrice != requestedPrice {
		t.Fatalf("reconciled requested economics = %#v", synced)
	}
	if synced.Remark == nil || *synced.Remark != remark || synced.SubmittedAt == nil || *synced.SubmittedAt != "2026-07-02T01:29:58Z" {
		t.Fatalf("reconciled metadata = %#v", synced)
	}
	if syncEvent.EventType != "BROKER_SYNC_UPDATED" || syncEvent.PreviousStatus == nil || *syncEvent.PreviousStatus != "FILLED_PART" || syncEvent.NextStatus != "FILLED_ALL" {
		t.Fatalf("sync event = %#v", syncEvent)
	}

	orders := store.listOrders()
	if len(orders.Orders) != 1 || orders.Orders[0].InternalOrderID != filled.InternalOrderID {
		t.Fatalf("reconciled orders = %#v", orders.Orders)
	}
	events := store.orderEvents(filled.InternalOrderID)
	if len(events.Events) != 2 {
		t.Fatalf("reconciled events = %#v", events.Events)
	}
}
