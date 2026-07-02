package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	srv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestSubscriptionRoutesUseInstrumentRequestContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(&routeTestProvider{})
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, service)

	first := postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId": "chart-main",
		"instruments": []any{
			map[string]any{"channel": "KLINE", "market": "hk", "symbol": "00700", "interval": "1m"},
		},
	})
	entry := singleRouteEntry(t, first)
	if entry["key"] != "KLINE:HK:00700:1m" || entry["channel"] != "KLINE" || entry["interval"] != "1m" {
		t.Fatalf("instrument channel/interval were not preserved: %#v", entry)
	}

	postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId": "chart-main",
		"instruments": []any{
			map[string]any{"market": "US", "symbol": "AAPL"},
		},
	})
	postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId": "other",
		"instruments": []any{
			map[string]any{"market": "HK", "symbol": "00700"},
		},
	})

	released := postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions/release", map[string]any{
		"consumerId": "chart-main",
		"instruments": []any{
			map[string]any{"channel": "KLINE", "market": "HK", "symbol": "00700", "interval": "1m"},
		},
	})
	if released["released"] != true {
		t.Fatalf("release response = %#v", released)
	}
	if released["totalActiveSubscriptions"] != float64(2) {
		t.Fatalf("snapshot after single release = %#v", released)
	}

	snapshot := getSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions")
	entries := routeEntriesByKey(t, snapshot)
	if _, exists := entries["KLINE:HK:00700:1m"]; exists {
		t.Fatalf("released entry still present: %#v", snapshot)
	}
	if _, exists := entries["SNAPSHOT:US:AAPL"]; !exists {
		t.Fatalf("chart-main snapshot entry was removed by single release: %#v", snapshot)
	}
	if _, exists := entries["SNAPSHOT:HK:00700"]; !exists {
		t.Fatalf("other consumer entry missing after single release: %#v", snapshot)
	}

	cleared := deleteSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions?consumerId=other")
	if cleared["cleared"] != true {
		t.Fatalf("delete response = %#v", cleared)
	}
	snapshot = getSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions")
	entry = singleRouteEntry(t, snapshot)
	consumers := jftradeCheckedTypeAssertion[[]any](entry["consumers"])
	if entry["key"] != "SNAPSHOT:US:AAPL" || len(consumers) != 1 || consumers[0] != "chart-main" || entry["refCount"] != float64(1) {
		t.Fatalf("remaining entry after consumer delete = %#v", entry)
	}

	deleteSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions")
	snapshot = getSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions")
	if snapshot["totalActiveSubscriptions"] != float64(0) {
		t.Fatalf("snapshot after clear all = %#v", snapshot)
	}
}

func TestSubscriptionReleaseConsumerOnlyClearsConsumer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(&routeTestProvider{})
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, service)

	postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId": "chart-main",
		"instruments": []any{
			map[string]any{"market": "HK", "symbol": "00700"},
			map[string]any{"market": "US", "symbol": "AAPL"},
		},
	})
	postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId": "other",
		"instruments": []any{
			map[string]any{"market": "HK", "symbol": "00700"},
		},
	})

	released := postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions/release", map[string]any{
		"consumerId": "chart-main",
	})
	if released["totalActiveSubscriptions"] != float64(1) {
		t.Fatalf("snapshot after consumer-only release = %#v", released)
	}
	entry := singleRouteEntry(t, released)
	consumers := jftradeCheckedTypeAssertion[[]any](entry["consumers"])
	if entry["key"] != "SNAPSHOT:HK:00700" || len(consumers) != 1 || consumers[0] != "other" {
		t.Fatalf("remaining entry after consumer-only release = %#v", entry)
	}
}

func TestCandlesRoutePreservesLegacyQueryParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	service := srv.NewService(provider)
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, service)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/candles/us/aapl?period=k_60m&limit=5&from=2026-05-01&to=2026-05-02%2015:04:05", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET candles status = %d, body = %s", response.Code, response.Body.String())
	}
	if !provider.candlesCalled {
		t.Fatal("GetHistoricalCandles was not called")
	}
	if provider.candlesMarket != "us" || provider.candlesSymbol != "aapl" {
		t.Fatalf("instrument = %s/%s", provider.candlesMarket, provider.candlesSymbol)
	}
	if provider.candlesPeriod != "1h" {
		t.Fatalf("period = %q, want 1h", provider.candlesPeriod)
	}
	if provider.candlesLimit != 5 {
		t.Fatalf("limit = %d, want 5", provider.candlesLimit)
	}
	if !strings.HasPrefix(provider.candlesFromTime, "2026-05-01T00:00:00") {
		t.Fatalf("fromTime = %q", provider.candlesFromTime)
	}
	if !strings.HasPrefix(provider.candlesToTime, "2026-05-02T15:04:05") {
		t.Fatalf("toTime = %q", provider.candlesToTime)
	}
}

func TestCandlesRouteRejectsUnsupportedPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	service := srv.NewService(provider)
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, service)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/candles/HK/00700?period=unsupported", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("GET candles status = %d, body = %s", response.Code, response.Body.String())
	}
	if provider.candlesCalled {
		t.Fatal("GetHistoricalCandles should not be called for invalid period")
	}
}

func TestCandlesRouteDefaultsInvalidLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	service := srv.NewService(provider)
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, service)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/candles/HK/00700?limit=abc", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET candles status = %d, body = %s", response.Code, response.Body.String())
	}
	if !provider.candlesCalled {
		t.Fatal("GetHistoricalCandles should be called with the default limit")
	}
	if provider.candlesLimit != 0 {
		t.Fatalf("limit = %d, want 0", provider.candlesLimit)
	}
}

func TestDepthRouteDefaultsInvalidNum(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	service := srv.NewService(provider)
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, service)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/depth/HK/00700?num=abc", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET depth status = %d, body = %s", response.Code, response.Body.String())
	}
	if !provider.depthCalled {
		t.Fatal("GetDepth should be called with the default num")
	}
	if provider.depthNum != 10 {
		t.Fatalf("num = %d, want 10", provider.depthNum)
	}
}

func TestReadRoutesCoverMarketsSecuritySnapshotSearchHeartbeatAndNormalize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{
		markets: []srv.MarketProfile{{"market": "US"}},
		securityDetails: srv.SecurityDetails{
			"instrument": map[string]any{"market": "US", "symbol": "AAPL"},
		},
		snapshot: &srv.Tick{
			InstrumentID: "US.AAPL",
			Market:       "US",
			Symbol:       "AAPL",
			Price:        decimal.RequireFromString("101.5"),
			Bid:          decimal.RequireFromString("101.4"),
			Ask:          decimal.RequireFromString("101.6"),
			ObservedAt:   "2026-06-22T00:00:00Z",
			QuoteAt:      "2026-06-22T00:00:00Z",
			Source:       "test-feed",
		},
		normalizedInstrument: map[string]any{
			"market":       "US",
			"symbol":       "AAPL",
			"instrumentId": "US.AAPL",
		},
	}
	service := srv.NewService(provider)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	marketsRec := httptest.NewRecorder()
	router.ServeHTTP(marketsRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/markets", nil))
	if marketsRec.Code != http.StatusOK || !strings.Contains(marketsRec.Body.String(), `"defaultMarket":"HK"`) {
		t.Fatalf("markets = %d %s", marketsRec.Code, marketsRec.Body.String())
	}

	securityRec := httptest.NewRecorder()
	router.ServeHTTP(securityRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/securities/us/aapl", nil))
	if securityRec.Code != http.StatusOK || provider.securityMarket != "us" || provider.securitySymbol != "aapl" {
		t.Fatalf("security = %d %s provider=%s/%s", securityRec.Code, securityRec.Body.String(), provider.securityMarket, provider.securitySymbol)
	}

	snapshotRec := httptest.NewRecorder()
	router.ServeHTTP(snapshotRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/snapshots/us/aapl?refresh=true", nil))
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot = %d %s", snapshotRec.Code, snapshotRec.Body.String())
	}
	if provider.snapshotInstrumentID != "US.AAPL" {
		t.Fatalf("snapshot instrumentID = %q", provider.snapshotInstrumentID)
	}
	if !strings.Contains(snapshotRec.Body.String(), `"instrumentId":"US.AAPL"`) || !strings.Contains(snapshotRec.Body.String(), `"fromCache":false`) {
		t.Fatalf("snapshot body = %s", snapshotRec.Body.String())
	}

	postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId": "chart-main",
		"instruments": []any{
			map[string]any{"market": "US", "symbol": "AAPL"},
		},
	})
	heartbeatRec := httptest.NewRecorder()
	heartbeatReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/market-data/subscriptions/heartbeat", strings.NewReader(`{"consumerId":"chart-main"}`))
	heartbeatReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(heartbeatRec, heartbeatReq)
	if heartbeatRec.Code != http.StatusOK || !strings.Contains(heartbeatRec.Body.String(), `"totalActiveSubscriptions":1`) {
		t.Fatalf("heartbeat = %d %s", heartbeatRec.Code, heartbeatRec.Body.String())
	}

	searchRec := httptest.NewRecorder()
	router.ServeHTTP(searchRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/instruments?query=nvda", nil))
	if searchRec.Code != http.StatusOK || !strings.Contains(searchRec.Body.String(), `"query":"nvda"`) {
		t.Fatalf("search = %d %s", searchRec.Code, searchRec.Body.String())
	}

	normalizeRec := httptest.NewRecorder()
	normalizeReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/market-data/instruments/normalize", strings.NewReader(`{"market":"us","symbol":"aapl"}`))
	normalizeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(normalizeRec, normalizeReq)
	if normalizeRec.Code != http.StatusOK {
		t.Fatalf("normalize = %d %s", normalizeRec.Code, normalizeRec.Body.String())
	}
	if provider.normalizeRequest["market"] != "us" || provider.normalizeRequest["symbol"] != "aapl" {
		t.Fatalf("normalize request = %#v", provider.normalizeRequest)
	}
}

func TestReadRoutesMapProviderAndRequestFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{
		marketsErr:   errors.New("markets unavailable"),
		securityErr:  errors.New("security unavailable"),
		snapshotErr:  errors.New("snapshot unavailable"),
		normalizeErr: errors.New("instrument invalid"),
	}
	service := srv.NewService(provider)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	marketsRec := httptest.NewRecorder()
	router.ServeHTTP(marketsRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/markets", nil))
	if marketsRec.Code != http.StatusInternalServerError {
		t.Fatalf("markets status = %d body=%s", marketsRec.Code, marketsRec.Body.String())
	}

	securityRec := httptest.NewRecorder()
	router.ServeHTTP(securityRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/securities/us/aapl", nil))
	if securityRec.Code != http.StatusBadGateway {
		t.Fatalf("security status = %d body=%s", securityRec.Code, securityRec.Body.String())
	}

	snapshotRec := httptest.NewRecorder()
	router.ServeHTTP(snapshotRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/snapshots/us/aapl", nil))
	if snapshotRec.Code != http.StatusBadGateway {
		t.Fatalf("snapshot status = %d body=%s", snapshotRec.Code, snapshotRec.Body.String())
	}

	heartbeatRec := httptest.NewRecorder()
	heartbeatReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/market-data/subscriptions/heartbeat", strings.NewReader(`bad`))
	heartbeatReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(heartbeatRec, heartbeatReq)
	if heartbeatRec.Code != http.StatusBadRequest {
		t.Fatalf("heartbeat status = %d body=%s", heartbeatRec.Code, heartbeatRec.Body.String())
	}

	normalizeBadJSON := httptest.NewRecorder()
	normalizeBadJSONReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/market-data/instruments/normalize", strings.NewReader(`bad`))
	normalizeBadJSONReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(normalizeBadJSON, normalizeBadJSONReq)
	if normalizeBadJSON.Code != http.StatusBadRequest {
		t.Fatalf("normalize bad json status = %d body=%s", normalizeBadJSON.Code, normalizeBadJSON.Body.String())
	}

	normalizeRec := httptest.NewRecorder()
	normalizeReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/market-data/instruments/normalize", strings.NewReader(`{"market":"us","symbol":"bad"}`))
	normalizeReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(normalizeRec, normalizeReq)
	if normalizeRec.Code != http.StatusBadRequest {
		t.Fatalf("normalize provider err status = %d body=%s", normalizeRec.Code, normalizeRec.Body.String())
	}
}

func postSubscriptionJSON(t *testing.T, handler http.Handler, path string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d, body = %s", path, response.Code, response.Body.String())
	}
	return decodeRouteData(t, response)
}

func getSubscriptionJSON(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", path, response.Code, response.Body.String())
	}
	return decodeRouteData(t, response)
}

func deleteSubscriptionJSON(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("DELETE %s status = %d, body = %s", path, response.Code, response.Body.String())
	}
	return decodeRouteData(t, response)
}

func decodeRouteData(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("response ok=false: %s", response.Body.String())
	}
	return envelope.Data
}

func singleRouteEntry(t *testing.T, snapshot map[string]any) map[string]any {
	t.Helper()
	entries, ok := snapshot["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("entries = %#v", snapshot["entries"])
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("entry = %#v", entries[0])
	}
	return entry
}

func routeEntriesByKey(t *testing.T, snapshot map[string]any) map[string]map[string]any {
	t.Helper()
	entries, ok := snapshot["entries"].([]any)
	if !ok {
		t.Fatalf("entries = %#v", snapshot["entries"])
	}
	byKey := make(map[string]map[string]any, len(entries))
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("entry = %#v", raw)
		}
		key := jftradeCheckedTypeAssertion[string](entry["key"])
		byKey[key] = entry
	}
	return byKey
}

type routeTestProvider struct {
	candlesCalled        bool
	candlesMarket        string
	candlesSymbol        string
	candlesPeriod        string
	candlesLimit         int
	candlesFromTime      string
	candlesToTime        string
	candlesErr           error
	depthCalled          bool
	depthNum             int
	depthErr             error
	markets              []srv.MarketProfile
	marketsErr           error
	securityDetails      srv.SecurityDetails
	securityErr          error
	securityMarket       string
	securitySymbol       string
	snapshot             *srv.Tick
	snapshotErr          error
	snapshotInstrumentID string
	normalizedInstrument map[string]any
	normalizeErr         error
	normalizeRequest     map[string]any
}

func (p *routeTestProvider) GetMarkets(context.Context) ([]srv.MarketProfile, error) {
	return p.markets, p.marketsErr
}

func (p *routeTestProvider) GetSecurityDetails(_ context.Context, market, symbol string) (srv.SecurityDetails, error) {
	p.securityMarket = market
	p.securitySymbol = symbol
	return p.securityDetails, p.securityErr
}

func (p *routeTestProvider) QuerySnapshot(_ context.Context, instrumentID string) (*srv.Tick, error) {
	p.snapshotInstrumentID = instrumentID
	return p.snapshot, p.snapshotErr
}

func (*routeTestProvider) QueryTicker(context.Context, string) (*srv.Tick, error) {
	return nil, nil
}

func (p *routeTestProvider) GetHistoricalCandles(_ context.Context, market, symbol, period string, limit int, fromTime, toTime string) (srv.CandlesResponse, error) {
	p.candlesCalled = true
	p.candlesMarket = market
	p.candlesSymbol = symbol
	p.candlesPeriod = period
	p.candlesLimit = limit
	p.candlesFromTime = fromTime
	p.candlesToTime = toTime
	return srv.CandlesResponse{"candles": []any{}}, p.candlesErr
}

func (p *routeTestProvider) GetDepth(_ context.Context, _ string, _ string, num int) (srv.DepthResponse, error) {
	p.depthCalled = true
	p.depthNum = num
	return nil, p.depthErr
}

func (p *routeTestProvider) NormalizeInstrument(_ context.Context, input map[string]any) (map[string]any, error) {
	p.normalizeRequest = input
	return p.normalizedInstrument, p.normalizeErr
}

func (*routeTestProvider) Health(context.Context) (srv.HealthStatus, error) {
	return srv.HealthStatus{}, nil
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
