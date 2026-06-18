package servercore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategyRuntimeLiveModeRecordsExecutionOrder(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
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

func TestStrategyRuntimeRiskCloseOnlyRejectsBuyOrder(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
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
	if _, err := server.strategyStore.updateStrategyRuntimeRisk(instanceID, strategyRuntimeRiskSettings{
		Mode:      "enforce",
		CloseOnly: true,
	}); err != nil {
		t.Fatalf("updateStrategyRuntimeRisk: %v", err)
	}

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 0 {
		t.Fatalf("expected runtime risk to reject broker order, got %d", got)
	}
	if orders := server.executionOrders.listOrders().Orders; len(orders) != 0 {
		t.Fatalf("expected no execution order after risk rejection, got %+v", orders)
	}
	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	foundRejected := false
	for _, entry := range audit.Entries {
		if entry.Kind == "risk_rejected" && strings.Contains(entry.Detail, "rule=close_only") {
			foundRejected = true
			break
		}
	}
	if !foundRejected {
		t.Fatalf("expected risk_rejected audit entry, got %+v", audit.Entries)
	}
}

func TestStrategyRuntimeRiskEvaluatesOrderLimits(t *testing.T) {
	executor := &strategyLiveOrderExecutor{
		instance: managedStrategyInstance{
			Binding: strategyInstanceBinding{
				RuntimeRisk: strategyRuntimeRiskSettings{
					Mode:             "enforce",
					CloseOnly:        true,
					MaxOrderQuantity: new(5.0),
					MaxOrderNotional: new(500.0),
				},
			},
		},
		runner: &strategySymbolRuntime{
			lastClosedPrice: 100,
			cachedPositions: []broker.PositionSnapshot{{
				Market:           "US",
				Symbol:           "AAPL",
				Quantity:         4,
				SellableQuantity: 4,
			}},
		},
	}

	tests := []struct {
		name     string
		side     string
		quantity float64
		price    *float64
		want     string
	}{
		{name: "buy blocked by close only", side: "BUY", quantity: 1, want: "close_only"},
		{name: "sell exceeds position", side: "SELL", quantity: 5, want: "close_only_insufficient_position"},
		{name: "sell exceeds quantity", side: "SELL", quantity: 6, want: "close_only_insufficient_position"},
		{name: "sell exceeds notional", side: "SELL", quantity: 4, price: new(float64(130)), want: "max_order_notional"},
		{name: "sell allowed", side: "SELL", quantity: 4, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := executor.evaluateRuntimeRisk(trdsrv.ExecutionOrderCommand{
				Symbol: "US.AAPL",
				Side:   test.side,
				Query: broker.PlaceOrderQuery{
					Symbol:   "US.AAPL",
					Side:     test.side,
					Quantity: test.quantity,
					Price:    test.price,
				},
			})
			if decision.reason != test.want {
				t.Fatalf("reason = %q, want %q", decision.reason, test.want)
			}
		})
	}
	executor.instance.Binding.RuntimeRisk.CloseOnly = false
	quantityDecision := executor.evaluateRuntimeRisk(trdsrv.ExecutionOrderCommand{
		Symbol: "US.AAPL",
		Side:   "BUY",
		Query: broker.PlaceOrderQuery{
			Symbol:   "US.AAPL",
			Side:     "BUY",
			Quantity: 6,
		},
	})
	if quantityDecision.reason != "max_order_quantity" {
		t.Fatalf("quantity decision reason = %q, want max_order_quantity", quantityDecision.reason)
	}
}

func TestStrategyRuntimeUsesStrategyDefaultPercentQuantity(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	definition := strategyDesignDefinition{
		ID:           "runtime-default-qty-test",
		Name:         "Runtime Default Qty Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Default Qty Test\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\nstrategy.entry(\"Long\", strategy.long)",
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

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 500, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 501, strategyRuntimeTestTime(10, 1, 0)))

	placedOrder, ok := stub.lastPlacedOrder()
	if !ok {
		t.Fatal("expected placed order")
	}
	if got := placedOrder.Quantity.Float64(); got != 20 {
		t.Fatalf("expected default 10%% equity quantity 20, got %v", got)
	}
}

func TestStrategyRuntimePyramidingLimitsSameDirectionEntries(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	stub.positions = []broker.PositionSnapshot{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         1,
		SellableQuantity: 1,
	}}
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	definition := strategyDesignDefinition{
		ID:           "runtime-pyramiding-test",
		Name:         "Runtime Pyramiding Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Pyramiding Test\", overlay=true, pyramiding=2)\nstrategy.entry(\"Long\", strategy.long, qty=1)",
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

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 102, strategyRuntimeTestTime(10, 2, 0)))

	if got := stub.placedOrderCount(); got != 1 {
		t.Fatalf("expected pyramiding=2 to allow one additional long entry and skip the third signal, got %d orders", got)
	}
}

func TestStrategyRuntimeDefaultPyramidingSkipsExistingSameDirectionPosition(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	stub.positions = []broker.PositionSnapshot{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         1,
		SellableQuantity: 1,
	}}
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	definition := strategyDesignDefinition{
		ID:           "runtime-default-pyramiding-test",
		Name:         "Runtime Default Pyramiding Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Default Pyramiding Test\", overlay=true)\nstrategy.entry(\"Long\", strategy.long, qty=1)",
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

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 0 {
		t.Fatalf("expected default pyramiding to skip existing long position, got %d orders", got)
	}
}

func TestStrategyRuntimeRefreshesBrokerPositionsBeforeSellOnKLineClose(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	definition := strategyDesignDefinition{
		ID:           "runtime-sell-test",
		Name:         "Runtime Sell Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Sell Test\", overlay=true)\nstrategy.close(\"Long\")",
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
	server := newTestServer(t, store)
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
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Disconnected Refresh Test\", overlay=true)\nstrategy.close(\"Long\")",
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
