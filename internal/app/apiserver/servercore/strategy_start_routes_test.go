package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestStartStrategyRouteRejectsNotStartableInstance(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if err := server.strategyStore.saveStrategy(managedStrategyInstance{
		ID:       "legacy-instance",
		PluginID: "legacy-runtime",
		Definition: strategyDefinitionSummary{
			StrategyID: "legacy-definition",
			Name:       "Legacy Definition",
			Version:    "0.1.0",
		},
		Params: map[string]any{
			"runtime":      "legacy-runtime",
			"sourceFormat": "legacy-source",
			"script":       "function onInit(ctx) {}",
		},
		Status: strategyStatusStopped,
	}); err != nil {
		t.Fatalf("save legacy strategy: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/v1/strategies/legacy-instance/start", "application/json", nil)
	if err != nil {
		t.Fatalf("POST start: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST start status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode start error: %v", err)
	}
	if envelope.OK || envelope.Error.Code != "BAD_REQUEST" || envelope.Error.Message != "strategy runtime legacy-runtime is not startable yet" {
		t.Fatalf("unexpected start error envelope: %#v", envelope)
	}
}
