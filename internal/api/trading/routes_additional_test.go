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

type routeStubMarketDataReader struct {
	queryFunds            func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error)
	queryPositions        func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error)
	queryOrderFees        func(context.Context, broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error)
	queryMarginRatios     func(context.Context, broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error)
	queryCashFlows        func(context.Context, broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error)
	queryQuote            func(context.Context, broker.QuoteQuery) (*broker.QuoteSnapshot, error)
	querySecuritySnapshot func(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error)
}

func (s *routeStubMarketDataReader) QueryFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
	if s.queryFunds != nil {
		return s.queryFunds(ctx, query)
	}
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	if s.queryPositions != nil {
		return s.queryPositions(ctx, query)
	}
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryOrders(context.Context, broker.ReadQuery, string) ([]broker.OrderSnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryHistoryOrders(context.Context, broker.OrderHistoryQuery) ([]broker.OrderSnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryOrderFills(context.Context, broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryHistoryOrderFills(context.Context, broker.OrderFillHistoryQuery) ([]broker.OrderFillSnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryOrderFees(ctx context.Context, query broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
	if s.queryOrderFees != nil {
		return s.queryOrderFees(ctx, query)
	}
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryMarginRatios(ctx context.Context, query broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
	if s.queryMarginRatios != nil {
		return s.queryMarginRatios(ctx, query)
	}
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryCashFlows(ctx context.Context, query broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
	if s.queryCashFlows != nil {
		return s.queryCashFlows(ctx, query)
	}
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryMaxTradeQuantity(context.Context, broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryQuote(ctx context.Context, query broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
	if s.queryQuote != nil {
		return s.queryQuote(ctx, query)
	}
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryKLines(context.Context, broker.KLineQuery) (*broker.KLineSnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QuerySecurityInfo(context.Context, broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QuerySecuritySearch(context.Context, broker.SecuritySearchQuery) (*broker.SecuritySearchSnapshot, error) {
	return nil, nil
}

func (s *routeStubMarketDataReader) QuerySecuritySnapshot(ctx context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	if s.querySecuritySnapshot != nil {
		return s.querySecuritySnapshot(ctx, query)
	}
	return nil, nil
}

func (s *routeStubMarketDataReader) QueryOrderBook(context.Context, broker.OrderBookQuery) (*broker.OrderBookSnapshot, error) {
	return nil, nil
}

type routeStubTradingService struct {
	placeOrder   func(context.Context, broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error)
	cancelOrders func(context.Context, broker.ReadQuery, ...broker.CancelOrder) error
}

func (s *routeStubTradingService) PlaceOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	if s.placeOrder != nil {
		return s.placeOrder(ctx, query)
	}
	return nil, nil
}

func (s *routeStubTradingService) CancelOrders(ctx context.Context, query broker.ReadQuery, orders ...broker.CancelOrder) error {
	if s.cancelOrders != nil {
		return s.cancelOrders(ctx, query, orders...)
	}
	return nil
}

type routeUnlockBroker struct {
	*routeTestBroker
	unlockTrade func(context.Context, broker.UnlockTradeRequest) error
}

func (b *routeUnlockBroker) UnlockTrade(ctx context.Context, req broker.UnlockTradeRequest) error {
	if b.unlockTrade != nil {
		return b.unlockTrade(ctx, req)
	}
	return nil
}

func TestPortfolioRoutesUseBrokerSnapshotsAndMissingBrokerSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	usd := "USD"
	cash := 1234.5
	averageCost := 98.7
	reader := &routeStubMarketDataReader{
		queryFunds: func(_ context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
			if query.BrokerID != "futu" || query.AccountID != "acc-1" || query.TradingEnvironment != "REAL" {
				t.Fatalf("funds query = %#v", query)
			}
			return &broker.FundsSnapshot{
				AccountID:          "acc-1",
				TradingEnvironment: "REAL",
				CurrencyBalances: []broker.CurrencyBalanceSnapshot{{
					AccountID: "acc-1", TradingEnvironment: "REAL", Currency: usd, Cash: &cash,
				}},
			}, nil
		},
		queryPositions: func(_ context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error) {
			if query.BrokerID != "futu" || query.AccountID != "acc-1" || query.TradingEnvironment != "REAL" {
				t.Fatalf("positions query = %#v", query)
			}
			return []broker.PositionSnapshot{{
				AccountID: "acc-1", TradingEnvironment: "REAL", Market: "US", Symbol: "US.AAPL",
				Quantity: 10, AverageCostPrice: &averageCost, MarketValue: 1000,
			}}, nil
		},
	}

	service := srv.NewService(srv.WithActiveBroker(func() broker.Broker {
		return &routeTestBroker{id: "futu", data: reader}
	}))
	router := gin.New()
	RegisterPortfolioRoutes(router.Group("/api/v1"), service)

	for _, tc := range []struct {
		path     string
		contains []string
	}{
		{
			path:     "/api/v1/portfolio/futu/cash-balances?accountId=acc-1&tradingEnvironment=REAL",
			contains: []string{`"balances"`, `"currency":"USD"`, `"cashBalance":1234.5`},
		},
		{
			path:     "/api/v1/portfolio/futu/positions?accountId=acc-1&tradingEnvironment=REAL",
			contains: []string{`"positions"`, `"symbol":"US.AAPL"`, `"averagePrice":98.7`},
		},
		{
			path:     "/api/v1/portfolio/futu/cash-reconciliation?accountId=acc-1&tradingEnvironment=REAL",
			contains: []string{`"balances"`, `"status":"missing-in-projection"`},
		},
		{
			path:     "/api/v1/portfolio/futu/reconciliation?accountId=acc-1&tradingEnvironment=REAL",
			contains: []string{`"positions"`, `"status":"missing-in-projection"`},
		},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		for _, want := range tc.contains {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("%s body=%s, want substring %q", tc.path, rec.Body.String(), want)
			}
		}
	}

	missingRec := httptest.NewRecorder()
	missingReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/portfolio/ib/positions", nil)
	router.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing broker status=%d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestBrokerWriteRoutesMapBodyBrokerAndOperationOutcomes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("invalid bodies return bad request", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(srv.WithActiveBroker(func() broker.Broker { return &routeTestBroker{id: "futu"} }))
		RegisterRoutes(router.Group("/api/v1"), service)

		for _, tc := range []struct {
			method string
			path   string
			body   string
		}{
			{method: http.MethodPost, path: "/api/v1/brokers/futu/orders", body: `{"symbol":`},
			{method: http.MethodDelete, path: "/api/v1/brokers/futu/orders", body: `{"orders":`},
			{method: http.MethodPost, path: "/api/v1/brokers/futu/unlock", body: `{"unlock":`},
		} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("wrong broker and unsupported write resource return not found", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(srv.WithActiveBroker(func() broker.Broker { return &routeTestBroker{id: "futu"} }))
		RegisterRoutes(router.Group("/api/v1"), service)

		wrongBrokerRec := httptest.NewRecorder()
		wrongBrokerReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/brokers/ib/orders", strings.NewReader(`{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}`))
		wrongBrokerReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(wrongBrokerRec, wrongBrokerReq)
		if wrongBrokerRec.Code != http.StatusNotFound {
			t.Fatalf("wrong broker status=%d body=%s", wrongBrokerRec.Code, wrongBrokerRec.Body.String())
		}

		unsupportedRec := httptest.NewRecorder()
		unsupportedReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/brokers/futu/funds", strings.NewReader(`{}`))
		unsupportedReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(unsupportedRec, unsupportedReq)
		if unsupportedRec.Code != http.StatusNotFound {
			t.Fatalf("unsupported resource status=%d body=%s", unsupportedRec.Code, unsupportedRec.Body.String())
		}
	})

	t.Run("cancel errors and success are mapped with operation semantics", func(t *testing.T) {
		var gotOrders []broker.CancelOrder
		tradingService := &routeStubTradingService{
			cancelOrders: func(_ context.Context, _ broker.ReadQuery, orders ...broker.CancelOrder) error {
				gotOrders = append([]broker.CancelOrder(nil), orders...)
				if orders[0].BrokerOrderID == "fail" {
					return errors.New("broker cancel failed")
				}
				return nil
			},
		}
		router := gin.New()
		service := srv.NewService(srv.WithActiveBroker(func() broker.Broker {
			return &routeTestBroker{id: "futu", trading: tradingService}
		}))
		RegisterRoutes(router.Group("/api/v1"), service)

		failRec := httptest.NewRecorder()
		failReq := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/brokers/futu/orders", strings.NewReader(`{"orders":[{"brokerOrderId":"fail","symbol":"US.AAPL"}]}`))
		failReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(failRec, failReq)
		if failRec.Code != http.StatusBadGateway || !strings.Contains(failRec.Body.String(), `"code":"CANCEL_FAILED"`) {
			t.Fatalf("cancel fail status=%d body=%s", failRec.Code, failRec.Body.String())
		}

		successRec := httptest.NewRecorder()
		successReq := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/brokers/futu/orders", strings.NewReader(`{"orders":[{"orderId":7,"brokerOrderId":"ok","symbol":"US.AAPL"}]}`))
		successReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(successRec, successReq)
		if successRec.Code != http.StatusOK {
			t.Fatalf("cancel success status=%d body=%s", successRec.Code, successRec.Body.String())
		}
		if len(gotOrders) != 1 || gotOrders[0].OrderID != 7 || gotOrders[0].BrokerOrderID != "ok" {
			t.Fatalf("cancel orders = %#v", gotOrders)
		}
	})

	t.Run("unlock success returns ok envelope", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(srv.WithActiveBroker(func() broker.Broker {
			return &routeUnlockBroker{
				routeTestBroker: &routeTestBroker{id: "futu"},
				unlockTrade: func(_ context.Context, req broker.UnlockTradeRequest) error {
					if !req.Unlock || req.PasswordMD5 != "abc" {
						t.Fatalf("unlock request = %#v", req)
					}
					return nil
				},
			}
		}))
		RegisterRoutes(router.Group("/api/v1"), service)

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/brokers/futu/unlock", strings.NewReader(`{"unlock":true,"passwordMd5":"abc"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unlock status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"ok":true`) {
			t.Fatalf("unlock body=%s", rec.Body.String())
		}
	})
}
