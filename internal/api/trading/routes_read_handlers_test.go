package trading

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestTradingReadHandlersValidateAndNormalizeBusinessQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var (
		gotCashFlows  broker.CashFlowQuery
		gotOrderFees  broker.OrderFeeQuery
		gotMargins    broker.MarginRatioQuery
		gotQuote      broker.QuoteQuery
		gotSecurities broker.SecuritySnapshotQuery
	)

	reader := &routeStubMarketDataReader{
		queryCashFlows: func(_ context.Context, query broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
			gotCashFlows = query
			return []broker.CashFlowSnapshot{{AccountID: "acc-1", Market: "US"}}, nil
		},
		queryOrderFees: func(_ context.Context, query broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
			gotOrderFees = query
			return []broker.OrderFeeSnapshot{{BrokerOrderIDEx: "ord-1"}}, nil
		},
		queryMarginRatios: func(_ context.Context, query broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
			gotMargins = query
			return []broker.MarginRatioSnapshot{{Symbol: "US.AAPL"}}, nil
		},
		queryQuote: func(_ context.Context, query broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
			gotQuote = query
			return &broker.QuoteSnapshot{Symbol: "US.AAPL", LastPrice: 100}, nil
		},
		querySecuritySnapshot: func(_ context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
			gotSecurities = query
			return &broker.SecuritySnapshotResult{Snapshots: []broker.SecuritySnapshotItem{{Symbol: "US.AAPL"}}}, nil
		},
	}

	router := gin.New()
	service := srv.NewService(srv.WithActiveBroker(func() broker.Broker {
		return &routeTestBroker{id: "futu", data: reader}
	}))
	RegisterRoutes(router.Group("/api/v1"), service)

	t.Run("cash flows requires clearing date and forwards direction", func(t *testing.T) {
		badRec := httptest.NewRecorder()
		badReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/cash-flows?accountId=acc-1", nil)
		router.ServeHTTP(badRec, badReq)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("bad status=%d body=%s", badRec.Code, badRec.Body.String())
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/cash-flows?accountId=acc-1&tradingEnvironment=REAL&market=US&clearingDate=2026-06-23&direction=OUT", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if gotCashFlows.ClearingDate != "2026-06-23" || gotCashFlows.Direction != "OUT" || gotCashFlows.AccountID != "acc-1" || gotCashFlows.Market != "US" {
			t.Fatalf("cash flow query = %#v", gotCashFlows)
		}
	})

	t.Run("order fees merges both order id inputs", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/order-fees?orderIdEx=ord-1&orderIdExList=ord-2,ord-1&market=US", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !reflect.DeepEqual(gotOrderFees.OrderIDExList, []string{"ord-1", "ord-2"}) {
			t.Fatalf("order fee query = %#v", gotOrderFees)
		}
	})

	t.Run("margin ratios validates symbol and normalizes by market", func(t *testing.T) {
		missingRec := httptest.NewRecorder()
		missingReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/margin-ratios?market=US", nil)
		router.ServeHTTP(missingRec, missingReq)
		if missingRec.Code != http.StatusBadRequest {
			t.Fatalf("missing symbol status=%d body=%s", missingRec.Code, missingRec.Body.String())
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/margin-ratios?market=US&symbol=AAPL&symbols=MSFT,AAPL", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !reflect.DeepEqual(gotMargins.Symbols, []string{"US.AAPL", "US.MSFT"}) {
			t.Fatalf("margin query = %#v", gotMargins)
		}
	})

	t.Run("quote requires symbol and merges aliases", func(t *testing.T) {
		badRec := httptest.NewRecorder()
		badReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/quote", nil)
		router.ServeHTTP(badRec, badReq)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("bad status=%d body=%s", badRec.Code, badRec.Body.String())
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/quote?symbol=US.AAPL&symbols=US.MSFT,US.AAPL", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !reflect.DeepEqual(gotQuote.Symbols, []string{"US.AAPL", "US.MSFT"}) {
			t.Fatalf("quote query = %#v", gotQuote)
		}
		if !strings.Contains(rec.Body.String(), `"quote"`) {
			t.Fatalf("quote body=%s", rec.Body.String())
		}
	})

	t.Run("securities requires symbol and merges aliases", func(t *testing.T) {
		badRec := httptest.NewRecorder()
		badReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/securities", nil)
		router.ServeHTTP(badRec, badReq)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("bad status=%d body=%s", badRec.Code, badRec.Body.String())
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/brokers/futu/securities?symbol=US.AAPL&symbols=US.MSFT,US.AAPL", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !reflect.DeepEqual(gotSecurities.Symbols, []string{"US.AAPL", "US.MSFT"}) {
			t.Fatalf("securities query = %#v", gotSecurities)
		}
		if !strings.Contains(rec.Body.String(), `"securities"`) {
			t.Fatalf("securities body=%s", rec.Body.String())
		}
	})
}
