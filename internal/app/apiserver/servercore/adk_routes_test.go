package servercore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	adksession "google.golang.org/adk/session"

	asst "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestADKApprovalApproveRouteReturnsRunningResolutionEnvelope(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	releaseTool := make(chan struct{})
	toolStarted := make(chan struct{}, 1)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name: "approval.required", Permission: "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		select {
		case toolStarted <- struct{}{}:
		default:
		}
		select {
		case <-releaseTool:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return map[string]any{"saved": true}, nil
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "approval-agent-running",
		Name:           "Approval Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	chatPayload := []byte(`{"agentId":"` + agent.ID + `","message":"<execute-tool name=\"approval.required\" />"}`)
	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", bytes.NewReader(chatPayload))
	if err != nil {
		t.Fatalf("POST adk chat: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
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
	approveResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST approve approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, approveResp.Body.Close()) }()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("POST approve approval status = %d", approveResp.StatusCode)
	}
	var approveEnvelope struct {
		OK   bool                     `json:"ok"`
		Data jfadk.ApprovalResolution `json:"data"`
	}
	if err := json.NewDecoder(approveResp.Body).Decode(&approveEnvelope); err != nil {
		t.Fatalf("decode approve envelope: %v", err)
	}
	if !approveEnvelope.OK {
		t.Fatal("expected approve envelope ok=true")
	}
	if approveEnvelope.Data.Approval.Status != jfadk.ApprovalStatusApproved {
		t.Fatalf("approval status = %q, want approved", approveEnvelope.Data.Approval.Status)
	}
	if approveEnvelope.Data.Run == nil || approveEnvelope.Data.Run.Status != jfadk.RunStatusRunning || approveEnvelope.Data.Run.ResumeState != "approval_resuming" {
		t.Fatalf("resolution run = %+v, want running background continuation", approveEnvelope.Data.Run)
	}
	if len(approveEnvelope.Data.Run.ToolCalls) != 1 || approveEnvelope.Data.Run.ToolCalls[0].Status != "RUNNING" {
		t.Fatalf("resolution tool calls = %+v, want running", approveEnvelope.Data.Run.ToolCalls)
	}

	select {
	case <-toolStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approved continuation to resume")
	}

	runResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/runs/"+chatEnvelope.Data.Run.ID)
	if err != nil {
		t.Fatalf("GET run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, runResp.Body.Close()) }()
	if runResp.StatusCode != http.StatusOK {
		t.Fatalf("GET run status = %d", runResp.StatusCode)
	}
	var runEnvelope struct {
		OK   bool      `json:"ok"`
		Data jfadk.Run `json:"data"`
	}
	if err := json.NewDecoder(runResp.Body).Decode(&runEnvelope); err != nil {
		t.Fatalf("decode run envelope: %v", err)
	}
	if !runEnvelope.OK || runEnvelope.Data.Status != jfadk.RunStatusRunning {
		t.Fatalf("run envelope = %+v, want running", runEnvelope)
	}

	sessionResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+runEnvelope.Data.SessionID)
	if err != nil {
		t.Fatalf("GET session detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, sessionResp.Body.Close()) }()
	if sessionResp.StatusCode != http.StatusOK {
		t.Fatalf("GET session detail status = %d", sessionResp.StatusCode)
	}
	var sessionEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Timeline []jfadk.TimelineEntry `json:"timeline"`
		} `json:"data"`
	}
	if err := json.NewDecoder(sessionResp.Body).Decode(&sessionEnvelope); err != nil {
		t.Fatalf("decode session envelope: %v", err)
	}
	toolGroupSeen := false
	for _, entry := range sessionEnvelope.Data.Timeline {
		if entry.Kind == jfadk.TimelineKindApprovalGroup {
			t.Fatalf("timeline approval group = %+v, want resolved approval omitted", entry)
		}
		if entry.Kind == jfadk.TimelineKindToolGroup && entry.RunID == chatEnvelope.Data.Run.ID {
			if len(entry.ToolCalls) == 1 && entry.ToolCalls[0].Status == "RUNNING" {
				toolGroupSeen = true
			}
		}
	}
	if !toolGroupSeen {
		t.Fatalf("session timeline = %+v, want running tool group for run %s", sessionEnvelope.Data.Timeline, chatEnvelope.Data.Run.ID)
	}

	close(releaseTool)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
		if err != nil || !ok {
			t.Fatalf("stored run ok=%v err=%v", ok, err)
		}
		if run.Status == jfadk.RunStatusCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
	t.Fatalf("stored run after async approval = %+v ok=%v err=%v, want completed", run, ok, err)
}

func TestADKApprovalRouteReturnsResolutionEnvelope(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "approval-agent",
		Name:           "Approval Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	chatPayload := []byte(`{"agentId":"` + agent.ID + `","message":"<execute-tool name=\"approval.required\" />"}`)
	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", bytes.NewReader(chatPayload))
	if err != nil {
		t.Fatalf("POST adk chat: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
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
	denyResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/deny", "application/json", nil)
	if err != nil {
		t.Fatalf("POST deny approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, denyResp.Body.Close()) }()
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
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
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

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/providers/"+provider.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE provider: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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
	cancelResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/runs/"+run.ID+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	defer func() { jftradeCheckTestError(t, cancelResp.Body.Close()) }()
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d", cancelResp.StatusCode)
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/runs?status=CANCELLED&agentId=agent-1&limit=1")
	if err != nil {
		t.Fatalf("GET runs: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
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

func TestADKRunPauseAndResumeRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	run := jfadk.Run{
		ID: "run-goal-pause", SessionID: "session-1", AgentID: "agent-1",
		Status: jfadk.RunStatusRunning, Message: "goal running", WorkMode: jfadk.WorkModeLoop,
		WorkflowStatus: "RUNNING", CreatedAt: now, UpdatedAt: now,
		ToolCalls: []jfadk.ToolCall{}, PendingApprovals: []jfadk.Approval{},
	}
	if err := server.adkRuntime.Store().SaveRun(t.Context(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	pauseResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/runs/"+run.ID+"/pause", "application/json", nil)
	if err != nil {
		t.Fatalf("POST pause: %v", err)
	}
	defer func() { jftradeCheckTestError(t, pauseResp.Body.Close()) }()
	if pauseResp.StatusCode != http.StatusOK {
		t.Fatalf("pause status = %d", pauseResp.StatusCode)
	}
	var pauseEnvelope struct {
		OK   bool      `json:"ok"`
		Data jfadk.Run `json:"data"`
	}
	if err := json.NewDecoder(pauseResp.Body).Decode(&pauseEnvelope); err != nil {
		t.Fatalf("decode pause: %v", err)
	}
	if !pauseEnvelope.OK || pauseEnvelope.Data.PauseRequestedAt == nil || pauseEnvelope.Data.ResumeState != "user_pause_requested" {
		t.Fatalf("pause envelope = %+v, want pause requested", pauseEnvelope)
	}

	pausedAt := time.Now().UTC().Format(time.RFC3339Nano)
	paused := pauseEnvelope.Data
	paused.Status = jfadk.RunStatusPaused
	paused.WorkflowStatus = "PAUSED"
	paused.PausedAt = &pausedAt
	paused.PausedReason = "user"
	paused.ResumeState = "user_paused"
	if err := server.adkRuntime.Store().SaveRun(t.Context(), paused); err != nil {
		t.Fatalf("Save paused run: %v", err)
	}
	resumeResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/runs/"+run.ID+"/resume", "application/json", nil)
	if err != nil {
		t.Fatalf("POST resume: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resumeResp.Body.Close()) }()
	if resumeResp.StatusCode != http.StatusOK {
		t.Fatalf("resume status = %d", resumeResp.StatusCode)
	}
	var resumeEnvelope struct {
		OK   bool      `json:"ok"`
		Data jfadk.Run `json:"data"`
	}
	if err := json.NewDecoder(resumeResp.Body).Decode(&resumeEnvelope); err != nil {
		t.Fatalf("decode resume: %v", err)
	}
	if !resumeEnvelope.OK || resumeEnvelope.Data.Status != jfadk.RunStatusRunning || resumeEnvelope.Data.PauseRequestedAt != nil || resumeEnvelope.Data.PausedAt != nil {
		t.Fatalf("resume envelope = %+v, want running with pause fields cleared", resumeEnvelope)
	}
}

func TestADKRunPauseResumeRoutesRejectInvalidRuns(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	child := jfadk.Run{
		ID: "run-child-pause", SessionID: "session-1", AgentID: "agent-1",
		Status: jfadk.RunStatusRunning, WorkMode: jfadk.WorkModeLoop, ParentRunID: "run-parent",
		WorkflowStatus: "RUNNING", CreatedAt: now, UpdatedAt: now,
		ToolCalls: []jfadk.ToolCall{}, PendingApprovals: []jfadk.Approval{},
	}
	if err := server.adkRuntime.Store().SaveRun(t.Context(), child); err != nil {
		t.Fatalf("SaveRun child: %v", err)
	}
	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/runs/"+child.ID+"/pause", "application/json", nil)
	if err != nil {
		t.Fatalf("POST pause child: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pause child status = %d, want bad request", resp.StatusCode)
	}
	missingResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/runs/missing-run/resume", "application/json", nil)
	if err != nil {
		t.Fatalf("POST resume missing: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("resume missing status = %d, want not found", missingResp.StatusCode)
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
	if snapshotEnvelope.Data.Providers[0].RequestTimeoutMs != 240_000 {
		t.Fatalf("provider requestTimeoutMs = %d, want 240000", snapshotEnvelope.Data.Providers[0].RequestTimeoutMs)
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

func TestADKSessionsCRUDAndFilteringRoutes(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.store.SaveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 720_000, StreamIdleTimeoutMs: 420_000}); err != nil {
		t.Fatalf("saveADKSettings: %v", err)
	}
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "session-agent",
		Name:           "Session Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	createBody := []byte(`{"agentId":"` + agent.ID + `","title":"组合诊断会话"}`)
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/sessions", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST sessions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
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

	composerReq, err := http.NewRequestWithContext(t.Context(),
		http.MethodPatch,
		srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID+"/composer-state",
		bytes.NewReader([]byte(`{"chatDraft":"未发送草稿","workModeOverride":"loop","permissionModeOverride":"less_approval","goalObjectiveDraft":"目标草稿","goalObjectiveTouched":true}`)),
	)
	if err != nil {
		t.Fatalf("NewRequest composer state: %v", err)
	}
	composerReq.Header.Set("Content-Type", "application/json")
	composerResp, err := http.DefaultClient.Do(composerReq)
	if err != nil {
		t.Fatalf("PATCH composer state: %v", err)
	}
	defer func() { jftradeCheckTestError(t, composerResp.Body.Close()) }()
	var composerEnvelope struct {
		OK   bool                       `json:"ok"`
		Data jfadk.SessionComposerState `json:"data"`
	}
	if err := json.NewDecoder(composerResp.Body).Decode(&composerEnvelope); err != nil {
		t.Fatalf("decode composer state: %v", err)
	}
	if !composerEnvelope.OK || composerEnvelope.Data.ChatDraft != "未发送草稿" || composerEnvelope.Data.WorkModeOverride != jfadk.WorkModeLoop || composerEnvelope.Data.PermissionModeOverride != jfadk.PermissionModeLessApproval {
		t.Fatalf("composer state envelope = %+v", composerEnvelope)
	}

	invalidComposerReq, err := http.NewRequestWithContext(t.Context(),
		http.MethodPatch,
		srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID+"/composer-state",
		bytes.NewReader([]byte(`{"workModeOverride":"sequential"}`)),
	)
	if err != nil {
		t.Fatalf("NewRequest invalid composer state: %v", err)
	}
	invalidComposerReq.Header.Set("Content-Type", "application/json")
	invalidComposerResp, err := http.DefaultClient.Do(invalidComposerReq)
	if err != nil {
		t.Fatalf("PATCH invalid composer state: %v", err)
	}
	defer func() { jftradeCheckTestError(t, invalidComposerResp.Body.Close()) }()
	if invalidComposerResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid composer status = %d, want 400", invalidComposerResp.StatusCode)
	}

	invalidPermissionReq, err := http.NewRequestWithContext(t.Context(),
		http.MethodPatch,
		srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID+"/composer-state",
		bytes.NewReader([]byte(`{"permissionModeOverride":"root"}`)),
	)
	if err != nil {
		t.Fatalf("NewRequest invalid permission state: %v", err)
	}
	invalidPermissionReq.Header.Set("Content-Type", "application/json")
	invalidPermissionResp, err := http.DefaultClient.Do(invalidPermissionReq)
	if err != nil {
		t.Fatalf("PATCH invalid permission state: %v", err)
	}
	defer func() { jftradeCheckTestError(t, invalidPermissionResp.Body.Close()) }()
	if invalidPermissionResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid permission status = %d, want 400", invalidPermissionResp.StatusCode)
	}

	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", bytes.NewReader([]byte(`{"agentId":"`+agent.ID+`","sessionId":"`+createEnvelope.Data.ID+`","message":"@strategy.save_draft 保存会话草稿"}`)))
	if err != nil {
		t.Fatalf("POST session chat: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("POST session chat status = %d", chatResp.StatusCode)
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions?agentId="+agent.ID+"&query=组合&limit=5")
	if err != nil {
		t.Fatalf("GET sessions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
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
	listPayload, err := json.Marshal(listEnvelope.Data.Sessions)
	if err != nil {
		t.Fatalf("marshal list sessions: %v", err)
	}
	if bytes.Contains(listPayload, []byte("未发送草稿")) {
		t.Fatalf("session list leaked composer draft: %s", string(listPayload))
	}

	getResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID)
	if err != nil {
		t.Fatalf("GET session detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getResp.Body.Close()) }()
	var getEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Session       jfadk.Session              `json:"session"`
			Timeline      []jfadk.TimelineEntry      `json:"timeline"`
			ComposerState jfadk.SessionComposerState `json:"composerState"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getEnvelope); err != nil {
		t.Fatalf("decode session detail: %v", err)
	}
	if !getEnvelope.OK || getEnvelope.Data.Session.ID != createEnvelope.Data.ID || len(getEnvelope.Data.Timeline) == 0 {
		t.Fatalf("session detail envelope = %+v", getEnvelope)
	}
	if getEnvelope.Data.ComposerState.ChatDraft != "未发送草稿" || getEnvelope.Data.ComposerState.GoalObjectiveDraft != "目标草稿" {
		t.Fatalf("session detail composer state = %+v", getEnvelope.Data.ComposerState)
	}

	renameReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID, bytes.NewReader([]byte(`{"title":"重命名会话"}`)))
	if err != nil {
		t.Fatalf("NewRequest rename: %v", err)
	}
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("PUT session rename: %v", err)
	}
	defer func() { jftradeCheckTestError(t, renameResp.Body.Close()) }()
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

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID, nil)
	if err != nil {
		t.Fatalf("NewRequest delete: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE session: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete session status = %d", deleteResp.StatusCode)
	}

	missingResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+createEnvelope.Data.ID)
	if err != nil {
		t.Fatalf("GET deleted session: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted session status = %d, want 404", missingResp.StatusCode)
	}
}

func TestADKSessionDetailOmitsResolvedApprovalGroups(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "session-approval-agent",
		Name:           "Session Approval Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := server.adkRuntime.Store().CreateSession(t.Context(), agent.ID, "approval cleanup")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", strings.NewReader(`{"agentId":"`+agent.ID+`","sessionId":"`+session.ID+`","message":"<execute-tool name=\"approval.required\" />"}`))
	if err != nil {
		t.Fatalf("POST session chat: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("POST session chat status = %d", chatResp.StatusCode)
	}
	var chatEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Run              jfadk.Run        `json:"run"`
			PendingApprovals []jfadk.Approval `json:"pendingApprovals"`
		} `json:"data"`
	}
	if err := json.NewDecoder(chatResp.Body).Decode(&chatEnvelope); err != nil {
		t.Fatalf("decode session chat: %v", err)
	}
	if !chatEnvelope.OK || len(chatEnvelope.Data.PendingApprovals) != 1 {
		t.Fatalf("chat envelope = %+v, want one pending approval", chatEnvelope)
	}
	approvalID := chatEnvelope.Data.PendingApprovals[0].ID

	approveResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST approve: %v", err)
	}
	defer func() { jftradeCheckTestError(t, approveResp.Body.Close()) }()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d", approveResp.StatusCode)
	}

	deadline := time.Now().Add(2 * time.Second)
	terminal := false
	for time.Now().Before(deadline) {
		run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
		if err != nil || !ok {
			t.Fatalf("Run lookup err=%v ok=%v", err, ok)
		}
		if run.Status != jfadk.RunStatusPending && run.Status != jfadk.RunStatusRunning {
			terminal = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !terminal {
		run, ok, err := server.adkRuntime.Store().Run(t.Context(), chatEnvelope.Data.Run.ID)
		t.Fatalf("run did not reach terminal status after approval: run=%+v ok=%v err=%v", run, ok, err)
	}

	getResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/"+session.ID)
	if err != nil {
		t.Fatalf("GET session detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getResp.Body.Close()) }()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET session detail status = %d", getResp.StatusCode)
	}
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
	if !getEnvelope.OK {
		t.Fatalf("session detail envelope = %+v", getEnvelope)
	}
	for _, entry := range getEnvelope.Data.Timeline {
		if entry.Kind == jfadk.TimelineKindApprovalGroup {
			t.Fatalf("timeline approval group = %+v, want resolved approval omitted", entry)
		}
	}

	listResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/approvals?status=PENDING")
	if err != nil {
		t.Fatalf("GET pending approvals: %v", err)
	}
	defer func() { jftradeCheckTestError(t, listResp.Body.Close()) }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("GET pending approvals status = %d", listResp.StatusCode)
	}
	var listEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Approvals []jfadk.Approval `json:"approvals"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode pending approvals: %v", err)
	}
	if !listEnvelope.OK {
		t.Fatalf("pending approvals envelope = %+v", listEnvelope)
	}
	for _, approval := range listEnvelope.Data.Approvals {
		if approval.ID == approvalID {
			t.Fatalf("pending approvals = %+v, want resolved approval %q omitted", listEnvelope.Data.Approvals, approvalID)
		}
	}
}

func TestADKAuditRouteFiltersByKindAndSubjectID(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit", "saved", map[string]any{"idx": 1})
	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit", "saved-again", map[string]any{"idx": 2})
	server.adkRuntime.RecordAudit(t.Context(), "provider.saved", "provider-audit", "provider", map[string]any{"idx": 3})

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/audit?kind=agent.saved&subjectId=agent-audit&limit=1")
	if err != nil {
		t.Fatalf("GET audit: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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

func TestADKAuditRouteDefaultPagination(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit-1", "saved", nil)
	server.adkRuntime.RecordAudit(t.Context(), "agent.saved", "agent-audit-2", "saved", nil)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/audit?limit=oops&offset=-10")
	if err != nil {
		t.Fatalf("GET audit with invalid page params: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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
		t.Fatalf("decode audit default page: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("audit default page envelope = %+v", envelope)
	}
	if envelope.Data.Page.Limit != 100 || envelope.Data.Page.Offset != 0 || envelope.Data.Page.Total != 2 || envelope.Data.Page.Returned != 2 || envelope.Data.Page.HasMore {
		t.Fatalf("audit default page = %+v, want default pagination", envelope.Data.Page)
	}
}

func TestADKChatStreamEmitsSessionRunAndFinalEvents(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.store.SaveADKSettings(ADKRuntimeSettings{RunTimeoutMs: 720_000, StreamIdleTimeoutMs: 420_000}); err != nil {
		t.Fatalf("saveADKSettings: %v", err)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "stream-agent",
		Name:           "Stream Agent",
		ProviderID:     testADKProviderID,
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat/stream", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"hello"}`))
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
	streamID := rec.Header().Get("X-ADK-Stream-ID")
	if streamID == "" {
		t.Fatal("stream id header is empty")
	}
	replayReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/adk/streams/"+streamID+"?after=1", nil)
	replayRec := httptest.NewRecorder()
	server.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("stream replay status = %d body=%q", replayRec.Code, replayRec.Body.String())
	}
	replayed := parseADKStreamFrames(t, replayRec.Body.String())
	if len(replayed) == 0 {
		t.Fatal("stream replay returned no events")
	}
	for _, event := range replayed {
		if event.Sequence <= 1 || !event.Replay || event.StreamID != streamID {
			t.Fatalf("replayed event = %+v, want sequence > 1 with replay metadata", event)
		}
	}
	runReplayReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/adk/runs/"+finalEvent.Response.Run.ID+"/stream?after=1", nil)
	runReplayRec := httptest.NewRecorder()
	server.ServeHTTP(runReplayRec, runReplayReq)
	if runReplayRec.Code != http.StatusOK {
		t.Fatalf("run stream replay status = %d body=%q", runReplayRec.Code, runReplayRec.Body.String())
	}

	detachedCtx, cancelDetached := context.WithCancel(context.Background())
	detachedReq := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost,
		"/api/v1/adk/chat/stream",
		strings.NewReader(`{"agentId":"`+agent.ID+`","message":"detached execution"}`),
	).WithContext(detachedCtx)
	detachedReq.Header.Set("Content-Type", "application/json")
	cancelDetached()
	detachedRec := httptest.NewRecorder()
	server.ServeHTTP(detachedRec, detachedReq)

	deadline := time.Now().Add(2 * time.Second)
	for {
		runs, listErr := server.adkRuntime.Store().ListRuns(t.Context())
		if listErr != nil {
			t.Fatalf("ListRuns after detached stream: %v", listErr)
		}
		completed := false
		for _, run := range runs {
			if run.UserMessage == "detached execution" && run.Status == jfadk.RunStatusCompleted {
				completed = true
				break
			}
		}
		if completed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("detached stream did not complete after request cancellation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestADKChatReturnsCompletedEnvelopeWithVisibleToolFailure(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("disk full")
	})

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "chat-failed-run-agent",
		Name:           "Failed Run Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeLessApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"@strategy.save_draft 保存失败草稿"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat status = %d body=%q", rec.Code, rec.Body.String())
	}
	var envelope struct {
		OK   bool               `json:"ok"`
		Data jfadk.ChatResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode chat envelope: %v body=%q", err, rec.Body.String())
	}
	if !envelope.OK {
		t.Fatalf("chat envelope = %+v, want ok=true", envelope)
	}
	if envelope.Data.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("run status = %q, want %q", envelope.Data.Run.Status, jfadk.RunStatusCompleted)
	}
	if envelope.Data.Run.FailureReason != "" {
		t.Fatalf("failureReason = %q, want empty", envelope.Data.Run.FailureReason)
	}
	if envelope.Data.Run.ErrorCode != "" {
		t.Fatalf("errorCode = %q, want empty", envelope.Data.Run.ErrorCode)
	}
	if !envelope.Data.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", envelope.Data.Run.Degraded)
	}
	if len(envelope.Data.Run.ToolCalls) != 1 || envelope.Data.Run.ToolCalls[0].Error == nil || !strings.Contains(*envelope.Data.Run.ToolCalls[0].Error, "disk full") {
		t.Fatalf("toolCalls = %+v, want failed tool error containing 'disk full'", envelope.Data.Run.ToolCalls)
	}
}

func TestADKChatStreamReturnsFinalEventForCompletedRunWithToolFailure(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("disk full")
	})

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "stream-failed-run-agent",
		Name:           "Stream Failed Run Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeLessApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat/stream", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"@strategy.save_draft 保存失败草稿"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d body=%q", rec.Code, rec.Body.String())
	}
	frames := parseADKStreamFrames(t, rec.Body.String())
	if len(frames) == 0 {
		t.Fatalf("expected SSE frames, raw=%q", rec.Body.String())
	}
	last := frames[len(frames)-1]
	if last.Type != "final" || last.Response == nil {
		t.Fatalf("last event = %+v, want final response", last)
	}
	if last.Response.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("run status = %q, want completed", last.Response.Run.Status)
	}
	if last.Response.Run.FailureReason != "" {
		t.Fatalf("failureReason = %q, want empty", last.Response.Run.FailureReason)
	}
	if !last.Response.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", last.Response.Run.Degraded)
	}
	if len(last.Response.Run.ToolCalls) != 1 || last.Response.Run.ToolCalls[0].Error == nil || !strings.Contains(*last.Response.Run.ToolCalls[0].Error, "disk full") {
		t.Fatalf("toolCalls = %+v, want failed tool error containing 'disk full'", last.Response.Run.ToolCalls)
	}
	for _, frame := range frames {
		if frame.Type == "error" {
			t.Fatalf("frames = %+v, want failed run to end with final instead of error", frames)
		}
	}
}

func TestADKChatStreamRecoversCompletedRunAsFinalEventWhenFinalMessageAppendFails(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	failingService := &failingAppendSessionService{
		base:     adksession.InMemoryService(),
		failText: "问题摘要：attach final failure",
	}
	if err := server.adkRuntime.CloseSessionServices(); err != nil {
		t.Fatalf("CloseSessionServices: %v", err)
	}
	server.adkRuntime = jfadk.NewRuntimeWithSessionService(server.adkRuntime.Store(), server.adkRuntime.Tools(), failingService)
	server.assistantSvc = asst.NewService(server.adkRuntime)
	server.router = server.buildRouter()
	t.Cleanup(func() {
		jftradeErr1 := server.adkRuntime.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("disk full")
	})

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
		ID:             "stream-recover-failed-run-agent",
		Name:           "Stream Recover Failed Run Agent",
		ProviderID:     testADKProviderID,
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: jfadk.PermissionModeLessApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/adk/chat/stream", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"@strategy.save_draft attach final failure"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat stream status = %d body=%q", rec.Code, rec.Body.String())
	}
	frames := parseADKStreamFrames(t, rec.Body.String())
	if len(frames) == 0 {
		t.Fatalf("expected SSE frames, raw=%q", rec.Body.String())
	}
	last := frames[len(frames)-1]
	if last.Type != "final" || last.Response == nil {
		t.Fatalf("last event = %+v, want recovered final response", last)
	}
	if last.Response.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("run status = %q, want completed", last.Response.Run.Status)
	}
	if last.Response.Run.FailureReason != "" {
		t.Fatalf("failureReason = %q, want empty", last.Response.Run.FailureReason)
	}
	if !last.Response.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", last.Response.Run.Degraded)
	}
	for _, frame := range frames {
		if frame.Type == "error" {
			t.Fatalf("frames = %+v, want recovered terminal run to emit final without error", frames)
		}
	}
}

func TestADKChatStreamReturnsErrorEventForInvalidPayload(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat/stream", "application/json", strings.NewReader(`{"message":`))
	if err != nil {
		t.Fatalf("POST invalid chat stream: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/providers", "application/json", strings.NewReader(`{"displayName":`))
	if err != nil {
		t.Fatalf("POST invalid provider payload: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

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

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/providers", "application/json", strings.NewReader(`{
		"displayName":"Slow Provider",
		"baseUrl":"https://api.openai.com/v1",
		"model":"gpt-4o-mini",
		"requestTimeoutMs":250000,
		"enabled":true
	}`))
	if err != nil {
		t.Fatalf("POST provider: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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
			resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/agents", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("POST agent payload: %v", err)
			}
			defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

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

	installResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/skills", "application/json", strings.NewReader(`{"url":"not-a-valid-url"}`))
	if err != nil {
		t.Fatalf("POST invalid skill install: %v", err)
	}
	defer func() { jftradeCheckTestError(t, installResp.Body.Close()) }()
	status, code, message := decodeAPIErrorEnvelope(t, installResp)
	if status != http.StatusBadRequest || code != "ADK_SKILL_INSTALL_FAILED" || strings.TrimSpace(message) == "" {
		t.Fatalf("skill install error = %d/%s/%q, want 400/ADK_SKILL_INSTALL_FAILED/non-empty", status, code, message)
	}

	deleteReq, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/adk/skills/jftrade-market", nil)
	if err != nil {
		t.Fatalf("NewRequest delete builtin skill: %v", err)
	}
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("DELETE builtin skill: %v", err)
	}
	defer func() { jftradeCheckTestError(t, deleteResp.Body.Close()) }()
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
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/agents", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST create agent: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
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

	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/sessions", "application/json", strings.NewReader(`{"agentId":"missing-agent","title":"x"}`))
	if err != nil {
		t.Fatalf("POST invalid session create: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
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

	missingResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/sessions/session-missing")
	if err != nil {
		t.Fatalf("GET missing session detail: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
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

	renameReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/adk/sessions/"+session.ID, strings.NewReader(`{"title":`))
	if err != nil {
		t.Fatalf("NewRequest invalid rename: %v", err)
	}
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("PUT invalid rename: %v", err)
	}
	defer func() { jftradeCheckTestError(t, renameResp.Body.Close()) }()
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

	cancelMissingResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/runs/run-missing/cancel", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing cancel run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, cancelMissingResp.Body.Close()) }()
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

	getMissingResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/adk/runs/run-missing")
	if err != nil {
		t.Fatalf("GET missing run: %v", err)
	}
	defer func() { jftradeCheckTestError(t, getMissingResp.Body.Close()) }()
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
	server.adkRuntime.Tools().Register(jfadk.ToolDescriptor{
		Name:               "approval.required",
		Permission:         "write_strategy",
		AllowedModes:       []string{jfadk.PermissionModeApproval},
		RequiresApprovalIn: []string{jfadk.PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
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

	missingResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/approval-missing/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST missing approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, missingResp.Body.Close()) }()
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
		ProviderID:     testADKProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: jfadk.PermissionModeApproval,
		Status:         jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	chatResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/chat", "application/json", strings.NewReader(`{"agentId":"`+agent.ID+`","message":"<execute-tool name=\"approval.required\" />"}`))
	if err != nil {
		t.Fatalf("POST chat for approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, chatResp.Body.Close()) }()
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

	firstResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST first approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, firstResp.Body.Close()) }()
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("first approval status = %d", firstResp.StatusCode)
	}

	secondResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/adk/approvals/"+approvalID+"/approve", "application/json", nil)
	if err != nil {
		t.Fatalf("POST second approval: %v", err)
	}
	defer func() { jftradeCheckTestError(t, secondResp.Body.Close()) }()
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

type failingAppendSessionService struct {
	base     adksession.Service
	failText string
}

func (s *failingAppendSessionService) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	return s.base.Create(ctx, req)
}

func (s *failingAppendSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	return s.base.Get(ctx, req)
}

func (s *failingAppendSessionService) List(ctx context.Context, req *adksession.ListRequest) (*adksession.ListResponse, error) {
	return s.base.List(ctx, req)
}

func (s *failingAppendSessionService) Delete(ctx context.Context, req *adksession.DeleteRequest) error {
	return s.base.Delete(ctx, req)
}

func (s *failingAppendSessionService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	if strings.Contains(sessionEventText(event), s.failText) {
		return errors.New("simulated append failure")
	}
	return s.base.AppendEvent(ctx, session, event)
}

func sessionEventText(event *adksession.Event) string {
	if event == nil {
		return ""
	}
	var parts []string
	if event.Content != nil {
		for _, part := range event.Content.Parts {
			if part != nil && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	if event.Content != nil {
		for _, part := range event.Content.Parts {
			if part != nil && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}
