package servercore

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/gorilla/websocket"
	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	"github.com/shopspring/decimal"
)

func TestLiveWebSocketSendsHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer conn.Close()

	event := readLiveWebSocketEvent(t, conn)
	if event["type"] != "heartbeat" || event["at"] == "" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if got := int64(event["intervalMs"].(float64)); got != int64(15*time.Second/time.Millisecond) {
		t.Fatalf("intervalMs = %d", got)
	}
	if stale, _ := event["stale"].(bool); stale {
		t.Fatalf("unexpected stale heartbeat: %+v", event)
	}
}

func TestLiveWebSocketSendsSystemNotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.handleFutuSystemNotify(&notifypb.Response{
		RetType: new(int32(0)),
		S2C: &notifypb.S2C{
			Type: new(int32(notifypb.NotifyType_NotifyType_ProgramStatus)),
			ProgramStatus: &commonpb.ProgramStatus{
				Type:       commonpb.ProgramStatusType_ProgramStatusType_Ready.Enum(),
				StrExtDesc: new("OpenD ready for requests"),
			},
		},
	})

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer conn.Close()

	heartbeat := readLiveWebSocketEvent(t, conn)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	event := readLiveWebSocketEvent(t, conn)
	if event["type"] != "system.notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if event["title"] != "OpenD 已就绪" {
		t.Fatalf("title = %#v", event["title"])
	}
	if event["message"] != "已就绪：OpenD ready for requests" {
		t.Fatalf("message = %#v", event["message"])
	}
}

func TestLiveWebSocketSendsBBGONotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	bbgo.Notify("strategy %s started", "demo-grid")

	conn := dialLiveWebSocket(t, srv.URL)
	defer conn.Close()

	heartbeat := readLiveWebSocketEvent(t, conn)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	event := readLiveWebSocketEvent(t, conn)
	if event["type"] != "system.notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if event["title"] != "BBGO 通知" {
		t.Fatalf("title = %#v", event["title"])
	}
	if event["message"] != "strategy demo-grid started" {
		t.Fatalf("message = %#v", event["message"])
	}
}

func TestLiveWebSocketHeartbeatReportsStaleMarketData(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.marketdataSvc.AcquireSubscription(context.Background(), "test-live-heartbeat", []mdsrv.InstrumentRef{
		{Market: "HK", Symbol: "00700"},
	}); err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	server.marketdataSvc.Seed(marketTickSample{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        mustDecimal("320.5"),
		Bid:          mustDecimal("320.4"),
		Ask:          mustDecimal("320.6"),
		ObservedAt:   time.Now().UTC().Add(-(liveHeartbeatStaleThreshold + 500*time.Millisecond)).Format(time.RFC3339Nano),
		QuoteAt:      time.Now().UTC().Add(-(liveHeartbeatStaleThreshold + 500*time.Millisecond)).Format(time.RFC3339Nano),
		Source:       "bbgo:futu",
	})

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer conn.Close()

	heartbeat := readLiveWebSocketEvent(t, conn)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}
	if stale, _ := heartbeat["stale"].(bool); !stale {
		t.Fatalf("expected stale heartbeat, got %+v", heartbeat)
	}
}

func TestLiveWebSocketInitialMarketTickRefreshesObservedAt(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	quoteAt := time.Now().UTC().Add(-300 * time.Millisecond).Truncate(time.Millisecond)
	observedAt := quoteAt.Add(-100 * time.Millisecond)
	server.marketdataSvc.Seed(marketTickSample{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        mustDecimal("320.5"),
		Bid:          mustDecimal("320.4"),
		Ask:          mustDecimal("320.6"),
		Volume:       1282000,
		Turnover:     mustDecimal("411000000"),
		QuoteAt:      quoteAt.Format(time.RFC3339Nano),
		ObservedAt:   observedAt.Format(time.RFC3339Nano),
		Source:       "bbgo:futu",
		Session:      "regular",
	})

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer conn.Close()

	if err := conn.WriteJSON(liveWebSocketClientMessage{
		Type: "subscribe",
		Subscriptions: liveWebSocketSubscriptions{
			ActiveInstruments: []string{"HK.00700"},
		},
	}); err != nil {
		t.Fatalf("subscribe live websocket: %v", err)
	}

	_ = readLiveWebSocketEvent(t, conn)
	event := readLiveWebSocketEvent(t, conn)
	if event["type"] != "market-data.tick" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	snapshot, ok := event["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot = %#v", event["snapshot"])
	}
	if snapshot["observedAt"] != event["at"] {
		t.Fatalf("snapshot.observedAt = %#v, event.at = %#v", snapshot["observedAt"], event["at"])
	}
	parsedObservedAt := httpserver.ParseQueryTime(event["at"].(string), time.Time{})
	if parsedObservedAt.Before(observedAt) {
		t.Fatalf("event at = %s, want >= %s", parsedObservedAt, observedAt)
	}
}

func TestLiveWebSocketSendsConsoleRefresh(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer conn.Close()

	if err := conn.WriteJSON(liveWebSocketClientMessage{
		Type: "subscribe",
		Subscriptions: liveWebSocketSubscriptions{
			ConsoleRefresh: true,
		},
	}); err != nil {
		t.Fatalf("subscribe live websocket: %v", err)
	}

	_ = readLiveWebSocketEvent(t, conn)
	event := readLiveWebSocketEvent(t, conn)
	if event["type"] != "console.refresh" {
		t.Fatalf("unexpected console refresh event: %+v", event)
	}
	if event["checkedAt"] == "" {
		t.Fatalf("missing checkedAt: %+v", event)
	}
}

func dialLiveWebSocket(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/v1/ws/live"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial live websocket: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	return conn
}

func readLiveWebSocketEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	return event
}

func mustDecimal(value string) decimal.Decimal {
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		panic(err)
	}
	return parsed
}
