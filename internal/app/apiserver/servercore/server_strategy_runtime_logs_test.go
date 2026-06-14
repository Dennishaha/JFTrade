package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrategiesEndpointReturnsList(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
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
	}); err != nil {
		t.Fatalf("saveStrategy: %v", err)
	}
	if err := server.strategyStore.appendStrategyRuntimeEvent("instance-1", "started", "started", "mean-revert"); err != nil {
		t.Fatalf("appendStrategyRuntimeEvent: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

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
	if len(logsEnvelope.Data.Logs) != 1 || !strings.Contains(logsEnvelope.Data.Logs[0], "started") {
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

func TestStrategiesEndpointIncludesPersistedRuntimeLogTail(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if err := server.strategyStore.saveStrategy(managedStrategyInstance{
		ID: "instance-tail",
		Definition: strategyDefinitionSummary{
			StrategyID: "mean-revert",
			Name:       "Mean Revert",
			Version:    "1.0.0",
		},
		Params:    map[string]any{"window": 20},
		Status:    strategyStatusStopped,
		CreatedAt: "2026-05-22T00:00:00Z",
	}); err != nil {
		t.Fatalf("saveStrategy: %v", err)
	}
	if err := server.strategyStore.appendStrategyRuntimeEvent("instance-tail", "runtime error US.AAPL: boom", "runtime_error", "boom"); err != nil {
		t.Fatalf("appendStrategyRuntimeEvent: %v", err)
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/strategies")
	if err != nil {
		t.Fatalf("GET strategies: %v", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		OK   bool               `json:"ok"`
		Data []strategyListItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode strategies: %v", err)
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("expected 1 strategy, got %+v", envelope.Data)
	}
	if len(envelope.Data[0].Logs) == 0 || !strings.Contains(envelope.Data[0].Logs[0], "runtime error US.AAPL: boom") {
		t.Fatalf("expected persisted runtime log tail in strategy list item, got %+v", envelope.Data[0].Logs)
	}
}

func TestStrategyLogsAndAuditEndpointsSupportPaginationAndFilters(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if err := server.strategyStore.saveStrategy(managedStrategyInstance{
		ID: "instance-logs",
		Definition: strategyDefinitionSummary{
			StrategyID: "mean-revert",
			Name:       "Mean Revert",
			Version:    "1.0.0",
		},
		Params:    map[string]any{"window": 20},
		Status:    strategyStatusStopped,
		CreatedAt: "2026-05-22T00:00:00Z",
	}); err != nil {
		t.Fatalf("saveStrategy: %v", err)
	}
	for _, event := range []struct {
		message string
		kind    string
		detail  string
	}{
		{message: "notify-only signal US.AAPL BUY 10", kind: "signal_notified", detail: "signal detail"},
		{message: "runtime error US.AAPL: boom", kind: "runtime_error", detail: "boom"},
		{message: "live order submitted US.AAPL BUY 10", kind: "order_submitted", detail: "internalOrderId=1"},
	} {
		if err := server.strategyStore.appendStrategyRuntimeEvent("instance-logs", event.message, event.kind, event.detail); err != nil {
			t.Fatalf("appendStrategyRuntimeEvent(%s): %v", event.kind, err)
		}
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	logsResp, err := http.Get(srv.URL + "/api/v1/strategies/instance-logs/logs?limit=1&offset=1")
	if err != nil {
		t.Fatalf("GET paged logs: %v", err)
	}
	defer logsResp.Body.Close()
	var logsEnvelope struct {
		OK   bool                 `json:"ok"`
		Data strategyLogsResponse `json:"data"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsEnvelope); err != nil {
		t.Fatalf("decode paged logs: %v", err)
	}
	if logsEnvelope.Data.Page.Total != 3 || logsEnvelope.Data.Page.Returned != 1 || !logsEnvelope.Data.Page.HasMore {
		t.Fatalf("unexpected logs page: %+v", logsEnvelope.Data.Page)
	}

	legacyQueryResp, err := http.Get(srv.URL + "/api/v1/strategies/instance-logs/logs?limit=bogus&fromTime=2026-05-22")
	if err != nil {
		t.Fatalf("GET logs with legacy query parsing: %v", err)
	}
	defer legacyQueryResp.Body.Close()
	if legacyQueryResp.StatusCode != http.StatusOK {
		t.Fatalf("GET logs with legacy query parsing status = %d", legacyQueryResp.StatusCode)
	}

	filteredLogsResp, err := http.Get(srv.URL + "/api/v1/strategies/instance-logs/logs?level=error")
	if err != nil {
		t.Fatalf("GET filtered logs: %v", err)
	}
	defer filteredLogsResp.Body.Close()
	if err := json.NewDecoder(filteredLogsResp.Body).Decode(&logsEnvelope); err != nil {
		t.Fatalf("decode filtered logs: %v", err)
	}
	if logsEnvelope.Data.Page.Total != 1 || len(logsEnvelope.Data.Logs) != 1 || !strings.Contains(logsEnvelope.Data.Logs[0], "runtime error") {
		t.Fatalf("unexpected filtered logs response: %+v", logsEnvelope.Data)
	}

	auditResp, err := http.Get(srv.URL + "/api/v1/strategies/instance-logs/audit?kind=runtime_error")
	if err != nil {
		t.Fatalf("GET filtered audit: %v", err)
	}
	defer auditResp.Body.Close()
	var auditEnvelope struct {
		OK   bool                  `json:"ok"`
		Data strategyAuditResponse `json:"data"`
	}
	if err := json.NewDecoder(auditResp.Body).Decode(&auditEnvelope); err != nil {
		t.Fatalf("decode filtered audit: %v", err)
	}
	if auditEnvelope.Data.Page.Total != 1 || len(auditEnvelope.Data.Entries) != 1 || auditEnvelope.Data.Entries[0].Kind != "runtime_error" {
		t.Fatalf("unexpected filtered audit response: %+v", auditEnvelope.Data)
	}
}
