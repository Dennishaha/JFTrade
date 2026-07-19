package trading

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

type stubMarketDataReader struct {
	queryFunds             func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error)
	queryPositions         func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error)
	queryOrders            func(context.Context, broker.ReadQuery, string) ([]broker.OrderSnapshot, error)
	queryHistoryOrders     func(context.Context, broker.OrderHistoryQuery) ([]broker.OrderSnapshot, error)
	queryOrderFills        func(context.Context, broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error)
	queryHistoryOrderFills func(context.Context, broker.OrderFillHistoryQuery) ([]broker.OrderFillSnapshot, error)
	queryOrderFees         func(context.Context, broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error)
	queryMarginRatios      func(context.Context, broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error)
	queryCashFlows         func(context.Context, broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error)
	queryMaxTradeQuantity  func(context.Context, broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error)
	queryQuote             func(context.Context, broker.QuoteQuery) (*broker.QuoteSnapshot, error)
	queryKLines            func(context.Context, broker.KLineQuery) (*broker.KLineSnapshot, error)
	querySecurityInfo      func(context.Context, broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error)
	querySecuritySnapshot  func(context.Context, broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error)
	queryOrderBook         func(context.Context, broker.OrderBookQuery) (*broker.OrderBookSnapshot, error)
}

func (s *stubMarketDataReader) QueryFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
	if s.queryFunds != nil {
		return s.queryFunds(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	if s.queryPositions != nil {
		return s.queryPositions(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryOrders(ctx context.Context, query broker.ReadQuery, symbol string) ([]broker.OrderSnapshot, error) {
	if s.queryOrders != nil {
		return s.queryOrders(ctx, query, symbol)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryHistoryOrders(ctx context.Context, query broker.OrderHistoryQuery) ([]broker.OrderSnapshot, error) {
	if s.queryHistoryOrders != nil {
		return s.queryHistoryOrders(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryOrderFills(ctx context.Context, query broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error) {
	if s.queryOrderFills != nil {
		return s.queryOrderFills(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryHistoryOrderFills(ctx context.Context, query broker.OrderFillHistoryQuery) ([]broker.OrderFillSnapshot, error) {
	if s.queryHistoryOrderFills != nil {
		return s.queryHistoryOrderFills(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryOrderFees(ctx context.Context, query broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
	if s.queryOrderFees != nil {
		return s.queryOrderFees(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryMarginRatios(ctx context.Context, query broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
	if s.queryMarginRatios != nil {
		return s.queryMarginRatios(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryCashFlows(ctx context.Context, query broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
	if s.queryCashFlows != nil {
		return s.queryCashFlows(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryMaxTradeQuantity(ctx context.Context, query broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error) {
	if s.queryMaxTradeQuantity != nil {
		return s.queryMaxTradeQuantity(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryQuote(ctx context.Context, query broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
	if s.queryQuote != nil {
		return s.queryQuote(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryKLines(ctx context.Context, query broker.KLineQuery) (*broker.KLineSnapshot, error) {
	if s.queryKLines != nil {
		return s.queryKLines(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QuerySecurityInfo(ctx context.Context, query broker.SecurityInfoQuery) (*broker.SecurityInfoSnapshot, error) {
	if s.querySecurityInfo != nil {
		return s.querySecurityInfo(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QuerySecuritySearch(context.Context, broker.SecuritySearchQuery) (*broker.SecuritySearchSnapshot, error) {
	return nil, nil
}

func (s *stubMarketDataReader) QuerySecuritySnapshot(ctx context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	if s.querySecuritySnapshot != nil {
		return s.querySecuritySnapshot(ctx, query)
	}
	return nil, nil
}

func (s *stubMarketDataReader) QueryOrderBook(ctx context.Context, query broker.OrderBookQuery) (*broker.OrderBookSnapshot, error) {
	if s.queryOrderBook != nil {
		return s.queryOrderBook(ctx, query)
	}
	return nil, nil
}

type stubTradingService struct {
	placeOrder   func(context.Context, broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error)
	cancelOrders func(context.Context, broker.ReadQuery, ...broker.CancelOrder) error
}

func (s *stubTradingService) PlaceOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	if s.placeOrder != nil {
		return s.placeOrder(ctx, query)
	}
	return nil, nil
}

func (s *stubTradingService) CancelOrders(ctx context.Context, query broker.ReadQuery, orders ...broker.CancelOrder) error {
	if s.cancelOrders != nil {
		return s.cancelOrders(ctx, query, orders...)
	}
	return nil
}

type stubBroker struct {
	id      string
	data    broker.MarketDataReader
	trading broker.TradingService
}

func (b *stubBroker) ID() string                    { return b.id }
func (b *stubBroker) Descriptor() broker.Descriptor { return broker.Descriptor{} }
func (b *stubBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}
func (b *stubBroker) Trading() broker.TradingService      { return b.trading }
func (b *stubBroker) MarketData() broker.MarketDataReader { return b.data }

type unlockStubBroker struct {
	*stubBroker
	unlockTrade func(context.Context, broker.UnlockTradeRequest) error
}

func (b *unlockStubBroker) UnlockTrade(ctx context.Context, req broker.UnlockTradeRequest) error {
	if b.unlockTrade != nil {
		return b.unlockTrade(ctx, req)
	}
	return nil
}

func TestServiceBrokerReadOperationsMapSnapshotsAndQueries(t *testing.T) {
	var (
		gotHistoryOrders broker.OrderHistoryQuery
		gotOrdersQuery   broker.ReadQuery
		gotFillsQuery    broker.OrderFillQuery
		gotHistoryFills  broker.OrderFillHistoryQuery
		gotCashFlows     broker.CashFlowQuery
		gotOrderFees     broker.OrderFeeQuery
		gotMargins       broker.MarginRatioQuery
		gotMaxTrade      broker.MaxTradeQuantityQuery
		gotQuote         broker.QuoteQuery
		gotKLines        broker.KLineQuery
		gotSecurities    broker.SecuritySnapshotQuery
	)
	reader := &stubMarketDataReader{
		queryFunds: func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
			return &broker.FundsSnapshot{
				AccountID:          "acc-1",
				TradingEnvironment: "REAL",
				Market:             "US",
				Currency:           new("USD"),
				CurrencyBalances: []broker.CurrencyBalanceSnapshot{{
					AccountID:          "acc-1",
					TradingEnvironment: "REAL",
					Currency:           "USD",
					Cash:               new(float64(1000)),
				}},
			}, nil
		},
		queryPositions: func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
			return []broker.PositionSnapshot{{
				AccountID:          "acc-1",
				TradingEnvironment: "REAL",
				Market:             "US",
				Symbol:             "US.AAPL",
				Quantity:           10,
				AverageCostPrice:   new(float64(95)),
				MarketValue:        1050,
			}}, nil
		},
		queryOrders: func(_ context.Context, query broker.ReadQuery, symbol string) ([]broker.OrderSnapshot, error) {
			gotOrdersQuery = query
			if symbol != "US.AAPL" {
				t.Fatalf("QueryOrders symbol=%q", symbol)
			}
			return []broker.OrderSnapshot{{
				BrokerOrderID: "order-current",
				Symbol:        "US.AAPL",
				Side:          "BUY",
				OrderType:     "LIMIT",
				Status:        "SUBMITTED",
				Quantity:      1,
			}}, nil
		},
		queryHistoryOrders: func(_ context.Context, query broker.OrderHistoryQuery) ([]broker.OrderSnapshot, error) {
			gotHistoryOrders = query
			return []broker.OrderSnapshot{{
				BrokerOrderID:      "order-history",
				Symbol:             "US.AAPL",
				Side:               "SELL",
				OrderType:          "LIMIT",
				Status:             "FILLED",
				Quantity:           2,
				AccountID:          "",
				TradingEnvironment: "",
				Market:             "",
			}}, nil
		},
		queryOrderFills: func(_ context.Context, query broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error) {
			gotFillsQuery = query
			return []broker.OrderFillSnapshot{{
				AccountID:          "acc-1",
				TradingEnvironment: "REAL",
				Market:             "US",
				BrokerOrderID:      "order-current",
				BrokerFillID:       "fill-current",
				Symbol:             "US.AAPL",
				Side:               "BUY",
				FilledQuantity:     1,
			}}, nil
		},
		queryHistoryOrderFills: func(_ context.Context, query broker.OrderFillHistoryQuery) ([]broker.OrderFillSnapshot, error) {
			gotHistoryFills = query
			return []broker.OrderFillSnapshot{{
				AccountID:          "acc-1",
				TradingEnvironment: "REAL",
				Market:             "US",
				BrokerOrderID:      "order-history",
				BrokerFillID:       "fill-history",
				Symbol:             "US.AAPL",
				Side:               "SELL",
				FilledQuantity:     2,
			}}, nil
		},
		queryCashFlows: func(_ context.Context, query broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
			gotCashFlows = query
			return []broker.CashFlowSnapshot{{AccountID: "acc-1", Market: "US"}}, nil
		},
		queryOrderFees: func(_ context.Context, query broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
			gotOrderFees = query
			return []broker.OrderFeeSnapshot{{BrokerOrderIDEx: "fee-1"}}, nil
		},
		queryMarginRatios: func(_ context.Context, query broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
			gotMargins = query
			return []broker.MarginRatioSnapshot{{Symbol: "US.AAPL"}}, nil
		},
		queryMaxTradeQuantity: func(_ context.Context, query broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error) {
			gotMaxTrade = query
			return &broker.MaxTradeQuantitySnapshot{Symbol: "US.AAPL", Price: 100, MaxCashBuy: 9}, nil
		},
		queryQuote: func(_ context.Context, query broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
			gotQuote = query
			return &broker.QuoteSnapshot{Symbol: "US.AAPL", LastPrice: 101}, nil
		},
		queryKLines: func(_ context.Context, query broker.KLineQuery) (*broker.KLineSnapshot, error) {
			gotKLines = query
			return &broker.KLineSnapshot{Symbol: "US.AAPL", Period: "1d"}, nil
		},
		querySecuritySnapshot: func(_ context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
			gotSecurities = query
			return &broker.SecuritySnapshotResult{AccountID: "acc-1", Snapshots: []broker.SecuritySnapshotItem{{Symbol: "US.AAPL"}}}, nil
		},
	}
	service := NewService(
		WithActiveBroker(func() broker.Broker {
			return &stubBroker{id: "futu", data: reader}
		}),
		WithBrokerRuntime(func(context.Context) *BrokerRuntimeResponse {
			return &BrokerRuntimeResponse{Session: BrokerRuntimeSession{Connectivity: "connected"}}
		}),
	)
	query := broker.ReadQuery{BrokerID: "futu", AccountID: "acc-1", TradingEnvironment: "REAL", Market: "US"}

	runtimeResp, err := service.Runtime(t.Context(), "futu")
	if err != nil || runtimeResp.Session.Connectivity != "connected" {
		t.Fatalf("Runtime = %#v, %v", runtimeResp, err)
	}

	fundsResp, err := service.Funds(t.Context(), query)
	if err != nil {
		t.Fatalf("Funds: %v", err)
	}
	if fundsResp.Connectivity != "connected" {
		t.Fatalf("Funds connectivity=%v", fundsResp.Connectivity)
	}
	if len(fundsResp.CurrencyBalances) != 1 {
		t.Fatalf("Funds currencyBalances=%#v", fundsResp.CurrencyBalances)
	}

	positionsResp, err := service.Positions(t.Context(), query)
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	positions := positionsResp.Positions
	if len(positions) != 1 || positions[0].Symbol != "US.AAPL" {
		t.Fatalf("Positions response=%#v", positionsResp)
	}

	historyOrdersResp, err := service.Orders(t.Context(), OrdersQuery{
		ReadQuery: query,
		Scope:     "HISTORY",
		Symbol:    "US.AAPL",
		StartTime: "2025-01-01",
		EndTime:   "2025-01-31",
		Statuses:  []string{"FILLED"},
	})
	if err != nil {
		t.Fatalf("Orders history: %v", err)
	}
	if gotHistoryOrders.Symbol != "US.AAPL" || len(gotHistoryOrders.Statuses) != 1 || gotHistoryOrders.Statuses[0] != "FILLED" {
		t.Fatalf("QueryHistoryOrders=%#v", gotHistoryOrders)
	}
	historyOrders := historyOrdersResp.Orders
	historyOrder := historyOrders[0]
	if historyOrder.AccountID != "acc-1" || historyOrder.TradingEnvironment != "REAL" || historyOrder.Market != "US" {
		t.Fatalf("history order fallback=%#v", historyOrder)
	}

	currentOrdersResp, err := service.Orders(t.Context(), OrdersQuery{ReadQuery: query, Symbol: "US.AAPL"})
	if err != nil {
		t.Fatalf("Orders current: %v", err)
	}
	if gotOrdersQuery.AccountID != "acc-1" {
		t.Fatalf("QueryOrders query=%#v", gotOrdersQuery)
	}
	if len(currentOrdersResp.Orders) != 1 {
		t.Fatalf("current orders=%#v", currentOrdersResp)
	}

	currentFillsResp, err := service.Fills(t.Context(), FillsQuery{ReadQuery: query, Symbol: "US.AAPL"})
	if err != nil {
		t.Fatalf("Fills current: %v", err)
	}
	if gotFillsQuery.Symbol != "US.AAPL" {
		t.Fatalf("QueryOrderFills=%#v", gotFillsQuery)
	}
	if len(currentFillsResp.Fills) != 1 {
		t.Fatalf("current fills=%#v", currentFillsResp)
	}

	historyFillsResp, err := service.Fills(t.Context(), FillsQuery{
		ReadQuery: query,
		Scope:     "HISTORY",
		Symbol:    "US.AAPL",
		StartTime: "2025-01-01",
		EndTime:   "2025-01-31",
	})
	if err != nil {
		t.Fatalf("Fills history: %v", err)
	}
	if gotHistoryFills.Symbol != "US.AAPL" || gotHistoryFills.StartTime != "2025-01-01" {
		t.Fatalf("QueryHistoryOrderFills=%#v", gotHistoryFills)
	}
	if len(historyFillsResp.Fills) != 1 {
		t.Fatalf("history fills=%#v", historyFillsResp)
	}

	if _, err := service.CashFlows(t.Context(), broker.CashFlowQuery{ReadQuery: query, ClearingDate: "2025-01-01"}); err != nil {
		t.Fatalf("CashFlows: %v", err)
	}
	if gotCashFlows.ClearingDate != "2025-01-01" {
		t.Fatalf("QueryCashFlows=%#v", gotCashFlows)
	}

	if _, err := service.OrderFees(t.Context(), broker.OrderFeeQuery{ReadQuery: query, OrderIDExList: []string{"fee-1"}}); err != nil {
		t.Fatalf("OrderFees: %v", err)
	}
	if len(gotOrderFees.OrderIDExList) != 1 || gotOrderFees.OrderIDExList[0] != "fee-1" {
		t.Fatalf("QueryOrderFees=%#v", gotOrderFees)
	}

	if _, err := service.MarginRatios(t.Context(), broker.MarginRatioQuery{ReadQuery: query, Symbols: []string{"US.AAPL"}}); err != nil {
		t.Fatalf("MarginRatios: %v", err)
	}
	if len(gotMargins.Symbols) != 1 || gotMargins.Symbols[0] != "US.AAPL" {
		t.Fatalf("QueryMarginRatios=%#v", gotMargins)
	}

	if _, err := service.MaxTradeQuantity(t.Context(), broker.MaxTradeQuantityQuery{ReadQuery: query, Symbol: "US.AAPL", OrderType: "LIMIT", Price: 100}); err != nil {
		t.Fatalf("MaxTradeQuantity: %v", err)
	}
	if gotMaxTrade.Symbol != "US.AAPL" || gotMaxTrade.Price != 100 {
		t.Fatalf("QueryMaxTradeQuantity=%#v", gotMaxTrade)
	}

	if _, err := service.Quote(t.Context(), broker.QuoteQuery{ReadQuery: query, Symbols: []string{"US.AAPL"}}); err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if len(gotQuote.Symbols) != 1 || gotQuote.Symbols[0] != "US.AAPL" {
		t.Fatalf("QueryQuote=%#v", gotQuote)
	}

	if _, err := service.KLines(t.Context(), broker.KLineQuery{ReadQuery: query, Symbol: "US.AAPL", Period: "1d", Limit: 10}); err != nil {
		t.Fatalf("KLines: %v", err)
	}
	if gotKLines.Symbol != "US.AAPL" || gotKLines.Limit != 10 {
		t.Fatalf("QueryKLines=%#v", gotKLines)
	}

	if _, err := service.Securities(t.Context(), broker.SecuritySnapshotQuery{ReadQuery: query, Symbols: []string{"US.AAPL"}}); err != nil {
		t.Fatalf("Securities: %v", err)
	}
	if len(gotSecurities.Symbols) != 1 || gotSecurities.Symbols[0] != "US.AAPL" {
		t.Fatalf("QuerySecuritySnapshot=%#v", gotSecurities)
	}
}

func TestServicePortfolioAndFallbackResponses(t *testing.T) {
	reader := &stubMarketDataReader{
		queryFunds: func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
			return &broker.FundsSnapshot{
				AccountID:          "acc-1",
				TradingEnvironment: "REAL",
				Market:             "US",
				Currency:           new("USD"),
				Cash:               new(float64(2500)),
				CurrencyBalances:   nil,
			}, nil
		},
		queryPositions: func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
			return []broker.PositionSnapshot{{
				AccountID:          "acc-1",
				TradingEnvironment: "REAL",
				Market:             "US",
				Symbol:             "US.NVDA",
				Quantity:           3,
				CostPrice:          new(float64(98)),
				MarketValue:        360,
			}}, nil
		},
	}
	service := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: reader}
	}))
	query := broker.ReadQuery{BrokerID: "futu", AccountID: "acc-1", TradingEnvironment: "REAL", Market: "US"}

	cashBalances, err := service.PortfolioCashBalances(t.Context(), query)
	if err != nil {
		t.Fatalf("PortfolioCashBalances: %v", err)
	}
	balances := cashBalances.Balances
	if len(balances) != 1 {
		t.Fatalf("balances=%#v", cashBalances)
	}
	balance := balances[0]
	if balance.Currency != "USD" || balance.CashBalance != 2500.0 {
		t.Fatalf("balance=%#v", balance)
	}
	if balance.UpdatedAt == "" || balance.CreatedAt == "" {
		t.Fatalf("balance timestamps=%#v", balance)
	}

	portfolioPositions, err := service.PortfolioPositions(t.Context(), query)
	if err != nil {
		t.Fatalf("PortfolioPositions: %v", err)
	}
	position := portfolioPositions.Positions[0]
	if position.AveragePrice != 98.0 || position.Symbol != "US.NVDA" {
		t.Fatalf("portfolio position=%#v", position)
	}

	reconciliation, err := service.PortfolioReconciliation(t.Context(), query)
	if err != nil {
		t.Fatalf("PortfolioReconciliation: %v", err)
	}
	recPosition := reconciliation.Positions[0]
	if recPosition.Status != "missing-in-projection" || recPosition.QuantityDelta != 3.0 {
		t.Fatalf("reconciliation=%#v", recPosition)
	}

	cashReconciliation, err := service.PortfolioCashReconciliation(t.Context(), query)
	if err != nil {
		t.Fatalf("PortfolioCashReconciliation: %v", err)
	}
	recBalance := cashReconciliation.Balances
	if len(recBalance) != 0 {
		t.Fatalf("cash reconciliation=%#v", cashReconciliation)
	}

	noBrokerService := NewService()
	fundsResp, err := noBrokerService.Funds(t.Context(), broker.ReadQuery{BrokerID: "futu"})
	if err != nil {
		t.Fatalf("Funds without broker: %v", err)
	}
	if fundsResp.Connectivity != "degraded" || fundsResp.LastError == nil {
		t.Fatalf("Funds fallback=%#v", fundsResp)
	}

	cashReconciliationReader := &stubMarketDataReader{
		queryFunds: func(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
			return &broker.FundsSnapshot{
				CurrencyBalances: []broker.CurrencyBalanceSnapshot{{
					AccountID:               "acc-1",
					TradingEnvironment:      "REAL",
					Currency:                "USD",
					Cash:                    new(float64(500)),
					AvailableWithdrawalCash: new(float64(250)),
					NetCashPower:            new(float64(400)),
				}},
			}, nil
		},
	}
	cashReconciliationService := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: cashReconciliationReader}
	}))
	cashReconciliationResp, err := cashReconciliationService.PortfolioCashReconciliation(t.Context(), query)
	if err != nil {
		t.Fatalf("PortfolioCashReconciliation: %v", err)
	}
	recBalances := cashReconciliationResp.Balances
	if len(recBalances) != 1 {
		t.Fatalf("cash reconciliation=%#v", cashReconciliationResp)
	}
	recBalanceEntry := recBalances[0]
	if recBalanceEntry.Status != "missing-in-projection" || recBalanceEntry.CashDelta != 500.0 {
		t.Fatalf("cash reconciliation entry=%#v", recBalanceEntry)
	}

	timeoutReader := &stubMarketDataReader{
		queryQuote: func(context.Context, broker.QuoteQuery) (*broker.QuoteSnapshot, error) {
			return nil, errors.New("i/o timeout")
		},
	}
	timeoutService := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu", data: timeoutReader}
	}))
	quoteResp, err := timeoutService.Quote(t.Context(), queryToQuote(query, "US.AAPL"))
	if err != nil {
		t.Fatalf("Quote fallback: %v", err)
	}
	if quoteResp.Connectivity != "disconnected" {
		t.Fatalf("Quote connectivity=%#v", quoteResp)
	}
}

func TestServiceBrokerWriteAndTimeoutBehaviors(t *testing.T) {
	var (
		gotPlacedQuery   broker.PlaceOrderQuery
		gotCancelQuery   broker.ReadQuery
		gotCancelOrders  []broker.CancelOrder
		gotUnlockRequest broker.UnlockTradeRequest
	)
	writer := &stubTradingService{
		placeOrder: func(_ context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
			gotPlacedQuery = query
			return &broker.PlaceOrderResult{BrokerOrderID: "placed-1", Status: "SUBMITTED"}, nil
		},
		cancelOrders: func(_ context.Context, query broker.ReadQuery, orders ...broker.CancelOrder) error {
			gotCancelQuery = query
			gotCancelOrders = append([]broker.CancelOrder(nil), orders...)
			return nil
		},
	}
	activeBroker := &unlockStubBroker{
		stubBroker: &stubBroker{id: "futu", data: &stubMarketDataReader{
			queryFunds: func(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			queryPositions: func(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
				return []broker.PositionSnapshot{{Symbol: "US.AAPL", Quantity: 2}}, nil
			},
		}, trading: writer},
		unlockTrade: func(_ context.Context, req broker.UnlockTradeRequest) error {
			gotUnlockRequest = req
			return nil
		},
	}
	service := NewService(WithActiveBroker(func() broker.Broker { return activeBroker }))
	query := broker.ReadQuery{BrokerID: "futu", AccountID: "acc-1", TradingEnvironment: "REAL", Market: "US"}

	placeResp, err := service.PlaceBrokerOrder(t.Context(), broker.PlaceOrderQuery{
		ReadQuery: query,
		Symbol:    "US.AAPL",
		Side:      "BUY",
		OrderType: "LIMIT",
		Quantity:  1,
		Price:     new(float64(100)),
	})
	if err != nil {
		t.Fatalf("PlaceBrokerOrder: %v", err)
	}
	if gotPlacedQuery.Symbol != "US.AAPL" || placeResp.Order.BrokerOrderID != "placed-1" {
		t.Fatalf("place response=%#v query=%#v", placeResp, gotPlacedQuery)
	}
	if placeResp.PlacedAt == "" {
		t.Fatalf("place timestamp=%#v", placeResp)
	}

	cancelResp, err := service.CancelBrokerOrders(t.Context(), query, []broker.CancelOrder{{BrokerOrderID: "order-1"}})
	if err != nil {
		t.Fatalf("CancelBrokerOrders: %v", err)
	}
	if gotCancelQuery.AccountID != "acc-1" || len(gotCancelOrders) != 1 || gotCancelOrders[0].BrokerOrderID != "order-1" {
		t.Fatalf("cancel query=%#v orders=%#v", gotCancelQuery, gotCancelOrders)
	}
	if cancelResp.Cancelled != 1 {
		t.Fatalf("cancel response=%#v", cancelResp)
	}

	unlockResp, err := service.UnlockTrade(t.Context(), broker.UnlockTradeRequest{
		ReadQuery:   query,
		Unlock:      true,
		PasswordMD5: "abc",
	})
	if err != nil {
		t.Fatalf("UnlockTrade: %v", err)
	}
	if !unlockResp.Unlocked || gotUnlockRequest.PasswordMD5 != "abc" {
		t.Fatalf("unlock response=%#v req=%#v", unlockResp, gotUnlockRequest)
	}

	timeoutResp := service.FundsWithTimeout(t.Context(), query, time.Millisecond)
	if timeoutResp.LastError == nil || !strings.Contains(*timeoutResp.LastError, "timed out after") {
		t.Fatalf("FundsWithTimeout=%#v", timeoutResp)
	}

	positionsResp := service.PositionsWithTimeout(t.Context(), query, time.Second)
	if positionsResp.Connectivity != "connected" || len(positionsResp.Positions) != 1 {
		t.Fatalf("PositionsWithTimeout=%#v", positionsResp)
	}

	if _, err := NewService().PlaceBrokerOrder(t.Context(), broker.PlaceOrderQuery{}); !errors.Is(err, ErrNoBroker) {
		t.Fatalf("PlaceBrokerOrder no broker err=%v", err)
	}
	if _, err := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu"}
	})).PlaceBrokerOrder(t.Context(), broker.PlaceOrderQuery{ReadQuery: broker.ReadQuery{BrokerID: "futu"}}); !errors.Is(err, ErrTradingUnsupported) {
		t.Fatalf("PlaceBrokerOrder unsupported err=%v", err)
	}
	if _, err := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu"}
	})).UnlockTrade(t.Context(), broker.UnlockTradeRequest{ReadQuery: broker.ReadQuery{BrokerID: "futu"}}); !errors.Is(err, ErrUnlockUnsupported) {
		t.Fatalf("UnlockTrade unsupported err=%v", err)
	}
	if runtime, err := NewService().Runtime(t.Context(), "ib"); err != nil || runtime == nil || runtime.Session.Connectivity != "" {
		t.Fatalf("Runtime without any configured broker = %#v, err=%v", runtime, err)
	}
	if _, err := NewService(WithActiveBroker(func() broker.Broker {
		return &stubBroker{id: "futu"}
	})).Runtime(t.Context(), "ib"); !errors.Is(err, ErrBrokerNotFound) {
		t.Fatalf("Runtime active mismatch err=%v", err)
	}
}

func TestNormalizeSymbolsAndRuntimeDefaults(t *testing.T) {
	service := NewService(
		WithDefaultMarket(func() string { return "US" }),
		WithActiveBroker(func() broker.Broker { return &stubBroker{id: "futu"} }),
	)

	query := service.ReadQuery("futu", "REAL", "acc-1", "")
	if query.Market != "US" {
		t.Fatalf("ReadQuery default market=%#v", query)
	}

	normalized, err := NormalizeSymbols("US", []string{"AAPL", "MSFT"})
	if err != nil {
		t.Fatalf("NormalizeSymbols: %v", err)
	}
	if len(normalized) != 2 || normalized[0] != "US.AAPL" || normalized[1] != "US.MSFT" {
		t.Fatalf("NormalizeSymbols=%#v", normalized)
	}
	if _, err := NormalizeSymbols("US", []string{""}); err == nil {
		t.Fatal("NormalizeSymbols(invalid) error=nil")
	}

	runtimeResp, err := service.Runtime(t.Context(), "futu")
	if err != nil {
		t.Fatalf("Runtime default: %v", err)
	}
	if runtimeResp == nil || runtimeResp.Session.Connectivity != "" || len(runtimeResp.Accounts) != 0 {
		t.Fatalf("Runtime default response=%#v", runtimeResp)
	}
}

func queryToQuote(query broker.ReadQuery, symbol string) broker.QuoteQuery {
	return broker.QuoteQuery{ReadQuery: query, Symbols: []string{symbol}}
}

// responseJSONKeys 将 typed 响应序列化回 map，用于断言 JSON 键与历史 map 响应一致。
func responseJSONKeys(t *testing.T, response any) map[string]any {
	t.Helper()
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return keys
}
