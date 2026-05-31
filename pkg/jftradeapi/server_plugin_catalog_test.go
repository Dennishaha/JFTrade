package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPluginCatalogLifecycleEndpoints(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if err := server.strategyStore.savePlugin(managedStrategyPlugin{
		Descriptor: strategyPluginDescriptor{
			ID:          "demo-plugin",
			Type:        pluginTypeGoStrategy,
			DisplayName: "Demo Plugin",
			Version:     "1.0.0",
			Description: "demo dynamic plugin",
			Keywords:    []string{"strategy", "go-plugin"},
		},
		Artifact: &strategyPluginArtifact{
			Build: strategyPluginBuildTuple{
				JFTradeVersion: "legacy-version",
				GoVersion:      runtime.Version(),
				GOOS:           runtime.GOOS,
				GOARCH:         runtime.GOARCH,
				BuildMode:      pluginBuildMode,
			},
		},
	}); err != nil {
		t.Fatalf("savePlugin: %v", err)
	}
	srv := httptest.NewServer(server)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/plugins")
	if err != nil {
		t.Fatalf("GET plugins: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET plugins status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool                          `json:"ok"`
		Data strategyPluginCatalogResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode plugins: %v", err)
	}
	if len(envelope.Data.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(envelope.Data.Plugins))
	}
	if !envelope.Data.Plugins[0].Compatibility.RequiresRebuild {
		t.Fatalf("expected plugin to require rebuild: %+v", envelope.Data.Plugins[0].Compatibility)
	}

	installResp, err := http.Post(srv.URL+"/api/v1/plugins/demo-plugin/install", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST install: %v", err)
	}
	defer installResp.Body.Close()
	if installResp.StatusCode != http.StatusOK {
		t.Fatalf("POST install status = %d", installResp.StatusCode)
	}
	var installEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Operation strategyPluginOperation `json:"operation"`
		} `json:"data"`
	}
	if err := json.NewDecoder(installResp.Body).Decode(&installEnvelope); err != nil {
		t.Fatalf("decode install: %v", err)
	}
	if installEnvelope.Data.Operation.Status != "SUCCEEDED" {
		t.Fatalf("unexpected install operation: %+v", installEnvelope.Data.Operation)
	}

	opResp, err := http.Get(srv.URL + "/api/v1/plugins/operations/" + installEnvelope.Data.Operation.OperationID)
	if err != nil {
		t.Fatalf("GET operation: %v", err)
	}
	defer opResp.Body.Close()
	if opResp.StatusCode != http.StatusOK {
		t.Fatalf("GET operation status = %d", opResp.StatusCode)
	}

	guidanceResp, err := http.Get(srv.URL + "/api/v1/plugins/demo-plugin/uninstall-guidance")
	if err != nil {
		t.Fatalf("GET uninstall guidance: %v", err)
	}
	defer guidanceResp.Body.Close()
	if guidanceResp.StatusCode != http.StatusOK {
		t.Fatalf("GET uninstall guidance status = %d", guidanceResp.StatusCode)
	}

	uninstallResp, err := http.Post(srv.URL+"/api/v1/plugins/demo-plugin/uninstall", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST uninstall: %v", err)
	}
	defer uninstallResp.Body.Close()
	if uninstallResp.StatusCode != http.StatusOK {
		t.Fatalf("POST uninstall status = %d", uninstallResp.StatusCode)
	}
	var uninstallEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Operation strategyPluginOperation `json:"operation"`
		} `json:"data"`
	}
	if err := json.NewDecoder(uninstallResp.Body).Decode(&uninstallEnvelope); err != nil {
		t.Fatalf("decode uninstall: %v", err)
	}
	if uninstallEnvelope.Data.Operation.Status != "SUCCEEDED" {
		t.Fatalf("unexpected uninstall operation: %+v", uninstallEnvelope.Data.Operation)
	}
}
