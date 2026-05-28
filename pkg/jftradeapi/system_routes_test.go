package jftradeapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemStatusEndpointReturnsStatus(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if err := server.strategyStore.saveStrategy(managedStrategyInstance{
		ID:       "instance-running",
		PluginID: "demo-plugin",
		Definition: strategyDefinitionSummary{
			StrategyID: "demo-plugin",
			Name:       "Demo Plugin",
			Version:    "1.0.0",
		},
		Status: strategyStatusRunning,
	}); err != nil {
		t.Fatalf("saveStrategy: %v", err)
	}
	srv := httptest.NewServer(server)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET system status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET system status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode system status: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if got := envelope.Data["name"]; got != "JFTrade" {
		t.Fatalf("system name = %v", got)
	}
	if _, ok := envelope.Data["broker"]; !ok {
		t.Fatal("expected broker in system status response")
	}
	strategyRuntime, ok := envelope.Data["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", envelope.Data["strategyRuntime"])
	}
	if got := int(strategyRuntime["activeStrategies"].(float64)); got != 0 {
		t.Fatalf("activeStrategies = %d", got)
	}
	activeInstances, ok := strategyRuntime["activeInstances"].([]any)
	if !ok {
		t.Fatalf("expected activeInstances list, got %+v", strategyRuntime["activeInstances"])
	}
	if len(activeInstances) != 0 {
		t.Fatalf("expected no active runtime instances, got %+v", activeInstances)
	}
}

func TestNewServerReconcilesPersistedActiveStrategyStates(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	initialStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore initial: %v", err)
	}
	initialServer := NewServer(initialStore)
	if err := initialServer.strategyStore.saveStrategy(managedStrategyInstance{
		ID:       "instance-running",
		PluginID: "demo-plugin",
		Definition: strategyDefinitionSummary{
			StrategyID: "demo-plugin",
			Name:       "Demo Plugin",
			Version:    "1.0.0",
		},
		Status: strategyStatusRunning,
	}); err != nil {
		t.Fatalf("saveStrategy: %v", err)
	}

	reloadedStore, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore reload: %v", err)
	}
	reloadedServer := NewServer(reloadedStore)

	strategy, ok := reloadedServer.strategyStore.strategy("instance-running")
	if !ok {
		t.Fatal("expected reconciled strategy to exist")
	}
	if strategy.Status != strategyStatusStopped {
		t.Fatalf("reconciled status = %s, want %s", strategy.Status, strategyStatusStopped)
	}
	if len(strategy.Logs) == 0 || !strings.Contains(strategy.Logs[len(strategy.Logs)-1], "reconciled strategy state") {
		t.Fatalf("expected reconciliation log, got %+v", strategy.Logs)
	}
	audit, ok := reloadedServer.strategyStore.strategyAudit("instance-running")
	if !ok {
		t.Fatal("expected reconciled strategy audit to exist")
	}
	found := false
	for _, entry := range audit.Entries {
		if entry.Kind == "reconciled" && strings.Contains(entry.Detail, "stale running state") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected reconciliation audit entry, got %+v", audit.Entries)
	}

	strategyRuntime, ok := reloadedServer.systemStatus()["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", reloadedServer.systemStatus()["strategyRuntime"])
	}
	if got := int(strategyRuntime["activeStrategies"].(int)); got != 0 {
		t.Fatalf("activeStrategies after restart = %d, want 0", got)
	}
	if got := strategyRuntime["status"]; got != "idle" {
		t.Fatalf("runtime status after restart = %v, want idle", got)
	}
}
