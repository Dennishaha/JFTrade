package servercore

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestServerCloseUnregistersOnlyItsBBGONotificationSink(t *testing.T) {
	firstStore, err := NewSettingsStore(filepath.Join(t.TempDir(), "first", "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore(first): %v", err)
	}
	secondStore, err := NewSettingsStore(filepath.Join(t.TempDir(), "second", "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore(second): %v", err)
	}
	first := NewServer(firstStore)
	second := NewServer(secondStore)
	t.Cleanup(func() { _ = first.Close() })
	t.Cleanup(func() { _ = second.Close() })

	dispatchBBGONotification(liveNotification{Title: "before close"})
	if got := len(first.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("first notifications before close = %d", got)
	}
	if got := len(second.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("second notifications before close = %d", got)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first second Close: %v", err)
	}
	dispatchBBGONotification(liveNotification{Title: "after close"})

	if got := len(first.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("closed server notifications = %d", got)
	}
	events := second.liveNotificationsAfter(0)
	if len(events) != 2 || events[1].Title != "after close" {
		t.Fatalf("active server notifications = %#v", events)
	}
}

func TestLiveNotificationEventMapContract(t *testing.T) {
	event := liveNotificationEvent{
		Sequence: 7,
		At:       "2026-06-14T08:09:10Z",
		Level:    "warn",
		Title:    "BBGO notification",
		Message:  "retrying",
		Source:   "bbgo.notify",
		BrokerID: "futu",
		Category: "bbgo.notify",
	}
	got := liveNotificationEventMap(event)
	want := map[string]any{
		"type":     "system.notification",
		"id":       "system-notification-7",
		"at":       "2026-06-14T08:09:10Z",
		"level":    "warn",
		"title":    "BBGO notification",
		"message":  "retrying",
		"source":   "bbgo.notify",
		"brokerId": "futu",
		"category": "bbgo.notify",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event map = %#v, want %#v", got, want)
	}

	event.Message = ""
	delete(want, "message")
	if got := liveNotificationEventMap(event); !reflect.DeepEqual(got, want) {
		t.Fatalf("event map without message = %#v, want %#v", got, want)
	}
}
