package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestStrategyDefinitionEndpoints(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	payload := map[string]any{
		"id":           "pine-mean-revert",
		"name":         "Pine Mean Revert",
		"version":      "0.1.0",
		"description":  "pine strategy",
		"runtime":      strategyRuntimePinePlan,
		"sourceFormat": strategydefinition.SourceFormatPineV6,
		"symbol":       "00700",
		"interval":     "1m",
		"script":       "//@version=6\nstrategy(\"Pine Mean Revert\", overlay=true)\nslow = ta.ema(close, 24)\nlog.info(\"close\")",
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
	body, jftradeErr1 := json.Marshal(payload)
	jftradeCheckTestError(t, jftradeErr1)
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/strategy-definitions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST strategy definition: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST strategy definition status = %d", createResp.StatusCode)
	}
	var createEnvelope struct {
		OK   bool                     `json:"ok"`
		Data strategyDesignDefinition `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode created strategy definition: %v", err)
	}
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(createEnvelope.Data.ID) {
		t.Fatalf("created id = %q, want uuid", createEnvelope.Data.ID)
	}
	if createEnvelope.Data.ID == payload["id"] {
		t.Fatalf("expected create endpoint to ignore client id, got %q", createEnvelope.Data.ID)
	}
	if createEnvelope.Data.Runtime != strategyRuntimePinePlan {
		t.Fatalf("created definition runtime = %q, want %q", createEnvelope.Data.Runtime, strategyRuntimePinePlan)
	}
	if createEnvelope.Data.SourceFormat != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("created definition sourceFormat = %q, want %q", createEnvelope.Data.SourceFormat, strategydefinition.SourceFormatPineV6)
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/strategy-definitions")
	if err != nil {
		t.Fatalf("GET strategy definitions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
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
	if len(listEnvelope.Data) != 1 || listEnvelope.Data[0].ID != createEnvelope.Data.ID {
		t.Fatalf("unexpected definitions response: %+v", listEnvelope.Data)
	}

	detailResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/strategy-definitions/"+createEnvelope.Data.ID)
	if err != nil {
		t.Fatalf("GET strategy definition detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, detailResp.Body.Close()) }()
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
	if detailEnvelope.Data.Runtime != strategyRuntimePinePlan {
		t.Fatalf("unexpected strategy runtime: %+v", detailEnvelope.Data)
	}
	if detailEnvelope.Data.SourceFormat != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("unexpected strategy source format: %+v", detailEnvelope.Data)
	}
	if detailEnvelope.Data.VisualModel == nil || len(detailEnvelope.Data.VisualModel.Nodes) != 1 {
		t.Fatalf("unexpected visual model: %+v", detailEnvelope.Data.VisualModel)
	}
	if detailEnvelope.Data.DerivedWarmupBars != 24 {
		t.Fatalf("default derivedWarmupBars = %d, want 24", detailEnvelope.Data.DerivedWarmupBars)
	}
	if detailEnvelope.Data.DerivedWarmupInterval != "5m" {
		t.Fatalf("default derivedWarmupInterval = %q, want 5m", detailEnvelope.Data.DerivedWarmupInterval)
	}

	previewResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/strategy-definitions/"+createEnvelope.Data.ID+"?interval=5m")
	if err != nil {
		t.Fatalf("GET strategy definition detail preview: %v", err)
	}
	defer func() { jftradeCheckTestError(t, previewResp.Body.Close()) }()
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
	if previewEnvelope.Data.Runtime != strategyRuntimePinePlan {
		t.Fatalf("preview runtime = %q, want %q", previewEnvelope.Data.Runtime, strategyRuntimePinePlan)
	}
	if previewEnvelope.Data.SourceFormat != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("preview sourceFormat = %q, want %q", previewEnvelope.Data.SourceFormat, strategydefinition.SourceFormatPineV6)
	}
	if previewEnvelope.Data.DerivedWarmupInterval != "5m" {
		t.Fatalf("preview derivedWarmupInterval = %q, want 5m", previewEnvelope.Data.DerivedWarmupInterval)
	}

	payload["description"] = "updated pine strategy"
	updateBody, jftradeErr2 := json.Marshal(payload)
	jftradeCheckTestError(t, jftradeErr2)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/strategy-definitions/"+createEnvelope.Data.ID, bytes.NewReader(updateBody))
	if err != nil {
		t.Fatalf("build PUT request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT strategy definition: %v", err)
	}
	defer func() { jftradeCheckTestError(t, updateResp.Body.Close()) }()
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
	if updateEnvelope.Data.Description != "updated pine strategy" {
		t.Fatalf("unexpected updated definition: %+v", updateEnvelope.Data)
	}
	if updateEnvelope.Data.ID != createEnvelope.Data.ID {
		t.Fatalf("updated definition id = %q, want %q", updateEnvelope.Data.ID, createEnvelope.Data.ID)
	}
	if updateEnvelope.Data.Runtime != strategyRuntimePinePlan {
		t.Fatalf("updated definition runtime = %q, want %q", updateEnvelope.Data.Runtime, strategyRuntimePinePlan)
	}
	if updateEnvelope.Data.SourceFormat != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("updated definition sourceFormat = %q, want %q", updateEnvelope.Data.SourceFormat, strategydefinition.SourceFormatPineV6)
	}
}

func TestStrategyDefinitionCreateGeneratesUUIDWhenIDMissing(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	payload := map[string]any{
		"name":         "UUID Strategy",
		"description":  "strategy without explicit id",
		"runtime":      strategyRuntimePinePlan,
		"sourceFormat": strategydefinition.SourceFormatPineV6,
		"script":       "//@version=6\nstrategy(\"UUID Strategy\", overlay=true)\nlog.info(\"close\")",
	}
	body, jftradeErr3 := json.Marshal(payload)
	jftradeCheckTestError(t, jftradeErr3)
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/strategy-definitions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST strategy definition without id: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST strategy definition without id status = %d", createResp.StatusCode)
	}

	var createEnvelope struct {
		OK   bool                     `json:"ok"`
		Data strategyDesignDefinition `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode created strategy definition without id: %v", err)
	}
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(createEnvelope.Data.ID) {
		t.Fatalf("created id = %q, want uuid", createEnvelope.Data.ID)
	}
}

func TestStrategyDefinitionRejectsInvalidScriptPayloads(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	payload := map[string]any{
		"name":         "Broken Pine",
		"runtime":      strategyRuntimePinePlan,
		"sourceFormat": strategydefinition.SourceFormatPineV6,
		"script":       "//@version=6\nstrategy(\"Broken Pine\", overlay=true)\nx = request.security(\"NASDAQ:AAPL\", \"D\", close)",
	}
	body, jftradeErr4 := json.Marshal(payload)
	jftradeCheckTestError(t, jftradeErr4)
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/strategy-definitions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST invalid strategy definition: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST invalid strategy definition status = %d, want %d", createResp.StatusCode, http.StatusBadRequest)
	}
}

func TestDeleteMissingStrategyDefinitionReturnsNotFound(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/strategy-definitions/missing-definition", nil)
	if err != nil {
		t.Fatalf("build DELETE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("DELETE missing definition: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE missing definition status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestDeleteStrategyDefinitionRequiresDeletingLinkedInstancesFirst(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	definition, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "pine-delete-guard",
		Name:         "Delete Guard",
		Description:  "delete guard",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Delete Guard\", overlay=true)\nlog.info(\"close\")",
	})
	if err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "5m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/strategy-definitions/"+definition.ID, nil)
	if err != nil {
		t.Fatalf("build delete definition request: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete definition with linked instance: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete definition with linked instance status = %d, want %d", deleteResp.StatusCode, http.StatusBadRequest)
	}
	var blockedEnvelope envelope
	if err := json.NewDecoder(deleteResp.Body).Decode(&blockedEnvelope); err != nil {
		t.Fatalf("decode blocked delete response: %v", err)
	}
	if blockedEnvelope.Error == nil || !strings.Contains(blockedEnvelope.Error.Message, "请先删除对应实例再删除") {
		t.Fatalf("unexpected blocked delete response: %+v", blockedEnvelope)
	}
	if _, ok, err := server.designStore.definition(definition.ID); err != nil || !ok {
		t.Fatal("definition should still exist after blocked delete")
	}

	instanceDeleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/strategies/"+instance.ID, nil)
	if err != nil {
		t.Fatalf("build delete instance request: %v", err)
	}
	instanceDeleteResp, err := http.DefaultClient.Do(instanceDeleteReq)
	if err != nil {
		t.Fatalf("delete linked instance: %v", err)
	}
	defer func() { jftradeCheckTestError(t, instanceDeleteResp.Body.Close()) }()
	if instanceDeleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete linked instance status = %d, want %d", instanceDeleteResp.StatusCode, http.StatusOK)
	}

	deleteReq, err = http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/strategy-definitions/"+definition.ID, nil)
	if err != nil {
		t.Fatalf("build second delete definition request: %v", err)
	}
	deleteResp, err = http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete definition after removing instances: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete definition after removing instances status = %d, want %d", deleteResp.StatusCode, http.StatusOK)
	}
	if _, ok, err := server.designStore.definition(definition.ID); err != nil || ok {
		t.Fatal("definition should be hidden after soft delete")
	}
	definitions := server.designStore.listDefinitions()
	if len(definitions) != 0 {
		t.Fatalf("expected no active definitions after delete, got %+v", definitions)
	}
}
