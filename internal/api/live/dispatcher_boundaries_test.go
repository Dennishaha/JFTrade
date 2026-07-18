package live

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	livecore "github.com/jftrade/jftrade-main/internal/live"
)

type failingEventWriter struct {
	err error
}

func (w failingEventWriter) WriteEvent(any) error { return w.err }

func TestDispatcherInitialAndLiveDataFailures(t *testing.T) {
	wantErr := errors.New("websocket write failed")

	t.Run("heartbeat write", func(t *testing.T) {
		d := newTestDispatcher(t, &fakeBackend{}, livecore.Subscriptions{}, failingEventWriter{err: wantErr})
		if err := d.writeInitialEvents(); !errors.Is(err, wantErr) {
			t.Fatalf("writeInitialEvents error = %v, want %v", err, wantErr)
		}
	})

	t.Run("market tick provider", func(t *testing.T) {
		backendErr := errors.New("market data unavailable")
		d := newTestDispatcher(t, &fakeBackend{ticksErr: backendErr}, livecore.Subscriptions{}, &recordingWriter{})
		if err := d.writeInitialEvents(); !errors.Is(err, backendErr) {
			t.Fatalf("writeInitialEvents error = %v, want %v", err, backendErr)
		}
	})

	t.Run("market tick write", func(t *testing.T) {
		backend := &fakeBackend{ticks: []TickEvent{{
			InstrumentID: "US.AAPL",
			ObservedAt:   "2026-07-02T01:02:03Z",
			Payload:      map[string]any{},
		}}}
		d := newTestDispatcher(t, backend, livecore.Subscriptions{ActiveInstruments: []string{"US.AAPL"}}, failingEventWriter{err: wantErr})
		if err := d.writeLiveData(); !errors.Is(err, wantErr) {
			t.Fatalf("writeLiveData error = %v, want %v", err, wantErr)
		}
	})

	t.Run("notification write", func(t *testing.T) {
		backend := &fakeBackend{notifications: []livecore.Event{{Sequence: 1, At: "2026-07-02T01:02:03Z"}}}
		d := newTestDispatcher(t, backend, livecore.Subscriptions{}, failingEventWriter{err: wantErr})
		if err := d.writeNotifications(); !errors.Is(err, wantErr) {
			t.Fatalf("writeNotifications error = %v, want %v", err, wantErr)
		}
	})
}

func TestDispatcherAuxiliarySubscriptionBranches(t *testing.T) {
	wantErr := errors.New("websocket write failed")
	security := livecore.SecurityDetailsSubscription{Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL"}
	depth := livecore.DepthSubscription{Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Num: 10}

	t.Run("console refresh and error propagation", func(t *testing.T) {
		d := newTestDispatcher(t, &fakeBackend{}, livecore.Subscriptions{ConsoleRefresh: true}, &recordingWriter{})
		if err := d.writeConsoleRefresh(); err != nil {
			t.Fatalf("writeConsoleRefresh: %v", err)
		}
		if err := newTestDispatcher(t, &fakeBackend{}, livecore.Subscriptions{ConsoleRefresh: true}, failingEventWriter{err: wantErr}).writeAuxiliarySubscriptions(false); !errors.Is(err, wantErr) {
			t.Fatalf("writeAuxiliarySubscriptions error = %v, want %v", err, wantErr)
		}
	})

	t.Run("security provider errors are skipped and unchanged data is deduplicated", func(t *testing.T) {
		backend := &fakeBackend{securityErr: errors.New("security unavailable")}
		d := newTestDispatcher(t, backend, livecore.Subscriptions{SecurityDetails: []livecore.SecurityDetailsSubscription{security}}, &recordingWriter{})
		if err := d.writeSecurityDetailsEvents(false); err != nil {
			t.Fatalf("provider failure: %v", err)
		}
		backend.securityErr = nil
		if err := d.writeSecurityDetailsEvents(true); err != nil {
			t.Fatalf("forced security event: %v", err)
		}
		if err := d.writeSecurityDetailsEvents(false); err != nil {
			t.Fatalf("deduplicated security event: %v", err)
		}
		failing := newTestDispatcher(t, backend, livecore.Subscriptions{SecurityDetails: []livecore.SecurityDetailsSubscription{security}}, failingEventWriter{err: wantErr})
		if err := failing.writeSecurityDetailsEvents(true); !errors.Is(err, wantErr) {
			t.Fatalf("security write error = %v, want %v", err, wantErr)
		}
	})

	t.Run("depth provider errors are skipped and unchanged data is deduplicated", func(t *testing.T) {
		backend := &fakeBackend{depthErr: errors.New("depth unavailable")}
		d := newTestDispatcher(t, backend, livecore.Subscriptions{Depth: []livecore.DepthSubscription{depth}}, &recordingWriter{})
		if err := d.writeDepthEvents(false); err != nil {
			t.Fatalf("provider failure: %v", err)
		}
		backend.depthErr = nil
		if err := d.writeDepthEvents(true); err != nil {
			t.Fatalf("forced depth event: %v", err)
		}
		if err := d.writeDepthEvents(false); err != nil {
			t.Fatalf("deduplicated depth event: %v", err)
		}
		failing := newTestDispatcher(t, backend, livecore.Subscriptions{Depth: []livecore.DepthSubscription{depth}}, failingEventWriter{err: wantErr})
		if err := failing.writeDepthEvents(true); !errors.Is(err, wantErr) {
			t.Fatalf("depth write error = %v, want %v", err, wantErr)
		}
	})

	t.Run("security failure bubbles through auxiliary dispatcher", func(t *testing.T) {
		d := newTestDispatcher(t, &fakeBackend{}, livecore.Subscriptions{SecurityDetails: []livecore.SecurityDetailsSubscription{security}}, failingEventWriter{err: wantErr})
		if err := d.writeAuxiliarySubscriptions(true); !errors.Is(err, wantErr) {
			t.Fatalf("writeAuxiliarySubscriptions error = %v, want %v", err, wantErr)
		}
	})
}

func TestDispatcherRunPropagatesTriggerAndTickerFailures(t *testing.T) {
	wantErr := errors.New("dispatch failed")
	hour := time.Hour
	security := livecore.SecurityDetailsSubscription{Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL"}
	depth := livecore.DepthSubscription{Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Num: 10}

	tests := []struct {
		name          string
		backend       *fakeBackend
		subscriptions livecore.Subscriptions
		options       Options
		depthTrigger  bool
		keepUpdate    bool
	}{
		{
			name:          "client subscription update",
			backend:       &fakeBackend{},
			subscriptions: livecore.Subscriptions{ConsoleRefresh: true},
			options:       Options{HeartbeatInterval: hour, DataInterval: hour, ConsoleRefreshInterval: hour, SecurityDetailsInterval: hour, DepthRefreshInterval: hour},
			keepUpdate:    true,
		},
		{
			name:          "depth push",
			backend:       &fakeBackend{},
			subscriptions: livecore.Subscriptions{Depth: []livecore.DepthSubscription{depth}},
			options:       Options{HeartbeatInterval: hour, DataInterval: hour, ConsoleRefreshInterval: hour, SecurityDetailsInterval: hour, DepthRefreshInterval: hour},
			depthTrigger:  true,
		},
		{
			name:    "heartbeat ticker",
			backend: &fakeBackend{},
			options: Options{HeartbeatInterval: time.Millisecond, DataInterval: hour, ConsoleRefreshInterval: hour, SecurityDetailsInterval: hour, DepthRefreshInterval: hour},
		},
		{
			name:    "data ticker",
			backend: &fakeBackend{ticksErr: wantErr},
			options: Options{HeartbeatInterval: hour, DataInterval: time.Millisecond, ConsoleRefreshInterval: hour, SecurityDetailsInterval: hour, DepthRefreshInterval: hour},
		},
		{
			name:          "console ticker",
			backend:       &fakeBackend{},
			subscriptions: livecore.Subscriptions{ConsoleRefresh: true},
			options:       Options{HeartbeatInterval: hour, DataInterval: hour, ConsoleRefreshInterval: time.Millisecond, SecurityDetailsInterval: hour, DepthRefreshInterval: hour},
		},
		{
			name:          "security ticker",
			backend:       &fakeBackend{},
			subscriptions: livecore.Subscriptions{SecurityDetails: []livecore.SecurityDetailsSubscription{security}},
			options:       Options{HeartbeatInterval: hour, DataInterval: hour, ConsoleRefreshInterval: hour, SecurityDetailsInterval: time.Millisecond, DepthRefreshInterval: hour},
		},
		{
			name:          "depth ticker",
			backend:       &fakeBackend{},
			subscriptions: livecore.Subscriptions{Depth: []livecore.DepthSubscription{depth}},
			options:       Options{HeartbeatInterval: hour, DataInterval: hour, ConsoleRefreshInterval: hour, SecurityDetailsInterval: hour, DepthRefreshInterval: time.Millisecond},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHandler(test.backend, test.options)
			client := (&livecore.ClientRegistry{}).Register()
			client.SetSubscriptions(test.subscriptions)
			if !test.keepUpdate {
				<-client.Updated()
			}
			depthUpdated := make(chan struct{}, 1)
			if test.depthTrigger {
				depthUpdated <- struct{}{}
			}
			d := &dispatcher{
				handler: handler, requestCtx: t.Context(), writer: failingEventWriter{err: wantErr}, client: client,
				clientClosed: make(chan struct{}), depthUpdated: depthUpdated,
				lastSentByInstrument: map[string]string{}, lastSecurityResolvedAt: map[string]string{}, lastDepthResolvedAt: map[string]string{},
			}
			if err := d.run(); !errors.Is(err, wantErr) {
				t.Fatalf("run error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestDispatcherEnvelopeDefaultsAndMapFallback(t *testing.T) {
	writer := &recordingWriter{}
	d := &dispatcher{writer: writer}
	if err := d.writeEnvelope("system.test", "system", "", "", "", map[string]any{"status": "ok"}); err != nil {
		t.Fatalf("writeEnvelope: %v", err)
	}
	if len(writer.events) != 1 || writer.events[0]["entityId"] != "system.test" || writer.events[0]["eventId"] == "" {
		t.Fatalf("event = %#v", writer.events)
	}
	if got := mapString(map[string]any{"type": 42}, "type", "fallback"); got != "fallback" {
		t.Fatalf("mapString fallback = %q", got)
	}
}

func TestProviderAwareBackendHelpersRetainExplicitBrokerSelection(t *testing.T) {
	backend := &providerAwareFakeBackend{fakeBackend: &fakeBackend{}}
	stats := ClientStats{Connected: 1, Limit: 2}
	if heartbeat := providerHeartbeat(backend, time.Second, stats, []string{"US.AAPL"}, "alpha"); heartbeat["brokerId"] != "alpha" {
		t.Fatalf("provider heartbeat = %#v", heartbeat)
	}
	if _, err := providerMarketTicks(backend, t.Context(), "alpha", []string{"US.AAPL"}, ""); err != nil {
		t.Fatalf("provider market ticks: %v", err)
	}
	if _, err := providerSecurityDetails(backend, t.Context(), "alpha", "US", "AAPL"); err != nil {
		t.Fatalf("provider security details: %v", err)
	}
	if _, err := providerDepth(backend, t.Context(), "alpha", "US", "AAPL", 10); err != nil {
		t.Fatalf("provider depth: %v", err)
	}
	if backend.providerCalls != 4 {
		t.Fatalf("provider-aware calls = %d, want 4", backend.providerCalls)
	}
	if checkWebSocketOrigin(nil) {
		t.Fatal("nil websocket request origin was accepted")
	}
	httpsRequest := httptest.NewRequest(http.MethodGet, "https://example.test/ws/live", nil)
	httpsRequest.Host = "example.test"
	httpsRequest.Header.Set("Origin", "https://example.test")
	httpsRequest.TLS = &tls.ConnectionState{}
	if !checkWebSocketOrigin(httpsRequest) {
		t.Fatal("same-origin HTTPS websocket request was rejected")
	}
	t.Run("checked type assertion rejects unexpected values", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("checked type assertion did not panic")
			}
		}()
		_ = jftradeCheckedTypeAssertion[int]("not an integer")
	})
}

type providerAwareFakeBackend struct {
	*fakeBackend
	providerCalls int
}

func (b *providerAwareFakeBackend) HeartbeatForProvider(
	time.Duration,
	ClientStats,
	[]string,
	string,
) map[string]any {
	b.providerCalls++
	return map[string]any{"brokerId": "alpha"}
}

func (b *providerAwareFakeBackend) MarketTicksForProvider(
	context.Context,
	string,
	[]string,
	string,
) ([]TickEvent, error) {
	b.providerCalls++
	return nil, nil
}

func (b *providerAwareFakeBackend) SecurityDetailsForProvider(
	context.Context,
	string,
	string,
	string,
) (map[string]any, error) {
	b.providerCalls++
	return map[string]any{}, nil
}

func (b *providerAwareFakeBackend) DepthForProvider(
	context.Context,
	string,
	string,
	string,
	int32,
) (map[string]any, error) {
	b.providerCalls++
	return map[string]any{}, nil
}

func TestHandlerNilAndClosedLifecycleBoundaries(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ws/live", nil)
	for _, handler := range []*Handler{nil, {}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("nil backend status = %d", response.Code)
		}
	}

	var nilHandler *Handler
	if stats := nilHandler.Stats(); stats != (ClientStats{}) {
		t.Fatalf("nil stats = %#v", stats)
	}
	if ids := nilHandler.ActiveInstrumentIDs(); ids != nil {
		t.Fatalf("nil active IDs = %#v", ids)
	}
	if err := nilHandler.Close(); err != nil {
		t.Fatalf("nil close: %v", err)
	}

	handler := NewHandler(&fakeBackend{}, Options{})
	if limit := handler.connectionLimit(); limit != defaultConnectionLimit {
		t.Fatalf("default connection limit = %d", limit)
	}
	if !handler.tryAcquire(1) {
		t.Fatal("first acquire failed")
	}
	handler.release(nil)
	handler.active.Done()
	if err := handler.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if handler.tryAcquire(1) {
		t.Fatal("closed handler accepted a connection")
	}
}

func TestDepthUpdateSubscriptionFiltersAndCoalesces(t *testing.T) {
	backend := &fakeBackend{nilUnsubscribe: true}
	handler := NewHandler(backend, Options{})
	client := (&livecore.ClientRegistry{}).Register()
	client.SetSubscriptions(livecore.Subscriptions{Depth: []livecore.DepthSubscription{{
		Market: "US", Symbol: "AAPL", InstrumentID: "US.AAPL", Num: 10,
	}}})
	updates, unsubscribe := handler.subscribeDepthUpdates(client)
	unsubscribe()

	backend.mu.Lock()
	callback := backend.depthSubscriber
	backend.mu.Unlock()
	callback("US.MSFT")
	select {
	case <-updates:
		t.Fatal("unrelated symbol triggered a depth update")
	default:
	}
	callback(" us.aapl ")
	callback("US.AAPL")
	select {
	case <-updates:
	default:
		t.Fatal("matching symbol did not trigger a depth update")
	}
}

func newTestDispatcher(t *testing.T, backend *fakeBackend, subscriptions livecore.Subscriptions, writer eventWriter) *dispatcher {
	t.Helper()
	handler := NewHandler(backend, Options{})
	client := (&livecore.ClientRegistry{}).Register()
	client.SetSubscriptions(subscriptions)
	return &dispatcher{
		handler: handler, requestCtx: t.Context(), writer: writer, client: client,
		lastSentByInstrument: map[string]string{}, lastSecurityResolvedAt: map[string]string{}, lastDepthResolvedAt: map[string]string{},
	}
}
