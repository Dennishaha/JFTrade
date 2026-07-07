package adk

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

func TestStoreSessionContextAndNoticeBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)
	if _, err := store.SaveSessionContext(ctx, SessionContextState{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty SaveSessionContext err = %v, want os.ErrNotExist", err)
	}
	if err := store.DeleteSessionContext(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty DeleteSessionContext err = %v, want os.ErrNotExist", err)
	}
	state, err := store.SaveSessionContext(ctx, SessionContextState{SessionID: " session-context ", CurrentInputTokens: 10})
	if err != nil {
		t.Fatalf("SaveSessionContext first: %v", err)
	}
	if state.SessionID != "session-context" || state.ContextRevisionID == "" || state.CreatedAt == "" || state.UpdatedAt == "" {
		t.Fatalf("first context state = %+v", state)
	}
	updated, err := store.SaveSessionContext(ctx, SessionContextState{SessionID: state.SessionID, ContextRevisionID: state.ContextRevisionID, CurrentInputTokens: 20})
	if err != nil {
		t.Fatalf("SaveSessionContext update: %v", err)
	}
	if updated.CreatedAt != state.CreatedAt || updated.CurrentInputTokens != 20 {
		t.Fatalf("updated context state = %+v, first %+v", updated, state)
	}
	loaded, ok, err := store.SessionContext(ctx, "session-context")
	if err != nil || !ok || loaded.ContextRevisionID != state.ContextRevisionID {
		t.Fatalf("SessionContext loaded = %+v ok=%v err=%v", loaded, ok, err)
	}
	if err := store.DeleteSessionContext(ctx, "session-context"); err != nil {
		t.Fatalf("DeleteSessionContext: %v", err)
	}
	if notices, err := (*Store)(nil).SessionNotices(ctx, "session"); err != nil || len(notices) != 0 {
		t.Fatalf("nil SessionNotices = %+v/%v, want empty/nil", notices, err)
	}
	if notices, err := store.SessionNotices(ctx, " "); err != nil || len(notices) != 0 {
		t.Fatalf("empty SessionNotices = %+v/%v, want empty/nil", notices, err)
	}
	notice, err := store.SaveSessionNotice(ctx, TimelineEntry{SessionID: "session", Kind: " ", Text: " hello "})
	if err != nil {
		t.Fatalf("SaveSessionNotice blank kind: %v", err)
	}
	if notice.Kind != TimelineKindContextNotice || notice.Text != "hello" {
		t.Fatalf("normalized notice = %+v", notice)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableSessionNotices+` (id, session_id, run_id, kind, status, payload_json, created_at, updated_at) VALUES (?, ?, '', ?, ?, ?, ?, ?)`,
		"notice-bad-json", "session-bad", TimelineKindContextNotice, TimelineStatusFinal, "{", nowString(), nowString()); err != nil {
		t.Fatalf("insert bad notice: %v", err)
	}
	if _, err := store.SessionNotices(ctx, "session-bad"); err == nil {
		t.Fatal("SessionNotices bad payload err = nil, want error")
	}

	dir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed session context: %v", err)
	}
	jftradeCheckTestError(t, closedStore.Close())
	if _, _, err := closedStore.SessionContext(ctx, "session"); err == nil {
		t.Fatal("closed SessionContext err = nil, want error")
	}
	if _, err := closedStore.SaveSessionContext(ctx, SessionContextState{SessionID: "session"}); err == nil {
		t.Fatal("closed SaveSessionContext err = nil, want error")
	}
	if err := closedStore.DeleteSessionContext(ctx, "session"); err == nil {
		t.Fatal("closed DeleteSessionContext err = nil, want error")
	}
	if _, err := closedStore.SessionNotices(ctx, "session"); err == nil {
		t.Fatal("closed SessionNotices err = nil, want error")
	}
}

func TestNewStoreAndDeleteSessionBoundaryBranches(t *testing.T) {
	if err := (*Store)(nil).Close(); err != nil {
		t.Fatalf("nil store close: %v", err)
	}
	if got := (*Store)(nil).SkillsPath(); got != "" {
		t.Fatalf("nil store skills path = %q, want empty", got)
	}
	(*Store)(nil).SetSessionService(nil)

	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if _, err := NewStore(filepath.Join(blocker, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills")); err == nil || !strings.Contains(err.Error(), "create adk db directory") {
		t.Fatalf("db directory error = %v", err)
	}
	if _, err := NewStore(filepath.Join(dir, "ok.db"), filepath.Join(blocker, "adk.json"), filepath.Join(dir, "skills")); err == nil || !strings.Contains(err.Error(), "create adk secret directory") {
		t.Fatalf("secret directory error = %v", err)
	}
	if _, err := NewStore(filepath.Join(dir, "ok2.db"), filepath.Join(dir, "secrets2", "adk.json"), blocker); err == nil || !strings.Contains(err.Error(), "create adk skills directory") {
		t.Fatalf("skills directory error = %v", err)
	}
	if _, err := NewStore(dir, filepath.Join(dir, "secrets3", "adk.json"), filepath.Join(dir, "skills3")); err == nil || !strings.Contains(err.Error(), "database path is not a regular file") {
		t.Fatalf("open directory as db error = %v", err)
	}

	store := newBusinessStore(t)
	if err := store.DeleteSession(t.Context(), " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty DeleteSession err = %v, want os.ErrNotExist", err)
	}

	for _, tc := range []struct {
		name      string
		dropTable string
	}{
		{name: "approvals", dropTable: tableApprovals},
		{name: "tasks", dropTable: tableTasks},
		{name: "runs", dropTable: tableRuns},
		{name: "session_contexts", dropTable: tableSessionContexts},
		{name: "session_context_live", dropTable: tableSessionContextLive},
		{name: "handoff", dropTable: tableHandoffSegments},
		{name: "notices", dropTable: tableSessionNotices},
		{name: "composer", dropTable: tableSessionComposer},
		{name: "sessions", dropTable: tableSessions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			dir := t.TempDir()
			broken, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
			if err != nil {
				t.Fatalf("NewStore broken: %v", err)
			}
			t.Cleanup(func() { jftradeCheckTestError(t, broken.Close()) })
			if _, err := broken.db.ExecContext(ctx, `DROP TABLE `+tc.dropTable); err != nil {
				t.Fatalf("drop %s: %v", tc.dropTable, err)
			}
			if err := broken.DeleteSession(ctx, "session"); err == nil {
				t.Fatalf("DeleteSession with missing %s err = nil, want error", tc.dropTable)
			}
		})
	}
}

func TestStoreProviderSecretAndDefaultErrorBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := NewStore(" ", filepath.Join(t.TempDir(), "secrets", "adk.json"), filepath.Join(t.TempDir(), "skills")); err == nil || !strings.Contains(err.Error(), "db path") {
		t.Fatalf("NewStore blank path err = %v, want db path error", err)
	}
	if _, err := NewStore(string([]byte{0}), filepath.Join(t.TempDir(), "secrets", "adk.json"), filepath.Join(t.TempDir(), "skills")); err == nil {
		t.Fatal("NewStore nul path err = nil, want open error")
	}

	store := newBusinessStore(t)
	blocker := filepath.Join(t.TempDir(), "secret-parent-file")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatalf("write secret blocker: %v", err)
	}
	store.secrets.path = filepath.Join(blocker, "adk.json")
	if _, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "secret-error-provider", BaseURL: "https://example.test/v1", Model: "model", APIKey: "sk-secret", Enabled: true,
	}); err == nil {
		t.Fatal("SaveProvider secret write err = nil, want error")
	}

	defaultStore := newBusinessStore(t)
	first, err := defaultStore.SaveProvider(ctx, ProviderWriteRequest{ID: "first-default-error", BaseURL: "https://example.test/v1", Model: "model", Enabled: true})
	if err != nil {
		t.Fatalf("SaveProvider first default: %v", err)
	}
	second, err := defaultStore.SaveProvider(ctx, ProviderWriteRequest{ID: "second-default-error", BaseURL: "https://example.test/v1", Model: "model", Enabled: true})
	if err != nil {
		t.Fatalf("SaveProvider second default: %v", err)
	}
	if _, err := defaultStore.db.ExecContext(ctx, `UPDATE `+tableProviders+` SET payload_json = json_set(payload_json, '$.default', json('false')) WHERE id IN (?, ?)`, first.ID, second.ID); err != nil {
		t.Fatalf("clear provider defaults: %v", err)
	}
	if _, err := defaultStore.db.ExecContext(ctx, `CREATE TRIGGER fail_provider_default_update BEFORE UPDATE ON `+tableProviders+` BEGIN SELECT RAISE(FAIL, 'provider default update failed'); END`); err != nil {
		t.Fatalf("create provider trigger: %v", err)
	}
	if _, err := defaultStore.ListProviders(ctx); err == nil || !strings.Contains(err.Error(), "provider default update failed") {
		t.Fatalf("ListProviders default repair err = %v, want trigger error", err)
	}
	if _, err := defaultStore.SetDefaultProvider(ctx, second.ID); err == nil || !strings.Contains(err.Error(), "provider default update failed") {
		t.Fatalf("SetDefaultProvider default save err = %v, want trigger error", err)
	}

	providers := []Provider{
		{ID: "b", CreatedAt: "same", Default: false},
		{ID: "a", CreatedAt: "same", Default: false},
		{ID: "default", CreatedAt: "later", Default: true},
	}
	sortProvidersDefaultFirst(providers)
	if providers[0].ID != "default" || providers[1].ID != "a" || providers[2].ID != "b" {
		t.Fatalf("sortProvidersDefaultFirst id tie = %+v", providers)
	}
}

func TestStoreRunApprovalAndMemoryErrorBranches(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)
	if err := store.SaveRun(ctx, Run{
		ID: "run-bad-json", SessionID: "session", AgentID: "agent", Status: RunStatusRunning,
		ToolCalls: []ToolCall{{ID: "tool", Output: func() {}}},
	}); err == nil {
		t.Fatal("SaveRun marshal err = nil, want error")
	}
	if err := store.SaveApproval(ctx, Approval{
		ID: "approval-bad-json", RunID: "run", AgentID: "agent", Status: ApprovalStatusPending,
		Input: map[string]any{"bad": func() {}},
	}); err == nil {
		t.Fatal("SaveApproval marshal err = nil, want error")
	}
	if _, created, err := store.SaveApprovalIfConfirmationAbsent(ctx, Approval{
		ID: "approval-if-bad-json", RunID: "run", AgentID: "agent", Status: ApprovalStatusPending,
		Input: map[string]any{"bad": func() {}},
	}); err == nil || created {
		t.Fatalf("SaveApprovalIfConfirmationAbsent bad json created=%v err=%v, want marshal error", created, err)
	}

	if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableApprovals+` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"approval-bad-payload", "run-bad-payload", "agent", ApprovalStatusPending, `"bad"`, nowString(), nowString()); err != nil {
		t.Fatalf("insert bad approval payload: %v", err)
	}
	if err := store.SaveRunAndDenyPendingApprovals(ctx, Run{ID: "run-bad-payload", SessionID: "session", AgentID: "agent", Status: RunStatusFailed}); err == nil {
		t.Fatal("SaveRunAndDenyPendingApprovals bad approval payload err = nil, want error")
	}

	approval := Approval{ID: "approval-trigger-ignore", RunID: "run-trigger", AgentID: "agent", Status: ApprovalStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()}
	if err := store.SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval trigger ignore: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER ignore_approval_update BEFORE UPDATE ON `+tableApprovals+` WHEN OLD.id = 'approval-trigger-ignore' BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatalf("create approval ignore trigger: %v", err)
	}
	current, changed, err := store.ResolvePendingApproval(ctx, approval.ID, ApprovalStatusApproved)
	if err != nil || changed || current.ID != approval.ID || current.Status != ApprovalStatusPending {
		t.Fatalf("ResolvePendingApproval ignored update = %+v changed=%v err=%v, want current pending/no change", current, changed, err)
	}

	if _, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: " ", Scope: "workspace"}); err == nil || !strings.Contains(err.Error(), "key is required") {
		t.Fatalf("SaveMemory blank key err = %v, want key required", err)
	}
	if _, err := store.ListMemoryFiltered(ctx, "team", "", ""); err == nil || !strings.Contains(err.Error(), "workspace or agent") {
		t.Fatalf("ListMemoryFiltered bad scope err = %v, want scope error", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableMemory+` (id, agent_id, scope, memory_key, payload_json, created_at, updated_at) VALUES (?, '', 'workspace', ?, ?, ?, ?)`,
		"memory-bad-json", "bad-json", `"bad"`, nowString(), nowString()); err != nil {
		t.Fatalf("insert bad memory: %v", err)
	}
	if _, err := store.ListMemoryFiltered(ctx, "workspace", "", "bad-json"); err == nil {
		t.Fatal("ListMemoryFiltered bad payload err = nil, want error")
	}
	if err := store.DeleteTask(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteTask blank err = %v, want not exist", err)
	}
	if err := store.DeleteMemory(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteMemory blank err = %v, want not exist", err)
	}

	pauseStore := newBusinessStore(t)
	if _, err := pauseStore.db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
		t.Fatalf("drop runs for pause guard: %v", err)
	}
	pauseRuntime := &Runtime{store: pauseStore}
	if _, err := pauseRuntime.saveRunPreservingUserGoalPause(ctx, Run{ID: "root-loop", WorkMode: WorkModeLoop, Status: RunStatusRunning}); err == nil {
		t.Fatal("saveRunPreservingUserGoalPause prepare err = nil, want error")
	}
	saveErrStore := newBusinessStore(t)
	if _, err := saveErrStore.db.ExecContext(ctx, `CREATE TRIGGER fail_run_pause_save BEFORE INSERT ON `+tableRuns+` BEGIN SELECT RAISE(FAIL, 'pause save failed'); END`); err != nil {
		t.Fatalf("create pause save trigger: %v", err)
	}
	if _, err := (&Runtime{store: saveErrStore}).saveRunPreservingUserGoalPause(ctx, Run{ID: "root-loop-save", WorkMode: WorkModeLoop, Status: RunStatusRunning}); err == nil || !strings.Contains(err.Error(), "pause save failed") {
		t.Fatalf("saveRunPreservingUserGoalPause save err = %v, want trigger error", err)
	}
}

func TestStoreBuiltinAndLowLevelJSONErrorBranches(t *testing.T) {
	ctx := t.Context()
	skillsErrorStore := newBusinessStore(t)
	if _, err := skillsErrorStore.db.ExecContext(ctx, `DROP TABLE `+tableSkills); err != nil {
		t.Fatalf("drop skills: %v", err)
	}
	if err := skillsErrorStore.ensureBuiltins(ctx); err == nil {
		t.Fatal("ensureBuiltins missing skills table err = nil, want error")
	}

	agentErrorStore := newBusinessStore(t)
	if _, err := agentErrorStore.db.ExecContext(ctx, `DROP TABLE `+tableAgents); err != nil {
		t.Fatalf("drop agents: %v", err)
	}
	if err := agentErrorStore.ensureBuiltins(ctx); err == nil {
		t.Fatal("ensureBuiltins missing agents table err = nil, want error")
	}

	legacyStore := newBusinessStore(t)
	if _, err := legacyStore.SaveSkill(ctx, Skill{
		ID: strategypinespec.LegacyBuiltinSkillName, DisplayName: "Legacy", Source: "builtin", Builtin: true, Enabled: true,
	}); err != nil {
		t.Fatalf("SaveSkill legacy: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `CREATE TRIGGER fail_legacy_skill_delete BEFORE DELETE ON `+tableSkills+` BEGIN SELECT RAISE(FAIL, 'legacy delete failed'); END`); err != nil {
		t.Fatalf("create legacy delete trigger: %v", err)
	}
	if err := legacyStore.deleteLegacyBuiltinSkills(ctx); err == nil || !strings.Contains(err.Error(), "legacy delete failed") {
		t.Fatalf("deleteLegacyBuiltinSkills delete err = %v, want trigger error", err)
	}

	jsonStore := newBusinessStore(t)
	if _, err := jsonStore.db.ExecContext(ctx, `INSERT INTO `+tableAgents+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"agent-bad-json", `"bad"`, nowString(), nowString()); err != nil {
		t.Fatalf("insert bad agent json: %v", err)
	}
	if _, _, err := jsonStore.Agent(ctx, "agent-bad-json"); err == nil {
		t.Fatal("Agent bad payload err = nil, want error")
	}
	if err := jsonStore.saveJSON(ctx, tableAgents, "bad-save-json", nowString(), nowString(), map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("saveJSON marshal err = nil, want error")
	}
	if err := savePreparedRunWithExecutor(ctx, jsonStore.db, Run{
		ID: "prepared-bad-json", SessionID: "session", AgentID: "agent", Status: RunStatusRunning,
		ToolCalls: []ToolCall{{ID: "tool", Output: func() {}}},
	}); err == nil {
		t.Fatal("savePreparedRunWithExecutor marshal err = nil, want error")
	}

	badSecret := secretStore{path: filepath.Join(t.TempDir(), "bad.json")}
	if err := os.WriteFile(badSecret.path, []byte("{"), 0o600); err != nil {
		t.Fatalf("write bad secret: %v", err)
	}
	if _, err := badSecret.read(); err == nil {
		t.Fatal("secretStore read invalid JSON err = nil, want error")
	}
	if _, _, err := badSecret.get("provider"); err == nil {
		t.Fatal("secretStore get invalid JSON err = nil, want error")
	}
	if err := badSecret.set("provider", "sk"); err == nil {
		t.Fatal("secretStore set invalid JSON err = nil, want error")
	}
	if err := badSecret.delete("provider"); err == nil {
		t.Fatal("secretStore delete invalid JSON err = nil, want error")
	}
	blankSecret := secretStore{path: filepath.Join(t.TempDir(), "blank.json")}
	if err := os.WriteFile(blankSecret.path, []byte(" \n\t "), 0o600); err != nil {
		t.Fatalf("write blank secret: %v", err)
	}
	if data, err := blankSecret.read(); err != nil || len(data) != 0 {
		t.Fatalf("secretStore blank read = %#v/%v, want empty/nil", data, err)
	}
	blocker := filepath.Join(t.TempDir(), "secret-dir-file")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatalf("write secret write blocker: %v", err)
	}
	if err := (secretStore{path: filepath.Join(blocker, "adk.json")}).write(map[string]string{"provider": "sk"}); err == nil {
		t.Fatal("secretStore write mkdir err = nil, want error")
	}
}

func TestWorkflowStoreFullCRUDAndLogBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	workflow, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{
		Name: "Coverage Workflow", Status: WorkflowStatusEnabled, AgentID: "agent", WorkMode: WorkModeLoop,
		PromptTemplate: "run {{symbol}}", DefaultInputs: map[string]any{"symbol": "TME"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowDefinition generated id: %v", err)
	}
	if workflow.ID == "" || workflow.CreatedAt == "" || workflow.UpdatedAt == "" {
		t.Fatalf("generated workflow = %+v", workflow)
	}
	loadedWorkflow, ok, err := store.WorkflowDefinition(ctx, workflow.ID)
	if err != nil || !ok || loadedWorkflow.ID != workflow.ID {
		t.Fatalf("WorkflowDefinition = %+v ok=%v err=%v", loadedWorkflow, ok, err)
	}
	page, total, err := store.ListWorkflowDefinitionsPage(ctx, WorkflowStatusEnabled, 10, 0)
	if err != nil || total != 1 || len(page) != 1 {
		t.Fatalf("ListWorkflowDefinitionsPage = len:%d total:%d err:%v", len(page), total, err)
	}

	schedule, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
		WorkflowID: workflow.ID, Type: WorkflowTriggerTypeSchedule, Status: WorkflowTriggerStatusEnabled,
		NextRunAt: "2026-01-01T00:00:00Z", Config: map[string]any{"cron": "* * * * *"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger schedule: %v", err)
	}
	webhook, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{
		WorkflowID: workflow.ID, Type: WorkflowTriggerTypeWebhook, Status: WorkflowTriggerStatusEnabled,
		Config: map[string]any{"path": "/hook"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger webhook: %v", err)
	}
	if schedule.ID == "" || webhook.ID == "" {
		t.Fatalf("generated triggers schedule=%+v webhook=%+v", schedule, webhook)
	}
	loadedTrigger, ok, err := store.WorkflowTrigger(ctx, schedule.ID)
	if err != nil || !ok || loadedTrigger.ID != schedule.ID {
		t.Fatalf("WorkflowTrigger = %+v ok=%v err=%v", loadedTrigger, ok, err)
	}
	triggers, err := store.ListWorkflowTriggers(ctx, workflow.ID)
	if err != nil || len(triggers) != 2 {
		t.Fatalf("ListWorkflowTriggers = %+v err=%v", triggers, err)
	}
	enabledWebhooks, err := store.ListEnabledWorkflowTriggersByType(ctx, WorkflowTriggerTypeWebhook)
	if err != nil || len(enabledWebhooks) != 1 || enabledWebhooks[0].ID != webhook.ID {
		t.Fatalf("ListEnabledWorkflowTriggersByType = %+v err=%v", enabledWebhooks, err)
	}
	due, err := store.ListDueWorkflowScheduleTriggers(ctx, "2026-01-01T00:00:01Z", 10)
	if err != nil || len(due) != 1 || due[0].ID != schedule.ID {
		t.Fatalf("ListDueWorkflowScheduleTriggers = %+v err=%v", due, err)
	}

	logEntry, err := store.SaveWorkflowTriggerLog(ctx, WorkflowTriggerLog{
		WorkflowID: workflow.ID, TriggerID: schedule.ID, TriggerType: schedule.Type,
		Status: WorkflowTriggerLogStatusPendingApproval, RunID: "run-one",
		Inputs: map[string]any{"symbol": "TME"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTriggerLog: %v", err)
	}
	if logEntry.ID == "" || logEntry.CreatedAt == "" {
		t.Fatalf("generated log = %+v", logEntry)
	}
	loadedLog, ok, err := store.WorkflowTriggerLog(ctx, logEntry.ID)
	if err != nil || !ok || loadedLog.ID != logEntry.ID {
		t.Fatalf("WorkflowTriggerLog = %+v ok=%v err=%v", loadedLog, ok, err)
	}
	logs, total, err := store.ListWorkflowTriggerLogsPage(ctx, workflow.ID, schedule.ID, WorkflowTriggerLogStatusPendingApproval, 10, 0)
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("ListWorkflowTriggerLogsPage = len:%d total:%d err:%v", len(logs), total, err)
	}
	activeLogs, err := store.ListActiveWorkflowTriggerLogs(ctx, schedule.ID)
	if err != nil || len(activeLogs) != 1 {
		t.Fatalf("ListActiveWorkflowTriggerLogs = %+v err=%v", activeLogs, err)
	}

	deletedTrigger, err := store.DeleteWorkflowTrigger(ctx, webhook.ID)
	if err != nil || deletedTrigger.DeletedAt == nil || deletedTrigger.Status != WorkflowTriggerStatusDisabled {
		t.Fatalf("DeleteWorkflowTrigger = %+v err=%v", deletedTrigger, err)
	}
	if _, err := store.DeleteWorkflowTrigger(ctx, webhook.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteWorkflowTrigger again err = %v, want not exist", err)
	}
	deletedWorkflow, err := store.DeleteWorkflowDefinition(ctx, workflow.ID)
	if err != nil || deletedWorkflow.DeletedAt == nil || deletedWorkflow.Status != WorkflowStatusDisabled {
		t.Fatalf("DeleteWorkflowDefinition = %+v err=%v", deletedWorkflow, err)
	}
	disabledSchedule, ok, err := store.WorkflowTrigger(ctx, schedule.ID)
	if err != nil || !ok || disabledSchedule.Status != WorkflowTriggerStatusDisabled {
		t.Fatalf("workflow delete did not disable schedule trigger = %+v ok=%v err=%v", disabledSchedule, ok, err)
	}
	if _, err := store.DeleteWorkflowDefinition(ctx, workflow.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteWorkflowDefinition again err = %v, want not exist", err)
	}
}

func TestStoreSessionComposerBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)
	if _, _, err := store.SessionComposerState(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blank SessionComposerState err = %v, want not exist", err)
	}
	if _, err := store.SaveSessionComposerState(ctx, " ", SessionComposerStatePatch{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blank SaveSessionComposerState err = %v, want not exist", err)
	}
	if _, err := store.SaveSessionComposerState(ctx, "missing-session", SessionComposerStatePatch{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing SaveSessionComposerState err = %v, want not exist", err)
	}
	session, err := store.CreateSession(ctx, "agent", "Composer")
	if err != nil {
		t.Fatalf("CreateSession composer: %v", err)
	}
	badWorkMode := "bad-mode"
	if _, err := store.SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{WorkModeOverride: &badWorkMode}); err == nil || !strings.Contains(err.Error(), "invalid composer work mode") {
		t.Fatalf("bad work mode composer err = %v, want invalid work mode", err)
	}
	badPermission := "bad-permission"
	if _, err := store.SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{PermissionModeOverride: &badPermission}); err == nil || !strings.Contains(err.Error(), "invalid composer permission mode") {
		t.Fatalf("bad permission composer err = %v, want invalid permission", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableSessionComposer+` (id, session_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"bad-composer-json", "bad-composer-json", `"bad"`, nowString(), nowString()); err != nil {
		t.Fatalf("insert bad composer: %v", err)
	}
	if _, _, err := store.SessionComposerState(ctx, "bad-composer-json"); err == nil {
		t.Fatal("SessionComposerState bad payload err = nil, want error")
	}
	normalized := normalizeSessionComposerState("session", SessionComposerState{WorkModeOverride: "bad-mode", PermissionModeOverride: "bad-permission"})
	if normalized.WorkModeOverride != "" || normalized.PermissionModeOverride != "" {
		t.Fatalf("normalizeSessionComposerState invalid modes = %+v, want cleared", normalized)
	}
}

func TestStoreNormalizationEdgeBranches(t *testing.T) {
	if got := normalizeContextWindowTokens(100_000_000); got != 10_000_000 {
		t.Fatalf("normalizeContextWindowTokens high = %d, want cap", got)
	}
	if got := normalizeRecentUserWindow(1); got != 2 {
		t.Fatalf("normalizeRecentUserWindow low positive = %d, want 2", got)
	}
	if got := normalizeProviderRequestTimeoutMs(999_999); got != 600_000 {
		t.Fatalf("normalizeProviderRequestTimeoutMs high = %d, want cap", got)
	}
	if got := normalizeHeaders(map[string]string{" ": "x", "X-Test": " "}); got != nil {
		t.Fatalf("normalizeHeaders empty normalized = %#v, want nil", got)
	}
	if err := validateProviderBaseURL("%zz"); err == nil || !strings.Contains(err.Error(), "invalid provider base URL") {
		t.Fatalf("validateProviderBaseURL parse err = %v, want invalid URL", err)
	}
}
