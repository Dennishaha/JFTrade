package servercore

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
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
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	event := readLiveWebSocketEvent(t, conn)
	payload := liveWebSocketPayload(t, event, "heartbeat")
	if event["source"] != "system" || payload["at"] == "" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if got := int64(jftradeCheckedTypeAssertion[float64](payload["intervalMs"])); got != int64(15*time.Second/time.Millisecond) {
		t.Fatalf("intervalMs = %d", got)
	}
	if stale := jftradeCheckedTypeAssertion[bool](payload["stale"]); stale {
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
			ProgramStatus: &notifypb.ProgramStatus{
				ProgramStatus: &commonpb.ProgramStatus{
					Type:       commonpb.ProgramStatusType_ProgramStatusType_Ready.Enum(),
					StrExtDesc: new("OpenD ready for requests"),
				},
			},
		},
	})

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	conn := dialLiveWebSocket(t, srv.URL)
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	heartbeat := readLiveWebSocketEvent(t, conn)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	event := readLiveWebSocketEvent(t, conn)
	payload := liveWebSocketPayload(t, event, "system.notification")
	if event["source"] != "notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if payload["title"] != "OpenD 已就绪" {
		t.Fatalf("title = %#v", payload["title"])
	}
	if payload["message"] != "已就绪：OpenD ready for requests" {
		t.Fatalf("message = %#v", payload["message"])
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
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	heartbeat := readLiveWebSocketEvent(t, conn)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}

	event := readLiveWebSocketEvent(t, conn)
	payload := liveWebSocketPayload(t, event, "system.notification")
	if event["source"] != "notification" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	if payload["title"] != "BBGO 通知" {
		t.Fatalf("title = %#v", payload["title"])
	}
	if payload["message"] != "strategy demo-grid started" {
		t.Fatalf("message = %#v", payload["message"])
	}
}

func TestLiveWebSocketHeartbeatReportsStaleMarketData(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.marketdataSvc.SetSubscriptionReconciler(nil)
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
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	heartbeat := readLiveWebSocketEvent(t, conn)
	if heartbeat["type"] != "heartbeat" {
		t.Fatalf("unexpected first event: %+v", heartbeat)
	}
	payload := liveWebSocketPayload(t, heartbeat, "heartbeat")
	if stale := jftradeCheckedTypeAssertion[bool](payload["stale"]); !stale {
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
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	if err := conn.WriteJSON(liveWebSocketClientMessage{
		Type: "subscribe",
		Subscriptions: liveWebSocketSubscriptions{
			ActiveInstruments: []string{"HK.00700"},
		},
	}); err != nil {
		t.Fatalf("subscribe live websocket: %v", err)
	}

	event := readLiveWebSocketEventOfType(t, conn, "market-data.tick")
	payload := liveWebSocketPayload(t, event, "market-data.tick")
	if event["source"] != "market-data" {
		t.Fatalf("unexpected event type: %+v", event)
	}
	snapshot, ok := payload["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot = %#v", payload["snapshot"])
	}
	if snapshot["observedAt"] != payload["at"] {
		t.Fatalf("snapshot.observedAt = %#v, payload.at = %#v", snapshot["observedAt"], payload["at"])
	}
	if payload["source"] != "bbgo:futu" {
		t.Fatalf("tick payload source = %#v", payload["source"])
	}
	parsedObservedAt := httpserver.ParseQueryTime(jftradeCheckedTypeAssertion[string](payload["at"]), time.Time{})
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
	defer func() { jftradeCheckTestError(t, conn.Close()) }()

	if err := conn.WriteJSON(liveWebSocketClientMessage{
		Type: "subscribe",
		Subscriptions: liveWebSocketSubscriptions{
			ConsoleRefresh: true,
		},
	}); err != nil {
		t.Fatalf("subscribe live websocket: %v", err)
	}

	event := readLiveWebSocketEventOfType(t, conn, "console.refresh")
	payload := liveWebSocketPayload(t, event, "console.refresh")
	if event["source"] != "system" {
		t.Fatalf("unexpected console refresh event: %+v", event)
	}
	if payload["checkedAt"] == "" {
		t.Fatalf("missing checkedAt: %+v", event)
	}
}

func dialLiveWebSocket(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/v1/ws/live"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		t.Cleanup(func() { jftradeCheckTestError(t, resp.Body.Close()) })
	}
	if err != nil {
		t.Fatalf("Dial live websocket: %v", err)
	}
	jftradeErr3 := conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	jftradeCheckTestError(t, jftradeErr3)
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

func readLiveWebSocketEventOfType(
	t *testing.T,
	conn *websocket.Conn,
	eventType string,
) map[string]any {
	t.Helper()
	for range 4 {
		event := readLiveWebSocketEvent(t, conn)
		if event["type"] == eventType {
			return event
		}
	}
	t.Fatalf("live websocket did not emit %s", eventType)
	return nil
}

func liveWebSocketPayload(t testing.TB, event map[string]any, eventType string) map[string]any {
	t.Helper()
	if event["type"] != eventType {
		t.Fatalf("event type = %#v, want %s: %+v", event["type"], eventType, event)
	}
	if event["eventId"] == "" || event["entityId"] == "" || event["serverTime"] == "" {
		t.Fatalf("incomplete live envelope: %+v", event)
	}
	payload := jftradeCheckedTypeAssertion[map[string]any](event["payload"])
	if payload["type"] != eventType {
		t.Fatalf("payload type = %#v, want %s: %+v", payload["type"], eventType, payload)
	}
	return payload
}

func mustDecimal(value string) decimal.Decimal {
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		panic(err)
	}
	return parsed
}
