package trading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type executionRouteOrderUpdateSource struct {
	accounts     []srv.Account
	currentCalls int
	historyCalls int
}

func (s *executionRouteOrderUpdateSource) DiscoverAccounts(context.Context) ([]srv.Account, error) {
	return append([]srv.Account(nil), s.accounts...), nil
}

func (s *executionRouteOrderUpdateSource) CurrentOrders(context.Context, srv.OrderQuery) ([]srv.Order, error) {
	s.currentCalls++
	return nil, nil
}

func (s *executionRouteOrderUpdateSource) HistoryOrders(context.Context, srv.OrderQuery, time.Time, time.Time) ([]srv.Order, error) {
	s.historyCalls++
	return nil, nil
}

func (s *executionRouteOrderUpdateSource) Subscribe(context.Context, []srv.Account, []srv.OrderQuery, srv.OrderUpdateHandler) (srv.OrderUpdateSubscription, error) {
	return executionRouteSubscription{}, nil
}

type executionRouteSubscription struct{}

func (executionRouteSubscription) Stop() error { return nil }

type executionRouteExecutionUpdates struct{}

func (*executionRouteExecutionUpdates) ApplyOrder(context.Context, string, srv.Order, srv.OrderWriteMetadata) {
}

func (*executionRouteExecutionUpdates) ApplyFill(context.Context, string, srv.Fill) {}

func TestExecutionRoutesValidatePayloadsAndMapHandlerErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("place and preview reject malformed json", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService()
		RegisterExecutionRoutes(router.Group("/api/v1"), service)

		for _, path := range []string{
			"/api/v1/execution/orders",
			"/api/v1/execution/orders/preview",
		} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(`{"market":`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("preview validation errors become bad request", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService()
		RegisterExecutionRoutes(router.Group("/api/v1"), service)

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders/preview", strings.NewReader(`{"brokerId":"ib","market":"US","symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"price":100}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("place maps broker timeout and request validation failures", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(
			srv.WithPlaceOrder(func(context.Context, srv.ExecutionOrderCommand) (srv.ExecutionOrder, error) {
				return srv.ExecutionOrder{}, broker.NewBrokerError("futu", broker.ErrCodeTimeout, "timed out")
			}),
		)
		RegisterExecutionRoutes(router.Group("/api/v1"), service)

		timeoutRec := httptest.NewRecorder()
		timeoutReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders", strings.NewReader(`{"market":"US","symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"price":100}`))
		timeoutReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(timeoutRec, timeoutReq)
		if timeoutRec.Code != http.StatusGatewayTimeout {
			t.Fatalf("timeout status=%d body=%s", timeoutRec.Code, timeoutRec.Body.String())
		}

		validationRec := httptest.NewRecorder()
		validationReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders", strings.NewReader(`{"market":"US","symbol":"AAPL","side":"BUY","orderType":"LIMIT","quantity":1}`))
		validationReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(validationRec, validationReq)
		if validationRec.Code != http.StatusBadRequest {
			t.Fatalf("validation status=%d body=%s", validationRec.Code, validationRec.Body.String())
		}
	})

	t.Run("list and events propagate service failures", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(
			srv.WithListOrders(func(context.Context, srv.ExecutionOrderFilter) (srv.ExecutionOrders, error) {
				return srv.ExecutionOrders{}, context.DeadlineExceeded
			}),
			srv.WithGetOrderEvents(func(context.Context, string) (srv.ExecutionOrderEvents, error) {
				return srv.ExecutionOrderEvents{}, context.DeadlineExceeded
			}),
		)
		RegisterExecutionRoutes(router.Group("/api/v1"), service)

		listRec := httptest.NewRecorder()
		listReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/execution/orders?scope=active", nil)
		router.ServeHTTP(listRec, listReq)
		if listRec.Code != http.StatusInternalServerError {
			t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
		}

		eventsRec := httptest.NewRecorder()
		eventsReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/execution/orders/internal-1/events", nil)
		router.ServeHTTP(eventsRec, eventsReq)
		if eventsRec.Code != http.StatusInternalServerError {
			t.Fatalf("events status=%d body=%s", eventsRec.Code, eventsRec.Body.String())
		}
	})

	t.Run("cancel success writes ok envelope", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(
			srv.WithCancelOrder(func(context.Context, string) (srv.ExecutionOrder, error) {
				return srv.ExecutionOrder{InternalOrderID: "internal-1", Status: "CANCELING"}, nil
			}),
		)
		RegisterExecutionRoutes(router.Group("/api/v1"), service)

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/execution/orders/internal-1/cancel", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestExecutionOrdersRouteSwitchesBetweenActiveAndHistorySync(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	source := &executionRouteOrderUpdateSource{
		accounts: []srv.Account{{
			ID: "acc-1", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
		}},
	}
	worker := srv.NewOrderUpdatesWorker(source, &executionRouteExecutionUpdates{}, srv.OrderUpdatesConfig{
		Now: func() time.Time { return now },
	})
	service := srv.NewService(
		srv.WithOrderUpdates(worker),
		srv.WithListOrders(func(context.Context, srv.ExecutionOrderFilter) (srv.ExecutionOrders, error) {
			return srv.ExecutionOrders{}, nil
		}),
	)

	router := gin.New()
	RegisterExecutionRoutes(router.Group("/api/v1"), service)

	activeRec := httptest.NewRecorder()
	activeReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/execution/orders?scope=active", nil)
	router.ServeHTTP(activeRec, activeReq)
	if activeRec.Code != http.StatusOK {
		t.Fatalf("active scope status=%d body=%s", activeRec.Code, activeRec.Body.String())
	}
	if source.currentCalls != 1 || source.historyCalls != 0 {
		t.Fatalf("active scope current/history = %d/%d, want 1/0", source.currentCalls, source.historyCalls)
	}

	now = now.Add(2 * time.Second)
	allRec := httptest.NewRecorder()
	allReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/execution/orders", nil)
	router.ServeHTTP(allRec, allReq)
	if allRec.Code != http.StatusOK {
		t.Fatalf("default scope status=%d body=%s", allRec.Code, allRec.Body.String())
	}
	if source.currentCalls != 2 || source.historyCalls != 1 {
		t.Fatalf("default scope current/history = %d/%d, want 2/1", source.currentCalls, source.historyCalls)
	}
}

func TestExecutionHandlersRejectMissingInternalOrderIDAndTrimWhitespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("events and cancel reject missing uri values", func(t *testing.T) {
		handlers := []gin.HandlerFunc{
			handleExecutionEvents(srv.NewService()),
			handleExecutionCancel(srv.NewService()),
		}
		for _, handler := range handlers {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/execution/orders//events", nil)

			handler(ctx)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		}
	})

	t.Run("bind helper trims whitespace", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "internalOrderId", Value: " internal-1 "}}
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/execution/orders/internal-1/events", nil)

		id, ok := bindInternalOrderID(ctx)
		if !ok || id != "internal-1" {
			t.Fatalf("bindInternalOrderID id=%q ok=%v", id, ok)
		}
	})
}
