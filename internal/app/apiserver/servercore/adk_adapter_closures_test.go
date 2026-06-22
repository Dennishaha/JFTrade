package servercore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	asst "github.com/jftrade/jftrade-main/internal/assistant"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/shopspring/decimal"
)

func TestADKToolDepsCoreClosuresCoverDisconnectedAndCachedFlows(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	deps := server.adkToolDeps()

	if !deps.ADKEnabled() {
		t.Fatal("ADKEnabled() = false, want true for initialized test server")
	}
	if status := deps.SystemStatus(); len(status) == 0 {
		t.Fatalf("SystemStatus() = %#v, want non-empty status payload", status)
	}

	healthAny, err := deps.FutuOpenDHealth(context.Background())
	if err != nil {
		t.Fatalf("FutuOpenDHealth: %v", err)
	}
	health := healthAny.(map[string]any)
	if health["status"] != "offline" {
		t.Fatalf("FutuOpenDHealth status = %#v, want offline", health["status"])
	}
	runtime := health["runtime"].(map[string]any)
	if runtime["connectivity"] != "disconnected" {
		t.Fatalf("runtime.connectivity = %#v, want disconnected", runtime["connectivity"])
	}

	catalog := deps.PluginCatalog().(stratsrv.PluginCatalog)
	if catalog.TargetDir == "" {
		t.Fatalf("PluginCatalog target dir = empty: %#v", catalog)
	}

	subscriptionsAny, activeAny, err := deps.MarketSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("MarketSubscriptions: %v", err)
	}
	subscriptions := subscriptionsAny.(mdsrv.SubscriptionsSnapshot)
	if subscriptions["totalActiveSubscriptions"] != 0 {
		t.Fatalf("subscriptions = %#v, want zero active subscriptions", subscriptions)
	}
	if len(activeAny.([]string)) != 0 {
		t.Fatalf("active instruments = %#v, want empty initial set", activeAny)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	price := decimal.RequireFromString("12.34")
	server.marketdataSvc.Ingest(mdsrv.Tick{
		InstrumentID: "US.AAPL",
		Market:       "US",
		Symbol:       "AAPL",
		Price:        price,
		Bid:          price,
		Ask:          price,
		Turnover:     price,
		Volume:       100,
		QuoteAt:      now,
		ObservedAt:   now,
		Source:       "unit-test",
	})

	snapshotAny, err := deps.MarketSnapshot(context.Background(), "US", "AAPL")
	if err != nil {
		t.Fatalf("MarketSnapshot: %v", err)
	}
	snapshot := snapshotAny.(map[string]any)
	request := snapshot["request"].(map[string]any)
	if request["instrumentId"] != "US.AAPL" {
		t.Fatalf("snapshot request = %#v, want instrumentId US.AAPL", request)
	}

	candlesAny, err := deps.MarketCandles(context.Background(), "US", "AAPL", "tick", 5)
	if err != nil {
		t.Fatalf("MarketCandles: %v", err)
	}
	candles := candlesAny.(map[string]any)
	if candles["totalReturned"].(int) < 1 {
		t.Fatalf("MarketCandles totalReturned = %#v, want at least one cached candle", candles["totalReturned"])
	}

	if got := deps.ManagedAccounts().([]ManagedBrokerAccount); len(got) != 0 {
		t.Fatalf("ManagedAccounts = %#v, want empty default managed accounts", got)
	}
	if deps.BrokerEnabled() {
		t.Fatal("BrokerEnabled() = true, want false with default disabled integration")
	}
	if market := deps.DefaultTradeMarket(); strings.TrimSpace(market) == "" {
		t.Fatal("DefaultTradeMarket() = empty, want configured default market")
	}

	funds := deps.BrokerFunds(context.Background(), broker.ReadQuery{BrokerID: "futu"}, time.Second).(map[string]any)
	if funds["lastError"] == nil {
		t.Fatalf("BrokerFunds = %#v, want disconnected/degraded fallback payload", funds)
	}
	positions := deps.BrokerPositions(context.Background(), broker.ReadQuery{BrokerID: "futu"}, time.Second).(map[string]any)
	if positions["lastError"] == nil {
		t.Fatalf("BrokerPositions = %#v, want disconnected/degraded fallback payload", positions)
	}

	if orders := deps.ExecutionOrders().([]trdsrv.ExecutionOrder); len(orders) != 0 {
		t.Fatalf("ExecutionOrders = %#v, want empty order snapshot", orders)
	}
	orderEvents := deps.ExecutionOrderEvents("missing-order").(trdsrv.ExecutionOrderEvents)
	if orderEvents.InternalOrderID != "missing-order" || len(orderEvents.Events) != 0 {
		t.Fatalf("ExecutionOrderEvents = %#v, want empty timeline for missing order", orderEvents)
	}

	if _, err := deps.BrokerOrders(context.Background(), BrokerReadInput{Scope: "archive"}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("BrokerOrders invalid scope error = %v, want invalid scope", err)
	}
	if _, err := deps.BrokerFills(context.Background(), BrokerReadInput{Scope: "archive"}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("BrokerFills invalid scope error = %v, want invalid scope", err)
	}
	if _, err := deps.BrokerCashFlows(context.Background(), BrokerReadInput{}); err == nil || !strings.Contains(err.Error(), "clearingDate") {
		t.Fatalf("BrokerCashFlows missing clearingDate error = %v, want validation failure", err)
	}
	cashFlows, err := deps.BrokerCashFlows(context.Background(), BrokerReadInput{
		TradingEnvironment: "REAL",
		Market:             "US",
		ClearingDate:       "2025-01-03",
	})
	if err != nil {
		t.Fatalf("BrokerCashFlows(valid): %v", err)
	}
	if cashFlows.(map[string]any)["lastError"] == nil {
		t.Fatalf("BrokerCashFlows(valid) = %#v, want disconnected/degraded backend fallback payload", cashFlows)
	}
	if _, err := deps.BrokerFees(context.Background(), BrokerReadInput{}); err == nil || !strings.Contains(err.Error(), "orderIdEx") {
		t.Fatalf("BrokerFees missing orderIdEx error = %v, want validation failure", err)
	}
	fees, err := deps.BrokerFees(context.Background(), BrokerReadInput{
		TradingEnvironment: "REAL",
		Market:             "US",
		OrderIDEx:          []string{"fee-1"},
	})
	if err != nil {
		t.Fatalf("BrokerFees(valid): %v", err)
	}
	if fees.(map[string]any)["lastError"] == nil {
		t.Fatalf("BrokerFees(valid) = %#v, want backend fallback payload", fees)
	}
	if _, err := deps.BrokerMarginRatios(context.Background(), BrokerReadInput{Market: "US"}); err == nil || !strings.Contains(err.Error(), "symbol") {
		t.Fatalf("BrokerMarginRatios missing symbol error = %v, want validation failure", err)
	}
	marginRatios, err := deps.BrokerMarginRatios(context.Background(), BrokerReadInput{
		Market:  "US",
		Symbols: []string{"AAPL"},
	})
	if err != nil {
		t.Fatalf("BrokerMarginRatios(valid): %v", err)
	}
	if marginRatios.(map[string]any)["lastError"] == nil {
		t.Fatalf("BrokerMarginRatios(valid) = %#v, want backend fallback payload", marginRatios)
	}
	if _, err := deps.MarketDepth(context.Background(), "US", "AAPL", 10); err == nil {
		t.Fatal("MarketDepth() error = nil, want missing broker market-data error")
	}

	riskState := deps.RiskState().(map[string]any)
	killSwitch := riskState["killSwitch"].(map[string]any)
	if killSwitch["killSwitchActive"] != false {
		t.Fatalf("RiskState killSwitch = %#v, want inactive default kill switch", killSwitch)
	}
	if _, parseErr := time.Parse(time.RFC3339Nano, riskState["checkedAt"].(string)); parseErr != nil {
		t.Fatalf("RiskState checkedAt parse error: %v", parseErr)
	}
	riskEvents := deps.RiskEvents().(map[string]any)
	entries := riskEvents["entries"].([]any)
	if len(entries) != 0 {
		t.Fatalf("RiskEvents entries = %#v, want empty default risk events", entries)
	}
}

func TestADKToolDepsStrategyAndBacktestClosures(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	deps := server.adkToolDeps()

	validation, err := ValidateADKStrategyScript("test", `//@version=6
strategy("ADK Closure Strategy", overlay=true)
log.info("ready")`)
	if err != nil {
		t.Fatalf("ValidateADKStrategyScript: %v", err)
	}

	draftAny, err := deps.SaveStrategyDraft(StrategyDraftInput{Name: "ADK Closure Draft", Validation: validation})
	if err != nil {
		t.Fatalf("SaveStrategyDraft: %v", err)
	}
	draft := draftAny.(strategyDesignDefinition)
	if draft.ID == "" || draft.Name != "ADK Closure Draft" {
		t.Fatalf("draft = %#v, want persisted draft definition", draft)
	}

	definitionAny, err := deps.SaveStrategyDefinition(StrategyDefinitionInput{
		Name:        "ADK Closure Definition",
		Description: "saved via adapter closure",
		Validation:  validation,
	})
	if err != nil {
		t.Fatalf("SaveStrategyDefinition(create): %v", err)
	}
	definition := definitionAny.(strategyDesignDefinition)
	if definition.ID == "" || definition.Name != "ADK Closure Definition" {
		t.Fatalf("definition = %#v, want persisted definition", definition)
	}
	if _, err := deps.SaveStrategyDefinition(StrategyDefinitionInput{
		DefinitionID: "missing-definition",
		Name:         "Missing",
		Validation:   validation,
	}); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("SaveStrategyDefinition(missing) error = %v, want not found", err)
	}

	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.TME"},
		Interval:      "5m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{
			BrokerID:           "futu",
			AccountID:          "acct-1",
			TradingEnvironment: "SIMULATE",
			Market:             "US",
		},
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}

	definitions := deps.ListStrategyDefinitions()
	if len(definitions) < 2 {
		t.Fatalf("ListStrategyDefinitions = %#v, want saved draft and definition entries", definitions)
	}
	instances := deps.ListStrategyInstances()
	if len(instances) != 1 || instances[0].Market != "US" || instances[0].AccountID != "acct-1" {
		t.Fatalf("ListStrategyInstances = %#v, want broker-bound instance summary", instances)
	}

	if _, err := deps.UpdateStrategyInstanceMode("missing-instance", strategyExecutionModeNotifyOnly); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("UpdateStrategyInstanceMode(missing) error = %v, want not found", err)
	}
	updatedAny, err := deps.UpdateStrategyInstanceMode(instance.ID, strategyExecutionModeNotifyOnly)
	if err != nil {
		t.Fatalf("UpdateStrategyInstanceMode(valid): %v", err)
	}
	updated := updatedAny.(strategyListItem)
	if updated.Binding.ExecutionMode != strategyExecutionModeNotifyOnly {
		t.Fatalf("updated.Binding.ExecutionMode = %q, want %q", updated.Binding.ExecutionMode, strategyExecutionModeNotifyOnly)
	}

	useExtendedHours := true
	run := &backtestRunState{
		ID:     "bt-adk-closure",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:      definition.ID,
			DefinitionVersion: definition.Version,
			Market:            "US",
			Code:              "TME",
			Symbol:            "US.TME",
			Interval:          "5m",
			StartTime:         "2025-01-01T09:30:00Z",
			EndTime:           "2025-01-01T10:00:00Z",
			InitialBalance:    100000,
			RehabType:         "forward",
			UseExtendedHours:  &useExtendedHours,
		},
		Result: &backtest.RunResult{
			QuoteCurrency: "USD",
			FinalBalance:  101500,
			PnL:           1500,
			Logs:          []string{"first line", "second line"},
		},
		CreatedAt: "2025-01-01T09:30:00Z",
		UpdatedAt: "2025-01-01T10:00:00Z",
	}
	if err := server.backtestRuns.add(run); err != nil {
		t.Fatalf("backtestRuns.add: %v", err)
	}

	runs := deps.ListBacktestRuns()
	if len(runs) != 1 || runs[0].ID != "bt-adk-closure" || runs[0].UseExtendedHours == nil || *runs[0].UseExtendedHours != true {
		t.Fatalf("ListBacktestRuns = %#v, want summarized stored run", runs)
	}
	view, err := deps.BacktestResultView(BacktestResultViewInput{RunID: "bt-adk-closure", View: "summary", Limit: 1})
	if err != nil {
		t.Fatalf("BacktestResultView(valid): %v", err)
	}
	viewPayload := view.(map[string]any)
	runPayload := viewPayload["run"].(map[string]any)
	if runPayload["status"] != "completed" {
		t.Fatalf("BacktestResultView run = %#v, want completed status", runPayload)
	}
	if _, err := deps.BacktestResultView(BacktestResultViewInput{}); err == nil {
		t.Fatal("BacktestResultView(empty) error = nil, want validation failure")
	}

	if _, err := deps.EnsureBacktestData(nil, BacktestStartInput{Market: "US", Symbol: "US.TME", Interval: "5m"}); err == nil || !strings.Contains(err.Error(), "definitionIds") {
		t.Fatalf("EnsureBacktestData(empty) error = %v, want definitionIds validation", err)
	}
	if _, err := deps.EnsureResearchBacktestData(ResearchBacktestInput{}); err == nil || !strings.Contains(err.Error(), "script") {
		t.Fatalf("EnsureResearchBacktestData(empty) error = %v, want script validation", err)
	}
	if _, err := deps.EnqueueBacktest(BacktestStartInput{DefinitionID: "missing"}); err == nil {
		t.Fatal("EnqueueBacktest(missing definition) error = nil, want failure")
	}
	if _, err := deps.StartResearchBacktest(ResearchBacktestInput{}); err == nil || !strings.Contains(err.Error(), "script") {
		t.Fatalf("StartResearchBacktest(empty) error = %v, want script validation", err)
	}

	cancelled := false
	server.backtestRuns.setCancel("bt-adk-closure", func() { cancelled = true })
	deps.CancelBacktest("bt-adk-closure")
	if !cancelled {
		t.Fatal("CancelBacktest() did not delegate to run store cancel function")
	}

	deps.RecordAudit(context.Background(), "adapter.test", "subject-1", "adapter audit recorded", map[string]any{"source": "test"})
	events, err := server.adkRuntime.Store().ListAuditEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(events) == 0 || events[0].Kind != "adapter.test" || events[0].SubjectID != "subject-1" {
		t.Fatalf("audit events = %#v, want recorded adapter audit event", events)
	}
}

func TestADKAdapterHelpersAndOptimizationRuns(t *testing.T) {
	if got := brokerBindingMarket(nil); got != "" {
		t.Fatalf("brokerBindingMarket(nil) = %q, want empty string", got)
	}
	if got := brokerBindingAccountID(nil); got != "" {
		t.Fatalf("brokerBindingAccountID(nil) = %q, want empty string", got)
	}
	binding := &strategyBrokerAccountBinding{Market: "US", AccountID: "acct-1"}
	if got := brokerBindingMarket(binding); got != "US" {
		t.Fatalf("brokerBindingMarket(binding) = %q, want US", got)
	}
	if got := brokerBindingAccountID(binding); got != "acct-1" {
		t.Fatalf("brokerBindingAccountID(binding) = %q, want acct-1", got)
	}

	if got := backtestRunSummaryFromSrvRun(nil); got != (BacktestRunSummary{}) {
		t.Fatalf("backtestRunSummaryFromSrvRun(nil) = %#v, want zero value", got)
	}

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	run := &backtestRunState{
		ID:     "bt-opt",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:   "def-1",
			Market:         "US",
			Symbol:         "US.AAPL",
			Interval:       "1m",
			InitialBalance: 10000,
		},
		Result:    &backtest.RunResult{PnL: 10},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-01T00:01:00Z",
	}
	if err := server.backtestRuns.add(run); err != nil {
		t.Fatalf("backtestRuns.add: %v", err)
	}

	optRuns := assistantOptimizationRuns{server: server}
	if _, ok := optRuns.Get("missing"); ok {
		t.Fatal("OptimizationRuns.Get(missing) = true, want false")
	}
	got, ok := optRuns.Get("bt-opt")
	result, resultOK := got.Result.(*backtest.RunResult)
	if !ok || got.Status != "completed" || !resultOK || result == nil || result.PnL != 10 {
		t.Fatalf("OptimizationRuns.Get(bt-opt) = %#v ok=%v, want completed result", got, ok)
	}

	cancelled := false
	server.backtestRuns.setCancel("bt-opt", func() { cancelled = true })
	optRuns.Cancel("bt-opt")
	if !cancelled {
		t.Fatal("OptimizationRuns.Cancel() did not delegate to backtest cancellation")
	}

	emptyRuns := assistantOptimizationRuns{}
	if _, ok := emptyRuns.Get("bt-opt"); ok {
		t.Fatal("OptimizationRuns.Get on nil server = true, want false")
	}
	emptyRuns.Cancel("bt-opt")

	if zero := (asst.OptimizationRun{}); zero.Status != "" {
		t.Fatalf("zero optimization run changed unexpectedly: %#v", zero)
	}
}
