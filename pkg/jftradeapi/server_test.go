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

func TestMarketDataSubscriptionHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	postJSON := func(path string, payload map[string]any) map[string]any {
		body, _ := json.Marshal(payload)
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s status = %d", path, resp.StatusCode)
		}
		var envelope struct {
			OK   bool           `json:"ok"`
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if !envelope.OK {
			t.Fatalf("POST %s returned ok=false", path)
		}
		return envelope.Data
	}

	data := postJSON("/api/v1/market-data/subscriptions", map[string]any{
		"channel":    "KLINE",
		"market":     "HK",
		"symbol":     "00700",
		"interval":   "1m",
		"consumerId": "chart-main",
	})
	if got := int(data["totalActiveSubscriptions"].(float64)); got != 1 {
		t.Fatalf("totalActiveSubscriptions after acquire = %d", got)
	}

	data = postJSON("/api/v1/market-data/subscriptions/heartbeat", map[string]any{"consumerId": "chart-main"})
	if got := int(data["totalActiveSubscriptions"].(float64)); got != 1 {
		t.Fatalf("totalActiveSubscriptions after heartbeat = %d", got)
	}

	data = postJSON("/api/v1/market-data/subscriptions/release", map[string]any{
		"channel":    "KLINE",
		"market":     "HK",
		"symbol":     "00700",
		"interval":   "1m",
		"consumerId": "chart-main",
	})
	if got := int(data["totalActiveSubscriptions"].(float64)); got != 0 {
		t.Fatalf("totalActiveSubscriptions after release = %d", got)
	}
}

func TestStrategiesEndpointReturnsList(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if err := server.strategyStore.saveStrategy(managedStrategyInstance{
		ID:       "instance-1",
		PluginID: "mean-revert",
		Definition: strategyDefinitionSummary{
			StrategyID: "mean-revert",
			Name:       "Mean Revert",
			Version:    "1.0.0",
		},
		Params:    map[string]any{"window": 20},
		Status:    strategyStatusRunning,
		CreatedAt: "2026-05-22T00:00:00Z",
		Logs:      []string{"started"},
		AuditEntries: []strategyAuditEntry{{
			InstanceID: "instance-1",
			Kind:       "started",
			Detail:     "mean-revert",
			At:         "2026-05-22T00:00:00Z",
		}},
	}); err != nil {
		t.Fatalf("saveStrategy: %v", err)
	}
	srv := httptest.NewServer(server)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/strategies")
	if err != nil {
		t.Fatalf("GET strategies: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET strategies status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool  `json:"ok"`
		Data []any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode strategies: %v", err)
	}
	if !envelope.OK || envelope.Data == nil {
		t.Fatalf("unexpected strategies response: %+v", envelope)
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("expected 1 strategy, got %d", len(envelope.Data))
	}

	logsResp, err := http.Get(srv.URL + "/api/v1/strategies/instance-1/logs")
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	defer logsResp.Body.Close()
	if logsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET logs status = %d", logsResp.StatusCode)
	}
	var logsEnvelope struct {
		OK   bool                 `json:"ok"`
		Data strategyLogsResponse `json:"data"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsEnvelope); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	if len(logsEnvelope.Data.Logs) != 1 || logsEnvelope.Data.Logs[0] != "started" {
		t.Fatalf("unexpected logs response: %+v", logsEnvelope.Data)
	}

	auditResp, err := http.Get(srv.URL + "/api/v1/strategies/instance-1/audit")
	if err != nil {
		t.Fatalf("GET audit: %v", err)
	}
	defer auditResp.Body.Close()
	if auditResp.StatusCode != http.StatusOK {
		t.Fatalf("GET audit status = %d", auditResp.StatusCode)
	}
	var auditEnvelope struct {
		OK   bool                  `json:"ok"`
		Data strategyAuditResponse `json:"data"`
	}
	if err := json.NewDecoder(auditResp.Body).Decode(&auditEnvelope); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if len(auditEnvelope.Data.Entries) != 1 || auditEnvelope.Data.Entries[0].Kind != "started" {
		t.Fatalf("unexpected audit response: %+v", auditEnvelope.Data)
	}
}

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

func TestStrategyDefinitionEndpoints(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	payload := map[string]any{
		"id":          "js-mean-revert",
		"name":        "JS Mean Revert",
		"version":     "0.1.0",
		"description": "quickjs strategy",
		"runtime":     strategyRuntimeQuickJS,
		"symbol":      "00700",
		"interval":    "1m",
		"script":      "function onInit(ctx) { console.log(ctx.name); }",
		"visualModel": map[string]any{
			"engine":  "logic-flow",
			"version": 1,
			"nodes": []map[string]any{
				{
					"id":   "on-kline-root",
					"type": "circle",
					"x":    180,
					"y":    300,
					"text": "K 线收盘",
					"properties": map[string]any{
						"blockKind": "onKLineClosed",
					},
				},
			},
			"edges": []map[string]any{},
		},
	}
	body, _ := json.Marshal(payload)
	createResp, err := http.Post(srv.URL+"/api/v1/strategy-definitions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST strategy definition: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST strategy definition status = %d", createResp.StatusCode)
	}

	listResp, err := http.Get(srv.URL + "/api/v1/strategy-definitions")
	if err != nil {
		t.Fatalf("GET strategy definitions: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET strategy definitions status = %d", listResp.StatusCode)
	}
	var listEnvelope struct {
		OK   bool                       `json:"ok"`
		Data []strategyDesignDefinition `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode strategy definitions: %v", err)
	}
	if len(listEnvelope.Data) != 1 || listEnvelope.Data[0].ID != "js-mean-revert" {
		t.Fatalf("unexpected definitions response: %+v", listEnvelope.Data)
	}

	detailResp, err := http.Get(srv.URL + "/api/v1/strategy-definitions/js-mean-revert")
	if err != nil {
		t.Fatalf("GET strategy definition detail: %v", err)
	}
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("GET strategy definition detail status = %d", detailResp.StatusCode)
	}
	var detailEnvelope struct {
		OK   bool                     `json:"ok"`
		Data strategyDesignDefinition `json:"data"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailEnvelope); err != nil {
		t.Fatalf("decode strategy definition detail: %v", err)
	}
	if detailEnvelope.Data.Runtime != strategyRuntimeQuickJS {
		t.Fatalf("unexpected strategy runtime: %+v", detailEnvelope.Data)
	}
	if detailEnvelope.Data.VisualModel == nil || len(detailEnvelope.Data.VisualModel.Nodes) != 1 {
		t.Fatalf("unexpected visual model: %+v", detailEnvelope.Data.VisualModel)
	}

	payload["description"] = "updated quickjs strategy"
	updateBody, _ := json.Marshal(payload)
	request, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/strategy-definitions/js-mean-revert", bytes.NewReader(updateBody))
	if err != nil {
		t.Fatalf("build PUT request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT strategy definition: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT strategy definition status = %d", updateResp.StatusCode)
	}
	var updateEnvelope struct {
		OK   bool                     `json:"ok"`
		Data strategyDesignDefinition `json:"data"`
	}
	if err := json.NewDecoder(updateResp.Body).Decode(&updateEnvelope); err != nil {
		t.Fatalf("decode updated strategy definition: %v", err)
	}
	if updateEnvelope.Data.Description != "updated quickjs strategy" {
		t.Fatalf("unexpected updated definition: %+v", updateEnvelope.Data)
	}
}

func TestInstantiateAndTransitionQuickJSStrategyInstance(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:       "js-breakout",
		Name:     "JS Breakout",
		Version:  "0.1.0",
		Runtime:  strategyRuntimeQuickJS,
		Symbol:   "00700",
		Interval: "1m",
		Script:   "function onKLineClosed(ctx) { console.log(ctx.kline.close); }",
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	srv := httptest.NewServer(server)
	defer srv.Close()

	createResp, err := http.Post(srv.URL+"/api/v1/strategy-definitions/js-breakout/instantiate", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST instantiate: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST instantiate status = %d", createResp.StatusCode)
	}
	var createEnvelope struct {
		OK   bool             `json:"ok"`
		Data strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode instantiate: %v", err)
	}
	if createEnvelope.Data.Status != strategyStatusStopped {
		t.Fatalf("unexpected instantiated status: %+v", createEnvelope.Data)
	}
	instanceID := createEnvelope.Data.ID

	assertTransition := func(action string, expectedStatus string) {
		resp, err := http.Post(srv.URL+"/api/v1/strategies/"+instanceID+"/"+action, "application/json", bytes.NewReader([]byte(`{}`)))
		if err != nil {
			t.Fatalf("POST %s: %v", action, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s status = %d", action, resp.StatusCode)
		}
		var envelope struct {
			OK   bool             `json:"ok"`
			Data strategyListItem `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode %s: %v", action, err)
		}
		if envelope.Data.Status != expectedStatus {
			t.Fatalf("%s expected status %s, got %+v", action, expectedStatus, envelope.Data)
		}
	}

	assertTransition("start", strategyStatusRunning)
	assertTransition("pause", strategyStatusPaused)
	assertTransition("stop", strategyStatusStopped)

	logsResp, err := http.Get(srv.URL + "/api/v1/strategies/" + instanceID + "/logs")
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	defer logsResp.Body.Close()
	var logsEnvelope struct {
		OK   bool                 `json:"ok"`
		Data strategyLogsResponse `json:"data"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsEnvelope); err != nil {
		t.Fatalf("decode logs after transitions: %v", err)
	}
	if len(logsEnvelope.Data.Logs) < 4 {
		t.Fatalf("expected lifecycle logs, got %+v", logsEnvelope.Data.Logs)
	}
}
