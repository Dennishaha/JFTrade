package jftradeapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrategyRuntimeObservationAppearsInStrategiesAndSystemStatus(t *testing.T) {
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
		ExecutionMode: strategyExecutionModeNotifyOnly,
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

	srv := httptest.NewServer(server)
	defer srv.Close()

	strategiesResp, err := http.Get(srv.URL + "/api/v1/strategies")
	if err != nil {
		t.Fatalf("GET strategies: %v", err)
	}
	defer strategiesResp.Body.Close()
	var strategiesEnvelope struct {
		OK   bool               `json:"ok"`
		Data []strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(strategiesResp.Body).Decode(&strategiesEnvelope); err != nil {
		t.Fatalf("decode strategies response: %v", err)
	}
	if len(strategiesEnvelope.Data) != 1 {
		t.Fatalf("expected 1 strategy item, got %+v", strategiesEnvelope.Data)
	}
	observation := strategiesEnvelope.Data[0].RuntimeObservation
	if observation == nil {
		t.Fatalf("expected runtime observation in strategies response, got %+v", strategiesEnvelope.Data[0])
	}
	if observation.ActualStatus != strategyStatusRunning {
		t.Fatalf("actualStatus = %s, want %s", observation.ActualStatus, strategyStatusRunning)
	}
	if len(observation.ActiveSymbols) != 1 || observation.ActiveSymbols[0] != "US.AAPL" {
		t.Fatalf("unexpected activeSymbols: %+v", observation.ActiveSymbols)
	}
	if observation.LastClosedKLineAt == nil || observation.LastSignalAt == nil {
		t.Fatalf("expected lastClosedKlineAt and lastSignalAt, got %+v", observation)
	}
	if observation.LastOrderAt != nil {
		t.Fatalf("notify_only should not have lastOrderAt, got %+v", observation.LastOrderAt)
	}

	statusResp, err := http.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET system status: %v", err)
	}
	defer statusResp.Body.Close()
	var statusEnvelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(statusResp.Body).Decode(&statusEnvelope); err != nil {
		t.Fatalf("decode system status response: %v", err)
	}
	strategyRuntime, ok := statusEnvelope.Data["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", statusEnvelope.Data["strategyRuntime"])
	}
	if got := int(strategyRuntime["activeStrategies"].(float64)); got != 1 {
		t.Fatalf("activeStrategies = %d, want 1", got)
	}
	activeInstances, ok := strategyRuntime["activeInstances"].([]any)
	if !ok || len(activeInstances) != 1 {
		t.Fatalf("expected 1 active runtime instance, got %+v", strategyRuntime["activeInstances"])
	}
	activeInstance, ok := activeInstances[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected active instance summary: %+v", activeInstances[0])
	}
	if activeInstance["instanceId"] != instanceID {
		t.Fatalf("unexpected active instance id: %+v", activeInstance)
	}
	if activeInstance["lastClosedKlineAt"] == nil || activeInstance["lastSignalAt"] == nil {
		t.Fatalf("expected runtime timestamps in active instance summary, got %+v", activeInstance)
	}
}

func TestStrategyRuntimeObservationPersistsAcrossServerRestart(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return stub }

	instanceID := instantiateStrategyRuntimeTestInstance(t, server, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
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
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))
	if _, err := server.strategyStore.transitionStrategy(instanceID, strategyStatusStopped, "stopped", "test stop"); err != nil {
		t.Fatalf("transitionStrategy stop: %v", err)
	}
	server.strategyRuntimeManager.stopStrategy(instanceID)

	reloadedStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore reload: %v", err)
	}
	reloadedServer := NewServer(reloadedStore)
	strategies := reloadedServer.enrichStrategyItems(reloadedServer.strategyStore.strategies())
	if len(strategies) != 1 {
		t.Fatalf("expected 1 strategy after reload, got %+v", strategies)
	}
	observation := strategies[0].RuntimeObservation
	if observation == nil {
		t.Fatalf("expected persisted runtime observation after reload, got %+v", strategies[0])
	}
	if observation.ActualStatus != strategyStatusStopped {
		t.Fatalf("persisted actual status = %s, want %s", observation.ActualStatus, strategyStatusStopped)
	}
	if observation.LastClosedKLineAt == nil || observation.LastSignalAt == nil {
		t.Fatalf("expected persisted timestamps after reload, got %+v", observation)
	}
	strategyRuntime, ok := reloadedServer.systemStatus()["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", reloadedServer.systemStatus()["strategyRuntime"])
	}
	if got := strategyRuntime["activeStrategies"].(int); got != 0 {
		t.Fatalf("activeStrategies after reload = %d, want 0", got)
	}
}

func TestStrategyRuntimePanicAutoReconcilesToStopped(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	stub := newStrategyRuntimeStubExchange()
	stub.panicOnPlaceOrder = true
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

	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 100, strategyRuntimeTestTime(10, 0, 30)))
	server.strategyRuntimeManager.handleMarketTrade(strategyRuntimeTestTrade("US.AAPL", 101, strategyRuntimeTestTime(10, 1, 0)))

	strategy, ok := server.strategyStore.strategy(instanceID)
	if !ok {
		t.Fatalf("strategy(%s) not found after panic reconciliation", instanceID)
	}
	if strategy.Status != strategyStatusStopped {
		t.Fatalf("strategy status after panic = %s, want %s", strategy.Status, strategyStatusStopped)
	}
	if got := len(server.strategyRuntimeManager.activeInstrumentIDs()); got != 0 {
		t.Fatalf("expected runtime manager to remove failed runtime, got %d active instruments", got)
	}

	notifications := server.liveNotificationsAfter(0)
	foundNotification := false
	for _, note := range notifications {
		if note.Title == "策略运行异常退出" {
			foundNotification = true
			break
		}
	}
	if !foundNotification {
		t.Fatalf("expected runtime exit notification, got %+v", notifications)
	}

	audit, ok := server.strategyStore.strategyAudit(instanceID)
	if !ok {
		t.Fatalf("strategyAudit(%s) not found", instanceID)
	}
	foundExitAudit := false
	for _, entry := range audit.Entries {
		if entry.Kind == "runtime_exited" && strings.Contains(entry.Detail, "broker submit panic") {
			foundExitAudit = true
			break
		}
	}
	if !foundExitAudit {
		t.Fatalf("expected runtime_exited audit entry, got %+v", audit.Entries)
	}

	strategyRuntime, ok := server.systemStatus()["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", server.systemStatus()["strategyRuntime"])
	}
	if got := strategyRuntime["activeStrategies"].(int); got != 0 {
		t.Fatalf("activeStrategies after panic = %d, want 0", got)
	}
}
