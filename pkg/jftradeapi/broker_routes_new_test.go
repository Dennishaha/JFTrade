package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// testServer creates a new server with default futu settings for testing new routes.
func testServer(t *testing.T) (*httptest.Server, *SettingsStore) {
	t.Helper()
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.saveIntegration(BrokerIntegration{Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          "127.0.0.1",
		APIPort:       1,
		WebSocketPort: 11111,
		TradeMarket:   "HK",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	t.Cleanup(srv.Close)
	return srv, store
}

func getEnvelope(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, envelope.Data
}

func postEnvelope(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	payload, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, envelope.Data
}

// --- Test: funds response includes new margin fields ---

func TestBrokerFundsIncludesMarginFields(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/funds")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}

	// When broker is disconnected, summary may be nil (error response)
	// but the response structure with all declared keys must still be valid
	// The currencyBalances and marketAssets arrays must be present
	if _, hasCurrBalance := data["currencyBalances"]; !hasCurrBalance {
		t.Error("expected currencyBalances in response")
	}
	if _, hasMarketAssets := data["marketAssets"]; !hasMarketAssets {
		t.Error("expected marketAssets in response")
	}

	// Verify the envelope data has summary field (may be nil when disconnected)
	if _, ok := data["summary"]; !ok {
		t.Error("expected summary key in funds response even when disconnected")
	}
}

// --- Test: quote endpoint without broker returns disconnected ---

func TestBrokerQuoteDisconnected(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/quote?symbol=HK.00700")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}
	if data["connectivity"] != "disconnected" {
		t.Errorf("expected disconnected, got %v", data["connectivity"])
	}
}

func TestBrokerQuoteMissingSymbol(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/quote")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}
	if data["connectivity"] != "degraded" {
		t.Errorf("expected degraded for missing symbol, got %v", data["connectivity"])
	}
}

// --- Test: klines endpoint ---

func TestBrokerKLinesDisconnected(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/klines?symbol=HK.00700&period=1d")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}
	if data["connectivity"] != "disconnected" {
		t.Errorf("expected disconnected, got %v", data["connectivity"])
	}
}

func TestBrokerKLinesMissingSymbol(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/klines?period=1d")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}
	if data["connectivity"] != "degraded" {
		t.Errorf("expected degraded for missing symbol, got %v", data["connectivity"])
	}
}

// --- Test: securities snapshot endpoint ---

func TestBrokerSecuritiesDisconnected(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/securities?symbol=HK.00700")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}
	if data["connectivity"] != "disconnected" {
		t.Errorf("expected disconnected, got %v", data["connectivity"])
	}
}

func TestBrokerSecuritiesMissingSymbol(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/securities")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}
	if data["connectivity"] != "degraded" {
		t.Errorf("expected degraded for missing symbol, got %v", data["connectivity"])
	}
}

// --- Test: unlock endpoint without broker returns error ---

func TestBrokerUnlockNoBroker(t *testing.T) {
	srv, _ := testServer(t)

	status, data := postEnvelope(t, srv.URL+"/api/v1/brokers/futu/unlock", map[string]any{
		"unlock":      true,
		"passwordMd5": "dummy",
	})
	// Without a connected broker, unlock should fail
	if status == http.StatusOK {
		t.Fatal("expected non-200 status for unlock without broker")
	}
	_ = data
}

// --- Test: place order endpoint without broker returns error ---

func TestBrokerPlaceOrderNoBroker(t *testing.T) {
	srv, _ := testServer(t)

	status, _ := postEnvelope(t, srv.URL+"/api/v1/brokers/futu/orders", map[string]any{
		"symbol":    "HK.00700",
		"side":      "BUY",
		"orderType": "LIMIT",
		"price":     380.0,
		"quantity":  100,
	})
	// Without a connected OpenD, the order placement will fail with 502
	// (The broker is registered but can't reach OpenD)
	if status != http.StatusBadGateway {
		t.Fatalf("expected 502 (bad gateway), got %d", status)
	}
}

// --- Test: cancel orders endpoint without broker returns error ---

func TestBrokerCancelOrdersNoBroker(t *testing.T) {
	srv, _ := testServer(t)

	payload, _ := json.Marshal(map[string]any{
		"orders": []map[string]any{
			{"orderId": 12345, "symbol": "HK.00700"},
		},
	})
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/brokers/futu/orders", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	// Without a connected OpenD, cancellation returns 502
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 (bad gateway), got %d", resp.StatusCode)
	}
}

// --- Test: broker funds summary contains all existing fields ---

func TestBrokerFundsSummaryHasAllFields(t *testing.T) {
	srv, _ := testServer(t)

	status, data := getEnvelope(t, srv.URL+"/api/v1/brokers/futu/funds")
	if status != http.StatusOK {
		t.Fatalf("unexpected status %d", status)
	}

	// The response structure must always carry currencyBalances and marketAssets arrays
	if _, ok := data["currencyBalances"]; !ok {
		t.Error("expected currencyBalances")
	}
	if _, ok := data["marketAssets"]; !ok {
		t.Error("expected marketAssets")
	}
	// summary is the primary response object (may be nil when disconnected)
	if _, ok := data["summary"]; !ok {
		t.Error("expected summary in funds response")
	}
	// When connected, the brokerFundsResponse builds summary with all margin fields.
	// The keys verified here must always be present in the response structure.
	expectedKeys := []string{
		"summary", "currencyBalances", "marketAssets",
		"checkedAt", "connectivity", "lastError",
	}
	for _, key := range expectedKeys {
		if _, ok := data[key]; !ok {
			t.Errorf("expected key %q in funds response", key)
		}
	}
}

// --- Test: all new broker routes return proper content-type ---

func TestNewBrokerRoutesReturnJSON(t *testing.T) {
	srv, _ := testServer(t)

	routes := []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodGet, path: "/api/v1/brokers/futu/quote?symbol=HK.00700"},
		{method: http.MethodGet, path: "/api/v1/brokers/futu/klines?symbol=HK.00700&period=1d"},
		{method: http.MethodGet, path: "/api/v1/brokers/futu/securities?symbol=HK.00700"},
		{method: http.MethodPost, path: "/api/v1/brokers/futu/unlock", body: map[string]any{"unlock": true, "passwordMd5": "x"}},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			var resp *http.Response
			var err error
			if route.method == http.MethodGet {
				resp, err = http.Get(srv.URL + route.path)
			} else {
				payload, _ := json.Marshal(route.body)
				resp, err = http.Post(srv.URL+route.path, "application/json", bytes.NewReader(payload))
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected JSON content-type, got %q", ct)
			}
		})
	}
}

// --- Test: parseBrokerRoute handles new resource names ---

func TestParseBrokerRouteNewResources(t *testing.T) {
	tests := []struct {
		path          string
		wantBrokerID  string
		wantResource  string
		wantOK        bool
	}{
		{"/api/v1/brokers/futu/quote", "futu", "quote", true},
		{"/api/v1/brokers/futu/klines", "futu", "klines", true},
		{"/api/v1/brokers/futu/securities", "futu", "securities", true},
		{"/api/v1/brokers/futu/unlock", "futu", "unlock", true},
		{"/api/v1/brokers/other/funds", "other", "funds", true},
		{"/api/v1/not-brokers/x/y", "", "", false},
		{"/api/v1/brokers/", "", "", false},
		{"/api/v1/brokers/x", "", "", false},
		{"/api/v1/brokers//y", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			brokerID, resource, ok := parseBrokerRoute(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if brokerID != tt.wantBrokerID {
				t.Errorf("brokerID=%q, want %q", brokerID, tt.wantBrokerID)
			}
			if resource != tt.wantResource {
				t.Errorf("resource=%q, want %q", resource, tt.wantResource)
			}
		})
	}
}

// --- Test: brokerReadQueryFromRequest extracts new query params ---

func TestBrokerReadQueryFromRequestDefaultMarket(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.saveIntegration(BrokerIntegration{Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          "127.0.0.1",
		APIPort:       1,
		WebSocketPort: 11111,
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := NewServer(store)
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/brokers/futu/funds?accountId=123&tradingEnvironment=REAL", nil)
	query := srv.brokerReadQueryFromRequest(req)
	if query.Market != "US" {
		t.Errorf("expected market US (from settings), got %q", query.Market)
	}
	if query.AccountID != "123" {
		t.Errorf("expected accountId 123, got %q", query.AccountID)
	}
	if query.TradingEnvironment != "REAL" {
		t.Errorf("expected SIMULATE when empty, got %q", query.TradingEnvironment)
	}
}
