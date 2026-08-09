package liveapp

import (
	"strings"
	"testing"
	"time"

	livecore "github.com/jftrade/jftrade-main/internal/live"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestBBGONotificationMappingPreservesAlertSemantics(t *testing.T) {
	note := NotificationFromBBGO("strategy %s retry warning", "demo-grid")
	if note == nil || note.Title != "BBGO 通知" || note.Message != "strategy demo-grid retry warning" || note.Level != "warn" {
		t.Fatalf("mapped string notification = %#v", note)
	}
	if note.Source != "bbgo.notify" || note.Category != "bbgo.notify" || strings.TrimSpace(note.At) == "" {
		t.Fatalf("mapped notification metadata = %#v", note)
	}
	if NotificationFromBBGO(nil) != nil {
		t.Fatal("nil bbgo notification was accepted")
	}
	errNote := NotificationFromBBGO(assertStringer("risk engine timeout"), "retry")
	if errNote == nil || errNote.Level != "error" || errNote.Message != "risk engine timeout retry" {
		t.Fatalf("stringer notification = %#v", errNote)
	}
	errorNote := NotificationFromBBGO(assertError("order submit failed"))
	if errorNote == nil || errorNote.Title != "BBGO 错误" || errorNote.Level != "error" || errorNote.Message != "order submit failed" {
		t.Fatalf("error notification = %#v", errorNote)
	}
	if NotificationText(livecore.Notification{Title: "标题"}) != "标题" ||
		NotificationText(livecore.Notification{Title: "标题", Message: "内容"}) != "标题 - 内容" {
		t.Fatal("notification text formatting changed")
	}
	if !ShouldForwardToBBGO(livecore.Notification{Level: "warn"}) ||
		!ShouldForwardToBBGO(livecore.Notification{Level: "error"}) ||
		!ShouldForwardToBBGO(livecore.Notification{Level: "success", Category: "broker.connection"}) ||
		ShouldForwardToBBGO(livecore.Notification{Level: "info", Category: "broker.quota"}) {
		t.Fatal("notification forwarding policy changed")
	}
}

func TestBBGONotificationSourceStartsStopsAndMapsUploads(t *testing.T) {
	notifications := make(chan livecore.Notification, 4)
	source := BBGONotificationSource{}
	stop, err := source.Start(nil)
	if err != nil || stop != nil {
		t.Fatalf("Start nil sink = %v/%v", stop, err)
	}
	stop, err = source.Start(func(note livecore.Notification) *livecore.Event {
		notifications <- note
		return &livecore.Event{Sequence: 1, At: note.At, Level: note.Level, Title: note.Title, Message: note.Message, Source: note.Source, Category: note.Category}
	})
	if err != nil || stop == nil {
		t.Fatalf("Start sink = %v/%v", stop, err)
	}
	dispatchBBGONotification(livecore.Notification{Title: "bridge", Message: "online"})
	assertNotification(t, notifications, "bridge", "online", "")

	forwarded := forwardedBBGONotification{note: livecore.Notification{Title: "OpenD", Message: "connected"}}
	if forwarded.String() != "OpenD - connected" || NotificationFromBBGO(forwarded) != nil {
		t.Fatal("forwarded notification loop guard failed")
	}
	liveSocketBBGONotifier{}.Upload(nil)
	liveSocketBBGONotifier{}.Upload(&bbgotypes.UploadFile{Caption: "report ready", FileType: bbgotypes.FileTypeDocument})
	assertNotification(t, notifications, "", "report ready", "bbgo.upload")
	liveSocketBBGONotifier{}.Upload(&bbgotypes.UploadFile{FileType: bbgotypes.FileTypeText})
	assertNotification(t, notifications, "", "text", "bbgo.upload")

	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	dispatchBBGONotification(livecore.Notification{Title: "after stop"})
	select {
	case note := <-notifications:
		t.Fatalf("received notification after stop: %+v", note)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestDeliverNotificationConvertsSinkPanicsToDeliveryFailure(t *testing.T) {
	delivery := DeliverNotification(func(livecore.Event) livecore.NotificationDelivery {
		panic("desktop unavailable")
	}, livecore.Event{})
	if delivery.Status != livecore.NotificationDeliveryFailed {
		t.Fatalf("panic delivery = %#v", delivery)
	}
}

func assertNotification(t *testing.T, notifications <-chan livecore.Notification, title, message, category string) {
	t.Helper()
	select {
	case note := <-notifications:
		if (title != "" && note.Title != title) || note.Message != message || (category != "" && note.Category != category) {
			t.Fatalf("notification = %+v", note)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

type assertStringer string

func (s assertStringer) String() string { return string(s) }

type assertError string

func (e assertError) Error() string { return string(e) }
