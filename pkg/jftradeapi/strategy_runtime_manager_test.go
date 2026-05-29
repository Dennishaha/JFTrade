package jftradeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

type strategyRuntimeStubExchange struct {
	mu                sync.Mutex
	markets           bbgotypes.MarketMap
	history           map[string][]bbgotypes.KLine
	funds             *futu.BrokerFundsSnapshot
	positions         []futu.BrokerPositionSnapshot
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
		funds: &futu.BrokerFundsSnapshot{
			AccountID:               "123456",
			TradingEnvironment:      "SIMULATE",
			Market:                  "US",
			TotalAssets:             floatPtr(100000),
			AvailableFunds:          floatPtr(100000),
			AvailableWithdrawalCash: floatPtr(100000),
			CurrencyBalances: []futu.BrokerCurrencyBalanceSnapshot{{
				AccountID:               "123456",
				TradingEnvironment:      "SIMULATE",
				Currency:                "USD",
				Cash:                    floatPtr(100000),
				NetCashPower:            floatPtr(100000),
				AvailableWithdrawalCash: floatPtr(100000),
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
	stream := bbgotypes.NewStandardStream()
	return &stream
}

func (e *strategyRuntimeStubExchange) QueryMarkets(context.Context) (bbgotypes.MarketMap, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make(bbgotypes.MarketMap, len(e.markets))
	for symbol, market := range e.markets {
		result[symbol] = market
	}
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
	ticker := tickerMap[symbol]
	return &ticker, nil
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

func (e *strategyRuntimeStubExchange) QueryBrokerFunds(context.Context, futu.BrokerReadQuery) (*futu.BrokerFundsSnapshot, error) {
	if e.queryFundsErr != nil {
		return nil, e.queryFundsErr
	}
	copyValue := *e.funds
	copyValue.CurrencyBalances = append([]futu.BrokerCurrencyBalanceSnapshot(nil), e.funds.CurrencyBalances...)
	return &copyValue, nil
}

func (e *strategyRuntimeStubExchange) QueryBrokerPositions(context.Context, futu.BrokerReadQuery) ([]futu.BrokerPositionSnapshot, error) {
	if e.queryPositionsErr != nil {
		return nil, e.queryPositionsErr
	}
	return append([]futu.BrokerPositionSnapshot(nil), e.positions...), nil
}

func (e *strategyRuntimeStubExchange) PlaceBrokerOrder(_ context.Context, query futu.BrokerPlaceOrderQuery, submitOrder bbgotypes.SubmitOrder) (*futu.BrokerPlaceOrderResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.panicOnPlaceOrder {
		panic("broker submit panic")
	}
	e.nextOrderID++
	e.placedOrders = append(e.placedOrders, submitOrder)
	now := time.Now().UTC()
	order := bbgotypes.Order{
		SubmitOrder:    submitOrder,
		Exchange:       e.Name(),
		OrderID:        e.nextOrderID,
		OriginalStatus: "SUBMITTED",
		CreationTime:   bbgotypes.Time(now),
		UpdateTime:     bbgotypes.Time(now),
	}
	market := strings.ToUpper(strings.TrimSpace(query.Market))
	if market == "" {
		market = strategyRuntimeMarketFromSymbol(submitOrder.Symbol, "")
	}
	return &futu.BrokerPlaceOrderResult{
		AccountID:          "123456",
		TradingEnvironment: "SIMULATE",
		Market:             market,
		Order:              order,
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

func TestStrategyRuntimeNotifyOnlyEmitsSignalNotification(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	notifications := server.liveNotificationsAfter(0)
	if len(notifications) == 0 {
		t.Fatalf("expected strategy signal notification")
	}
	found := false
	for _, note := range notifications {
		if note.Title != "策略下单信号" {
			continue
		}
		found = true
		if !strings.Contains(note.Message, "买入 10 股") || !strings.Contains(note.Message, "仅通知模式") {
			t.Fatalf("unexpected signal message: %+v", note)
		}
	}
	if !found {
		t.Fatalf("expected 策略下单信号 notification, got %+v", notifications)
	}
	if got := len(server.executionOrders.listOrders().Orders); got != 0 {
		t.Fatalf("notify_only should not place execution orders, got %d", got)
	}
	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	signalAudited := false
	for _, entry := range audit.Entries {
		if entry.Kind == "signal_notified" {
			signalAudited = true
			break
		}
	}
	if !signalAudited {
		t.Fatalf("expected signal_notified audit entry, got %+v", audit.Entries)
	}
}

func TestStrategyRuntimeLiveModeRecordsExecutionOrder(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 1 {
		t.Fatalf("expected 1 broker order, got %d", got)
	}
	placedOrder, ok := stub.lastPlacedOrder()
	if !ok {
		t.Fatal("expected placed order")
	}
	if placedOrder.TimeInForce != bbgotypes.TimeInForce("DAY") {
		t.Fatalf("expected live strategy order timeInForce DAY, got %q", placedOrder.TimeInForce)
	}
	orders := server.executionOrders.listOrders().Orders
	if len(orders) != 1 {
		t.Fatalf("expected 1 execution order, got %+v", orders)
	}
	if orders[0].Symbol == nil || *orders[0].Symbol != "US.AAPL" {
		t.Fatalf("unexpected execution order symbol: %+v", orders[0])
	}
	notifications := server.liveNotificationsAfter(0)
	found := false
	for _, note := range notifications {
		if note.Title == "Futu 订单已提交" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected execution placed notification, got %+v", notifications)
	}
	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	foundSubmitted := false
	for _, entry := range audit.Entries {
		if entry.Kind == "order_submitted" {
			foundSubmitted = true
			break
		}
	}
	if !foundSubmitted {
		t.Fatalf("expected order_submitted audit entry, got %+v", audit.Entries)
	}
}

func TestStrategyRuntimeRefreshesBrokerPositionsBeforeSellOnKLineClose(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	definition := strategyDesignDefinition{
		ID:           "runtime-sell-test",
		Name:         "Runtime Sell Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy Runtime Sell Test\non kline_close:\n  sell shares 1 policy same_direction type MARKET",
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.strategyStore.strategy(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instance.ID)

	stub.positions = []futu.BrokerPositionSnapshot{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         1,
		SellableQuantity: 1,
	}}

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 1 {
		t.Fatalf("expected 1 broker order after runtime position refresh, got %d", got)
	}
	orders := server.executionOrders.listOrders().Orders
	if len(orders) != 1 {
		t.Fatalf("expected 1 execution order, got %+v", orders)
	}
	if orders[0].Side == nil || *orders[0].Side != "SELL" {
		t.Fatalf("expected SELL execution order, got %+v", orders[0])
	}
	if orders[0].RequestedQuantity == nil || *orders[0].RequestedQuantity != 1 {
		t.Fatalf("expected quantity 1 execution order, got %+v", orders[0])
	}
	if runtime := server.strategyRuntimeManager.runtime(instance.ID); runtime == nil {
		t.Fatalf("expected active runtime for %s", instance.ID)
	}
	audit, ok := server.strategyStore.strategyAudit(instance.ID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instance.ID)
	}
	foundSubmitted := false
	for _, entry := range audit.Entries {
		if entry.Kind == "order_submitted" {
			foundSubmitted = true
			break
		}
	}
	if !foundSubmitted {
		t.Fatalf("expected order_submitted audit entry, got %+v", audit.Entries)
	}
}

func TestStrategyRuntimeDisconnectedBrokerRefreshKeepsCachedState(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	stub.positions = []futu.BrokerPositionSnapshot{{
		Symbol:           "US.AAPL",
		Quantity:         1,
		SellableQuantity: 1,
	}}
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	definition := strategyDesignDefinition{
		ID:           "runtime-disconnected-refresh-test",
		Name:         "Runtime Disconnected Refresh Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy Runtime Disconnected Refresh Test\non kline_close:\n  sell shares 1 policy same_direction type MARKET",
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.strategyStore.strategy(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instance.ID)

	stub.queryFundsErr = opend.ErrClosed
	stub.queryPositionsErr = opend.ErrClosed

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 1 {
		t.Fatalf("expected cached position to allow 1 broker order, got %d", got)
	}
	audit, ok := server.strategyStore.strategyAudit(instance.ID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instance.ID)
	}
	for _, entry := range audit.Entries {
		if entry.Kind == "runtime_error" && strings.Contains(entry.Detail, "client closed") {
			t.Fatalf("expected disconnected refresh to avoid runtime_error audit entry, got %+v", audit.Entries)
		}
	}
	observation, ok := server.strategyRuntimeManager.runtimeObservation(instance.ID)
	if !ok {
		t.Fatalf("expected runtime observation for %s", instance.ID)
	}
	if observation.LastError != nil {
		t.Fatalf("expected runtime observation without lastError after disconnected refresh, got %+v", observation)
	}
}

func TestStrategyRuntimePollsClosedKLinesWhenTradePushStalls(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	stub.positions = []futu.BrokerPositionSnapshot{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         3,
		SellableQuantity: 3,
	}}
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	originalInterval := strategyRuntimeClosedKLineSyncInterval
	strategyRuntimeClosedKLineSyncInterval = 10 * time.Millisecond
	defer func() {
		strategyRuntimeClosedKLineSyncInterval = originalInterval
	}()

	definition := strategyDesignDefinition{
		ID:           "runtime-poll-kline-test",
		Name:         "Runtime Poll KLine Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy Runtime Poll KLine Test\non kline_close:\n  sell shares 1 policy same_direction type MARKET",
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.strategyStore.strategy(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instance.ID)

	stub.appendHistory(
		"US.AAPL",
		strategyRuntimeHistoricalKLine("US.AAPL", "1m", 100, strategyRuntimeTestTime(10, 0, 0)),
		strategyRuntimeHistoricalKLine("US.AAPL", "1m", 101, strategyRuntimeTestTime(10, 1, 0)),
		strategyRuntimeHistoricalKLine("US.AAPL", "1m", 102, strategyRuntimeTestTime(10, 2, 0)),
	)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stub.placedOrderCount() >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := stub.placedOrderCount(); got != 3 {
		t.Fatalf("expected 3 broker orders from polled closed klines, got %d", got)
	}
	observation, ok := server.strategyRuntimeManager.runtimeObservation(instance.ID)
	if !ok {
		t.Fatalf("expected runtime observation for %s", instance.ID)
	}
	if observation.LastClosedKLineAt == nil || observation.LastOrderAt == nil {
		t.Fatalf("expected runtime observation to record polled kline/order timestamps, got %+v", observation)
	}
}

func TestStrategyRuntimeStartEnsuresMissingMarketMetadata(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.TME"},
		Interval:      "5m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}

	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy should inject missing market metadata: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	observation, ok := server.strategyRuntimeManager.runtimeObservation(instanceID)
	if !ok {
		t.Fatalf("expected runtime observation after start")
	}
	if len(observation.ActiveSymbols) != 1 || observation.ActiveSymbols[0] != "US.TME" {
		t.Fatalf("unexpected active symbols: %+v", observation.ActiveSymbols)
	}
	if _, ok := stub.markets["US.TME"]; !ok {
		t.Fatalf("expected EnsureMarket to inject US.TME into market map")
	}
}

func TestStrategyRuntimeObservationAppearsInStrategiesAndSystemStatus(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.strategyRuntimeManager.stopStrategy(instanceID)

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	srv := httptest.NewServer(server)
	defer srv.Close()

	strategiesResp, err := http.Get(srv.URL + "/api/v1/strategies")
	if err != nil {
		t.Fatalf("GET strategies: %v", err)
	}
	defer strategiesResp.Body.Close()
	var strategiesEnvelope struct {
		OK   bool               `json:"ok"`
		Data []strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(strategiesResp.Body).Decode(&strategiesEnvelope); err != nil {
		t.Fatalf("decode strategies response: %v", err)
	}
	if len(strategiesEnvelope.Data) != 1 {
		t.Fatalf("expected 1 strategy item, got %+v", strategiesEnvelope.Data)
	}
	observation := strategiesEnvelope.Data[0].RuntimeObservation
	if observation == nil {
		t.Fatalf("expected runtime observation in strategies response, got %+v", strategiesEnvelope.Data[0])
	}
	if observation.ActualStatus != strategyStatusRunning {
		t.Fatalf("actualStatus = %s, want %s", observation.ActualStatus, strategyStatusRunning)
	}
	if len(observation.ActiveSymbols) != 1 || observation.ActiveSymbols[0] != "US.AAPL" {
		t.Fatalf("unexpected activeSymbols: %+v", observation.ActiveSymbols)
	}
	if observation.LastClosedKLineAt == nil || observation.LastSignalAt == nil {
		t.Fatalf("expected lastClosedKlineAt and lastSignalAt, got %+v", observation)
	}
	if observation.LastOrderAt != nil {
		t.Fatalf("notify_only should not have lastOrderAt, got %+v", observation.LastOrderAt)
	}

	statusResp, err := http.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET system status: %v", err)
	}
	defer statusResp.Body.Close()
	var statusEnvelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(statusResp.Body).Decode(&statusEnvelope); err != nil {
		t.Fatalf("decode system status response: %v", err)
	}
	strategyRuntime, ok := statusEnvelope.Data["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", statusEnvelope.Data["strategyRuntime"])
	}
	if got := int(strategyRuntime["activeStrategies"].(float64)); got != 1 {
		t.Fatalf("activeStrategies = %d, want 1", got)
	}
	activeInstances, ok := strategyRuntime["activeInstances"].([]any)
	if !ok || len(activeInstances) != 1 {
		t.Fatalf("expected 1 active runtime instance, got %+v", strategyRuntime["activeInstances"])
	}
	activeInstance, ok := activeInstances[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected active instance summary: %+v", activeInstances[0])
	}
	if activeInstance["instanceId"] != instanceID {
		t.Fatalf("unexpected active instance id: %+v", activeInstance)
	}
	if activeInstance["lastClosedKlineAt"] == nil || activeInstance["lastSignalAt"] == nil {
		t.Fatalf("expected runtime timestamps in active instance summary, got %+v", activeInstance)
	}
}

func TestStrategyRuntimeObservationPersistsAcrossServerRestart(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusStopped, "stopped", "test stop"); err != nil {
		t.Fatalf("transitionStrategy stop: %v", err)
	}
	server.strategyRuntimeManager.stopStrategy(instanceID)

	reloadedStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore reload: %v", err)
	}
	reloadedServer := NewServer(reloadedStore)
	strategies := reloadedServer.enrichStrategyItems(reloadedServer.strategyStore.strategies())
	if len(strategies) != 1 {
		t.Fatalf("expected 1 strategy after reload, got %+v", strategies)
	}
	observation := strategies[0].RuntimeObservation
	if observation == nil {
		t.Fatalf("expected persisted runtime observation after reload, got %+v", strategies[0])
	}
	if observation.ActualStatus != strategyStatusStopped {
		t.Fatalf("persisted actual status = %s, want %s", observation.ActualStatus, strategyStatusStopped)
	}
	if observation.LastClosedKLineAt == nil || observation.LastSignalAt == nil {
		t.Fatalf("expected persisted timestamps after reload, got %+v", observation)
	}
	strategyRuntime, ok := reloadedServer.systemStatus()["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", reloadedServer.systemStatus()["strategyRuntime"])
	}
	if got := strategyRuntime["activeStrategies"].(int); got != 0 {
		t.Fatalf("activeStrategies after reload = %d, want 0", got)
	}
}

func TestStrategyRuntimePanicAutoReconcilesToStopped(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	stub.panicOnPlaceOrder = true
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.strategyRuntimeManager.startStrategy(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	strategy, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found after panic reconciliation", instanceID)
	}
	if strategy.Status != strategyStatusStopped {
		t.Fatalf("strategy status after panic = %s, want %s", strategy.Status, strategyStatusStopped)
	}
	if got := len(server.strategyRuntimeManager.activeInstrumentIDs()); got != 0 {
		t.Fatalf("expected runtime manager to remove failed runtime, got %d active instruments", got)
	}

	notifications := server.liveNotificationsAfter(0)
	foundNotification := false
	for _, note := range notifications {
		if note.Title == "策略运行异常退出" {
			foundNotification = true
			break
		}
	}
	if !foundNotification {
		t.Fatalf("expected runtime exit notification, got %+v", notifications)
	}

	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	foundExitAudit := false
	for _, entry := range audit.Entries {
		if entry.Kind == "runtime_exited" && strings.Contains(entry.Detail, "broker submit panic") {
			foundExitAudit = true
			break
		}
	}
	if !foundExitAudit {
		t.Fatalf("expected runtime_exited audit entry, got %+v", audit.Entries)
	}

	strategyRuntime, ok := server.systemStatus()["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", server.systemStatus()["strategyRuntime"])
	}
	if got := strategyRuntime["activeStrategies"].(int); got != 0 {
		t.Fatalf("activeStrategies after panic = %d, want 0", got)
	}
}

func instantiateStrategyRuntimeTestInstance(t *testing.T, server *Server, binding strategyInstanceBinding) string {
	t.Helper()
	definition := strategyDesignDefinition{
		ID:           "runtime-test",
		Name:         "Runtime Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy Runtime Test\non kline_close:\n  buy shares 10",
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, binding)
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	return instance.ID
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

func floatPtr(value float64) *float64 {
	return &value
}
