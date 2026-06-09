package jftradeapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestADKApprovalRouteReturnsResolutionEnvelope(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "approval-agent",
		Name:           "Approval Agent",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	chatPayload := []byte(`{"agentId":"` + agent.ID + `","message":"@strategy.save_draft 保存一个策略草稿"}`)
	chatResp, err := http.Post(srv.URL+"/api/v1/adk/chat", "application/json", bytes.NewReader(chatPayload))
	if err != nil {
		t.Fatalf("POST adk chat: %v", err)
	}
	defer chatResp.Body.Close()
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("POST adk chat status = %d", chatResp.StatusCode)
	}
	var chatEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Run              jfadk.Run        `json:"run"`
			PendingApprovals []jfadk.Approval `json:"pendingApprovals"`
		} `json:"data"`
	}
	if err := json.NewDecoder(chatResp.Body).Decode(&chatEnvelope); err != nil {
		t.Fatalf("decode chat envelope: %v", err)
	}
	if !chatEnvelope.OK || chatEnvelope.Data.Run.Status != jfadk.RunStatusPending || len(chatEnvelope.Data.PendingApprovals) != 1 {
		t.Fatalf("chat envelope = %+v, want pending approval", chatEnvelope)
	}

	approvalID := chatEnvelope.Data.PendingApprovals[0].ID
	denyResp, err := http.Post(srv.URL+"/api/v1/adk/approvals/"+approvalID+"/deny", "application/json", nil)
	if err != nil {
		t.Fatalf("POST deny approval: %v", err)
	}
	defer denyResp.Body.Close()
	if denyResp.StatusCode != http.StatusOK {
		t.Fatalf("POST deny approval status = %d", denyResp.StatusCode)
	}
	var denyEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.ApprovalResolution `json:"data"`
	}
	if err := json.NewDecoder(denyResp.Body).Decode(&denyEnvelope); err != nil {
		t.Fatalf("decode deny envelope: %v", err)
	}
	if !denyEnvelope.OK {
		t.Fatal("expected deny envelope ok=true")
	}
	if denyEnvelope.Data.Approval.Status != jfadk.ApprovalStatusDenied {
		t.Fatalf("approval status = %q, want denied", denyEnvelope.Data.Approval.Status)
	}
	if denyEnvelope.Data.Run == nil || denyEnvelope.Data.Run.Status != jfadk.RunStatusPending || denyEnvelope.Data.Run.ResumeState != "approval_resuming" {
		t.Fatalf("resolution run = %+v, want pending background continuation", denyEnvelope.Data.Run)
	}
	if denyEnvelope.Data.Message != nil {
		t.Fatalf("resolution message = %+v, want no synchronous assistant summary", denyEnvelope.Data.Message)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
		if err != nil || !ok {
			t.Fatalf("stored run ok=%v err=%v", ok, err)
		}
		if run.Status == jfadk.RunStatusDenied {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
	if err != nil || !ok || run.Status != jfadk.RunStatusDenied {
		t.Fatalf("stored run after async denial = %+v ok=%v err=%v, want denied", run, ok, err)
	}
}

func TestADKProviderDeleteRejectsReferencedProvider(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	provider, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID:          "openai",
		DisplayName: "OpenAI",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		ProviderID:     provider.ID,
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/adk/providers/"+provider.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE provider: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("DELETE provider status = %d, want conflict", resp.StatusCode)
	}
}

func TestADKRunCancelAndFilteredList(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	run := jfadk.Run{
		ID: "run-cancel", SessionID: "session-1", AgentID: "agent-1",
		Status: jfadk.RunStatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := server.adkRuntime.Store().SaveRun(t.Context(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	cancelResp, err := http.Post(srv.URL+"/api/v1/adk/runs/"+run.ID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d", cancelResp.StatusCode)
	}

	listResp, err := http.Get(srv.URL + "/api/v1/adk/runs?status=CANCELLED&agentId=agent-1&limit=1")
	if err != nil {
		t.Fatalf("GET runs: %v", err)
	}
	defer listResp.Body.Close()
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Runs []jfadk.Run `json:"runs"`
			Page struct {
				Total int `json:"total"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if !envelope.OK || envelope.Data.Page.Total != 1 || len(envelope.Data.Runs) != 1 || envelope.Data.Runs[0].Status != jfadk.RunStatusCancelled {
		t.Fatalf("filtered runs envelope = %+v", envelope)
	}
}

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

	resp, err := http.Get(srv.URL + "/api/v1/adk/metrics")
	if err != nil {
		t.Fatalf("GET metrics: %v", err)
	}
	defer resp.Body.Close()
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

	resp, err := http.Get(srv.URL + "/api/v1/adk/metrics?limit=abc&offset=-1&status=FAILED")
	if err != nil {
		t.Fatalf("GET metrics with noise query params: %v", err)
	}
	defer resp.Body.Close()
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
	getResp, err := http.Get(srv.URL + "/api/v1/adk/optimization-tasks/" + task.ID)
	if err != nil {
		t.Fatalf("GET optimization task: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET optimization status = %d", getResp.StatusCode)
	}

	cancelResp, err := http.Post(srv.URL+"/api/v1/adk/optimization-tasks/"+task.ID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST optimization cancel: %v", err)
	}
	defer cancelResp.Body.Close()
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

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{ID: "workflow-agent", Name: "Workflow", Status: jfadk.AgentStatusEnabled})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	createResp, err := http.Post(srv.URL+"/api/v1/adk/tasks", "application/json", strings.NewReader(`{"id":"task-route","title":"Route task","status":"TODO","agentId":"`+agent.ID+`"}`))
	if err != nil {
		t.Fatalf("POST task: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST task status = %d", createResp.StatusCode)
	}
	patchReq, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/adk/tasks/task-route", strings.NewReader(`{"status":"DONE"}`))
	if err != nil {
		t.Fatalf("NewRequest patch task: %v", err)
	}
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("PUT task: %v", err)
	}
	defer patchResp.Body.Close()
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
	listResp, err := http.Get(srv.URL + "/api/v1/adk/tasks?status=DONE&agentId=" + agent.ID)
	if err != nil {
		t.Fatalf("GET tasks: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET tasks status = %d", listResp.StatusCode)
	}
	deleteReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/adk/tasks/task-route", nil)
	if err != nil {
		t.Fatalf("NewRequest delete task: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE task: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE task status = %d", deleteResp.StatusCode)
	}

	memoryResp, err := http.Post(srv.URL+"/api/v1/adk/memory", "application/json", strings.NewReader(`{"scope":"agent","agentId":"`+agent.ID+`","key":"style","value":"risk first"}`))
	if err != nil {
		t.Fatalf("POST memory: %v", err)
	}
	defer memoryResp.Body.Close()
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
	filterResp, err := http.Get(srv.URL + "/api/v1/adk/memory?scope=agent&agentId=" + agent.ID + "&key=style")
	if err != nil {
		t.Fatalf("GET memory: %v", err)
	}
	defer filterResp.Body.Close()
	if filterResp.StatusCode != http.StatusOK {
		t.Fatalf("GET memory status = %d", filterResp.StatusCode)
	}
	deleteMemoryReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/adk/memory/"+memoryEnvelope.Data.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest delete memory: %v", err)
	}
	deleteMemoryResp, err := http.DefaultClient.Do(deleteMemoryReq)
	if err != nil {
		t.Fatalf("DELETE memory: %v", err)
	}
	defer deleteMemoryResp.Body.Close()
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

	missingGetResp, err := http.Get(srv.URL + "/api/v1/adk/optimization-tasks/missing-task")
	if err != nil {
		t.Fatalf("GET missing optimization task: %v", err)
	}
	defer missingGetResp.Body.Close()
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

	missingCancelResp, err := http.Post(srv.URL+"/api/v1/adk/optimization-tasks/missing-task/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing optimization cancel: %v", err)
	}
	defer missingCancelResp.Body.Close()
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

	resp, err := http.Post(srv.URL+"/api/v1/assistant/chat", "application/json", bytes.NewReader([]byte(`{"prompt":"hello"}`)))
	if err != nil {
		t.Fatalf("POST assistant chat: %v", err)
	}
	defer resp.Body.Close()

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
	if _, err := server.store.saveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 660_000, StreamIdleTimeoutMs: 420_000}); err != nil {
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

	snapshotResp, err := http.Get(srv.URL + "/api/v1/adk")
	if err != nil {
		t.Fatalf("GET snapshot: %v", err)
	}
	defer snapshotResp.Body.Close()
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
	if snapshotEnvelope.Data.Providers[0].RequestTimeoutMs != 240_000 {
		t.Fatalf("provider requestTimeoutMs = %d, want 240000", snapshotEnvelope.Data.Providers[0].RequestTimeoutMs)
	}
	if snapshotEnvelope.Data.RuntimeSettings.RunTimeoutMs != 660_000 || snapshotEnvelope.Data.RuntimeSettings.StreamIdleTimeoutMs != 420_000 {
		t.Fatalf("runtimeSettings = %+v", snapshotEnvelope.Data.RuntimeSettings)
	}

	toolsResp, err := http.Get(srv.URL + "/api/v1/adk/tools")
	if err != nil {
		t.Fatalf("GET tools: %v", err)
	}
	defer toolsResp.Body.Close()
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

func TestADKSessionsCRUDAndFilteringRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.store.saveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 720_000, StreamIdleTimeoutMs: 420_000}); err != nil {
		t.Fatalf("saveADKSettings: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "session-agent",
		Name:           "Session Agent",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	createBody := []byte(`{"agentId":"` + agent.ID + `","title":"组合诊断会话"}`)
	createResp, err := http.Post(srv.URL+"/api/v1/adk/sessions", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST sessions: %v", err)
	}
	defer createResp.Body.Close()
	var createEnvelope struct {
		OK   bool          `json:"ok"`
		Data jfadk.Session `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode create session: %v", err)
	}
	if !createEnvelope.OK || createEnvelope.Data.AgentID != agent.ID {
		t.Fatalf("create session envelope = %+v", createEnvelope)
	}

	chatResp, err := http.Post(srv.URL+"/api/v1/adk/chat", "application/json", bytes.NewReader([]byte(`{"agentId":"`+agent.ID+`","sessionId":"`+createEnvelope.Data.ID+`","message":"@strategy.save_draft 保存会话草稿"}`)))
	if err != nil {
		t.Fatalf("POST session chat: %v", err)
	}
	defer chatResp.Body.Close()
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("POST session chat status = %d", chatResp.StatusCode)
	}

	listResp, err := http.Get(srv.URL + "/api/v1/adk/sessions?agentId=" + agent.ID + "&query=组合&limit=5")
	if err != nil {
		t.Fatalf("GET sessions: %v", err)
	}
	defer listResp.Body.Close()
	var listEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Sessions []jfadk.Session `json:"sessions"`
			Page     struct {
				Total    int  `json:"total"`
				Returned int  `json:"returned"`
				HasMore  bool `json:"hasMore"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if !listEnvelope.OK || listEnvelope.Data.Page.Total != 1 || len(listEnvelope.Data.Sessions) != 1 || listEnvelope.Data.Page.HasMore {
		t.Fatalf("list sessions envelope = %+v", listEnvelope)
	}

	getResp, err := http.Get(srv.URL + "/api/v1/adk/sessions/" + createEnvelope.Data.ID)
	if err != nil {
		t.Fatalf("GET session detail: %v", err)
	}
	defer getResp.Body.Close()
	var getEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Session  jfadk.Session         `json:"session"`
			Timeline []jfadk.TimelineEntry `json:"timeline"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getEnvelope); err != nil {
		t.Fatalf("decode session detail: %v", err)
	}
	if !getEnvelope.OK || getEnvelope.Data.Session.ID != createEnvelope.Data.ID || len(getEnvelope.Data.Timeline) == 0 {
		t.Fatalf("session detail envelope = %+v", getEnvelope)
	}

	renameReq, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID, bytes.NewReader([]byte(`{"title":"重命名会话"}`)))
	if err != nil {
		t.Fatalf("NewRequest rename: %v", err)
	}
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("PUT session rename: %v", err)
	}
	defer renameResp.Body.Close()
	var renameEnvelope struct {
		OK   bool          `json:"ok"`
		Data jfadk.Session `json:"data"`
	}
	if err := json.NewDecoder(renameResp.Body).Decode(&renameEnvelope); err != nil {
		t.Fatalf("decode rename session: %v", err)
	}
	if !renameEnvelope.OK || renameEnvelope.Data.Title != "重命名会话" {
		t.Fatalf("rename session envelope = %+v", renameEnvelope)
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest delete: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE session: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete session status = %d", deleteResp.StatusCode)
	}

	missingResp, err := http.Get(srv.URL + "/api/v1/adk/sessions/" + createEnvelope.Data.ID)
	if err != nil {
		t.Fatalf("GET deleted session: %v", err)
	}
	defer missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted session status = %d, want 404", missingResp.StatusCode)
	}
}

func TestADKAuditRouteFiltersByKindAndSubjectID(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit", "saved", map[string]any{"idx": 1})
	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit", "saved-again", map[string]any{"idx": 2})
	server.adkRuntime.RecordAudit(t.Context(), "provider.saved", "provider-audit", "provider", map[string]any{"idx": 3})

	resp, err := http.Get(srv.URL + "/api/v1/adk/audit?kind=agent.saved&subjectId=agent-audit&limit=1")
	if err != nil {
		t.Fatalf("GET audit: %v", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Events []jfadk.AuditEvent `json:"events"`
			Page   struct {
				Total    int `json:"total"`
				Returned int `json:"returned"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if !envelope.OK || envelope.Data.Page.Total != 2 || envelope.Data.Page.Returned != 1 || len(envelope.Data.Events) != 1 {
		t.Fatalf("audit envelope = %+v", envelope)
	}
	if envelope.Data.Events[0].Kind != "agent.saved" || envelope.Data.Events[0].SubjectID != "agent-audit" {
		t.Fatalf("audit event = %+v, want filtered agent.saved subject", envelope.Data.Events[0])
	}
}

func TestADKAuditRoutePaginationFallbacks(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit-1", "saved", nil)
	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit-2", "saved", nil)

	resp, err := http.Get(srv.URL + "/api/v1/adk/audit?limit=oops&offset=-10")
	if err != nil {
		t.Fatalf("GET audit with invalid page params: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Events []jfadk.AuditEvent `json:"events"`
			Page   struct {
				Limit    int  `json:"limit"`
				Offset   int  `json:"offset"`
				Total    int  `json:"total"`
				Returned int  `json:"returned"`
				HasMore  bool `json:"hasMore"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode audit fallback page: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("audit fallback envelope = %+v", envelope)
	}
	if envelope.Data.Page.Limit != 100 || envelope.Data.Page.Offset != 0 || envelope.Data.Page.Total != 2 || envelope.Data.Page.Returned != 2 || envelope.Data.Page.HasMore {
		t.Fatalf("audit fallback page = %+v, want default pagination", envelope.Data.Page)
	}
}

func TestADKChatStreamEmitsSessionRunAndFinalEvents(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.store.saveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 720_000, StreamIdleTimeoutMs: 420_000}); err != nil {
		t.Fatalf("saveADKSettings: %v", err)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "stream-agent",
		Name:           "Stream Agent",
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/adk/chat/stream", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-ADK-Stream-Idle-Timeout-Ms") != "420000" {
		t.Fatalf("stream idle timeout header = %q, want 420000", rec.Header().Get("X-ADK-Stream-Idle-Timeout-Ms"))
	}
	frames := parseADKStreamFrames(t, rec.Body.String())
	var eventTypes []string
	var finalEvent *adkChatStreamEvent
	for i := range frames {
		eventTypes = append(eventTypes, frames[i].Type)
		if frames[i].Type == "final" {
			finalEvent = &frames[i]
		}
	}
	sort.Strings(eventTypes)
	if !containsString(eventTypes, "run") || !containsString(eventTypes, "session") || !containsString(eventTypes, "final") {
		t.Fatalf("stream event types = %#v, want session/run/final; raw=%q", eventTypes, rec.Body.String())
	}
	if finalEvent == nil || finalEvent.Response == nil {
		t.Fatalf("final event = %+v, want response", finalEvent)
	}
	if finalEvent.Response.Run.AgentID != agent.ID {
		t.Fatalf("final run agent = %q, want %q", finalEvent.Response.Run.AgentID, agent.ID)
	}
	if finalEvent.Response.Run.MaxDurationMs != 720_000 {
		t.Fatalf("final run maxDurationMs = %d, want 720000", finalEvent.Response.Run.MaxDurationMs)
	}
}

func TestADKChatStreamReturnsErrorEventForInvalidPayload(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := http.Post(srv.URL+"/api/v1/adk/chat/stream", "application/json", strings.NewReader(`{"message":`))
	if err != nil {
		t.Fatalf("POST invalid chat stream: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll invalid stream: %v", err)
	}
	frames := parseADKStreamFrames(t, string(body))
	if len(frames) == 0 {
		t.Fatalf("expected at least one SSE frame, raw=%q", string(body))
	}
	last := frames[len(frames)-1]
	if last.Type != "error" || !strings.HasPrefix(last.Message, "invalid chat payload") {
		t.Fatalf("last stream event = %+v, want invalid payload error", last)
	}
}

func TestADKProviderSaveRejectsInvalidPayload(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := http.Post(srv.URL+"/api/v1/adk/providers", "application/json", strings.NewReader(`{"displayName":`))
	if err != nil {
		t.Fatalf("POST invalid provider payload: %v", err)
	}
	defer resp.Body.Close()

	status, code, message := decodeAPIErrorEnvelope(t, resp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "invalid provider payload" {
		t.Fatalf("provider validation error = %d/%s/%q, want 400/BAD_REQUEST/invalid provider payload", status, code, message)
	}
}

func TestADKProviderSaveReturnsRequestTimeoutMs(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := http.Post(srv.URL+"/api/v1/adk/providers", "application/json", strings.NewReader(`{
		"displayName":"Slow Provider",
		"baseUrl":"https://api.openai.com/v1",
		"model":"gpt-4o-mini",
		"requestTimeoutMs":250000,
		"enabled":true
	}`))
	if err != nil {
		t.Fatalf("POST provider: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST provider status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data jfadk.Provider `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode provider save: %v", err)
	}
	if !envelope.OK || envelope.Data.RequestTimeoutMs != 250_000 {
		t.Fatalf("provider save envelope = %+v", envelope)
	}
}

func TestADKAgentSaveValidationFailures(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	disabledProvider, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID:          "provider-disabled",
		DisplayName: "Disabled Provider",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		APIKey:      "sk-disabled",
		Enabled:     false,
	})
	if err != nil {
		t.Fatalf("SaveProvider disabled: %v", err)
	}
	if _, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID:          "provider-no-key",
		DisplayName: "No Key Provider",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("SaveProvider no-key: %v", err)
	}

	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{
			name:        "disabled provider",
			body:        `{"id":"agent-disabled-provider","name":"Agent","providerId":"` + disabledProvider.ID + `","permissionMode":"approval","status":"ENABLED"}`,
			wantMessage: "provider is disabled",
		},
		{
			name:        "missing provider key",
			body:        `{"id":"agent-no-key","name":"Agent","providerId":"provider-no-key","permissionMode":"approval","status":"ENABLED"}`,
			wantMessage: "provider API key is not configured",
		},
		{
			name:        "unknown tool",
			body:        `{"id":"agent-bad-tool","name":"Agent","permissionMode":"approval","status":"ENABLED","tools":["tool.does_not_exist"]}`,
			wantMessage: "unknown ADK tool: tool.does_not_exist",
		},
		{
			name:        "unknown skill",
			body:        `{"id":"agent-bad-skill","name":"Agent","permissionMode":"approval","status":"ENABLED","skills":["skill-does-not-exist"]}`,
			wantMessage: "unknown ADK skill: skill-does-not-exist",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/api/v1/adk/agents", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST agent payload: %v", err)
			}
			defer resp.Body.Close()

			status, code, message := decodeAPIErrorEnvelope(t, resp)
			if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != tc.wantMessage {
				t.Fatalf("agent validation error = %d/%s/%q, want 400/BAD_REQUEST/%q", status, code, message, tc.wantMessage)
			}
		})
	}
}

func TestADKSkillInstallAndUninstallFailureRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	installResp, err := http.Post(srv.URL+"/api/v1/adk/skills", "application/json", strings.NewReader(`{"url":"not-a-valid-url"}`))
	if err != nil {
		t.Fatalf("POST invalid skill install: %v", err)
	}
	defer installResp.Body.Close()
	status, code, message := decodeAPIErrorEnvelope(t, installResp)
	if status != http.StatusBadRequest || code != "ADK_SKILL_INSTALL_FAILED" || strings.TrimSpace(message) == "" {
		t.Fatalf("skill install error = %d/%s/%q, want 400/ADK_SKILL_INSTALL_FAILED/non-empty", status, code, message)
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/adk/skills/jftrade-market", nil)
	if err != nil {
		t.Fatalf("NewRequest delete builtin skill: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE builtin skill: %v", err)
	}
	defer deleteResp.Body.Close()
	status, code, message = decodeAPIErrorEnvelope(t, deleteResp)
	if status != http.StatusInternalServerError || code != "ADK_SKILL_UNINSTALL_FAILED" || !strings.Contains(strings.ToLower(message), "builtin") {
		t.Fatalf("skill uninstall error = %d/%s/%q, want 500/ADK_SKILL_UNINSTALL_FAILED/*builtin*", status, code, message)
	}
}

func TestADKBindAgentWithPreinstalledNeodataFinancialSearch(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	skillDir := filepath.Join(server.adkRuntime.Store().SkillsPath(), "neodata-financial-search")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(`---
name: neodata-financial-search
description: Search NeoData financial filings and earnings materials.
allowed-tools: [http.fetch]
metadata:
  version: 2026.06
---
Use NeoData search results as reference material and cite the source URL.`), 0o644); err != nil {
		t.Fatalf("WriteFile skill doc: %v", err)
	}

	createBody := `{"id":"agent-neodata","name":"Agent NeoData","permissionMode":"approval","status":"ENABLED","tools":["http.fetch"],"skills":["neodata-financial-search"]}`
	createResp, err := http.Post(srv.URL+"/api/v1/adk/agents", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST create agent: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST create agent status = %d", createResp.StatusCode)
	}
	var agentEnvelope struct {
		OK   bool        `json:"ok"`
		Data jfadk.Agent `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&agentEnvelope); err != nil {
		t.Fatalf("decode agent envelope: %v", err)
	}
	if !agentEnvelope.OK || !containsString(agentEnvelope.Data.Skills, "neodata-financial-search") {
		t.Fatalf("agent envelope = %+v", agentEnvelope)
	}
}

func TestADKSessionNegativeRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	createResp, err := http.Post(srv.URL+"/api/v1/adk/sessions", "application/json", strings.NewReader(`{"agentId":"missing-agent","title":"x"}`))
	if err != nil {
		t.Fatalf("POST invalid session create: %v", err)
	}
	defer createResp.Body.Close()
	status, code, message := decodeAPIErrorEnvelope(t, createResp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "enabled agent is required" {
		t.Fatalf("session create error = %d/%s/%q, want 400/BAD_REQUEST/enabled agent is required", status, code, message)
	}

	invalidDetailReq := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/api/v1/adk/sessions/%zz"},
		Header: make(http.Header),
	}
	invalidDetailRec := httptest.NewRecorder()
	server.ServeHTTP(invalidDetailRec, invalidDetailReq)
	status, code, message = decodeAPIErrorRecorder(t, invalidDetailRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "sessionId is invalid" {
		t.Fatalf("session invalid id error = %d/%s/%q", status, code, message)
	}

	missingResp, err := http.Get(srv.URL + "/api/v1/adk/sessions/session-missing")
	if err != nil {
		t.Fatalf("GET missing session detail: %v", err)
	}
	defer missingResp.Body.Close()
	status, code, message = decodeAPIErrorEnvelope(t, missingResp)
	if status != http.StatusNotFound || code != "NOT_FOUND" || message != "session not found" {
		t.Fatalf("session missing error = %d/%s/%q", status, code, message)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "session-negative-agent",
		Name:           "Session Negative Agent",
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := server.adkRuntime.Store().CreateSession(t.Context(), agent.ID, "valid")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	renameReq, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/adk/sessions/"+session.ID, strings.NewReader(`{"title":`))
	if err != nil {
		t.Fatalf("NewRequest invalid rename: %v", err)
	}
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("PUT invalid rename: %v", err)
	}
	defer renameResp.Body.Close()
	status, code, message = decodeAPIErrorEnvelope(t, renameResp)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "invalid session payload" {
		t.Fatalf("session rename payload error = %d/%s/%q", status, code, message)
	}
}

func TestADKRunNegativeRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	server := newTestServer(t, store)
	invalidCancelReq := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Path: "/api/v1/adk/runs/%zz/cancel"},
		Header: make(http.Header),
	}
	invalidCancelRec := httptest.NewRecorder()
	server.ServeHTTP(invalidCancelRec, invalidCancelReq)
	status, code, message := decodeAPIErrorRecorder(t, invalidCancelRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "runId is invalid" {
		t.Fatalf("run cancel invalid error = %d/%s/%q", status, code, message)
	}

	cancelMissingResp, err := http.Post(srv.URL+"/api/v1/adk/runs/run-missing/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing cancel run: %v", err)
	}
	defer cancelMissingResp.Body.Close()
	status, code, message = decodeAPIErrorEnvelope(t, cancelMissingResp)
	if status != http.StatusNotFound || code != "ADK_RUN_CANCEL_FAILED" || message != "run not found" {
		t.Fatalf("run cancel missing error = %d/%s/%q", status, code, message)
	}

	invalidGetReq := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/api/v1/adk/runs/%zz"},
		Header: make(http.Header),
	}
	invalidGetRec := httptest.NewRecorder()
	server.ServeHTTP(invalidGetRec, invalidGetReq)
	status, code, message = decodeAPIErrorRecorder(t, invalidGetRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "runId is invalid" {
		t.Fatalf("run get invalid error = %d/%s/%q", status, code, message)
	}

	getMissingResp, err := http.Get(srv.URL + "/api/v1/adk/runs/run-missing")
	if err != nil {
		t.Fatalf("GET missing run: %v", err)
	}
	defer getMissingResp.Body.Close()
	status, code, message = decodeAPIErrorEnvelope(t, getMissingResp)
	if status != http.StatusNotFound || code != "NOT_FOUND" || message != "run not found" {
		t.Fatalf("run get missing error = %d/%s/%q", status, code, message)
	}
}

func TestADKApprovalNegativeAndIdempotentRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	invalidReq := &http.Request{
		Method: http.MethodPost,
		URL:    &url.URL{Path: "/api/v1/adk/approvals/%zz/approve"},
		Header: make(http.Header),
	}
	invalidRec := httptest.NewRecorder()
	server.ServeHTTP(invalidRec, invalidReq)
	status, code, message := decodeAPIErrorRecorder(t, invalidRec)
	if status != http.StatusBadRequest || code != "BAD_REQUEST" || message != "approvalId is invalid" {
		t.Fatalf("approval invalid error = %d/%s/%q", status, code, message)
	}

	missingResp, err := http.Post(srv.URL+"/api/v1/adk/approvals/approval-missing/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing approval: %v", err)
	}
	defer missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusOK {
		t.Fatalf("missing approval status = %d, want 200", missingResp.StatusCode)
	}
	var missingEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.ApprovalResolution `json:"data"`
	}
	if err := json.NewDecoder(missingResp.Body).Decode(&missingEnvelope); err != nil {
		t.Fatalf("decode missing approval envelope: %v", err)
	}
	if !missingEnvelope.OK || missingEnvelope.Data.Run != nil || missingEnvelope.Data.Message != nil || missingEnvelope.Data.Approval.ID != "" || missingEnvelope.Data.Approval.Status != "" {
		t.Fatalf("missing approval envelope = %+v, want idempotent empty success result", missingEnvelope)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "approval-idempotent-agent",
		Name:           "Approval Idempotent Agent",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	chatResp, err := http.Post(srv.URL+"/api/v1/adk/chat", "application/json", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"@strategy.save_draft 保存草稿"}`))
	if err != nil {
		t.Fatalf("POST chat for approval: %v", err)
	}
	defer chatResp.Body.Close()
	var chatEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			PendingApprovals []jfadk.Approval `json:"pendingApprovals"`
		} `json:"data"`
	}
	if err := json.NewDecoder(chatResp.Body).Decode(&chatEnvelope); err != nil {
		t.Fatalf("decode chat envelope: %v", err)
	}
	if len(chatEnvelope.Data.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(chatEnvelope.Data.PendingApprovals))
	}
	approvalID := chatEnvelope.Data.PendingApprovals[0].ID

	firstResp, err := http.Post(srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST first approval: %v", err)
	}
	defer firstResp.Body.Close()
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("first approval status = %d", firstResp.StatusCode)
	}

	secondResp, err := http.Post(srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST second approval: %v", err)
	}
	defer secondResp.Body.Close()
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("second approval status = %d", secondResp.StatusCode)
	}
	var secondEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.ApprovalResolution `json:"data"`
	}
	if err := json.NewDecoder(secondResp.Body).Decode(&secondEnvelope); err != nil {
		t.Fatalf("decode second approval envelope: %v", err)
	}
	if !secondEnvelope.OK || secondEnvelope.Data.Approval.Status != jfadk.ApprovalStatusApproved {
		t.Fatalf("second approval envelope = %+v, want idempotent approved result", secondEnvelope)
	}
}

func parseADKStreamFrames(t *testing.T, body string) []adkChatStreamEvent {
	t.Helper()
	segments := strings.Split(body, "\n\n")
	events := make([]adkChatStreamEvent, 0, len(segments))
	for _, segment := range segments {
		if !strings.Contains(segment, "data:") {
			continue
		}
		var dataLines []string
		for _, line := range strings.Split(segment, "\n") {
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		payload := strings.Join(dataLines, "\n")
		if strings.TrimSpace(payload) == "" {
			continue
		}
		var event adkChatStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatalf("unmarshal SSE frame: %v; payload=%q", err, payload)
		}
		events = append(events, event)
	}
	return events
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func decodeAPIErrorEnvelope(t *testing.T, resp *http.Response) (int, string, string) {
	t.Helper()
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return resp.StatusCode, envelope.Error.Code, envelope.Error.Message
}

func decodeAPIErrorRecorder(t *testing.T, rec *httptest.ResponseRecorder) (int, string, string) {
	t.Helper()
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode recorder error envelope: %v body=%q", err, rec.Body.String())
	}
	return rec.Code, envelope.Error.Code, envelope.Error.Message
}
