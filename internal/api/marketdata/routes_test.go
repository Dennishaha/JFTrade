package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

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
	candlesCalled   bool
	candlesMarket   string
	candlesSymbol   string
	candlesPeriod   string
	candlesLimit    int
	candlesFromTime string
	candlesToTime   string
	depthCalled     bool
	depthNum        int
}

func (*routeTestProvider) GetMarkets(context.Context) ([]srv.MarketProfile, error) {
	return nil, nil
}

func (*routeTestProvider) GetSecurityDetails(context.Context, string, string) (srv.SecurityDetails, error) {
	return nil, nil
}

func (*routeTestProvider) QuerySnapshot(context.Context, string) (*srv.Tick, error) {
	return nil, nil
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
	return srv.CandlesResponse{"candles": []any{}}, nil
}

func (p *routeTestProvider) GetDepth(_ context.Context, _ string, _ string, num int) (srv.DepthResponse, error) {
	p.depthCalled = true
	p.depthNum = num
	return nil, nil
}

func (*routeTestProvider) NormalizeInstrument(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
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
