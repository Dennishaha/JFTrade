package servercore

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"

	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	"github.com/jftrade/jftrade-main/internal/live"
)

var (
	bbgoNotifierBridgeOnce  sync.Once
	bbgoNotifierBridgeMu    sync.RWMutex
	bbgoNotifierBridgeSeq   uint64
	bbgoNotifierBridgeSinks = map[uint64]func(live.Notification) *live.Event{}
)

type forwardedBBGONotification struct {
	note live.Notification
}

func (notification forwardedBBGONotification) String() string {
	return liveNotificationText(notification.note)
}

type liveSocketBBGONotifier struct{}

type bbgoNotificationSource struct{}

func (bbgoNotificationSource) Start(sink live.PublishFunc) (live.StopFunc, error) {
	if sink == nil {
		return nil, nil
	}
	bbgoNotifierBridgeOnce.Do(func() {
		bbgo.Notification.AddNotifier(liveSocketBBGONotifier{})
	})
	id := atomic.AddUint64(&bbgoNotifierBridgeSeq, 1)
	bbgoNotifierBridgeMu.Lock()
	bbgoNotifierBridgeSinks[id] = sink
	bbgoNotifierBridgeMu.Unlock()
	var once sync.Once
	return func() error {
		once.Do(func() {
			bbgoNotifierBridgeMu.Lock()
			delete(bbgoNotifierBridgeSinks, id)
			bbgoNotifierBridgeMu.Unlock()
		})
		return nil
	}, nil
}

func (liveSocketBBGONotifier) Notify(obj any, args ...any) {
	note := liveNotificationFromBBGONotify(obj, args...)
	if note == nil {
		return
	}
	dispatchBBGONotification(*note)
}

func (liveSocketBBGONotifier) Upload(file *bbgotypes.UploadFile) {
	if file == nil {
		return
	}
	note := live.Notification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    "info",
		Title:    "BBGO 文件通知",
		Source:   "bbgo.notify",
		Category: "bbgo.upload",
	}
	if caption := strings.TrimSpace(file.Caption); caption != "" {
		note.Message = caption
	} else {
		note.Message = string(file.FileType)
	}
	dispatchBBGONotification(note)
}

func dispatchBBGONotification(note live.Notification) {
	bbgoNotifierBridgeMu.RLock()
	sinks := make([]func(live.Notification) *live.Event, 0, len(bbgoNotifierBridgeSinks))
	for _, sink := range bbgoNotifierBridgeSinks {
		sinks = append(sinks, sink)
	}
	bbgoNotifierBridgeMu.RUnlock()
	for _, sink := range sinks {
		sink(note)
	}
}

func (s *serverApplication) ensureLiveNotificationBridge(ctx context.Context) {
	marketDataRuntime := s.runtimes.MarketData()
	if marketDataRuntime == nil || !s.futuIntegrationEnabled() {
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
	if shouldForwardNotificationToBBGO(note) {
		bbgo.Notify(forwardedBBGONotification{note: note})
	}
}

func (s *serverApplication) recordLiveNotification(note live.Notification) *live.Event {
	event, _ := s.recordLiveNotificationWithDelivery(note)
	return event
}

func (s *serverApplication) recordLiveNotificationWithDelivery(note live.Notification) (*live.Event, live.NotificationDelivery) {
	publisher := s.runtimes.LiveNotifications()
	if publisher == nil {
		return nil, live.NotificationNotDelivered(live.NotificationDeliveryUnavailable, "live notification publisher is unavailable")
	}
	event := publisher.Publish(note)
	delivery := live.NotificationNotDelivered(live.NotificationDeliveryUnavailable, "desktop system notifications are not configured")
	if event != nil {
		s.emitWorkflowEvent(assistantassembly.WorkflowEvent{
			ID:       fmt.Sprintf("system-notification-%d", event.Sequence),
			Type:     "system.notification",
			Source:   "notification",
			EntityID: fmt.Sprintf("system-notification-%d", event.Sequence),
			At:       event.At,
			Payload: map[string]any{
				"type":     "system.notification",
				"id":       fmt.Sprintf("system-notification-%d", event.Sequence),
				"at":       event.At,
				"level":    event.Level,
				"title":    event.Title,
				"message":  event.Message,
				"source":   event.Source,
				"brokerId": event.BrokerID,
				"category": event.Category,
			},
		})
		delivery = s.emitLiveNotificationSink(*event)
	}
	return event, delivery
}

func (s *serverApplication) emitLiveNotificationSink(event live.Event) (delivery live.NotificationDelivery) {
	delivery = live.NotificationNotDelivered(live.NotificationDeliveryUnavailable, "desktop system notifications are not configured")
	if s == nil {
		return delivery
	}
	sink := s.runtimes.LiveNotificationSink()
	if sink == nil {
		return delivery
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("JFTrade live notification sink failed: %v", recovered)
			delivery = live.NotificationNotDelivered(live.NotificationDeliveryFailed, fmt.Sprintf("desktop notification sink failed: %v", recovered))
		}
	}()
	return sink(event)
}

func (s *serverApplication) liveNotificationsAfter(sequence uint64) []live.Event {
	publisher := s.runtimes.LiveNotifications()
	if publisher == nil {
		return nil
	}
	return publisher.After(sequence)
}

func liveNotificationText(note live.Notification) string {
	if note.Message == "" {
		return note.Title
	}
	return note.Title + " - " + note.Message
}

func liveNotificationEventMap(event live.Event) map[string]any {
	payload := map[string]any{
		"type":     "system.notification",
		"id":       fmt.Sprintf("system-notification-%d", event.Sequence),
		"at":       event.At,
		"level":    event.Level,
		"title":    event.Title,
		"source":   event.Source,
		"brokerId": event.BrokerID,
		"category": event.Category,
	}
	if event.Message != "" {
		payload["message"] = event.Message
	}
	return payload
}

func shouldForwardNotificationToBBGO(note live.Notification) bool {
	return note.Level == "warn" || note.Level == "error" || (note.Category == "broker.connection" && note.Level == "success")
}
