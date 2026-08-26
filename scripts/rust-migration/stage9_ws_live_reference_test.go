package rustmigration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	livecore "github.com/jftrade/jftrade-main/internal/live"
)

const stage9WSLiveFixtureVersion = "stage9.ws-live.v1"

type stage9WSLiveFixture struct {
	Version string             `json:"version"`
	Route   stage9WSLiveRoute  `json:"route"`
	Cases   []stage9WSLiveCase `json:"cases"`
}

type stage9WSLiveRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type stage9WSLiveCase struct {
	Name        string               `json:"name"`
	Method      string               `json:"method"`
	RequestPath string               `json:"requestPath"`
	Scenario    string               `json:"scenario"`
	Input       stage9WSLiveInput    `json:"input"`
	Expected    stage9WSLiveExpected `json:"expected"`
}

type stage9WSLiveInput struct {
	BackendAvailable          bool                       `json:"backendAvailable"`
	ConnectionLimit           int                        `json:"connectionLimit"`
	OriginPolicy              string                     `json:"originPolicy"`
	OfferProtocol             bool                       `json:"offerProtocol"`
	HeartbeatAt               string                     `json:"heartbeatAt"`
	HeartbeatIntervalMs       int                        `json:"heartbeatIntervalMs"`
	DataIntervalMs            int                        `json:"dataIntervalMs"`
	ConsoleRefreshIntervalMs  int                        `json:"consoleRefreshIntervalMs"`
	SecurityDetailsIntervalMs int                        `json:"securityDetailsIntervalMs"`
	DepthRefreshIntervalMs    int                        `json:"depthRefreshIntervalMs"`
	SecurityResolvedAt        string                     `json:"securityResolvedAt"`
	DepthResolvedAt           string                     `json:"depthResolvedAt"`
	DepthUpdatedAt            string                     `json:"depthUpdatedAt"`
	Subscribe                 *stage9WSLiveSubscriptions `json:"subscribe,omitempty"`
	Ticks                     []stage9WSLiveTick         `json:"ticks,omitempty"`
	TicksError                bool                       `json:"ticksError,omitempty"`
	Notifications             []stage9WSLiveNotification `json:"notifications,omitempty"`
	SecurityError             bool                       `json:"securityError,omitempty"`
	DepthError                bool                       `json:"depthError,omitempty"`
	DepthPayload              map[string]any             `json:"depthPayload,omitempty"`
}

type stage9WSLiveSubscriptions struct {
	ProviderBrokerID  string                             `json:"providerBrokerId"`
	ActiveInstruments []string                           `json:"activeInstruments,omitempty"`
	SecurityDetails   []stage9WSLiveSecuritySubscription `json:"securityDetails,omitempty"`
	Depth             []stage9WSLiveDepthSubscription    `json:"depth,omitempty"`
	ConsoleRefresh    bool                               `json:"consoleRefresh,omitempty"`
}

type stage9WSLiveSecuritySubscription struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
}

type stage9WSLiveDepthSubscription struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
	Num          int32  `json:"num"`
}

type stage9WSLiveTick struct {
	InstrumentID string         `json:"instrumentId"`
	ObservedAt   string         `json:"observedAt"`
	Payload      map[string]any `json:"payload"`
}

type stage9WSLiveNotification struct {
	Sequence uint64 `json:"sequence"`
	At       string `json:"at"`
	Level    string `json:"level"`
	Title    string `json:"title"`
	Message  string `json:"message,omitempty"`
	Source   string `json:"source"`
	BrokerID string `json:"brokerId"`
	Category string `json:"category"`
}

type stage9WSLiveExpected struct {
	Sessions []stage9WSLiveSession `json:"sessions"`
	Rejected *stage9WSLiveRejected `json:"rejected,omitempty"`
	Calls    stage9WSLiveCalls     `json:"calls"`
}

type stage9WSLiveSession struct {
	Handshake stage9WSLiveHandshake `json:"handshake"`
	Frames    []string              `json:"frames"`
	Close     *stage9WSLiveClose    `json:"close,omitempty"`
}

type stage9WSLiveHandshake struct {
	Status           int    `json:"status"`
	SelectedProtocol string `json:"selectedProtocol"`
}

type stage9WSLiveClose struct {
	Kind string `json:"kind"`
	Code int    `json:"code,omitempty"`
}

type stage9WSLiveRejected struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Body        string `json:"body"`
}

type stage9WSLiveCalls struct {
	EnsureNotificationBridge int  `json:"ensureNotificationBridge"`
	DepthSubscribe           int  `json:"depthSubscribe"`
	DepthUnsubscribe         int  `json:"depthUnsubscribe"`
	MarketTicksCalled        bool `json:"marketTicksCalled"`
	SecurityCalls            int  `json:"securityCalls"`
	DepthCalls               int  `json:"depthCalls"`
}

// TestStage9WSLiveFixtureMatchesCurrentGoOwner freezes the WebSocket handshake,
// replay frames, lifecycle and failure observations without connecting OpenD,
// BBGO, a Provider, or any production live-data source.
func TestStage9WSLiveFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 ws-live fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/ws-live.json")
	want := stage9WSLiveFixture{
		Version: stage9WSLiveFixtureVersion,
		Route:   stage9WSLiveRoute{Method: http.MethodGet, Path: "/api/v1/ws/live"},
		Cases:   make([]stage9WSLiveCase, 0),
	}
	for _, testCase := range stage9WSLiveCases() {
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()
		})
		caseCopy := testCase
		caseCopy.Expected = runStage9WSLiveCase(t, caseCopy)
		want.Cases = append(want.Cases, caseCopy)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode ws-live fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write ws-live fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read ws-live fixture: %v", err)
	}
	var got stage9WSLiveFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode ws-live fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if index >= len(got.Cases) || !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf("stage 9 ws-live case %s drifted: got=%#v want=%#v", want.Cases[index].Name, got.Cases[index], want.Cases[index])
			}
		}
		t.Fatal("stage 9 ws-live fixture drifted from the Go owner")
	}
}

func stage9WSLiveCases() []stage9WSLiveCase {
	return []stage9WSLiveCase{
		{
			Name: "heartbeat-no-origin-with-desktop-protocol", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "heartbeat",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2, OfferProtocol: true,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 15000,
				DataIntervalMs: 3600000, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000},
		},
		{
			Name: "subscription-event-order-and-normalization", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "subscription-order",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 3600000, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000,
				SecurityResolvedAt: "2026-08-22T00:00:01Z", DepthResolvedAt: "2026-08-22T00:00:02Z",
				Subscribe: &stage9WSLiveSubscriptions{
					ProviderBrokerID: " futu ", ActiveInstruments: []string{" us.aapl ", "US.AAPL"},
					SecurityDetails: []stage9WSLiveSecuritySubscription{{Market: " hk ", Symbol: " 00700 ", InstrumentID: " hk.00700 "}},
					Depth:           []stage9WSLiveDepthSubscription{{Market: " us ", Symbol: " tme ", InstrumentID: " us.tme ", Num: 99}},
					ConsoleRefresh:  true,
				},
			},
		},
		{
			Name: "notification-replay-tick-and-deduplication", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "tick-notification",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 5, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000,
				Subscribe: &stage9WSLiveSubscriptions{ProviderBrokerID: "futu", ActiveInstruments: []string{"US.AAPL"}},
				Ticks: []stage9WSLiveTick{{InstrumentID: "US.AAPL", ObservedAt: "2026-08-22T00:00:03Z",
					Payload: map[string]any{"type": "market-data.tick", "at": "2026-08-22T00:00:03Z", "source": "bbgo:futu", "price": "100.5"}}},
				Notifications: []stage9WSLiveNotification{{Sequence: 1, At: "2026-08-22T00:00:04Z", Level: "warn", Title: "Provider warming", Message: "retrying", Source: "fixture", BrokerID: "futu", Category: "provider"}},
			},
		},
		{
			Name: "depth-push-refresh-and-release", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "depth-update",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 3600000, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000,
				DepthResolvedAt: "2026-08-22T00:00:02Z", DepthUpdatedAt: "2026-08-22T00:00:05Z",
				Subscribe: &stage9WSLiveSubscriptions{ProviderBrokerID: "futu", Depth: []stage9WSLiveDepthSubscription{{Market: "us", Symbol: "tme", InstrumentID: "US.TME", Num: 50}}},
			},
		},
		{
			Name: "invalid-subscription-closes-without-code-frame", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "invalid-subscription",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 3600000, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000},
		},
		{
			Name: "provider-error-cancels-stream", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "provider-error",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 5, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000,
				TicksError: true, Subscribe: &stage9WSLiveSubscriptions{ProviderBrokerID: "futu", ActiveInstruments: []string{"US.AAPL"}}},
		},
		{
			Name: "server-close-cancels-active-client", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "server-close",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 3600000, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000},
		},
		{
			Name: "client-reconnects-after-disconnect", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "client-reconnect",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 3600000, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000},
		},
		{
			Name: "origin-forbidden-during-handshake", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "origin-forbidden",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 2, OriginPolicy: "forbidden",
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000},
		},
		{
			Name: "connection-limit-rejection", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "connection-limit",
			Input: stage9WSLiveInput{BackendAvailable: true, ConnectionLimit: 1,
				HeartbeatAt: "2026-08-22T00:00:00Z", HeartbeatIntervalMs: 3600000,
				DataIntervalMs: 3600000, ConsoleRefreshIntervalMs: 3600000,
				SecurityDetailsIntervalMs: 3600000, DepthRefreshIntervalMs: 3600000},
		},
		{
			Name: "backend-unavailable-is-not-found", Method: http.MethodGet,
			RequestPath: "/api/v1/ws/live", Scenario: "backend-unavailable",
			Input: stage9WSLiveInput{BackendAvailable: false, ConnectionLimit: 2},
		},
	}
}

type stage9WSLiveBackend struct {
	mu                       sync.Mutex
	input                    stage9WSLiveInput
	depthSubscriber          func(string)
	ensureNotificationBridge int
	depthSubscribe           int
	depthUnsubscribe         int
	marketTicksCalled        bool
	securityCalls            int
	depthCalls               int
}

func newStage9WSLiveBackend(input stage9WSLiveInput) *stage9WSLiveBackend {
	return &stage9WSLiveBackend{input: input}
}

func (b *stage9WSLiveBackend) ConnectionLimit() int { return b.input.ConnectionLimit }

func (b *stage9WSLiveBackend) Heartbeat(interval time.Duration, stats apilive.ClientStats, _ []string, providerBrokerID string) map[string]any {
	return map[string]any{
		"type": "heartbeat", "at": b.input.HeartbeatAt, "intervalMs": interval.Milliseconds(),
		"providerBrokerId": providerBrokerID,
		"liveClients":      map[string]any{"connected": stats.Connected, "limit": stats.Limit, "atLimit": stats.AtLimit},
	}
}

func (b *stage9WSLiveBackend) MarketTicks(_ context.Context, providerBrokerID string, instrumentIDs []string, _ string) ([]apilive.TickEvent, error) {
	b.mu.Lock()
	b.marketTicksCalled = true
	b.mu.Unlock()
	if b.input.TicksError {
		return nil, errors.New("fixture market data unavailable")
	}
	result := make([]apilive.TickEvent, 0, len(b.input.Ticks))
	for _, tick := range b.input.Ticks {
		result = append(result, apilive.TickEvent{InstrumentID: tick.InstrumentID, ObservedAt: tick.ObservedAt, Payload: tick.Payload})
	}
	_ = providerBrokerID
	_ = instrumentIDs
	return result, nil
}

func (b *stage9WSLiveBackend) NotificationsAfter(sequence uint64) []livecore.Event {
	result := make([]livecore.Event, 0, len(b.input.Notifications))
	for _, notification := range b.input.Notifications {
		if notification.Sequence <= sequence {
			continue
		}
		result = append(result, livecore.Event{
			Sequence: notification.Sequence, At: notification.At, Level: notification.Level,
			Title: notification.Title, Message: notification.Message, Source: notification.Source,
			BrokerID: notification.BrokerID, Category: notification.Category,
		})
	}
	return result
}

func (b *stage9WSLiveBackend) EnsureNotificationBridge(context.Context) {
	b.mu.Lock()
	b.ensureNotificationBridge++
	b.mu.Unlock()
}

func (b *stage9WSLiveBackend) SecurityDetails(_ context.Context, _ string, market, symbol string) (map[string]any, error) {
	b.mu.Lock()
	b.securityCalls++
	b.mu.Unlock()
	if b.input.SecurityError {
		return nil, errors.New("fixture security unavailable")
	}
	return map[string]any{
		"request":  map[string]any{"market": market, "symbol": symbol, "instrumentId": market + "." + symbol},
		"security": map[string]any{"name": "Tencent Holdings"},
		"meta":     map[string]any{"resolvedAt": b.input.SecurityResolvedAt},
	}, nil
}

func (b *stage9WSLiveBackend) Depth(_ context.Context, _ string, market, symbol string, num int32) (map[string]any, error) {
	b.mu.Lock()
	b.depthCalls++
	resolvedAt := b.input.DepthResolvedAt
	depthError := b.input.DepthError
	depthPayload := b.input.DepthPayload
	b.mu.Unlock()
	if depthError {
		return nil, errors.New("fixture depth unavailable")
	}
	if depthPayload == nil {
		depthPayload = map[string]any{"bids": []any{map[string]any{"price": "100"}}}
	}
	return map[string]any{
		"request": map[string]any{"market": market, "symbol": symbol, "instrumentId": market + "." + symbol, "num": num},
		"depth":   depthPayload, "meta": map[string]any{"resolvedAt": resolvedAt},
	}, nil
}

func (b *stage9WSLiveBackend) SubscribeDepthUpdates(fn func(string)) func() {
	b.mu.Lock()
	b.depthSubscribe++
	b.depthSubscriber = fn
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		b.depthUnsubscribe++
		b.mu.Unlock()
	}
}

func (b *stage9WSLiveBackend) setDepthResolvedAt(value string) {
	b.mu.Lock()
	b.input.DepthResolvedAt = value
	b.mu.Unlock()
}

func (b *stage9WSLiveBackend) snapshotCalls() stage9WSLiveCalls {
	b.mu.Lock()
	defer b.mu.Unlock()
	return stage9WSLiveCalls{
		EnsureNotificationBridge: b.ensureNotificationBridge,
		DepthSubscribe:           b.depthSubscribe, DepthUnsubscribe: b.depthUnsubscribe,
		MarketTicksCalled: b.marketTicksCalled, SecurityCalls: b.securityCalls, DepthCalls: b.depthCalls,
	}
}

func (b *stage9WSLiveBackend) waitUnsubscribed(t *testing.T, handler *apilive.Handler, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		calls := b.snapshotCalls()
		if calls.DepthUnsubscribe >= want && handler.Stats().Connected == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("live backend cleanup did not finish: calls=%+v stats=%+v", b.snapshotCalls(), handler.Stats())
}

func stage9WSLiveOptions(input stage9WSLiveInput) apilive.Options {
	return apilive.Options{
		HeartbeatInterval:       time.Duration(input.HeartbeatIntervalMs) * time.Millisecond,
		DataInterval:            time.Duration(input.DataIntervalMs) * time.Millisecond,
		ConsoleRefreshInterval:  time.Duration(input.ConsoleRefreshIntervalMs) * time.Millisecond,
		SecurityDetailsInterval: time.Duration(input.SecurityDetailsIntervalMs) * time.Millisecond,
		DepthRefreshInterval:    time.Duration(input.DepthRefreshIntervalMs) * time.Millisecond,
	}
}

func runStage9WSLiveCase(t *testing.T, testCase stage9WSLiveCase) stage9WSLiveExpected {
	if testCase.Input.OriginPolicy == "forbidden" {
		return runStage9WSLiveOriginForbidden(t, testCase.Input)
	}
	if testCase.Scenario == "backend-unavailable" {
		return runStage9WSLiveBackendUnavailable(t, testCase.Input)
	}
	server, handler, backend := newStage9WSLiveServer(t, testCase.Input)
	defer server.Close()
	defer func() { _ = handler.Close() }()
	switch testCase.Scenario {
	case "heartbeat":
		return runStage9WSLiveHeartbeat(t, server, handler, backend, testCase.Input)
	case "subscription-order":
		return runStage9WSLiveSubscriptionOrder(t, server, handler, backend, testCase.Input)
	case "tick-notification":
		return runStage9WSLiveTickNotification(t, server, handler, backend, testCase.Input)
	case "depth-update":
		return runStage9WSLiveDepthUpdate(t, server, handler, backend, testCase.Input)
	case "invalid-subscription":
		return runStage9WSLiveInvalidSubscription(t, server, handler, backend, testCase.Input)
	case "provider-error":
		return runStage9WSLiveProviderError(t, server, handler, backend, testCase.Input)
	case "server-close":
		return runStage9WSLiveServerClose(t, server, handler, backend, testCase.Input)
	case "client-reconnect":
		return runStage9WSLiveReconnect(t, server, handler, backend, testCase.Input)
	case "connection-limit":
		return runStage9WSLiveConnectionLimit(t, server, handler, backend, testCase.Input)
	default:
		t.Fatalf("unknown ws-live scenario %q", testCase.Scenario)
		return stage9WSLiveExpected{}
	}
}

func newStage9WSLiveServer(t *testing.T, input stage9WSLiveInput) (*httptest.Server, *apilive.Handler, *stage9WSLiveBackend) {
	t.Helper()
	var backend *stage9WSLiveBackend
	var owner apilive.Backend
	if input.BackendAvailable {
		backend = newStage9WSLiveBackend(input)
		owner = backend
	}
	handler := apilive.NewHandler(owner, stage9WSLiveOptions(input))
	return httptest.NewServer(handler), handler, backend
}

func runStage9WSLiveHeartbeat(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	conn, response := dialStage9WSLive(t, server, input)
	frame := readStage9WSLiveFrame(t, conn)
	_ = conn.Close()
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(response, conn), Frames: []string{frame}}}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveSubscriptionOrder(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	conn, response := dialStage9WSLive(t, server, input)
	frames := []string{readStage9WSLiveFrame(t, conn)}
	writeStage9WSLiveSubscription(t, conn, input.Subscribe)
	frames = append(frames, readStage9WSLiveUntilType(t, conn, "market.depth")...)
	_ = conn.Close()
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(response, conn), Frames: frames}}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveTickNotification(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	conn, response := dialStage9WSLive(t, server, input)
	frames := []string{readStage9WSLiveFrame(t, conn)}
	frames = append(frames, readStage9WSLiveUntilType(t, conn, "system.notification")...)
	writeStage9WSLiveSubscription(t, conn, input.Subscribe)
	frames = append(frames, readStage9WSLiveUntilType(t, conn, "market-data.tick")...)
	if err := conn.SetReadDeadline(time.Now().Add(40 * time.Millisecond)); err != nil {
		t.Fatalf("set tick dedup deadline: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("duplicate tick was emitted for an unchanged observedAt")
	}
	_ = conn.Close()
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(response, conn), Frames: frames}}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveDepthUpdate(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	conn, response := dialStage9WSLive(t, server, input)
	frames := []string{readStage9WSLiveFrame(t, conn)}
	writeStage9WSLiveSubscription(t, conn, input.Subscribe)
	frames = append(frames, readStage9WSLiveUntilType(t, conn, "market.depth")...)
	backend.setDepthResolvedAt(input.DepthUpdatedAt)
	backend.mu.Lock()
	subscriber := backend.depthSubscriber
	backend.mu.Unlock()
	if subscriber == nil {
		t.Fatal("depth subscriber was not registered")
	}
	subscriber(" us.tme ")
	frames = append(frames, readStage9WSLiveUntilType(t, conn, "market.depth")...)
	_ = conn.Close()
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(response, conn), Frames: frames}}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveInvalidSubscription(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	conn, response := dialStage9WSLive(t, server, input)
	frames := []string{readStage9WSLiveFrame(t, conn)}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"subscribe","subscriptions":{"activeInstruments":["US.AAPL"]}}`)); err != nil {
		t.Fatalf("write invalid subscription: %v", err)
	}
	closeObservation := readStage9WSLiveClose(t, conn, &frames)
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(response, conn), Frames: frames, Close: &closeObservation}}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveProviderError(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	conn, response := dialStage9WSLive(t, server, input)
	frames := []string{readStage9WSLiveFrame(t, conn)}
	writeStage9WSLiveSubscription(t, conn, input.Subscribe)
	closeObservation := readStage9WSLiveClose(t, conn, &frames)
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(response, conn), Frames: frames, Close: &closeObservation}}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveServerClose(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	conn, response := dialStage9WSLive(t, server, input)
	frames := []string{readStage9WSLiveFrame(t, conn)}
	if err := handler.Close(); err != nil {
		t.Fatalf("close live handler: %v", err)
	}
	closeObservation := readStage9WSLiveClose(t, conn, &frames)
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(response, conn), Frames: frames, Close: &closeObservation}}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveReconnect(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	first, firstResponse := dialStage9WSLive(t, server, input)
	firstFrames := []string{readStage9WSLiveFrame(t, first)}
	_ = first.Close()
	backend.waitUnsubscribed(t, handler, 1)
	second, secondResponse := dialStage9WSLive(t, server, input)
	secondFrames := []string{readStage9WSLiveFrame(t, second)}
	if err := handler.Close(); err != nil {
		t.Fatalf("close reconnect handler: %v", err)
	}
	return stage9WSLiveExpected{Sessions: []stage9WSLiveSession{
		{Handshake: stage9WSLiveHandshakeFrom(firstResponse, first), Frames: firstFrames},
		{Handshake: stage9WSLiveHandshakeFrom(secondResponse, second), Frames: secondFrames},
	}, Calls: backend.snapshotCalls()}
}

func runStage9WSLiveConnectionLimit(t *testing.T, server *httptest.Server, handler *apilive.Handler, backend *stage9WSLiveBackend, input stage9WSLiveInput) stage9WSLiveExpected {
	first, firstResponse := dialStage9WSLive(t, server, input)
	firstFrames := []string{readStage9WSLiveFrame(t, first)}
	second, response, err := dialStage9WSLiveRaw(t, server, input)
	if second != nil {
		_ = second.Close()
	}
	if err == nil || response == nil {
		t.Fatalf("second connection unexpectedly succeeded: err=%v response=%v", err, response)
	}
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatalf("read connection limit body: %v", readErr)
	}
	_ = response.Body.Close()
	_ = first.Close()
	backend.waitUnsubscribed(t, handler, 1)
	return stage9WSLiveExpected{
		Sessions: []stage9WSLiveSession{{Handshake: stage9WSLiveHandshakeFrom(firstResponse, first), Frames: firstFrames}},
		Rejected: &stage9WSLiveRejected{Status: response.StatusCode, ContentType: response.Header.Get("Content-Type"), Body: normalizeStage9WSLiveWire(body)},
		Calls:    backend.snapshotCalls(),
	}
}

func runStage9WSLiveOriginForbidden(t *testing.T, input stage9WSLiveInput) stage9WSLiveExpected {
	server, handler, _ := newStage9WSLiveServer(t, input)
	defer server.Close()
	defer func() { _ = handler.Close() }()
	input.OriginPolicy = "forbidden"
	_, response, err := dialStage9WSLiveRaw(t, server, input)
	if err == nil || response == nil {
		t.Fatalf("forbidden origin unexpectedly succeeded: err=%v response=%v", err, response)
	}
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatalf("read origin rejection body: %v", readErr)
	}
	_ = response.Body.Close()
	return stage9WSLiveExpected{
		Sessions: []stage9WSLiveSession{},
		Rejected: &stage9WSLiveRejected{Status: response.StatusCode, ContentType: response.Header.Get("Content-Type"), Body: normalizeStage9WSLiveWire(body)},
		Calls:    stage9WSLiveCalls{},
	}
}

func runStage9WSLiveBackendUnavailable(t *testing.T, input stage9WSLiveInput) stage9WSLiveExpected {
	server, handler, _ := newStage9WSLiveServer(t, input)
	defer server.Close()
	defer func() { _ = handler.Close() }()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/api/v1/ws/live", nil)
	if err != nil {
		t.Fatalf("build unavailable request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request unavailable handler: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatalf("read unavailable body: %v", readErr)
	}
	_ = response.Body.Close()
	return stage9WSLiveExpected{
		Sessions: []stage9WSLiveSession{},
		Rejected: &stage9WSLiveRejected{Status: response.StatusCode, ContentType: response.Header.Get("Content-Type"), Body: normalizeStage9WSLiveWire(body)},
		Calls:    stage9WSLiveCalls{},
	}
}

func dialStage9WSLive(t *testing.T, server *httptest.Server, input stage9WSLiveInput) (*websocket.Conn, *http.Response) {
	t.Helper()
	conn, response, err := dialStage9WSLiveRaw(t, server, input)
	if err != nil || conn == nil || response == nil {
		t.Fatalf("dial ws-live: err=%v response=%v", err, response)
	}
	return conn, response
}

func dialStage9WSLiveRaw(_ *testing.T, server *httptest.Server, input stage9WSLiveInput) (*websocket.Conn, *http.Response, error) {
	headers := http.Header{}
	if input.OriginPolicy == "forbidden" {
		headers.Set("Origin", "http://evil.example")
	}
	if input.OfferProtocol {
		headers.Set("Sec-WebSocket-Protocol", "jftrade.desktop.v1")
	}
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws/live"
	return websocket.DefaultDialer.Dial(wsURL, headers)
}

func stage9WSLiveHandshakeFrom(response *http.Response, conn *websocket.Conn) stage9WSLiveHandshake {
	selectedProtocol := ""
	if conn != nil {
		selectedProtocol = conn.Subprotocol()
	}
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	return stage9WSLiveHandshake{Status: status, SelectedProtocol: selectedProtocol}
}

func writeStage9WSLiveSubscription(t *testing.T, conn *websocket.Conn, subscriptions *stage9WSLiveSubscriptions) {
	t.Helper()
	if subscriptions == nil {
		t.Fatal("subscription scenario has no subscription input")
	}
	if err := conn.WriteJSON(map[string]any{"type": "subscribe", "subscriptions": subscriptions}); err != nil {
		t.Fatalf("write subscription: %v", err)
	}
}

func readStage9WSLiveFrame(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set ws-live read deadline: %v", err)
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws-live frame: %v", err)
	}
	return normalizeStage9WSLiveWire(payload)
}

func readStage9WSLiveUntilType(t *testing.T, conn *websocket.Conn, wantType string) []string {
	t.Helper()
	frames := make([]string, 0, 2)
	for {
		frame := readStage9WSLiveFrame(t, conn)
		frames = append(frames, frame)
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(frame), &envelope); err != nil {
			t.Fatalf("decode ws-live frame: %v (%s)", err, frame)
		}
		if envelope.Type == wantType {
			return frames
		}
	}
}

func readStage9WSLiveClose(t *testing.T, conn *websocket.Conn, frames *[]string) stage9WSLiveClose {
	t.Helper()
	for {
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("set ws-live close deadline: %v", err)
		}
		_, payload, err := conn.ReadMessage()
		if err == nil {
			*frames = append(*frames, normalizeStage9WSLiveWire(payload))
			continue
		}
		var closeError *websocket.CloseError
		if errors.As(err, &closeError) {
			return stage9WSLiveClose{Kind: "close-code", Code: closeError.Code}
		}
		return stage9WSLiveClose{Kind: "transport-error"}
	}
}

var stage9WSLiveTimePattern = regexp.MustCompile(`20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z`)

func normalizeStage9WSLiveWire(payload []byte) string {
	return stage9WSLiveTimePattern.ReplaceAllString(string(payload), "fixture-time")
}

var _ apilive.Backend = (*stage9WSLiveBackend)(nil)
