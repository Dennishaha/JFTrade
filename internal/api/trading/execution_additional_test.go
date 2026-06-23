package trading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

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
}
