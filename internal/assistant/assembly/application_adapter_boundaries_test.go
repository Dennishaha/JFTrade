package assembly

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestApplicationAdapterKeepsNilPortsCallable(t *testing.T) {
	var adapter *ApplicationAdapter
	ctx := t.Context()
	deps := adapter.ToolDeps()

	if len(deps.SystemStatus()) != 0 || deps.ADKEnabled() {
		t.Fatal("nil application ports reported available services")
	}
	if health, err := deps.FutuOpenDHealth(ctx); err != nil || health.(map[string]any)["status"] != "unavailable" {
		t.Fatalf("FutuOpenDHealth = %#v, %v", health, err)
	}
	if deps.PluginCatalog() == nil || deps.ManagedAccounts() == nil || deps.BrokerEnabled() || deps.DefaultTradeMarket() != "" {
		t.Fatal("nil catalog and broker fallbacks are unstable")
	}
	if subscriptions, active, err := deps.MarketSubscriptions(ctx); err == nil || subscriptions != nil || active != nil {
		t.Fatalf("MarketSubscriptions = %#v/%#v, %v", subscriptions, active, err)
	}
	for name, call := range map[string]func() error{
		"snapshot":          func() error { _, err := deps.MarketSnapshot(ctx, "US", "AAPL"); return err },
		"candles":           func() error { _, err := deps.MarketCandles(ctx, "US", "AAPL", "1m", 10); return err },
		"depth":             func() error { _, err := deps.MarketDepth(ctx, "US", "AAPL", 10); return err },
		"workflow snapshot": func() error { _, err := adapter.WorkflowMarketSnapshot(ctx, "US.AAPL"); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s error = nil, want unavailable", name)
		}
	}
	if deps.BrokerFunds(ctx, broker.ReadQuery{}, time.Second) != nil || deps.BrokerPositions(ctx, broker.ReadQuery{}, time.Second) != nil {
		t.Fatal("nil trading ports returned broker state")
	}
	if deps.RiskState() == nil || deps.RiskEvents() != nil {
		t.Fatal("nil system ports returned invalid risk fallbacks")
	}
	deps.RecordAudit(ctx, "ignored", "", "", nil)

	assertUnavailableApplicationOperations(t, deps)
	if adapter.runtimeSettings() != (jfsettings.ADKRuntimeSettings{}) || adapter.streamIdleTimeout() != 0 || adapter.runtimeLimits() != (RuntimeLimits{}) {
		t.Fatal("nil runtime settings returned non-zero values")
	}
	if adapter.assistantService() != nil || adapter.runtime() != nil || adapter.system() != nil || adapter.marketData() != nil ||
		adapter.strategy() != nil || adapter.trading() != nil || adapter.backtest() != nil || adapter.productFeatures() != nil || adapter.watchlist() != nil {
		t.Fatal("nil service providers returned a service")
	}
}

func TestApplicationAdapterUsesConfiguredRuntimeAndSettings(t *testing.T) {
	root := t.TempDir()
	settings := jfsettings.ADKRuntimeSettings{RunTimeoutMs: 2500, StreamIdleTimeoutMs: 420000}
	runtime, err := OpenApplication(ApplicationOptions{
		Paths: Paths{
			Database: filepath.Join(root, "adk.db"), Session: filepath.Join(root, "sessions.db"),
			Secrets: filepath.Join(root, "secrets.json"), Skills: filepath.Join(root, "skills"),
		},
		Ports: ApplicationPorts{RuntimeSettings: func() jfsettings.ADKRuntimeSettings { return settings }},
	})
	if err != nil {
		t.Fatalf("OpenApplication: %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("runtime.Close: %v", err)
		}
	})
	accounts := []jfsettings.ManagedBrokerAccount{{ID: "account-1", Enabled: true}}
	systemService := system.NewService()
	adapter := NewApplicationAdapter(ApplicationPorts{
		Runtime:         func() Runtime { return runtime },
		System:          func() *system.Service { return systemService },
		RuntimeSettings: func() jfsettings.ADKRuntimeSettings { return settings },
		ManagedAccounts: func() []jfsettings.ManagedBrokerAccount { return accounts },
		BrokerIntegration: func() jfsettings.BrokerIntegration {
			return jfsettings.BrokerIntegration{Enabled: true, Config: jfsettings.FutuIntegrationConfig{TradeMarket: "HK"}}
		},
		FutuOpenDHealth: func(context.Context) (any, error) { return map[string]any{"status": "ok"}, nil },
	})

	if !adapter.assistantEnabled() || adapter.assistantService() != runtime.Service() {
		t.Fatal("configured assistant runtime was not projected")
	}
	if adapter.systemStatus()["name"] != "JFTrade" || adapter.riskState() == nil || adapter.riskEvents() == nil {
		t.Fatal("configured system service was not projected")
	}
	if health, healthErr := adapter.futuOpenDHealth(t.Context()); healthErr != nil || health.(map[string]any)["status"] != "ok" {
		t.Fatalf("configured Futu health = %#v, %v", health, healthErr)
	}
	if got := adapter.managedAccounts().([]jfsettings.ManagedBrokerAccount); len(got) != 1 || got[0].ID != "account-1" {
		t.Fatalf("managed accounts = %#v", got)
	}
	if !adapter.brokerEnabled() || adapter.defaultTradeMarket() != "HK" {
		t.Fatal("configured broker settings were not projected")
	}
	if adapter.runtimeSettings() != settings || adapter.streamIdleTimeout() != settings.StreamIdleTimeoutMs ||
		adapter.runtimeLimits().RunTimeout != 2500*time.Millisecond {
		t.Fatal("configured runtime limits were not projected")
	}
	adapter.recordAudit(t.Context(), "adapter.test", "runtime", "configured runtime", nil)
	events, err := runtime.Service().GetAudit(t.Context(), assistant.AuditQuery{Kind: "adapter.test"})
	if err != nil || len(events) != 1 {
		t.Fatalf("runtime audit events = %#v, err=%v", events, err)
	}
}

func TestApplicationAdapterValidatesDomainInputsBeforeDelegation(t *testing.T) {
	tradingService := trdsrv.NewService()
	adapter := NewApplicationAdapter(ApplicationPorts{Trading: func() *trdsrv.Service { return tradingService }})
	ctx := t.Context()
	if _, err := adapter.executionOrders(); err == nil {
		t.Fatal("executionOrders unavailable error = nil")
	}
	if _, err := adapter.executionOrderEvents("order-1"); err == nil {
		t.Fatal("executionOrderEvents unavailable error = nil")
	}
	for name, call := range map[string]func() error{
		"orders scope":   func() error { _, err := adapter.brokerOrders(ctx, BrokerReadInput{Scope: "archive"}); return err },
		"fills scope":    func() error { _, err := adapter.brokerFills(ctx, BrokerReadInput{Scope: "archive"}); return err },
		"cash flow date": func() error { _, err := adapter.brokerCashFlows(ctx, BrokerReadInput{}); return err },
		"fee order IDs":  func() error { _, err := adapter.brokerFees(ctx, BrokerReadInput{}); return err },
		"margin symbols": func() error { _, err := adapter.brokerMarginRatios(ctx, BrokerReadInput{Market: "US"}); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s error = nil", name)
		}
	}

	request := backtestStartRequest(BacktestStartInput{
		DefinitionID: "definition-1", Market: "US", Symbol: "US.AAPL", Interval: "1m",
		StartDate: "2026-01-01", EndDate: "2026-01-02", InitialBalance: 10000, ChartType: "candle",
	})
	if request.DefinitionID != "definition-1" || request.InitialBalance != 10000 || string(request.ChartType) != "candle" {
		t.Fatalf("backtest request = %+v", request)
	}
	extended := true
	research := researchBacktestRequest(ResearchBacktestInput{
		Script: "strategy()", Market: "US", Symbol: "US.AAPL", Interval: "5m",
		UseExtendedHours: &extended, ChartType: "line",
	})
	if research.Script != "strategy()" || research.UseExtendedHours == nil || !*research.UseExtendedHours || string(research.ChartType) != "line" {
		t.Fatalf("research request = %+v", research)
	}
	readiness := backtestDataReadinessFromService(&btsrv.DataReadiness{
		Status: "syncing_data",
		Sync: &btsrv.SyncStarted{
			TaskID: "sync-1", Symbol: "US.AAPL", Since: "2026-01-01", Until: "2026-01-02",
		},
	})
	if readiness.DataSync == nil || readiness.DataSync.Status != "queued" {
		t.Fatalf("queued readiness = %+v", readiness)
	}
	if backtestRunSummaryFromService(nil) != (BacktestRunSummary{}) {
		t.Fatal("nil backtest run did not produce a zero summary")
	}

	if _, ok := (applicationOptimizationRuns{}).Get("missing"); ok {
		t.Fatal("nil optimization adapter returned a run")
	}
	withoutBacktest := applicationOptimizationRuns{adapter: NewApplicationAdapter(ApplicationPorts{})}
	if _, ok := withoutBacktest.Get("missing"); ok {
		t.Fatal("missing backtest service returned a run")
	}
	(applicationOptimizationRuns{}).Cancel("ignored")
	withoutBacktest.Cancel("ignored")

	lastError := " broker rejected "
	instance := stratsrv.InstanceView{
		ID: "instance-1", Definition: stratsrv.DefinitionSummary{Name: "Strategy", Version: "1.0.0"},
		Params: map[string]any{"definitionId": " definition-from-params "}, Status: "RUNNING",
		Binding: stratsrv.InstanceBinding{
			Symbols: []string{"US.AAPL"}, Interval: "1m", ExecutionMode: "paper",
			BrokerAccount: &stratsrv.BrokerAccountBinding{Market: "US", AccountID: "account-1"},
		},
		Logs: []string{" started "},
		RuntimeObservation: &stratsrv.RuntimeObservation{
			ActualStatus: " RUNNING ", ActiveSymbols: []string{"US.AAPL"}, LastError: &lastError,
		},
	}
	summary := strategyInstanceSummary(instance)
	if summary.DefinitionID != "definition-from-params" || summary.Market != "US" || summary.LatestLog != "started" || summary.LastError != "broker rejected" {
		t.Fatalf("strategy instance summary = %+v", summary)
	}
	instance.Definition.StrategyID = "definition-direct"
	if strategySummaryDefinitionID(instance) != "definition-direct" {
		t.Fatal("strategy definition ID did not prefer the typed definition")
	}
	program := &strategyir.Program{Metadata: strategyir.StrategyMetadata{Symbol: " US.MSFT ", Interval: " 15m "}}
	if symbol, interval := validationInstrument(StrategyPineValidation{Program: program}); symbol != "US.MSFT" || interval != "15m" {
		t.Fatalf("validation instrument = %q/%q", symbol, interval)
	}
	if symbol, interval := validationInstrument(StrategyPineValidation{}); symbol != "" || interval != "" {
		t.Fatalf("nil validation instrument = %q/%q", symbol, interval)
	}
	if brokerBindingMarket(nil) != "" || brokerBindingAccountID(nil) != "" {
		t.Fatal("nil broker binding returned identifiers")
	}
}

func assertUnavailableApplicationOperations(t *testing.T, deps ToolDeps) {
	t.Helper()
	for name, call := range map[string]func() error{
		"strategy definitions": func() error { _, err := deps.ListStrategyDefinitions(); return err },
		"strategy versions":    func() error { _, _, err := deps.ListStrategyDefinitionVersions("definition"); return err },
		"strategy version":     func() error { _, _, err := deps.GetStrategyDefinitionVersion("definition", "1"); return err },
		"strategy definition":  func() error { _, err := deps.SaveStrategyDefinition(StrategyDefinitionInput{}); return err },
		"strategy mode":        func() error { _, err := deps.UpdateStrategyInstanceMode("instance", "paper"); return err },
		"backtest data":        func() error { _, err := deps.EnsureBacktestData(nil, BacktestStartInput{}); return err },
		"research data":        func() error { _, err := deps.EnsureResearchBacktestData(ResearchBacktestInput{}); return err },
		"research start":       func() error { _, err := deps.StartResearchBacktest(ResearchBacktestInput{}); return err },
		"backtest result":      func() error { _, err := deps.BacktestResultView(BacktestResultViewInput{}); return err },
		"broker fills":         func() error { _, err := deps.BrokerFills(t.Context(), BrokerReadInput{}); return err },
		"broker cash flows":    func() error { _, err := deps.BrokerCashFlows(t.Context(), BrokerReadInput{}); return err },
		"broker fees":          func() error { _, err := deps.BrokerFees(t.Context(), BrokerReadInput{}); return err },
		"broker margin":        func() error { _, err := deps.BrokerMarginRatios(t.Context(), BrokerReadInput{}); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s error = nil, want unavailable", name)
		}
	}
	if deps.ListStrategyInstances() != nil || deps.ListBacktestRuns() != nil {
		t.Fatal("unavailable list operations returned data")
	}
	if progress, ok := deps.BacktestKLineSyncProgress("missing"); progress != nil || ok {
		t.Fatalf("missing sync progress = %#v/%v", progress, ok)
	}
	deps.CancelBacktest("missing")
}
