package servercore

import (
	"context"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/liveapp"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	"github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func (s *serverApplication) ensureLiveNotificationBridge(ctx context.Context) {
	marketDataRuntime := s.runtimes.MarketData()
	if marketDataRuntime == nil || !s.futuCoordinator().Enabled() {
		return
	}
	go func() {
		bridgeCtx, cancel := context.WithTimeout(ctx, liveStreamConnectTimeout)
		defer cancel()
		jftradeErr1 := marketDataRuntime.EnsureSystemNotifications(bridgeCtx)
		besteffort.LogError(jftradeErr1)
	}()
}

func (s *serverApplication) handleFutuSystemNotification(note live.Notification) {
	s.recordLiveNotification(note)
	if liveapp.ShouldForwardToBBGO(note) {
		liveapp.ForwardToBBGO(note)
	}
}

func (s *serverApplication) recordLiveNotification(note live.Notification) *live.Event {
	event, _ := s.recordLiveNotificationWithDelivery(note)
	return event
}

func (s *serverApplication) recordLiveNotificationWithDelivery(note live.Notification) (*live.Event, live.NotificationDelivery) {
	publisher := s.runtimes.LiveNotifications()
	if publisher == nil {
		return nil, live.NotificationNotDelivered(
			live.NotificationDeliveryUnavailable, "live notification publisher is unavailable",
		)
	}
	event := publisher.Publish(note)
	delivery := live.NotificationNotDelivered(
		live.NotificationDeliveryUnavailable, "desktop system notifications are not configured",
	)
	if event == nil {
		return nil, delivery
	}
	payload := liveapp.NotificationEventMap(*event)
	payload["message"] = event.Message
	s.emitWorkflowEvent(assistantassembly.WorkflowEvent{
		ID: fmt.Sprintf("system-notification-%d", event.Sequence), Type: "system.notification",
		Source: "notification", EntityID: fmt.Sprintf("system-notification-%d", event.Sequence), At: event.At,
		Payload: payload,
	})
	return event, s.emitLiveNotificationSink(*event)
}

func (s *serverApplication) emitLiveNotificationSink(event live.Event) live.NotificationDelivery {
	if s == nil {
		return liveapp.DeliverNotification(nil, event)
	}
	return liveapp.DeliverNotification(s.runtimes.LiveNotificationSink(), event)
}

func (s *serverApplication) liveNotificationsAfter(sequence uint64) []live.Event {
	if s == nil {
		return nil
	}
	publisher := s.runtimes.LiveNotifications()
	if publisher == nil {
		return nil
	}
	return publisher.After(sequence)
}
