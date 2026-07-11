package servercore

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/live"
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
	disableTestExchangeCalendarAutoRefresh(t, firstStore)
	disableTestExchangeCalendarAutoRefresh(t, secondStore)
	first := NewServer(firstStore)
	second := NewServer(secondStore)
	t.Cleanup(func() { jftradeErr1 := first.Close(); jftradeCheckTestError(t, jftradeErr1) })
	t.Cleanup(func() { jftradeErr2 := second.Close(); jftradeCheckTestError(t, jftradeErr2) })

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

func TestRecordLiveNotificationCallsSink(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	disableTestExchangeCalendarAutoRefresh(t, store)
	server := NewServer(store)
	t.Cleanup(func() { jftradeErr1 := server.Close(); jftradeCheckTestError(t, jftradeErr1) })

	var got liveNotificationEvent
	server.liveNotificationSink = func(event liveNotificationEvent) live.NotificationDelivery {
		got = event
		return live.NotificationDelivered("sent")
	}

	event := server.recordLiveNotification(liveNotification{Level: "warn", Title: "Risk", Message: "blocked", Category: "execution.order"})
	if event == nil {
		t.Fatal("recordLiveNotification event = nil")
	}
	if got.Sequence != event.Sequence || got.Title != "Risk" || got.Category != "execution.order" {
		t.Fatalf("sink event = %#v, want sequence %d title/category", got, event.Sequence)
	}
}

func TestSystemNotificationTestRouteReturnsDeliveryStatus(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	disableTestExchangeCalendarAutoRefresh(t, store)
	server := NewServer(store)
	t.Cleanup(func() { jftradeErr1 := server.Close(); jftradeCheckTestError(t, jftradeErr1) })
	server.auth.enabled = false
	server.liveNotificationSink = func(liveNotificationEvent) live.NotificationDelivery {
		return live.NotificationDelivered("sent to operating system")
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/settings/system-notifications/test", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`"event"`, `"delivery"`, `"status":"delivered"`, `"delivered":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %s", body, want)
		}
	}
}

func TestRecordLiveNotificationSinkPanicDoesNotDropEvent(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	disableTestExchangeCalendarAutoRefresh(t, store)
	server := NewServer(store)
	t.Cleanup(func() { jftradeErr1 := server.Close(); jftradeCheckTestError(t, jftradeErr1) })

	server.liveNotificationSink = func(liveNotificationEvent) live.NotificationDelivery {
		panic("desktop notification failed")
	}

	event := server.recordLiveNotification(liveNotification{Level: "error", Title: "OpenD"})
	if event == nil {
		t.Fatal("recordLiveNotification event = nil")
	}
	if got := len(server.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("live notifications after sink panic = %d, want 1", got)
	}
}
