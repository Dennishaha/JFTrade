package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestInstantiatePineStrategyDefinitionBuildsCompiledPlan(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return newStrategyRuntimeStubExchange() }
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "pine-breakout",
		Name:         "Pine Breakout",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Pine Breakout\", overlay=true)\nfast = ta.ema(close, 5)\nslow = ta.ema(close, 21)\nif ta.crossover(fast, slow)\n    strategy.entry(\"Long\", strategy.long, qty=(strategy.equity * 50 / 100) / close)\nelse\n    alert(\"no signal\")",
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/v1/strategy-definitions/pine-breakout/instantiate", "application/json", bytes.NewReader([]byte(`{"instruments":[{"market":"US","code":"AAPL"},{"market":"HK","code":"00700"}],"interval":"15m","executionMode":"notify_only","brokerAccount":{"brokerId":"futu","accountId":"123456","tradingEnvironment":"simulate","market":"us"}}`)))
	if err != nil {
		t.Fatalf("POST instantiate Pine strategy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST instantiate Pine strategy status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope struct {
		OK   bool             `json:"ok"`
		Data strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode Pine instantiate response: %v", err)
	}
	if envelope.Data.PluginID != IDPinePlanPlugin() {
		t.Fatalf("unexpected Pine plugin id: %+v", envelope.Data)
	}
	if envelope.Data.Runtime != strategyRuntimePinePlan {
		t.Fatalf("unexpected Pine runtime field: %+v", envelope.Data)
	}
	if envelope.Data.SourceFormat != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("unexpected Pine source format field: %+v", envelope.Data)
	}
	if !envelope.Data.Startable {
		t.Fatalf("expected Pine compiled instance to be startable: %+v", envelope.Data)
	}
	if len(envelope.Data.Binding.Symbols) != 2 || envelope.Data.Binding.Symbols[0] != "US.AAPL" || envelope.Data.Binding.Symbols[1] != "HK.00700" {
		t.Fatalf("unexpected binding symbols: %+v", envelope.Data.Binding)
	}
	if len(envelope.Data.Binding.Instruments) != 2 || envelope.Data.Binding.Instruments[0].Market != "US" || envelope.Data.Binding.Instruments[0].Code != "AAPL" || envelope.Data.Binding.Instruments[1].Market != "HK" || envelope.Data.Binding.Instruments[1].Code != "00700" {
		t.Fatalf("unexpected binding instruments: %+v", envelope.Data.Binding)
	}
	if envelope.Data.Binding.Interval != "15m" {
		t.Fatalf("unexpected binding interval: %+v", envelope.Data.Binding)
	}
	if envelope.Data.Binding.ExecutionMode != strategyExecutionModeNotifyOnly {
		t.Fatalf("unexpected binding execution mode: %+v", envelope.Data.Binding)
	}
	if envelope.Data.Binding.BrokerAccount == nil || envelope.Data.Binding.BrokerAccount.BrokerID != "futu" || envelope.Data.Binding.BrokerAccount.AccountID != "123456" || envelope.Data.Binding.BrokerAccount.TradingEnvironment != "SIMULATE" || envelope.Data.Binding.BrokerAccount.Market != "US" {
		t.Fatalf("unexpected binding broker account: %+v", envelope.Data.Binding)
	}
	if got := envelope.Data.Params["runtime"]; got != strategyRuntimePinePlan {
		t.Fatalf("unexpected Pine runtime params: %+v", envelope.Data.Params)
	}
	if got := envelope.Data.Params["sourceFormat"]; got != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("unexpected Pine source format params: %+v", envelope.Data.Params)
	}
	if got := envelope.Data.Params["interval"]; got != "15m" {
		t.Fatalf("unexpected Pine binding interval params: %+v", envelope.Data.Params)
	}
	if got := envelope.Data.Params["executionMode"]; got != strategyExecutionModeNotifyOnly {
		t.Fatalf("unexpected Pine execution mode params: %+v", envelope.Data.Params)
	}
	if got, ok := envelope.Data.Params["brokerAccount"].(map[string]any); !ok || got["brokerId"] != "futu" {
		t.Fatalf("unexpected Pine broker account params: %+v", envelope.Data.Params)
	}
	if got, ok := envelope.Data.Params["instruments"].([]any); !ok || len(got) != 2 {
		t.Fatalf("unexpected Pine binding instruments params: %+v", envelope.Data.Params)
	}
	compiledRequirements, ok := envelope.Data.Params["compiledRequirements"].(map[string]any)
	if !ok {
		t.Fatalf("compiledRequirements type = %T", envelope.Data.Params["compiledRequirements"])
	}
	if compiledRequirements["requiresTotalAccountValue"] != true {
		t.Fatalf("expected compiled requirements to request total account value, got %+v", compiledRequirements)
	}
	indicators, ok := compiledRequirements["indicators"].([]any)
	if !ok || len(indicators) != 2 {
		t.Fatalf("unexpected compiled indicators: %+v", compiledRequirements["indicators"])
	}

	instanceID := envelope.Data.ID
	updateRequest, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/strategies/"+instanceID, bytes.NewReader([]byte(`{"instruments":[{"market":"US","code":"MSFT"}],"interval":"30m","executionMode":"live"}`)))
	if err != nil {
		t.Fatalf("build PUT strategy request: %v", err)
	}
	updateRequest.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(updateRequest)
	if err != nil {
		t.Fatalf("PUT strategy binding: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT strategy binding status = %d, want %d", updateResp.StatusCode, http.StatusOK)
	}
	var updateEnvelope struct {
		OK   bool             `json:"ok"`
		Data strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(updateResp.Body).Decode(&updateEnvelope); err != nil {
		t.Fatalf("decode updated strategy binding: %v", err)
	}
	if len(updateEnvelope.Data.Binding.Symbols) != 1 || updateEnvelope.Data.Binding.Symbols[0] != "US.MSFT" {
		t.Fatalf("unexpected updated binding symbols: %+v", updateEnvelope.Data.Binding)
	}
	if len(updateEnvelope.Data.Binding.Instruments) != 1 || updateEnvelope.Data.Binding.Instruments[0].Market != "US" || updateEnvelope.Data.Binding.Instruments[0].Code != "MSFT" {
		t.Fatalf("unexpected updated binding instruments: %+v", updateEnvelope.Data.Binding)
	}
	if updateEnvelope.Data.Binding.Interval != "30m" || updateEnvelope.Data.Binding.ExecutionMode != strategyExecutionModeLive {
		t.Fatalf("unexpected updated binding fields: %+v", updateEnvelope.Data.Binding)
	}
	if updateEnvelope.Data.Binding.BrokerAccount == nil || updateEnvelope.Data.Binding.BrokerAccount.BrokerID != "futu" {
		t.Fatalf("expected update to preserve broker account binding: %+v", updateEnvelope.Data.Binding)
	}

	assertTransition := func(action string, expectedStatus string) {
		transitionResp, transitionErr := http.Post(srv.URL+"/api/v1/strategies/"+instanceID+"/"+action, "application/json", bytes.NewReader([]byte(`{}`)))
		if transitionErr != nil {
			t.Fatalf("POST Pine %s: %v", action, transitionErr)
		}
		defer transitionResp.Body.Close()
		if transitionResp.StatusCode != http.StatusOK {
			t.Fatalf("POST Pine %s status = %d, want %d", action, transitionResp.StatusCode, http.StatusOK)
		}
		var transitionEnvelope struct {
			OK   bool             `json:"ok"`
			Data strategyListItem `json:"data"`
		}
		if err := json.NewDecoder(transitionResp.Body).Decode(&transitionEnvelope); err != nil {
			t.Fatalf("decode Pine %s response: %v", action, err)
		}
		if transitionEnvelope.Data.Status != expectedStatus {
			t.Fatalf("Pine %s status = %s, want %s", action, transitionEnvelope.Data.Status, expectedStatus)
		}
		if transitionEnvelope.Data.Runtime != strategyRuntimePinePlan {
			t.Fatalf("Pine %s runtime = %q, want %q", action, transitionEnvelope.Data.Runtime, strategyRuntimePinePlan)
		}
		if transitionEnvelope.Data.SourceFormat != strategydefinition.SourceFormatPineV6 {
			t.Fatalf("Pine %s sourceFormat = %q, want %q", action, transitionEnvelope.Data.SourceFormat, strategydefinition.SourceFormatPineV6)
		}
		if !transitionEnvelope.Data.Startable {
			t.Fatalf("expected transitioned Pine instance to remain startable: %+v", transitionEnvelope.Data)
		}
	}

	assertTransition("start", strategyStatusRunning)
	if _, ok := server.strategyRuntimeManager.runtimeObservation(instanceID); !ok {
		t.Fatalf("expected runtime observation after start")
	}
	assertTransition("pause", strategyStatusPaused)
	if _, ok := server.strategyRuntimeManager.runtimeObservation(instanceID); ok {
		t.Fatalf("expected pause to stop active runtime for %s", instanceID)
	}
	assertTransition("stop", strategyStatusStopped)

	deleteRequest, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/strategies/"+instanceID, nil)
	if err != nil {
		t.Fatalf("build DELETE strategy request: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		t.Fatalf("DELETE strategy: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE strategy status = %d, want %d", deleteResp.StatusCode, http.StatusOK)
	}
	var deleteEnvelope struct {
		OK   bool             `json:"ok"`
		Data strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(deleteResp.Body).Decode(&deleteEnvelope); err != nil {
		t.Fatalf("decode deleted strategy response: %v", err)
	}
	if deleteEnvelope.Data.ID != instanceID {
		t.Fatalf("unexpected deleted strategy response: %+v", deleteEnvelope.Data)
	}
	listResp, err := http.Get(srv.URL + "/api/v1/strategies")
	if err != nil {
		t.Fatalf("GET strategies after delete: %v", err)
	}
	defer listResp.Body.Close()
	var listEnvelope struct {
		OK   bool               `json:"ok"`
		Data []strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode strategies after delete: %v", err)
	}
	if len(listEnvelope.Data) != 0 {
		t.Fatalf("expected no strategies after delete, got %+v", listEnvelope.Data)
	}
}

func TestInstantiateStrategyDefinitionRejectsMalformedJSON(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "pine-malformed-binding",
		Name:         "Malformed Binding",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Malformed Binding\", overlay=true)\nlog.info(\"close\")",
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/v1/strategy-definitions/pine-malformed-binding/instantiate", "application/json", bytes.NewReader([]byte(`{"interval":`)))
	if err != nil {
		t.Fatalf("POST instantiate malformed JSON: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST malformed instantiate status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
