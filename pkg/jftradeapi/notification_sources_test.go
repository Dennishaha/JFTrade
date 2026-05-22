package jftradeapi

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

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
		RetType: proto.Int32(0),
		S2C: &notifypb.S2C{
			Type: proto.Int32(int32(notifypb.NotifyType_NotifyType_APIQuota)),
			ApiQuota: &notifypb.APIQuota{
				Remain:    proto.Int32(5),
				OwnUsed:   proto.Int32(3),
				TotalUsed: proto.Int32(8),
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
