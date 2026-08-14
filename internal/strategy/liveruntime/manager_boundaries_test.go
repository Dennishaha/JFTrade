package liveruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
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

func TestManagerCompatibilityResolversAndCommandBoundariesFailClosed(t *testing.T) {
	stub := newStrategyRuntimeStubExchange()
	manager := NewManager(Dependencies{ExchangeProvider: func() Exchange { return stub }})
	if account := manager.resolveAccount(stratsrv.InstanceBinding{}); account != stub {
		t.Fatalf("compatibility account source = %#v", account)
	}
	if _, err := manager.placeExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err == nil {
		t.Fatal("manager without trade commands accepted placement")
	}
	if _, err := manager.cancelExecutionOrder(t.Context(), "order-1"); err == nil {
		t.Fatal("manager without trade commands accepted cancellation")
	}
	manager.SetExchangeProvider(nil)
	if account := manager.resolveAccount(stratsrv.InstanceBinding{}); account != nil {
		t.Fatalf("nil compatibility provider account = %#v", account)
	}
	empty := NewManager(Dependencies{})
	if account := empty.resolveAccount(stratsrv.InstanceBinding{}); account != nil {
		t.Fatalf("missing account resolver = %#v", account)
	}
	var nilManager *Manager
	if account := nilManager.resolveAccount(stratsrv.InstanceBinding{}); account != nil {
		t.Fatalf("nil manager account resolver = %#v", account)
	}

	wantErr := errors.New("capabilities unavailable")
	capabilityManager := NewManager(Dependencies{MarketDataCapabilities: func(context.Context) (mdsrv.ProviderCapabilities, error) {
		return mdsrv.ProviderCapabilities{}, wantErr
	}})
	if err := capabilityManager.validateMarketDataCapabilities(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("capability error = %v", err)
	}
	capabilityManager.deps.MarketDataCapabilities = func(context.Context) (mdsrv.ProviderCapabilities, error) {
		return mdsrv.ProviderCapabilities{StreamingCandles: true}, nil
	}
	if err := capabilityManager.validateMarketDataCapabilities(t.Context()); err != nil {
		t.Fatalf("streaming capability rejected: %v", err)
	}
}

func TestStrategyRuntimeRequiresStreamingCandlesForLiveAndNotifyOnly(t *testing.T) {
	for _, executionMode := range []string{"live", "notify_only"} {
		t.Run(executionMode, func(t *testing.T) {
			manager := NewManager(Dependencies{
				MarketDataCapabilities: func(context.Context) (mdsrv.ProviderCapabilities, error) {
					return mdsrv.ProviderCapabilities{HistoricalCandles: true, StreamingCandles: false}, nil
				},
			})
			err := manager.Start(t.Context(), stratsrv.ManagedInstance{
				ID: "poll-only-" + executionMode,
				Binding: stratsrv.InstanceBinding{
					Symbols: []string{"US.AAPL"}, Interval: "1m", ExecutionMode: executionMode,
				},
				Params: map[string]any{"script": `strategy.entry("Long", strategy.long)`},
			})
			if err == nil || !strings.Contains(err.Error(), "streaming candles") {
				t.Fatalf("Start error = %v, want streaming capability rejection", err)
			}
		})
	}
}

func TestStrategyRuntimeRejectsUnhealthyActiveProviderUnlessExchangeOverrideOwnsHealth(t *testing.T) {
	manager := NewManager(Dependencies{
		MarketDataCapabilities: func(context.Context) (mdsrv.ProviderCapabilities, error) {
			return mdsrv.ProviderCapabilities{StreamingCandles: true}, nil
		},
		MarketDataHealth: func(context.Context) (mdsrv.HealthStatus, error) {
			return mdsrv.HealthStatus{Connected: true, Readiness: mdsrv.ProviderReadinessFailed, LastError: "sidecar unavailable"}, nil
		},
	})
	if err := manager.validateMarketDataCapabilities(t.Context()); err == nil || !strings.Contains(err.Error(), "sidecar unavailable") {
		t.Fatalf("unhealthy provider validation = %v", err)
	}
	manager.deps.MarketDataHealth = func(context.Context) (mdsrv.HealthStatus, error) {
		return mdsrv.HealthStatus{}, errors.New("health callback failed")
	}
	if err := manager.validateMarketDataCapabilities(t.Context()); err == nil || !strings.Contains(err.Error(), "health callback failed") {
		t.Fatalf("health callback error = %v", err)
	}
	manager.deps.MarketDataHealth = func(context.Context) (mdsrv.HealthStatus, error) {
		return mdsrv.HealthStatus{}, nil
	}
	if err := manager.validateMarketDataCapabilities(t.Context()); err == nil || !strings.Contains(err.Error(), "provider is unhealthy") {
		t.Fatalf("generic unhealthy provider error = %v", err)
	}

	healthCalls := 0
	manager.deps.MarketDataHealth = func(context.Context) (mdsrv.HealthStatus, error) {
		healthCalls++
		return mdsrv.HealthStatus{}, errors.New("health probe should be bypassed")
	}
	manager.SetExchangeProvider(func() Exchange { return newStrategyRuntimeStubExchange() })
	if err := manager.validateMarketDataCapabilities(t.Context()); err != nil {
		t.Fatalf("explicit exchange health override validation = %v", err)
	}
	if healthCalls != 0 {
		t.Fatalf("health callback calls under explicit exchange override = %d", healthCalls)
	}
}

func TestStrategyRuntimeResolvesExactBoundBrokerWithoutLegacyFallback(t *testing.T) {
	stub := newStrategyRuntimeStubExchange()
	resolvedBrokerID := ""
	manager := NewManager(Dependencies{
		ExchangeProvider: func() Exchange { return stub },
		ExchangeResolver: func(binding stratsrv.InstanceBinding) Exchange {
			if binding.BrokerAccount != nil {
				resolvedBrokerID = binding.BrokerAccount.BrokerID
			}
			return nil
		},
	})
	instance := stratsrv.ManagedInstance{Binding: stratsrv.InstanceBinding{
		Symbols: []string{"US.AAPL"},
		BrokerAccount: &stratsrv.BrokerAccountBinding{
			BrokerID: "paper-broker", AccountID: "account-1",
			TradingEnvironment: "SIMULATE", Market: "US",
		},
	}}
	if _, _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil {
		t.Fatal("exact broker resolution unexpectedly used legacy fallback")
	}
	if resolvedBrokerID != "paper-broker" {
		t.Fatalf("resolved broker = %q, want paper-broker", resolvedBrokerID)
	}
}

func TestLiveStrategyRequiresExplicitBrokerAccountBinding(t *testing.T) {
	base := stratsrv.InstanceBinding{Symbols: []string{"US.AAPL"}}
	tests := []struct {
		name  string
		value func() *stratsrv.BrokerAccountBinding
		want  string
	}{
		{name: "missing binding", value: func() *stratsrv.BrokerAccountBinding { return nil }, want: "explicit broker account"},
		{name: "missing broker", value: func() *stratsrv.BrokerAccountBinding {
			return &stratsrv.BrokerAccountBinding{AccountID: "account-1", TradingEnvironment: "SIMULATE", Market: "US"}
		}, want: "brokerId"},
		{name: "missing account", value: func() *stratsrv.BrokerAccountBinding {
			return &stratsrv.BrokerAccountBinding{BrokerID: "futu", TradingEnvironment: "SIMULATE", Market: "US"}
		}, want: "accountId"},
		{name: "missing environment", value: func() *stratsrv.BrokerAccountBinding {
			return &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "account-1", Market: "US"}
		}, want: "tradingEnvironment"},
		{name: "missing market", value: func() *stratsrv.BrokerAccountBinding {
			return &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "account-1", TradingEnvironment: "SIMULATE"}
		}, want: "market"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding := base
			binding.ExecutionMode = "live"
			binding.BrokerAccount = test.value()
			manager := NewManager(Dependencies{ExchangeProvider: func() Exchange { return newStrategyRuntimeStubExchange() }})
			if _, _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), stratsrv.ManagedInstance{Binding: binding}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadStrategyRuntimeInputs error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNotifyOnlyLoadsRealtimeMarketDataWithoutBrokerAccount(t *testing.T) {
	stub := newStrategyRuntimeStubExchange()
	accountResolved := false
	manager := NewManager(Dependencies{
		MarketDataProvider: func() MarketDataSource { return stub },
		AccountResolver: func(stratsrv.InstanceBinding) AccountSource {
			accountResolved = true
			return nil
		},
	})
	instance := stratsrv.ManagedInstance{Binding: stratsrv.InstanceBinding{
		Symbols: []string{"US.AAPL"}, ExecutionMode: "notify_only",
	}}
	marketData, account, markets, funds, positions, err := manager.loadStrategyRuntimeInputs(t.Context(), instance)
	if err != nil {
		t.Fatalf("load notify-only inputs: %v", err)
	}
	if marketData != stub || account != nil || funds != nil || positions != nil || len(markets) == 0 {
		t.Fatalf("notify-only inputs = marketData:%#v account:%#v markets:%#v funds:%#v positions:%#v", marketData, account, markets, funds, positions)
	}
	if accountResolved {
		t.Fatal("notify-only runtime resolved a broker account")
	}
}

func TestManagerInputLoadingReportsEachUnavailableDependency(t *testing.T) {
	instance := stratsrv.ManagedInstance{Binding: stratsrv.InstanceBinding{
		Symbols: []string{"US.AAPL"},
		BrokerAccount: &stratsrv.BrokerAccountBinding{
			BrokerID: "futu", TradingEnvironment: "SIMULATE", AccountID: "1", Market: "US",
		},
	}}
	manager := NewManager(Dependencies{ExchangeProvider: func() Exchange { return nil }})
	if _, _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
		!strings.Contains(err.Error(), "market-data") {
		t.Fatalf("nil market-data load error = %v", err)
	}

	stub := newStrategyRuntimeStubExchange()
	manager.SetExchangeProvider(func() Exchange { return stub })
	stub.queryMarketsErr = errors.New("markets failed")
	if _, _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
		!strings.Contains(err.Error(), "markets") {
		t.Fatalf("market load error = %v", err)
	}
	stub.queryMarketsErr = nil
	stub.queryFundsErr = errors.New("funds failed")
	if _, _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
		!strings.Contains(err.Error(), "funds") {
		t.Fatalf("fund load error = %v", err)
	}
	stub.queryFundsErr = nil
	stub.queryPositionsErr = errors.New("positions failed")
	if _, _, _, _, _, err := manager.loadStrategyRuntimeInputs(t.Context(), instance); err == nil ||
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
	stub.nilStream = true
	if _, err := manager.buildSymbolRuntime(
		t.Context(), context.Background(), stub, stub, stub.markets, stub.funds, nil,
		instance, "strategy.entry(\"Long\", strategy.long)", "US.AAPL", bbgotypes.Interval1m,
	); err == nil || !strings.Contains(err.Error(), "nil stream") {
		t.Fatalf("nil market stream error = %v", err)
	}
	stub.nilStream = false
	standard := bbgotypes.NewStandardStream()
	stub.stream = &struct{ bbgotypes.Stream }{Stream: &standard}
	if _, _, err := newStrategyRuntimeSession(stub, stub.markets, stub.funds, nil, stub.markets["US.AAPL"], "US.AAPL"); err == nil ||
		!strings.Contains(err.Error(), "kline emission") {
		t.Fatalf("non-emitter stream error = %v", err)
	}
}

func TestRuntimeBuildCallbacksRecordErrorsAndIgnoredOrders(t *testing.T) {
	events := make([]string, 0, 2)
	manager := NewManager(Dependencies{AppendRuntimeEvent: func(_ string, _ string, kind string, detail string) error {
		events = append(events, kind+":"+detail)
		return nil
	}})
	stub := newStrategyRuntimeStubExchange()
	session, emitter, err := newStrategyRuntimeSession(stub, stub.markets, stub.funds, nil, stub.markets["US.AAPL"], "US.AAPL")
	if err != nil {
		t.Fatalf("newStrategyRuntimeSession: %v", err)
	}
	instance := stratsrv.ManagedInstance{
		ID: "instance", Definition: stratsrv.DefinitionSummary{Name: " Callback Test "},
		Binding: stratsrv.InstanceBinding{Symbols: []string{"US.AAPL"}, Interval: "1m"},
	}
	runner := manager.newSymbolRuntime(
		t.Context(), stub, stub, instance, "US.AAPL", bbgotypes.Interval1m,
		stub.markets["US.AAPL"], session, emitter, stub.funds, nil,
	)
	if runner.name != "Callback Test" || runner.exchange != stub.Name() {
		t.Fatalf("symbol runtime = %+v", runner)
	}
	runner.onClosedKLine(time.Now().UTC())
	runner.onError(" ")
	runner.onError("provider disconnected")
	manager.recordIgnoredOrder(instance.ID, "US.AAPL", "notify-only")
	if len(events) != 2 || !strings.HasPrefix(events[0], "runtime_error:") || events[1] != "order_ignored:notify-only" {
		t.Fatalf("runtime callback events = %#v", events)
	}
}
