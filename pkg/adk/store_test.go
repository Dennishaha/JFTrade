package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessionService, err := NewSQLiteSessionService(filepath.Join(dir, "adk-session.db"))
	if err != nil {
		t.Fatalf("NewSessionService: %v", err)
	}
	t.Cleanup(func() { _ = CloseSessionService(sessionService) })
	t.Cleanup(func() { _ = store.Close() })
	if err := MigrateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	runtime := NewRuntimeWithSessionService(store, NewToolRegistry(), sessionService)
	ensureTestProvider(t, runtime)
	return runtime
}

func TestStoreMigrationNormalizesHiddenAgentWorkflowDefaults(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "adk.db")
	secretsPath := filepath.Join(dir, "secrets", "adk.json")
	skillsPath := filepath.Join(dir, "skills")
	store, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	agent := mustSaveAgent(t, NewRuntime(store, NewToolRegistry()), AgentWriteRequest{
		ID: "legacy-sequential-agent", Name: "Legacy Sequential", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	if agent.WorkMode != WorkModeLoop {
		t.Fatalf("initial agent work mode = %q, want loop", agent.WorkMode)
	}
	_ = store.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := db.Exec(`UPDATE `+tableAgents+` SET payload_json = json_set(payload_json, '$.workMode', 'sequential') WHERE id = ?`, agent.ID); err != nil {
		_ = db.Close()
		t.Fatalf("update raw agent payload: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM adk_schema_migrations WHERE version = 30`); err != nil {
		_ = db.Close()
		t.Fatalf("delete migration marker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	migrated, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore migrate: %v", err)
	}
	defer migrated.Close()
	var rawMode string
	if err := migrated.db.Get(&rawMode, `SELECT json_extract(payload_json, '$.workMode') FROM `+tableAgents+` WHERE id = ?`, agent.ID); err != nil {
		t.Fatalf("read raw agent mode: %v", err)
	}
	if rawMode != WorkModeChat {
		t.Fatalf("raw migrated work mode = %q, want %q", rawMode, WorkModeChat)
	}
}

func newRuntimeWithRegistry(t *testing.T, store *Store, registry *ToolRegistry) *Runtime {
	t.Helper()
	sessionService, err := NewSQLiteSessionService(filepath.Join(filepath.Dir(store.SkillsPath()), "adk-session.db"))
	if err != nil {
		t.Fatalf("NewSessionService: %v", err)
	}
	t.Cleanup(func() { _ = CloseSessionService(sessionService) })
	if err := MigrateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	runtime := NewRuntimeWithSessionService(store, registry, sessionService)
	ensureTestProvider(t, runtime)
	return runtime
}

func TestNewStoreDropsLegacyMessageTables(t *testing.T) {
	runtime := newTestRuntime(t)

	var count int
	if err := runtime.Store().db.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('adk_messages', 'adk_transcript_entries')`); err != nil {
		t.Fatalf("legacy table lookup: %v", err)
	}
	if count != 0 {
		t.Fatalf("legacy table count = %d, want 0", count)
	}
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
	var encoded Provider
	found := false
	for _, item := range providers {
		if item.ID == provider.ID {
			encoded = item
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("provider %q not found in %+v", provider.ID, providers)
	}
	if encoded.HasAPIKey != true {
		t.Fatalf("listed provider HasAPIKey = false")
	}
	if encoded.DefaultHeaders != nil {
		t.Fatalf("unexpected headers: %#v", encoded.DefaultHeaders)
	}
	if encoded.RequestTimeoutMs != int(DefaultProviderRequestTimeout/time.Millisecond) {
		t.Fatalf("requestTimeoutMs = %d, want %d", encoded.RequestTimeoutMs, int(DefaultProviderRequestTimeout/time.Millisecond))
	}
}

func TestProviderRequestTimeoutDefaultsAndClamp(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	provider, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:          "openai-clamped",
		DisplayName: "OpenAI",
		BaseURL:     "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveProvider default: %v", err)
	}
	if provider.RequestTimeoutMs != int(DefaultProviderRequestTimeout/time.Millisecond) {
		t.Fatalf("default requestTimeoutMs = %d", provider.RequestTimeoutMs)
	}

	provider, err = runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:               provider.ID,
		DisplayName:      provider.DisplayName,
		BaseURL:          provider.BaseURL,
		Model:            provider.Model,
		RequestTimeoutMs: 1_000,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("SaveProvider clamped: %v", err)
	}
	if provider.RequestTimeoutMs != 15_000 {
		t.Fatalf("clamped requestTimeoutMs = %d, want 15000", provider.RequestTimeoutMs)
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
		ProviderID:     testProviderID,
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
	if resolution.Message == nil || !strings.Contains(resolution.Message.Content, "已完成 ADK 分析") {
		t.Fatalf("resolution message = %+v, want regenerated final reply", resolution.Message)
	}
	if resolution.Run.UserMessage != "@strategy.save_draft 保存策略" {
		t.Fatalf("run user message = %q, want original request", resolution.Run.UserMessage)
	}
	if len(resolution.Run.ToolSummaries) != 1 || !strings.Contains(resolution.Run.ToolSummaries[0], "strategy.save_draft") {
		t.Fatalf("tool summaries = %+v, want saved draft summary", resolution.Run.ToolSummaries)
	}
}

func TestIdempotentApprovalRecoversPendingRunWithStaleEmbeddedApproval(t *testing.T) {
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
		ID:             "agent-stale-approval",
		Name:           "Agent",
		ProviderID:     testProviderID,
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
	if len(response.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(response.PendingApprovals))
	}
	approvalID := response.PendingApprovals[0].ID
	approval, changed, err := runtime.Store().ResolvePendingApproval(ctx, approvalID, ApprovalStatusApproved)
	if err != nil {
		t.Fatalf("ResolvePendingApproval: %v", err)
	}
	if !changed || approval.Status != ApprovalStatusApproved {
		t.Fatalf("direct approval = %+v changed=%v, want approved change", approval, changed)
	}
	if executed {
		t.Fatalf("write tool executed before runtime recovery")
	}

	resolution, err := runtime.ResolveApproval(ctx, approvalID, true)
	if err != nil {
		t.Fatalf("ResolveApproval retry: %v", err)
	}
	if !executed {
		t.Fatalf("write tool was not executed by idempotent recovery")
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("resolution run = %+v, want completed", resolution.Run)
	}
	if len(resolution.Run.PendingApprovals) != 1 || resolution.Run.PendingApprovals[0].Status != ApprovalStatusApproved {
		t.Fatalf("run approvals = %+v, want embedded approved approval", resolution.Run.PendingApprovals)
	}
	persistedRun, ok, err := runtime.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("persisted run ok=%v err=%v", ok, err)
	}
	if persistedRun.Status != RunStatusCompleted {
		t.Fatalf("persisted run status = %q, want completed", persistedRun.Status)
	}
}

func TestReconcileResolvedApprovalsRecoversPendingRun(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	registry := NewToolRegistry()
	var executed atomic.Bool
	registry.Register(ToolDescriptor{
		Name:         "strategy.save_draft",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeSandboxAuto, PermissionModeHighAuto},
	}, func(context.Context, map[string]any) (any, error) {
		executed.Store(true)
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent-reconcile-approval",
		Name:           "Agent",
		ProviderID:     testProviderID,
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
	approvalID := response.PendingApprovals[0].ID
	if _, changed, err := runtime.Store().ResolvePendingApproval(ctx, approvalID, ApprovalStatusApproved); err != nil || !changed {
		t.Fatalf("ResolvePendingApproval changed=%v err=%v", changed, err)
	}

	runtime.ReconcileResolvedApprovals(ctx)

	var persistedRun Run
	var ok bool
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		persistedRun, ok, err = runtime.Store().Run(ctx, response.Run.ID)
		if err != nil || !ok {
			t.Fatalf("persisted run ok=%v err=%v", ok, err)
		}
		if executed.Load() && persistedRun.Status == RunStatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !executed.Load() {
		t.Fatalf("write tool was not executed by resolved approval reconciliation")
	}
	if persistedRun.Status != RunStatusCompleted {
		t.Fatalf("persisted run status = %q, want completed", persistedRun.Status)
	}
	if len(persistedRun.PendingApprovals) != 1 || persistedRun.PendingApprovals[0].Status != ApprovalStatusApproved {
		t.Fatalf("persisted approvals = %+v, want approved", persistedRun.PendingApprovals)
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
		ProviderID:     testProviderID,
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
		ProviderID:     testProviderID,
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
		ProviderID:     testProviderID,
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
	if resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("run status = %q, want %q", resolution.Run.Status, RunStatusCompleted)
	}
	if resolution.Run.ErrorCode != "" {
		t.Fatalf("run error code = %q, want empty", resolution.Run.ErrorCode)
	}
	if resolution.Run.FailureReason != "" {
		t.Fatalf("run failure reason = %q, want empty", resolution.Run.FailureReason)
	}
	if !resolution.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", resolution.Run.Degraded)
	}
	if resolution.Message == nil || strings.TrimSpace(resolution.Message.Content) == "" {
		t.Fatalf("resolution message = %+v, want assistant summary", resolution.Message)
	}

	events, err := runtime.Store().ListAuditEvents(ctx)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	var completedEvent *AuditEvent
	for i := range events {
		event := &events[i]
		if event.SubjectID == resolution.Run.ID && event.Kind == "run.completed" {
			completedEvent = event
			break
		}
	}
	if completedEvent == nil {
		t.Fatalf("expected run.completed audit event for run %s", resolution.Run.ID)
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

func TestStartRunUsesConfiguredRuntimeTimeout(t *testing.T) {
	runtime := newTestRuntime(t)
	runtime.SetRuntimeLimitsProvider(func() RuntimeLimits {
		return RuntimeLimits{RunTimeout: 12 * time.Minute}
	})

	run, _, finish, err := runtime.startRun(context.Background(), "session-1", Agent{ID: "agent-1"}, "hello")
	if err != nil {
		t.Fatalf("startRun: %v", err)
	}
	defer finish()

	if run.MaxDurationMs != int64((12*time.Minute)/time.Millisecond) {
		t.Fatalf("run.MaxDurationMs = %d, want %d", run.MaxDurationMs, int64((12*time.Minute)/time.Millisecond))
	}
}

func TestReconcileExpiredRunsMarksHungRunTimedOut(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	startedAt := time.Now().UTC().Add(-DefaultRunTimeout - time.Minute).Format(time.RFC3339Nano)
	run := Run{
		ID:            "run-hung",
		SessionID:     "session-1",
		AgentID:       "agent-1",
		MaxDurationMs: DefaultRunTimeout.Milliseconds(),
		Status:        RunStatusRunning,
		Message:       "running",
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
	if !strings.Contains(reloaded.FailureReason, DefaultRunTimeout.String()) {
		t.Fatalf("failure reason = %q, want timeout detail", reloaded.FailureReason)
	}
	if len(reloaded.ToolCalls) != 1 || reloaded.ToolCalls[0].Status != "FAILED" {
		t.Fatalf("tool calls = %+v, want timed out running tool to be marked failed", reloaded.ToolCalls)
	}
	if reloaded.ToolCalls[0].CompletedAt == nil {
		t.Fatal("expected tool call completed_at to be set")
	}
}

func TestReconcileExpiredRunsUsesRunSpecificTimeout(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	startedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	shortRun := Run{
		ID:            "run-short-timeout",
		SessionID:     "session-1",
		AgentID:       "agent-1",
		MaxDurationMs: 60_000,
		Status:        RunStatusRunning,
		Message:       "running",
		CreatedAt:     startedAt,
		StartedAt:     startedAt,
		UpdatedAt:     startedAt,
		Usage:         &RunUsage{},
	}
	longRun := Run{
		ID:            "run-long-timeout",
		SessionID:     "session-1",
		AgentID:       "agent-1",
		MaxDurationMs: 300_000,
		Status:        RunStatusRunning,
		Message:       "running",
		CreatedAt:     startedAt,
		StartedAt:     startedAt,
		UpdatedAt:     startedAt,
		Usage:         &RunUsage{},
	}
	if err := runtime.Store().SaveRun(ctx, shortRun); err != nil {
		t.Fatalf("SaveRun shortRun: %v", err)
	}
	if err := runtime.Store().SaveRun(ctx, longRun); err != nil {
		t.Fatalf("SaveRun longRun: %v", err)
	}

	runtime.ReconcileExpiredRuns(ctx)

	reloadedShort, ok, err := runtime.Store().Run(ctx, shortRun.ID)
	if err != nil || !ok {
		t.Fatalf("Run shortRun: %v ok=%v", err, ok)
	}
	if reloadedShort.Status != RunStatusTimedOut {
		t.Fatalf("short run status = %q, want timed out", reloadedShort.Status)
	}

	reloadedLong, ok, err := runtime.Store().Run(ctx, longRun.ID)
	if err != nil || !ok {
		t.Fatalf("Run longRun: %v ok=%v", err, ok)
	}
	if reloadedLong.Status != RunStatusRunning {
		t.Fatalf("long run status = %q, want running", reloadedLong.Status)
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
	appendADKEvent(t, runtime, "agent", session.ID, newAssistantEvent(run.ID, []*genai.Part{{Text: "done"}}, time.Unix(40, 0)))

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

func TestSessionComposerStatePersistsAndDeletesWithSession(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "agent-composer", "composer")

	state, ok, err := runtime.Store().SessionComposerState(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionComposerState empty: %v", err)
	}
	if ok || state.SessionID != session.ID || state.ChatDraft != "" {
		t.Fatalf("empty composer state = %+v ok=%v", state, ok)
	}

	saved, err := runtime.Store().SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{
		ChatDraft:            new(strings.Repeat("x", MaxMessageLength+20)),
		WorkModeOverride:     new(WorkModeLoop),
		GoalObjectiveDraft:   new("目标草稿"),
		GoalObjectiveTouched: new(true),
	})
	if err != nil {
		t.Fatalf("SaveSessionComposerState: %v", err)
	}
	if len([]rune(saved.ChatDraft)) != MaxMessageLength || saved.WorkModeOverride != WorkModeLoop || !saved.GoalObjectiveTouched {
		t.Fatalf("saved composer state = %+v", saved)
	}

	if _, err := runtime.Store().SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{WorkModeOverride: new("sequential")}); err == nil {
		t.Fatal("SaveSessionComposerState invalid mode err = nil")
	}

	if err := runtime.Store().DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	deleted, ok, err := runtime.Store().SessionComposerState(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionComposerState after delete: %v", err)
	}
	if ok || deleted.ChatDraft != "" {
		t.Fatalf("deleted composer state = %+v ok=%v, want empty", deleted, ok)
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
	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		ProviderID:     testProviderID,
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
	research, ok, err := runtime.Skills().Get(ctx, strategypinespec.ResearchBuiltinSkillName)
	if err != nil {
		t.Fatalf("Get builtin research skill: %v", err)
	}
	if !ok {
		t.Fatalf("builtin skill %s not found", strategypinespec.ResearchBuiltinSkillName)
	}
	if !research.Builtin || research.Source != "builtin" || research.ValidationStatus != "VALID" || research.ContentHash == "" {
		t.Fatalf("research skill metadata = %+v", research)
	}
	if research.Version != strategypinespec.BuiltinSkillVersion {
		t.Fatalf("research skill version = %q, want %q", research.Version, strategypinespec.BuiltinSkillVersion)
	}
	for _, toolName := range []string{
		strategypinespec.ToolName,
		"strategy.validate_pine",
		"strategy.research_backtest",
		"backtest.runs",
		"backtest.result_view",
		"workflow.wait",
		"market.snapshot",
		"market.candles",
	} {
		if !containsString(research.Tools, toolName) {
			t.Fatalf("research skill tools = %+v, want %s", research.Tools, toolName)
		}
	}
	for _, forbidden := range []string{"strategy.save_draft", "strategy.save_definition", "strategy.optimize"} {
		if containsString(research.Tools, forbidden) {
			t.Fatalf("research skill unexpectedly exposes %s: %+v", forbidden, research.Tools)
		}
	}
	publish, ok, err := runtime.Skills().Get(ctx, strategypinespec.PublishBuiltinSkillName)
	if err != nil {
		t.Fatalf("Get builtin publish skill: %v", err)
	}
	if !ok {
		t.Fatalf("builtin skill %s not found", strategypinespec.PublishBuiltinSkillName)
	}
	if !publish.Builtin || publish.Source != "builtin" || publish.ValidationStatus != "VALID" || publish.ContentHash == "" {
		t.Fatalf("publish skill metadata = %+v", publish)
	}
	for _, toolName := range []string{"strategy.validate_pine", "strategy.save_draft", "strategy.save_definition", "strategy.update_instance_mode", "strategy.optimize", "backtest.runs"} {
		if !containsString(publish.Tools, toolName) {
			t.Fatalf("publish skill tools = %+v, want %s", publish.Tools, toolName)
		}
	}
	if containsString(publish.Tools, "strategy.research_backtest") {
		t.Fatalf("publish skill unexpectedly exposes research_backtest: %+v", publish.Tools)
	}
	if _, ok, err := runtime.Skills().Get(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy strategy skill ok=%v err=%v, want absent", ok, err)
	}
	for _, item := range []struct {
		skillName string
		resource  string
	}{
		{strategypinespec.ResearchBuiltinSkillName, "references/pine-v6-spec.md"},
		{strategypinespec.ResearchBuiltinSkillName, "references/pine-v6-examples.md"},
		{strategypinespec.ResearchBuiltinSkillName, "references/pine-v6-cheatsheet.md"},
		{strategypinespec.ResearchBuiltinSkillName, "references/strategy-research-workflow.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/pine-v6-spec.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/pine-v6-examples.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/pine-v6-cheatsheet.md"},
		{strategypinespec.PublishBuiltinSkillName, "references/strategy-publish-checklist.md"},
	} {
		if _, err := os.Stat(filepath.Join(runtime.Store().SkillsPath(), item.skillName, item.resource)); err != nil {
			t.Fatalf("resource %s/%s stat: %v", item.skillName, item.resource, err)
		}
	}
}

func TestStoreBuiltinSkillsSplitStrategySkillAndRemoveLegacyRecord(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	store := runtime.Store()
	if _, ok, err := store.Skill(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy store skill ok=%v err=%v, want absent", ok, err)
	}
	for _, skillName := range []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName} {
		skill, ok, err := store.Skill(ctx, skillName)
		if err != nil || !ok {
			t.Fatalf("store skill %s ok=%v err=%v", skillName, ok, err)
		}
		if !skill.Builtin || !strings.EqualFold(skill.Source, "builtin") {
			t.Fatalf("store skill %s metadata = %+v, want builtin", skillName, skill)
		}
	}

	if _, err := store.SaveSkill(ctx, Skill{ID: strategypinespec.LegacyBuiltinSkillName, DisplayName: "Legacy Strategy", Source: "builtin", Builtin: true}); err != nil {
		t.Fatalf("SaveSkill legacy builtin: %v", err)
	}
	if err := store.ensureBuiltins(ctx); err != nil {
		t.Fatalf("ensureBuiltins after legacy builtin: %v", err)
	}
	if _, ok, err := store.Skill(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy builtin store skill ok=%v err=%v, want deleted", ok, err)
	}

	if _, err := store.SaveSkill(ctx, Skill{ID: strategypinespec.LegacyBuiltinSkillName, DisplayName: "External Strategy", Source: "filesystem", Builtin: false}); err != nil {
		t.Fatalf("SaveSkill legacy external: %v", err)
	}
	if err := store.ensureBuiltins(ctx); err != nil {
		t.Fatalf("ensureBuiltins after legacy external: %v", err)
	}
	if skill, ok, err := store.Skill(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || !ok || skill.Source != "filesystem" {
		t.Fatalf("legacy external store skill = %+v ok=%v err=%v, want preserved", skill, ok, err)
	}
}

func TestBuiltinStrategySkillRefreshesOutdatedBundle(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	skillDir := filepath.Join(runtime.Store().SkillsPath(), strategypinespec.ResearchBuiltinSkillName)
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(`---
name: jftrade-strategy-research
description: outdated research builtin
allowed-tools: [strategy.validate_pine]
metadata:
  source: builtin
  version: 1
---
Old research instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile outdated strategy skill: %v", err)
	}
	if err := os.Remove(filepath.Join(skillDir, "references", "pine-v6-spec.md")); err != nil {
		t.Fatalf("Remove spec resource: %v", err)
	}
	legacyDir := filepath.Join(runtime.Store().SkillsPath(), strategypinespec.LegacyBuiltinSkillName)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll legacyDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "SKILL.md"), []byte(`---
name: jftrade-strategy
description: legacy builtin
allowed-tools: [strategy.validate_pine]
metadata:
  source: builtin
  version: 7
---
Legacy strategy instructions.
`), 0o644); err != nil {
		t.Fatalf("WriteFile legacy strategy skill: %v", err)
	}

	if err := runtime.Skills().ensureBuiltins(); err != nil {
		t.Fatalf("ensureBuiltins: %v", err)
	}

	skill, ok, err := runtime.Skills().Get(ctx, strategypinespec.ResearchBuiltinSkillName)
	if err != nil || !ok {
		t.Fatalf("Get refreshed strategy skill ok=%v err=%v", ok, err)
	}
	if skill.Version != strategypinespec.BuiltinSkillVersion {
		t.Fatalf("refreshed strategy skill version = %q, want %q", skill.Version, strategypinespec.BuiltinSkillVersion)
	}
	raw, err := os.ReadFile(filepath.Join(skillDir, "references", "pine-v6-spec.md"))
	if err != nil {
		t.Fatalf("ReadFile restored spec: %v", err)
	}
	if !strings.Contains(string(raw), "# JFTrade Pine Script v6 规范") {
		t.Fatalf("restored spec content = %q, want DSL heading", string(raw))
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy builtin skill dir stat err = %v, want not exist", err)
	}
	if _, ok, err := runtime.Skills().Get(ctx, strategypinespec.LegacyBuiltinSkillName); err != nil || ok {
		t.Fatalf("legacy strategy skill ok=%v err=%v, want absent", ok, err)
	}
}

func TestBuiltinStrategyAgentTemplatesExposeExplicitStrategyTools(t *testing.T) {
	for _, agentID := range []string{"investment-analyst", "strategy-researcher", "risk-reviewer"} {
		template, ok := BuiltinAgentTemplate(agentID)
		if !ok {
			t.Fatalf("BuiltinAgentTemplate(%q) not found", agentID)
		}
		if containsString(template.Skills, strategypinespec.LegacyBuiltinSkillName) {
			t.Fatalf("template %q still references legacy strategy skill: %+v", agentID, template.Skills)
		}
	}
	investment, _ := BuiltinAgentTemplate("investment-analyst")
	if !containsString(investment.Skills, strategypinespec.ResearchBuiltinSkillName) || containsString(investment.Skills, strategypinespec.PublishBuiltinSkillName) {
		t.Fatalf("investment skills = %+v, want research only", investment.Skills)
	}
	for _, toolName := range strategypinespec.ResearchSkillAllowedTools() {
		if !containsString(investment.Tools, toolName) {
			t.Fatalf("investment tools = %+v, want research tool %s", investment.Tools, toolName)
		}
	}
	for _, toolName := range []string{"strategy.save_definition", "strategy.update_instance_mode"} {
		if containsString(investment.Tools, toolName) {
			t.Fatalf("investment tools unexpectedly include publish tool %s: %+v", toolName, investment.Tools)
		}
	}

	researcher, _ := BuiltinAgentTemplate("strategy-researcher")
	for _, skillName := range []string{strategypinespec.ResearchBuiltinSkillName, strategypinespec.PublishBuiltinSkillName} {
		if !containsString(researcher.Skills, skillName) {
			t.Fatalf("strategy-researcher skills = %+v, want %s", researcher.Skills, skillName)
		}
	}
	for _, toolName := range append(strategypinespec.ResearchSkillAllowedTools(), strategypinespec.PublishSkillAllowedTools()...) {
		if !containsString(researcher.Tools, toolName) {
			t.Fatalf("strategy-researcher tools = %+v, want %s", researcher.Tools, toolName)
		}
	}

	risk, _ := BuiltinAgentTemplate("risk-reviewer")
	if !containsString(risk.Skills, strategypinespec.PublishBuiltinSkillName) || containsString(risk.Skills, strategypinespec.ResearchBuiltinSkillName) {
		t.Fatalf("risk skills = %+v, want publish only", risk.Skills)
	}
	for _, toolName := range strategypinespec.PublishSkillAllowedTools() {
		if !containsString(risk.Tools, toolName) {
			t.Fatalf("risk tools = %+v, want publish tool %s", risk.Tools, toolName)
		}
	}
}

func TestBuiltinRefreshDoesNotOverrideNonBuiltinSkill(t *testing.T) {
	runtime := newTestRuntime(t)
	skillDir := filepath.Join(runtime.Store().SkillsPath(), strategypinespec.LegacyBuiltinSkillName)
	if err := os.RemoveAll(skillDir); err != nil {
		t.Fatalf("RemoveAll skillDir: %v", err)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skillDir: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	externalSkill := `---
name: jftrade-strategy
description: custom external strategy skill
allowed-tools: [strategy.definitions]
metadata:
  source: https://example.com/jftrade-strategy/SKILL.md
  version: custom
---
Use the custom external strategy instructions.
`
	if err := os.WriteFile(skillPath, []byte(externalSkill), 0o644); err != nil {
		t.Fatalf("WriteFile external strategy skill: %v", err)
	}

	if err := runtime.Skills().ensureBuiltins(); err != nil {
		t.Fatalf("ensureBuiltins: %v", err)
	}

	raw, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("ReadFile strategy skill: %v", err)
	}
	if string(raw) != externalSkill {
		t.Fatalf("external skill was overwritten:\n%s", string(raw))
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

func TestInstallSkillURLInstallsNeodataFinancialSearch(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	originalValidator := skillInstallHostValidator
	skillInstallHostValidator = func(context.Context, string) error { return nil }
	t.Cleanup(func() { skillInstallHostValidator = originalValidator })
	skillServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/skills/neodata-financial-search/SKILL.md" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(`---
name: neodata-financial-search
description: Search NeoData financial filings and earnings materials.
allowed-tools: [http.fetch]
metadata:
  version: 2026.06
---
Use NeoData search results as reference material and cite the source URL.`))
	}))
	t.Cleanup(skillServer.Close)

	skillURL := skillServer.URL + "/skills/neodata-financial-search/SKILL.md"
	skill, err := runtime.Skills().InstallURL(ctx, skillURL)
	if err != nil {
		t.Fatalf("InstallURL: %v", err)
	}
	if skill.ID != "neodata-financial-search" {
		t.Fatalf("skill ID = %q, want neodata-financial-search", skill.ID)
	}
	if skill.Source != skillURL {
		t.Fatalf("skill source = %q, want %q", skill.Source, skillURL)
	}
	if skill.Version != "2026.06" {
		t.Fatalf("skill version = %q, want 2026.06", skill.Version)
	}
	if len(skill.Tools) != 1 || skill.Tools[0] != "http.fetch" {
		t.Fatalf("skill tools = %+v, want [http.fetch]", skill.Tools)
	}
	installedPath := filepath.Join(runtime.Store().SkillsPath(), "neodata-financial-search", "SKILL.md")
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("installed skill path stat: %v", err)
	}
	stored, ok, err := runtime.Skills().Get(ctx, "neodata-financial-search")
	if err != nil {
		t.Fatalf("Get installed skill: %v", err)
	}
	if !ok {
		t.Fatal("installed skill not found in registry")
	}
	if stored.ContentHash == "" || stored.ValidationStatus != "VALID" {
		t.Fatalf("stored skill metadata = %+v", stored)
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
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"strategy.save_draft"},
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
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"strategy.save_draft"},
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

func TestApprovalResumingRunIsRecoveredAfterRuntimeRestart(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	releaseTool := make(chan struct{})
	toolStarted := make(chan struct{}, 1)
	executions := 0
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{PermissionModeApproval},
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		executions++
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
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"strategy.save_draft"},
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
	approvedApproval, changed, err := runtime.Store().ResolvePendingApproval(ctx, approval.ID, ApprovalStatusApproved)
	if err != nil || !changed {
		t.Fatalf("ResolvePendingApproval changed=%v err=%v", changed, err)
	}
	run, ok, err := runtime.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	run.Status = RunStatusRunning
	run.ResumeState = "approval_resuming"
	for index := range run.PendingApprovals {
		if run.PendingApprovals[index].ID == approvedApproval.ID {
			run.PendingApprovals[index] = approvedApproval
		}
	}
	for index := range run.ToolCalls {
		if run.ToolCalls[index].Status != "PENDING_APPROVAL" {
			continue
		}
		run.ToolCalls[index].Status = "RUNNING"
		run.ToolCalls[index].RequiresUser = false
		run.ToolCalls[index].UpdatedAt = nowString()
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	restarted := newRuntimeWithRegistry(t, runtime.Store(), registry)
	restartedRun, ok, err := restarted.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("restarted run lookup err=%v ok=%v", err, ok)
	}
	if restartedRun.Status != RunStatusRunning || restartedRun.ResumeState != "approval_resuming" {
		t.Fatalf("restarted run = %+v, want running approval_resuming", restartedRun)
	}

	restarted.ReconcileResolvedApprovals(ctx)

	select {
	case <-toolStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered approval continuation to resume")
	}

	timeline, ok, err := restarted.Store().SessionTimeline(ctx, response.Run.SessionID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	toolGroupSeen := false
	for _, entry := range timeline {
		if entry.Kind == TimelineKindApprovalGroup {
			t.Fatalf("timeline approval group = %+v, want resolved approval omitted", entry)
		}
		if entry.Kind == TimelineKindToolGroup && entry.RunID == response.Run.ID {
			toolGroupSeen = true
			if len(entry.ToolCalls) != 1 || entry.ToolCalls[0].Status != "RUNNING" {
				t.Fatalf("timeline tool group = %+v, want running tool call", entry)
			}
		}
	}
	if !toolGroupSeen {
		t.Fatalf("timeline = %+v, want tool group for run %s", timeline, response.Run.ID)
	}

	close(releaseTool)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, ok, err := restarted.Store().Run(ctx, response.Run.ID)
		if err != nil || !ok {
			t.Fatalf("stored run lookup err=%v ok=%v", err, ok)
		}
		if stored.Status == RunStatusCompleted {
			if executions != 1 {
				t.Fatalf("executions = %d, want 1", executions)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	stored, ok, err := restarted.Store().Run(ctx, response.Run.ID)
	t.Fatalf("stored run after recovered continuation = %+v ok=%v err=%v, want completed", stored, ok, err)
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
	var executions atomic.Int64
	registry := NewToolRegistry()
	for _, name := range []string{"strategy.save_draft", "strategy.optimize"} {
		registry.Register(ToolDescriptor{
			Name: name, Permission: "write_strategy",
			AllowedModes: []string{PermissionModeApproval},
		}, func(context.Context, map[string]any) (any, error) {
			executions.Add(1)
			return map[string]any{"ok": true}, nil
		})
	}
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"strategy.save_draft", "strategy.optimize"},
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
	if got := executions.Load(); got != 0 {
		t.Fatalf("executions after first approval = %d, want 0", got)
	}
	if first.Run == nil || first.Run.Status != RunStatusPending {
		t.Fatalf("first approval run = %+v, want pending", first.Run)
	}
	second, err := runtime.ResolveApproval(ctx, response.PendingApprovals[1].ID, true)
	if err != nil {
		t.Fatalf("second approval: %v", err)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("executions after all approvals = %d, want 2", got)
	}
	if second.Run == nil || second.Run.Status != RunStatusCompleted {
		t.Fatalf("second approval run = %+v, want completed", second.Run)
	}
}

func TestADKTaskUpdateDeleteAndValidation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-a", Title: "Original", Status: "TODO", AgentID: "agent-a", DependsOn: []string{"task-b", "task-b"},
		Order: 2, ModeHint: WorkModeTask, AgentRole: "实现 Agent", PlannerStepID: "__planner_step_2",
		PlanSource: workflowPlanSourcePlanner, WorkflowMode: WorkModeTask, Objective: "完成目标",
		PlannerWarnings: []string{"裁剪了多余步骤", "裁剪了多余步骤"},
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "task-b" {
		t.Fatalf("dependsOn = %+v, want deduplicated task-b", task.DependsOn)
	}
	if task.Order != 2 || task.ModeHint != WorkModeTask || task.AgentRole != "实现 Agent" || task.PlannerStepID != "__planner_step_2" || task.PlanSource != workflowPlanSourcePlanner || task.WorkflowMode != WorkModeTask || task.Objective != "完成目标" {
		t.Fatalf("task planner metadata = %+v, want saved metadata", task)
	}
	if len(task.PlannerWarnings) != 1 || task.PlannerWarnings[0] != "裁剪了多余步骤" {
		t.Fatalf("planner warnings = %+v, want deduplicated warning", task.PlannerWarnings)
	}
	description := "kept details"
	status := "IN_PROGRESS"
	warnings := []string{"planner warning"}
	updated, err := runtime.Store().UpdateTask(ctx, task.ID, TaskPatchRequest{Description: &description, Status: &status, Order: new(3), AgentRole: new("验证 Agent"), PlannerWarnings: warnings})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Title != "Original" || updated.Description != description || updated.Status != status {
		t.Fatalf("updated task = %+v, want partial update preserving title", updated)
	}
	if updated.Order != 3 || updated.AgentRole != "验证 Agent" || len(updated.PlannerWarnings) != 1 || updated.PlannerWarnings[0] != "planner warning" {
		t.Fatalf("updated planner metadata = %+v, want patched metadata", updated)
	}
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "bad-status", Title: "Bad", Status: "NOPE"}); err == nil {
		t.Fatalf("SaveTask invalid status err = nil, want error")
	}
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "self", Title: "Self", DependsOn: []string{"self"}}); err == nil {
		t.Fatalf("SaveTask self dependency err = nil, want error")
	}
	tasks, total, err := runtime.Store().ListTasksPage(ctx, "IN_PROGRESS", "agent-a", "", 20, 0)
	if err != nil {
		t.Fatalf("ListTasksPage: %v", err)
	}
	if total != 1 || len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("filtered tasks total=%d tasks=%+v, want task-a", total, tasks)
	}
	if err := runtime.Store().DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, ok, err := runtime.Store().Task(ctx, task.ID); err != nil || ok {
		t.Fatalf("Task after delete ok=%v err=%v, want missing", ok, err)
	}
}

func TestADKMemoryFiltersDeleteAndAgentValidation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{ID: "agent-memory", Name: "Agent", ProviderID: testProviderID, Status: AgentStatusEnabled})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workspace, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "workspace", Key: "Market", Value: "HK"})
	if err != nil {
		t.Fatalf("SaveMemory workspace: %v", err)
	}
	agentEntry, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "agent", AgentID: agent.ID, Key: "Style", Value: "risk first"})
	if err != nil {
		t.Fatalf("SaveMemory agent: %v", err)
	}
	if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "agent", Key: "missing", Value: "bad"}); err == nil {
		t.Fatalf("SaveMemory agent without agentId err = nil, want error")
	}
	entries, err := runtime.Store().ListMemoryFiltered(ctx, "agent", agent.ID, "style")
	if err != nil {
		t.Fatalf("ListMemoryFiltered: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != agentEntry.ID {
		t.Fatalf("agent memory entries = %+v, want style entry", entries)
	}
	promptEntries, err := runtime.Store().ListMemory(ctx, agent.ID)
	if err != nil {
		t.Fatalf("ListMemory: %v", err)
	}
	if len(promptEntries) != 2 {
		t.Fatalf("prompt memory len=%d entries=%+v, want workspace + agent", len(promptEntries), promptEntries)
	}
	if err := runtime.Store().DeleteMemory(ctx, workspace.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if _, ok, err := runtime.Store().Memory(ctx, workspace.ID); err != nil || ok {
		t.Fatalf("Memory after delete ok=%v err=%v, want missing", ok, err)
	}
}

func TestPrepareAgentInjectsMemoryOnlyWhenEnabled(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "workspace", Key: "preference", Value: "use HK market"}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	disabled, err := runtime.prepareAgent(ctx, Agent{ID: "agent", Instruction: "base", MemoryEnabled: false})
	if err != nil {
		t.Fatalf("prepareAgent disabled: %v", err)
	}
	if strings.Contains(disabled.Instruction, "JFTrade memory") {
		t.Fatalf("disabled instruction = %q, want no memory", disabled.Instruction)
	}
	enabled, err := runtime.prepareAgent(ctx, Agent{ID: "agent", Instruction: "base", MemoryEnabled: true})
	if err != nil {
		t.Fatalf("prepareAgent enabled: %v", err)
	}
	if !strings.Contains(enabled.Instruction, "use HK market") {
		t.Fatalf("enabled instruction = %q, want memory", enabled.Instruction)
	}
}

func TestToolsSearchReturnsOnlyCurrentAgentTools(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "visible.read", DisplayName: "Visible", Category: "test", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	registry.Register(ToolDescriptor{Name: "hidden.read", DisplayName: "Hidden", Category: "test", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	registered, ok := registry.Get("tools.search")
	if !ok {
		t.Fatalf("tools.search not registered")
	}
	output, err := executeRegisteredTool(contextWithToolAgent(ctx, Agent{ID: "agent", Tools: []string{"tools.search", "visible.read"}}), registered, map[string]any{"query": "read"})
	if err != nil {
		t.Fatalf("execute tools.search: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("output = %T, want map", output)
	}
	tools, ok := payload["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools payload = %T, want []map[string]any", payload["tools"])
	}
	if len(tools) != 1 || tools[0]["name"] != "visible.read" {
		t.Fatalf("tools.search tools = %+v, want only visible.read", tools)
	}
	if _, ok := tools[0]["requiresApprovalIn"]; !ok {
		t.Fatalf("tools.search result lacks requiresApprovalIn: %+v", tools[0])
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestWorkflowWriteToolsRequireApprovalExceptLowRiskTaskCreate(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	registry := NewToolRegistry()
	var taskCreates int
	registry.Register(ToolDescriptor{Name: "tasks.create", Permission: "write_task", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		taskCreates++
		return map[string]any{"created": true}, nil
	})
	registry.Register(ToolDescriptor{Name: "memory.remember", Permission: "write_memory", AllowedModes: []string{PermissionModeApproval}}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"remembered": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "workflow-agent", Name: "Workflow", ProviderID: testProviderID, Tools: []string{"tasks.create", "memory.remember"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="tasks.create" title="Follow up" /><execute-tool name="memory.remember" key="market" value="HK" />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if taskCreates != 1 {
		t.Fatalf("task creates = %d, want executed without approval", taskCreates)
	}
	if len(response.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want only memory write approval", len(response.PendingApprovals))
	}
	if response.PendingApprovals[0].ToolName != "memory.remember" {
		t.Fatalf("pending approval tool = %q, want memory.remember", response.PendingApprovals[0].ToolName)
	}
}
