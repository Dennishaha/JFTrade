package jftradeapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestShouldStartForAPIOnlyArgs(t *testing.T) {
	if !shouldStartForArgs([]string{"api"}) {
		t.Fatal("expected api command to start JFTrade sidecar")
	}
	if !shouldStartForArgs([]string{"serve-api"}) {
		t.Fatal("expected serve-api command to start JFTrade sidecar")
	}
	if !shouldStartForArgs([]string{"run", "--config", "./config/jftrade.yaml"}) {
		t.Fatal("expected bbgo run command to start JFTrade sidecar")
	}
}

func TestBrokerRuntimeDescriptorIncludesReadFeatures(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/brokers/futu/runtime")
	if err != nil {
		t.Fatalf("GET broker runtime: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET broker runtime status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode broker runtime: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected broker runtime ok=true")
	}

	descriptor, ok := envelope.Data["descriptor"].(map[string]any)
	if !ok {
		t.Fatalf("descriptor = %#v", envelope.Data["descriptor"])
	}
	capabilities, ok := descriptor["capabilities"].([]any)
	if !ok || len(capabilities) == 0 {
		t.Fatalf("capabilities = %#v", descriptor["capabilities"])
	}
	firstCapability, ok := capabilities[0].(map[string]any)
	if !ok {
		t.Fatalf("first capability = %#v", capabilities[0])
	}
	readFeatures, ok := firstCapability["readFeatures"].(map[string]any)
	if !ok {
		t.Fatalf("readFeatures = %#v", firstCapability["readFeatures"])
	}
	marginRatios, ok := readFeatures["marginRatios"].(map[string]any)
	if !ok {
		t.Fatalf("marginRatios capability = %#v", readFeatures["marginRatios"])
	}
	environments, ok := marginRatios["supportedEnvironments"].([]any)
	if !ok || len(environments) != 1 || environments[0] != "REAL" {
		t.Fatalf("marginRatios supportedEnvironments = %#v", marginRatios["supportedEnvironments"])
	}
	maxTradeQuantity, ok := readFeatures["maxTradeQuantity"].(map[string]any)
	if !ok {
		t.Fatalf("maxTradeQuantity capability = %#v", readFeatures["maxTradeQuantity"])
	}
	if got := maxTradeQuantity["requiresPrice"]; got != true {
		t.Fatalf("maxTradeQuantity requiresPrice = %#v, want true", got)
	}
}

func TestNewServerUsesStrategyRuntimeDBEnvOverride(t *testing.T) {
	customRuntimeDBPath := filepath.Join(t.TempDir(), "custom", "strategy-runtime-override.db")
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", customRuntimeDBPath)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if server.strategyRuntimeStore == nil {
		t.Fatal("expected strategy runtime store to be initialized with env override")
	}
	if _, err := os.Stat(customRuntimeDBPath); err != nil {
		t.Fatalf("expected runtime db file at env override path, got error: %v", err)
	}
	if got := deriveStrategyRuntimeDBPath(store.path); got != customRuntimeDBPath {
		t.Fatalf("deriveStrategyRuntimeDBPath() = %s, want %s", got, customRuntimeDBPath)
	}
}
