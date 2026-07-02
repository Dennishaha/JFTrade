package marketdata

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/marketdata"
)

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
		{name: "release malformed JSON", path: "/api/v1/market-data/subscriptions/release", body: `{`, detail: "invalid release request"},
		{name: "release missing consumer", path: "/api/v1/market-data/subscriptions/release", body: `{}`, detail: "consumerId is required"},
		{name: "release incomplete target", path: "/api/v1/market-data/subscriptions/release", body: `{"consumerId":"chart","instruments":[{"market":"US"}]}`, detail: "release target market and symbol are required"},
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

func TestBestEffortLoggingAcceptsErrorsAndNonErrors(t *testing.T) {
	jftradeLogError("ignored", nil, errors.New("expected test error"))
}
