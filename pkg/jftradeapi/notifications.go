package jftradeapi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bbgo "github.com/c9s/bbgo/pkg/bbgo"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

const (
	maxLiveNotificationEvents = 200
	liveNotificationRetention = 24 * time.Hour
)

var (
	bbgoNotifierBridgeOnce  sync.Once
	bbgoNotifierBridgeMu    sync.RWMutex
	bbgoNotifierBridgeSeq   uint64
	bbgoNotifierBridgeSinks = map[uint64]func(liveNotification) *liveNotificationEvent{}
)

type liveNotification struct {
	At       string
	Level    string
	Title    string
	Message  string
	Source   string
	BrokerID string
	Category string
}

type liveNotificationEvent struct {
	Sequence   uint64
	RecordedAt time.Time
	At         string
	Level      string
	Title      string
	Message    string
	Source     string
	BrokerID   string
	Category   string
}

type forwardedBBGONotification struct {
	note liveNotification
}

func (notification forwardedBBGONotification) String() string {
	return notification.note.text()
}

type liveSocketBBGONotifier struct{}

func registerBBGONotificationSink(sink func(liveNotification) *liveNotificationEvent) uint64 {
	if sink == nil {
		return 0
	}
	bbgoNotifierBridgeOnce.Do(func() {
		bbgo.Notification.AddNotifier(liveSocketBBGONotifier{})
	})
	id := atomic.AddUint64(&bbgoNotifierBridgeSeq, 1)
	bbgoNotifierBridgeMu.Lock()
	bbgoNotifierBridgeSinks[id] = sink
	bbgoNotifierBridgeMu.Unlock()
	return id
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
	go func() {
		bridgeCtx, cancel := context.WithTimeout(ctx, liveStreamConnectTimeout)
		defer cancel()
		_ = exchange.EnsureSystemNotifications(bridgeCtx)
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
	return s.liveNotifications.record(note)
}

func (s *Server) liveNotificationsAfter(sequence uint64) []liveNotificationEvent {
	return s.liveNotifications.after(sequence)
}

func (s *Server) writeLiveNotifications(writer liveEventWriter, lastSequence *uint64) error {
	events := s.liveNotificationsAfter(*lastSequence)
	for _, event := range events {
		if err := writer.WriteEvent(liveNotificationEventMap(event)); err != nil {
			return err
		}
		*lastSequence = event.Sequence
	}
	return nil
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

func (note liveNotification) text() string {
	if note.Message == "" {
		return note.Title
	}
	return note.Title + " - " + note.Message
}

func shouldForwardNotificationToBBGO(note liveNotification) bool {
	return note.Level == "warn" || note.Level == "error" || (note.Category == "broker.connection" && note.Level == "success")
}
