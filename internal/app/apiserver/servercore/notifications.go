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

	"github.com/jftrade/jftrade-main/internal/live"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

var (
	bbgoNotifierBridgeOnce  sync.Once
	bbgoNotifierBridgeMu    sync.RWMutex
	bbgoNotifierBridgeSeq   uint64
	bbgoNotifierBridgeSinks = map[uint64]func(liveNotification) *liveNotificationEvent{}
)

type liveNotification = live.Notification
type liveNotificationEvent = live.Event

type forwardedBBGONotification struct {
	note liveNotification
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
	note := liveNotification{
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

func dispatchBBGONotification(note liveNotification) {
	bbgoNotifierBridgeMu.RLock()
	sinks := make([]func(liveNotification) *liveNotificationEvent, 0, len(bbgoNotifierBridgeSinks))
	for _, sink := range bbgoNotifierBridgeSinks {
		sinks = append(sinks, sink)
	}
	bbgoNotifierBridgeMu.RUnlock()
	for _, sink := range sinks {
		sink(note)
	}
}

func (s *Server) ensureLiveNotificationBridge(ctx context.Context) {
	exchange := s.futuExchange()
	if exchange == nil {
		return
	}
	go func() {
		bridgeCtx, cancel := context.WithTimeout(ctx, liveStreamConnectTimeout)
		defer cancel()
		jftradeErr1 := exchange.EnsureSystemNotifications(bridgeCtx)
		besteffort.LogError(jftradeErr1)
	}()
}

func (s *Server) handleFutuSystemNotify(response *notifypb.Response) {
	note := liveNotificationFromFutuResponse(response)
	if note == nil {
		return
	}
	s.recordLiveNotification(*note)
	if shouldForwardNotificationToBBGO(*note) {
		bbgo.Notify(forwardedBBGONotification{note: *note})
	}
}

func (s *Server) recordLiveNotification(note liveNotification) *liveNotificationEvent {
	event, _ := s.recordLiveNotificationWithDelivery(note)
	return event
}

func (s *Server) recordLiveNotificationWithDelivery(note liveNotification) (*liveNotificationEvent, live.NotificationDelivery) {
	event := s.liveNotifications.Publish(note)
	delivery := live.NotificationNotDelivered(live.NotificationDeliveryUnavailable, "desktop system notifications are not configured")
	if event != nil {
		s.emitWorkflowEvent(jfadk.WorkflowEvent{
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

func (s *Server) emitLiveNotificationSink(event liveNotificationEvent) (delivery live.NotificationDelivery) {
	delivery = live.NotificationNotDelivered(live.NotificationDeliveryUnavailable, "desktop system notifications are not configured")
	if s == nil || s.liveNotificationSink == nil {
		return delivery
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("JFTrade live notification sink failed: %v", recovered)
			delivery = live.NotificationNotDelivered(live.NotificationDeliveryFailed, fmt.Sprintf("desktop notification sink failed: %v", recovered))
		}
	}()
	return s.liveNotificationSink(event)
}

func (s *Server) liveNotificationsAfter(sequence uint64) []liveNotificationEvent {
	return s.liveNotifications.After(sequence)
}

func liveNotificationText(note liveNotification) string {
	if note.Message == "" {
		return note.Title
	}
	return note.Title + " - " + note.Message
}

func liveNotificationEventMap(event liveNotificationEvent) map[string]any {
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

func shouldForwardNotificationToBBGO(note liveNotification) bool {
	return note.Level == "warn" || note.Level == "error" || (note.Category == "broker.connection" && note.Level == "success")
}
