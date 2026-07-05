package servercore

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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
