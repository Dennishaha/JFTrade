package jftradeapi

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", portText, err)
	}
	return host, port
}

func newMarketDataTestServerWithQuoteRuntime(t *testing.T, addr string) *Server {
	t.Helper()
	host, port := splitHostPort(t, addr)
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
			Host:                    host,
			APIPort:                 port,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
			TradeMarket:             "HK",
			SecurityFirm:            "FUTUSECURITIES",
		}),
		UpdatedAt: now,
		CreatedAt: now,
	}
	store.mu.Unlock()
	return NewServer(store)
}

func seedCachedTickSample(server *Server, sample marketTickSample) {
	server.tickCache.seed(sample)
}

func float64Ptr(v float64) *float64 {
	return &v
}

func assertSnapshotResponse(t *testing.T, response map[string]any, instrumentID string, fromCache bool, source string) {
	t.Helper()
	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	if got := request["instrumentId"]; got != instrumentID {
		t.Fatalf("instrumentId = %v, want %s", got, instrumentID)
	}
	snapshot, ok := response["snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot payload type = %T", response["snapshot"])
	}
	if got := snapshot["price"]; got == nil {
		t.Fatal("expected snapshot price")
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != fromCache {
		t.Fatalf("fromCache = %v, want %v", got, fromCache)
	}
	if got := meta["source"]; got != source {
		t.Fatalf("source = %v, want %s", got, source)
	}
	if got := meta["instrumentId"]; got != instrumentID {
		t.Fatalf("meta instrumentId = %v, want %s", got, instrumentID)
	}
}

func assertTickCandlesResponse(t *testing.T, response map[string]any, instrumentID string, fromCache bool, wantCount int) {
	t.Helper()
	request, ok := response["request"].(map[string]any)
	if !ok {
		t.Fatalf("request payload type = %T", response["request"])
	}
	instrument, ok := request["instrument"].(map[string]any)
	if !ok {
		t.Fatalf("instrument payload type = %T", request["instrument"])
	}
	if got := instrument["instrumentId"]; got != instrumentID {
		t.Fatalf("instrumentId = %v, want %s", got, instrumentID)
	}
	totalReturned, ok := response["totalReturned"].(int)
	if !ok {
		t.Fatalf("totalReturned payload type = %T", response["totalReturned"])
	}
	if totalReturned != wantCount {
		t.Fatalf("totalReturned = %d, want %d", totalReturned, wantCount)
	}
	candles, ok := response["candles"].([]map[string]any)
	if !ok {
		t.Fatalf("candles payload type = %T", response["candles"])
	}
	if len(candles) != wantCount {
		t.Fatalf("len(candles) = %d, want %d", len(candles), wantCount)
	}
	if wantCount > 0 {
		if _, ok := candles[0]["open"].(string); !ok {
			t.Fatalf("tick candle open payload type = %T", candles[0]["open"])
		}
	}
	meta, ok := response["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta payload type = %T", response["meta"])
	}
	if got := meta["fromCache"]; got != fromCache {
		t.Fatalf("fromCache = %v, want %v", got, fromCache)
	}
}
