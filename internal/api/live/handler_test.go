package live

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	livecore "github.com/jftrade/jftrade-main/internal/live"
)

type fakeBackend struct {
	mu              sync.Mutex
	limit           int
	ticks           []TickEvent
	notifications   []livecore.Event
	lastTickIDs     []string
	depthNum        int32
	depthSubscriber func(string)
	unsubscribed    bool
}

func (b *fakeBackend) ConnectionLimit() int { return b.limit }

func (b *fakeBackend) Heartbeat(interval time.Duration, stats ClientStats, _ []string) map[string]any {
	return map[string]any{
		"type": "heartbeat", "intervalMs": interval.Milliseconds(),
		"liveClients": map[string]any{"connected": stats.Connected, "limit": stats.Limit, "atLimit": stats.AtLimit},
	}
}

func (b *fakeBackend) MarketTicks(_ context.Context, instrumentIDs []string, _ string) ([]TickEvent, error) {
	b.mu.Lock()
	b.lastTickIDs = append([]string(nil), instrumentIDs...)
	ticks := append([]TickEvent(nil), b.ticks...)
	b.mu.Unlock()
	return ticks, nil
}

func (b *fakeBackend) NotificationsAfter(sequence uint64) []livecore.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []livecore.Event
	for _, event := range b.notifications {
		if event.Sequence > sequence {
			result = append(result, event)
		}
	}
	return result
}

func (b *fakeBackend) EnsureNotificationBridge(context.Context) {}

func (b *fakeBackend) SecurityDetails(_ context.Context, market, symbol string) (map[string]any, error) {
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": market + "." + symbol},
		"security": map[string]any{"name": "Tencent Holdings"},
		"meta":     map[string]any{"resolvedAt": "2026-06-14T00:00:00Z"},
	}, nil
}

func (b *fakeBackend) SubscribeDepth(_ context.Context, _ string, num int32) {
	b.mu.Lock()
	b.depthNum = num
	b.mu.Unlock()
}

func (b *fakeBackend) Depth(_ context.Context, market, symbol string, num int32) (map[string]any, error) {
	return map[string]any{
		"request": map[string]any{"market": market, "symbol": symbol, "instrumentId": market + "." + symbol, "num": num},
		"depth":   map[string]any{"bids": []any{map[string]any{"price": "100"}}},
		"meta":    map[string]any{"resolvedAt": "2026-06-14T00:00:01Z"},
	}, nil
}

func (b *fakeBackend) SubscribeDepthUpdates(fn func(string)) func() {
	b.mu.Lock()
	b.depthSubscriber = fn
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		b.unsubscribed = true
		b.mu.Unlock()
	}
}

func TestHandlerHeartbeatSubscribeNormalizationAndPayloads(t *testing.T) {
	backend := &fakeBackend{limit: 2}
	handler := NewHandler(backend, Options{DataInterval: 10 * time.Millisecond})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		_ = handler.Close()
		server.Close()
	})

	conn := dial(t, server.URL)
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{
		"type": "subscribe",
		"subscriptions": map[string]any{
			"activeInstruments": []string{" us.aapl ", "US.AAPL"},
			"securityDetails": []map[string]any{{
				"market": " hk ", "symbol": " 00700 ", "instrumentId": " hk.00700 ",
			}},
			"depth": []map[string]any{{
				"market": " us ", "symbol": " tme ", "instrumentId": " us.tme ", "num": 99,
			}},
		},
	}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	first := readEvent(t, conn)
	if first["type"] != "heartbeat" {
		t.Fatalf("first event = %#v, want heartbeat", first)
	}

	seenSecurity := false
	seenDepth := false
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && (!seenSecurity || !seenDepth) {
		event := readEvent(t, conn)
		switch event["type"] {
		case "market.security-details":
			seenSecurity = true
			request := event["request"].(map[string]any)
			if request["instrumentId"] != "HK.00700" {
				t.Fatalf("security request = %#v", request)
			}
		case "market.depth":
			seenDepth = true
			request := event["request"].(map[string]any)
			if request["instrumentId"] != "US.TME" || request["num"] != float64(50) {
				t.Fatalf("depth request = %#v", request)
			}
		}
	}
	if !seenSecurity || !seenDepth {
		t.Fatalf("payloads seen: security=%v depth=%v", seenSecurity, seenDepth)
	}

	backend.mu.Lock()
	depthSubscriber := backend.depthSubscriber
	backend.mu.Unlock()
	if depthSubscriber == nil {
		t.Fatal("depth update subscriber was not registered")
	}
	depthSubscriber(" us.tme ")
	depthPush := readEvent(t, conn)
	if depthPush["type"] != "market.depth" {
		t.Fatalf("depth push event = %#v", depthPush)
	}

	var gotIDs []string
	var gotDepthNum int32
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		backend.mu.Lock()
		gotIDs = append([]string(nil), backend.lastTickIDs...)
		gotDepthNum = backend.depthNum
		backend.mu.Unlock()
		if len(gotIDs) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(gotIDs) != 1 || gotIDs[0] != "US.AAPL" {
		t.Fatalf("normalized tick ids = %v", gotIDs)
	}
	if gotDepthNum != 50 {
		t.Fatalf("depth num = %d, want 50", gotDepthNum)
	}
}

func TestHandlerNotificationSequenceZeroReplay(t *testing.T) {
	backend := &fakeBackend{
		limit: 1,
		notifications: []livecore.Event{{
			Sequence: 1, At: "2026-06-14T00:00:00Z", Level: "info",
			Title: "ready", Message: "connected", Source: "test", Category: "system",
		}},
	}
	handler := NewHandler(backend, Options{})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		_ = handler.Close()
		server.Close()
	})

	conn := dial(t, server.URL)
	defer conn.Close()
	_ = readEvent(t, conn)
	event := readEvent(t, conn)
	if event["type"] != "system.notification" || event["id"] != "system-notification-1" {
		t.Fatalf("notification event = %#v", event)
	}
}

func TestHandlerConnectionLimitAndCloseLifecycle(t *testing.T) {
	backend := &fakeBackend{limit: 1}
	handler := NewHandler(backend, Options{})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	first := dial(t, server.URL)
	_ = readEvent(t, first)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	second, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if second != nil {
		_ = second.Close()
	}
	if err == nil || response == nil || response.StatusCode != 503 {
		t.Fatalf("second dial err=%v status=%v", err, responseStatus(response))
	}
	defer response.Body.Close()
	var envelope map[string]any
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode limit response: %v", err)
	}
	errorPayload := envelope["error"].(map[string]any)
	if errorPayload["code"] != "LIVE_WS_LIMIT_REACHED" {
		t.Fatalf("limit response = %#v", envelope)
	}

	if err := handler.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_ = first.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := first.ReadMessage(); err == nil {
		t.Fatal("expected Close to terminate active connection")
	}
	_ = first.Close()

	deadline := time.Now().Add(time.Second)
	for handler.Stats().Connected != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stats := handler.Stats(); stats.Connected != 0 {
		t.Fatalf("stats after Close = %+v", stats)
	}
	backend.mu.Lock()
	unsubscribed := backend.unsubscribed
	backend.mu.Unlock()
	if !unsubscribed {
		t.Fatal("expected depth update subscription to be released")
	}
}

func TestDispatcherDeduplicatesTickObservedAt(t *testing.T) {
	backend := &fakeBackend{
		limit: 1,
		ticks: []TickEvent{{
			InstrumentID: "US.AAPL",
			ObservedAt:   "2026-06-14T00:00:00Z",
			Payload:      map[string]any{"type": "market-data.tick", "at": "2026-06-14T00:00:00Z"},
		}},
	}
	handler := NewHandler(backend, Options{})
	client := (&livecore.ClientRegistry{}).Register()
	client.SetSubscriptions(livecore.Subscriptions{ActiveInstruments: []string{"US.AAPL"}})
	writer := &recordingWriter{}
	d := &dispatcher{
		handler: handler, requestCtx: t.Context(), writer: writer, client: client,
		lastSentByInstrument: map[string]string{}, lastSecurityResolvedAt: map[string]string{},
		lastDepthResolvedAt: map[string]string{},
	}
	if err := d.writeLiveData(); err != nil {
		t.Fatalf("first writeLiveData: %v", err)
	}
	if err := d.writeLiveData(); err != nil {
		t.Fatalf("second writeLiveData: %v", err)
	}
	if got := writer.countType("market-data.tick"); got != 1 {
		t.Fatalf("tick count = %d, want 1", got)
	}
}

type recordingWriter struct {
	events []map[string]any
}

func (w *recordingWriter) WriteEvent(value any) error {
	event, ok := value.(map[string]any)
	if !ok {
		return errors.New("event is not a map")
	}
	w.events = append(w.events, event)
	return nil
}

func (w *recordingWriter) countType(eventType string) int {
	count := 0
	for _, event := range w.events {
		if event["type"] == eventType {
			count++
		}
	}
	return count
}

func dial(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	return conn
}

func readEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	return event
}

func responseStatus(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}
