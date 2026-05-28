package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
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
		"id":           "dsl-mean-revert",
		"name":         "DSL Mean Revert",
		"version":      "0.1.0",
		"description":  "dsl strategy",
		"runtime":      strategyRuntimeDSLPlan,
		"sourceFormat": strategydefinition.SourceFormatDSLV1,
		"symbol":       "00700",
		"interval":     "1m",
		"script":       "strategy DSL Mean Revert\nversion 0.1.0\non init:\n  log \"init\"\non kline_close:\n  let slow = ma(EMA, 2, hour)\n  log \"close\"",
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
	if len(listEnvelope.Data) != 1 || listEnvelope.Data[0].ID != "dsl-mean-revert" {
		t.Fatalf("unexpected definitions response: %+v", listEnvelope.Data)
	}

	detailResp, err := http.Get(srv.URL + "/api/v1/strategy-definitions/dsl-mean-revert")
	if err != nil {
		t.Fatalf("GET strategy definition detail: %v", err)
	}
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("GET strategy definition detail status = %d", detailResp.StatusCode)
	}
	var detailEnvelope struct {
		OK   bool                       `json:"ok"`
		Data strategyDefinitionResponse `json:"data"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailEnvelope); err != nil {
		t.Fatalf("decode strategy definition detail: %v", err)
	}
	if detailEnvelope.Data.Runtime != strategyRuntimeDSLPlan {
		t.Fatalf("unexpected strategy runtime: %+v", detailEnvelope.Data)
	}
	if detailEnvelope.Data.SourceFormat != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("unexpected strategy source format: %+v", detailEnvelope.Data)
	}
	if detailEnvelope.Data.VisualModel == nil || len(detailEnvelope.Data.VisualModel.Nodes) != 1 {
		t.Fatalf("unexpected visual model: %+v", detailEnvelope.Data.VisualModel)
	}
	if detailEnvelope.Data.DerivedWarmupBars != 120 {
		t.Fatalf("default derivedWarmupBars = %d, want 120", detailEnvelope.Data.DerivedWarmupBars)
	}
	if detailEnvelope.Data.DerivedWarmupInterval != "1m" {
		t.Fatalf("default derivedWarmupInterval = %q, want 1m", detailEnvelope.Data.DerivedWarmupInterval)
	}

	previewResp, err := http.Get(srv.URL + "/api/v1/strategy-definitions/dsl-mean-revert?interval=5m")
	if err != nil {
		t.Fatalf("GET strategy definition detail preview: %v", err)
	}
	defer previewResp.Body.Close()
	if previewResp.StatusCode != http.StatusOK {
		t.Fatalf("GET strategy definition detail preview status = %d", previewResp.StatusCode)
	}
	var previewEnvelope struct {
		OK   bool                       `json:"ok"`
		Data strategyDefinitionResponse `json:"data"`
	}
	if err := json.NewDecoder(previewResp.Body).Decode(&previewEnvelope); err != nil {
		t.Fatalf("decode strategy definition detail preview: %v", err)
	}
	if previewEnvelope.Data.DerivedWarmupBars != 24 {
		t.Fatalf("preview derivedWarmupBars = %d, want 24", previewEnvelope.Data.DerivedWarmupBars)
	}
	if previewEnvelope.Data.DerivedWarmupInterval != "5m" {
		t.Fatalf("preview derivedWarmupInterval = %q, want 5m", previewEnvelope.Data.DerivedWarmupInterval)
	}

	payload["description"] = "updated dsl strategy"
	updateBody, _ := json.Marshal(payload)
	request, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/strategy-definitions/dsl-mean-revert", bytes.NewReader(updateBody))
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
	if updateEnvelope.Data.Description != "updated dsl strategy" {
		t.Fatalf("unexpected updated definition: %+v", updateEnvelope.Data)
	}
}

func TestInstantiateDSLStrategyDefinitionBuildsCompiledPlan(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-breakout",
		Name:         "DSL Breakout",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Symbol:       "00700",
		Interval:     "1m",
		Script:       "strategy DSL Breakout\non kline_close:\n  let fast = ma(EMA, 5, day)\n  if cross_over(fast, fast):\n    buy cash_percent 50\n  else:\n    protect auto trailing_stop 2 day 4% window session",
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	srv := httptest.NewServer(server)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/strategy-definitions/dsl-breakout/instantiate", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST instantiate DSL strategy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST instantiate DSL strategy status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope struct {
		OK   bool             `json:"ok"`
		Data strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode DSL instantiate response: %v", err)
	}
	if envelope.Data.PluginID != IDDSLPlanPlugin() {
		t.Fatalf("unexpected DSL plugin id: %+v", envelope.Data)
	}
	if envelope.Data.Runtime != strategyRuntimeDSLPlan {
		t.Fatalf("unexpected DSL runtime field: %+v", envelope.Data)
	}
	if envelope.Data.SourceFormat != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("unexpected DSL source format field: %+v", envelope.Data)
	}
	if !envelope.Data.Startable {
		t.Fatalf("expected DSL compiled instance to be startable: %+v", envelope.Data)
	}
	if got := envelope.Data.Params["runtime"]; got != strategyRuntimeDSLPlan {
		t.Fatalf("unexpected DSL runtime params: %+v", envelope.Data.Params)
	}
	if got := envelope.Data.Params["sourceFormat"]; got != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("unexpected DSL source format params: %+v", envelope.Data.Params)
	}
	compiledRequirements, ok := envelope.Data.Params["compiledRequirements"].(map[string]any)
	if !ok {
		t.Fatalf("compiledRequirements type = %T", envelope.Data.Params["compiledRequirements"])
	}
	if compiledRequirements["requiresAvailableCash"] != true {
		t.Fatalf("expected compiled requirements to request available cash, got %+v", compiledRequirements)
	}
	indicators, ok := compiledRequirements["indicators"].([]any)
	if !ok || len(indicators) != 2 {
		t.Fatalf("unexpected compiled indicators: %+v", compiledRequirements["indicators"])
	}

	instanceID := envelope.Data.ID
	assertTransition := func(action string, expectedStatus string) {
		transitionResp, transitionErr := http.Post(srv.URL+"/api/v1/strategies/"+instanceID+"/"+action, "application/json", bytes.NewReader([]byte(`{}`)))
		if transitionErr != nil {
			t.Fatalf("POST DSL %s: %v", action, transitionErr)
		}
		defer transitionResp.Body.Close()
		if transitionResp.StatusCode != http.StatusOK {
			t.Fatalf("POST DSL %s status = %d, want %d", action, transitionResp.StatusCode, http.StatusOK)
		}
		var transitionEnvelope struct {
			OK   bool             `json:"ok"`
			Data strategyListItem `json:"data"`
		}
		if err := json.NewDecoder(transitionResp.Body).Decode(&transitionEnvelope); err != nil {
			t.Fatalf("decode DSL %s response: %v", action, err)
		}
		if transitionEnvelope.Data.Status != expectedStatus {
			t.Fatalf("DSL %s status = %s, want %s", action, transitionEnvelope.Data.Status, expectedStatus)
		}
		if !transitionEnvelope.Data.Startable {
			t.Fatalf("expected transitioned DSL instance to remain startable: %+v", transitionEnvelope.Data)
		}
	}

	assertTransition("start", strategyStatusRunning)
	assertTransition("pause", strategyStatusPaused)
	assertTransition("stop", strategyStatusStopped)
}

func TestInstantiateStoredDefinitionNormalizesLegacySourceFormatToDSL(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "legacy-breakout",
		Name:         "Legacy Breakout",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: "legacy-v0",
		Symbol:       "00700",
		Interval:     "1m",
		Script:       "strategy Legacy Breakout\non kline_close:\n  log \"close\"",
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	srv := httptest.NewServer(server)
	defer srv.Close()

	createResp, err := http.Post(srv.URL+"/api/v1/strategy-definitions/legacy-breakout/instantiate", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST instantiate: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST normalized legacy source format instantiate status = %d, want %d", createResp.StatusCode, http.StatusOK)
	}
	var createEnvelope struct {
		OK   bool             `json:"ok"`
		Data strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode normalized instantiate: %v", err)
	}
	if createEnvelope.Data.SourceFormat != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("expected normalized DSL source format, got %+v", createEnvelope.Data)
	}
	if createEnvelope.Data.Runtime != strategyRuntimeDSLPlan {
		t.Fatalf("expected normalized DSL runtime, got %+v", createEnvelope.Data)
	}
}

func TestBacktestRouteUsesDerivedStrategyWarmup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-route-auto-warmup.db")
	t.Setenv("JFTRADE_BACKTEST_DB", dbPath)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-auto-warmup-route",
		Name:         "DSL Auto Warmup Route",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Symbol:       "US.AAPL",
		Interval:     "1m",
		Script: `strategy DSL Auto Warmup Route
version 1
symbol US.AAPL
interval 1m

on kline_close:
  let fast = ma(MA, 1)
  let slow = ma(MA, 20)
  if cross_over(fast, slow):
    buy shares 1`,
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}

	klineStore, err := backtest.NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := make([]bbgotypes.KLine, 0, 23)
	for index := range 23 {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		openPrice := 100.0
		closePrice := 100.0
		switch {
		case index == 20:
			closePrice = 120.0
		case index > 20:
			openPrice = 120.0
			closePrice = 121.0
		}
		klines = append(klines, bbgotypes.KLine{
			StartTime: bbgotypes.Time(startAt),
			EndTime:   bbgotypes.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  bbgotypes.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(openPrice),
			High:      fixedpoint.NewFromFloat(closePrice + 1),
			Low:       fixedpoint.NewFromFloat(openPrice - 1),
			Close:     fixedpoint.NewFromFloat(closePrice),
			Volume:    fixedpoint.NewFromFloat(1000),
		})
	}
	if err := klineStore.InsertKLines(klines, "forward"); err != nil {
		_ = klineStore.Close()
		t.Fatalf("InsertKLines: %v", err)
	}
	if err := klineStore.Close(); err != nil {
		t.Fatalf("klineStore.Close: %v", err)
	}

	srv := httptest.NewServer(server)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"definitionId":   "dsl-auto-warmup-route",
		"symbol":         "US.AAPL",
		"interval":       "1m",
		"startTime":      klines[20].StartTime.Time().Format(time.RFC3339),
		"endTime":        klines[22].EndTime.Time().Format(time.RFC3339),
		"initialBalance": 10000,
		"rehabType":      "forward",
	})
	createResp, err := http.Post(srv.URL+"/api/v1/backtests", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST backtest: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST backtest status = %d", createResp.StatusCode)
	}
	var createEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode backtest create response: %v", err)
	}

	var runEnvelope struct {
		OK   bool             `json:"ok"`
		Data backtestRunState `json:"data"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resultResp, err := http.Get(srv.URL + "/api/v1/backtests/" + createEnvelope.Data.ID)
		if err != nil {
			t.Fatalf("GET backtest result: %v", err)
		}
		if resultResp.StatusCode != http.StatusOK {
			resultResp.Body.Close()
			t.Fatalf("GET backtest result status = %d", resultResp.StatusCode)
		}
		if err := json.NewDecoder(resultResp.Body).Decode(&runEnvelope); err != nil {
			resultResp.Body.Close()
			t.Fatalf("decode backtest result: %v", err)
		}
		resultResp.Body.Close()
		if runEnvelope.Data.Status == "completed" || runEnvelope.Data.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if runEnvelope.Data.Status != "completed" {
		if runEnvelope.Data.Result != nil {
			t.Fatalf("backtest status = %s, error = %q", runEnvelope.Data.Status, runEnvelope.Data.Result.Error)
		}
		t.Fatalf("backtest status = %s, expected completed", runEnvelope.Data.Status)
	}
	if runEnvelope.Data.Result == nil {
		t.Fatal("expected backtest result payload")
	}
	if runEnvelope.Data.Result.Error != "" {
		t.Fatalf("backtest result error = %q", runEnvelope.Data.Result.Error)
	}
	if runEnvelope.Data.Result.TotalTrades == 0 {
		t.Fatalf("TotalTrades = %d, want > 0", runEnvelope.Data.Result.TotalTrades)
	}
	if len(runEnvelope.Data.Result.DrawdownCurve) != len(runEnvelope.Data.Result.PnLCurve) {
		t.Fatalf("DrawdownCurve len = %d, want %d", len(runEnvelope.Data.Result.DrawdownCurve), len(runEnvelope.Data.Result.PnLCurve))
	}
	if runEnvelope.Data.Result.MaxDrawdown < 0 {
		t.Fatalf("MaxDrawdown = %f, want >= 0", runEnvelope.Data.Result.MaxDrawdown)
	}
	if runEnvelope.Data.Result.CurrentDrawdown < 0 {
		t.Fatalf("CurrentDrawdown = %f, want >= 0", runEnvelope.Data.Result.CurrentDrawdown)
	}
	if len(runEnvelope.Data.Result.OrderBook) == 0 {
		t.Fatal("expected order book entries from auto warmup backtest")
	}
}
