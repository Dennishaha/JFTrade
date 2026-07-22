package live

import (
	"context"
	"encoding/json"
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
	ticksErr        error
	notifications   []livecore.Event
	lastTickIDs     []string
	depthNum        int32
	depthResolvedAt string
	depthSubscriber func(string)
	unsubscribed    bool
	securityErr     error
	depthErr        error
	nilUnsubscribe  bool
}

func (b *fakeBackend) ConnectionLimit() int { return b.limit }

func (b *fakeBackend) Heartbeat(interval time.Duration, stats ClientStats, _ []string) map[string]any {
	return map[string]any{
		"type": "heartbeat", "at": time.Now().UTC().Format(time.RFC3339Nano), "intervalMs": interval.Milliseconds(),
		"liveClients": map[string]any{"connected": stats.Connected, "limit": stats.Limit, "atLimit": stats.AtLimit},
	}
}

func (b *fakeBackend) MarketTicks(_ context.Context, instrumentIDs []string, _ string) ([]TickEvent, error) {
	b.mu.Lock()
	b.lastTickIDs = append([]string(nil), instrumentIDs...)
	ticks := append([]TickEvent(nil), b.ticks...)
	b.mu.Unlock()
	return ticks, b.ticksErr
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
	if b.securityErr != nil {
		return nil, b.securityErr
	}
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": market + "." + symbol},
		"security": map[string]any{"name": "Tencent Holdings"},
		"meta":     map[string]any{"resolvedAt": "2026-06-14T00:00:00Z"},
	}, nil
}

func (b *fakeBackend) Depth(_ context.Context, market, symbol string, num int32) (map[string]any, error) {
	b.mu.Lock()
	b.depthNum = num
	depthErr := b.depthErr
	resolvedAt := b.depthResolvedAt
	b.mu.Unlock()
	if depthErr != nil {
		return nil, depthErr
	}
	if resolvedAt == "" {
		resolvedAt = "2026-06-14T00:00:01Z"
	}
	return map[string]any{
		"request": map[string]any{"market": market, "symbol": symbol, "instrumentId": market + "." + symbol, "num": num},
		"depth":   map[string]any{"bids": []any{map[string]any{"price": "100"}}},
		"meta":    map[string]any{"resolvedAt": resolvedAt},
	}, nil
}

func (b *fakeBackend) SubscribeDepthUpdates(fn func(string)) func() {
	b.mu.Lock()
	b.depthSubscriber = fn
	if b.nilUnsubscribe {
		b.mu.Unlock()
		return nil
	}
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
		jftradeErr2 := handler.Close()
		jftradeCheckTestError(t, jftradeErr2)
		server.Close()
	})

	conn := dial(t, server.URL)
	defer func() { jftradeCheckTestError(t, conn.Close()) }()
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
	if first["type"] != "heartbeat" || first["source"] != "system" {
		t.Fatalf("first event = %#v, want heartbeat", first)
	}

	seenSecurity := false
	seenDepth := false
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && (!seenSecurity || !seenDepth) {
		event := readEvent(t, conn)
		switch event["type"] {
		case "market.security-details":
			payload := liveEnvelopePayload(t, event, "market.security-details")
			seenSecurity = true
			request := jftradeCheckedTypeAssertion[map[string]any](payload["request"])
			if request["instrumentId"] != "HK.00700" {
				t.Fatalf("security request = %#v", request)
			}
		case "market.depth":
			payload := liveEnvelopePayload(t, event, "market.depth")
			seenDepth = true
			request := jftradeCheckedTypeAssertion[map[string]any](payload["request"])
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
	depthPush := readMatchingEvent(t, conn, "depth update", func(event map[string]any) bool {
		return event["type"] == "market.depth"
	})
	if depthPush["type"] != "market.depth" || depthPush["source"] != "market-data" || depthPush["entityId"] != "US.TME|50" {
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

func TestHandlerDepthUpdatePublishesFreshPayload(t *testing.T) {
	const initialResolvedAt = "2026-06-14T00:00:01Z"
	const updatedResolvedAt = "2026-06-14T00:00:02Z"
	quietInterval := time.Hour

	backend := &fakeBackend{limit: 1}
	handler := NewHandler(backend, Options{
		HeartbeatInterval:       quietInterval,
		DataInterval:            quietInterval,
		ConsoleRefreshInterval:  quietInterval,
		SecurityDetailsInterval: quietInterval,
		DepthRefreshInterval:    quietInterval,
	})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		jftradeCheckTestError(t, handler.Close())
		server.Close()
	})

	conn := dial(t, server.URL)
	defer func() { jftradeCheckTestError(t, conn.Close()) }()
	if initial := readEvent(t, conn); initial["type"] != "heartbeat" {
		t.Fatalf("initial event = %#v, want heartbeat", initial)
	}
	if err := conn.WriteJSON(map[string]any{
		"type": "subscribe",
		"subscriptions": map[string]any{
			"depth": []map[string]any{{
				"market": "us", "symbol": "tme", "instrumentId": "US.TME", "num": 50,
			}},
		},
	}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	initialDepth := readMatchingEvent(t, conn, "initial depth", func(event map[string]any) bool {
		return event["type"] == "market.depth"
	})
	initialPayload := liveEnvelopePayload(t, initialDepth, "market.depth")
	initialMeta := jftradeCheckedTypeAssertion[map[string]any](initialPayload["meta"])
	if initialMeta["resolvedAt"] != initialResolvedAt {
		t.Fatalf("initial depth payload = %#v", initialPayload)
	}

	backend.mu.Lock()
	backend.depthResolvedAt = updatedResolvedAt
	depthSubscriber := backend.depthSubscriber
	backend.mu.Unlock()
	if depthSubscriber == nil {
		t.Fatal("depth update subscriber was not registered")
	}
	depthSubscriber("US.TME")

	updatedDepth := readMatchingEvent(t, conn, "fresh depth update", func(event map[string]any) bool {
		if event["type"] != "market.depth" {
			return false
		}
		payload, ok := event["payload"].(map[string]any)
		if !ok {
			return false
		}
		meta, ok := payload["meta"].(map[string]any)
		return ok && meta["resolvedAt"] == updatedResolvedAt
	})
	updatedPayload := liveEnvelopePayload(t, updatedDepth, "market.depth")
	updatedRequest := jftradeCheckedTypeAssertion[map[string]any](updatedPayload["request"])
	if updatedDepth["source"] != "market-data" || updatedDepth["entityId"] != "US.TME|50" || updatedRequest["instrumentId"] != "US.TME" || updatedRequest["num"] != float64(50) {
		t.Fatalf("fresh depth event = %#v", updatedDepth)
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
		jftradeErr1 := handler.Close()
		jftradeCheckTestError(t, jftradeErr1)
		server.Close()
	})

	conn := dial(t, server.URL)
	defer func() { jftradeCheckTestError(t, conn.Close()) }()
	_ = readEvent(t, conn)
	event := readEvent(t, conn)
	payload := liveEnvelopePayload(t, event, "system.notification")
	if event["source"] != "notification" || payload["id"] != "system-notification-1" {
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
		jftradeErr1 := second.Close()
		jftradeCheckTestError(t, jftradeErr1)
	}
	if err == nil || response == nil || response.StatusCode != 503 {
		t.Fatalf("second dial err=%v status=%v", err, responseStatus(response))
		return
	}
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	var envelope map[string]any
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode limit response: %v", err)
	}
	errorPayload := jftradeCheckedTypeAssertion[map[string]any](envelope["error"])
	if errorPayload["code"] != "LIVE_WS_LIMIT_REACHED" {
		t.Fatalf("limit response = %#v", envelope)
	}

	if err := handler.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	jftradeErr2 := first.SetReadDeadline(time.Now().Add(time.Second))
	jftradeCheckTestError(t, jftradeErr2)
	if _, _, err := first.ReadMessage(); err == nil {
		t.Fatal("expected Close to terminate active connection")
	}
	jftradeErr3 := first.Close()
	jftradeCheckTestError(t, jftradeErr3)

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

func TestHandlerRejectsUntrustedWebSocketOrigin(t *testing.T) {
	handler := NewHandler(&fakeBackend{limit: 1}, Options{})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		jftradeCheckTestError(t, handler.Close())
		server.Close()
	})

	for _, value := range []string{"http://evil.example", "null"} {
		t.Run(value, func(t *testing.T) {
			headers := http.Header{"Origin": []string{value}}
			conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), headers)
			if conn != nil {
				jftradeCheckTestError(t, conn.Close())
			}
			if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
				t.Fatalf("dial err=%v status=%v", err, responseStatus(response))
				return
			}
			jftradeCheckTestError(t, response.Body.Close())
		})
	}
}

func TestHandlerAcceptsSameOriginWebSocket(t *testing.T) {
	handler := NewHandler(&fakeBackend{limit: 1}, Options{})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		jftradeCheckTestError(t, handler.Close())
		server.Close()
	})

	headers := http.Header{"Origin": []string{server.URL}}
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), headers)
	if response != nil && response.Body != nil {
		t.Cleanup(func() { jftradeCheckTestError(t, response.Body.Close()) })
	}
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, conn.Close()) })
	_ = readEvent(t, conn)
}

func TestDispatcherDeduplicatesTickObservedAt(t *testing.T) {
	backend := &fakeBackend{
		limit: 1,
		ticks: []TickEvent{{
			InstrumentID: "US.AAPL",
			ObservedAt:   "2026-06-14T00:00:00Z",
			Payload:      map[string]any{"type": "market-data.tick", "at": "2026-06-14T00:00:00Z", "source": "bbgo:futu"},
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
	payload := liveEnvelopePayload(t, writer.events[0], "market-data.tick")
	if payload["at"] != "2026-06-14T00:00:00Z" {
		t.Fatalf("tick payload = %#v", payload)
	}
	if writer.events[0]["source"] != "market-data" || payload["source"] != "bbgo:futu" {
		t.Fatalf("source fields were not preserved: envelope=%#v payload=%#v", writer.events[0], payload)
	}
	if payload["brokerId"] != "futu" {
		t.Fatalf("default tick provider = %#v, want futu", payload["brokerId"])
	}
}

func TestDispatcherProviderSwitchTagsAndDoesNotDeduplicateNewProvider(t *testing.T) {
	backend := &fakeBackend{ticks: []TickEvent{{
		InstrumentID: "US.AAPL",
		ObservedAt:   "2026-06-14T00:00:00Z",
		Payload:      map[string]any{"type": "market-data.tick", "at": "2026-06-14T00:00:00Z"},
	}}}
	client := (&livecore.ClientRegistry{}).Register()
	client.SetSubscriptions(livecore.Subscriptions{
		ProviderBrokerID:  "alpha",
		ActiveInstruments: []string{"US.AAPL"},
	})
	writer := &recordingWriter{}
	d := &dispatcher{
		handler: NewHandler(backend, Options{}), requestCtx: t.Context(),
		writer: writer, client: client, lastSentByInstrument: map[string]string{},
		lastSecurityResolvedAt: map[string]string{}, lastDepthResolvedAt: map[string]string{},
	}
	if err := d.writeLiveData(); err != nil {
		t.Fatalf("alpha writeLiveData: %v", err)
	}
	client.SetSubscriptions(livecore.Subscriptions{
		ProviderBrokerID:  "futu",
		ActiveInstruments: []string{"US.AAPL"},
	})
	if err := d.writeLiveData(); err != nil {
		t.Fatalf("futu writeLiveData: %v", err)
	}
	if got := writer.countType("market-data.tick"); got != 2 {
		t.Fatalf("tick count across provider switch = %d, want 2", got)
	}
	if got := liveEnvelopePayload(t, writer.events[0], "market-data.tick")["brokerId"]; got != "alpha" {
		t.Fatalf("first tick brokerId = %#v", got)
	}
	if got := liveEnvelopePayload(t, writer.events[1], "market-data.tick")["brokerId"]; got != "futu" {
		t.Fatalf("second tick brokerId = %#v", got)
	}
}

type recordingWriter struct {
	events []map[string]any
}

func (w *recordingWriter) WriteEvent(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var event map[string]any
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
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

func liveEnvelopePayload(t testing.TB, event map[string]any, eventType string) map[string]any {
	t.Helper()
	if event["type"] != eventType {
		t.Fatalf("event type = %#v, want %s: %#v", event["type"], eventType, event)
	}
	if event["eventId"] == "" || event["entityId"] == "" || event["serverTime"] == "" {
		t.Fatalf("incomplete live envelope: %#v", event)
	}
	payload := jftradeCheckedTypeAssertion[map[string]any](event["payload"])
	if payload["type"] != eventType {
		t.Fatalf("payload type = %#v, want %s: %#v", payload["type"], eventType, payload)
	}
	return payload
}

func dial(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		t.Cleanup(func() { jftradeCheckTestError(t, resp.Body.Close()) })
	}
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	jftradeErr4 := conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	jftradeCheckTestError(t, jftradeErr4)
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

func readMatchingEvent(
	t *testing.T,
	conn *websocket.Conn,
	description string,
	matches func(map[string]any) bool,
) map[string]any {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	seenTypes := make([]string, 0, 4)
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("SetReadDeadline while waiting for %s: %v", description, err)
		}
		var event map[string]any
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("ReadJSON while waiting for %s after events %v: %v", description, seenTypes, err)
		}
		if eventType, ok := event["type"].(string); ok {
			seenTypes = append(seenTypes, eventType)
		}
		if matches(event) {
			return event
		}
	}
}

func responseStatus(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
