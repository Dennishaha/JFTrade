package liveruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestManagerMaintenanceStateAndPollingConfiguration(t *testing.T) {
	var nilManager *Manager
	nilManager.SetExchangeProvider(nil)
	nilManager.SetPineWorkerRunner(nil)
	nilManager.SetClosedKLineSyncInterval(time.Second)
	nilManager.Stop("missing")
	if got := nilManager.currentClosedKLineSyncInterval(); got != 0 {
		t.Fatalf("nil manager polling interval = %s, want disabled", got)
	}
	if reason := nilManager.MaintenanceBusyReason(t.Context()); reason != "" {
		t.Fatalf("nil manager maintenance reason = %q", reason)
	}

	manager := NewManager(Dependencies{})
	if got := manager.currentClosedKLineSyncInterval(); got != defaultClosedKLineSyncInterval {
		t.Fatalf("default polling interval = %s, want %s", got, defaultClosedKLineSyncInterval)
	}
	manager.SetClosedKLineSyncInterval(25 * time.Millisecond)
	if got := manager.currentClosedKLineSyncInterval(); got != 25*time.Millisecond {
		t.Fatalf("configured polling interval = %s", got)
	}
	if reason := manager.MaintenanceBusyReason(t.Context()); reason != "" {
		t.Fatalf("idle manager maintenance reason = %q", reason)
	}

	manager.starting["starting-instance"] = struct{}{}
	if reason := manager.MaintenanceBusyReason(t.Context()); reason != "存在活动策略实例" {
		t.Fatalf("starting manager maintenance reason = %q", reason)
	}
	delete(manager.starting, "starting-instance")
	manager.runtimes["running-instance"] = &managedRuntime{}
	if reason := manager.MaintenanceBusyReason(t.Context()); reason != "存在活动策略实例" {
		t.Fatalf("running manager maintenance reason = %q", reason)
	}
	delete(manager.runtimes, "running-instance")
	manager.Stop("missing")
}

func TestManagerInputLoadingReportsEachUnavailableDependency(t *testing.T) {
	instance := stratsrv.ManagedInstance{Binding: stratsrv.InstanceBinding{
		Symbols: []string{"US.AAPL"},
		BrokerAccount: &stratsrv.BrokerAccountBinding{
			BrokerID: "futu", TradingEnvironment: "SIMULATE", AccountID: "1", Market: "US",
		},
	}}
	manager := NewManager(Dependencies{ExchangeProvider: func() Exchange { return nil }})
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
		!strings.Contains(err.Error(), "exchange") {
		t.Fatalf("nil exchange load error = %v", err)
	}

	stub := newStrategyRuntimeStubExchange()
	manager.SetExchangeProvider(func() Exchange { return stub })
	stub.queryMarketsErr = errors.New("markets failed")
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
		!strings.Contains(err.Error(), "markets") {
		t.Fatalf("market load error = %v", err)
	}
	stub.queryMarketsErr = nil
	stub.queryFundsErr = errors.New("funds failed")
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
		!strings.Contains(err.Error(), "funds") {
		t.Fatalf("fund load error = %v", err)
	}
	stub.queryFundsErr = nil
	stub.queryPositionsErr = errors.New("positions failed")
	if _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
		!strings.Contains(err.Error(), "positions") {
		t.Fatalf("position load error = %v", err)
	}
}

func TestManagerActivationReservationAndUnknownTradeBoundaries(t *testing.T) {
	cancelled := false
	managed := &managedRuntime{
		cancel:  func() { cancelled = true },
		symbols: map[string]*symbolRuntime{},
	}
	manager := NewManager(Dependencies{})
	manager.runtimes["duplicate"] = &managedRuntime{symbols: map[string]*symbolRuntime{}}
	if err := manager.activateStrategyRuntime("duplicate", managed); err == nil || !cancelled {
		t.Fatalf("duplicate activation error = %v cancelled=%v", err, cancelled)
	}
	if _, err := manager.reserveRuntimeStart("duplicate"); err == nil {
		t.Fatal("running reservation error = nil")
	}

	manager.HandleMarketTrade(bbgotypes.Trade{Symbol: " "})
	manager.HandleMarketTrade(bbgotypes.Trade{Symbol: "US.MISSING"})

	empty := NewManager(Dependencies{})
	if err := empty.Close(); err != nil {
		t.Fatalf("close empty manager: %v", err)
	}
	if len(empty.runtimes) != 0 || len(empty.starting) != 0 {
		t.Fatalf("closed empty manager = %#v", empty)
	}
}

func TestManagerBuildSymbolRequiresMarketAndPineWorker(t *testing.T) {
	manager := NewManager(Dependencies{})
	stub := newStrategyRuntimeStubExchange()
	instance := stratsrv.ManagedInstance{
		ID:         "instance",
		Definition: stratsrv.DefinitionSummary{Name: "Coverage"},
		Binding:    stratsrv.InstanceBinding{Symbols: []string{"US.AAPL"}, Interval: "1m"},
	}
	if _, err := manager.buildSymbolRuntime(
		t.Context(),
		context.Background(),
		stub,
		bbgotypes.MarketMap{},
		stub.funds,
		nil,
		instance,
		"strategy.entry(\"Long\", strategy.long)",
		"US.AAPL",
		bbgotypes.Interval1m,
	); err == nil {
		t.Fatal("missing market metadata error = nil")
	}
	if _, err := manager.buildSymbolRuntime(
		t.Context(),
		context.Background(),
		stub,
		stub.markets,
		stub.funds,
		nil,
		instance,
		"strategy.entry(\"Long\", strategy.long)",
		"US.AAPL",
		bbgotypes.Interval1m,
	); err == nil {
		t.Fatal("missing Pine worker error = nil")
	}
}
