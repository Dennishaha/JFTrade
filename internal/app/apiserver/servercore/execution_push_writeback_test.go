package servercore

import (
	"path/filepath"
	"testing"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestExecutionPushHandlersWriteBackAndNotify(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	price := 320.5
	placed := server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    "EXT-9001",
		TradingEnvironment: "SIMULATE",
		AccountID:          "1001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		RequestedQuantity:  100,
		RequestedPrice:     &price,
		SubmittedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})

	(&tradingExecutionOrderUpdates{server: server}).ApplyFill(t.Context(), "futu", trdsrv.Fill{
		AccountID:          "1001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    stringPointerOrNil("EXT-9001"),
		BrokerFillID:       "1",
		Symbol:             "HK.00700",
		SymbolName:         stringPointerOrNil("Tencent"),
		Side:               "BUY",
		FilledQuantity:     100,
		FillPrice:          new(320.5),
		FilledAt:           "2026-05-20T09:32:00Z",
	})

	filledOrder, ok := server.executionOrders.order(placed.InternalOrderID)
	if !ok {
		t.Fatal("expected placed order to remain in execution store")
	}
	if got := filledOrder.Status; got != "FILLED_ALL" {
		t.Fatalf("filled order status = %q, want FILLED_ALL", got)
	}
	events := server.executionOrders.orderEvents(placed.InternalOrderID)
	if len(events.Events) != 2 {
		t.Fatalf("expected place and fill events, got %#v", events.Events)
	}
	fillNotificationFound := false
	for _, note := range server.liveNotificationsAfter(0) {
		if note.Title == "Futu 成交成功" {
			fillNotificationFound = true
			break
		}
	}
	if !fillNotificationFound {
		t.Fatalf("expected Futu 成交成功 notification, got %#v", server.liveNotificationsAfter(0))
	}

	placedCancel := server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "9002",
		TradingEnvironment: "SIMULATE",
		AccountID:          "1001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "SELL",
		OrderType:          "LIMIT",
		Status:             "CANCEL_REQUESTED",
		RequestedQuantity:  50,
		RequestedPrice:     &price,
		SubmittedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		EventType:          "COMMAND_CANCEL_ACCEPTED",
	})

	(&tradingExecutionOrderUpdates{server: server}).ApplyOrder(t.Context(), "futu", trdsrv.Order{
		AccountID:          "1001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9002",
		Symbol:             "HK.00700",
		Side:               "SELL",
		OrderType:          "LIMIT",
		Status:             "CANCELLED_ALL",
		Quantity:           50,
		Price:              &price,
		SubmittedAt:        "2026-05-20T09:33:00Z",
		UpdatedAt:          "2026-05-20T09:34:00Z",
	}, trdsrv.OrderWriteMetadata{
		DiscoveredEventType: "BROKER_PUSH_DISCOVERED",
		UpdatedEventType:    "BROKER_PUSH_ORDER",
		Source:              "broker",
		SourceDetail:        "broker.push",
	})

	cancelledOrder, ok := server.executionOrders.order(placedCancel.InternalOrderID)
	if !ok {
		t.Fatal("expected cancelled order to remain in execution store")
	}
	if got := cancelledOrder.Status; got != "CANCELLED_ALL" {
		t.Fatalf("cancelled order status = %q, want CANCELLED_ALL", got)
	}
	cancelNotificationFound := false
	for _, note := range server.liveNotificationsAfter(0) {
		if note.Title == "Futu 撤单成功" {
			cancelNotificationFound = true
			break
		}
	}
	if !cancelNotificationFound {
		t.Fatalf("expected Futu 撤单成功 notification, got %#v", server.liveNotificationsAfter(0))
	}
}

func TestRecordPlacedOrderReusesExistingBrokerDiscoveredOrder(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	(&tradingExecutionOrderUpdates{server: server}).ApplyOrder(t.Context(), "futu", trdsrv.Order{
		AccountID:          "1001",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    stringPointerOrNil("EXT-9001"),
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		Quantity:           100,
		Price:              new(320.5),
		SubmittedAt:        "2026-05-20T09:33:00Z",
		UpdatedAt:          "2026-05-20T09:33:00Z",
	}, trdsrv.OrderWriteMetadata{
		DiscoveredEventType: "BROKER_PUSH_DISCOVERED",
		UpdatedEventType:    "BROKER_PUSH_ORDER",
		Source:              "broker",
		SourceDetail:        "broker.push",
	})

	ordersBefore := server.executionOrders.listOrders()
	if len(ordersBefore.Orders) != 1 {
		t.Fatalf("expected one discovered order, got %#v", ordersBefore.Orders)
	}
	discovered := ordersBefore.Orders[0]
	placed := server.executionOrders.recordPlacedOrder(executionPlacedOrderRecord{
		BrokerID:           "futu",
		BrokerOrderID:      "9001",
		BrokerOrderIDEx:    "EXT-9001",
		TradingEnvironment: "SIMULATE",
		AccountID:          "1001",
		Market:             "HK",
		Symbol:             "HK.00700",
		Side:               "BUY",
		OrderType:          "LIMIT",
		Status:             "SUBMITTED",
		RequestedQuantity:  100,
		RequestedPrice:     new(320.5),
		SubmittedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		EventType:          "COMMAND_PLACE_ACCEPTED",
	})

	if placed.InternalOrderID != discovered.InternalOrderID {
		t.Fatalf("internalOrderId = %q, want %q", placed.InternalOrderID, discovered.InternalOrderID)
	}
	ordersAfter := server.executionOrders.listOrders()
	if len(ordersAfter.Orders) != 1 {
		t.Fatalf("expected one order after command acceptance, got %#v", ordersAfter.Orders)
	}
	events := server.executionOrders.orderEvents(placed.InternalOrderID)
	if len(events.Events) != 2 {
		t.Fatalf("expected push + command events, got %#v", events.Events)
	}
	if got := events.Events[1].EventType; got != "COMMAND_PLACE_ACCEPTED" {
		t.Fatalf("second event type = %q, want COMMAND_PLACE_ACCEPTED", got)
	}
}
