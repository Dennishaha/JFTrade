package liveapp

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	livecore "github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

var (
	bbgoNotifierBridgeOnce  sync.Once
	bbgoNotifierBridgeMu    sync.RWMutex
	bbgoNotifierBridgeSeq   uint64
	bbgoNotifierBridgeSinks = map[uint64]func(livecore.Notification) *livecore.Event{}
)

type forwardedBBGONotification struct {
	note livecore.Notification
}

func (notification forwardedBBGONotification) String() string {
	return NotificationText(notification.note)
}

type liveSocketBBGONotifier struct{}

// BBGONotificationSource adapts the process-wide bbgo notification fanout to
// one replay publisher. Each source instance unregisters only its own sink.
type BBGONotificationSource struct{}

func (BBGONotificationSource) Start(sink livecore.PublishFunc) (livecore.StopFunc, error) {
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
	note := NotificationFromBBGO(obj, args...)
	if note != nil {
		dispatchBBGONotification(*note)
	}
}

func (liveSocketBBGONotifier) Upload(file *bbgotypes.UploadFile) {
	if file == nil {
		return
	}
	note := livecore.Notification{
		At: time.Now().UTC().Format(time.RFC3339Nano), Level: "info", Title: "BBGO 文件通知",
		Source: "bbgo.notify", Category: "bbgo.upload",
	}
	if caption := strings.TrimSpace(file.Caption); caption != "" {
		note.Message = caption
	} else {
		note.Message = string(file.FileType)
	}
	dispatchBBGONotification(note)
}

func dispatchBBGONotification(note livecore.Notification) {
	bbgoNotifierBridgeMu.RLock()
	sinks := make([]func(livecore.Notification) *livecore.Event, 0, len(bbgoNotifierBridgeSinks))
	for _, sink := range bbgoNotifierBridgeSinks {
		sinks = append(sinks, sink)
	}
	bbgoNotifierBridgeMu.RUnlock()
	for _, sink := range sinks {
		sink(note)
	}
}

// ForwardToBBGO forwards an application notification without allowing the
// process-wide notifier bridge to ingest it a second time.
func ForwardToBBGO(note livecore.Notification) {
	bbgo.Notify(forwardedBBGONotification{note: note})
}

func NotificationFromBBGO(obj any, args ...any) *livecore.Notification {
	if obj == nil {
		return nil
	}
	if _, ok := obj.(forwardedBBGONotification); ok {
		return nil
	}
	note := livecore.Notification{
		At: time.Now().UTC().Format(time.RFC3339Nano), Level: "info", Title: "BBGO 通知",
		Source: "bbgo.notify", Category: "bbgo.notify",
	}
	switch value := obj.(type) {
	case error:
		note.Level = "error"
		note.Title = "BBGO 错误"
		note.Message = strings.TrimSpace(value.Error())
	default:
		text := strings.TrimSpace(FormatBBGONotifyText(obj, args...))
		if text == "" {
			return nil
		}
		note.Level = inferBBGONotificationLevel(text)
		note.Message = text
	}
	if note.Message == "" {
		return nil
	}
	return &note
}

func FormatBBGONotifyText(obj any, args ...any) string {
	switch value := obj.(type) {
	case string:
		if len(args) == 0 {
			return value
		}
		formatted := fmt.Sprintf(value, args...)
		if formatted != value {
			return formatted
		}
		return strings.TrimSpace(value + " " + joinNotifyArgs(args...))
	case fmt.Stringer:
		if len(args) == 0 {
			return value.String()
		}
		return strings.TrimSpace(value.String() + " " + joinNotifyArgs(args...))
	default:
		if len(args) == 0 {
			return fmt.Sprint(obj)
		}
		return strings.TrimSpace(fmt.Sprint(obj) + " " + joinNotifyArgs(args...))
	}
}

func joinNotifyArgs(args ...any) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, strings.TrimSpace(fmt.Sprint(arg)))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func inferBBGONotificationLevel(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "panic"), strings.Contains(lower, "fatal"),
		strings.Contains(lower, "error"), strings.Contains(lower, "failed"),
		strings.Contains(lower, "timeout"), strings.Contains(text, "失败"),
		strings.Contains(text, "错误"), strings.Contains(text, "超时"):
		return "error"
	case strings.Contains(lower, "warn"), strings.Contains(lower, "risk"),
		strings.Contains(lower, "retry"), strings.Contains(text, "警告"),
		strings.Contains(text, "告警"), strings.Contains(text, "风险"):
		return "warn"
	default:
		return "info"
	}
}

func NotificationText(note livecore.Notification) string {
	if note.Message == "" {
		return note.Title
	}
	return note.Title + " - " + note.Message
}

func ShouldForwardToBBGO(note livecore.Notification) bool {
	return note.Level == "warn" || note.Level == "error" ||
		(note.Category == "broker.connection" && note.Level == "success")
}

func NotificationEventMap(event livecore.Event) map[string]any {
	payload := map[string]any{
		"type": "system.notification", "id": fmt.Sprintf("system-notification-%d", event.Sequence),
		"at": event.At, "level": event.Level, "title": event.Title, "source": event.Source,
		"brokerId": event.BrokerID, "category": event.Category,
	}
	if event.Message != "" {
		payload["message"] = event.Message
	}
	return payload
}

func DeliverNotification(
	sink func(livecore.Event) livecore.NotificationDelivery,
	event livecore.Event,
) (delivery livecore.NotificationDelivery) {
	delivery = livecore.NotificationNotDelivered(
		livecore.NotificationDeliveryUnavailable, "desktop system notifications are not configured",
	)
	if sink == nil {
		return delivery
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("JFTrade live notification sink failed: %v", recovered)
			delivery = livecore.NotificationNotDelivered(
				livecore.NotificationDeliveryFailed,
				fmt.Sprintf("desktop notification sink failed: %v", recovered),
			)
		}
	}()
	return sink(event)
}
