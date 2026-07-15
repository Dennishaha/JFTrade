package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	srv "github.com/jftrade/jftrade-main/internal/marketdata"
)

type cancellingSubscriptionReconciler struct {
	cancel context.CancelFunc
}

func (r *cancellingSubscriptionReconciler) ReconcileSubscriptions(context.Context, []srv.InstrumentRef) error {
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}

func (*cancellingSubscriptionReconciler) SubscriptionState() map[string]any {
	return nil
}

func TestInstrumentHandlersRejectMissingURIParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(&routeTestProvider{})
	tests := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "security details", handler: handleSecurityDetails(service)},
		{name: "snapshot", handler: handleSnapshot(service)},
		{name: "candles", handler: handleCandles(service)},
		{name: "depth", handler: handleDepth(service)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)

			test.handler(context)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"BAD_REQUEST"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCandlesAndDepthRoutesMapProviderFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{
		candlesErr: errors.New("candle feed unavailable"),
		depthErr:   errors.New("order book unavailable"),
	}
	service := srv.NewService(provider)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "candles", path: "/api/v1/market-data/candles/HK/00700", code: "OPEND_CANDLES_FAILED"},
		{name: "depth with explicit level count", path: "/api/v1/market-data/depth/HK/00700?num=25", code: "OPEND_DEPTH_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, test.path, nil)
			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
	if provider.depthNum != 25 {
		t.Fatalf("depth num = %d, want 25", provider.depthNum)
	}
}

func TestLiveReadRoutesReturnConflictForMissingSubscriptionLease(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{
		snapshot:   &srv.Tick{InstrumentID: "US.AAPL", Market: "US", Symbol: "AAPL", Price: decimal.NewFromInt(1), ObservedAt: "2026-07-16T00:00:00Z"},
		candlesErr: srv.NewSubscriptionRequiredError("KLINE", "US", "AAPL", "1m"),
	}
	service := srv.NewService(provider)
	service.SetSubscriptionReconciler(&cancellingSubscriptionReconciler{})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	for _, path := range []string{
		"/api/v1/market-data/snapshots/US/AAPL",
		"/api/v1/market-data/candles/US/AAPL?period=1m",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"MARKET_DATA_SUBSCRIPTION_REQUIRED"`) {
			t.Fatalf("missing lease %s response = %d %s", path, response.Code, response.Body.String())
		}
	}

	postSubscriptionJSON(t, router, "/api/v1/market-data/subscriptions", map[string]any{
		"consumerId":  "chart",
		"instruments": []any{map[string]any{"channel": "SNAPSHOT", "market": "US", "symbol": "AAPL"}},
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/snapshots/US/AAPL", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("leased snapshot response = %d %s", response.Code, response.Body.String())
	}
}

func TestSubscriptionRoutesRejectMalformedAndIncompleteRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(&routeTestProvider{})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	tests := []struct {
		name   string
		path   string
		body   string
		detail string
	}{
		{name: "acquire malformed JSON", path: "/api/v1/market-data/subscriptions", body: `{`, detail: "invalid subscription request"},
		{name: "acquire missing consumer", path: "/api/v1/market-data/subscriptions", body: `{"instruments":[{"market":"US","symbol":"AAPL"}]}`, detail: "consumerId and instruments are required"},
		{name: "acquire missing instruments", path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart"}`, detail: "consumerId and instruments are required"},
		{name: "acquire drops incomplete instruments", path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart","instruments":[{"market":"US"},{"symbol":"AAPL"}]}`, detail: "consumerId and instruments are required"},
		{name: "acquire rejects invalid channel", path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"NEWS"}]}`, detail: "unsupported subscription channel"},
		{name: "acquire rejects unmanaged order book", path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"ORDER_BOOK"}]}`, detail: "unsupported subscription channel"},
		{name: "acquire rejects KLINE without interval", path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"KLINE"}]}`, detail: "unsupported KLINE subscription interval"},
		{name: "release malformed JSON", path: "/api/v1/market-data/subscriptions/release", body: `{`, detail: "invalid release request"},
		{name: "release missing consumer", path: "/api/v1/market-data/subscriptions/release", body: `{}`, detail: "consumerId is required"},
		{name: "release incomplete target", path: "/api/v1/market-data/subscriptions/release", body: `{"consumerId":"chart","instruments":[{"market":"US"}]}`, detail: "release target market and symbol are required"},
		{name: "release rejects invalid interval", path: "/api/v1/market-data/subscriptions/release", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"KLINE","interval":"2m"}]}`, detail: "unsupported KLINE subscription interval"},
		{name: "heartbeat requires consumer", path: "/api/v1/market-data/subscriptions/heartbeat", body: `{}`, detail: "consumerId is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.detail) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSubscriptionRequestHelpersPreserveOnlyValidTargets(t *testing.T) {
	valid := srv.InstrumentRef{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m"}
	instruments := subscriptionInstruments(subscriptionRequest{Instruments: []srv.InstrumentRef{
		{Market: " ", Symbol: "AAPL"},
		{Market: "US", Symbol: " "},
		valid,
	}})
	if len(instruments) != 1 || instruments[0] != valid {
		t.Fatalf("filtered instruments = %#v", instruments)
	}
	if instruments := subscriptionInstruments(subscriptionRequest{Instruments: []srv.InstrumentRef{{Market: "US"}}}); instruments != nil {
		t.Fatalf("all-invalid instruments = %#v, want nil", instruments)
	}

	if target, hasTarget, validTarget := subscriptionReleaseTarget(subscriptionRequest{
		Instruments: []srv.InstrumentRef{{Market: "US"}},
	}); target != (srv.InstrumentRef{}) || hasTarget || validTarget {
		t.Fatalf("invalid release target = %#v, %t, %t", target, hasTarget, validTarget)
	}
}

func TestSubscriptionHandlersMapCanceledServiceOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "get", method: http.MethodGet, path: "/api/v1/market-data/subscriptions"},
		{name: "acquire", method: http.MethodPost, path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}`},
		{name: "release", method: http.MethodPost, path: "/api/v1/market-data/subscriptions/release", body: `{"consumerId":"chart"}`},
		{name: "clear", method: http.MethodDelete, path: "/api/v1/market-data/subscriptions"},
		{name: "heartbeat", method: http.MethodPost, path: "/api/v1/market-data/subscriptions/heartbeat", body: `{"consumerId":"chart"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := srv.NewService(&routeTestProvider{})
			router := gin.New()
			RegisterRoutes(router.Group("/api/v1"), service)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			request := httptest.NewRequestWithContext(ctx, test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"SUBSCRIPTION_FAILED"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestReleaseAndClearMapSnapshotCancellationAfterLogicalCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "release", method: http.MethodPost, path: "/api/v1/market-data/subscriptions/release", body: `{"consumerId":"chart"}`},
		{name: "clear", method: http.MethodDelete, path: "/api/v1/market-data/subscriptions"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := srv.NewService(&routeTestProvider{})
			ctx, cancel := context.WithCancel(t.Context())
			reconciler := &cancellingSubscriptionReconciler{}
			service.SetSubscriptionReconciler(reconciler)
			reconciler.cancel = cancel
			router := gin.New()
			RegisterRoutes(router.Group("/api/v1"), service)
			request := httptest.NewRequestWithContext(ctx, test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"SUBSCRIPTION_FAILED"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestBestEffortLoggingAcceptsErrorsAndNonErrors(t *testing.T) {
	jftradeLogError("ignored", nil, errors.New("expected test error"))
}
