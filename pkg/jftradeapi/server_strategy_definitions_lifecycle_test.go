package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestInstantiateDSLStrategyDefinitionBuildsCompiledPlan(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.strategyRuntimeManager.exchangeProvider = func() strategyRuntimeExchange { return newStrategyRuntimeStubExchange() }
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-breakout",
		Name:         "DSL Breakout",
		Version:      "0.1.0",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		Script:       "strategy DSL Breakout\non kline_close:\n  let fast = ma(EMA, 5, day)\n  if cross_over(fast, fast):\n    buy cash_percent 50\n  else:\n    protect auto trailing_stop 2 day 4% window session",
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/v1/strategy-definitions/dsl-breakout/instantiate", "application/json", bytes.NewReader([]byte(`{"instruments":[{"market":"US","code":"AAPL"},{"market":"HK","code":"00700"}],"interval":"15m","executionMode":"notify_only","brokerAccount":{"brokerId":"futu","accountId":"123456","tradingEnvironment":"simulate","market":"us"}}`)))
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
	if got := envelope.Data.Params["runtime"]; got != strategyRuntimeDSLPlan {
		t.Fatalf("unexpected DSL runtime params: %+v", envelope.Data.Params)
	}
	if got := envelope.Data.Params["sourceFormat"]; got != strategydefinition.SourceFormatDSLV1 {
		t.Fatalf("unexpected DSL source format params: %+v", envelope.Data.Params)
	}
	if got := envelope.Data.Params["interval"]; got != "15m" {
		t.Fatalf("unexpected DSL binding interval params: %+v", envelope.Data.Params)
	}
	if got := envelope.Data.Params["executionMode"]; got != strategyExecutionModeNotifyOnly {
		t.Fatalf("unexpected DSL execution mode params: %+v", envelope.Data.Params)
	}
	if got, ok := envelope.Data.Params["brokerAccount"].(map[string]any); !ok || got["brokerId"] != "futu" {
		t.Fatalf("unexpected DSL broker account params: %+v", envelope.Data.Params)
	}
	if got, ok := envelope.Data.Params["instruments"].([]any); !ok || len(got) != 2 {
		t.Fatalf("unexpected DSL binding instruments params: %+v", envelope.Data.Params)
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
