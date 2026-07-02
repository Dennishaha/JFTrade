package assistant

import (
	"errors"
	"os"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestServiceCatalogCRUDRecordsBusinessAudit(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()

	if snapshot, err := service.Snapshot(ctx); err != nil || asMap(t, snapshot)["runtimeSettings"] != nil {
		t.Fatalf("Snapshot = %#v, %v", snapshot, err)
	}
	if tools, err := service.Tools(ctx); err != nil || tools == nil {
		t.Fatalf("Tools = %#v, %v", tools, err)
	}

	backup, err := service.SaveProvider(ctx, jfadk.ProviderWriteRequest{
		ID: "backup-provider", DisplayName: "Backup Provider", BaseURL: "https://example.test/v1",
		Model: "backup-model", APIKey: "sk-backup", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	defaultProvider, err := service.SetDefaultProvider(ctx, backup.ID)
	if err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}
	if defaultProvider.ID != backup.ID || !defaultProvider.Default {
		t.Fatalf("default provider = %#v", defaultProvider)
	}

	agent, err := service.SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "service-crud-agent", Name: "Service CRUD Agent",
		Status: jfadk.AgentStatusEnabled, ProviderID: backup.ID,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	enabledAgents, err := service.ListAgents(ctx, AgentQuery{Status: strings.ToLower(jfadk.AgentStatusEnabled)})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if !assistantAgentIDs(enabledAgents)[agent.ID] {
		t.Fatalf("enabled agents missing %q: %#v", agent.ID, enabledAgents)
	}
	if err := service.DeleteProvider(ctx, backup.ID); err == nil || !strings.Contains(err.Error(), "used by agent") {
		t.Fatalf("DeleteProvider in use err = %v", err)
	}

	task, err := service.SaveTask(ctx, jfadk.TaskWriteRequest{
		ID: "service-task", Title: "Check market data", Status: "TODO", AgentID: agent.ID,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	patched, err := service.UpdateTask(ctx, task.ID, jfadk.TaskPatchRequest{
		Status: new("DONE"), ResultSummary: new("checked"),
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if patched.Status != "DONE" || patched.ResultSummary != "checked" {
		t.Fatalf("patched task = %#v", patched)
	}
	loadedTask, err := service.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loadedTask.ID != task.ID {
		t.Fatalf("loaded task = %#v", loadedTask)
	}
	taskPage, err := service.ListTasks(ctx, TaskQuery{Status: "DONE", AgentID: agent.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if taskPage.Total != 1 || len(taskPage.Items) != 1 {
		t.Fatalf("task page = %#v", taskPage)
	}
	if err := service.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := service.GetTask(ctx, task.ID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetTask deleted err = %v", err)
	}

	memory, err := service.SaveMemory(ctx, jfadk.MemoryWriteRequest{Scope: "agent", AgentID: agent.ID, Key: "Risk Rule", Value: "Use simulate before real trade"})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	memories, err := service.ListMemory(ctx, MemoryQuery{AgentID: agent.ID, Key: "risk-rule"})
	if err != nil {
		t.Fatalf("ListMemory: %v", err)
	}
	if len(memories) != 1 || memories[0].ID != memory.ID {
		t.Fatalf("memories = %#v", memories)
	}
	if err := service.DeleteMemory(ctx, memory.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if err := service.DeleteMemory(ctx, memory.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteMemory missing err = %v", err)
	}

	events, err := runtime.Store().ListAuditEvents(ctx)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	kinds := map[string]bool{}
	for _, event := range events {
		kinds[event.Kind] = true
	}
	for _, kind := range []string{"provider.saved", "provider.default_set", "agent.saved", "task.saved", "task.updated", "task.deleted", "memory.saved", "memory.deleted"} {
		if !kinds[kind] {
			t.Fatalf("audit kind %q missing from %#v", kind, kinds)
		}
	}

	if err := service.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if err := service.DeleteProvider(ctx, backup.ID); err != nil {
		t.Fatalf("DeleteProvider after agent delete: %v", err)
	}
}

func TestServiceSessionAndRunReadBoundaries(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "session-boundary-agent", Name: "Session Boundary", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	session, err := service.CreateSession(ctx, CreateSessionRequest{AgentID: agent.ID, Title: "Boundary Session"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.Title != "Boundary Session" {
		t.Fatalf("session = %#v", session)
	}
	if _, err := service.CreateSession(ctx, CreateSessionRequest{AgentID: "missing-agent"}); err == nil || !strings.Contains(err.Error(), "enabled agent") {
		t.Fatalf("CreateSession missing agent err = %v", err)
	}
	renamed, err := service.RenameSession(ctx, session.ID, "Renamed")
	if err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	if renamed.Title != "Renamed" {
		t.Fatalf("renamed session = %#v", renamed)
	}
	composer, err := service.UpdateSessionComposerState(ctx, session.ID, jfadk.SessionComposerStatePatch{
		ChatDraft: new("draft"),
	})
	if err != nil {
		t.Fatalf("UpdateSessionComposerState: %v", err)
	}
	if composer.ChatDraft != "draft" {
		t.Fatalf("composer = %#v", composer)
	}
	detail, err := service.GetSessionDetail(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if detail.Session.ID != session.ID || detail.ComposerState.ChatDraft != "draft" {
		t.Fatalf("session detail = %#v", detail)
	}
	page, err := service.ListSessions(ctx, SessionQuery{AgentID: agent.ID, Query: "renamed", Limit: 10})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != session.ID {
		t.Fatalf("session page = %#v", page)
	}

	run := jfadk.Run{ID: "run-session-boundary", SessionID: session.ID, AgentID: agent.ID, WorkMode: jfadk.WorkModeLoop, Status: jfadk.RunStatusRunning}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	loadedRun, err := service.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if loadedRun.ID != run.ID || loadedRun.Status != jfadk.RunStatusRunning {
		t.Fatalf("loaded run = %#v", loadedRun)
	}
	runs, err := service.ListRuns(ctx, RunQuery{Status: jfadk.RunStatusRunning, AgentID: agent.ID, SessionID: session.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if runs.Total != 1 || len(runs.Items) != 1 {
		t.Fatalf("runs page = %#v", runs)
	}
	updatedRun, err := service.UpdateRunObjective(ctx, run.ID, "new objective")
	if err != nil {
		t.Fatalf("UpdateRunObjective: %v", err)
	}
	if updatedRun.Objective != "new objective" {
		t.Fatalf("updated run = %#v", updatedRun)
	}
	if _, err := service.GetRun(ctx, "missing-run"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetRun missing err = %v", err)
	}

	if err := service.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := service.GetSession(ctx, session.ID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetSession deleted err = %v", err)
	}
	if _, err := service.GetRun(ctx, run.ID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetRun after session delete err = %v", err)
	}
}

func TestServiceRuntimeUnavailableCatalogAndReadWriteBoundaries(t *testing.T) {
	service := NewService(nil)
	ctx := t.Context()

	cases := []struct {
		name string
		call func() error
	}{
		{"tools", func() error { _, err := service.Tools(ctx); return err }},
		{"list tasks", func() error { _, err := service.ListTasks(ctx, TaskQuery{}); return err }},
		{"get task", func() error { _, err := service.GetTask(ctx, "task"); return err }},
		{"save task", func() error { _, err := service.SaveTask(ctx, jfadk.TaskWriteRequest{Title: "task"}); return err }},
		{"update task", func() error { _, err := service.UpdateTask(ctx, "task", jfadk.TaskPatchRequest{}); return err }},
		{"delete task", func() error { return service.DeleteTask(ctx, "task") }},
		{"list memory", func() error { _, err := service.ListMemory(ctx, MemoryQuery{}); return err }},
		{"save memory", func() error { _, err := service.SaveMemory(ctx, jfadk.MemoryWriteRequest{Key: "k"}); return err }},
		{"delete memory", func() error { return service.DeleteMemory(ctx, "memory") }},
		{"list providers", func() error { _, err := service.ListProviders(ctx); return err }},
		{"save provider", func() error {
			_, err := service.SaveProvider(ctx, jfadk.ProviderWriteRequest{ID: "provider"})
			return err
		}},
		{"set default provider", func() error { _, err := service.SetDefaultProvider(ctx, "provider"); return err }},
		{"delete provider", func() error { return service.DeleteProvider(ctx, "provider") }},
		{"test provider", func() error { _, err := service.TestProvider(ctx, "provider"); return err }},
		{"list agents", func() error { _, err := service.ListAgents(ctx, AgentQuery{}); return err }},
		{"save agent", func() error { _, err := service.SaveAgent(ctx, jfadk.AgentWriteRequest{ID: "agent"}); return err }},
		{"delete agent", func() error { return service.DeleteAgent(ctx, "agent") }},
		{"list sessions", func() error { _, err := service.ListSessions(ctx, SessionQuery{}); return err }},
		{"create session", func() error { _, err := service.CreateSession(ctx, CreateSessionRequest{AgentID: "agent"}); return err }},
		{"get session", func() error { _, err := service.GetSession(ctx, "session"); return err }},
		{"get session detail", func() error { _, err := service.GetSessionDetail(ctx, "session"); return err }},
		{"rename session", func() error { _, err := service.RenameSession(ctx, "session", "title"); return err }},
		{"update composer", func() error {
			_, err := service.UpdateSessionComposerState(ctx, "session", jfadk.SessionComposerStatePatch{})
			return err
		}},
		{"delete session", func() error { return service.DeleteSession(ctx, "session") }},
		{"get session context", func() error { _, err := service.GetSessionContext(ctx, "session"); return err }},
		{"compact session context", func() error {
			_, err := service.CompactSessionContext(ctx, "session", "balanced", "manual", "too large")
			return err
		}},
		{"chat", func() error { _, err := service.Chat(ctx, jfadk.ChatRequest{Message: "ping"}); return err }},
		{"chat stream", func() error {
			_, err := service.ChatStream(ctx, jfadk.ChatRequest{Message: "ping"}, nil)
			return err
		}},
		{"preview session", func() error { _, err := service.PreviewSession(ctx, jfadk.ChatRequest{Message: "ping"}); return err }},
		{"list runs", func() error { _, err := service.ListRuns(ctx, RunQuery{}); return err }},
		{"get run", func() error { _, err := service.GetRun(ctx, "run"); return err }},
		{"cancel run", func() error { _, err := service.CancelRun(ctx, "run"); return err }},
		{"pause goal run", func() error { _, err := service.PauseGoalRun(ctx, "run"); return err }},
		{"resume goal run", func() error { _, err := service.ResumeGoalRun(ctx, "run"); return err }},
		{"update run objective", func() error { _, err := service.UpdateRunObjective(ctx, "run", "objective"); return err }},
		{"list approvals", func() error { _, err := service.ListApprovals(ctx, ApprovalQuery{}); return err }},
		{"resolve approval", func() error { _, err := service.ResolveApproval(ctx, "approval", true); return err }},
		{"resolve approval async", func() error { _, err := service.ResolveApprovalAsync(ctx, "approval", false); return err }},
		{"list skills", func() error { _, err := service.ListSkills(ctx); return err }},
		{"install skill", func() error { _, err := service.InstallSkill(ctx, "https://example.test/SKILL.md"); return err }},
		{"delete skill", func() error { return service.DeleteSkill(ctx, "skill") }},
		{"list optimization tasks", func() error { _, err := service.ListOptimizationTasks(ctx); return err }},
		{"get optimization task", func() error { _, err := service.GetOptimizationTask(ctx, "task"); return err }},
		{"cancel optimization task", func() error { _, err := service.CancelOptimizationTask(ctx, "task"); return err }},
		{"audit", func() error { _, err := service.GetAudit(ctx, AuditQuery{}); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil || !strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("%s err = %v, want unavailable", tc.name, err)
			}
		})
	}
}

func TestServiceCloseClosesRuntimeOwnedResources(t *testing.T) {
	_, service, _ := newAssistantServiceHarness(t)
	if err := service.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("Close second call: %v", err)
	}
}

func assistantAgentIDs(agents []jfadk.Agent) map[string]bool {
	ids := make(map[string]bool, len(agents))
	for _, agent := range agents {
		ids[agent.ID] = true
	}
	return ids
}
