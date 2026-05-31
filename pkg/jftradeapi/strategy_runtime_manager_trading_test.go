package jftradeapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

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
	if placedOrder.TimeInForce != "DAY" {
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

	stub.positions = []broker.PositionSnapshot{{
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
	stub.positions = []broker.PositionSnapshot{{
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
