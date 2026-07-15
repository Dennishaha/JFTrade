package servercore

import (
	"context"
	"errors"
	"strings"
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestStrategyRuntimeRemainingDependencyBoundaries(t *testing.T) {
	server := &Server{}
	deps := newStrategyRuntimeManagerDeps(server)
	if _, ok := deps.currentInstance("missing"); ok {
		t.Fatal("nil strategy store returned an instance")
	}
	if err := deps.appendRuntimeEvent("missing", "", "", ""); err != nil {
		t.Fatalf("nil append runtime event error = %v", err)
	}
	if err := deps.transitionInstance("missing", "STOPPED", "", ""); err != nil {
		t.Fatalf("nil transition error = %v", err)
	}
	if err := deps.reconcileRuntimeFailure("missing", "detail"); err != nil {
		t.Fatalf("nil reconcile error = %v", err)
	}
	if _, err := deps.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err == nil {
		t.Fatal("nil trading placement error = nil")
	}
	if _, err := deps.cancelExecutionOrder(t.Context(), "missing"); err == nil {
		t.Fatal("nil trading cancellation error = nil")
	}
	if count, err := deps.countRuntimeAudit(t.Context(), runtimeactivity.AuditQuery{}); err != nil || count != 0 {
		t.Fatalf("nil audit count = %d, %v", count, err)
	}
	if err := deps.upsertObservation(t.Context(), runtimeactivity.ObservationSnapshot{}); err != nil {
		t.Fatalf("nil observation error = %v", err)
	}
	if _, err := deps.acquireMarketDataLease(t.Context(), "consumer", []mdsrv.InstrumentRef{{Channel: "KLINE"}}); err == nil {
		t.Fatal("nil market-data lease error = nil")
	}
	deps.wakeMarketDataCollector()

	var nilManager *strategyRuntimeManager
	if _, ok := nilManager.currentInstance("missing"); ok {
		t.Fatal("nil manager returned an instance")
	}
	if err := nilManager.appendRuntimeEvent("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := nilManager.transitionInstance("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := nilManager.reconcileRuntimeFailure("", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := nilManager.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err == nil {
		t.Fatal("nil manager placement error = nil")
	}
	if _, err := nilManager.cancelExecutionOrder(t.Context(), ""); err == nil {
		t.Fatal("nil manager cancellation error = nil")
	}
	nilManager.close()
}

func TestStrategyRuntimeRemainingInputLoadErrors(t *testing.T) {
	instance := managedStrategyInstance{Binding: strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		BrokerAccount: &strategyBrokerAccountBinding{BrokerID: "futu", TradingEnvironment: "SIMULATE", AccountID: "1", Market: "US"},
	}}
	manager := &strategyRuntimeManager{exchangeProvider: func() strategyRuntimeExchange { return nil }}
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil || !strings.Contains(err.Error(), "exchange") {
		t.Fatalf("nil exchange load error = %v", err)
	}

	stub := newStrategyRuntimeStubExchange()
	manager.exchangeProvider = func() strategyRuntimeExchange { return stub }
	stub.queryMarketsErr = errors.New("markets failed")
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil || !strings.Contains(err.Error(), "markets") {
		t.Fatalf("market load error = %v", err)
	}
	stub.queryMarketsErr = nil
	stub.queryFundsErr = errors.New("funds failed")
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil || !strings.Contains(err.Error(), "funds") {
		t.Fatalf("fund load error = %v", err)
	}
	stub.queryFundsErr = nil
	stub.queryPositionsErr = errors.New("positions failed")
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil || !strings.Contains(err.Error(), "positions") {
		t.Fatalf("position load error = %v", err)
	}
}

func TestStrategyRuntimeRemainingActivationReservationAndTradeBoundaries(t *testing.T) {
	canceled := false
	managed := &managedStrategyRuntime{cancel: func() { canceled = true }, symbols: map[string]*strategySymbolRuntime{}}
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{"duplicate": {symbols: map[string]*strategySymbolRuntime{}}},
		starting: map[string]struct{}{},
	}
	if err := manager.activateStrategyRuntime("duplicate", managed); err == nil || !canceled {
		t.Fatalf("duplicate activation error = %v canceled=%v", err, canceled)
	}
	if _, err := manager.reserveRuntimeStart("duplicate"); err == nil {
		t.Fatal("running reservation error = nil")
	}

	manager.handleMarketTrade(bbgotypes.Trade{Symbol: " "})
	manager.handleMarketTrade(bbgotypes.Trade{Symbol: "US.MISSING"})

	manager = &strategyRuntimeManager{runtimes: map[string]*managedStrategyRuntime{}, starting: map[string]struct{}{}, deps: strategyRuntimeManagerDeps{}}
	manager.close()
	if len(manager.runtimes) != 0 || len(manager.starting) != 0 {
		t.Fatalf("closed empty manager = %#v", manager)
	}
}

func TestStrategyRuntimeRemainingBuildSymbolErrors(t *testing.T) {
	manager := &strategyRuntimeManager{}
	stub := newStrategyRuntimeStubExchange()
	instance := managedStrategyInstance{ID: "instance", Definition: strategyDefinitionSummary{Name: "Coverage"}, Binding: strategyInstanceBinding{Symbols: []string{"US.AAPL"}, Interval: "1m"}}
	if _, err := manager.buildSymbolRuntime(t.Context(), context.Background(), stub, bbgotypes.MarketMap{}, stub.funds, nil, instance, "strategy.entry(\"Long\", strategy.long)", "US.AAPL", bbgotypes.Interval1m); err == nil {
		t.Fatal("missing market metadata error = nil")
	}
	if _, err := manager.buildSymbolRuntime(t.Context(), context.Background(), stub, stub.markets, stub.funds, nil, instance, "strategy.entry(\"Long\", strategy.long)", "US.AAPL", bbgotypes.Interval1m); err == nil {
		t.Fatal("missing Pine worker error = nil")
	}
}

func TestNewStrategyRuntimeManagerDisabledExchange(t *testing.T) {
	settings, err := NewSettingsStore(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	manager := newStrategyRuntimeManager(server)
	if manager.exchangeProvider() != nil {
		t.Fatal("disabled runtime exchange was non-nil")
	}
}
