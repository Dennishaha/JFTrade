package servercore

import (
	"github.com/jftrade/jftrade-main/internal/app/apiserver/liveapp"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/tradingapp"
	livecore "github.com/jftrade/jftrade-main/internal/live"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func (s *serverApplication) notifyExecutionOrderPlaced(order trdsrv.ExecutionOrder) {
	s.emitExecutionNotification(tradingapp.OrderPlacedNotification(order))
}

func (s *serverApplication) notifyExecutionOrderLifecycle(
	order trdsrv.ExecutionOrder,
	event *trdsrv.ExecutionOrderEvent,
) {
	note, ok := tradingapp.OrderLifecycleNotification(order, event)
	if ok {
		s.emitExecutionNotification(note)
	}
}

func (s *serverApplication) emitExecutionNotification(note livecore.Notification) {
	s.recordLiveNotification(note)
	liveapp.ForwardToBBGO(note)
}
