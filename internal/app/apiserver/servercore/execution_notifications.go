package servercore

import (
	"fmt"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
)

func (s *Server) notifyExecutionOrderPlaced(order executionOrderSummaryResponse) {
	note := baseExecutionNotification(order, "broker.order.place")
	note.Level = "success"
	note.Title = "Futu 订单已提交"
	note.Message = executionOrderNotificationMessage(order)
	s.emitExecutionNotification(note)
}

func (s *Server) notifyExecutionOrderLifecycle(order executionOrderSummaryResponse, event *executionOrderEventResponse) {
	if event == nil {
		return
	}
	note, ok := executionNotificationForStatus(order, event)
	if !ok {
		return
	}
	s.emitExecutionNotification(note)
}

func (s *Server) emitExecutionNotification(note liveNotification) {
	s.recordLiveNotification(note)
	bbgo.Notify(forwardedBBGONotification{note: note})
}

func executionNotificationForStatus(order executionOrderSummaryResponse, event *executionOrderEventResponse) (liveNotification, bool) {
	status := strings.ToUpper(strings.TrimSpace(order.Status))
	switch status {
	case "SUBMITTED":
		if event.EventType != "BROKER_SYNC_DISCOVERED" && event.EventType != "BROKER_PUSH_DISCOVERED" {
			return liveNotification{}, false
		}
		note := baseExecutionNotification(order, "broker.order.place")
		note.Level = "success"
		note.Title = "Futu 订单已提交"
		note.Message = executionOrderNotificationMessage(order)
		return note, true
	case "CANCELLED_ALL", "CANCELLED_PART":
		note := baseExecutionNotification(order, "broker.order.cancel")
		note.Level = "success"
		note.Title = "Futu 撤单成功"
		note.Message = executionOrderNotificationMessage(order)
		return note, true
	case "FILLED_ALL":
		note := baseExecutionNotification(order, "broker.order.fill")
		note.Level = "success"
		note.Title = "Futu 成交成功"
		note.Message = executionOrderNotificationMessage(order)
		return note, true
	case "FILLED_PART":
		note := baseExecutionNotification(order, "broker.order.fill")
		note.Level = "info"
		note.Title = "Futu 订单部分成交"
		note.Message = executionOrderNotificationMessage(order)
		return note, true
	default:
		return liveNotification{}, false
	}
}

func baseExecutionNotification(order executionOrderSummaryResponse, category string) liveNotification {
	brokerID := order.BrokerID
	if strings.TrimSpace(brokerID) == "" {
		brokerID = "futu"
	}
	return liveNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Source:   "execution-orders",
		BrokerID: brokerID,
		Category: category,
	}
}

func executionOrderNotificationMessage(order executionOrderSummaryResponse) string {
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
