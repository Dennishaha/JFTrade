package servercore

import (
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

func TestLiveNotificationFromBBGONotifyFormatsStringArgs(t *testing.T) {
	note := liveNotificationFromBBGONotify("strategy %s retry warning", "demo-grid")
	if note == nil {
		t.Fatal("expected note")
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

func TestLiveNotificationFromFutuResponseMapsAPIQuota(t *testing.T) {
	note := liveNotificationFromFutuResponse(&notifypb.Response{
		RetType: new(int32(0)),
		S2C: &notifypb.S2C{
			Type: new(int32(notifypb.NotifyType_NotifyType_APIQuota)),
			ApiQuota: &notifypb.APIQuota{
				Remain:    new(int32(5)),
				OwnUsed:   new(int32(3)),
				TotalUsed: new(int32(8)),
			},
		},
	})
	if note == nil {
		t.Fatal("expected note")
	}
	if note.Title != "Futu API 订阅额度更新" {
		t.Fatalf("title = %q", note.Title)
	}
	if note.Level != "warn" {
		t.Fatalf("level = %q", note.Level)
	}
	if note.Source != "futu-opend" {
		t.Fatalf("source = %q", note.Source)
	}
	if note.BrokerID != "futu" {
		t.Fatalf("brokerId = %q", note.BrokerID)
	}
	if note.Category != "broker.quota" {
		t.Fatalf("category = %q", note.Category)
	}
	if note.Message != "剩余 5，当前连接已用 3，总已用 8。" {
		t.Fatalf("message = %q", note.Message)
	}
	if strings.TrimSpace(note.At) == "" {
		t.Fatal("expected timestamp")
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
