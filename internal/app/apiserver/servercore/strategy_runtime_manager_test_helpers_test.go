package servercore

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/broker"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type strategyRuntimeStubExchange struct {
	mu                sync.Mutex
	markets           bbgotypes.MarketMap
	history           map[string][]bbgotypes.KLine
	funds             *broker.FundsSnapshot
	positions         []broker.PositionSnapshot
	placedOrders      []bbgotypes.SubmitOrder
	nextOrderID       uint64
	queryFundsErr     error
	queryPositionsErr error
	panicOnPlaceOrder bool
}

func newStrategyRuntimeStubExchange() *strategyRuntimeStubExchange {
	markets := bbgotypes.MarketMap{
		"US.AAPL": {
			Symbol:        "US.AAPL",
			BaseCurrency:  "AAPL",
			QuoteCurrency: "USD",
		},
		"US.MSFT": {
			Symbol:        "US.MSFT",
			BaseCurrency:  "MSFT",
			QuoteCurrency: "USD",
		},
		"HK.00700": {
			Symbol:        "HK.00700",
			BaseCurrency:  "00700",
			QuoteCurrency: "HKD",
		},
	}
	return &strategyRuntimeStubExchange{
		markets: markets,
		history: map[string][]bbgotypes.KLine{
			"US.AAPL":  {strategyRuntimeHistoricalKLine("US.AAPL", "1m", 99, strategyRuntimeTestTime(9, 59, 0))},
			"US.MSFT":  {strategyRuntimeHistoricalKLine("US.MSFT", "30m", 300, strategyRuntimeTestTime(9, 30, 0))},
			"HK.00700": {strategyRuntimeHistoricalKLine("HK.00700", "15m", 420, strategyRuntimeTestTime(9, 30, 0))},
		},
		funds: &broker.FundsSnapshot{
			AccountID:               "123456",
			TradingEnvironment:      "SIMULATE",
			Market:                  "US",
			TotalAssets:             new(float64(100000)),
			AvailableFunds:          new(float64(100000)),
			AvailableWithdrawalCash: new(float64(100000)),
			CurrencyBalances: []broker.CurrencyBalanceSnapshot{{
				AccountID:               "123456",
				TradingEnvironment:      "SIMULATE",
				Currency:                "USD",
				Cash:                    new(float64(100000)),
				NetCashPower:            new(float64(100000)),
				AvailableWithdrawalCash: new(float64(100000)),
			}},
		},
	}
}

func (e *strategyRuntimeStubExchange) Name() bbgotypes.ExchangeName {
	return bbgotypes.ExchangeName("futu")
}

func (e *strategyRuntimeStubExchange) PlatformFeeCurrency() string {
	return "USD"
}

func (e *strategyRuntimeStubExchange) NewStream() bbgotypes.Stream {
	return new(bbgotypes.NewStandardStream())
}

func (e *strategyRuntimeStubExchange) QueryMarkets(context.Context) (bbgotypes.MarketMap, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make(bbgotypes.MarketMap, len(e.markets))
	maps.Copy(result, e.markets)
	return result, nil
}

func (e *strategyRuntimeStubExchange) EnsureMarket(symbol string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return
	}
	if _, ok := e.markets[symbol]; ok {
		return
	}
	market := bbgotypes.Market{
		Symbol:        symbol,
		BaseCurrency:  symbol,
		QuoteCurrency: "HKD",
	}
	if strings.HasPrefix(symbol, "US.") {
		market.QuoteCurrency = "USD"
	}
	e.markets[symbol] = market
	if _, ok := e.history[symbol]; !ok {
		e.history[symbol] = []bbgotypes.KLine{
			strategyRuntimeHistoricalKLine(symbol, "5m", 50, strategyRuntimeTestTime(9, 30, 0)),
		}
	}
}

func (e *strategyRuntimeStubExchange) QueryTicker(_ context.Context, symbol string) (*bbgotypes.Ticker, error) {
	tickerMap, err := e.QueryTickers(context.Background(), symbol)
	if err != nil {
		return nil, err
	}
	return new(tickerMap[symbol]), nil
}

func (e *strategyRuntimeStubExchange) QueryTickers(_ context.Context, symbols ...string) (map[string]bbgotypes.Ticker, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make(map[string]bbgotypes.Ticker, len(symbols))
	for _, symbol := range symbols {
		price := 100.0
		if history := e.history[strings.ToUpper(strings.TrimSpace(symbol))]; len(history) > 0 {
			price = history[len(history)-1].Close.Float64()
		}
		result[symbol] = bbgotypes.Ticker{
			Time: time.Now().UTC(),
			Last: fixedpoint.NewFromFloat(price),
			Buy:  fixedpoint.NewFromFloat(price),
			Sell: fixedpoint.NewFromFloat(price),
		}
	}
	return result, nil
}

func (e *strategyRuntimeStubExchange) QueryKLines(_ context.Context, symbol string, interval bbgotypes.Interval, options bbgotypes.KLineQueryOptions) ([]bbgotypes.KLine, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	history := append([]bbgotypes.KLine(nil), e.history[strings.ToUpper(strings.TrimSpace(symbol))]...)
	for index := range history {
		history[index].Interval = interval
	}
	if options.Limit > 0 && len(history) > options.Limit {
		history = history[len(history)-options.Limit:]
	}
	return history, nil
}

func (e *strategyRuntimeStubExchange) appendHistory(symbol string, klines ...bbgotypes.KLine) {
	e.mu.Lock()
	defer e.mu.Unlock()
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		return
	}
	nextHistory := append([]bbgotypes.KLine(nil), e.history[normalizedSymbol]...)
	nextHistory = append(nextHistory, klines...)
	e.history[normalizedSymbol] = nextHistory
}

func (e *strategyRuntimeStubExchange) QueryAccount(context.Context) (*bbgotypes.Account, error) {
	return bbgotypes.NewAccount(), nil
}

func (e *strategyRuntimeStubExchange) QueryAccountBalances(context.Context) (bbgotypes.BalanceMap, error) {
	return bbgotypes.BalanceMap{}, nil
}

func (e *strategyRuntimeStubExchange) SubmitOrder(context.Context, bbgotypes.SubmitOrder) (*bbgotypes.Order, error) {
	return nil, nil
}

func (e *strategyRuntimeStubExchange) QueryOpenOrders(context.Context, string) ([]bbgotypes.Order, error) {
	return nil, nil
}

func (e *strategyRuntimeStubExchange) CancelOrders(context.Context, ...bbgotypes.Order) error {
	return nil
}

func (e *strategyRuntimeStubExchange) QueryBrokerFunds(context.Context, broker.ReadQuery) (*broker.FundsSnapshot, error) {
	if e.queryFundsErr != nil {
		return nil, e.queryFundsErr
	}
	copyValue := *e.funds
	copyValue.CurrencyBalances = append([]broker.CurrencyBalanceSnapshot(nil), e.funds.CurrencyBalances...)
	return &copyValue, nil
}

func (e *strategyRuntimeStubExchange) QueryBrokerPositions(context.Context, broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	if e.queryPositionsErr != nil {
		return nil, e.queryPositionsErr
	}
	return append([]broker.PositionSnapshot(nil), e.positions...), nil
}

func (e *strategyRuntimeStubExchange) PlaceBrokerOrder(_ context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.panicOnPlaceOrder {
		panic("broker submit panic")
	}
	e.nextOrderID++
	submitOrder := bbgotypes.SubmitOrder{
		Symbol:   query.Symbol,
		Side:     bbgotypes.SideType(query.Side),
		Type:     bbgotypes.OrderType(query.OrderType),
		Quantity: fixedpoint.NewFromFloat(query.Quantity),
	}
	if query.Price != nil {
		submitOrder.Price = fixedpoint.NewFromFloat(*query.Price)
	}
	if query.TimeInForce != nil {
		submitOrder.TimeInForce = bbgotypes.TimeInForce(*query.TimeInForce)
	}
	e.placedOrders = append(e.placedOrders, submitOrder)
	market := strings.ToUpper(strings.TrimSpace(query.Market))
	if market == "" {
		market = strategyRuntimeMarketFromSymbol(query.Symbol, "")
	}
	return &broker.PlaceOrderResult{
		AccountID:          "123456",
		TradingEnvironment: "SIMULATE",
		Market:             market,
		BrokerOrderID:      fmt.Sprintf("%d", e.nextOrderID),
		Status:             "SUBMITTED",
	}, nil
}

func (e *strategyRuntimeStubExchange) placedOrderCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.placedOrders)
}

func (e *strategyRuntimeStubExchange) lastPlacedOrder() (bbgotypes.SubmitOrder, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.placedOrders) == 0 {
		return bbgotypes.SubmitOrder{}, false
	}
	return e.placedOrders[len(e.placedOrders)-1], true
}

func instantiateStrategyRuntimeTestInstance(t *testing.T, server *Server, binding strategyInstanceBinding) string {
	t.Helper()
	definition := strategyDesignDefinition{
		ID:           "runtime-test",
		Name:         "Runtime Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Test\", overlay=true)\nstrategy.entry(\"Long\", strategy.long, qty=10)",
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, binding)
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	return instance.ID
}

type fakeStrategyRuntimePineWorker struct {
	mu       sync.Mutex
	requests []pineworker.RunScriptRequest
	err      error
	response func(pineworker.RunScriptRequest) pineworker.RunScriptResponse
}

func newFakeStrategyRuntimePineWorker() *fakeStrategyRuntimePineWorker {
	return &fakeStrategyRuntimePineWorker{
		response: func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
			lastIndex := len(request.Candles) - 1
			if lastIndex < 0 {
				return pineworker.RunScriptResponse{JobID: request.JobID}
			}
			return pineworker.RunScriptResponse{
				JobID: request.JobID,
				OrderIntents: []pineworker.OrderIntent{{
					Kind:        "entry",
					ID:          "Long",
					Direction:   "long",
					Quantity:    10,
					HasQuantity: true,
					BarIndex:    lastIndex,
					Time:        request.Candles[lastIndex].OpenTime,
				}},
			}
		},
	}
}

func (worker *fakeStrategyRuntimePineWorker) RunScript(_ context.Context, request pineworker.RunScriptRequest) (pineworker.RunScriptResponse, error) {
	worker.mu.Lock()
	worker.requests = append(worker.requests, request)
	err := worker.err
	response := worker.response
	worker.mu.Unlock()
	if err != nil {
		return pineworker.RunScriptResponse{}, err
	}
	if response == nil {
		return pineworker.RunScriptResponse{JobID: request.JobID}, nil
	}
	return response(request), nil
}

func (worker *fakeStrategyRuntimePineWorker) lastRequest() (pineworker.RunScriptRequest, bool) {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	if len(worker.requests) == 0 {
		return pineworker.RunScriptRequest{}, false
	}
	return worker.requests[len(worker.requests)-1], true
}

func useFakeStrategyRuntimePineWorker(server *Server, worker *fakeStrategyRuntimePineWorker) {
	server.strategyRuntimeManager.pineWorkerRunner = worker
}

func strategyRuntimeHistoricalKLine(symbol string, interval bbgotypes.Interval, closePrice float64, start time.Time) bbgotypes.KLine {
	end := start.Add(interval.Duration()).Add(-time.Millisecond)
	price := fixedpoint.NewFromFloat(closePrice)
	return bbgotypes.KLine{
		Exchange:    bbgotypes.ExchangeName("futu"),
		Symbol:      symbol,
		StartTime:   bbgotypes.Time(start),
		EndTime:     bbgotypes.Time(end),
		Interval:    interval,
		Open:        price,
		Close:       price,
		High:        price,
		Low:         price,
		Volume:      fixedpoint.NewFromFloat(100),
		QuoteVolume: fixedpoint.NewFromFloat(closePrice * 100),
		Closed:      true,
	}
}

func strategyRuntimeTestTrade(symbol string, price float64, at time.Time) bbgotypes.Trade {
	return bbgotypes.Trade{
		ID:            uint64(at.Unix()),
		Symbol:        symbol,
		Side:          bbgotypes.SideTypeBuy,
		Price:         fixedpoint.NewFromFloat(price),
		Quantity:      fixedpoint.NewFromFloat(1),
		QuoteQuantity: fixedpoint.NewFromFloat(price),
		Time:          bbgotypes.Time(at),
	}
}

func strategyRuntimeTestTime(hour int, minute int, second int) time.Time {
	return time.Date(2026, time.May, 28, hour, minute, second, 0, time.UTC)
}
