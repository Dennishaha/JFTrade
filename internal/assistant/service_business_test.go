package assistant

import (
	"context"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestServiceSaveAgentValidationScenarios(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	runtime.Tools().Register(jfadk.ToolDescriptor{Name: "market.read"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})

	disabledProvider, err := runtime.Store().SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-disabled", DisplayName: "Disabled Provider", Enabled: false,
	})
	if err != nil {
		t.Fatalf("SaveProvider disabled: %v", err)
	}
	noKeyProvider, err := runtime.Store().SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-no-key", DisplayName: "No Key Provider", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider no-key: %v", err)
	}
	enabledProvider, err := runtime.Store().SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-ok", DisplayName: "OK Provider", APIKey: "sk-test", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider ok: %v", err)
	}

	cases := []struct {
		name    string
		req     jfadk.AgentWriteRequest
		wantErr string
	}{
		{
			name:    "invalid status",
			req:     jfadk.AgentWriteRequest{ID: "agent-invalid-status", Name: "Invalid Status", Status: "BROKEN"},
			wantErr: "invalid agent status",
		},
		{
			name:    "invalid work mode",
			req:     jfadk.AgentWriteRequest{ID: "agent-invalid-mode", Name: "Invalid Mode", Status: jfadk.AgentStatusEnabled, WorkMode: "parallel"},
			wantErr: "invalid agent work mode",
		},
		{
			name:    "invalid loop iterations",
			req:     jfadk.AgentWriteRequest{ID: "agent-invalid-loop", Name: "Invalid Loop", Status: jfadk.AgentStatusEnabled, LoopMaxIterations: jfadk.MaxLoopIterations + 1},
			wantErr: "loop max iterations must be between 1 and",
		},
		{
			name:    "missing provider",
			req:     jfadk.AgentWriteRequest{ID: "agent-missing-provider", Name: "Missing Provider", Status: jfadk.AgentStatusEnabled, ProviderID: "provider-missing"},
			wantErr: "provider not found",
		},
		{
			name:    "disabled provider",
			req:     jfadk.AgentWriteRequest{ID: "agent-disabled-provider", Name: "Disabled Provider", Status: jfadk.AgentStatusEnabled, ProviderID: disabledProvider.ID},
			wantErr: "provider is disabled",
		},
		{
			name:    "provider missing api key",
			req:     jfadk.AgentWriteRequest{ID: "agent-no-key", Name: "No Key", Status: jfadk.AgentStatusEnabled, ProviderID: noKeyProvider.ID},
			wantErr: "provider API key is not configured",
		},
		{
			name:    "unknown tool",
			req:     jfadk.AgentWriteRequest{ID: "agent-unknown-tool", Name: "Unknown Tool", Status: jfadk.AgentStatusEnabled, ProviderID: enabledProvider.ID, Tools: []string{"does.not.exist"}},
			wantErr: "unknown ADK tool",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.SaveAgent(ctx, tc.req)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("SaveAgent() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}

	disabledAgent, err := service.SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-disabled-ok", Name: "Disabled OK", Status: jfadk.AgentStatusDisabled, ProviderID: disabledProvider.ID,
	})
	if err != nil {
		t.Fatalf("SaveAgent disabled agent: %v", err)
	}
	if disabledAgent.Status != jfadk.AgentStatusDisabled {
		t.Fatalf("disabled agent status = %q, want %q", disabledAgent.Status, jfadk.AgentStatusDisabled)
	}

	enabledAgent, err := service.SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID:                "agent-enabled-ok",
		Name:              "Enabled OK",
		Status:            jfadk.AgentStatusEnabled,
		ProviderID:        enabledProvider.ID,
		Tools:             []string{"market.read"},
		WorkMode:          jfadk.WorkModeLoop,
		LoopMaxIterations: 1,
	})
	if err != nil {
		t.Fatalf("SaveAgent enabled agent: %v", err)
	}
	if enabledAgent.ProviderID != enabledProvider.ID {
		t.Fatalf("enabled agent provider = %q, want %q", enabledAgent.ProviderID, enabledProvider.ID)
	}
}

func TestServicePreviewSessionScenarios(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	defaultAgent, err := runtime.Store().DefaultAgent(ctx)
	if err != nil {
		t.Fatalf("DefaultAgent: %v", err)
	}
	preview, err := service.PreviewSession(ctx, jfadk.ChatRequest{Message: "default agent title"})
	if err != nil {
		t.Fatalf("PreviewSession default: %v", err)
	}
	if preview.AgentID != jfadk.DefaultBuiltinAgentID || preview.AgentID != defaultAgent.ID {
		t.Fatalf("PreviewSession default agent = %q, want %q", preview.AgentID, jfadk.DefaultBuiltinAgentID)
	}

	explicitAgent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-explicit", Name: "Explicit Agent", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent explicit: %v", err)
	}
	existingSession, err := runtime.Store().CreateSession(ctx, defaultAgent.ID, "Existing Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	preview, err = service.PreviewSession(ctx, jfadk.ChatRequest{SessionID: existingSession.ID, Message: "ignored"})
	if err != nil {
		t.Fatalf("PreviewSession existing: %v", err)
	}
	if preview.ID != existingSession.ID {
		t.Fatalf("PreviewSession existing id = %q, want %q", preview.ID, existingSession.ID)
	}

	const longMessage = "这是一个明显超过二十八个字符的会话标题，用来验证预览时的自动截断行为。"
	preview, err = service.PreviewSession(ctx, jfadk.ChatRequest{AgentID: explicitAgent.ID, Message: longMessage})
	if err != nil {
		t.Fatalf("PreviewSession explicit: %v", err)
	}
	if preview.AgentID != explicitAgent.ID {
		t.Fatalf("PreviewSession explicit agent = %q, want %q", preview.AgentID, explicitAgent.ID)
	}
	if got := len([]rune(preview.Title)); got != 28 {
		t.Fatalf("PreviewSession title rune len = %d, want 28; title=%q", got, preview.Title)
	}

	if _, err := service.PreviewSession(ctx, jfadk.ChatRequest{AgentID: "agent-missing", Message: "x"}); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("PreviewSession missing agent err = %v, want agent not found", err)
	}
}

func TestServiceRecoverTerminalChatResponseFromProjection(t *testing.T) {
	runtime, service, sessionService := newAssistantServiceHarness(t)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-recover", Name: "Recover Agent", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Recover Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	appendAssistantSessionEvent(t, sessionService, agent.ID, session.ID, newUserSessionEvent("run-recover", "请恢复终态回复", time.Unix(10, 0)))
	finalID := "assistant-final-recover"
	appendAssistantSessionEvent(t, sessionService, agent.ID, session.ID, newAssistantSessionEvent("run-recover", finalID, "最终答复", "中间推理", time.Unix(11, 0)))

	running := jfadk.Run{
		ID: "run-running", SessionID: session.ID, AgentID: agent.ID, Status: jfadk.RunStatusRunning,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, running); err != nil {
		t.Fatalf("SaveRun running: %v", err)
	}
	response, err := service.RecoverTerminalChatResponse(ctx, running.ID)
	if err != nil {
		t.Fatalf("RecoverTerminalChatResponse running: %v", err)
	}
	if response != nil {
		t.Fatalf("RecoverTerminalChatResponse for running run = %+v, want nil", response)
	}

	completed := jfadk.Run{
		ID: "run-recover", SessionID: session.ID, AgentID: agent.ID, Status: jfadk.RunStatusCompleted,
		FinalMessageID: finalID,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, completed); err != nil {
		t.Fatalf("SaveRun completed: %v", err)
	}
	response, err = service.RecoverTerminalChatResponse(ctx, completed.ID)
	if err != nil {
		t.Fatalf("RecoverTerminalChatResponse completed: %v", err)
	}
	if response == nil {
		t.Fatal("RecoverTerminalChatResponse completed = nil")
	}
	if response.Reply != "最终答复" || response.ReasoningContent != "中间推理" {
		t.Fatalf("recovered response = %+v, want final reply and reasoning", response)
	}
	if response.Run.ID != completed.ID || response.Session.ID != session.ID {
		t.Fatalf("recovered identifiers = run %q session %q", response.Run.ID, response.Session.ID)
	}
	if len(response.Timeline) == 0 {
		t.Fatalf("recovered timeline = %+v, want projected entries", response.Timeline)
	}
}

func TestServiceCRUDQueriesAndSnapshots(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t,
		WithRuntimeSettings(func() any { return map[string]any{"streamIdleTimeoutMs": 420000} }),
	)
	ctx := t.Context()

	runtime.Tools().Register(jfadk.ToolDescriptor{Name: "market.read", DisplayName: "Market Read"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})

	provider, err := service.SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-service", DisplayName: "Service Provider", APIKey: "sk-service", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	snapshotAny, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	snapshot := asMap(t, snapshotAny)
	if snapshot["runtimeSettings"] == nil {
		t.Fatalf("snapshot runtimeSettings = %#v, want configured settings", snapshot)
	}
	toolsAny, err := service.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	tools, ok := toolsAny.([]jfadk.ToolDescriptor)
	if !ok || len(tools) == 0 {
		t.Fatalf("Tools() = %#v, want registered descriptors", toolsAny)
	}
	providers, err := service.ListProviders(ctx)
	if err != nil || len(providers) == 0 {
		t.Fatalf("ListProviders() providers=%v err=%v", providers, err)
	}

	agent, err := service.SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-service", Name: "Service Agent", Status: jfadk.AgentStatusEnabled,
		ProviderID: provider.ID, Tools: []string{"market.read"}, WorkMode: jfadk.WorkModeLoop,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	agents, err := service.ListAgents(ctx, AgentQuery{Status: jfadk.AgentStatusEnabled})
	if err != nil || len(agents) == 0 {
		t.Fatalf("ListAgents() agents=%v err=%v", agents, err)
	}

	task, err := service.SaveTask(ctx, jfadk.TaskWriteRequest{
		Title: "准备研究", Status: "IN_PROGRESS", AgentID: agent.ID, Message: "整理盘前观察项",
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	taskPage, err := service.ListTasks(ctx, TaskQuery{Status: "IN_PROGRESS", AgentID: agent.ID, Limit: 20, Offset: 0})
	if err != nil || len(taskPage.Items) != 1 {
		t.Fatalf("ListTasks() page=%+v err=%v", taskPage, err)
	}
	fetchedTask, err := service.GetTask(ctx, task.ID)
	if err != nil || fetchedTask.ID != task.ID {
		t.Fatalf("GetTask() task=%+v err=%v", fetchedTask, err)
	}
	done := "DONE"
	summary := "已完成"
	updatedTask, err := service.UpdateTask(ctx, task.ID, jfadk.TaskPatchRequest{
		Status: &done, ResultSummary: &summary,
	})
	if err != nil || updatedTask.Status != done || updatedTask.ResultSummary != summary {
		t.Fatalf("UpdateTask() task=%+v err=%v", updatedTask, err)
	}

	memory, err := service.SaveMemory(ctx, jfadk.MemoryWriteRequest{
		AgentID: agent.ID, Key: "watch-note", Value: "留意量价配合", Scope: "agent",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	memories, err := service.ListMemory(ctx, MemoryQuery{AgentID: agent.ID, Key: "watch-note"})
	if err != nil || len(memories) != 1 {
		t.Fatalf("ListMemory() memories=%+v err=%v", memories, err)
	}

	session, err := service.CreateSession(ctx, CreateSessionRequest{AgentID: agent.ID, Title: "Service Session"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessionPage, err := service.ListSessions(ctx, SessionQuery{AgentID: agent.ID, Limit: 20, Offset: 0})
	if err != nil || len(sessionPage.Items) != 1 {
		t.Fatalf("ListSessions() page=%+v err=%v", sessionPage, err)
	}
	fetchedSession, err := service.GetSession(ctx, session.ID)
	if err != nil || fetchedSession.ID != session.ID {
		t.Fatalf("GetSession() session=%+v err=%v", fetchedSession, err)
	}
	renamedSession, err := service.RenameSession(ctx, session.ID, "Renamed Session")
	if err != nil || renamedSession.Title != "Renamed Session" {
		t.Fatalf("RenameSession() session=%+v err=%v", renamedSession, err)
	}
	workModeOverride := jfadk.WorkModeLoop
	composerState, err := service.UpdateSessionComposerState(ctx, session.ID, jfadk.SessionComposerStatePatch{
		WorkModeOverride: &workModeOverride,
	})
	if err != nil || composerState.WorkModeOverride != workModeOverride {
		t.Fatalf("UpdateSessionComposerState() state=%+v err=%v", composerState, err)
	}
	sessionDetail, err := service.GetSessionDetail(ctx, session.ID)
	if err != nil || sessionDetail.Session.ID != session.ID || len(sessionDetail.Timeline) != 0 {
		t.Fatalf("GetSessionDetail() detail=%+v err=%v", sessionDetail, err)
	}
	contextSnapshot, err := service.GetSessionContext(ctx, session.ID)
	if err != nil || contextSnapshot.SessionID != session.ID {
		t.Fatalf("GetSessionContext() snapshot=%+v err=%v", contextSnapshot, err)
	}

	run := jfadk.Run{
		ID:        "run-service-loop",
		SessionID: session.ID,
		AgentID:   agent.ID,
		WorkMode:  jfadk.WorkModeLoop,
		Status:    jfadk.RunStatusRunning,
		Objective: "跟踪开盘机会",
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	runPage, err := service.ListRuns(ctx, RunQuery{Status: jfadk.RunStatusRunning, AgentID: agent.ID, SessionID: session.ID, Limit: 20, Offset: 0})
	if err != nil || len(runPage.Items) != 1 {
		t.Fatalf("ListRuns() page=%+v err=%v", runPage, err)
	}
	fetchedRun, err := service.GetRun(ctx, run.ID)
	if err != nil || fetchedRun.ID != run.ID {
		t.Fatalf("GetRun() run=%+v err=%v", fetchedRun, err)
	}
	updatedRun, err := service.UpdateRunObjective(ctx, run.ID, "聚焦盘前流动性")
	if err != nil || updatedRun.Objective != "聚焦盘前流动性" {
		t.Fatalf("UpdateRunObjective() run=%+v err=%v", updatedRun, err)
	}

	approval := jfadk.Approval{
		ID: "approval-service", RunID: run.ID, AgentID: agent.ID, ToolName: "market.read", Status: jfadk.ApprovalStatusPending,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	approvalPage, err := service.ListApprovals(ctx, ApprovalQuery{Status: jfadk.ApprovalStatusPending, AgentID: agent.ID, Limit: 20, Offset: 0})
	if err != nil || len(approvalPage.Items) != 1 {
		t.Fatalf("ListApprovals() page=%+v err=%v", approvalPage, err)
	}

	auditEvents, err := service.GetAudit(ctx, AuditQuery{Kind: "task.saved", SubjectID: task.ID})
	if err != nil || len(auditEvents) == 0 {
		t.Fatalf("GetAudit() events=%+v err=%v", auditEvents, err)
	}

	optCreatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := runtime.Store().SaveOptimizationTask(ctx, jfadk.OptimizationTask{
		ID: "optimization-service", Status: "queued", Objective: "maximize return",
		CreatedAt: optCreatedAt, UpdatedAt: optCreatedAt,
	}); err != nil {
		t.Fatalf("SaveOptimizationTask service: %v", err)
	}
	optTask, err := service.GetOptimizationTask(ctx, "optimization-service")
	if err != nil {
		t.Fatalf("GetOptimizationTask() err = %v", err)
	}
	if asMap(t, optTask)["status"] != "queued" {
		t.Fatalf("GetOptimizationTask() task=%#v, want queued status", optTask)
	}

	if err := service.DeleteMemory(ctx, memory.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if err := service.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if err := service.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if err := service.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	extraProvider, err := service.SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "provider-unused", DisplayName: "Unused Provider", Enabled: false,
	})
	if err != nil {
		t.Fatalf("SaveProvider unused: %v", err)
	}
	if err := service.DeleteProvider(ctx, extraProvider.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
}

func TestServiceRunLifecycleAndApprovalWrappers(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-run-wrapper", Name: "Run Wrapper", Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeLoop,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Run Wrapper Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	cancelApproval := jfadk.Approval{
		ID: "approval-cancel", RunID: "run-cancel", AgentID: agent.ID, ToolName: "strategy.write",
		Status: jfadk.ApprovalStatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveApproval(ctx, cancelApproval); err != nil {
		t.Fatalf("SaveApproval cancel: %v", err)
	}
	cancelRun := jfadk.Run{
		ID:        "run-cancel",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    jfadk.RunStatusPending,
		ToolCalls: []jfadk.ToolCall{{
			ID: "tool-cancel", RunID: "run-cancel", ToolName: "strategy.write", Status: "PENDING_APPROVAL", RequiresUser: true,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}},
		PendingApprovals: []jfadk.Approval{cancelApproval},
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, cancelRun); err != nil {
		t.Fatalf("SaveRun cancel: %v", err)
	}
	cancelled, err := service.CancelRun(ctx, cancelRun.ID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if cancelled.Status != jfadk.RunStatusCancelled || cancelled.ErrorCode != "RUN_CANCELLED" {
		t.Fatalf("cancelled run = %+v", cancelled)
	}
	storedApproval, ok, err := runtime.Store().Approval(ctx, cancelApproval.ID)
	if err != nil || !ok || storedApproval.Status != jfadk.ApprovalStatusDenied {
		t.Fatalf("stored approval after cancel = %+v ok=%v err=%v", storedApproval, ok, err)
	}

	pauseRun := jfadk.Run{
		ID:             "run-pause",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         jfadk.RunStatusRunning,
		WorkMode:       jfadk.WorkModeLoop,
		WorkflowStatus: "running",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, pauseRun); err != nil {
		t.Fatalf("SaveRun pause: %v", err)
	}
	paused, err := service.PauseGoalRun(ctx, pauseRun.ID)
	if err != nil {
		t.Fatalf("PauseGoalRun: %v", err)
	}
	if paused.ResumeState != "user_pause_requested" || paused.PauseRequestedAt == nil {
		t.Fatalf("paused run = %+v", paused)
	}

	pausedAt := time.Now().UTC().Format(time.RFC3339Nano)
	resumeRun := jfadk.Run{
		ID:             "run-resume",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         jfadk.RunStatusPaused,
		WorkMode:       jfadk.WorkModeLoop,
		WorkflowStatus: "paused",
		PausedAt:       &pausedAt,
		PausedReason:   "user",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, resumeRun); err != nil {
		t.Fatalf("SaveRun resume: %v", err)
	}
	resumed, err := service.ResumeGoalRun(ctx, resumeRun.ID)
	if err != nil {
		t.Fatalf("ResumeGoalRun: %v", err)
	}
	if resumed.Status != jfadk.RunStatusRunning || resumed.ResumeState != "user_resuming" || resumed.PausedAt != nil {
		t.Fatalf("resumed run = %+v", resumed)
	}

	syncApproval := jfadk.Approval{
		ID: "approval-sync", RunID: "run-sync-resolve", AgentID: agent.ID, ToolName: "market.read",
		Status: jfadk.ApprovalStatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveApproval(ctx, syncApproval); err != nil {
		t.Fatalf("SaveApproval sync: %v", err)
	}
	syncRun := jfadk.Run{
		ID: "run-sync-resolve", SessionID: session.ID, AgentID: agent.ID, Status: jfadk.RunStatusCompleted,
		PendingApprovals: []jfadk.Approval{syncApproval},
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, syncRun); err != nil {
		t.Fatalf("SaveRun sync resolve: %v", err)
	}
	resolution, err := service.ResolveApproval(ctx, syncApproval.ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	if resolution.Approval.Status != jfadk.ApprovalStatusApproved {
		t.Fatalf("sync resolution = %+v", resolution)
	}

	asyncApproval := jfadk.Approval{
		ID: "approval-async", RunID: "run-async-resolve", AgentID: agent.ID, ToolName: "strategy.write",
		Status: jfadk.ApprovalStatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveApproval(ctx, asyncApproval); err != nil {
		t.Fatalf("SaveApproval async: %v", err)
	}
	asyncRun := jfadk.Run{
		ID:        "run-async-resolve",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    jfadk.RunStatusPending,
		ToolCalls: []jfadk.ToolCall{{
			ID: "tool-async", RunID: "run-async-resolve", ToolName: "strategy.write", Status: "PENDING_APPROVAL", RequiresUser: true,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}},
		PendingApprovals: []jfadk.Approval{asyncApproval},
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, asyncRun); err != nil {
		t.Fatalf("SaveRun async resolve: %v", err)
	}
	asyncResolution, err := service.ResolveApprovalAsync(ctx, asyncApproval.ID, true)
	if err != nil {
		t.Fatalf("ResolveApprovalAsync: %v", err)
	}
	if asyncResolution.Approval.Status != jfadk.ApprovalStatusApproved || asyncResolution.Run == nil || asyncResolution.Run.Status != jfadk.RunStatusRunning {
		t.Fatalf("async resolution = %+v", asyncResolution)
	}

	service.ReconcileExpiredRuns(ctx)
	service.ReconcileResolvedApprovals(ctx)
}
