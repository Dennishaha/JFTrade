package jftradeapi

import (
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
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

	server.handleFutuBrokerOrderFillPush(&trdcommonpb.TrdHeader{
		TrdEnv:    proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     proto.Uint64(1001),
		TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}, &trdcommonpb.OrderFill{
		TrdSide:         proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		FillID:          proto.Uint64(1),
		OrderID:         proto.Uint64(9001),
		OrderIDEx:       proto.String("EXT-9001"),
		Code:            proto.String("HK.00700"),
		Name:            proto.String("Tencent"),
		Qty:             proto.Float64(100),
		Price:           proto.Float64(320.5),
		CreateTime:      proto.String("2026-05-20 09:32:00"),
		CreateTimestamp: proto.Float64(float64(time.Date(2026, time.May, 20, 9, 32, 0, 0, time.UTC).Unix())),
		TrdMarket:       proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
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

	server.handleFutuBrokerOrderPush(&trdcommonpb.TrdHeader{
		TrdEnv:    proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     proto.Uint64(1001),
		TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}, &trdcommonpb.Order{
		TrdSide:     proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Sell)),
		OrderType:   proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus: proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Cancelled_All)),
		OrderID:     proto.Uint64(9002),
		Code:        proto.String("HK.00700"),
		Qty:         proto.Float64(50),
		Price:       proto.Float64(320.5),
		CreateTime:  proto.String("2026-05-20 09:33:00"),
		UpdateTime:  proto.String("2026-05-20 09:34:00"),
		TrdMarket:   proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
		TimeInForce: proto.Int32(int32(trdcommonpb.TimeInForce_TimeInForce_DAY)),
		Currency:    proto.Int32(int32(trdcommonpb.Currency_Currency_HKD)),
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

	server.handleFutuBrokerOrderPush(&trdcommonpb.TrdHeader{
		TrdEnv:    proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:     proto.Uint64(1001),
		TrdMarket: proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	}, &trdcommonpb.Order{
		TrdSide:     proto.Int32(int32(trdcommonpb.TrdSide_TrdSide_Buy)),
		OrderType:   proto.Int32(int32(trdcommonpb.OrderType_OrderType_Normal)),
		OrderStatus: proto.Int32(int32(trdcommonpb.OrderStatus_OrderStatus_Submitted)),
		OrderID:     proto.Uint64(9001),
		OrderIDEx:   proto.String("EXT-9001"),
		Code:        proto.String("HK.00700"),
		Qty:         proto.Float64(100),
		Price:       proto.Float64(320.5),
		CreateTime:  proto.String("2026-05-20 09:33:00"),
		UpdateTime:  proto.String("2026-05-20 09:33:00"),
		TrdMarket:   proto.Int32(int32(trdcommonpb.TrdMarket_TrdMarket_HK)),
	})

	ordersBefore := server.executionOrders.listOrders()
	if len(ordersBefore.Orders) != 1 {
		t.Fatalf("expected one discovered order, got %#v", ordersBefore.Orders)
	}
	discovered := ordersBefore.Orders[0]
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
