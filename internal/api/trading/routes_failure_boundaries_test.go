package trading

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestBrokerReadRoutesPreserveDegradedBackendFailureSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &routeStubMarketDataReader{
		queryQuote: func(context.Context, broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
			return nil, errors.New("quote backend unavailable")
		},
		queryPositions: func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
			return nil, errors.New("positions backend unavailable")
		},
	}
	service := srv.NewService(srv.WithActiveBroker(func() broker.Broker {
		return &routeTestBroker{id: "futu", data: reader}
	}))
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), service)
	RegisterPortfolioRoutes(router.Group("/api/v1"), service)

	cases := []struct {
		path     string
		contains []string
	}{
		{path: "/api/v1/brokers/futu/quote?symbol=US.AAPL", contains: []string{`"connectivity":"disconnected"`, `"lastError":"quote backend unavailable"`}},
		{path: "/api/v1/brokers/futu/positions", contains: []string{`"connectivity":"disconnected"`, `"lastError":"positions backend unavailable"`}},
		{path: "/api/v1/portfolio/futu/positions", contains: []string{`"positions":[]`}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("GET %s status=%d body=%s", tc.path, recorder.Code, recorder.Body.String())
			}
			for _, fragment := range tc.contains {
				if !strings.Contains(recorder.Body.String(), fragment) {
					t.Fatalf("GET %s body=%s, want %q", tc.path, recorder.Body.String(), fragment)
				}
			}
		})
	}
}

func TestBrokerReadRoutesRejectMissingAndInvalidOptionalInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService())

	paths := []string{
		"/api/v1/brokers/futu/fills?scope=unknown",
		"/api/v1/brokers/futu/max-trade-qtys?symbol=US.AAPL&orderType=LIMIT",
		"/api/v1/brokers/futu/max-trade-qtys?symbol=US.AAPL&orderType=LIMIT&price=100&adjustSideAndLimit=bad",
		"/api/v1/brokers/futu/max-trade-qtys?symbol=US.AAPL&orderType=LIMIT&price=100&positionId=bad",
		"/api/v1/brokers/futu/klines",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("GET %s status=%d want=400 body=%s", path, recorder.Code, recorder.Body.String())
			}
		})
	}

	if value, err := optionalFloat(" ", "adjustSideAndLimit"); err != nil || value != nil {
		t.Fatalf("optionalFloat blank = %#v, %v", value, err)
	}
	if value, err := optionalUint(" ", "positionId"); err != nil || value != nil {
		t.Fatalf("optionalUint blank = %#v, %v", value, err)
	}
}
