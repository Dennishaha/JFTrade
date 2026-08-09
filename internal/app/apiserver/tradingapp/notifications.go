package tradingapp

import (
	"fmt"
	"strings"
	"time"

	livecore "github.com/jftrade/jftrade-main/internal/live"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func OrderPlacedNotification(order trdsrv.ExecutionOrder) livecore.Notification {
	note := baseExecutionNotification(order, "broker.order.place")
	note.Level = "success"
	note.Title = executionBrokerLabel(order) + " 订单已提交"
	note.Message = ExecutionOrderNotificationMessage(order)
	return note
}

func OrderLifecycleNotification(
	order trdsrv.ExecutionOrder,
	event *trdsrv.ExecutionOrderEvent,
) (livecore.Notification, bool) {
	if event == nil {
		return livecore.Notification{}, false
	}
	status := strings.ToUpper(strings.TrimSpace(order.Status))
	switch status {
	case trdsrv.OrderStatusSubmitted, trdsrv.OrderStatusBrokerAccepted:
		if event.EventType != "BROKER_SYNC_DISCOVERED" && event.EventType != "BROKER_PUSH_DISCOVERED" {
			return livecore.Notification{}, false
		}
		return OrderPlacedNotification(order), true
	case trdsrv.OrderStatusCancelled:
		return executionStatusNotification(order, "broker.order.cancel", " 撤单成功", "success"), true
	case trdsrv.OrderStatusFilled:
		return executionStatusNotification(order, "broker.order.fill", " 成交成功", "success"), true
	case trdsrv.OrderStatusPartiallyFilled:
		return executionStatusNotification(order, "broker.order.fill", " 订单部分成交", "info"), true
	default:
		return livecore.Notification{}, false
	}
}

func executionStatusNotification(
	order trdsrv.ExecutionOrder,
	category string,
	titleSuffix string,
	level string,
) livecore.Notification {
	note := baseExecutionNotification(order, category)
	note.Level = level
	note.Title = executionBrokerLabel(order) + titleSuffix
	note.Message = ExecutionOrderNotificationMessage(order)
	return note
}

func baseExecutionNotification(order trdsrv.ExecutionOrder, category string) livecore.Notification {
	brokerID := strings.TrimSpace(order.BrokerID)
	if brokerID == "" {
		brokerID = "unknown"
	}
	return livecore.Notification{
		At: time.Now().UTC().Format(time.RFC3339Nano), Source: "execution-orders",
		BrokerID: brokerID, Category: category,
	}
}

func executionBrokerLabel(order trdsrv.ExecutionOrder) string {
	brokerID := strings.TrimSpace(order.BrokerID)
	if brokerID == "" {
		return "券商"
	}
	return strings.ToUpper(brokerID)
}

func ExecutionOrderNotificationMessage(order trdsrv.ExecutionOrder) string {
	parts := []string{}
	if order.TradingEnvironment != "" {
		parts = append(parts, order.TradingEnvironment)
	}
	if order.Symbol != nil && strings.TrimSpace(*order.Symbol) != "" {
		parts = append(parts, *order.Symbol)
	}
	if order.Side != nil && strings.TrimSpace(*order.Side) != "" {
		parts = append(parts, *order.Side)
	}
	if order.RequestedQuantity != nil && *order.RequestedQuantity > 0 {
		parts = append(parts, fmt.Sprintf("qty %.4f", *order.RequestedQuantity))
	}
	if order.FilledQuantity != nil && *order.FilledQuantity > 0 {
		parts = append(parts, fmt.Sprintf("filled %.4f", *order.FilledQuantity))
	}
	if order.BrokerOrderID != nil && strings.TrimSpace(*order.BrokerOrderID) != "" {
		parts = append(parts, "brokerOrderId "+*order.BrokerOrderID)
	}
	if len(parts) == 0 {
		return order.InternalOrderID
	}
	return strings.Join(parts, " | ")
}
