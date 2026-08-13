package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/broker"
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
	deps := newStrategyRuntimeDependencies(server)
	price := 100.0
	_, err := deps.TradeCommands.PlaceExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{
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

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

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
	orders := server.stores.ExecutionOrders.AllOrders().Orders
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
	audit, ok := strategyRuntimeTestAudit(server, instanceID)
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
	installStrategyRuntimeTestExchange(server, stub)

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)
	if _, err := server.stores.StrategyCatalog.UpdateInstanceRuntimeRisk(instanceID, stratsrv.RuntimeRiskSettings{
		Mode:      "enforce",
		CloseOnly: true,
	}); err != nil {
		t.Fatalf("updateStrategyRuntimeRisk: %v", err)
	}

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 0 {
		t.Fatalf("expected runtime risk to reject broker order, got %d", got)
	}
	if orders := server.stores.ExecutionOrders.AllOrders().Orders; len(orders) != 0 {
		t.Fatalf("expected no execution order after risk rejection, got %+v", orders)
	}
	audit, ok := strategyRuntimeTestAudit(server, instanceID)
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

	definition := stratsrv.Definition{
		ID:           "runtime-default-qty-test",
		Name:         "Runtime Default Qty Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Default Qty Test\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\nstrategy.entry(\"Long\", strategy.long)",
	}
	instance, err := server.stores.StrategyCatalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instance.ID)

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 500, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 501, strategyRuntimeTestTime(10, 1, 0)))

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

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 500, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 501, strategyRuntimeTestTime(10, 1, 0)))

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

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)

	stub.positions = []broker.PositionSnapshot{{Market: "US", Symbol: "AAPL", Quantity: 20, SellableQuantity: 20}}
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

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

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)

	stub.positions = []broker.PositionSnapshot{{Market: "US", Symbol: "AAPL", Quantity: 7, SellableQuantity: 7}}
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

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

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 1000, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 1001, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 0 {
		t.Fatalf("expected tiny order to be ignored, got %d broker orders", got)
	}
	audit, ok := strategyRuntimeTestAudit(server, instanceID)
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

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instanceID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	orders := server.stores.ExecutionOrders.AllOrders().Orders
	if len(orders) != 1 {
		t.Fatalf("expected one tracked execution order, got %+v", orders)
	}
	if orders[0].Status != "CANCEL_REQUESTED" {
		t.Fatalf("order status = %q, want CANCEL_REQUESTED", orders[0].Status)
	}
	audit, ok := strategyRuntimeTestAudit(server, instanceID)
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

	definition := stratsrv.Definition{
		ID:           "runtime-pyramiding-test",
		Name:         "Runtime Pyramiding Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Pyramiding Test\", overlay=true, pyramiding=2)\nstrategy.entry(\"Long\", strategy.long, qty=1)",
	}
	instance, err := server.stores.StrategyCatalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instance.ID)

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 102, strategyRuntimeTestTime(10, 2, 0)))

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

	definition := stratsrv.Definition{
		ID:           "runtime-default-pyramiding-test",
		Name:         "Runtime Default Pyramiding Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Default Pyramiding Test\", overlay=true)\nstrategy.entry(\"Long\", strategy.long, qty=1)",
	}
	instance, err := server.stores.StrategyCatalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instance.ID)

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

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

	definition := stratsrv.Definition{
		ID:           "runtime-sell-test",
		Name:         "Runtime Sell Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Sell Test\", overlay=true)\nstrategy.close(\"Long\")",
	}
	instance, err := server.stores.StrategyCatalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instance.ID)

	stub.positions = []broker.PositionSnapshot{{
		Market:           "US",
		Symbol:           "AAPL",
		Quantity:         1,
		SellableQuantity: 1,
	}}

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 1 {
		t.Fatalf("expected 1 broker order after runtime position refresh, got %d", got)
	}
	orders := server.stores.ExecutionOrders.AllOrders().Orders
	if len(orders) != 1 {
		t.Fatalf("expected 1 execution order, got %+v", orders)
	}
	if orders[0].Side == nil || *orders[0].Side != "SELL" {
		t.Fatalf("expected SELL execution order, got %+v", orders[0])
	}
	if orders[0].RequestedQuantity == nil || *orders[0].RequestedQuantity != 1 {
		t.Fatalf("expected quantity 1 execution order, got %+v", orders[0])
	}
	if _, ok := server.runtimes.StrategyRuntime().GetObservation(instance.ID); !ok {
		t.Fatalf("expected active runtime observation for %s", instance.ID)
	}
	audit, ok := strategyRuntimeTestAudit(server, instance.ID)
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

	definition := stratsrv.Definition{
		ID:           "runtime-disconnected-refresh-test",
		Name:         "Runtime Disconnected Refresh Test",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Runtime Disconnected Refresh Test\", overlay=true)\nstrategy.close(\"Long\")",
	}
	instance, err := server.stores.StrategyCatalog.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US"},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	instanceRecord, ok := server.stores.StrategyCatalog.GetInstance(instance.ID)
	if !ok {
		t.Fatalf("strategy(%s) not found", instance.ID)
	}
	if err := server.runtimes.StrategyRuntime().Start(context.Background(), instanceRecord); err != nil {
		t.Fatalf("startStrategy: %v", err)
	}
	if _, err := server.stores.StrategyCatalog.TransitionRuntime(instance.ID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instance.ID)

	stub.queryFundsErr = errors.New("client closed")
	stub.queryPositionsErr = errors.New("client closed")

	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.runtimes.StrategyRuntime().HandleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	if got := stub.placedOrderCount(); got != 1 {
		t.Fatalf("expected cached position to allow 1 broker order, got %d", got)
	}
	audit, ok := strategyRuntimeTestAudit(server, instance.ID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instance.ID)
	}
	for _, entry := range audit.Entries {
		if entry.Kind == "runtime_error" && strings.Contains(entry.Detail, "client closed") {
			t.Fatalf("expected disconnected refresh to avoid runtime_error audit entry, got %+v", audit.Entries)
		}
	}
	observation, ok := server.runtimes.StrategyRuntime().GetObservation(instance.ID)
	if !ok {
		t.Fatalf("expected runtime observation for %s", instance.ID)
	}
	if observation.LastError != nil {
		t.Fatalf("expected runtime observation without lastError after disconnected refresh, got %+v", observation)
	}
}
