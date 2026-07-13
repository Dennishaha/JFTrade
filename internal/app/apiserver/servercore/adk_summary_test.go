package servercore

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestADKStrategyInstanceSummariesIncludeFallbackDefinitionAndRuntimeState(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	observedID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL", "US.MSFT"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeLive,
		BrokerAccount: &strategyBrokerAccountBinding{
			BrokerID: "futu", AccountID: "123456", TradingEnvironment: "SIMULATE", Market: "US",
		},
	})
	plainID := instantiateStrategyRuntimeTestInstanceWithDefinitionID(t, server, "runtime-test-plain", strategyInstanceBinding{
		Symbols:       []string{"HK.00700"},
		Interval:      "15m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})

	observed, ok := server.strategyStore.strategy(observedID)
	if !ok {
		t.Fatalf("strategy(%s) not found", observedID)
	}
	observed.Definition.StrategyID = " "
	observed.Params["definitionId"] = "fallback-definition"
	if err := server.strategyStore.saveStrategy(observed); err != nil {
		t.Fatalf("saveStrategy observed: %v", err)
	}
	if _, err := server.strategyStore.transitionStrategy(observedID, strategyStatusRunning, "started", "test start"); err != nil {
		t.Fatalf("transitionStrategy: %v", err)
	}
	if err := server.strategyStore.appendStrategyRuntimeEvent(observedID, "strategy started", "runtime.started", "boot"); err != nil {
		t.Fatalf("appendStrategyRuntimeEvent(start): %v", err)
	}
	if err := server.strategyStore.appendStrategyRuntimeEvent(observedID, "broker rejected order", "runtime.error", "reject"); err != nil {
		t.Fatalf("appendStrategyRuntimeEvent(error): %v", err)
	}

	now := strategyRuntimeTestTime(10, 5, 0)
	if err := server.strategyRuntimeStore.UpsertObservation(t.Context(), strategyRuntimeObservationSnapshot{
		InstanceID:    observedID,
		ActualStatus:  "running",
		ActiveSymbols: []string{"US.AAPL", "US.MSFT"},
		LastErrorAt:   &now,
		LastError:     " broker rejected order ",
		UpdatedAt:     &now,
	}); err != nil {
		t.Fatalf("UpsertObservation: %v", err)
	}

	summaries := server.adkStrategyInstanceSummaries()
	if len(summaries) != 2 {
		t.Fatalf("summary count=%d want=2 summaries=%+v", len(summaries), summaries)
	}

	var observedSummary StrategyInstanceSummary
	var plainSummary StrategyInstanceSummary
	for _, summary := range summaries {
		switch summary.ID {
		case observedID:
			observedSummary = summary
		case plainID:
			plainSummary = summary
		}
	}

	if observedSummary.DefinitionID != "fallback-definition" || observedSummary.ActualStatus != strategyStatusRunning {
		t.Fatalf("observed summary=%+v want fallback definition id and running actual status", observedSummary)
	}
	if observedSummary.Market != "US" || observedSummary.AccountID != "123456" {
		t.Fatalf("observed broker summary=%+v want US/123456", observedSummary)
	}
	if observedSummary.LogCount < 1 || observedSummary.LastError != "broker rejected order" || observedSummary.LatestLog == "" {
		t.Fatalf("observed logs/errors=%+v want persisted log tail and trimmed last error", observedSummary)
	}
	if observedSummary.Status != strategyStatusRunning || !strings.Contains(observedSummary.LatestLog, "strategy") {
		t.Fatalf("observed runtime summary=%+v want running status and non-empty persisted latest log tail", observedSummary)
	}
	if len(observedSummary.ActiveSymbols) != 2 || observedSummary.ActiveSymbols[0] != "US.AAPL" || len(observedSummary.Symbols) != 2 {
		t.Fatalf("observed symbols=%+v active=%+v want copied symbols and active symbols", observedSummary.Symbols, observedSummary.ActiveSymbols)
	}

	if plainSummary.ID != plainID || plainSummary.DefinitionID == "" {
		t.Fatalf("plain summary=%+v want populated base summary", plainSummary)
	}
	if plainSummary.ActualStatus != "" || plainSummary.LastError != "" {
		t.Fatalf("plain runtime summary=%+v want empty runtime observation fields without persisted observation", plainSummary)
	}
	if plainSummary.Market != "" || plainSummary.AccountID != "" || len(plainSummary.ActiveSymbols) != 0 || plainSummary.LogCount < 1 || plainSummary.LatestLog == "" {
		t.Fatalf("plain broker summary=%+v want default broker fields and persisted instantiate log", plainSummary)
	}
}
