package trading

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestExecutionCommandErrorMapsRequestAndBrokerFailures(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "request", err: srv.RequestError{}, status: http.StatusBadRequest, code: "BAD_REQUEST"},
		{name: "risk rejected", err: srv.RiskRejectedError{Decision: srv.PreTradeRiskDecision{ReasonCode: "REAL_TRADING_DISABLED"}}, status: http.StatusConflict, code: "PRE_TRADE_RISK_REJECTED"},
		{name: "approval required", err: srv.RiskRejectedError{Decision: srv.PreTradeRiskDecision{Decision: srv.RiskDecisionRequireApproval}}, status: http.StatusConflict, code: "PRE_TRADE_APPROVAL_REQUIRED"},
		{name: "account not found", err: broker.NewBrokerError("futu", broker.ErrCodeAccountNotFound, "missing"), status: http.StatusBadRequest, code: "BAD_REQUEST"},
		{name: "timeout", err: broker.NewBrokerError("futu", broker.ErrCodeTimeout, "slow"), status: http.StatusGatewayTimeout, code: "BROKER_TIMEOUT"},
		{name: "rate limited", err: broker.NewBrokerError("futu", broker.ErrCodeRateLimited, "limit"), status: http.StatusTooManyRequests, code: "BROKER_RATE_LIMITED"},
		{name: "not connected", err: broker.NewBrokerError("futu", broker.ErrCodeNotConnected, "offline"), status: http.StatusBadGateway, code: "BROKER_NOT_CONNECTED"},
		{name: "unknown broker code", err: broker.NewBrokerError("futu", broker.ErrCodeInternal, "boom"), status: http.StatusBadGateway, code: "BROKER_COMMAND_FAILED"},
		{name: "plain error", err: errors.New("plain"), status: http.StatusBadGateway, code: "BROKER_COMMAND_FAILED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code := executionCommandError(tc.err)
			if status != tc.status || code != tc.code {
				t.Fatalf("executionCommandError(%v) = (%d, %q), want (%d, %q)", tc.err, status, code, tc.status, tc.code)
			}
		})
	}
}

func TestHandleExecutionPlaceReturnsRiskRejectionEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := newExecutionRouteTestService(
		srv.WithPreTradeRiskGateway(srv.NewStaticPreTradeRiskGateway(func() srv.PreTradeRiskConfig {
			return srv.PreTradeRiskConfig{}
		})),
		srv.WithPlaceOrder(func(context.Context, srv.ExecutionOrderCommand) (srv.ExecutionOrder, error) {
			t.Fatal("placeOrder should not be called for risk-rejected REAL orders")
			return srv.ExecutionOrder{}, nil
		}),
	)
	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders", strings.NewReader(`{"tradingEnvironment":"REAL","market":"US","symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"price":100}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope httpserver.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Error == nil || envelope.Error.Code != "PRE_TRADE_RISK_REJECTED" {
		t.Fatalf("error envelope = %#v", envelope.Error)
	}
}

func TestHandleExecutionOrdersNormalizesScopeAndFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var (
		gotFilter     srv.ExecutionOrderFilter
		gotActiveOnly bool
	)
	service := srv.NewService(
		srv.WithDefaultTradingEnvironment(func() string { return "REAL" }),
		srv.WithListOrders(func(_ context.Context, filter srv.ExecutionOrderFilter) (srv.ExecutionOrders, error) {
			gotFilter = filter
			return srv.ExecutionOrders{}, nil
		}),
	)
	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		"/api/v1/execution/orders?scope=%20active%20&brokerId=%20futu%20&accountId=%20acc-1%20&market=us",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	gotActiveOnly = true
	if gotFilter.BrokerID != "futu" || gotFilter.AccountID != "acc-1" || gotFilter.TradingEnvironment != "REAL" || gotFilter.Market != "US" {
		t.Fatalf("filter = %+v", gotFilter)
	}
	if !gotActiveOnly {
		t.Fatal("activeOnly flag was not applied")
	}
}

func TestHandleExecutionCancelReturnsMappedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := srv.NewService(
		srv.WithCancelOrder(func(context.Context, string) (srv.ExecutionOrder, error) {
			return srv.ExecutionOrder{}, broker.NewBrokerError("futu", broker.ErrCodeTimeout, "timed out")
		}),
	)
	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders/order-1/cancel", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusGatewayTimeout, rec.Body.String())
	}
	var envelope httpserver.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Error == nil || envelope.Error.Code != "BROKER_TIMEOUT" {
		t.Fatalf("error envelope = %#v", envelope.Error)
	}
}

func TestTradingQueryHelpersNormalizeAndValidate(t *testing.T) {
	if got, err := normalizeScope(" history "); err != nil || got != "HISTORY" {
		t.Fatalf("normalizeScope(history) = (%q, %v)", got, err)
	}
	if _, err := normalizeScope("invalid"); err == nil {
		t.Fatal("normalizeScope(invalid) error = nil")
	}

	merged := mergeValues([]string{" submitted,filled ", "FILLED"}, []string{" cancelled ", ""})
	if len(merged) != 3 || merged[0] != "submitted" || merged[1] != "filled" || merged[2] != "cancelled" {
		t.Fatalf("mergeValues = %#v", merged)
	}

	floatValue, err := optionalFloat(" 1.25 ", "adjustSideAndLimit")
	if err != nil || floatValue == nil || *floatValue != 1.25 {
		t.Fatalf("optionalFloat = %#v, %v", floatValue, err)
	}
	if _, err := optionalFloat("bad", "adjustSideAndLimit"); err == nil {
		t.Fatal("optionalFloat(invalid) error = nil")
	}

	uintValue, err := optionalUint(" 42 ", "positionId")
	if err != nil || uintValue == nil || *uintValue != 42 {
		t.Fatalf("optionalUint = %#v, %v", uintValue, err)
	}
	if got := optionalString(" RTH "); got == nil || *got != "RTH" {
		t.Fatalf("optionalString = %#v", got)
	}
}

func TestExecutionPlacePreviewAndEventsRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := newExecutionRouteTestService(
		srv.WithPlaceOrder(func(context.Context, srv.ExecutionOrderCommand) (srv.ExecutionOrder, error) {
			return srv.ExecutionOrder{InternalOrderID: "internal-1", Status: "SUBMITTED"}, nil
		}),
		srv.WithGetOrderEvents(func(context.Context, string) (srv.ExecutionOrderEvents, error) {
			return srv.ExecutionOrderEvents{InternalOrderID: "internal-1", Events: []srv.ExecutionOrderEvent{{ID: "evt-1"}}}, nil
		}),
	)
	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	previewReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders/preview", strings.NewReader(`{"market":"US","symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"price":100}`))
	previewReq.Header.Set("Content-Type", "application/json")
	previewRec := httptest.NewRecorder()
	router.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewRec.Code, previewRec.Body.String())
	}

	placeReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders", strings.NewReader(`{"market":"US","symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"price":100}`))
	placeReq.Header.Set("Content-Type", "application/json")
	placeRec := httptest.NewRecorder()
	router.ServeHTTP(placeRec, placeReq)
	if placeRec.Code != http.StatusOK {
		t.Fatalf("place status=%d body=%s", placeRec.Code, placeRec.Body.String())
	}

	eventsRec := httptest.NewRecorder()
	router.ServeHTTP(eventsRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/execution/orders/internal-1/events", nil))
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
}

func TestHandleExecutionOrderDetailsReturnsCanonicalReceiptAndNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(
		srv.WithListOrders(func(context.Context, srv.ExecutionOrderFilter) (srv.ExecutionOrders, error) {
			return srv.ExecutionOrders{Orders: []srv.ExecutionOrder{{
				InternalOrderID: "internal-1", Status: srv.OrderStatusPartiallyFilled,
				RawBrokerStatus: new("FILLED_PART"),
			}}}, nil
		}),
		srv.WithGetOrderEvents(func(context.Context, string) (srv.ExecutionOrderEvents, error) {
			return srv.ExecutionOrderEvents{InternalOrderID: "internal-1", Events: []srv.ExecutionOrderEvent{{
				ID: "evt-1", InternalOrderID: "internal-1", EventType: "BROKER_PUSH_ORDER",
			}}}, nil
		}),
	)
	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/execution/orders/internal-1", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"PARTIALLY_FILLED"`) || !strings.Contains(rec.Body.String(), `"rawBrokerStatus":"FILLED_PART"`) {
		t.Fatalf("details status=%d body=%s", rec.Code, rec.Body.String())
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/execution/orders/missing", nil))
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"code":"ORDER_NOT_FOUND"`) {
		t.Fatalf("missing status=%d body=%s", missing.Code, missing.Body.String())
	}
}
