package tradingapp

import (
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestOrderLifecycleNotificationHandlesUnrelatedAndPartialFillEvents(t *testing.T) {
	if _, ok := OrderLifecycleNotification(trdsrv.ExecutionOrder{}, nil); ok {
		t.Fatal("nil order event produced notification")
	}
	if _, ok := OrderLifecycleNotification(
		trdsrv.ExecutionOrder{Status: trdsrv.OrderStatusSubmitted},
		&trdsrv.ExecutionOrderEvent{EventType: "OTHER"},
	); ok {
		t.Fatal("unrelated submitted event produced notification")
	}
	partial, ok := OrderLifecycleNotification(
		trdsrv.ExecutionOrder{Status: trdsrv.OrderStatusPartiallyFilled},
		&trdsrv.ExecutionOrderEvent{},
	)
	if !ok || partial.Level != "info" || partial.Category != "broker.order.fill" {
		t.Fatalf("partial fill notification = %#v, %v", partial, ok)
	}
	if note := baseExecutionNotification(trdsrv.ExecutionOrder{}, "category"); note.BrokerID != "unknown" {
		t.Fatalf("default notification broker = %#v", note)
	}
}

func TestExecutionOrderNotificationMessageIncludesAvailableIdentifiers(t *testing.T) {
	if got := ExecutionOrderNotificationMessage(trdsrv.ExecutionOrder{InternalOrderID: "exec-empty"}); got != "exec-empty" {
		t.Fatalf("empty order message = %q", got)
	}
	symbol, side, brokerID := "AAPL", "BUY", "broker-1"
	quantity, filled := 2.0, 1.0
	message := ExecutionOrderNotificationMessage(trdsrv.ExecutionOrder{
		TradingEnvironment: "SIMULATE", Symbol: &symbol, Side: &side,
		RequestedQuantity: &quantity, FilledQuantity: &filled, BrokerOrderID: &brokerID,
	})
	for _, part := range []string{"SIMULATE", "AAPL", "BUY", "qty", "filled", "brokerOrderId"} {
		if !strings.Contains(message, part) {
			t.Fatalf("full order message %q missing %q", message, part)
		}
	}
}
