package trading

import (
	"context"
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCoverage98BrokerReadFailuresRemainVisibleAcrossAccountDataViews(t *testing.T) {
	backendErr := errors.New("broker connection unavailable")
	reader := &stubMarketDataReader{
		queryFunds: func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
			return nil, backendErr
		},
		queryPositions: func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
			return nil, backendErr
		},
		queryOrders: func(context.Context, broker.ReadQuery, string) ([]broker.OrderSnapshot, error) {
			return nil, backendErr
		},
		queryOrderFills: func(context.Context, broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error) {
			return nil, backendErr
		},
		queryCashFlows: func(context.Context, broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
			return nil, backendErr
		},
		queryOrderFees: func(context.Context, broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
			return nil, backendErr
		},
		queryMarginRatios: func(context.Context, broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
			return nil, backendErr
		},
	}
	service := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: reader}
	}))
	read := broker.ReadQuery{BrokerID: "futu", AccountID: "ACC-1", TradingEnvironment: "REAL", Market: "US"}

	cases := []struct {
		name string
		key  string
		call func() (any, error)
	}{
		{name: "funds", key: "summary", call: func() (any, error) { return service.Funds(t.Context(), read) }},
		{name: "positions", key: "positions", call: func() (any, error) { return service.Positions(t.Context(), read) }},
		{name: "current orders", key: "orders", call: func() (any, error) {
			return service.Orders(t.Context(), OrdersQuery{ReadQuery: read, Symbol: "US.AAPL"})
		}},
		{name: "current fills", key: "fills", call: func() (any, error) {
			return service.Fills(t.Context(), FillsQuery{ReadQuery: read, Symbol: "US.AAPL"})
		}},
		{name: "cash flows", key: "cashFlows", call: func() (any, error) {
			return service.CashFlows(t.Context(), broker.CashFlowQuery{ReadQuery: read, ClearingDate: "2026-07-17"})
		}},
		{name: "order fees", key: "fees", call: func() (any, error) {
			return service.OrderFees(t.Context(), broker.OrderFeeQuery{ReadQuery: read, OrderIDExList: []string{"order-1"}})
		}},
		{name: "margin ratios", key: "marginRatios", call: func() (any, error) {
			return service.MarginRatios(t.Context(), broker.MarginRatioQuery{ReadQuery: read, Symbols: []string{"US.AAPL"}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, err := tc.call()
			if err != nil {
				t.Fatalf("read failure returned transport error: %v", err)
			}
			keys := responseJSONKeys(t, response)
			if keys["connectivity"] != "disconnected" || keys["lastError"] != backendErr.Error() {
				t.Fatalf("failure response = %#v", response)
			}
			if _, ok := keys[tc.key]; !ok {
				t.Fatalf("failure response omitted %q: %#v", tc.key, response)
			}
		})
	}
}

func TestCoverage98FundsMapsMarketAssetsAlongsideCashBalances(t *testing.T) {
	available := 250.0
	service := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: &stubMarketDataReader{
			queryFunds: func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
				return &broker.FundsSnapshot{
					CurrencyBalances: []broker.CurrencyBalanceSnapshot{{Currency: "USD", Cash: &available}},
					MarketAssets:     []broker.MarketAssetSnapshot{{Market: "US", Assets: &available}},
				}, nil
			},
		}}
	}))

	response, err := service.Funds(t.Context(), broker.ReadQuery{BrokerID: "futu"})
	if err != nil {
		t.Fatalf("Funds: %v", err)
	}
	assets := response.MarketAssets
	if len(assets) != 1 {
		t.Fatalf("market assets = %#v", response.MarketAssets)
	}
	asset := assets[0]
	if asset.Market != "US" || asset.Assets != &available {
		t.Fatalf("market asset = %#v", asset)
	}

	balances, err := service.PortfolioCashBalances(t.Context(), broker.ReadQuery{BrokerID: "futu"})
	if err != nil {
		t.Fatalf("PortfolioCashBalances: %v", err)
	}
	portfolioBalances := balances.Balances
	if len(portfolioBalances) != 1 {
		t.Fatalf("portfolio balances = %#v", balances)
	}
	if got := portfolioBalances[0]; got.Currency != "USD" || got.CashBalance != available {
		t.Fatalf("portfolio balance = %#v", got)
	}

	unsupported := NewService(WithActiveBroker(func() broker.Broker { return &stubBroker{id: "futu"} }))
	if _, err := unsupported.CancelBrokerOrders(t.Context(), broker.ReadQuery{BrokerID: "futu"}, nil); !errors.Is(err, ErrTradingUnsupported) {
		t.Fatalf("CancelBrokerOrders without trading service = %v", err)
	}
}
