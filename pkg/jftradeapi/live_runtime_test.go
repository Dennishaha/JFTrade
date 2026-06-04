package jftradeapi

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
)

func TestLiveStreamDiagnosticsUseConfiguredLimit(t *testing.T) {
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

	server := newTestServer(t, store)
	limit := server.effectiveLiveStreamLimit()
	if limit != 2 {
		t.Fatalf("effectiveLiveStreamLimit = %d", limit)
	}
	if !server.tryAcquireLiveStreamSlot(limit) {
		t.Fatal("expected to acquire first live stream slot")
	}
	if !server.tryAcquireLiveStreamSlot(limit) {
		t.Fatal("expected to acquire second live stream slot")
	}
	if server.tryAcquireLiveStreamSlot(limit) {
		t.Fatal("expected third live stream slot acquisition to be rejected")
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

	server.releaseLiveStreamSlot()
	server.releaseLiveStreamSlot()
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

	server := newTestServer(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	server.ensureLiveMarketStream(ctx, []string{"HK.00700"})

	// Stream connect now runs in a background goroutine so the live stream
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

	server := newTestServer(t, store)
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
