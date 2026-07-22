package trading

import (
	"context"
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestServiceBrokerReadOperationsReturnFallbackWhenMarketDataUnavailable(t *testing.T) {
	service := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu"}
	}))
	query := broker.ReadQuery{BrokerID: "futu", AccountID: "acc-1", TradingEnvironment: "REAL", Market: "US"}

	cases := []struct {
		name string
		call func() (any, error)
		key  string
	}{
		{"funds", func() (any, error) { return service.Funds(t.Context(), query) }, "summary"},
		{"positions", func() (any, error) { return service.Positions(t.Context(), query) }, "positions"},
		{"orders", func() (any, error) { return service.Orders(t.Context(), OrdersQuery{ReadQuery: query}) }, "orders"},
		{"fills", func() (any, error) { return service.Fills(t.Context(), FillsQuery{ReadQuery: query}) }, "fills"},
		{"cash flows", func() (any, error) {
			return service.CashFlows(t.Context(), broker.CashFlowQuery{ReadQuery: query})
		}, "cashFlows"},
		{"fees", func() (any, error) {
			return service.OrderFees(t.Context(), broker.OrderFeeQuery{ReadQuery: query})
		}, "fees"},
		{"margin ratios", func() (any, error) {
			return service.MarginRatios(t.Context(), broker.MarginRatioQuery{ReadQuery: query})
		}, "marginRatios"},
		{"max quantity", func() (any, error) {
			return service.MaxTradeQuantity(t.Context(), broker.MaxTradeQuantityQuery{ReadQuery: query})
		}, "maxTradeQuantity"},
		{"quote", func() (any, error) { return service.Quote(t.Context(), broker.QuoteQuery{ReadQuery: query}) }, "quotes"},
		{"klines", func() (any, error) {
			return service.KLines(t.Context(), broker.KLineQuery{ReadQuery: query})
		}, "klines"},
		{"securities", func() (any, error) {
			return service.Securities(t.Context(), broker.SecuritySnapshotQuery{ReadQuery: query})
		}, "securities"},
		{"portfolio cash", func() (any, error) { return service.PortfolioCashBalances(t.Context(), query) }, "balances"},
		{"portfolio positions", func() (any, error) { return service.PortfolioPositions(t.Context(), query) }, "positions"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.call()
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if _, ok := responseJSONKeys(t, resp)[tc.key]; !ok {
				t.Fatalf("response missing %q: %#v", tc.key, resp)
			}
		})
	}
}

func TestServiceBrokerReadOperationsClassifyUpstreamFailures(t *testing.T) {
	upstream := errors.New("exchange unavailable")
	reader := &stubMarketDataReader{
		queryMaxTradeQuantity: func(context.Context, broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error) {
			return nil, upstream
		},
		queryKLines: func(context.Context, broker.KLineQuery) (*broker.KLineSnapshot, error) {
			return nil, upstream
		},
		querySecuritySnapshot: func(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
			return nil, upstream
		},
		queryFunds: func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
			return nil, upstream
		},
		queryPositions: func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
			return nil, upstream
		},
	}
	service := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: reader}
	}))
	query := broker.ReadQuery{BrokerID: "futu", AccountID: "acc-1", TradingEnvironment: "REAL", Market: "US"}

	cases := []struct {
		name string
		call func() (any, error)
	}{
		{"max quantity", func() (any, error) {
			return service.MaxTradeQuantity(t.Context(), broker.MaxTradeQuantityQuery{ReadQuery: query})
		}},
		{"klines", func() (any, error) {
			return service.KLines(t.Context(), broker.KLineQuery{ReadQuery: query})
		}},
		{"securities", func() (any, error) {
			return service.Securities(t.Context(), broker.SecuritySnapshotQuery{ReadQuery: query})
		}},
		{"portfolio cash", func() (any, error) { return service.PortfolioCashBalances(t.Context(), query) }},
		{"portfolio positions", func() (any, error) { return service.PortfolioPositions(t.Context(), query) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.call()
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if responseJSONKeys(t, resp)["connectivity"] == "connected" {
				t.Fatalf("response should not report connected: %#v", resp)
			}
		})
	}
}

func TestServiceBrokerWriteOperationsPropagateUpstreamFailures(t *testing.T) {
	upstream := errors.New("broker write failed")
	writer := &stubTradingService{
		placeOrder: func(context.Context, broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
			return nil, upstream
		},
		cancelOrders: func(context.Context, broker.ReadQuery, ...broker.CancelOrder) error {
			return upstream
		},
	}
	activeBroker := &unlockStubBroker{
		stubBroker: &stubBroker{id: "futu", trading: writer},
		unlockTrade: func(context.Context, broker.UnlockTradeRequest) error {
			return upstream
		},
	}
	service := NewService(
		WithActiveBroker(func() broker.Broker { return activeBroker }),
		WithPreTradeRiskGateway(NewStaticPreTradeRiskGateway(func() PreTradeRiskConfig {
			return PreTradeRiskConfig{RealTradingEnabled: true}
		})),
	)
	query := broker.ReadQuery{BrokerID: "futu", AccountID: "acc-1", TradingEnvironment: "REAL", Market: "US"}

	if _, err := service.PlaceBrokerOrder(t.Context(), broker.PlaceOrderQuery{
		ReadQuery: query,
		Symbol:    "US.AAPL",
		Side:      "BUY",
		OrderType: "MARKET",
		Quantity:  1,
	}); !errors.Is(err, upstream) {
		t.Fatalf("PlaceBrokerOrder error = %v, want upstream", err)
	}
	if _, err := service.CancelBrokerOrders(t.Context(), query, []broker.CancelOrder{{BrokerOrderID: "order-1"}}); !errors.Is(err, upstream) {
		t.Fatalf("CancelBrokerOrders error = %v, want upstream", err)
	}
	if _, err := service.UnlockTrade(t.Context(), broker.UnlockTradeRequest{ReadQuery: query, Unlock: true}); !errors.Is(err, upstream) {
		t.Fatalf("UnlockTrade error = %v, want upstream", err)
	}
}
