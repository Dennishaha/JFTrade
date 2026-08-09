package tradingapp

import (
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestOrderPlacedNotificationMapsBrokerLabelAndMessage(t *testing.T) {
	symbol, brokerID := "HK.00700", "futu"
	quantity := 100.0
	note := OrderPlacedNotification(trdsrv.ExecutionOrder{
		BrokerID: " futu ", TradingEnvironment: "SIMULATE", Symbol: &symbol,
		Side: ptr("BUY"), RequestedQuantity: &quantity, BrokerOrderID: &brokerID,
	})
	if note.Level != "success" || note.Category != "broker.order.place" || note.Title != "FUTU 订单已提交" {
		t.Fatalf("placed notification = %#v", note)
	}
	if !strings.Contains(note.Message, "SIMULATE") || !strings.Contains(note.Message, "HK.00700") {
		t.Fatalf("placed message = %q", note.Message)
	}
	unknown := OrderPlacedNotification(trdsrv.ExecutionOrder{})
	if unknown.BrokerID != "unknown" || unknown.Title != "券商 订单已提交" {
		t.Fatalf("unknown broker notification = %#v", unknown)
	}
}

func TestOrderLifecycleNotificationMapsSubmittedCancelledAndFilled(t *testing.T) {
	discovered := []string{"BROKER_SYNC_DISCOVERED", "BROKER_PUSH_DISCOVERED"}
	for _, eventType := range discovered {
		note, ok := OrderLifecycleNotification(
			trdsrv.ExecutionOrder{Status: trdsrv.OrderStatusSubmitted},
			&trdsrv.ExecutionOrderEvent{EventType: eventType},
		)
		if !ok || note.Category != "broker.order.place" {
			t.Fatalf("submitted %s notification = %#v, %v", eventType, note, ok)
		}
	}
	note, ok := OrderLifecycleNotification(
		trdsrv.ExecutionOrder{Status: trdsrv.OrderStatusCancelled},
		&trdsrv.ExecutionOrderEvent{},
	)
	if !ok || note.Category != "broker.order.cancel" || note.Level != "success" || !strings.Contains(note.Title, "撤单成功") {
		t.Fatalf("cancelled notification = %#v, %v", note, ok)
	}
	note, ok = OrderLifecycleNotification(
		trdsrv.ExecutionOrder{Status: trdsrv.OrderStatusFilled},
		&trdsrv.ExecutionOrderEvent{},
	)
	if !ok || note.Category != "broker.order.fill" || note.Level != "success" || !strings.Contains(note.Title, "成交成功") {
		t.Fatalf("filled notification = %#v, %v", note, ok)
	}
	if _, ok := OrderLifecycleNotification(
		trdsrv.ExecutionOrder{Status: "PENDING_SUBMIT"},
		&trdsrv.ExecutionOrderEvent{},
	); ok {
		t.Fatal("unknown order status produced notification")
	}
}

func TestExecutionOrderNotificationMessageOmitsBlankParts(t *testing.T) {
	blank := "  "
	message := ExecutionOrderNotificationMessage(trdsrv.ExecutionOrder{
		TradingEnvironment: "SIMULATE",
		Symbol:             &blank,
		Side:               &blank,
		BrokerOrderID:      &blank,
	})
	if message != "SIMULATE" {
		t.Fatalf("blank-part message = %q", message)
	}
}

func ptr[T any](value T) *T {
	return &value
}
