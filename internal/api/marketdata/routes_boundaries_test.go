package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	srv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
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
		{name: "candles", path: "/api/v1/market-data/candles/HK/00700", code: "MARKET_CANDLES_FAILED"},
		{name: "depth with explicit level count", path: "/api/v1/market-data/depth/HK/00700?num=25", code: "MARKET_DEPTH_FAILED"},
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

func TestMarketsRouteFailsWhenActiveProviderDescriptorIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{
		markets:       []srv.MarketProfile{{"market": "US"}},
		descriptorErr: errors.New("active provider is unavailable"),
	}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(provider))

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/markets", nil),
	)

	if response.Code != http.StatusInternalServerError ||
		!strings.Contains(response.Body.String(), `"code":"MARKET_DATA_FAILED"`) ||
		!strings.Contains(response.Body.String(), "active provider is unavailable") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestProviderFailureCodesPreserveFutuCompatibilityOnlyForFutu(t *testing.T) {
	const (
		futuCode    = "OPEND_CANDLES_FAILED"
		genericCode = "MARKET_CANDLES_FAILED"
	)
	tests := []struct {
		name             string
		explicitBrokerID string
		descriptor       srv.ProviderDescriptor
		descriptorErr    error
		want             string
	}{
		{name: "explicit Futu", explicitBrokerID: " FuTu ", want: futuCode},
		{name: "explicit non-Futu", explicitBrokerID: "yfinance", want: genericCode},
		{
			name:       "active Futu",
			descriptor: srv.ProviderDescriptor{ProviderID: "futu-opend", BrokerID: "FuTu"},
			want:       futuCode,
		},
		{
			name:       "active non-Futu",
			descriptor: srv.ProviderDescriptor{ProviderID: "yfinance", BrokerID: "yfinance"},
			want:       genericCode,
		},
		{
			name:          "unavailable active provider metadata",
			descriptorErr: errors.New("descriptor unavailable"),
			want:          genericCode,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := srv.NewService(&routeTestProvider{
				descriptor:    test.descriptor,
				descriptorErr: test.descriptorErr,
			})

			got := providerFailureCode(
				t.Context(), service, test.explicitBrokerID, futuCode, genericCode,
			)

			if got != test.want {
				t.Fatalf("provider failure code = %q, want %q", got, test.want)
			}
		})
	}
}

func TestActiveNonBrokerProviderMatchingHandlesMissingAndFutuDescriptors(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		service    *srv.Service
		want       bool
	}{
		{name: "empty provider id", service: srv.NewService(&routeTestProvider{})},
		{name: "missing service", providerID: "yfinance"},
		{
			name:       "descriptor unavailable",
			providerID: "yfinance",
			service: srv.NewService(&routeTestProvider{
				descriptorErr: errors.New("descriptor unavailable"),
			}),
		},
		{
			name:       "Futu remains broker-routed",
			providerID: "futu-opend",
			service: srv.NewService(&routeTestProvider{
				descriptor: srv.ProviderDescriptor{ProviderID: "futu-opend", BrokerID: "futu"},
			}),
		},
		{
			name:       "Yahoo provider alias",
			providerID: "YFINANCE",
			service: srv.NewService(&routeTestProvider{
				descriptor: srv.ProviderDescriptor{ProviderID: "yfinance", BrokerID: "yfinance"},
			}),
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usesActiveNonBrokerProvider(t.Context(), test.service, test.providerID); got != test.want {
				t.Fatalf("usesActiveNonBrokerProvider() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSnapshotRejectsMalformedRefreshQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(provider))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet,
		"/api/v1/market-data/snapshots/US/AAPL?refresh=not-a-boolean", nil,
	))

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), `"code":"BAD_REQUEST"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if provider.snapshotInstrumentID != "" {
		t.Fatalf("snapshot provider was called for malformed refresh: %q", provider.snapshotInstrumentID)
	}
}

func TestNormalizeOptionalQueryTimeAcceptsEmptyAndRejectsMalformedValues(t *testing.T) {
	value, err := normalizeOptionalQueryTime("  ")
	if err != nil || value != "" {
		t.Fatalf("empty time = %q, %v; want empty value", value, err)
	}
	if _, err := normalizeOptionalQueryTime("not-a-time"); err == nil {
		t.Fatal("normalizeOptionalQueryTime() accepted malformed input")
	}
}

func TestExplicitYFinanceReadsUseTheActiveMarketDataProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{
		descriptor: srv.ProviderDescriptor{
			ProviderID: "yfinance", BrokerID: "yfinance", Source: "yfinance",
			Capabilities: srv.ProviderCapabilities{
				Snapshots: true, HistoricalCandles: true,
			},
		},
		securityDetails: srv.SecurityDetails{"symbol": "AAPL"},
	}
	reader := &routeBrokerReader{}
	service := srv.NewService(provider)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service, reader)

	for _, path := range []string{
		"/api/v1/market-data/securities/US/AAPL?brokerId=yfinance",
		"/api/v1/market-data/candles/US/AAPL?brokerId=yfinance&period=1d",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	if len(reader.calls) != 0 {
		t.Fatalf("explicit yfinance reads were sent to broker reader: %#v", reader.calls)
	}
	if !provider.candlesCalled || provider.candlesMarket != "US" || provider.candlesSymbol != "AAPL" {
		t.Fatalf("yfinance candles were not read from active provider: called=%v market=%q symbol=%q", provider.candlesCalled, provider.candlesMarket, provider.candlesSymbol)
	}
}

func TestMarketDataReadErrorsExposeProviderSwitchRetrySignal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)

	writeMarketDataReadError(context, "MARKET_DATA_FAILED", srv.ErrProviderChanged)

	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), `"code":"MARKET_DATA_PROVIDER_CHANGED"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestMarketDataReadErrorsExposeProviderWarmupRetrySignal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)

	writeMarketDataReadError(context, "MARKET_DATA_FAILED", srv.ErrProviderWarming)

	if response.Code != http.StatusServiceUnavailable ||
		response.Header().Get("Retry-After") != "1" ||
		!strings.Contains(response.Body.String(), `"code":"MARKET_DATA_PROVIDER_WARMING"`) {
		t.Fatalf("response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestMarketDataReadErrorsExposeProviderBusyRetrySignal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)

	writeMarketDataReadError(context, "MARKET_DATA_FAILED", srv.ErrProviderBusy)

	if response.Code != http.StatusServiceUnavailable ||
		response.Header().Get("Retry-After") != "2" ||
		!strings.Contains(response.Body.String(), `"code":"MARKET_DATA_PROVIDER_BUSY"`) {
		t.Fatalf("response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestMarketDataReadErrorsRejectInvalidCandleSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)

	writeMarketDataReadError(context, "MARKET_DATA_FAILED", srv.ErrInvalidCandleSessions)

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), `"code":"MARKET_CANDLE_SESSIONS_INVALID"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestBrokerMarketDataReadErrorsPreserveClientActionability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
		retryAfter string
	}{
		{
			name:       "rate limit with retry delay",
			err:        broker.NewSnapshotRateLimitError(1500*time.Millisecond, errors.New("snapshot quota exhausted")),
			statusCode: http.StatusTooManyRequests,
			code:       "MARKET_SNAPSHOT_RATE_LIMITED",
			retryAfter: "2",
		},
		{
			name:       "rate limit without retry metadata",
			err:        broker.ErrSnapshotRateLimited,
			statusCode: http.StatusTooManyRequests,
			code:       "MARKET_SNAPSHOT_RATE_LIMITED",
			retryAfter: "1",
		},
		{
			name:       "invalid feature query",
			err:        productfeatures.ErrInvalidQuery,
			statusCode: http.StatusBadRequest,
			code:       "MARKET_DATA_QUERY_INVALID",
		},
		{
			name:       "unavailable broker capability",
			err:        productfeatures.ErrCapabilityUnavailable,
			statusCode: http.StatusConflict,
			code:       "BROKER_CAPABILITY_UNAVAILABLE",
		},
		{
			name:       "invalid candle sessions",
			err:        broker.ErrInvalidCandleSessions,
			statusCode: http.StatusBadRequest,
			code:       "MARKET_CANDLE_SESSIONS_INVALID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)

			writeBrokerMarketDataReadError(context, "MARKET_DATA_FAILED", test.err)

			if response.Code != test.statusCode || !strings.Contains(response.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Retry-After"); got != test.retryAfter {
				t.Fatalf("Retry-After = %q, want %q", got, test.retryAfter)
			}
		})
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
		"/api/v1/market-data/depth/US/AAPL?num=10",
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

func TestPollOnlyReadRoutesPrioritizeCapabilitiesAndPreserveLogicalLeases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeTestProvider{
		descriptor: srv.ProviderDescriptor{
			ProviderID: "poll-only", Source: "poll-only",
			Capabilities: srv.ProviderCapabilities{Snapshots: true},
		},
		snapshot: &srv.Tick{
			InstrumentID: "US.AAPL", Market: "US", Symbol: "AAPL",
			Price: decimal.NewFromInt(1), Source: "poll-only",
			ObservedAt: "2026-07-30T00:00:00Z",
		},
	}
	service := srv.NewService(provider)
	service.SetSubscriptionReconciler(&cancellingSubscriptionReconciler{})
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/v1/market-data/snapshots/US/AAPL", nil,
	))
	if snapshot.Code != http.StatusConflict ||
		!strings.Contains(snapshot.Body.String(), `"code":"MARKET_DATA_SUBSCRIPTION_REQUIRED"`) {
		t.Fatalf("poll-only snapshot = %d %s", snapshot.Code, snapshot.Body.String())
	}
	for _, path := range []string{
		"/api/v1/market-data/candles/US/AAPL?period=tick",
		"/api/v1/market-data/depth/US/AAPL?num=10",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(
			t.Context(), http.MethodGet, path, nil,
		))
		if response.Code != http.StatusConflict ||
			!strings.Contains(response.Body.String(), `"code":"MARKET_DATA_CAPABILITY_UNSUPPORTED"`) {
			t.Fatalf("poll-only unsupported %s = %d %s", path, response.Code, response.Body.String())
		}
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
		{name: "acquire rejects order book interval", path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL","channel":"ORDER_BOOK","interval":"1m"}]}`, detail: "subscription interval is only valid for KLINE"},
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
