package servercore

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionOrderStoreReconcilesFillBeforeOrderSnapshot(t *testing.T) {
	store := newExecutionOrderStore()
	fillPrice := 191.25
	fillIDEx := "fill-before-order-ex"
	orderIDEx := "order-before-sync-ex"
	rawFillStatus := "FILLED_ALL"

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
		Status:             &rawFillStatus,
	})
	if !changed || fillEvent == nil {
		t.Fatalf("fill discovery changed=%v event=%#v", changed, fillEvent)
	}
	if filled.Status != "FILLED" || filled.RawBrokerStatus == nil || *filled.RawBrokerStatus != "FILLED_ALL" || filled.Source != "broker" || filled.SourceDetail != "broker.fill" {
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
	if synced.Status != "FILLED" || synced.OrderType == nil || *synced.OrderType != "LIMIT" {
		t.Fatalf("reconciled order state = %#v", synced)
	}
	if synced.RequestedQuantity == nil || *synced.RequestedQuantity != 2 || synced.RequestedPrice == nil || *synced.RequestedPrice != requestedPrice {
		t.Fatalf("reconciled requested economics = %#v", synced)
	}
	if synced.Remark == nil || *synced.Remark != remark || synced.SubmittedAt == nil || *synced.SubmittedAt != "2026-07-02T01:29:58Z" {
		t.Fatalf("reconciled metadata = %#v", synced)
	}
	if syncEvent.EventType != "BROKER_SYNC_UPDATED" || syncEvent.PreviousStatus == nil || *syncEvent.PreviousStatus != "FILLED" || syncEvent.NextStatus != "FILLED" {
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

func TestExecutionOrderStoreDoesNotDoubleCountSnapshotCoveredFill(t *testing.T) {
	store := newExecutionOrderStore()
	seeded := seedOutOfOrderPlacedOrder(store, "snapshot-covered-fill")
	snapshotAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 5*time.Minute)
	delayedFillAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 4*time.Minute)
	newFillAt := executionTestTimestampAfter(t, seeded.UpdatedAt, 6*time.Minute)
	snapshotFilled := 4.0
	snapshotAverage := 100.0

	snapshot, _, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID: "SIM-1", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "snapshot-covered-fill", Symbol: "US.AAPL", Side: "BUY", OrderType: "LIMIT",
		Status: "FILLED_PART", Quantity: 10, FilledQuantity: &snapshotFilled,
		FilledAveragePrice: &snapshotAverage, UpdatedAt: snapshotAt,
	}, "BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
	if !changed || snapshot.FilledQuantity == nil || *snapshot.FilledQuantity != 4 {
		t.Fatalf("snapshot = %#v changed=%v", snapshot, changed)
	}

	delayedPrice := 100.0
	delayed, delayedEvent, changed := store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID: "SIM-1", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "snapshot-covered-fill", BrokerFillID: "covered-fill",
		Symbol: "US.AAPL", Side: "BUY", FilledQuantity: 4, FillPrice: &delayedPrice, FilledAt: delayedFillAt,
	})
	if changed || delayedEvent == nil || delayed.FilledQuantity == nil || *delayed.FilledQuantity != 4 {
		t.Fatalf("snapshot-covered fill = %#v event=%#v changed=%v", delayed, delayedEvent, changed)
	}
	if delayed.FilledAveragePrice == nil || *delayed.FilledAveragePrice != snapshotAverage {
		t.Fatalf("snapshot-covered average = %#v, want %v", delayed.FilledAveragePrice, snapshotAverage)
	}

	newPrice := 102.0
	updated, _, changed := store.recordBrokerOrderFill("futu", broker.OrderFillSnapshot{
		AccountID: "SIM-1", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "snapshot-covered-fill", BrokerFillID: "new-fill",
		Symbol: "US.AAPL", Side: "BUY", FilledQuantity: 2, FillPrice: &newPrice, FilledAt: newFillAt,
	})
	if !changed || updated.FilledQuantity == nil || *updated.FilledQuantity != 6 {
		t.Fatalf("new fill = %#v changed=%v", updated, changed)
	}
	wantAverage := (snapshotAverage*4 + newPrice*2) / 6
	if updated.FilledAveragePrice == nil || *updated.FilledAveragePrice != wantAverage {
		t.Fatalf("new fill average = %#v, want %v", updated.FilledAveragePrice, wantAverage)
	}
}

func TestExecutionOrderStoreRejectsStaleBrokerSnapshotRegression(t *testing.T) {
	store := newExecutionOrderStore()
	price := 100.0
	filledQuantity := 10.0
	filledAverage := 99.5
	order := store.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID: "futu", BrokerOrderID: "stale-1", TradingEnvironment: "SIMULATE",
		AccountID: "SIM-1", Market: "US", Symbol: "US.AAPL", Side: "BUY",
		OrderType: "LIMIT", Status: "SUBMITTED", RequestedQuantity: 10,
		RequestedPrice: &price, SubmittedAt: "2026-07-05T01:00:00Z", EventType: "COMMAND_PLACE_ACCEPTED",
	})
	filledAt := executionTestTimestampAfter(t, order.UpdatedAt, 5*time.Minute)
	staleAt := executionTestTimestampAfter(t, order.UpdatedAt, time.Minute)
	filled, _, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID: "SIM-1", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "stale-1", Symbol: "US.AAPL", Side: "BUY", OrderType: "LIMIT",
		Status: "FILLED_ALL", Quantity: 10, FilledQuantity: &filledQuantity,
		Price: &price, FilledAveragePrice: &filledAverage,
		SubmittedAt: "2026-07-05T01:00:00Z", UpdatedAt: filledAt,
	}, "BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER", "broker", "broker.push")
	if !changed || filled.Status != "FILLED" || filled.RawBrokerStatus == nil || *filled.RawBrokerStatus != "FILLED_ALL" {
		t.Fatalf("filled update = %#v changed=%v", filled, changed)
	}

	zeroFilled := 0.0
	_, staleEvent, changed := store.upsertBrokerOrderWithSource("futu", broker.OrderSnapshot{
		AccountID: "SIM-1", TradingEnvironment: "SIMULATE", Market: "US",
		BrokerOrderID: "stale-1", Symbol: "US.AAPL", Side: "BUY", OrderType: "LIMIT",
		Status: "SUBMITTED", Quantity: 10, FilledQuantity: &zeroFilled,
		Price: &price, SubmittedAt: "2026-07-05T01:00:00Z", UpdatedAt: staleAt,
	}, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED", "broker", "broker.current")
	if changed || staleEvent != nil {
		t.Fatalf("stale snapshot changed=%v event=%#v", changed, staleEvent)
	}
	persisted, ok := store.order(order.InternalOrderID)
	if !ok || persisted.Status != "FILLED" || persisted.FilledQuantity == nil || *persisted.FilledQuantity != 10 || persisted.UpdatedAt != filledAt {
		t.Fatalf("persisted order regressed: %#v", persisted)
	}
	if persisted.RawBrokerStatus == nil || *persisted.RawBrokerStatus != "FILLED_ALL" {
		t.Fatalf("raw broker status regressed: %#v", persisted.RawBrokerStatus)
	}
}
