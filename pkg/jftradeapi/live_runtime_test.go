package jftradeapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/gorilla/websocket"
)

func TestLiveSocketDiagnosticsUseConfiguredLimit(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    "127.0.0.1",
			APIPort:                 11110,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 2,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	server := NewServer(store)
	limit := server.effectiveLiveWebSocketLimit()
	if limit != 2 {
		t.Fatalf("effectiveLiveWebSocketLimit = %d", limit)
	}
	if !server.tryAcquireLiveWebSocketSlot(limit) {
		t.Fatal("expected to acquire first websocket slot")
	}
	if !server.tryAcquireLiveWebSocketSlot(limit) {
		t.Fatal("expected to acquire second websocket slot")
	}
	if server.tryAcquireLiveWebSocketSlot(limit) {
		t.Fatal("expected third websocket slot acquisition to be rejected")
	}

	diagnostics := server.liveSocketDiagnostics(store.integration().Config)
	if got := diagnostics["configuredOpenDWebSocketLimit"]; got != 2 {
		t.Fatalf("configuredOpenDWebSocketLimit = %#v", got)
	}
	if got := diagnostics["jftradeLiveWebSocketLimit"]; got != 2 {
		t.Fatalf("jftradeLiveWebSocketLimit = %#v", got)
	}
	if got := diagnostics["configuredOpenDWebSocketLimitActive"]; got != false {
		t.Fatalf("configuredOpenDWebSocketLimitActive = %#v", got)
	}
	if got := diagnostics["likelyConnectionSaturation"]; got != true {
		t.Fatalf("likelyConnectionSaturation = %#v", got)
	}

	server.releaseLiveWebSocketSlot()
	server.releaseLiveWebSocketSlot()
}

func TestLiveMarketStreamConnectFailureBacksOff(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    "127.0.0.1",
			APIPort:                 1,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	server := NewServer(store)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	server.ensureLiveMarketStream(ctx, []string{"HK.00700"})

	// Stream connect now runs in a background goroutine so the websocket
	// dispatch loop is not blocked by a slow OpenD handshake. Wait for the
	// failure state to settle before asserting on it.
	var (
		failureCount int
		retryAfter   time.Time
		lastError    string
		stream       bbgotypes.Stream
	)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		server.liveStreamState.mu.Lock()
		failureCount = server.liveStreamState.failureCount
		retryAfter = server.liveStreamState.retryAfter
		lastError = server.liveStreamState.lastError
		stream = server.liveStreamState.stream
		server.liveStreamState.mu.Unlock()
		if stream == nil && failureCount > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if stream != nil {
		t.Fatal("expected failed stream to be cleared")
	}
	if failureCount != 1 {
		t.Fatalf("liveStreamFailureCount = %d", failureCount)
	}
	if retryAfter.IsZero() || !retryAfter.After(time.Now().UTC()) {
		t.Fatalf("expected future retryAfter, got %s", retryAfter)
	}
	if lastError == "" {
		t.Fatal("expected last stream error to be recorded")
	}

	server.ensureLiveMarketStream(context.Background(), []string{"HK.00700"})
	server.liveStreamState.mu.Lock()
	deferredFailureCount := server.liveStreamState.failureCount
	deferredRetryAfter := server.liveStreamState.retryAfter
	server.liveStreamState.mu.Unlock()
	if deferredFailureCount != failureCount {
		t.Fatalf("expected retry to be deferred, failure count %d -> %d", failureCount, deferredFailureCount)
	}
	if !deferredRetryAfter.Equal(retryAfter) {
		t.Fatalf("expected retryAfter to stay %s, got %s", retryAfter, deferredRetryAfter)
	}
}

func TestLiveQuoteRefreshFailureBacksOff(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC()
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    "127.0.0.1",
			APIPort:                 1,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now.Format(time.RFC3339Nano),
		CreatedAt: now.Format(time.RFC3339Nano),
	}
	store.mu.Unlock()

	server := NewServer(store)
	server.marketSubscriptions.seed(&marketSubscription{
		Key:          "tick:HK:00700",
		Channel:      "TICK",
		Market:       "HK",
		Symbol:       "00700",
		InstrumentID: "HK.00700",
		Consumers:    map[string]time.Time{"test": now},
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	server.refreshLiveMarketTicksIfNeeded(ctx)

	server.liveQuoteState.mu.Lock()
	failureCount := server.liveQuoteState.failureCount
	retryAfter := server.liveQuoteState.retryAfter
	lastError := server.liveQuoteState.lastError
	server.liveQuoteState.mu.Unlock()
	if failureCount != 1 {
		t.Fatalf("liveQuoteFailureCount = %d", failureCount)
	}
	if retryAfter.IsZero() || !retryAfter.After(time.Now().UTC()) {
		t.Fatalf("expected future live quote retryAfter, got %s", retryAfter)
	}
	if lastError == "" {
		t.Fatal("expected live quote last error to be recorded")
	}

	server.refreshLiveMarketTicksIfNeeded(context.Background())
	server.liveQuoteState.mu.Lock()
	deferredFailureCount := server.liveQuoteState.failureCount
	deferredRetryAfter := server.liveQuoteState.retryAfter
	server.liveQuoteState.mu.Unlock()
	if deferredFailureCount != failureCount {
		t.Fatalf("expected quote retry to be deferred, failure count %d -> %d", failureCount, deferredFailureCount)
	}
	if !deferredRetryAfter.Equal(retryAfter) {
		t.Fatalf("expected quote retryAfter to stay %s, got %s", retryAfter, deferredRetryAfter)
	}
}

func TestLiveWebSocketLimitRejectsAndRecoversEndToEnd(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:                    "futu",
			Host:                    "127.0.0.1",
			APIPort:                 11110,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 1,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()

	server := NewServer(store)
	srv := httptest.NewServer(server)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/live"

	first, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("first Dial: %v", err)
	}
	defer first.Close()
	_ = first.SetReadDeadline(time.Now().Add(2 * time.Second))
	var heartbeat map[string]any
	if err := first.ReadJSON(&heartbeat); err != nil {
		t.Fatalf("first heartbeat: %v", err)
	}

	second, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		_ = second.Close()
		t.Fatal("expected second Dial to be rejected")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("second Dial status = %d, err = %v", status, err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first websocket: %v", err)
	}
	waitUntil(t, func() bool {
		count, _, _ := server.liveWebSocketStats()
		return count == 0
	})

	third, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("third Dial after release: %v", err)
	}
	defer third.Close()
	_ = third.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := third.ReadJSON(&heartbeat); err != nil {
		t.Fatalf("third heartbeat: %v", err)
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for condition")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
