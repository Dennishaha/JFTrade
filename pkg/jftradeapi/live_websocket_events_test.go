package jftradeapi

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bbgo "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
)

func TestLiveWebSocketSendsHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/live"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial live websocket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if event["type"] != "heartbeat" || event["at"] == "" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestLiveWebSocketSendsSystemNotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	server.handleFutuSystemNotify(&notifypb.Response{
		RetType: proto.Int32(0),
		S2C: &notifypb.S2C{
			Type: proto.Int32(int32(notifypb.NotifyType_NotifyType_ProgramStatus)),
			ProgramStatus: &commonpb.ProgramStatus{
				Type:       commonpb.ProgramStatusType_ProgramStatusType_Ready.Enum(),
				StrExtDesc: proto.String("OpenD ready for requests"),
			},
		},
	})

	srv := httptest.NewServer(server)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/live"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial live websocket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var heartbeat map[string]any
	if err := conn.ReadJSON(&heartbeat); err != nil {
		t.Fatalf("Read heartbeat: %v", err)
	}
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("Read notification: %v", err)
	}
	if event["type"] != "system.notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if event["title"] != "OpenD 已就绪" {
		t.Fatalf("title = %#v", event["title"])
	}
	if event["message"] != "已就绪：OpenD ready for requests" {
		t.Fatalf("message = %#v", event["message"])
	}
	if event["level"] != "success" {
		t.Fatalf("level = %#v", event["level"])
	}
	if event["source"] != "futu-opend" {
		t.Fatalf("source = %#v", event["source"])
	}
	if event["brokerId"] != "futu" {
		t.Fatalf("brokerId = %#v", event["brokerId"])
	}
}

func TestLiveWebSocketSendsBBGONotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	bbgo.Notify("strategy %s started", "demo-grid")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/live"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial live websocket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var heartbeat map[string]any
	if err := conn.ReadJSON(&heartbeat); err != nil {
		t.Fatalf("Read heartbeat: %v", err)
	}
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("Read notification: %v", err)
	}
	if event["type"] != "system.notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if event["title"] != "BBGO 通知" {
		t.Fatalf("title = %#v", event["title"])
	}
	if event["message"] != "strategy demo-grid started" {
		t.Fatalf("message = %#v", event["message"])
	}
	if event["level"] != "info" {
		t.Fatalf("level = %#v", event["level"])
	}
	if event["source"] != "bbgo.notify" {
		t.Fatalf("source = %#v", event["source"])
	}
	if event["category"] != "bbgo.notify" {
		t.Fatalf("category = %#v", event["category"])
	}
}
