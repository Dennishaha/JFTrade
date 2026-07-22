package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestStrategyRuntimeOrderUsesSharedPreTradeRiskGateway(t *testing.T) {
	brokerCalled := false
	server := &Server{}
	server.tradingSvc = trdsrv.NewService(
		trdsrv.WithPreTradeRiskGateway(trdsrv.NewStaticPreTradeRiskGateway(func() trdsrv.PreTradeRiskConfig {
			return trdsrv.PreTradeRiskConfig{}
		})),
		trdsrv.WithPlaceOrder(func(context.Context, trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
			brokerCalled = true
			return trdsrv.ExecutionOrder{}, nil
		}),
	)
	deps := newStrategyRuntimeManagerDeps(server)
	price := 100.0
	_, err := deps.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{
		Symbol: "US.AAPL",
		Query: broker.PlaceOrderQuery{
			ReadQuery: broker.ReadQuery{TradingEnvironment: "REAL", Market: "US"},
			Quantity:  1,
			Price:     &price,
		},
	})
	var rejected trdsrv.RiskRejectedError
	if !errors.As(err, &rejected) || rejected.Decision.ReasonCode != "REAL_TRADING_DISABLED" {
		t.Fatalf("strategy order error = %v, want shared pre-trade rejection", err)
	}
	if brokerCalled {
		t.Fatal("strategy order bypassed shared pre-trade risk gateway")
	}
}

func TestStrategyRuntimeLiveModeRecordsExecutionOrder(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)

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
		if note.Title == "FUTU 订单已提交" {
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

func TestStrategyRuntimeLiveOrderPassesStopPriceToExecutionGateway(t *testing.T) {
	var captured trdsrv.ExecutionOrderCommand
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{},
		deps: strategyRuntimeManagerDeps{
			placeExecutionOrder: func(_ context.Context, command trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
				captured = command
				return trdsrv.ExecutionOrder{InternalOrderID: "internal-stop"}, nil
			},
			appendRuntimeEvent: func(string, string, string, string) error { return nil },
		},
	}
	executor := &strategyLiveOrderExecutor{
		manager: manager,
		instance: managedStrategyInstance{
			ID: "stop-instance",
			Binding: strategyInstanceBinding{
				RuntimeRisk: strategyRuntimeRiskSettings{Mode: "off"},
			},
		},
		runner: &strategySymbolRuntime{lastClosedPrice: 100},
	}
	stopPrice := fixedpoint.NewFromFloat(95.25)
	orders, err := executor.SubmitOrders(t.Context(), bbgotypes.SubmitOrder{
		ClientOrderID: "stop-order",
		Symbol:        "US.AAPL",
		Side:          bbgotypes.SideTypeSell,
		Type:          bbgotypes.OrderTypeStopMarket,
		Quantity:      fixedpoint.NewFromFloat(1),
		StopPrice:     stopPrice,
		ReduceOnly:    true,
	})
	if err != nil || len(orders) != 1 {
		t.Fatalf("SubmitOrders = %#v, %v", orders, err)
	}
	if captured.OrderType != string(bbgotypes.OrderTypeStopMarket) || captured.Query.OrderType != string(bbgotypes.OrderTypeStopMarket) {
		t.Fatalf("execution order types = %q/%q", captured.OrderType, captured.Query.OrderType)
	}
	if captured.Query.StopPrice == nil || *captured.Query.StopPrice != stopPrice.Float64() {
		t.Fatalf("execution stop price = %#v, want %v", captured.Query.StopPrice, stopPrice.Float64())
	}
	if captured.Query.Price != nil {
		t.Fatalf("stop-market limit price = %#v, want nil", captured.Query.Price)
	}
	if !captured.Query.ReduceOnly {
		t.Fatal("execution reduce-only flag = false, want true")
	}
}

func TestStrategyRuntimeRiskCloseOnlyRejectsBuyOrder(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)

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
			if decision.Reason != test.want {
				t.Fatalf("reason = %q, want %q", decision.Reason, test.want)
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
	if quantityDecision.Reason != "max_order_quantity" {
		t.Fatalf("quantity decision reason = %q, want max_order_quantity", quantityDecision.Reason)
	}
}

func TestStrategyRuntimeLiveSizesEntryQuantityPctFromEquity(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{{
			Kind: "entry", ID: "SizedLong", Direction: "long", QuantityPct: 50, HasQuantityPct: true, BarIndex: lastIndex, Time: request.Candles[lastIndex].OpenTime,
		}}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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
	if got := placedOrder.Quantity.Float64(); got != 100 {
		t.Fatalf("expected equity-sized quantity 100, got %v", got)
	}
}

func TestStrategyRuntimeLiveUsesExplicitQuantityBeforeQuantityPct(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{{
			Kind: "entry", ID: "ExplicitLong", Direction: "long", Quantity: 20, HasQuantity: true, QuantityPct: 50, HasQuantityPct: true, BarIndex: lastIndex, Time: request.Candles[lastIndex].OpenTime,
		}}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 500, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 501, strategyRuntimeTestTime(10, 1, 0)))

	placedOrder, ok := stub.lastPlacedOrder()
	if !ok {
		t.Fatal("expected placed order")
	}
	if got := placedOrder.Quantity.Float64(); got != 20 {
		t.Fatalf("expected explicit quantity 20, got %v", got)
	}
}

func TestStrategyRuntimeLiveSizesCloseQuantityPctFromPosition(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{{
			Kind: "close", ID: "HalfFlat", Direction: "long", QuantityPct: 50, HasQuantityPct: true, BarIndex: lastIndex, Time: request.Candles[lastIndex].OpenTime,
		}}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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

	stub.positions = []broker.PositionSnapshot{{Market: "US", Symbol: "AAPL", Quantity: 20, SellableQuantity: 20}}
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	placedOrder, ok := stub.lastPlacedOrder()
	if !ok {
		t.Fatal("expected placed order")
	}
	if placedOrder.Side != "SELL" || placedOrder.Quantity.Float64() != 10 {
		t.Fatalf("expected SELL 10 close order, got %+v", placedOrder)
	}
}

func TestStrategyRuntimeLiveDefaultsCloseToFullPosition(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{{
			Kind: "close", ID: "FullFlat", Direction: "long", BarIndex: lastIndex, Time: request.Candles[lastIndex].OpenTime,
		}}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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

	stub.positions = []broker.PositionSnapshot{{Market: "US", Symbol: "AAPL", Quantity: 7, SellableQuantity: 7}}
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	placedOrder, ok := stub.lastPlacedOrder()
	if !ok {
		t.Fatal("expected placed order")
	}
	if placedOrder.Side != "SELL" || placedOrder.Quantity.Float64() != 7 {
		t.Fatalf("expected SELL 7 close order, got %+v", placedOrder)
	}
}

func TestStrategyRuntimeLiveIgnoredOrderRecordsRuntimeEvidence(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	market := stub.markets["US.AAPL"]
	market.MinQuantity = fixedpoint.NewFromFloat(100)
	market.StepSize = fixedpoint.NewFromFloat(100)
	stub.markets["US.AAPL"] = market
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{{
			Kind: "entry", ID: "TinyLong", Direction: "long", QuantityPct: 1, HasQuantityPct: true, BarIndex: lastIndex, Time: request.Candles[lastIndex].OpenTime,
		}}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 1000, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 1001, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 0 {
		t.Fatalf("expected tiny order to be ignored, got %d broker orders", got)
	}
	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	foundIgnored := false
	for _, entry := range audit.Entries {
		if entry.Kind == "order_ignored" && strings.Contains(entry.Detail, "below") {
			foundIgnored = true
			break
		}
	}
	if !foundIgnored {
		t.Fatalf("expected order_ignored audit entry, got %+v", audit.Entries)
	}
}

func TestStrategyRuntimeLiveCancelsTrackedOrderFromWorkerCommand(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		openTime := request.Candles[lastIndex].OpenTime
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{
			{Kind: "entry", ID: "Breakout", Direction: "long", Quantity: 1, HasQuantity: true, LimitPrice: 105, HasLimitPrice: true, BarIndex: lastIndex, Time: openTime},
			{Kind: "cancel", ID: "Breakout", BarIndex: lastIndex, Time: openTime},
		}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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

	orders := server.executionOrders.listOrders().Orders
	if len(orders) != 1 {
		t.Fatalf("expected one tracked execution order, got %+v", orders)
	}
	if orders[0].Status != "CANCEL_REQUESTED" {
		t.Fatalf("order status = %q, want CANCEL_REQUESTED", orders[0].Status)
	}
	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	foundCancel := false
	for _, entry := range audit.Entries {
		if entry.Kind == "order_cancel_requested" && strings.Contains(entry.Detail, orders[0].InternalOrderID) {
			foundCancel = true
			break
		}
	}
	if !foundCancel {
		t.Fatalf("expected order_cancel_requested audit entry, got %+v", audit.Entries)
	}
}

func TestStrategyRuntimeLiveCancelAllCancelsOnlyTrackedOrders(t *testing.T) {
	cancelled := []string{}
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{},
		deps: strategyRuntimeManagerDeps{
			cancelExecutionOrder: func(_ context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
				cancelled = append(cancelled, internalOrderID)
				return trdsrv.ExecutionOrder{InternalOrderID: internalOrderID}, nil
			},
			appendRuntimeEvent: func(string, string, string, string) error { return nil },
		},
	}
	executor := &strategyLiveOrderExecutor{
		manager:  manager,
		instance: managedStrategyInstance{ID: "instance-a"},
	}
	executor.trackOrder("owned-1", "internal-1")
	executor.trackOrder("owned-2", "internal-2")

	err := executor.CancelOrders(context.Background(),
		bbgoOrderForCancel("owned-1"),
		bbgoOrderForCancel("untracked"),
		bbgoOrderForCancel("owned-2"),
	)
	if err != nil {
		t.Fatalf("CancelOrders: %v", err)
	}
	if strings.Join(cancelled, ",") != "internal-1,internal-2" {
		t.Fatalf("cancelled = %#v, want only tracked orders", cancelled)
	}
	if _, ok := executor.trackedInternalOrderID("owned-1"); ok {
		t.Fatal("owned-1 remained tracked after successful cancel")
	}
	if _, ok := executor.trackedInternalOrderID("owned-2"); ok {
		t.Fatal("owned-2 remained tracked after successful cancel")
	}
}

func TestStrategyRuntimeLiveCancelFailureKeepsTrackedOrder(t *testing.T) {
	cancelErr := errors.New("cancel failed")
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{},
		deps: strategyRuntimeManagerDeps{
			cancelExecutionOrder: func(context.Context, string) (trdsrv.ExecutionOrder, error) {
				return trdsrv.ExecutionOrder{}, cancelErr
			},
			appendRuntimeEvent: func(string, string, string, string) error { return nil },
		},
	}
	executor := &strategyLiveOrderExecutor{
		manager:  manager,
		instance: managedStrategyInstance{ID: "instance-a"},
	}
	executor.trackOrder("owned", "internal-1")

	if err := executor.CancelOrders(context.Background(), bbgoOrderForCancel("owned")); !errors.Is(err, cancelErr) {
		t.Fatalf("CancelOrders error = %v, want %v", err, cancelErr)
	}
	if got, ok := executor.trackedInternalOrderID("owned"); !ok || got != "internal-1" {
		t.Fatalf("tracked order after failed cancel = %q/%v, want preserved", got, ok)
	}
}

func TestStrategyRuntimeLiveCancelMissingOrderIsNoop(t *testing.T) {
	cancelled := false
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{},
		deps: strategyRuntimeManagerDeps{
			cancelExecutionOrder: func(context.Context, string) (trdsrv.ExecutionOrder, error) {
				cancelled = true
				return trdsrv.ExecutionOrder{}, nil
			},
			appendRuntimeEvent: func(string, string, string, string) error { return nil },
		},
	}
	executor := &strategyLiveOrderExecutor{
		manager:  manager,
		instance: managedStrategyInstance{ID: "instance-a"},
	}
	if err := executor.CancelOrders(context.Background(), bbgoOrderForCancel("missing")); err != nil {
		t.Fatalf("CancelOrders missing: %v", err)
	}
	if cancelled {
		t.Fatal("missing cancel reached execution gateway")
	}
}

func bbgoOrderForCancel(clientOrderID string) bbgotypes.Order {
	return bbgotypes.Order{SubmitOrder: bbgotypes.SubmitOrder{ClientOrderID: clientOrderID}}
}

func TestStrategyRuntimeExecutesOnlyCurrentBarWorkerIntent(t *testing.T) {
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
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{
			{Kind: "entry", ID: "OldLong", Direction: "long", Quantity: 99, HasQuantity: true, BarIndex: lastIndex - 1},
			{Kind: "entry", ID: "CurrentLong", Direction: "long", Quantity: 1, HasQuantity: true, BarIndex: lastIndex, Time: request.Candles[lastIndex].OpenTime},
		}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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

	if got := stub.placedOrderCount(); got != 2 {
		t.Fatalf("expected one worker current-bar order per closed bar, got %d orders", got)
	}
	placedOrder, ok := stub.lastPlacedOrder()
	if !ok {
		t.Fatal("expected placed order")
	}
	if got := placedOrder.Quantity.Float64(); got != 1 {
		t.Fatalf("expected current-bar worker intent quantity 1, got %v", got)
	}
}

func TestStrategyRuntimeSkipsWhenWorkerReturnsNoCurrentBarIntent(t *testing.T) {
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
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{{
			Kind: "entry", ID: "OldLong", Direction: "long", Quantity: 1, HasQuantity: true, BarIndex: lastIndex - 1,
		}}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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
		t.Fatalf("expected stale worker intents to be skipped, got %d orders", got)
	}
}

func TestStrategyRuntimeRefreshesBrokerPositionsBeforeSellOnKLineClose(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)
	worker := newFakeStrategyRuntimePineWorker()
	worker.response = func(request pineworker.RunScriptRequest) pineworker.RunScriptResponse {
		lastIndex := len(request.Candles) - 1
		return pineworker.RunScriptResponse{JobID: request.JobID, OrderIntents: []pineworker.OrderIntent{{
			Kind: "close", ID: "Flat", Direction: "long", Quantity: 1, HasQuantity: true, BarIndex: lastIndex, Time: request.Candles[lastIndex].OpenTime,
		}}}
	}
	useFakeStrategyRuntimePineWorker(server, worker)

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
	installStrategyRuntimeTestExchange(server, stub)

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

func TestMarketDayStartUTCUsesOrderSymbolTimezone(t *testing.T) {
	now := time.Date(2026, time.January, 1, 2, 0, 0, 0, time.UTC)
	if got, want := marketDayStartUTC("US.AAPL", now), time.Date(2025, time.December, 31, 5, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("US day start = %s, want %s", got, want)
	}
	if got, want := marketDayStartUTC("HK.00700", now), time.Date(2025, time.December, 31, 16, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("HK day start = %s, want %s", got, want)
	}

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	overnight := time.Date(2026, time.June, 14, 20, 30, 0, 0, ny)
	if got, want := marketDayStartUTC("US.AAPL", overnight), time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("US overnight day start = %s, want %s", got, want)
	}
}

func TestTodaySubmittedOrderCountKeepsInstanceScopeWithinOrderMarketDay(t *testing.T) {
	runtimeStore, err := NewStrategyRuntimeStore(filepath.Join(t.TempDir(), "strategy-runtime.db"))
	if err != nil {
		t.Fatalf("NewStrategyRuntimeStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, runtimeStore.Close()) })
	manager := &strategyRuntimeManager{deps: strategyRuntimeManagerDeps{
		countRuntimeAudit: runtimeStore.CountAudit,
	}}
	instanceID := "multi-market-instance"
	for _, at := range []time.Time{
		time.Date(2025, time.December, 31, 6, 0, 0, 0, time.UTC),
		time.Date(2025, time.December, 31, 17, 0, 0, 0, time.UTC),
	} {
		if err := runtimeStore.AppendAudit(t.Context(), strategyRuntimeAuditEvent{
			InstanceID: instanceID,
			Kind:       "order_submitted",
			At:         at,
		}); err != nil {
			t.Fatalf("AppendAudit(%s): %v", at, err)
		}
	}
	now := time.Date(2026, time.January, 1, 2, 0, 0, 0, time.UTC)
	if got := manager.todaySubmittedOrderCount(instanceID, "US.AAPL", now); got != 2 {
		t.Fatalf("US market-day instance order count = %d, want 2", got)
	}
	if got := manager.todaySubmittedOrderCount(instanceID, "HK.00700", now); got != 1 {
		t.Fatalf("HK market-day instance order count = %d, want 1", got)
	}
}
