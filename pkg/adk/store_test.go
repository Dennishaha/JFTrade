package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adksessiondb "google.golang.org/adk/session/database"
	"google.golang.org/genai"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sessionService, err := adksessiondb.NewSessionService(OpenSQLiteDialector(filepath.Join(dir, "adk-session.db")))
	if err != nil {
		t.Fatalf("NewSessionService: %v", err)
	}
	if err := MigrateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return NewRuntimeWithSessionService(store, NewToolRegistry(), sessionService)
}

func newRuntimeWithRegistry(t *testing.T, store *Store, registry *ToolRegistry) *Runtime {
	t.Helper()
	sessionService, err := adksessiondb.NewSessionService(OpenSQLiteDialector(filepath.Join(filepath.Dir(store.SkillsPath()), "adk-session.db")))
	if err != nil {
		t.Fatalf("NewSessionService: %v", err)
	}
	if err := MigrateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return NewRuntimeWithSessionService(store, registry, sessionService)
}

func TestProviderSecretIsNotEchoed(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	provider, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:          "openai",
		DisplayName: "OpenAI",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		APIKey:      "sk-test-secret",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if !provider.HasAPIKey {
		t.Fatalf("provider.HasAPIKey = false, want true")
	}
	raw, ok, err := runtime.Store().ProviderAPIKey("openai")
	if err != nil {
		t.Fatalf("ProviderAPIKey: %v", err)
	}
	if !ok || raw != "sk-test-secret" {
		t.Fatalf("stored secret = %q/%v, want test secret", raw, ok)
	}
	providers, err := runtime.Store().ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("providers len = %d, want 1", len(providers))
	}
	encoded := providers[0]
	if encoded.HasAPIKey != true {
		t.Fatalf("listed provider HasAPIKey = false")
	}
	if encoded.DefaultHeaders != nil {
		t.Fatalf("unexpected headers: %#v", encoded.DefaultHeaders)
	}
}

func TestApprovalModeCreatesPendingApprovalForWriteTool(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := NewToolRegistry()
	executed := false
	registry.Register(ToolDescriptor{
		Name:         "strategy.save_draft",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeSandboxAuto, PermissionModeHighAuto},
	}, func(context.Context, map[string]any) (any, error) {
		executed = true
		return map[string]any{"saved": true}, nil
	})
	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@strategy.save_draft 保存策略"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Run.Status != RunStatusPending {
		t.Fatalf("run status = %q, want %q", response.Run.Status, RunStatusPending)
	}
	if len(response.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(response.PendingApprovals))
	}
	if executed {
		t.Fatalf("write tool executed before approval")
	}
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	if !executed {
		t.Fatalf("write tool was not executed after approval")
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("resolution run = %+v, want completed", resolution.Run)
	}
	if resolution.Message == nil || !strings.Contains(resolution.Message.Content, "已完成本地 ADK 分析") {
		t.Fatalf("resolution message = %+v, want regenerated final reply", resolution.Message)
	}
	if resolution.Run.UserMessage != "@strategy.save_draft 保存策略" {
		t.Fatalf("run user message = %q, want original request", resolution.Run.UserMessage)
	}
	if len(resolution.Run.ToolSummaries) != 1 || !strings.Contains(resolution.Run.ToolSummaries[0], "strategy.save_draft") {
		t.Fatalf("tool summaries = %+v, want saved draft summary", resolution.Run.ToolSummaries)
	}
}

func TestApprovalDenialCreatesAssistantSummary(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	registry := NewToolRegistry()
	executed := false
	registry.Register(ToolDescriptor{
		Name:         "strategy.save_draft",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeSandboxAuto, PermissionModeHighAuto},
	}, func(context.Context, map[string]any) (any, error) {
		executed = true
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@strategy.save_draft 保存策略"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, false)
	if err != nil {
		t.Fatalf("ResolveApproval deny: %v", err)
	}
	if executed {
		t.Fatalf("write tool executed after denial")
	}
	if resolution.Approval.Status != ApprovalStatusDenied {
		t.Fatalf("approval status = %q, want denied", resolution.Approval.Status)
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusDenied {
		t.Fatalf("run = %+v, want denied", resolution.Run)
	}
	if resolution.Message == nil || !strings.Contains(resolution.Message.Content, "已拒绝") {
		t.Fatalf("resolution message = %+v, want denial summary", resolution.Message)
	}
}

func TestApprovalDenialRecordsResumedAndDeniedAuditEvents(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:         "strategy.save_draft",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent-audit-deny",
		Name:           "Agent",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@strategy.save_draft 保存策略"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, false)
	if err != nil {
		t.Fatalf("ResolveApproval deny: %v", err)
	}
	if resolution.Run == nil {
		t.Fatal("expected run in approval resolution")
	}

	events, err := runtime.Store().ListAuditEvents(ctx)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	var resumedEvent *AuditEvent
	var deniedEvent *AuditEvent
	for i := range events {
		event := &events[i]
		if event.SubjectID != resolution.Run.ID {
			continue
		}
		switch event.Kind {
		case "run.resumed":
			resumedEvent = event
		case "run.denied":
			deniedEvent = event
		}
	}
	if resumedEvent == nil {
		t.Fatalf("expected run.resumed audit event for run %s", resolution.Run.ID)
	}
	if resumedEvent.Metadata["resumeState"] != "approval_denied" {
		t.Fatalf("run.resumed resumeState = %#v, want approval_denied", resumedEvent.Metadata["resumeState"])
	}
	if deniedEvent == nil {
		t.Fatalf("expected run.denied audit event for run %s", resolution.Run.ID)
	}
	if deniedEvent.Metadata["resumeState"] != "approval_denied" {
		t.Fatalf("run.denied resumeState = %#v, want approval_denied", deniedEvent.Metadata["resumeState"])
	}
}

func TestApprovedPendingRunMarksFailureWhenToolExecutionFails(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:         "strategy.save_draft",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, fmt.Errorf("disk full")
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent-failing-approval",
		Name:           "Agent",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@strategy.save_draft 保存策略"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval approve: %v", err)
	}
	if resolution.Run == nil {
		t.Fatal("expected run in approval resolution")
	}
	if resolution.Run.Status != RunStatusFailed {
		t.Fatalf("run status = %q, want %q", resolution.Run.Status, RunStatusFailed)
	}
	if resolution.Run.ErrorCode != "TOOL_EXECUTION_FAILED" {
		t.Fatalf("run error code = %q, want TOOL_EXECUTION_FAILED", resolution.Run.ErrorCode)
	}
	if resolution.Run.FailureReason != "disk full" {
		t.Fatalf("run failure reason = %q, want disk full", resolution.Run.FailureReason)
	}
	if resolution.Message == nil || strings.TrimSpace(resolution.Message.Content) == "" {
		t.Fatalf("resolution message = %+v, want assistant summary", resolution.Message)
	}

	events, err := runtime.Store().ListAuditEvents(ctx)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	var failedEvent *AuditEvent
	for i := range events {
		event := &events[i]
		if event.SubjectID == resolution.Run.ID && event.Kind == "run.failed" {
			failedEvent = event
			break
		}
	}
	if failedEvent == nil {
		t.Fatalf("expected run.failed audit event for run %s", resolution.Run.ID)
	}
	if failedEvent.Metadata["failureReason"] != "disk full" {
		t.Fatalf("run.failed failureReason = %#v, want disk full", failedEvent.Metadata["failureReason"])
	}
}

func TestGoogleADKExecutionRunHonorsContextDeadline(t *testing.T) {
	execution := &googleADKExecution{
		runBlocking: func(context.Context, *genai.Content) error {
			select {}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := execution.run(ctx, genai.NewContentFromText("查看系统状态", genai.RoleUser))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("run returned after %s, want timeout guard to return promptly", elapsed)
	}
}

func TestReconcileExpiredRunsMarksHungRunTimedOut(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	startedAt := time.Now().UTC().Add(-MaxRunTimeout - time.Minute).Format(time.RFC3339Nano)
	run := Run{
		ID:        "run-hung",
		SessionID: "session-1",
		AgentID:   "agent-1",
		Status:    RunStatusRunning,
		Message:   "running",
		ToolCalls: []ToolCall{{
			ID:        "tool-1",
			RunID:     "run-hung",
			ToolName:  "account.orders",
			Status:    "RUNNING",
			CreatedAt: startedAt,
			StartedAt: startedAt,
			UpdatedAt: startedAt,
		}},
		CreatedAt: startedAt,
		StartedAt: startedAt,
		UpdatedAt: startedAt,
		Usage:     &RunUsage{},
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	runtime.ReconcileExpiredRuns(ctx)

	reloaded, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ok {
		t.Fatalf("Run %q not found", run.ID)
	}
	if reloaded.Status != RunStatusTimedOut {
		t.Fatalf("run status = %q, want %q", reloaded.Status, RunStatusTimedOut)
	}
	if reloaded.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
	if !strings.Contains(reloaded.FailureReason, MaxRunTimeout.String()) {
		t.Fatalf("failure reason = %q, want timeout detail", reloaded.FailureReason)
	}
	if len(reloaded.ToolCalls) != 1 || reloaded.ToolCalls[0].Status != "FAILED" {
		t.Fatalf("tool calls = %+v, want timed out running tool to be marked failed", reloaded.ToolCalls)
	}
	if reloaded.ToolCalls[0].CompletedAt == nil {
		t.Fatal("expected tool call completed_at to be set")
	}
}

func TestDeleteProviderFailsWhenReferencedByAgent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:          "openai",
		DisplayName: "OpenAI",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		ProviderID:     "openai",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	err := runtime.Store().DeleteProvider(ctx, "openai")
	if err == nil || !strings.Contains(err.Error(), "used by agent") {
		t.Fatalf("DeleteProvider error = %v, want used by agent", err)
	}
}

func TestListProvidersSortsNewestFirstAndDeleteMissingIsIdempotent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "provider-older", DisplayName: "Older", APIKey: "sk-older", Enabled: true,
	})
	time.Sleep(10 * time.Millisecond)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "provider-newer", DisplayName: "Newer", APIKey: "sk-newer", Enabled: true,
	})

	providers, err := runtime.Store().ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) < 2 {
		t.Fatalf("providers len = %d, want at least 2", len(providers))
	}
	if providers[0].ID != "provider-newer" || providers[1].ID != "provider-older" {
		t.Fatalf("provider order = [%s %s], want [provider-newer provider-older]", providers[0].ID, providers[1].ID)
	}
	if !providers[0].HasAPIKey || !providers[1].HasAPIKey {
		t.Fatalf("providers api key visibility = %+v, want both true", providers[:2])
	}

	if err := runtime.Store().DeleteProvider(ctx, ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteProvider blank error = %v, want os.ErrNotExist", err)
	}
	if err := runtime.Store().DeleteProvider(ctx, "provider-missing"); err != nil {
		t.Fatalf("DeleteProvider missing = %v, want nil", err)
	}
}

func TestDeleteSessionRemovesApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "agent", "test")
	run := mustSaveRun(t, runtime, Run{ID: "run-test", SessionID: session.ID, AgentID: "agent", Status: RunStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()})
	approval := Approval{ID: "approval-test", RunID: run.ID, AgentID: "agent", ToolName: "strategy.save_draft", Status: ApprovalStatusPending}
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	message, err := runtime.Store().AddMessage(ctx, session.ID, "assistant", "done", "")
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	if err := runtime.Store().DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, err := runtime.Store().Approval(ctx, approval.ID); err != nil || ok {
		t.Fatalf("approval still exists: ok=%v err=%v", ok, err)
	}
	if _, ok, err := runtime.Store().Run(ctx, run.ID); err != nil || ok {
		t.Fatalf("run still exists: ok=%v err=%v", ok, err)
	}
	messages := mustMessages(t, runtime, session.ID)
	if len(messages) != 0 {
		t.Fatalf("messages = %+v, want empty after deleting session", messages)
	}
	if _, ok, err := runtime.Store().Session(ctx, session.ID); err != nil || ok {
		t.Fatalf("session still exists: ok=%v err=%v", ok, err)
	}
	_ = message
}

func TestListSessionsPageFiltersQueryAndPaginates(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	older := mustCreateSession(t, runtime, "agent-a", "Alpha Review")
	time.Sleep(10 * time.Millisecond)
	newer := mustCreateSession(t, runtime, "agent-a", "alpha Deep Dive")
	time.Sleep(10 * time.Millisecond)
	mustCreateSession(t, runtime, "agent-b", "Alpha Other Agent")
	time.Sleep(10 * time.Millisecond)
	mustCreateSession(t, runtime, "agent-a", "Gamma Notes")

	page, total, err := runtime.Store().ListSessionsPage(ctx, "agent-a", "ALPHA", 1, 0)
	if err != nil {
		t.Fatalf("ListSessionsPage: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(page) != 1 || page[0].ID != newer.ID {
		t.Fatalf("first page = %+v, want newest alpha session", page)
	}

	nextPage, total, err := runtime.Store().ListSessionsPage(ctx, "agent-a", "alpha", 1, 1)
	if err != nil {
		t.Fatalf("ListSessionsPage next: %v", err)
	}
	if total != 2 {
		t.Fatalf("next total = %d, want 2", total)
	}
	if len(nextPage) != 1 || nextPage[0].ID != older.ID {
		t.Fatalf("next page = %+v, want older alpha session", nextPage)
	}
}

func TestDeleteSessionMissingAndBlankAreNotFound(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if err := runtime.Store().DeleteSession(ctx, ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteSession blank error = %v, want os.ErrNotExist", err)
	}
	if err := runtime.Store().DeleteSession(ctx, "session-missing"); err != nil {
		t.Fatalf("DeleteSession missing = %v, want nil for idempotent delete", err)
	}
}

func TestListApprovalsPageFiltersAndSortsNewestFirst(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	approvals := []Approval{
		{
			ID: "approval-older", RunID: "run-a", AgentID: "agent-a", ToolName: "strategy.save_draft",
			Status: ApprovalStatusPending, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
		},
		{
			ID: "approval-newer", RunID: "run-b", AgentID: "agent-a", ToolName: "strategy.optimize",
			Status: ApprovalStatusPending, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-02T00:00:00Z",
		},
		{
			ID: "approval-other-agent", RunID: "run-c", AgentID: "agent-b", ToolName: "strategy.save_draft",
			Status: ApprovalStatusPending, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-03T00:00:00Z",
		},
		{
			ID: "approval-other-status", RunID: "run-d", AgentID: "agent-a", ToolName: "strategy.save_draft",
			Status: ApprovalStatusApproved, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-04T00:00:00Z",
		},
	}
	for _, approval := range approvals {
		if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval(%s): %v", approval.ID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	page, total, err := runtime.Store().ListApprovalsPage(ctx, ApprovalStatusPending, "agent-a", 10, 0)
	if err != nil {
		t.Fatalf("ListApprovalsPage: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(page) != 2 {
		t.Fatalf("page len = %d, want 2", len(page))
	}
	if page[0].ID != "approval-newer" || page[1].ID != "approval-older" {
		t.Fatalf("page order = [%s %s], want [approval-newer approval-older]", page[0].ID, page[1].ID)
	}
}

func TestListOptimizationTasksSortsByUpdatedAtDesc(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if _, err := runtime.Store().SaveOptimizationTask(ctx, OptimizationTask{
		ID:        "opt-older",
		Objective: "older",
		CreatedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveOptimizationTask first: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := runtime.Store().SaveOptimizationTask(ctx, OptimizationTask{
		ID:        "opt-newer",
		Objective: "newer",
		CreatedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveOptimizationTask second: %v", err)
	}

	tasks, err := runtime.Store().ListOptimizationTasks(ctx)
	if err != nil {
		t.Fatalf("ListOptimizationTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2", len(tasks))
	}
	if tasks[0].ID != "opt-newer" || tasks[1].ID != "opt-older" {
		t.Fatalf("tasks order = [%s %s], want [opt-newer opt-older]", tasks[0].ID, tasks[1].ID)
	}
}

func TestRecentOpenAIMessagesKeepsLatestConversation(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
		{Role: "user", Content: "third"},
	}
	history := recentOpenAIMessages(messages, 2, 100)
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Role != "assistant" || history[0].Content != "second" || history[1].Content != "third" {
		t.Fatalf("history = %+v, want latest assistant/user pair", history)
	}
}

func TestOpenAIToolsFromDescriptorsIncludesSchemaAndRisk(t *testing.T) {
	tools := openAIToolsFromDescriptors([]ToolDescriptor{{
		Name:          "market.snapshot",
		DisplayName:   "Snapshot",
		Description:   "read snapshot",
		Permission:    "read_internal",
		OutputSummary: "snapshot output",
		RiskLevel:     "low",
	}})
	if len(tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(tools))
	}
	if tools[0].Function.Name != "market-snapshot" {
		t.Fatalf("tool name = %q", tools[0].Function.Name)
	}
	if tools[0].Function.Parameters["type"] != "object" {
		t.Fatalf("tool parameters = %#v, want object schema", tools[0].Function.Parameters)
	}
	if !strings.Contains(tools[0].Function.Description, "Risk: low") {
		t.Fatalf("tool description = %q, want risk annotation", tools[0].Function.Description)
	}
}

func TestExecuteToolTagInvokesCanonicalToolWithParameters(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := NewToolRegistry()
	var received map[string]any
	registry.Register(ToolDescriptor{
		Name:        "portfolio.summary",
		DisplayName: "组合摘要",
		Description: "test portfolio summary",
		Category:    "portfolio",
		Permission:  "read_internal",
	}, func(_ context.Context, input map[string]any) (any, error) {
		received = input
		return map[string]any{"ok": true}, nil
	})
	runtime := NewRuntime(store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		Tools:          []string{"portfolio.summary"},
		PermissionMode: PermissionModeSandboxAuto,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="jftrade portfolio summary" parameters='{"showDetails": true, "showPositions": true}' />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.Run.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(response.Run.ToolCalls))
	}
	call := response.Run.ToolCalls[0]
	if call.ToolName != "portfolio.summary" || call.Status != "SUCCEEDED" {
		t.Fatalf("tool call = %+v, want successful portfolio.summary", call)
	}
	if received["showDetails"] != true || received["showPositions"] != true {
		t.Fatalf("received input = %#v, want parsed boolean parameters", received)
	}
}

func TestRejectUnsafeHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "10.0.0.1", "169.254.169.254", "::1"} {
		t.Run(host, func(t *testing.T) {
			if err := rejectUnsafeHost(context.Background(), host); err == nil {
				t.Fatalf("rejectUnsafeHost(%q) = nil, want error", host)
			}
		})
	}
}

func TestInternalSkillCannotBeUninstalled(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skill, ok, err := runtime.Skills().Get(ctx, "jftrade-market")
	if err != nil {
		t.Fatalf("Get builtin skill: %v", err)
	}
	if !ok {
		t.Fatal("builtin skill jftrade-market not found")
	}

	err = runtime.Skills().Uninstall(ctx, skill.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot be uninstalled") {
		t.Fatalf("Uninstall internal skill error = %v, want cannot be uninstalled", err)
	}
	stored, ok, err := runtime.Skills().Get(ctx, skill.ID)
	if err != nil {
		t.Fatalf("Get builtin skill: %v", err)
	}
	if !ok || stored.Source != "builtin" {
		t.Fatalf("internal skill was changed or removed: ok=%v skill=%+v", ok, stored)
	}
}

func TestExternalSkillUninstallRemovesInstallDir(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	installDir := filepath.Join(runtime.Store().SkillsPath(), "external-skill")
	installPath := filepath.Join(installDir, "SKILL.md")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(installPath, []byte("---\nname: external-skill\ndescription: external test skill\nmetadata:\n  source: https://example.com/SKILL.md\n---\nAlways cite external sources.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	skill, ok, err := runtime.Skills().Get(ctx, "external-skill")
	if err != nil {
		t.Fatalf("Get external skill: %v", err)
	}
	if !ok {
		t.Fatal("external skill not discovered")
	}

	if err := runtime.Skills().Uninstall(ctx, skill.ID); err != nil {
		t.Fatalf("Uninstall external skill: %v", err)
	}
	if _, ok, err := runtime.Skills().Get(ctx, skill.ID); err != nil || ok {
		t.Fatalf("external skill still exists: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("install dir stat error = %v, want not exist", err)
	}
}

func TestPreparedAgentLoadsOnlyEnabledBoundSkillsAndTools(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skillDir := filepath.Join(runtime.Store().SkillsPath(), "research")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: research\ndescription: external research skill\nmetadata:\n  source: test\n  version: 2\nallowed-tools: [http.fetch]\n---\nAlways cite external sources.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	agent := Agent{
		ID: "agent", Name: "Agent", Instruction: "Base instruction.",
		Tools: []string{"http.fetch", "system.status"}, Skills: []string{"research"},
	}
	prepared, err := runtime.prepareAgent(ctx, agent)
	if err != nil {
		t.Fatalf("prepareAgent: %v", err)
	}
	if prepared.Instruction != agent.Instruction {
		t.Fatalf("prepared instruction = %q, want original instruction", prepared.Instruction)
	}
	if len(prepared.Tools) != len(agent.Tools) {
		t.Fatalf("prepared tools = %#v, want original tools %#v", prepared.Tools, agent.Tools)
	}
}

func TestSkillRegistryReportsMetadataAndAllowedTools(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skill, ok, err := runtime.Skills().Get(ctx, "jftrade-market")
	if err != nil {
		t.Fatalf("Get builtin skill: %v", err)
	}
	if !ok {
		t.Fatal("builtin skill jftrade-market not found")
	}
	if !skill.Builtin || skill.Source != "builtin" {
		t.Fatalf("skill source metadata = %+v", skill)
	}
	if skill.ValidationStatus != "VALID" || skill.ContentHash == "" {
		t.Fatalf("skill validation metadata = %+v", skill)
	}
	if len(skill.Tools) == 0 || skill.Tools[0] == "" {
		t.Fatalf("skill tools = %+v, want allowed tools", skill.Tools)
	}
}

func TestInstallSkillArchivePreservesResources(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	skillFile, err := writer.Create("research-pack/SKILL.md")
	if err != nil {
		t.Fatalf("Create SKILL.md: %v", err)
	}
	if _, err := skillFile.Write([]byte("---\nname: research-pack\ndescription: Research pack\nallowed-tools: [http.fetch]\n---\nUse bundled resources.\n")); err != nil {
		t.Fatalf("Write SKILL.md: %v", err)
	}
	resourceFile, err := writer.Create("research-pack/references/playbook.md")
	if err != nil {
		t.Fatalf("Create resource: %v", err)
	}
	if _, err := resourceFile.Write([]byte("playbook content")); err != nil {
		t.Fatalf("Write resource: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close zip writer: %v", err)
	}
	sourceURL := "https://example.com/research-pack.zip"
	skill, err := runtime.Skills().installArchive(ctx, sourceURL, archive.Bytes())
	if err != nil {
		t.Fatalf("installArchive: %v", err)
	}
	if skill.Source != sourceURL {
		t.Fatalf("skill source = %q, want %q", skill.Source, sourceURL)
	}
	resourcePath := filepath.Join(runtime.Store().SkillsPath(), "research-pack", "references", "playbook.md")
	raw, err := os.ReadFile(resourcePath)
	if err != nil {
		t.Fatalf("ReadFile resource: %v", err)
	}
	if string(raw) != "playbook content" {
		t.Fatalf("resource content = %q", string(raw))
	}
}

func TestResolveSessionRejectsDifferentAgent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session, err := runtime.Store().CreateSession(ctx, "agent-a", "test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_, err = runtime.resolveSession(ctx, session.ID, Agent{ID: "agent-b"}, "hello")
	if err == nil || !strings.Contains(err.Error(), "different agent") {
		t.Fatalf("resolveSession error = %v, want different agent", err)
	}
}

func TestDeleteAgentSoftDeletesHistoricalRecord(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent", Name: "Agent", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	stored, ok, err := runtime.Store().Agent(ctx, agent.ID)
	if err != nil || !ok {
		t.Fatalf("Agent after delete: ok=%v err=%v", ok, err)
	}
	if stored.DeletedAt == nil || stored.Status != AgentStatusDisabled {
		t.Fatalf("soft deleted agent = %+v", stored)
	}
	agents, err := runtime.Store().ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("active agents = %+v, want none", agents)
	}
}

func TestListAgentsExcludesSoftDeletedWhileListAllIncludesThem(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-older", Name: "Older Agent", Status: AgentStatusEnabled,
	})
	time.Sleep(10 * time.Millisecond)
	mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-newer", Name: "Newer Agent", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, "agent-older"); err != nil {
		t.Fatalf("DeleteAgent older: %v", err)
	}

	active, err := runtime.Store().ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(active) != 1 || active[0].ID != "agent-newer" {
		t.Fatalf("active agents = %+v, want only agent-newer", active)
	}

	all, err := runtime.Store().ListAllAgents(ctx)
	if err != nil {
		t.Fatalf("ListAllAgents: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all agents len = %d, want 2", len(all))
	}
	var deletedFound bool
	for _, agent := range all {
		if agent.ID == "agent-older" {
			deletedFound = agent.DeletedAt != nil && agent.Status == AgentStatusDisabled
		}
	}
	if !deletedFound {
		t.Fatalf("all agents = %+v, want deleted agent-older preserved historically", all)
	}
}

func TestSaveAgentRestoresDeletedAgentRecord(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent", Name: "Agent", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	restored, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent Restored", Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent restore: %v", err)
	}
	if restored.DeletedAt != nil {
		t.Fatalf("restored agent deleted_at = %v, want nil", restored.DeletedAt)
	}
	agents, err := runtime.Store().ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "Agent Restored" {
		t.Fatalf("active agents = %+v, want restored visible agent", agents)
	}
}

func TestCancelPendingRunDeniesApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	run := mustSaveRun(t, runtime, Run{
		ID: "run-cancel", SessionID: "session", AgentID: "agent", Status: RunStatusPending,
		PendingApprovals: []Approval{{ID: "approval-cancel", RunID: "run-cancel", AgentID: "agent", Status: ApprovalStatusPending}},
		CreatedAt:        nowString(), UpdatedAt: nowString(),
	})
	if err := runtime.Store().SaveApproval(ctx, run.PendingApprovals[0]); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	cancelled, err := runtime.CancelRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if cancelled.Status != RunStatusCancelled || cancelled.CancelledAt == nil {
		t.Fatalf("cancelled run = %+v", cancelled)
	}
	approval, ok, err := runtime.Store().Approval(ctx, "approval-cancel")
	if err != nil || !ok || approval.Status != ApprovalStatusDenied {
		t.Fatalf("cancelled approval = %+v ok=%v err=%v", approval, ok, err)
	}
}

func TestCancelRunMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	_, err := runtime.CancelRun(ctx, "run-missing")
	if err == nil || err.Error() != "run not found" {
		t.Fatalf("CancelRun missing error = %v, want run not found", err)
	}
}

func TestResolveApprovalMissingReturnsIdempotentEmptyResult(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	resolution, err := runtime.ResolveApproval(ctx, "approval-missing", true)
	if err != nil {
		t.Fatalf("ResolveApproval missing: %v", err)
	}
	if resolution.Run != nil || resolution.Message != nil {
		t.Fatalf("missing approval resolution = %+v, want no run/message", resolution)
	}
	if resolution.Approval.ID != "" || resolution.Approval.Status != "" {
		t.Fatalf("missing approval = %+v, want zero-value approval", resolution.Approval)
	}
}

func TestStoreResolvePendingApprovalMissingAndIdempotent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	approval, changed, err := runtime.Store().ResolvePendingApproval(ctx, "approval-missing", ApprovalStatusApproved)
	if err != nil {
		t.Fatalf("ResolvePendingApproval missing: %v", err)
	}
	if changed {
		t.Fatal("missing approval unexpectedly reported changed=true")
	}
	if approval.ID != "" || approval.Status != "" {
		t.Fatalf("missing approval = %+v, want zero value", approval)
	}

	stored := Approval{
		ID: "approval-approved", RunID: "run-1", AgentID: "agent-1", ToolName: "strategy.save_draft",
		Status: ApprovalStatusApproved, CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	if err := runtime.Store().SaveApproval(ctx, stored); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	resolved, changed, err := runtime.Store().ResolvePendingApproval(ctx, stored.ID, ApprovalStatusDenied)
	if err != nil {
		t.Fatalf("ResolvePendingApproval approved: %v", err)
	}
	if changed {
		t.Fatal("non-pending approval unexpectedly changed")
	}
	if resolved.Status != ApprovalStatusApproved {
		t.Fatalf("resolved status = %q, want approved", resolved.Status)
	}
}

func TestListRunsPageFiltersAndSortsNewestFirst(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	runs := []Run{
		{
			ID: "run-older", SessionID: "session-a", AgentID: "agent-a", Status: RunStatusFailed,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
		},
		{
			ID: "run-newer", SessionID: "session-a", AgentID: "agent-a", Status: RunStatusFailed,
			CreatedAt: "2024-01-02T00:00:00Z", UpdatedAt: "2024-01-02T00:00:00Z",
		},
		{
			ID: "run-other-session", SessionID: "session-b", AgentID: "agent-a", Status: RunStatusFailed,
			CreatedAt: "2024-01-03T00:00:00Z", UpdatedAt: "2024-01-03T00:00:00Z",
		},
		{
			ID: "run-other-status", SessionID: "session-a", AgentID: "agent-a", Status: RunStatusCompleted,
			CreatedAt: "2024-01-04T00:00:00Z", UpdatedAt: "2024-01-04T00:00:00Z",
		},
	}
	for _, run := range runs {
		mustSaveRun(t, runtime, run)
	}

	page, total, err := runtime.Store().ListRunsPage(ctx, RunStatusFailed, "agent-a", "session-a", 10, 0)
	if err != nil {
		t.Fatalf("ListRunsPage: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(page) != 2 {
		t.Fatalf("page len = %d, want 2", len(page))
	}
	if page[0].ID != "run-newer" || page[1].ID != "run-older" {
		t.Fatalf("page order = [%s %s], want [run-newer run-older]", page[0].ID, page[1].ID)
	}
}

func TestDuplicateApprovalResolutionDoesNotExecuteTwice(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executions := 0
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions++
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", Tools: []string{"strategy.save_draft"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@strategy.save_draft save"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	id := response.PendingApprovals[0].ID
	if _, err := runtime.ResolveApproval(ctx, id, true); err != nil {
		t.Fatalf("first ResolveApproval: %v", err)
	}
	if _, err := runtime.ResolveApproval(ctx, id, true); err != nil {
		t.Fatalf("second ResolveApproval: %v", err)
	}
	if executions != 1 {
		t.Fatalf("executions = %d, want 1", executions)
	}
}

func TestPendingApprovalResumesThroughGoogleADKAfterRuntimeRestart(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executions := 0
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions++
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", Tools: []string{"strategy.save_draft"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID, Message: "@strategy.save_draft save",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	approval := response.PendingApprovals[0]
	if approval.FunctionCallID == "" || approval.ConfirmationCallID == "" {
		t.Fatalf("approval lacks GO-ADK confirmation identifiers: %+v", approval)
	}

	restarted := newRuntimeWithRegistry(t, runtime.Store(), registry)
	restartedRun, ok, err := restarted.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("restarted run lookup err=%v ok=%v", err, ok)
	}
	if restartedRun.Status != RunStatusPending {
		t.Fatalf("restarted run status = %q, want %q", restartedRun.Status, RunStatusPending)
	}
	resolution, err := restarted.ResolveApproval(ctx, approval.ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval after restart: %v", err)
	}
	if executions != 1 {
		t.Fatalf("executions = %d, want 1", executions)
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("resolution run = %+v, want completed", resolution.Run)
	}
	if resolution.Run.ResumeState != "adk_confirmation_resolved" {
		t.Fatalf("resume state = %q, want GO-ADK resume", resolution.Run.ResumeState)
	}
}

func TestUnrecoverablePendingApprovalRunIsMarkedOrphanedOnRestart(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	run := Run{
		ID:        "run-orphaned-pending",
		SessionID: "session-1",
		AgentID:   "agent-1",
		Status:    RunStatusPending,
		Message:   "waiting approval",
		CreatedAt: nowString(),
		StartedAt: nowString(),
		UpdatedAt: nowString(),
		PendingApprovals: []Approval{{
			ID:        "approval-1",
			RunID:     "run-orphaned-pending",
			AgentID:   "agent-1",
			ToolName:  "strategy.save_draft",
			Status:    ApprovalStatusPending,
			Reason:    "needs approval",
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
		}},
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	restarted := newRuntimeWithRegistry(t, runtime.Store(), NewToolRegistry())
	stored, ok, err := restarted.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusFailed {
		t.Fatalf("stored status = %q, want %q", stored.Status, RunStatusFailed)
	}
	if stored.ErrorCode != "RUN_ORPHANED" {
		t.Fatalf("stored error code = %q, want RUN_ORPHANED", stored.ErrorCode)
	}
	if stored.ResumeState != "approval_context_missing" {
		t.Fatalf("stored resume state = %q, want approval_context_missing", stored.ResumeState)
	}
}

func TestMultipleApprovalsExecuteOnlyAfterAllApproved(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executions := 0
	registry := NewToolRegistry()
	for _, name := range []string{"strategy.save_draft", "strategy.optimize"} {
		registry.Register(ToolDescriptor{
			Name: name, Permission: "write_strategy",
			AllowedModes: []string{PermissionModeApproval},
		}, func(context.Context, map[string]any) (any, error) {
			executions++
			return map[string]any{"ok": true}, nil
		})
	}
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", Tools: []string{"strategy.save_draft", "strategy.optimize"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="strategy.save_draft" /><execute-tool name="strategy.optimize" />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.PendingApprovals) != 2 {
		t.Fatalf("pending approvals = %d, want 2", len(response.PendingApprovals))
	}
	first, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("first approval: %v", err)
	}
	if executions != 0 {
		t.Fatalf("executions after first approval = %d, want 0", executions)
	}
	if first.Run == nil || first.Run.Status != RunStatusPending {
		t.Fatalf("first approval run = %+v, want pending", first.Run)
	}
	second, err := runtime.ResolveApproval(ctx, response.PendingApprovals[1].ID, true)
	if err != nil {
		t.Fatalf("second approval: %v", err)
	}
	if executions != 2 {
		t.Fatalf("executions after all approvals = %d, want 2", executions)
	}
	if second.Run == nil || second.Run.Status != RunStatusCompleted {
		t.Fatalf("second approval run = %+v, want completed", second.Run)
	}
}
