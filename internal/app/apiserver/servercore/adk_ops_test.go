package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestADKMetricsExposeLifecycleAndApprovalLatency(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	if _, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID:          "provider-metrics",
		DisplayName: "Metrics Provider",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		APIKey:      "sk-test",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "metrics-agent",
		Name:           "Metrics Agent",
		ProviderID:     "provider-metrics",
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	now := time.Now().UTC()
	completedAt := now.Format(time.RFC3339Nano)
	run := jfadk.Run{
		ID:            "run-metrics",
		SessionID:     "session-1",
		AgentID:       agent.ID,
		Status:        jfadk.RunStatusFailed,
		Message:       "failed",
		FailureReason: "boom",
		ErrorCode:     "MODEL_CALL_FAILED",
		ResumeState:   "adk_confirmation_resolved",
		CreatedAt:     now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		StartedAt:     now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		UpdatedAt:     completedAt,
		CompletedAt:   &completedAt,
		ToolCalls: []jfadk.ToolCall{{
			ID:         "tool-1",
			RunID:      "run-metrics",
			ToolName:   "strategy.save_draft",
			Permission: "write_strategy",
			Status:     "FAILED",
			CreatedAt:  now.Add(-90 * time.Second).Format(time.RFC3339Nano),
			UpdatedAt:  completedAt,
			DurationMs: 1500,
		}},
		Usage: &jfadk.RunUsage{TokensIn: 120, TokensOut: 45},
	}
	if err := server.adkRuntime.Store().SaveRun(t.Context(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if err := server.adkRuntime.Store().DeleteAgent(t.Context(), agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	approval := jfadk.Approval{
		ID:                 "approval-metrics",
		RunID:              run.ID,
		AgentID:            agent.ID,
		ToolName:           "strategy.save_draft",
		Status:             jfadk.ApprovalStatusPending,
		Reason:             "needs approval",
		FunctionCallID:     "fn-1",
		ConfirmationCallID: "cf-1",
		CreatedAt:          now.Add(-30 * time.Second).Format(time.RFC3339Nano),
		UpdatedAt:          now.Add(-30 * time.Second).Format(time.RFC3339Nano),
	}
	if err := server.adkRuntime.Store().SaveApproval(t.Context(), approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/metrics")
	if err != nil {
		t.Fatalf("GET metrics: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Runs struct {
				Total      int            `json:"total"`
				ByAgent    map[string]int `json:"byAgent"`
				ByProvider map[string]int `json:"byProvider"`
				Lifecycle  struct {
					Failed  int `json:"failed"`
					Resumed int `json:"resumed"`
				} `json:"lifecycle"`
			} `json:"runs"`
			Tools struct {
				ByStatus map[string]int `json:"byStatus"`
			} `json:"tools"`
			Approvals struct {
				Pending            int `json:"pending"`
				RecoverablePending int `json:"recoverablePending"`
				PendingWaitMs      struct {
					Average int64 `json:"average"`
				} `json:"pendingWaitMs"`
			} `json:"approvals"`
			Usage struct {
				Samples       int  `json:"samples"`
				TokensInTotal *int `json:"tokensInTotal"`
			} `json:"usage"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("metrics envelope = %+v", envelope)
	}
	if envelope.Data.Runs.Total != 1 || envelope.Data.Runs.ByAgent[agent.ID] != 1 {
		t.Fatalf("run metrics = %+v", envelope.Data.Runs)
	}
	if envelope.Data.Runs.ByProvider["provider-metrics"] != 1 {
		t.Fatalf("provider metrics = %+v", envelope.Data.Runs.ByProvider)
	}
	if envelope.Data.Runs.Lifecycle.Failed != 1 || envelope.Data.Runs.Lifecycle.Resumed != 1 {
		t.Fatalf("lifecycle metrics = %+v", envelope.Data.Runs.Lifecycle)
	}
	if envelope.Data.Tools.ByStatus["FAILED"] != 1 {
		t.Fatalf("tool status metrics = %+v", envelope.Data.Tools.ByStatus)
	}
	if envelope.Data.Approvals.Pending != 1 || envelope.Data.Approvals.RecoverablePending != 1 || envelope.Data.Approvals.PendingWaitMs.Average <= 0 {
		t.Fatalf("approval metrics = %+v", envelope.Data.Approvals)
	}
	if envelope.Data.Usage.Samples != 1 || envelope.Data.Usage.TokensInTotal == nil || *envelope.Data.Usage.TokensInTotal != 120 {
		t.Fatalf("usage metrics = %+v", envelope.Data.Usage)
	}
}

func TestADKMetricsIgnoresUnexpectedQueryParams(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/metrics?limit=abc&offset=-1&status=FAILED")
	if err != nil {
		t.Fatalf("GET metrics with noise query params: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Runs struct {
				Total int `json:"total"`
			} `json:"runs"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode metrics envelope: %v", err)
	}
	if !envelope.OK || envelope.Data.Runs.Total != 0 {
		t.Fatalf("metrics envelope = %+v, want empty successful metrics response", envelope)
	}
}

func TestADKOptimizationTaskCanBeQueriedAndCancelled(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	task, err := server.adkRuntime.Store().SaveOptimizationTask(t.Context(), jfadk.OptimizationTask{
		ID: "opt-test", Status: "queued", Objective: "return",
		Runs: []jfadk.OptimizationRunRef{{DefinitionID: "definition-1", RunID: "missing-run"}},
	})
	if err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}
	getResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/optimization-tasks/"+task.ID)
	if err != nil {
		t.Fatalf("GET optimization task: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getResp.Body.Close()) }()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET optimization status = %d", getResp.StatusCode)
	}

	cancelResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/optimization-tasks/"+task.ID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST optimization cancel: %v", err)
	}
	defer func() { jftradeCheckTestError(t, cancelResp.Body.Close()) }()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("optimization cancel status = %d", cancelResp.StatusCode)
	}
	stored, ok, err := server.adkRuntime.Store().OptimizationTask(t.Context(), task.ID)
	if err != nil || !ok || stored.Status != "cancelled" {
		t.Fatalf("cancelled task = %+v ok=%v err=%v", stored, ok, err)
	}
}

func TestADKTaskAndMemoryWorkflowRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{ID: "workflow-agent", Name: "Workflow", ProviderID: testADKProviderID, Status: jfadk.AgentStatusEnabled})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/tasks", "application/json", strings.NewReader(`{"id":"task-route","title":"Route task","status":"TODO","agentId":"`+agent.ID+`","order":1,"agentRole":"探索 Agent","plannerStepId":"__planner_step_1","planSource":"planner","workflowMode":"sequential","objective":"完成路线"}`))
	if err != nil {
		t.Fatalf("POST task: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST task status = %d", createResp.StatusCode)
	}
	patchReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/adk/tasks/task-route", strings.NewReader(`{"status":"DONE","order":2,"plannerWarnings":["planner warning"]}`))
	if err != nil {
		t.Fatalf("NewRequest patch task: %v", err)
	}
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("PUT task: %v", err)
	}
	defer func() { jftradeCheckTestError(t, patchResp.Body.Close()) }()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT task status = %d", patchResp.StatusCode)
	}
	var patchEnvelope struct {
		OK   bool       `json:"ok"`
		Data jfadk.Task `json:"data"`
	}
	if err := json.NewDecoder(patchResp.Body).Decode(&patchEnvelope); err != nil {
		t.Fatalf("decode patch task: %v", err)
	}
	if !patchEnvelope.OK || patchEnvelope.Data.Title != "Route task" || patchEnvelope.Data.Status != "DONE" {
		t.Fatalf("patch envelope = %+v, want preserved title and DONE", patchEnvelope)
	}
	if patchEnvelope.Data.Order != 2 || patchEnvelope.Data.AgentRole != "探索 Agent" || patchEnvelope.Data.PlannerStepID != "__planner_step_1" || patchEnvelope.Data.PlanSource != "planner" || patchEnvelope.Data.WorkflowMode != "sequential" || patchEnvelope.Data.Objective != "完成路线" {
		t.Fatalf("patched task planner metadata = %+v, want preserved/updated metadata", patchEnvelope.Data)
	}
	if len(patchEnvelope.Data.PlannerWarnings) != 1 || patchEnvelope.Data.PlannerWarnings[0] != "planner warning" {
		t.Fatalf("patched task warnings = %+v, want planner warning", patchEnvelope.Data.PlannerWarnings)
	}
	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/tasks?status=DONE&agentId="+agent.ID)
	if err != nil {
		t.Fatalf("GET tasks: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET tasks status = %d", listResp.StatusCode)
	}
	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/tasks/task-route", nil)
	if err != nil {
		t.Fatalf("NewRequest delete task: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE task: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE task status = %d", deleteResp.StatusCode)
	}

	memoryResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/memory", "application/json", strings.NewReader(`{"scope":"agent","agentId":"`+agent.ID+`","key":"style","value":"risk first"}`))
	if err != nil {
		t.Fatalf("POST memory: %v", err)
	}
	defer func() { jftradeCheckTestError(t, memoryResp.Body.Close()) }()
	if memoryResp.StatusCode != http.StatusOK {
		t.Fatalf("POST memory status = %d", memoryResp.StatusCode)
	}
	var memoryEnvelope struct {
		OK   bool              `json:"ok"`
		Data jfadk.MemoryEntry `json:"data"`
	}
	if err := json.NewDecoder(memoryResp.Body).Decode(&memoryEnvelope); err != nil {
		t.Fatalf("decode memory: %v", err)
	}
	filterResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/memory?scope=agent&agentId="+agent.ID+"&key=style")
	if err != nil {
		t.Fatalf("GET memory: %v", err)
	}
	defer func() { jftradeCheckTestError(t, filterResp.Body.Close()) }()
	if filterResp.StatusCode != http.StatusOK {
		t.Fatalf("GET memory status = %d", filterResp.StatusCode)
	}
	deleteMemoryReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/memory/"+memoryEnvelope.Data.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest delete memory: %v", err)
	}
	deleteMemoryResp, err := http.DefaultClient.Do(deleteMemoryReq)
	if err != nil {
		t.Fatalf("DELETE memory: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteMemoryResp.Body.Close()) }()
	if deleteMemoryResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE memory status = %d", deleteMemoryResp.StatusCode)
	}
}

func TestADKOptimizationTaskNegativeRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	invalidGetReq := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/api/v1/adk/optimization-tasks/%zz"},
		Header: make(http.Header),
	}
	invalidGetRec := httptest.NewRecorder()
	server.ServeHTTP(invalidGetRec, invalidGetReq)
	status, code, message := decodeAPIErrorRecorder(t, invalidGetRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "taskId is invalid" {
		t.Fatalf("optimization invalid get = %d/%s/%q", status, code, message)
	}

	missingGetResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/optimization-tasks/missing-task")
	if err != nil {
		t.Fatalf("GET missing optimization task: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingGetResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, missingGetResp)
	if status != http.StatusNotFound || code != "NOT_FOUND" || message != "optimization task not found" {
		t.Fatalf("optimization missing get = %d/%s/%q", status, code, message)
	}

	invalidCancelReq := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Path: "/api/v1/adk/optimization-tasks/%zz/cancel"},
		Header: make(http.Header),
	}
	invalidCancelRec := httptest.NewRecorder()
	server.ServeHTTP(invalidCancelRec, invalidCancelReq)
	status, code, message = decodeAPIErrorRecorder(t, invalidCancelRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "taskId is invalid" {
		t.Fatalf("optimization invalid cancel = %d/%s/%q", status, code, message)
	}

	missingCancelResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/optimization-tasks/missing-task/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing optimization cancel: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingCancelResp.Body.Close()) }()
	status, code, message = decodeAPIErrorEnvelope(t, missingCancelResp)
	if status != http.StatusNotFound || code != "NOT_FOUND" || message != "optimization task not found" {
		t.Fatalf("optimization missing cancel = %d/%s/%q", status, code, message)
	}
}

func TestAssistantChatCompatibilityRouteIsGone(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/assistant/chat", "application/json", bytes.NewReader([]byte(`{"prompt":"hello"}`)))
	if err != nil {
		t.Fatalf("POST assistant chat: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("assistant chat status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestADKSnapshotAndToolsRoutesReturnCatalogData(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	provider, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID:               "provider-snapshot",
		DisplayName:      "Snapshot Provider",
		BaseURL:          "https://api.openai.com/v1",
		Model:            "gpt-4o-mini",
		RequestTimeoutMs: 240_000,
		APIKey:           "sk-test",
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := server.store.SaveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 660_000, StreamIdleTimeoutMs: 420_000}); err != nil {
		t.Fatalf("saveADKSettings: %v", err)
	}
	if _, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "agent-snapshot",
		Name:           "Snapshot Agent",
		ProviderID:     provider.ID,
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	snapshotResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk")
	if err != nil {
		t.Fatalf("GET snapshot: %v", err)
	}
	defer func() { jftradeCheckTestError(t, snapshotResp.Body.Close()) }()
	var snapshotEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Providers       []jfadk.Provider       `json:"providers"`
			Agents          []jfadk.Agent          `json:"agents"`
			Skills          []jfadk.Skill          `json:"skills"`
			Tools           []jfadk.ToolDescriptor `json:"tools"`
			RuntimeSettings ADKRuntimeSettings     `json:"runtimeSettings"`
		} `json:"data"`
	}
	if err := json.NewDecoder(snapshotResp.Body).Decode(&snapshotEnvelope); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if !snapshotEnvelope.OK {
		t.Fatalf("snapshot envelope = %+v", snapshotEnvelope)
	}
	if len(snapshotEnvelope.Data.Providers) == 0 || len(snapshotEnvelope.Data.Agents) == 0 || len(snapshotEnvelope.Data.Skills) == 0 || len(snapshotEnvelope.Data.Tools) == 0 {
		t.Fatalf("snapshot data incomplete: %+v", snapshotEnvelope.Data)
	}
	var snapshotProvider jfadk.Provider
	for _, item := range snapshotEnvelope.Data.Providers {
		if item.ID == provider.ID {
			snapshotProvider = item
			break
		}
	}
	if snapshotProvider.ID == "" {
		t.Fatalf("provider %q missing from snapshot: %+v", provider.ID, snapshotEnvelope.Data.Providers)
	}
	if snapshotProvider.RequestTimeoutMs != 240_000 {
		t.Fatalf("provider requestTimeoutMs = %d, want 240000", snapshotProvider.RequestTimeoutMs)
	}
	if snapshotEnvelope.Data.RuntimeSettings.RunTimeoutMs != 660_000 || snapshotEnvelope.Data.RuntimeSettings.StreamIdleTimeoutMs != 420_000 {
		t.Fatalf("runtimeSettings = %+v", snapshotEnvelope.Data.RuntimeSettings)
	}

	toolsResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/tools")
	if err != nil {
		t.Fatalf("GET tools: %v", err)
	}
	defer func() { jftradeCheckTestError(t, toolsResp.Body.Close()) }()
	var toolsEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Tools []jfadk.ToolDescriptor `json:"tools"`
		} `json:"data"`
	}
	if err := json.NewDecoder(toolsResp.Body).Decode(&toolsEnvelope); err != nil {
		t.Fatalf("decode tools: %v", err)
	}
	if !toolsEnvelope.OK || len(toolsEnvelope.Data.Tools) == 0 {
		t.Fatalf("tools envelope = %+v", toolsEnvelope)
	}
}
