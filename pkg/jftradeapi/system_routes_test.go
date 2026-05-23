package jftradeapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	if got := int(strategyRuntime["activeStrategies"].(float64)); got != 1 {
		t.Fatalf("activeStrategies = %d", got)
	}
}
