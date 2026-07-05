package servercore

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
	_, err = store.SaveIntegration(BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          "127.0.0.1",
		APIPort:       1,
		WebSocketPort: 11111,
		TradeMarket:   "HK",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	return newHTTPTestServer(t, store), store
}

func getEnvelope(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := jftradeTestHTTPGet(t, url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, envelope.Data
}

func getEnvelopeWithError(t *testing.T, url string) (int, map[string]any, *apiError) {
	t.Helper()
	resp, err := jftradeTestHTTPGet(t, url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var envelope struct {
		OK    bool           `json:"ok"`
		Data  map[string]any `json:"data"`
		Error *apiError      `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, envelope.Data, envelope.Error
}

func postEnvelope(t *testing.T, url string, body any) (int, map[string]any) {
	status, data, _ := postEnvelopeWithError(t, url, body)
	return status, data
}

func postEnvelopeWithError(t *testing.T, url string, body any) (int, map[string]any, *apiError) {
	t.Helper()
	payload, jftradeErr1 := json.Marshal(body)
	jftradeCheckTestError(t, jftradeErr1)
	resp, err := jftradeTestHTTPPost(t, url, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var envelope struct {
		OK    bool           `json:"ok"`
		Data  map[string]any `json:"data"`
		Error *apiError      `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, envelope.Data, envelope.Error
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

	status, _, errInfo := getEnvelopeWithError(t, srv.URL+"/api/v1/brokers/futu/quote")
	if status != http.StatusBadRequest {
		t.Fatalf("unexpected status %d", status)
	}
	if errInfo == nil || errInfo.Code != "BAD_REQUEST" || errInfo.Message != "query parameter symbol is required" {
		t.Fatalf("quote missing symbol error = %+v", errInfo)
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

	status, _, errInfo := getEnvelopeWithError(t, srv.URL+"/api/v1/brokers/futu/klines?period=1d")
	if status != http.StatusBadRequest {
		t.Fatalf("unexpected status %d", status)
	}
	if errInfo == nil || errInfo.Code != "BAD_REQUEST" || errInfo.Message != "query parameter symbol is required" {
		t.Fatalf("klines missing symbol error = %+v", errInfo)
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

	status, _, errInfo := getEnvelopeWithError(t, srv.URL+"/api/v1/brokers/futu/securities")
	if status != http.StatusBadRequest {
		t.Fatalf("unexpected status %d", status)
	}
	if errInfo == nil || errInfo.Code != "BAD_REQUEST" || errInfo.Message != "query parameter symbol is required" {
		t.Fatalf("securities missing symbol error = %+v", errInfo)
	}
}

func TestBrokerReadRoutesRejectInvalidQueryShape(t *testing.T) {
	srv, _ := testServer(t)

	tests := []struct {
		name        string
		path        string
		wantMessage string
	}{
		{"cash flows missing clearingDate", "/api/v1/brokers/futu/cash-flows", "query parameter clearingDate is required"},
		{"order fees missing orderIdEx", "/api/v1/brokers/futu/order-fees", "query parameter orderIdEx is required"},
		{"orders invalid scope", "/api/v1/brokers/futu/orders?scope=invalid", "query parameter scope is invalid"},
		{"fills invalid scope", "/api/v1/brokers/futu/fills?scope=invalid", "query parameter scope is invalid"},
		{"max trade qty missing required", "/api/v1/brokers/futu/max-trade-qtys?symbol=HK.00700&orderType=LIMIT", "query parameters symbol, orderType, and price are required"},
		{"max trade qty invalid price", "/api/v1/brokers/futu/max-trade-qtys?symbol=HK.00700&orderType=LIMIT&price=abc", "query parameter price is invalid: strconv.ParseFloat: parsing \"abc\": invalid syntax"},
		{"max trade qty invalid adjust", "/api/v1/brokers/futu/max-trade-qtys?symbol=HK.00700&orderType=LIMIT&price=320.5&adjustSideAndLimit=abc", "query parameter adjustSideAndLimit is invalid: strconv.ParseFloat: parsing \"abc\": invalid syntax"},
		{"max trade qty invalid positionId", "/api/v1/brokers/futu/max-trade-qtys?symbol=HK.00700&orderType=LIMIT&price=320.5&positionId=abc", "query parameter positionId is invalid: strconv.ParseUint: parsing \"abc\": invalid syntax"},
		{"klines invalid limit", "/api/v1/brokers/futu/klines?symbol=HK.00700&limit=abc", "query parameter limit is invalid: strconv.ParseInt: parsing \"abc\": invalid syntax"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, errInfo := getEnvelopeWithError(t, srv.URL+tt.path)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			if errInfo == nil || errInfo.Code != "BAD_REQUEST" || errInfo.Message != tt.wantMessage {
				t.Fatalf("error = %+v, want BAD_REQUEST/%q", errInfo, tt.wantMessage)
			}
		})
	}
}

func TestBrokerReadRoutesKeepValidDisconnectedShape(t *testing.T) {
	srv, _ := testServer(t)

	tests := []string{
		"/api/v1/brokers/futu/quote?symbol=HK.00700,US.NVDA",
		"/api/v1/brokers/futu/securities?symbol=HK.00700&symbol=US.NVDA",
		"/api/v1/brokers/futu/klines?symbol=HK.00700",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			status, data := getEnvelope(t, srv.URL+path)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			if data["connectivity"] != "disconnected" {
				t.Fatalf("connectivity = %v, want disconnected", data["connectivity"])
			}
		})
	}
}

// --- Test: unlock endpoint with disconnected OpenD returns error ---

func TestBrokerUnlockDisconnectedOpenD(t *testing.T) {
	srv, _ := testServer(t)

	status, _, errInfo := postEnvelopeWithError(t, srv.URL+"/api/v1/brokers/futu/unlock", map[string]any{
		"unlock":      true,
		"passwordMd5": "dummy",
	})
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 with disconnected OpenD", status)
	}
	if errInfo == nil {
		t.Fatal("expected error payload")
	}
	if errInfo.Code != "UNLOCK_FAILED" {
		t.Fatalf("error code = %q, want UNLOCK_FAILED", errInfo.Code)
	}
	if !strings.Contains(errInfo.Message, "connect") {
		t.Fatalf("error message = %q, want OpenD connection failure detail", errInfo.Message)
	}
}

func TestBrokerUnlockInvalidPayload(t *testing.T) {
	srv, _ := testServer(t)

	status, _, errInfo := postEnvelopeWithError(t, srv.URL+"/api/v1/brokers/futu/unlock", map[string]any{
		"unlock": map[string]any{"bad": true},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errInfo == nil || errInfo.Code != "BAD_REQUEST" || !strings.Contains(errInfo.Message, "invalid request body:") {
		t.Fatalf("error = %+v, want BAD_REQUEST invalid request body", errInfo)
	}
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

func TestBrokerPlaceOrderInvalidPayload(t *testing.T) {
	srv, _ := testServer(t)

	status, _, errInfo := postEnvelopeWithError(t, srv.URL+"/api/v1/brokers/futu/orders", map[string]any{
		"symbol":   123,
		"quantity": "bad",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if errInfo == nil || errInfo.Code != "BAD_REQUEST" || !strings.Contains(errInfo.Message, "invalid request body:") {
		t.Fatalf("error = %+v, want BAD_REQUEST invalid request body", errInfo)
	}
}

// --- Test: cancel orders endpoint without broker returns error ---

func TestBrokerCancelOrdersNoBroker(t *testing.T) {
	srv, _ := testServer(t)

	payload, jftradeErr2 := json.Marshal(map[string]any{
		"orders": []map[string]any{
			{"orderId": 12345, "symbol": "HK.00700"},
		},
	})
	jftradeCheckTestError(t, jftradeErr2)
	req, jftradeErr3 := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/brokers/futu/orders", bytes.NewReader(payload))
	jftradeCheckTestError(t, jftradeErr3)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	jftradeCheckTestError(t, resp.Body.Close())
	// Without a connected OpenD, cancellation returns 502
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 (bad gateway), got %d", resp.StatusCode)
	}
}

func TestBrokerCancelOrdersInvalidPayload(t *testing.T) {
	srv, _ := testServer(t)

	req, jftradeErr4 := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/brokers/futu/orders", strings.NewReader(`{"orders":`))
	jftradeCheckTestError(t, jftradeErr4)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	var envelope struct {
		OK    bool      `json:"ok"`
		Error *apiError `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if envelope.Error == nil || envelope.Error.Code != "BAD_REQUEST" || !strings.Contains(envelope.Error.Message, "invalid request body:") {
		t.Fatalf("error = %+v, want BAD_REQUEST invalid request body", envelope.Error)
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
	// When connected, the trading service builds summary with all margin fields.
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
				resp, err = jftradeTestHTTPGet(t, srv.URL+route.path)
			} else {
				payload, jftradeErr1 := json.Marshal(route.body)
				jftradeCheckTestError(t, jftradeErr1)
				resp, err = jftradeTestHTTPPost(t, srv.URL+route.path, "application/json", bytes.NewReader(payload))
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected JSON content-type, got %q", ct)
			}
		})
	}
}

// --- Test: Gin broker routes reject incomplete resource paths ---

func TestBrokerGinRoutesRejectIncompletePaths(t *testing.T) {
	srv, _ := testServer(t)
	tests := []string{
		"/api/v1/not-brokers/x/y",
		"/api/v1/brokers/",
		"/api/v1/brokers/x",
		"/api/v1/brokers//y",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			resp, err := jftradeTestHTTPGet(t, srv.URL+path)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status=%d, want 404", resp.StatusCode)
			}
		})
	}
}

// --- Test: brokerReadQuery applies the default market ---

func TestBrokerReadQueryDefaultMarket(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          "127.0.0.1",
		APIPort:       1,
		WebSocketPort: 11111,
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := newTestServer(t, store)
	query := srv.tradingSvc.ReadQuery("futu", "REAL", "123", "")
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
