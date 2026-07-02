package trading

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestTradingRouteHelpersWriteHTTPBoundaryErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("bind uri rejects missing route parameters", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/brokers//funds", nil)

		if uri, ok := bindURI(ctx); ok || uri.BrokerID != "" || recorder.Code != http.StatusNotFound {
			t.Fatalf("bindURI uri=%+v ok=%v status=%d body=%s", uri, ok, recorder.Code, recorder.Body.String())
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
