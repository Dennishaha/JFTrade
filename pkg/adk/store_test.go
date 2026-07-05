package adk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
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
	t.Cleanup(func() { jftradeErr5 := CloseSessionService(sessionService); jftradeCheckTestError(t, jftradeErr5) })
	t.Cleanup(func() { jftradeErr3 := store.Close(); jftradeCheckTestError(t, jftradeErr3) })
	if err := ValidateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("ValidateSQLiteSessionService: %v", err)
	}
	runtime := NewRuntimeWithSessionService(store, NewToolRegistry(), sessionService)
	ensureTestProvider(t, runtime)
	return runtime
}

func TestNewStoreUsesSeparatedConcurrentReadAndSingleWritePools(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })

	if got := store.db.Stats().MaxOpenConnections; got != 8 {
		t.Fatalf("read MaxOpenConnections = %d, want 8", got)
	}
	if got := store.db.WriteStats().MaxOpenConnections; got != 1 {
		t.Fatalf("write MaxOpenConnections = %d, want 1", got)
	}
}

func TestStoreMigrationNormalizesHiddenAgentWorkflowDefaults(t *testing.T) {
	t.Skip("incremental ADK migrations were intentionally removed; strict incompatibility is covered below")
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
	jftradeErr1 := store.Close()
	jftradeCheckTestError(t, jftradeErr1)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE `+tableAgents+` SET payload_json = json_set(payload_json, '$.workMode', 'sequential') WHERE id = ?`, agent.ID); err != nil {
		jftradeErr2 := db.Close()
		jftradeCheckTestError(t, jftradeErr2)
		t.Fatalf("update raw agent payload: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM adk_schema_migrations WHERE version = 30`); err != nil {
		jftradeErr3 := db.Close()
		jftradeCheckTestError(t, jftradeErr3)
		t.Fatalf("delete migration marker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	migrated, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore migrate: %v", err)
	}
	defer func() { jftradeCheckTestError(t, migrated.Close()) }()
	var rawMode string
	if err := migrated.db.Get(&rawMode, `SELECT json_extract(payload_json, '$.workMode') FROM `+tableAgents+` WHERE id = ?`, agent.ID); err != nil {
		t.Fatalf("read raw agent mode: %v", err)
	}
	if rawMode != WorkModeChat {
		t.Fatalf("raw migrated work mode = %q, want %q", rawMode, WorkModeChat)
	}
}

func TestStoreMigrationRepairsOrphanTasksAndDuplicateConfirmations(t *testing.T) {
	t.Skip("incremental ADK migrations were intentionally removed; strict incompatibility is covered below")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "adk.db")
	secretsPath := filepath.Join(dir, "secrets", "adk.json")
	skillsPath := filepath.Join(dir, "skills")
	store, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `DELETE FROM adk_schema_migrations WHERE version IN (35, 36, 37, 38, 39, 40)`); err != nil {
		t.Fatalf("reset migration markers: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_adk_approvals_confirmation_call`); err != nil {
		t.Fatalf("drop confirmation index: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO `+tableTasks+` (id, status, agent_id, run_id, payload_json, created_at, updated_at) VALUES ('orphan-task', 'TODO', 'agent', 'missing-run', '{}', '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert orphan task: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO `+tableRuns+` (id, session_id, agent_id, status, payload_json, created_at, updated_at) VALUES ('terminal-with-approval', 'session', 'agent', 'FAILED', '{"id":"terminal-with-approval","sessionId":"session","agentId":"agent","status":"FAILED","pendingApprovals":[{"id":"resolved","status":"APPROVED"}]}', '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert terminal run: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO `+tableRuns+` (id, session_id, agent_id, status, payload_json, created_at, updated_at) VALUES ('run-owner', 'session', 'agent', 'PENDING_APPROVAL', '{"id":"run-owner","sessionId":"session","agentId":"agent","status":"PENDING_APPROVAL","toolCalls":[{"idempotencyKey":"function-owned"}]}', '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z'), ('run-wrong', 'session', 'agent', 'PENDING_APPROVAL', '{"id":"run-wrong","sessionId":"session","agentId":"agent","status":"PENDING_APPROVAL","toolCalls":[]}', '2024-01-02T00:00:00Z', '2024-01-02T00:00:00Z')`); err != nil {
		t.Fatalf("insert approval owner runs: %v", err)
	}
	insertApproval := `INSERT INTO ` + tableApprovals + ` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, 'agent', ?, json_object('id', ?, 'runId', ?, 'agentId', 'agent', 'status', ?, 'functionCallId', 'function-owned', 'confirmationCallId', 'confirmation-duplicate', 'createdAt', ?, 'updatedAt', ?), ?, ?)`
	if _, err := db.ExecContext(ctx, insertApproval, "approval-pending", "run-wrong", ApprovalStatusPending, "approval-pending", "run-wrong", ApprovalStatusPending, "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert pending approval: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertApproval, "approval-approved", "run-owner", ApprovalStatusApproved, "approval-approved", "run-owner", ApprovalStatusApproved, "2024-01-02T00:00:00Z", "2024-01-02T00:00:00Z", "2024-01-02T00:00:00Z", "2024-01-02T00:00:00Z"); err != nil {
		t.Fatalf("insert approved approval: %v", err)
	}
	if _, err := db.ExecContext(ctx, insertApproval, "approval-denied", "run-wrong", ApprovalStatusDenied, "approval-denied", "run-wrong", ApprovalStatusDenied, "2024-01-03T00:00:00Z", "2024-01-03T00:00:00Z", "2024-01-03T00:00:00Z", "2024-01-03T00:00:00Z"); err != nil {
		t.Fatalf("insert denied approval: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	migrated, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore migrate: %v", err)
	}
	defer func() { jftradeCheckTestError(t, migrated.Close()) }()
	var orphanCount int
	if err := migrated.db.Get(&orphanCount, `SELECT COUNT(*) FROM `+tableTasks+` WHERE id = 'orphan-task'`); err != nil || orphanCount != 0 {
		t.Fatalf("orphan task count = %d err=%v", orphanCount, err)
	}
	var embeddedApprovalCount int
	if err := migrated.db.Get(&embeddedApprovalCount, `SELECT json_array_length(json_extract(payload_json, '$.pendingApprovals')) FROM `+tableRuns+` WHERE id = 'terminal-with-approval'`); err != nil || embeddedApprovalCount != 0 {
		t.Fatalf("terminal embedded approval count = %d err=%v", embeddedApprovalCount, err)
	}
	approval, ok, err := migrated.ApprovalByConfirmationCallID(ctx, "confirmation-duplicate")
	if err != nil || !ok || approval.ID != "approval-denied" || approval.RunID != "run-owner" {
		t.Fatalf("canonical approval = %+v ok=%v err=%v", approval, ok, err)
	}
	duplicate := Approval{ID: "approval-late", RunID: "run", AgentID: "agent", Status: ApprovalStatusPending, ConfirmationCallID: "confirmation-duplicate"}
	if _, created, err := migrated.SaveApprovalIfConfirmationAbsent(ctx, duplicate); err != nil || created {
		t.Fatalf("duplicate approval created=%v err=%v", created, err)
	}
}

func TestStoreMigrationReopensCompletedWorkflowWithRecoverablePendingApproval(t *testing.T) {
	t.Skip("incremental ADK migrations were intentionally removed; strict incompatibility is covered below")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "adk.db")
	secretsPath := filepath.Join(dir, "secrets", "adk.json")
	skillsPath := filepath.Join(dir, "skills")
	store, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `DELETE FROM adk_schema_migrations WHERE version IN (41, 42)`); err != nil {
		t.Fatalf("reset recovery migrations: %v", err)
	}
	parentJSON := `{"id":"parent-terminal","sessionId":"session","agentId":"agent","status":"COMPLETED","workMode":"loop","workflowStatus":"COMPLETED","completedAt":"2026-06-21T00:00:00Z","finalMessageId":"message-final","pendingApprovals":[]}`
	childJSON := `{"id":"child-terminal","sessionId":"session","agentId":"agent","parentRunId":"parent-terminal","status":"COMPLETED","workMode":"chat","completedAt":"2026-06-21T00:00:00Z","finalMessageId":"message-child","pendingApprovals":[]}`
	if _, err := db.ExecContext(ctx, `INSERT INTO `+tableRuns+` (id, session_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, 'session', 'agent', 'COMPLETED', ?, '2026-06-21T00:00:00Z', '2026-06-21T00:00:00Z'), (?, 'session', 'agent', 'COMPLETED', ?, '2026-06-21T00:00:00Z', '2026-06-21T00:00:00Z')`, "parent-terminal", parentJSON, "child-terminal", childJSON); err != nil {
		t.Fatalf("insert terminal workflow: %v", err)
	}
	approvalJSON := `{"id":"approval-late","runId":"child-terminal","agentId":"agent","toolName":"strategy.research_backtest","status":"PENDING","functionCallId":"function-late","confirmationCallId":"confirmation-late","createdAt":"2026-06-21T00:01:00Z","updatedAt":"2026-06-21T00:01:00Z"}`
	if _, err := db.ExecContext(ctx, `INSERT INTO `+tableApprovals+` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES ('approval-late', 'child-terminal', 'agent', 'PENDING', ?, '2026-06-21T00:01:00Z', '2026-06-21T00:01:00Z')`, approvalJSON); err != nil {
		t.Fatalf("insert late approval: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	migrated, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore migrate: %v", err)
	}
	defer func() { jftradeCheckTestError(t, migrated.Close()) }()
	child, ok, err := migrated.Run(ctx, "child-terminal")
	if err != nil || !ok {
		t.Fatalf("child lookup ok=%v err=%v", ok, err)
	}
	if child.Status != RunStatusPending || child.CompletedAt != nil || child.FinalMessageID != "" || len(child.PendingApprovals) != 1 {
		t.Fatalf("recovered child = %+v", child)
	}
	parent, ok, err := migrated.Run(ctx, "parent-terminal")
	if err != nil || !ok {
		t.Fatalf("parent lookup ok=%v err=%v", ok, err)
	}
	if parent.Status != RunStatusPending || parent.WorkflowStatus != workflowStatusPaused || parent.CompletedAt != nil || len(parent.PendingApprovals) != 1 {
		t.Fatalf("recovered parent = %+v", parent)
	}
}

func TestNewStoreRejectsLegacyDatabaseWithoutMutatingIt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "adk.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE legacy_data (id TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO legacy_data (id, value) VALUES ('keep', 'untouched')`); err != nil {
		t.Fatalf("create legacy db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	_, err = NewStore(dbPath, filepath.Join(dir, "secrets.json"), filepath.Join(dir, "skills"))
	if err == nil || !strings.Contains(err.Error(), "schema metadata is missing") {
		t.Fatalf("NewStore legacy error = %v", err)
	}

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen legacy db: %v", err)
	}
	defer func() { jftradeCheckTestError(t, db.Close()) }()
	var value string
	if err := db.QueryRow(`SELECT value FROM legacy_data WHERE id = 'keep'`).Scan(&value); err != nil {
		t.Fatalf("legacy row was modified: %v", err)
	}
	if value != "untouched" {
		t.Fatalf("legacy value = %q", value)
	}
	var metadataCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='jftrade_schema_meta'`).Scan(&metadataCount); err != nil {
		t.Fatalf("inspect metadata table: %v", err)
	}
	if metadataCount != 0 {
		t.Fatal("legacy database was modified with schema metadata")
	}
}

func TestSaveApprovalIfConfirmationAbsentIsConcurrentIdempotent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	const workers = 24
	var wg sync.WaitGroup
	created := make(chan string, workers)
	errs := make(chan error, workers)
	for index := range workers {
		wg.Go(func() {
			approval := Approval{
				ID: "approval-concurrent-" + fmt.Sprint(index), RunID: "run-concurrent-owner", AgentID: "agent",
				ToolName: "strategy.research_backtest", Status: ApprovalStatusPending,
				FunctionCallID: "function-concurrent", ConfirmationCallID: "confirmation-concurrent",
			}
			saved, wasCreated, err := runtime.Store().SaveApprovalIfConfirmationAbsent(ctx, approval)
			if err != nil {
				errs <- err
				return
			}
			if saved.ConfirmationCallID != "confirmation-concurrent" {
				errs <- fmt.Errorf("saved approval confirmation = %q", saved.ConfirmationCallID)
				return
			}
			if wasCreated {
				created <- saved.ID
			}
		})
	}
	wg.Wait()
	close(created)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("SaveApprovalIfConfirmationAbsent concurrent error: %v", err)
		}
	}
	var createdIDs []string
	for id := range created {
		createdIDs = append(createdIDs, id)
	}
	if len(createdIDs) != 1 {
		t.Fatalf("created approvals = %+v, want exactly one creator", createdIDs)
	}
	approval, ok, err := runtime.Store().ApprovalByConfirmationCallID(ctx, "confirmation-concurrent")
	if err != nil || !ok {
		t.Fatalf("ApprovalByConfirmationCallID ok=%v err=%v", ok, err)
	}
	all, err := runtime.Store().ListApprovals(ctx)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(all) != 1 || all[0].ID != approval.ID {
		t.Fatalf("stored approvals = %+v, canonical = %+v", all, approval)
	}
}

func newRuntimeWithRegistry(t *testing.T, store *Store, registry *ToolRegistry) *Runtime {
	t.Helper()
	sessionService, err := NewSQLiteSessionService(filepath.Join(filepath.Dir(store.SkillsPath()), "adk-session.db"))
	if err != nil {
		t.Fatalf("NewSessionService: %v", err)
	}
	t.Cleanup(func() { jftradeErr4 := CloseSessionService(sessionService); jftradeCheckTestError(t, jftradeErr4) })
	if err := ValidateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("ValidateSQLiteSessionService: %v", err)
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
	t.Cleanup(func() { jftradeErr1 := store.Close(); jftradeCheckTestError(t, jftradeErr1) })
	registry := NewToolRegistry()
	executed := false
	registry.Register(ToolDescriptor{
		Name:         "approval.required",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
	}, func(context.Context, map[string]any) (any, error) {
		executed = true
		return map[string]any{"saved": true}, nil
	})
	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		ProviderID:     testProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required 保存策略"})
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
	if resolution.Run.UserMessage != "@approval.required 保存策略" {
		t.Fatalf("run user message = %q, want original request", resolution.Run.UserMessage)
	}
	if len(resolution.Run.ToolSummaries) != 1 || !strings.Contains(resolution.Run.ToolSummaries[0], "approval.required") {
		t.Fatalf("tool summaries = %+v, want saved draft summary", resolution.Run.ToolSummaries)
	}
}

func TestIdempotentApprovalRecoversPendingRunWithStaleEmbeddedApproval(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	registry := NewToolRegistry()
	executed := false
	registry.Register(ToolDescriptor{
		Name:         "approval.required",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
	}, func(context.Context, map[string]any) (any, error) {
		executed = true
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent-stale-approval",
		Name:           "Agent",
		ProviderID:     testProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required 保存策略"})
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
		Name:         "approval.required",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
	}, func(context.Context, map[string]any) (any, error) {
		executed.Store(true)
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent-reconcile-approval",
		Name:           "Agent",
		ProviderID:     testProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required 保存策略"})
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
		Name:         "approval.required",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
	}, func(context.Context, map[string]any) (any, error) {
		executed = true
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "Agent",
		ProviderID:     testProviderID,
		Tools:          []string{"approval.required"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required 保存策略"})
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
		Name:         "approval.required",
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
		Tools:          []string{"approval.required"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required 保存策略"})
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
		Name:         "approval.required",
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
		Tools:          []string{"approval.required"},
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required 保存策略"})
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

func TestResumeGoalRunAllowsTimedOutGoalWithFreshTimeoutWindow(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	runtime.SetRuntimeLimitsProvider(func() RuntimeLimits {
		return RuntimeLimits{RunTimeout: 45 * time.Minute}
	})

	completedAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	startedAt := time.Now().UTC().Add(-DefaultRunTimeout - time.Minute).Format(time.RFC3339Nano)
	run := Run{
		ID:             "run-timed-out-goal",
		SessionID:      "session-1",
		AgentID:        "agent-1",
		Status:         RunStatusTimedOut,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		Message:        "run timed out",
		FailureReason:  "run exceeded maximum duration",
		ErrorCode:      runErrorCode(RunStatusTimedOut),
		Degraded:       true,
		CreatedAt:      startedAt,
		StartedAt:      startedAt,
		UpdatedAt:      completedAt,
		CompletedAt:    &completedAt,
		MaxDurationMs:  DefaultRunTimeout.Milliseconds(),
		Usage:          &RunUsage{},
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	resumed, err := runtime.ResumeGoalRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ResumeGoalRun: %v", err)
	}
	if resumed.Status != RunStatusRunning || resumed.ResumeState != "user_resuming" {
		t.Fatalf("resumed status/state = %q/%q, want running/user_resuming", resumed.Status, resumed.ResumeState)
	}
	if resumed.CompletedAt != nil || resumed.ErrorCode != "" || resumed.FailureReason != "" || resumed.Degraded {
		t.Fatalf("resumed terminal fields not cleared: %+v", resumed)
	}
	if resumed.StartedAt == startedAt {
		t.Fatalf("StartedAt was not refreshed: %q", resumed.StartedAt)
	}
	if resumed.MaxDurationMs != int64((45*time.Minute)/time.Millisecond) {
		t.Fatalf("MaxDurationMs = %d, want fresh runtime timeout", resumed.MaxDurationMs)
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
