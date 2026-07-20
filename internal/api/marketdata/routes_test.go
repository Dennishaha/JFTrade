package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
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

func TestSubscriptionRoutesUseBrokerNeutralPollingWithoutFutuLease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(&routeTestProvider{})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	acquired := postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId":       "chart-alpha",
		"providerBrokerId": " Alpha ",
		"instruments": []any{
			map[string]any{"market": "US", "symbol": "AAPL", "channel": "KLINE", "interval": "1m"},
		},
	})
	if acquired["providerBrokerId"] != "alpha" || acquired["action"] != "acquired" {
		t.Fatalf("broker polling acquire = %#v", acquired)
	}
	if acquired["totalActiveSubscriptions"] != float64(0) {
		t.Fatalf("broker polling subscription shape = %#v", acquired)
	}
	quota := jftradeCheckedTypeAssertion[map[string]any](acquired["quota"])
	if quota["totalUsed"] != float64(0) {
		t.Fatalf("broker polling quota = %#v", quota)
	}
	transport := jftradeCheckedTypeAssertion[map[string]any](acquired["transport"])
	if transport["mode"] != "snapshot-poll-fallback" {
		t.Fatalf("broker polling transport = %#v", transport)
	}
	snapshot := getSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions")
	if snapshot["totalActiveSubscriptions"] != float64(0) {
		t.Fatalf("non-Futu polling consumed a Futu lease: %#v", snapshot)
	}

	heartbeat := postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions/heartbeat", map[string]any{
		"consumerId":       "chart-alpha",
		"providerBrokerId": "alpha",
	})
	if heartbeat["action"] != "heartbeat" {
		t.Fatalf("broker polling heartbeat = %#v", heartbeat)
	}
	released := postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions/release", map[string]any{
		"consumerId":       "chart-alpha",
		"providerBrokerId": "alpha",
		"instruments": []any{
			map[string]any{"market": "US", "symbol": "AAPL", "channel": "KLINE", "interval": "1m"},
		},
	})
	if released["action"] != "released" {
		t.Fatalf("broker polling release = %#v", released)
	}
}

func TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	legacy := &routeTestProvider{}
	reader := &routeBrokerReader{}
	router := gin.New()
	RegisterRoutes(
		router.Group("/api/v1"),
		srv.NewService(legacy),
		reader,
	)

	for _, path := range []string{
		"/api/v1/market-data/securities/us/aapl?brokerId=alpha",
		"/api/v1/market-data/snapshots/us/aapl?brokerId=alpha&refresh=true",
		"/api/v1/market-data/candles/us/aapl?brokerId=alpha&period=5m&limit=20",
		"/api/v1/market-data/depth/us/aapl?brokerId=alpha&num=12",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(
			response,
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil),
		)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
	if got, want := reader.calls, []string{
		"security:alpha:us:aapl",
		"snapshot:alpha:us:aapl:true",
		"candles:alpha:us:aapl:5m:20",
		"depth:alpha:us:aapl:12",
	}; !slices.Equal(got, want) {
		t.Fatalf("broker reader calls = %#v, want %#v", got, want)
	}
	if legacy.securityMarket != "" || legacy.snapshotInstrumentID != "" ||
		legacy.candlesCalled || legacy.depthCalled {
		t.Fatalf("explicit broker fell back to legacy provider: %#v", legacy)
	}

	withoutReader := gin.New()
	RegisterRoutes(withoutReader.Group("/api/v1"), srv.NewService(&routeTestProvider{}))
	for _, path := range []string{
		"/api/v1/market-data/securities/US/AAPL?brokerId=alpha",
		"/api/v1/market-data/snapshots/US/AAPL?brokerId=alpha",
		"/api/v1/market-data/candles/US/AAPL?brokerId=alpha",
		"/api/v1/market-data/depth/US/AAPL?brokerId=alpha",
	} {
		response := httptest.NewRecorder()
		withoutReader.ServeHTTP(
			response,
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil),
		)
		if response.Code != http.StatusConflict {
			t.Fatalf("GET %s status = %d, want 409; body = %s", path, response.Code, response.Body.String())
		}
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

func TestClearSubscriptionRoutePreservesRunningStrategyLease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(&routeTestProvider{})
	lease, err := service.AcquireManagedSubscription(t.Context(), "strategy-runtime:one", []srv.InstrumentRef{{
		Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "5m",
	}})
	if err != nil {
		t.Fatalf("AcquireManagedSubscription: %v", err)
	}
	defer lease.Release()
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId":  "chart-main",
		"instruments": []any{map[string]any{"market": "HK", "symbol": "00700"}},
	})
	cleared := deleteSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions")
	if cleared["totalActiveSubscriptions"] != float64(1) {
		t.Fatalf("web cleanup removed strategy lease: %#v", cleared)
	}
	entry := singleRouteEntry(t, cleared)
	consumers := jftradeCheckedTypeAssertion[[]any](entry["consumers"])
	if entry["key"] != "KLINE:US:AAPL:5m" || len(consumers) != 1 || consumers[0] != "strategy-runtime:one" {
		t.Fatalf("remaining managed entry = %#v", entry)
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

func TestCandlesRouteForwardsExclusiveBeforeAndRejectsInvalidCombinations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &routeBrokerReader{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(&routeTestProvider{}), reader)

	valid := httptest.NewRecorder()
	router.ServeHTTP(valid, httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/api/v1/market-data/candles/US/AAPL?brokerId=alpha&period=5m&limit=20&before=2026-07-18T13%3A40%3A00Z",
		nil,
	))
	if valid.Code != http.StatusOK || reader.lastBefore != "2026-07-18T13:40:00Z" {
		t.Fatalf("valid before status=%d before=%q body=%s", valid.Code, reader.lastBefore, valid.Body.String())
	}

	for _, path := range []string{
		"/api/v1/market-data/candles/US/AAPL?brokerId=alpha&period=5m&before=bad",
		"/api/v1/market-data/candles/US/AAPL?brokerId=alpha&period=5m&before=2026-07-18T13%3A40%3A00Z&from=2026-07-01",
		"/api/v1/market-data/candles/US/AAPL?brokerId=alpha&period=tick&before=2026-07-18T13%3A40%3A00Z",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Errorf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
		}
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

func TestCandlesRouteTickAndLegacyBeforePagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(provider))

	tick := httptest.NewRecorder()
	router.ServeHTTP(tick, httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/api/v1/market-data/candles/US/AAPL?period=tick",
		nil,
	))
	if tick.Code != http.StatusOK || !strings.Contains(tick.Body.String(), `"pagination":{"hasMore":false}`) {
		t.Fatalf("tick response status=%d body=%s", tick.Code, tick.Body.String())
	}

	before := httptest.NewRecorder()
	router.ServeHTTP(before, httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/api/v1/market-data/candles/US/AAPL?period=5m&limit=2&before=2026-07-18T13%3A40%3A00Z",
		nil,
	))
	if before.Code != http.StatusOK {
		t.Fatalf("legacy before response status=%d body=%s", before.Code, before.Body.String())
	}
	if provider.candlesToTime != "2026-07-18T13:39:59.999999999Z" {
		t.Fatalf("exclusive legacy toTime = %q", provider.candlesToTime)
	}

	pagination := defaultCandlePagination(map[string]any{
		"candles": []map[string]any{{"at": "2026-07-18T13:35:00Z"}},
	}, 1)
	if pagination["hasMore"] != true || pagination["nextBefore"] != "2026-07-18T13:35:00Z" {
		t.Fatalf("default pagination = %#v", pagination)
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
		descriptor: srv.ProviderDescriptor{
			ProviderID:       "futu-opend",
			DisplayName:      "Futu OpenD",
			Source:           "bbgo:futu",
			SupportedMarkets: []string{"US", "HK"},
			Capabilities: srv.ProviderCapabilities{
				Snapshots:         true,
				HistoricalCandles: true,
				OrderBookDepth:    true,
			},
		},
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

	providerRec := httptest.NewRecorder()
	router.ServeHTTP(providerRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/provider", nil))
	if providerRec.Code != http.StatusOK ||
		!strings.Contains(providerRec.Body.String(), `"providerId":"futu-opend"`) ||
		!strings.Contains(providerRec.Body.String(), `"orderBookDepth":true`) {
		t.Fatalf("provider = %d %s", providerRec.Code, providerRec.Body.String())
	}

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
	router.ServeHTTP(searchRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/instruments?market=US&query=nvda", nil))
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
		descriptorErr: errors.New("provider unavailable"),
		marketsErr:    errors.New("markets unavailable"),
		securityErr:   errors.New("security unavailable"),
		snapshotErr:   errors.New("snapshot unavailable"),
		normalizeErr:  errors.New("instrument invalid"),
	}
	service := srv.NewService(provider)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	providerRec := httptest.NewRecorder()
	router.ServeHTTP(providerRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/provider", nil))
	if providerRec.Code != http.StatusBadGateway {
		t.Fatalf("provider status = %d body=%s", providerRec.Code, providerRec.Body.String())
	}

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

func TestInstrumentSearchRouteReturnsSubsetResolutionContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	provider.searchInstruments = func(_ context.Context, query string, limit int) ([]srv.InstrumentCandidate, error) {
		if limit != 100 {
			t.Fatalf("SearchInstruments(%q, %d), want limit 100", query, limit)
		}
		entries := []srv.InstrumentCandidate{
			{Market: "US", InstrumentID: "US.000001", Code: "000001", Name: "US Product", SecurityType: "Warrant"},
			{Market: "SH", InstrumentID: "SH.000001", Code: "000001", Name: "SSE Composite", SecurityType: "Index"},
			{Market: "SZ", InstrumentID: "SZ.000001", Code: "000001", Name: "Ping An Bank", SecurityType: "Eqty"},
			{Market: "JP", InstrumentID: "JP.000001", Code: "000001", Name: "JP Product", SecurityType: "Plate"},
		}
		if query == "all-types" {
			for index := range entries {
				entries[index].Code = fmt.Sprintf("TYPE%d", index)
				entries[index].InstrumentID = entries[index].Market + "." + entries[index].Code
			}
			return entries, nil
		}
		if query != "000001" {
			t.Fatalf("unexpected query %q", query)
		}
		return entries, nil
	}
	provider.lookupInstrument = func(_ context.Context, market, code string) ([]srv.InstrumentCandidate, error) {
		return []srv.InstrumentCandidate{{
			Market:       market,
			InstrumentID: market + "." + code,
			Code:         code,
			Name:         market + " Listing",
			SecurityType: "Eqty",
			LotSize:      100,
			Source:       "test",
		}}, nil
	}
	service := srv.NewService(provider)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/instruments?market=CN&query=000001&limit=20", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET instruments status = %d body=%s", response.Code, response.Body.String())
	}
	data := decodeRouteData(t, response)
	if data["query"] != "000001" || data["requestedMarket"] != "CN" || data["resolutionStatus"] != "ambiguous" || data["totalReturned"] != float64(2) {
		t.Fatalf("resolution metadata = %#v", data)
	}
	entries := jftradeCheckedTypeAssertion[[]any](data["entries"])
	first := jftradeCheckedTypeAssertion[map[string]any](entries[0])
	second := jftradeCheckedTypeAssertion[map[string]any](entries[1])
	if first["instrumentId"] != "SH.000001" || first["resolvedMarket"] != "CN" || first["symbol"] != "000001" ||
		first["securityType"] != "Index" || first["selectable"] != true || second["instrumentId"] != "SZ.000001" {
		t.Fatalf("stable candidate entries = %#v", entries)
	}
	provider.searchMu.Lock()
	searchCalls := append([]string(nil), provider.searchCalls...)
	provider.searchMu.Unlock()
	if !slices.Equal(searchCalls, []string{"000001:100"}) {
		t.Fatalf("search calls = %#v", searchCalls)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/instruments?query=all-types", nil)
	router.ServeHTTP(response, request)
	allMarketData := decodeRouteData(t, response)
	allMarketEntries := jftradeCheckedTypeAssertion[[]any](allMarketData["entries"])
	if len(allMarketEntries) != 4 {
		t.Fatalf("all-market entries = %#v", allMarketEntries)
	}
	wantTypes := []string{"Warrant", "Index", "Eqty", "Plate"}
	for index, rawEntry := range allMarketEntries {
		entry := jftradeCheckedTypeAssertion[map[string]any](rawEntry)
		if entry["securityType"] != wantTypes[index] {
			t.Fatalf("security type[%d] = %#v, want %s", index, entry, wantTypes[index])
		}
	}
	last := jftradeCheckedTypeAssertion[map[string]any](allMarketEntries[3])
	if last["market"] != "JP" || last["selectable"] != false || last["unavailableReason"] == "" {
		t.Fatalf("unsupported all-market candidate = %#v", last)
	}

	// A qualified query bypasses parent expansion and only looks up its leaf.
	response = httptest.NewRecorder()
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/instruments?market=CN&query=SH.600519", nil)
	router.ServeHTTP(response, request)
	qualified := decodeRouteData(t, response)
	if qualified["resolutionStatus"] != "resolved" || qualified["totalReturned"] != float64(1) {
		t.Fatalf("qualified resolution = %#v", qualified)
	}
	provider.lookupMu.Lock()
	lastCall := provider.lookupCalls[len(provider.lookupCalls)-1]
	provider.lookupMu.Unlock()
	if lastCall != "SH.600519" {
		t.Fatalf("qualified lookup call = %q", lastCall)
	}
}

func TestInstrumentSearchRouteValidatesInputAndMapsProviderFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	provider.searchInstruments = func(_ context.Context, query string, _ int) ([]srv.InstrumentCandidate, error) {
		switch query {
		case "missing":
			return []srv.InstrumentCandidate{}, nil
		case "Toyota":
			return []srv.InstrumentCandidate{{Market: "JP", InstrumentID: "JP.7203", Code: "7203", Name: "Toyota", SecurityType: "Eqty"}}, nil
		case "Apple":
			return []srv.InstrumentCandidate{{Market: "US", InstrumentID: "US.AAPL", Code: "AAPL", Name: "Apple Inc.", SecurityType: "Eqty"}}, nil
		default:
			return nil, errors.New("OpenD search failed")
		}
	}
	service := srv.NewService(provider)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	for query, wantStatus := range map[string]string{
		"Apple":   "resolved",
		"Toyota":  "unavailable",
		"missing": "not_found",
	} {
		response := httptest.NewRecorder()
		path := "/api/v1/market-data/instruments?query=" + query
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status for %s = %d body=%s", query, response.Code, response.Body.String())
		}
		data := decodeRouteData(t, response)
		if data["resolutionStatus"] != wantStatus {
			t.Fatalf("resolution status for %s = %#v, want %s", query, data, wantStatus)
		}
		if query == "Toyota" {
			entry := jftradeCheckedTypeAssertion[map[string]any](jftradeCheckedTypeAssertion[[]any](data["entries"])[0])
			if entry["selectable"] != false || entry["unavailableReason"] == "" {
				t.Fatalf("unavailable candidate = %#v", entry)
			}
		}
	}

	for _, path := range []string{
		"/api/v1/market-data/instruments",
		"/api/v1/market-data/instruments?query=Apple&limit=0",
		"/api/v1/market-data/instruments?query=Apple&limit=101",
		"/api/v1/market-data/instruments?query=Apple&limit=bad",
		"/api/v1/market-data/instruments?market=JP&query=Toyota",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid request %s status = %d body=%s", path, response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/instruments?query=provider-error", nil))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "MARKET_INSTRUMENT_SEARCH_FAILED") {
		t.Fatalf("provider error status = %d body=%s", response.Code, response.Body.String())
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
	descriptor           srv.ProviderDescriptor
	descriptorErr        error
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
	lookupMu             sync.Mutex
	lookupCalls          []string
	lookupInstrument     func(context.Context, string, string) ([]srv.InstrumentCandidate, error)
	searchMu             sync.Mutex
	searchCalls          []string
	searchInstruments    func(context.Context, string, int) ([]srv.InstrumentCandidate, error)
}

type routeBrokerReader struct {
	calls      []string
	lastBefore string
}

func (r *routeBrokerReader) ReadMarketSnapshot(
	_ context.Context,
	brokerID string,
	market string,
	symbol string,
	refresh bool,
) (map[string]any, error) {
	r.calls = append(r.calls, fmt.Sprintf("snapshot:%s:%s:%s:%t", brokerID, market, symbol, refresh))
	return map[string]any{"meta": map[string]any{"brokerId": brokerID}}, nil
}

func (r *routeBrokerReader) ReadMarketSecurityDetails(
	_ context.Context,
	brokerID string,
	market string,
	symbol string,
) (map[string]any, error) {
	r.calls = append(r.calls, fmt.Sprintf("security:%s:%s:%s", brokerID, market, symbol))
	return map[string]any{"meta": map[string]any{"brokerId": brokerID}}, nil
}

func (r *routeBrokerReader) ReadMarketCandles(
	_ context.Context,
	brokerID string,
	market string,
	symbol string,
	period string,
	limit int,
	_ string,
	_ string,
	before string,
) (map[string]any, error) {
	r.lastBefore = before
	r.calls = append(r.calls, fmt.Sprintf("candles:%s:%s:%s:%s:%d", brokerID, market, symbol, period, limit))
	return map[string]any{"meta": map[string]any{"brokerId": brokerID}}, nil
}

func (r *routeBrokerReader) ReadMarketDepth(
	_ context.Context,
	brokerID string,
	market string,
	symbol string,
	num int,
) (map[string]any, error) {
	r.calls = append(r.calls, fmt.Sprintf("depth:%s:%s:%s:%d", brokerID, market, symbol, num))
	return map[string]any{"meta": map[string]any{"brokerId": brokerID}}, nil
}

func (p *routeTestProvider) Descriptor(context.Context) (srv.ProviderDescriptor, error) {
	if p.descriptor.ProviderID == "" {
		p.descriptor.ProviderID = "route-test"
		p.descriptor.DisplayName = "Route Test"
		p.descriptor.Source = "test"
	}
	return p.descriptor, p.descriptorErr
}

func (p *routeTestProvider) GetMarkets(context.Context) ([]srv.MarketProfile, error) {
	return p.markets, p.marketsErr
}

func (p *routeTestProvider) GetSecurityDetails(_ context.Context, market, symbol string) (srv.SecurityDetails, error) {
	p.securityMarket = market
	p.securitySymbol = symbol
	return p.securityDetails, p.securityErr
}

func (p *routeTestProvider) LookupInstrument(ctx context.Context, market, code string) ([]srv.InstrumentCandidate, error) {
	p.lookupMu.Lock()
	p.lookupCalls = append(p.lookupCalls, market+"."+code)
	p.lookupMu.Unlock()
	if p.lookupInstrument == nil {
		return nil, nil
	}
	return p.lookupInstrument(ctx, market, code)
}

func (p *routeTestProvider) SearchInstruments(ctx context.Context, query string, limit int) ([]srv.InstrumentCandidate, error) {
	p.searchMu.Lock()
	p.searchCalls = append(p.searchCalls, fmt.Sprintf("%s:%d", query, limit))
	p.searchMu.Unlock()
	if p.searchInstruments == nil {
		return nil, nil
	}
	return p.searchInstruments(ctx, query, limit)
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
