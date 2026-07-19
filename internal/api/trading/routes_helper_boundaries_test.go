package trading

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestTradingRouteHelpersWriteHTTPBoundaryErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("bind uri rejects missing route parameters", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/brokers//funds", nil)

		if brokerID, ok := bindBrokerURI(ctx); ok || brokerID != "" || recorder.Code != http.StatusNotFound {
			t.Fatalf("bindBrokerURI brokerID=%q ok=%v status=%d body=%s", brokerID, ok, recorder.Code, recorder.Body.String())
		}
	})

	t.Run("route handlers stop after an invalid route URI", func(t *testing.T) {
		for _, handler := range []gin.HandlerFunc{handleRead(srv.NewService(), "funds"), handlePlaceOrder(srv.NewService())} {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/brokers//funds", nil)
			handler(ctx)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		}
	})

	t.Run("bind query rejects type conversion errors", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/brokers/futu/funds?Page=bad", nil)
		var query struct {
			Page int
		}

		if bindQuery(ctx, &query, "invalid typed query") || recorder.Code != http.StatusBadRequest {
			t.Fatalf("bindQuery status=%d body=%s query=%+v", recorder.Code, recorder.Body.String(), query)
		}
		if !strings.Contains(recorder.Body.String(), "invalid typed query") {
			t.Fatalf("bindQuery body=%s", recorder.Body.String())
		}
	})

	t.Run("read result maps backend errors to broker read failure", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/brokers/futu/funds", nil)

		writeReadResult(ctx, nil, errors.New("backend unavailable"))
		if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "BROKER_READ_FAILED") {
			t.Fatalf("writeReadResult status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("read result preserves broker and snapshot retry semantics", func(t *testing.T) {
		tests := []struct {
			name       string
			err        error
			statusCode int
			retryAfter string
		}{
			{name: "unknown broker", err: srv.ErrBrokerNotFound, statusCode: http.StatusNotFound},
			{
				name:       "snapshot retry metadata",
				err:        broker.NewSnapshotRateLimitError(1500*time.Millisecond, errors.New("quota exhausted")),
				statusCode: http.StatusTooManyRequests,
				retryAfter: "2",
			},
			{
				name:       "snapshot retry default",
				err:        broker.ErrSnapshotRateLimited,
				statusCode: http.StatusTooManyRequests,
				retryAfter: "1",
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/brokers/futu/quote", nil)

				writeReadResult(ctx, nil, test.err)

				if recorder.Code != test.statusCode || recorder.Header().Get("Retry-After") != test.retryAfter {
					t.Fatalf("status=%d retry=%q body=%s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
				}
			})
		}
	})
}

func TestPortfolioReadUnknownResourceIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/portfolio/:brokerId/unknown", handlePortfolioRead(srv.NewService(), "unknown"))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/futu/unknown", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown portfolio resource status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestTradingRoutesRejectMalformedQueryEncodingBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := srv.NewService()
	RegisterRoutes(router.Group("/api/v1"), service)
	RegisterPortfolioRoutes(router.Group("/api/v1"), service)

	for _, endpoint := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "portfolio", method: http.MethodGet, path: "/api/v1/portfolio/futu/cash-balances"},
		{name: "funds", method: http.MethodGet, path: "/api/v1/brokers/futu/funds"},
		{name: "positions", method: http.MethodGet, path: "/api/v1/brokers/futu/positions"},
		{name: "orders", method: http.MethodGet, path: "/api/v1/brokers/futu/orders"},
		{name: "fills", method: http.MethodGet, path: "/api/v1/brokers/futu/fills"},
		{name: "cash flows", method: http.MethodGet, path: "/api/v1/brokers/futu/cash-flows"},
		{name: "order fees", method: http.MethodGet, path: "/api/v1/brokers/futu/order-fees"},
		{name: "margin ratios", method: http.MethodGet, path: "/api/v1/brokers/futu/margin-ratios"},
		{name: "max trade quantity", method: http.MethodGet, path: "/api/v1/brokers/futu/max-trade-qtys"},
		{name: "quote", method: http.MethodGet, path: "/api/v1/brokers/futu/quote"},
		{name: "klines", method: http.MethodGet, path: "/api/v1/brokers/futu/klines"},
		{name: "securities", method: http.MethodGet, path: "/api/v1/brokers/futu/securities"},
		{name: "write", method: http.MethodPost, path: "/api/v1/brokers/futu/orders"},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), endpoint.method, endpoint.path, nil)
			request.URL.RawQuery = "%zz"
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"BAD_REQUEST"`) {
				t.Fatalf("%s response = %d %s", endpoint.name, response.Code, response.Body.String())
			}
		})
	}

	// Direct handler use is also guarded: a missing bound broker id must not
	// be treated as a request for the implicit active broker.
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/portfolio//cash-balances", nil)
	handlePortfolioRead(service, "cash-balances")(ctx)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing portfolio broker response = %d %s", response.Code, response.Body.String())
	}
}
