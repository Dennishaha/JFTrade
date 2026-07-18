package trading

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type routeTestBroker struct {
	id      string
	trading broker.TradingService
	data    broker.MarketDataReader
}

func (b *routeTestBroker) ID() string                    { return b.id }
func (b *routeTestBroker) Descriptor() broker.Descriptor { return broker.Descriptor{} }
func (b *routeTestBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b *routeTestBroker) Trading() broker.TradingService      { return b.trading }
func (b *routeTestBroker) MarketData() broker.MarketDataReader { return b.data }

func TestBrokerRoutesPreserveFallbackSemanticsAndRequestValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := srv.NewService()
	RegisterRoutes(router.Group("/api/v1"), service)
	RegisterPortfolioRoutes(router.Group("/api/v1"), service)

	fundsRec := httptest.NewRecorder()
	router.ServeHTTP(fundsRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/funds", nil))
	if fundsRec.Code != http.StatusOK {
		t.Fatalf("funds status=%d body=%s", fundsRec.Code, fundsRec.Body.String())
	}
	var envelope httpserver.Envelope
	if err := json.Unmarshal(fundsRec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal funds envelope: %v", err)
	}
	data := envelope.Data.(map[string]any)
	if data["connectivity"] != "degraded" || data["lastError"] == nil {
		t.Fatalf("funds data=%#v", data)
	}

	portfolioRec := httptest.NewRecorder()
	router.ServeHTTP(portfolioRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/portfolio/futu/cash-balances", nil))
	if portfolioRec.Code != http.StatusOK {
		t.Fatalf("portfolio status=%d body=%s", portfolioRec.Code, portfolioRec.Body.String())
	}

	feesRec := httptest.NewRecorder()
	router.ServeHTTP(feesRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/order-fees", nil))
	if feesRec.Code != http.StatusBadRequest {
		t.Fatalf("order fees status=%d body=%s", feesRec.Code, feesRec.Body.String())
	}

	cashFlowRec := httptest.NewRecorder()
	router.ServeHTTP(cashFlowRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/cash-flows", nil))
	if cashFlowRec.Code != http.StatusBadRequest {
		t.Fatalf("cash flow status=%d body=%s", cashFlowRec.Code, cashFlowRec.Body.String())
	}

	writeReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/brokers/futu/orders", strings.NewReader(`{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}`))
	writeReq.Header.Set("Content-Type", "application/json")
	writeRec := httptest.NewRecorder()
	router.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("write status=%d body=%s", writeRec.Code, writeRec.Body.String())
	}

	notFoundRec := httptest.NewRecorder()
	router.ServeHTTP(notFoundRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/ib/runtime", nil))
	if notFoundRec.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", notFoundRec.Code, notFoundRec.Body.String())
	}
	if err := json.Unmarshal(notFoundRec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal runtime envelope: %v", err)
	}
	if runtimeData, ok := envelope.Data.(map[string]any); !ok || len(runtimeData) != 0 {
		t.Fatalf("runtime data=%#v, want broker-neutral degraded empty state", envelope.Data)
	}
}

func TestBrokerWriteRoutesClassifyUnsupportedTradingAndUnlock(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := srv.NewService(srv.WithActiveBroker(func() broker.Broker {
		return &routeTestBroker{id: "futu"}
	}))
	RegisterRoutes(router.Group("/api/v1"), service)

	orderReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/brokers/futu/orders", strings.NewReader(`{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1,"price":100}`))
	orderReq.Header.Set("Content-Type", "application/json")
	orderRec := httptest.NewRecorder()
	router.ServeHTTP(orderRec, orderReq)
	if orderRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("order status=%d body=%s", orderRec.Code, orderRec.Body.String())
	}

	unlockReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/brokers/futu/unlock", strings.NewReader(`{"unlock":true,"passwordMd5":"abc"}`))
	unlockReq.Header.Set("Content-Type", "application/json")
	unlockRec := httptest.NewRecorder()
	router.ServeHTTP(unlockRec, unlockReq)
	if unlockRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unlock status=%d body=%s", unlockRec.Code, unlockRec.Body.String())
	}
}

func TestBrokerReadRoutesCoverOrdersFillsQuotesKLinesAndSecurities(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := srv.NewService()
	RegisterRoutes(router.Group("/api/v1"), service)

	for _, path := range []string{
		"/api/v1/brokers/futu/orders?scope=history&symbol=US.AAPL",
		"/api/v1/brokers/futu/fills?scope=current&symbol=US.AAPL",
		"/api/v1/brokers/futu/quote?symbol=US.AAPL",
		"/api/v1/brokers/futu/klines?symbol=US.AAPL&period=1d&limit=10",
		"/api/v1/brokers/futu/securities?symbol=US.AAPL",
		"/api/v1/brokers/futu/margin-ratios?market=US&symbol=AAPL",
		"/api/v1/brokers/futu/max-trade-qtys?market=US&symbol=US.AAPL&orderType=LIMIT&price=100&adjustSideAndLimit=1.5&positionId=42",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestBrokerReadRoutesRejectInvalidScopeAndNumericQueryValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := srv.NewService()
	RegisterRoutes(router.Group("/api/v1"), service)

	scopeRec := httptest.NewRecorder()
	router.ServeHTTP(scopeRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/orders?scope=bad", nil))
	if scopeRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid scope status=%d body=%s", scopeRec.Code, scopeRec.Body.String())
	}

	maxQtyRec := httptest.NewRecorder()
	router.ServeHTTP(maxQtyRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/max-trade-qtys?market=US&symbol=US.AAPL&orderType=LIMIT&price=bad", nil))
	if maxQtyRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid max trade qty status=%d body=%s", maxQtyRec.Code, maxQtyRec.Body.String())
	}

	klineRec := httptest.NewRecorder()
	router.ServeHTTP(klineRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/klines?symbol=US.AAPL&limit=bad", nil))
	if klineRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid klines limit status=%d body=%s", klineRec.Code, klineRec.Body.String())
	}
}
