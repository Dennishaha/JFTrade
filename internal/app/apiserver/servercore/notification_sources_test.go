package servercore

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	live "github.com/jftrade/jftrade-main/internal/live"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestLiveNotificationFromBBGONotifyFormatsStringArgs(t *testing.T) {
	note := liveNotificationFromBBGONotify("strategy %s retry warning", "demo-grid")
	if note == nil {
		t.Fatal("expected note")
		return
	}
	if note.Title != "BBGO 通知" {
		t.Fatalf("title = %q", note.Title)
	}
	if note.Message != "strategy demo-grid retry warning" {
		t.Fatalf("message = %q", note.Message)
	}
	if note.Level != "warn" {
		t.Fatalf("level = %q", note.Level)
	}
	if note.Source != "bbgo.notify" {
		t.Fatalf("source = %q", note.Source)
	}
	if note.Category != "bbgo.notify" {
		t.Fatalf("category = %q", note.Category)
	}
	if strings.TrimSpace(note.At) == "" {
		t.Fatal("expected timestamp")
	}
}

func TestLiveNotificationFromBBGONotifyHandlesErrorsStringersAndForwardedNotes(t *testing.T) {
	if note := liveNotificationFromBBGONotify(nil); note != nil {
		t.Fatalf("nil bbgo notify = %#v, want nil", note)
	}
	forwarded := live.Notification{Title: "OpenD 连接状态变化", Message: "行情未登录，交易已登录。"}
	if note := liveNotificationFromBBGONotify(forwardedBBGONotification{note: forwarded}); note != nil {
		t.Fatalf("forwarded notification = %#v, want nil to avoid loops", note)
	}

	errNote := liveNotificationFromBBGONotify(assertStringer("risk engine timeout"), "retry")
	if errNote == nil || errNote.Level != "error" || errNote.Message != "risk engine timeout retry" {
		t.Fatalf("stringer note = %+v, want error-level timeout message", errNote)
	}
	errorObjNote := liveNotificationFromBBGONotify(assertError("order submit failed"))
	if errorObjNote == nil || errorObjNote.Title != "BBGO 错误" || errorObjNote.Level != "error" || errorObjNote.Message != "order submit failed" {
		t.Fatalf("error note = %+v", errorObjNote)
	}

	if text := liveNotificationText(live.Notification{Title: "标题"}); text != "标题" {
		t.Fatalf("title-only text = %q", text)
	}
	if text := liveNotificationText(live.Notification{Title: "标题", Message: "内容"}); text != "标题 - 内容" {
		t.Fatalf("title/message text = %q", text)
	}
	if !shouldForwardNotificationToBBGO(live.Notification{Level: "warn"}) ||
		!shouldForwardNotificationToBBGO(live.Notification{Level: "error"}) ||
		!shouldForwardNotificationToBBGO(live.Notification{Level: "success", Category: "broker.connection"}) ||
		shouldForwardNotificationToBBGO(live.Notification{Level: "info", Category: "broker.quota"}) {
		t.Fatal("shouldForwardNotificationToBBGO did not match broker alert policy")
	}
}

func TestBBGONotificationBridgeStartStopStringAndUpload(t *testing.T) {
	forwarded := forwardedBBGONotification{note: live.Notification{Title: "OpenD", Message: "connected"}}
	if got := forwarded.String(); got != "OpenD - connected" {
		t.Fatalf("forwarded String() = %q", got)
	}

	source := bbgoNotificationSource{}
	stop, err := source.Start(nil)
	if err != nil || stop != nil {
		t.Fatalf("Start nil sink = %v/%v, want nil nil", stop, err)
	}

	notifications := make(chan live.Notification, 4)
	stop, err = source.Start(func(note live.Notification) *live.Event {
		notifications <- note
		return &live.Event{Sequence: 1, At: note.At, Level: note.Level, Title: note.Title, Message: note.Message, Source: note.Source, Category: note.Category}
	})
	if err != nil || stop == nil {
		t.Fatalf("Start sink = %v/%v", stop, err)
	}
	dispatchBBGONotification(live.Notification{Title: "bridge", Message: "online"})
	select {
	case note := <-notifications:
		if note.Title != "bridge" || note.Message != "online" {
			t.Fatalf("dispatched note = %+v", note)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bridge notification")
	}

	liveSocketBBGONotifier{}.Upload(nil)
	liveSocketBBGONotifier{}.Upload(&bbgotypes.UploadFile{Caption: "report ready", FileType: bbgotypes.FileTypeDocument})
	select {
	case note := <-notifications:
		if note.Category != "bbgo.upload" || note.Message != "report ready" {
			t.Fatalf("upload note = %+v", note)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upload notification")
	}
	liveSocketBBGONotifier{}.Upload(&bbgotypes.UploadFile{FileType: bbgotypes.FileTypeText})
	select {
	case note := <-notifications:
		if note.Category != "bbgo.upload" || note.Message != "text" {
			t.Fatalf("upload fallback note = %+v", note)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upload fallback notification")
	}

	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	dispatchBBGONotification(live.Notification{Title: "after stop"})
	select {
	case note := <-notifications:
		t.Fatalf("received notification after stop: %+v", note)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestExchangeCalendarAlertRecordingHonorsNotificationSetting(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	alert := exchangecalendar.SourceAlert{
		SourceID: "nyse_official",
		Market:   "US",
		Level:    "warn",
		Kind:     "fetch_failed",
		Title:    "交易所日历源抓取失败",
		Message:  "US 市场日历源 nyse_official 抓取失败。",
	}

	server.recordExchangeCalendarAlert(alert)
	if got := len(server.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("notifications with default setting = %d, want 1", got)
	}

	var disabled jfsettings.ExchangeCalendarSettings
	if err := json.Unmarshal([]byte(`{"autoRefreshEnabled":true,"errorNotificationsEnabled":false,"refreshIntervalHours":24,"warmupMarkets":["US"]}`), &disabled); err != nil {
		t.Fatalf("Unmarshal settings: %v", err)
	}
	if _, err := store.SaveExchangeCalendarSettings(disabled); err != nil {
		t.Fatalf("SaveExchangeCalendarSettings: %v", err)
	}
	server.recordExchangeCalendarAlert(alert)
	if got := len(server.liveNotificationsAfter(0)); got != 1 {
		t.Fatalf("notifications after disabling = %d, want unchanged 1", got)
	}
}

func TestLiveNotificationFromExchangeCalendarAlertMapsSourceAndCategory(t *testing.T) {
	note := liveNotificationFromExchangeCalendarAlert(exchangecalendar.SourceAlert{
		SourceID: "nyse_official",
		Market:   "US",
		Level:    "error",
		Kind:     "structure_changed",
		Title:    "交易所日历源解析异常",
		Message:  "US 市场日历源 nyse_official 抓取成功但未解析到有效交易日。",
	})
	if note == nil {
		t.Fatal("expected note")
		return
	}
	if note.Level != "error" {
		t.Fatalf("level = %q", note.Level)
	}
	if note.Source != "exchange-calendars" {
		t.Fatalf("source = %q", note.Source)
	}
	if note.Category != "market.calendar.source" {
		t.Fatalf("category = %q", note.Category)
	}
	if strings.TrimSpace(note.At) == "" {
		t.Fatal("expected timestamp")
	}
}

type assertStringer string

func (s assertStringer) String() string { return string(s) }

type assertError string

func (e assertError) Error() string { return string(e) }
