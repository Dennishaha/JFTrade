package adk

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newBusinessStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })
	return store
}

func TestStoreProviderLifecycleMaintainsDefaultAndSecrets(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	first, err := store.SaveProvider(ctx, ProviderWriteRequest{
		DisplayName: "OpenAI Primary",
		BaseURL:     "https://api.openai.com/v1/",
		Model:       "gpt-4.1",
		APIKey:      "sk-primary",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveProvider first: %v", err)
	}
	if !first.Default || first.ID != "openai-primary" || first.BaseURL != "https://api.openai.com/v1" || !first.HasAPIKey {
		t.Fatalf("first provider = %#v", first)
	}
	if key, ok, err := store.ProviderAPIKey(first.ID); err != nil || !ok || key != "sk-primary" {
		t.Fatalf("ProviderAPIKey = %q, %v, %v", key, ok, err)
	}
	second, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID:      "backup",
		BaseURL: "https://example.test/v1",
		Model:   "model-b",
		Enabled: true,
		APIKey:  "sk-backup",
	})
	if err != nil {
		t.Fatalf("SaveProvider second: %v", err)
	}
	if second.Default {
		t.Fatalf("second provider unexpectedly default: %#v", second)
	}

	defaultProvider, err := store.SetDefaultProvider(ctx, second.ID)
	if err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}
	if !defaultProvider.Default || defaultProvider.ID != second.ID {
		t.Fatalf("default provider = %#v", defaultProvider)
	}
	providers, err := store.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) != 2 || providers[0].ID != second.ID || !providers[0].Default || providers[1].Default {
		t.Fatalf("providers = %#v", providers)
	}
	if _, err := store.UpdateProviderCapabilities(ctx, "missing", map[string]bool{"streaming": true}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("UpdateProviderCapabilities missing err = %v", err)
	}
	updated, err := store.UpdateProviderCapabilities(ctx, first.ID, map[string]bool{"streaming": true})
	if err != nil {
		t.Fatalf("UpdateProviderCapabilities: %v", err)
	}
	if !updated.Capabilities["streaming"] {
		t.Fatalf("updated capabilities = %#v", updated.Capabilities)
	}

	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID: "uses-backup", Name: "Uses Backup", ProviderID: second.ID, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	if err := store.DeleteProvider(ctx, second.ID); err == nil || !strings.Contains(err.Error(), agent.Name) {
		t.Fatalf("DeleteProvider in use err = %v", err)
	}
	if err := store.DeleteAgent(ctx, agent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if err := store.DeleteProvider(ctx, second.ID); err != nil {
		t.Fatalf("DeleteProvider default after agent removal: %v", err)
	}
	providers, err = store.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders after delete: %v", err)
	}
	if len(providers) != 1 || providers[0].ID != first.ID || !providers[0].Default {
		t.Fatalf("providers after delete = %#v", providers)
	}
	if _, ok, err := store.ProviderAPIKey(second.ID); err != nil || ok {
		t.Fatalf("deleted provider key = ok:%v err:%v", ok, err)
	}
}

func TestStoreAgentSessionCascadeAndTaskMemoryBoundaries(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	if err := store.DeleteAgent(ctx, DefaultBuiltinAgentID); !errors.Is(err, ErrBuiltinAgentProtected) {
		t.Fatalf("DeleteAgent builtin err = %v", err)
	}
	if _, err := store.SaveAgent(ctx, AgentWriteRequest{ID: DefaultBuiltinAgentID, Status: AgentStatusDisabled}); !errors.Is(err, ErrBuiltinAgentProtected) {
		t.Fatalf("SaveAgent disabled builtin err = %v", err)
	}
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID: "research-agent", Name: "Research Agent", Status: AgentStatusEnabled,
		RecentUserWindow: -1, WorkMode: WorkModeLoop, LoopMaxIterations: 999,
	})
	if err != nil {
		t.Fatalf("SaveAgent custom: %v", err)
	}
	if agent.RecentUserWindow <= 0 || agent.LoopMaxIterations != MaxLoopIterations {
		t.Fatalf("normalized agent = %#v", agent)
	}

	session, err := store.CreateSession(ctx, agent.ID, "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.Title != "新的 ADK 会话" {
		t.Fatalf("session title = %q", session.Title)
	}
	renamed, err := store.RenameSession(ctx, session.ID, strings.Repeat("长", 90))
	if err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	if len([]rune(renamed.Title)) != 80 {
		t.Fatalf("renamed title length = %d", len([]rune(renamed.Title)))
	}
	if _, err := store.RenameSession(ctx, session.ID, " "); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("RenameSession empty err = %v", err)
	}

	run := Run{ID: "run-1", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	approval := Approval{ID: "approval-1", RunID: run.ID, AgentID: agent.ID, Status: ApprovalStatusPending}
	if err := store.SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	task, err := store.SaveTask(ctx, TaskWriteRequest{
		ID: "task-1", Title: "Inspect data", Status: "IN_PROGRESS", AgentID: agent.ID, RunID: run.ID,
		DependsOn: []string{" task-0 ", "task-0"},
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "task-0" {
		t.Fatalf("task dependsOn = %#v", task.DependsOn)
	}
	if _, err := store.SaveTask(ctx, TaskWriteRequest{ID: "self", Title: "Self", DependsOn: []string{"self"}}); err == nil || !strings.Contains(err.Error(), "depend on itself") {
		t.Fatalf("SaveTask self dependency err = %v", err)
	}
	updatedTask, err := store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("DONE"), ResultSummary: new("finished")})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updatedTask.Status != "DONE" || updatedTask.ResultSummary != "finished" {
		t.Fatalf("updated task = %#v", updatedTask)
	}
	if _, _, err := store.ListTasksPage(ctx, "BAD", "", "", 10, 0); err == nil || !strings.Contains(err.Error(), "invalid task status") {
		t.Fatalf("ListTasksPage invalid status err = %v", err)
	}

	workspaceMemory, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: " Risk Notes ", Value: " workspace risk note ", Scope: "workspace", AgentID: agent.ID})
	if err != nil {
		t.Fatalf("SaveMemory workspace: %v", err)
	}
	if workspaceMemory.AgentID != "" || workspaceMemory.Key != "risk-notes" {
		t.Fatalf("workspace memory = %#v", workspaceMemory)
	}
	agentMemory, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "Agent Notes", Value: strings.Repeat("x", 2100), Scope: "agent", AgentID: agent.ID})
	if err != nil {
		t.Fatalf("SaveMemory agent: %v", err)
	}
	if agentMemory.AgentID != agent.ID || len([]rune(agentMemory.Value)) != 2000 {
		t.Fatalf("agent memory = %#v", agentMemory)
	}
	if _, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "agent-only", Scope: "agent"}); err == nil || !strings.Contains(err.Error(), "requires agentId") {
		t.Fatalf("SaveMemory missing agent err = %v", err)
	}
	filtered, err := store.ListMemoryFiltered(ctx, "", agent.ID, "")
	if err != nil {
		t.Fatalf("ListMemoryFiltered: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("filtered memory = %#v", filtered)
	}
	if err := store.DeleteMemory(ctx, workspaceMemory.ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if err := store.DeleteMemory(ctx, workspaceMemory.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteMemory missing err = %v", err)
	}
	if _, err := store.SaveSessionContext(ctx, SessionContextState{
		SessionID:           session.ID,
		CurrentInputTokens:  120,
		ContextWindowTokens: 1000,
		Breakdown:           SessionContextBreakdown{RecentUserTokens: 40, ProtectedTailTokens: 80},
	}); err != nil {
		t.Fatalf("SaveSessionContext: %v", err)
	}
	if _, err := store.SaveHandoffSegment(ctx, HandoffSegment{
		SessionID:       session.ID,
		Sequence:        1,
		StartEventIndex: 0,
		EndEventIndex:   4,
		Summary:         "compressed research context",
		Mode:            "normal",
		Active:          true,
	}); err != nil {
		t.Fatalf("SaveHandoffSegment: %v", err)
	}
	if _, err := store.SaveSessionNotice(ctx, TimelineEntry{
		SessionID: session.ID,
		RunID:     run.ID,
		Kind:      TimelineKindContextNotice,
		Status:    TimelineStatusFinal,
		Text:      "context compacted",
	}); err != nil {
		t.Fatalf("SaveSessionNotice: %v", err)
	}
	composerTouched := true
	if _, err := store.SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{
		ChatDraft:            new("review risk before order"),
		GoalObjectiveDraft:   new("inspect fills"),
		GoalObjectiveTouched: &composerTouched,
	}); err != nil {
		t.Fatalf("SaveSessionComposerState: %v", err)
	}

	if err := store.DeleteSession(ctx, session.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, ok, err := store.Run(ctx, run.ID); err != nil || ok {
		t.Fatalf("run after session delete ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Approval(ctx, approval.ID); err != nil || ok {
		t.Fatalf("approval after session delete ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.Task(ctx, task.ID); err != nil || ok {
		t.Fatalf("task after session delete ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.SessionContext(ctx, session.ID); err != nil || ok {
		t.Fatalf("session context after session delete ok=%v err=%v", ok, err)
	}
	if segments, err := store.HandoffSegments(ctx, session.ID, false); err != nil || len(segments) != 0 {
		t.Fatalf("handoff segments after session delete = %+v err=%v", segments, err)
	}
	if notices, err := store.SessionNotices(ctx, session.ID); err != nil || len(notices) != 0 {
		t.Fatalf("notices after session delete = %+v err=%v", notices, err)
	}
	if composer, ok, err := store.SessionComposerState(ctx, session.ID); err != nil || ok || composer.ChatDraft != "" {
		t.Fatalf("composer after session delete = %+v ok=%v err=%v", composer, ok, err)
	}
	if err := store.DeleteSession(ctx, " "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteSession empty err = %v", err)
	}
}

func TestStoreRunApprovalSkillAndOptimizationBusinessQueries(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	agent, err := store.EnsureAgent(ctx, AgentWriteRequest{ID: "query-agent", Name: "Query Agent", Status: AgentStatusEnabled})
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}
	existingAgent, err := store.EnsureAgent(ctx, AgentWriteRequest{ID: "query-agent", Name: "Changed", Status: AgentStatusDisabled})
	if err != nil {
		t.Fatalf("EnsureAgent existing: %v", err)
	}
	if existingAgent.ID != agent.ID || existingAgent.Name != agent.Name {
		t.Fatalf("EnsureAgent existing = %#v, want original %#v", existingAgent, agent)
	}

	alpha, err := store.CreateSession(ctx, agent.ID, "Alpha research")
	if err != nil {
		t.Fatalf("CreateSession alpha: %v", err)
	}
	beta, err := store.CreateSession(ctx, agent.ID, "Beta execution")
	if err != nil {
		t.Fatalf("CreateSession beta: %v", err)
	}
	sessions, total, err := store.ListSessionsPage(ctx, agent.ID, "alpha", 10, 0)
	if err != nil {
		t.Fatalf("ListSessionsPage: %v", err)
	}
	if total != 1 || len(sessions) != 1 || sessions[0].ID != alpha.ID {
		t.Fatalf("filtered sessions total=%d sessions=%#v", total, sessions)
	}

	running := Run{ID: "run-running", SessionID: alpha.ID, AgentID: agent.ID, Status: RunStatusRunning, WorkMode: WorkModeLoop}
	completed := Run{ID: "run-completed", SessionID: beta.ID, AgentID: agent.ID, Status: RunStatusCompleted}
	if err := store.SaveRun(ctx, running); err != nil {
		t.Fatalf("SaveRun running: %v", err)
	}
	if err := store.SaveRun(ctx, completed); err != nil {
		t.Fatalf("SaveRun completed: %v", err)
	}
	runs, total, err := store.ListRunsPage(ctx, RunStatusRunning, agent.ID, alpha.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListRunsPage: %v", err)
	}
	if total != 1 || len(runs) != 1 || runs[0].ID != running.ID {
		t.Fatalf("filtered runs total=%d runs=%#v", total, runs)
	}

	firstApproval := Approval{ID: "approval-confirm-1", RunID: running.ID, AgentID: agent.ID, Status: ApprovalStatusPending, ConfirmationCallID: "call-1"}
	savedApproval, created, err := store.SaveApprovalIfConfirmationAbsent(ctx, firstApproval)
	if err != nil || !created || savedApproval.ID != firstApproval.ID {
		t.Fatalf("SaveApprovalIfConfirmationAbsent first = %#v created=%v err=%v", savedApproval, created, err)
	}
	duplicateApproval, created, err := store.SaveApprovalIfConfirmationAbsent(ctx, Approval{ID: "approval-confirm-2", RunID: running.ID, AgentID: agent.ID, Status: ApprovalStatusPending, ConfirmationCallID: " call-1 "})
	if err != nil || created || duplicateApproval.ID != firstApproval.ID {
		t.Fatalf("SaveApprovalIfConfirmationAbsent duplicate = %#v created=%v err=%v", duplicateApproval, created, err)
	}
	if _, ok, err := store.ApprovalByConfirmationCallID(ctx, "call-1"); err != nil || !ok {
		t.Fatalf("ApprovalByConfirmationCallID ok=%v err=%v", ok, err)
	}

	if _, resolved, err := store.ResolvePendingApproval(ctx, firstApproval.ID, ApprovalStatusApproved); err != nil || !resolved {
		t.Fatalf("ResolvePendingApproval pending resolved=%v err=%v", resolved, err)
	}
	if _, resolved, err := store.ResolvePendingApproval(ctx, firstApproval.ID, ApprovalStatusDenied); err != nil || resolved {
		t.Fatalf("ResolvePendingApproval already resolved=%v err=%v", resolved, err)
	}
	if _, ok, err := store.ResolvePendingApproval(ctx, "missing", ApprovalStatusDenied); err != nil || ok {
		t.Fatalf("ResolvePendingApproval missing ok=%v err=%v", ok, err)
	}

	pending := Approval{ID: "approval-pending", RunID: running.ID, AgentID: agent.ID, Status: ApprovalStatusPending}
	if err := store.SaveApproval(ctx, pending); err != nil {
		t.Fatalf("SaveApproval pending: %v", err)
	}
	if err := store.SaveRunAndDenyPendingApprovals(ctx, Run{ID: running.ID, SessionID: alpha.ID, AgentID: agent.ID, Status: RunStatusFailed}); err != nil {
		t.Fatalf("SaveRunAndDenyPendingApprovals: %v", err)
	}
	denied, ok, err := store.Approval(ctx, pending.ID)
	if err != nil || !ok || denied.Status != ApprovalStatusDenied {
		t.Fatalf("pending approval after run save = %#v ok=%v err=%v", denied, ok, err)
	}

	approvals, total, err := store.ListApprovalsPage(ctx, ApprovalStatusDenied, agent.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListApprovalsPage: %v", err)
	}
	if total != 1 || len(approvals) != 1 || approvals[0].ID != pending.ID {
		t.Fatalf("filtered approvals total=%d approvals=%#v", total, approvals)
	}

	customSkill, err := store.SaveSkill(ctx, Skill{DisplayName: "Custom Skill", Description: "demo", Source: "local", Enabled: true})
	if err != nil {
		t.Fatalf("SaveSkill custom: %v", err)
	}
	if customSkill.ID != "custom-skill" || customSkill.CreatedAt == "" {
		t.Fatalf("custom skill = %#v", customSkill)
	}
	updatedSkill, err := store.SaveSkill(ctx, Skill{ID: customSkill.ID, DisplayName: "Custom Skill Updated", Source: "local", Enabled: false})
	if err != nil {
		t.Fatalf("SaveSkill update: %v", err)
	}
	if updatedSkill.CreatedAt != customSkill.CreatedAt || updatedSkill.UpdatedAt == "" {
		t.Fatalf("updated skill = %#v original=%#v", updatedSkill, customSkill)
	}
	skill, ok, err := store.Skill(ctx, customSkill.ID)
	if err != nil || !ok || skill.DisplayName != "Custom Skill Updated" {
		t.Fatalf("Skill lookup = %#v ok=%v err=%v", skill, ok, err)
	}
	if err := store.DeleteSkill(ctx, customSkill.ID); err != nil {
		t.Fatalf("DeleteSkill custom: %v", err)
	}
	if _, ok, err := store.Skill(ctx, customSkill.ID); err != nil || ok {
		t.Fatalf("Skill after delete ok=%v err=%v", ok, err)
	}

	task, err := store.SaveOptimizationTask(ctx, OptimizationTask{ID: "opt-1", Status: "RUNNING", Objective: "Improve Sharpe", Runs: []OptimizationRunRef{{DefinitionID: "def-1", RunID: running.ID}}})
	if err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}
	task.Status = "COMPLETED"
	updatedTask, err := store.SaveOptimizationTask(ctx, task)
	if err != nil {
		t.Fatalf("SaveOptimizationTask update: %v", err)
	}
	if updatedTask.CreatedAt != task.CreatedAt || updatedTask.Status != "COMPLETED" {
		t.Fatalf("updated optimization task = %#v original=%#v", updatedTask, task)
	}
	optimizationTasks, err := store.ListOptimizationTasks(ctx)
	if err != nil {
		t.Fatalf("ListOptimizationTasks: %v", err)
	}
	if len(optimizationTasks) != 1 || optimizationTasks[0].ID != task.ID {
		t.Fatalf("optimization tasks = %#v", optimizationTasks)
	}
}

func TestStoreOperationalBoundaryErrorsAndDefaults(t *testing.T) {
	ctx := t.Context()

	var nilStore *Store
	nilStore.SetSessionService(nil)
	if got := nilStore.SkillsPath(); got != "" {
		t.Fatalf("nil SkillsPath = %q", got)
	}
	if err := nilStore.Close(); err != nil {
		t.Fatalf("nil Close = %v", err)
	}
	if _, err := NewStore("", filepath.Join(t.TempDir(), "secrets.json"), filepath.Join(t.TempDir(), "skills")); err == nil || !strings.Contains(err.Error(), "db path is required") {
		t.Fatalf("NewStore empty path err = %v", err)
	}

	store := newBusinessStore(t)
	if got := store.SkillsPath(); got == "" || !strings.HasSuffix(got, "skills") {
		t.Fatalf("SkillsPath = %q", got)
	}
	if _, ok, err := store.DefaultProvider(ctx); err != nil || ok {
		t.Fatalf("DefaultProvider with no provider ok=%v err=%v", ok, err)
	}
	if _, err := store.SetDefaultProvider(ctx, "missing-provider"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SetDefaultProvider missing err = %v", err)
	}
	if err := store.DeleteProvider(ctx, "missing-provider"); err != nil {
		t.Fatalf("DeleteProvider missing should be idempotent, got %v", err)
	}
	if _, err := store.SaveProvider(ctx, ProviderWriteRequest{ID: "bad-scheme", BaseURL: "file:///tmp/model"}); err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("SaveProvider bad scheme err = %v", err)
	}
	if _, err := store.SaveProvider(ctx, ProviderWriteRequest{ID: "bad-header", BaseURL: "https://provider.example/v1", DefaultHeaders: map[string]string{"Host": "evil.example"}}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("SaveProvider bad header err = %v", err)
	}

	defaultAgent, err := store.DefaultAgent(ctx)
	if err != nil {
		t.Fatalf("DefaultAgent: %v", err)
	}
	if defaultAgent.ID != DefaultBuiltinAgentID || defaultAgent.Status != AgentStatusEnabled {
		t.Fatalf("DefaultAgent = %#v", defaultAgent)
	}
	agents, err := store.ListAllAgents(ctx)
	if err != nil {
		t.Fatalf("ListAllAgents: %v", err)
	}
	if len(agents) == 0 || agents[0].ID != DefaultBuiltinAgentID {
		t.Fatalf("ListAllAgents = %#v", agents)
	}

	if _, err := store.SaveTask(ctx, TaskWriteRequest{Title: "   "}); err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("SaveTask empty title err = %v", err)
	}
	if _, err := store.UpdateTask(ctx, "missing-task", TaskPatchRequest{Title: new("x")}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("UpdateTask missing err = %v", err)
	}
	task, err := store.SaveTask(ctx, TaskWriteRequest{ID: "patch-task", Title: "Patch Task", DependsOn: []string{"before"}})
	if err != nil {
		t.Fatalf("SaveTask patch-task: %v", err)
	}
	if _, err := store.UpdateTask(ctx, task.ID, TaskPatchRequest{Title: new(" ")}); err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("UpdateTask empty title err = %v", err)
	}
	if _, err := store.UpdateTask(ctx, task.ID, TaskPatchRequest{DependsOn: []string{task.ID}}); err == nil || !strings.Contains(err.Error(), "depend on itself") {
		t.Fatalf("UpdateTask self dependency err = %v", err)
	}
	order := 7
	fullyPatched, err := store.UpdateTask(ctx, task.ID, TaskPatchRequest{
		Title:           new("Patched Task"),
		Description:     new("  normalize me  "),
		Status:          new("blocked"),
		AgentID:         new(" agent-patch "),
		RunID:           new(" run-patch "),
		DependsOn:       []string{" z ", "a", "z"},
		Order:           &order,
		ModeHint:        new(" loop "),
		AgentRole:       new(" reviewer "),
		PlannerStepID:   new(" step-1 "),
		PlanSource:      new(" planner "),
		WorkflowMode:    new(" sequential "),
		Objective:       new(" hedge risk "),
		Message:         new(" inspect exposure "),
		Executor:        new("agent"),
		ChildProviderID: new(" provider-child "),
		ChildModel:      new(" child-model "),
		ResultSummary:   new(" blocked by missing data "),
		PlannerWarnings: []string{" stale ", "", "stale", "risk"},
	})
	if err != nil {
		t.Fatalf("UpdateTask full patch: %v", err)
	}
	if fullyPatched.Title != "Patched Task" || fullyPatched.Description != "normalize me" || fullyPatched.Status != "BLOCKED" ||
		fullyPatched.AgentID != "agent-patch" || fullyPatched.RunID != "run-patch" || fullyPatched.Order != order ||
		fullyPatched.ModeHint != "loop" || fullyPatched.AgentRole != "reviewer" || fullyPatched.PlannerStepID != "step-1" ||
		fullyPatched.PlanSource != "planner" || fullyPatched.WorkflowMode != "sequential" || fullyPatched.Objective != "hedge risk" ||
		fullyPatched.Message != "inspect exposure" || fullyPatched.Executor != "agent" || fullyPatched.ChildProviderID != "provider-child" ||
		fullyPatched.ChildModel != "child-model" || fullyPatched.ResultSummary != "blocked by missing data" {
		t.Fatalf("fully patched task = %#v", fullyPatched)
	}
	if len(fullyPatched.DependsOn) != 2 || fullyPatched.DependsOn[0] != "a" || fullyPatched.DependsOn[1] != "z" {
		t.Fatalf("fully patched dependsOn = %#v", fullyPatched.DependsOn)
	}
	if len(fullyPatched.PlannerWarnings) != 2 || fullyPatched.PlannerWarnings[0] != "risk" || fullyPatched.PlannerWarnings[1] != "stale" {
		t.Fatalf("fully patched planner warnings = %#v", fullyPatched.PlannerWarnings)
	}
	if err := store.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask success: %v", err)
	}
	if err := store.DeleteTask(ctx, "missing-task"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteTask missing err = %v", err)
	}

	firstMemory, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "Project Bias", Value: "first", Scope: "workspace"})
	if err != nil {
		t.Fatalf("SaveMemory first: %v", err)
	}
	secondMemory, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: " project-bias ", Value: "second", Scope: "workspace"})
	if err != nil {
		t.Fatalf("SaveMemory upsert: %v", err)
	}
	if secondMemory.ID != firstMemory.ID || secondMemory.CreatedAt != firstMemory.CreatedAt || secondMemory.Value != "second" {
		t.Fatalf("upserted memory = %#v first=%#v", secondMemory, firstMemory)
	}
	if _, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "x", Scope: "team"}); err == nil || !strings.Contains(err.Error(), "workspace or agent") {
		t.Fatalf("SaveMemory bad scope err = %v", err)
	}

	if err := store.AddAuditEvent(ctx, AuditEvent{Kind: "provider.updated", SubjectID: "p1", Detail: "updated", Metadata: map[string]any{"field": "baseUrl"}}); err != nil {
		t.Fatalf("AddAuditEvent first: %v", err)
	}
	if err := store.AddAuditEvent(ctx, AuditEvent{Kind: "agent.saved", SubjectID: defaultAgent.ID, Detail: "saved"}); err != nil {
		t.Fatalf("AddAuditEvent second: %v", err)
	}
	audit, err := store.ListAuditEvents(ctx)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(audit) != 2 {
		t.Fatalf("audit events = %#v", audit)
	}
	foundProviderAudit := false
	for _, event := range audit {
		if event.Kind == "provider.updated" && event.SubjectID == "p1" && event.ID != "" {
			foundProviderAudit = true
		}
	}
	if !foundProviderAudit {
		t.Fatalf("provider audit not found: %#v", audit)
	}
}

func TestStoreDefaultSelectionSecretsAndListBoundaries(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)

	alpha, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "alpha", DisplayName: "Alpha", BaseURL: "https://alpha.example/v1", Model: "model-a", APIKey: "sk-alpha", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider alpha: %v", err)
	}
	beta, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "beta", DisplayName: "Beta", BaseURL: "https://beta.example/v1", Model: "model-b", APIKey: "sk-beta", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider beta: %v", err)
	}
	if !alpha.Default || beta.Default {
		t.Fatalf("initial default providers alpha=%#v beta=%#v", alpha, beta)
	}
	if _, err := store.SetDefaultProvider(ctx, beta.ID); err != nil {
		t.Fatalf("SetDefaultProvider beta: %v", err)
	}
	if err := store.DeleteProvider(ctx, beta.ID); err != nil {
		t.Fatalf("DeleteProvider beta: %v", err)
	}
	providers, err := store.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders after deleting default: %v", err)
	}
	if len(providers) != 1 || providers[0].ID != alpha.ID || !providers[0].Default {
		t.Fatalf("providers after deleting default = %#v", providers)
	}
	if _, ok, err := store.ProviderAPIKey(beta.ID); err != nil || ok {
		t.Fatalf("deleted beta secret ok=%v err=%v", ok, err)
	}

	updatedAlpha, err := store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "alpha", DisplayName: "Alpha Updated", BaseURL: "https://alpha.example/v2", Model: "model-a2", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider alpha update: %v", err)
	}
	if updatedAlpha.CreatedAt != alpha.CreatedAt || !updatedAlpha.Default || updatedAlpha.RequestTimeoutMs == 0 {
		t.Fatalf("updated alpha = %#v original=%#v", updatedAlpha, alpha)
	}

	rawProviders := []Provider{
		{ID: "p1", DisplayName: "P1", BaseURL: "https://p1.example/v1", Default: false},
		{ID: "p2", DisplayName: "P2", BaseURL: "https://p2.example/v1", Default: true},
		{ID: "p3", DisplayName: "P3", BaseURL: "https://p3.example/v1", Default: true},
	}
	if !normalizeDefaultProviderSelection(rawProviders) {
		t.Fatal("normalizeDefaultProviderSelection with duplicate defaults changed=false, want true")
	}
	if !rawProviders[1].Default || rawProviders[2].Default {
		t.Fatalf("duplicate default normalization = %#v", rawProviders)
	}
	noDefaultProviders := []Provider{{ID: "p1", DisplayName: "P1", BaseURL: "https://p1.example/v1"}}
	if !normalizeDefaultProviderSelection(noDefaultProviders) || !noDefaultProviders[0].Default {
		t.Fatalf("missing default normalization = %#v", noDefaultProviders)
	}
	if normalizeDefaultProviderSelection(nil) {
		t.Fatal("empty provider normalization changed=true, want false")
	}
	if err := currentErrOrNotFound(nil, false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("currentErrOrNotFound missing = %v", err)
	}
	sentinel := errors.New("database unavailable")
	if err := currentErrOrNotFound(sentinel, false); !errors.Is(err, sentinel) {
		t.Fatalf("currentErrOrNotFound error = %v", err)
	}
	if err := currentErrOrNotFound(nil, true); err != nil {
		t.Fatalf("currentErrOrNotFound ok = %v", err)
	}

	defaultAgent, err := store.DefaultAgent(ctx)
	if err != nil {
		t.Fatalf("DefaultAgent: %v", err)
	}
	customAgent, err := store.SaveAgent(ctx, AgentWriteRequest{ID: "z-custom", Name: "Z Custom", Status: AgentStatusEnabled})
	if err != nil {
		t.Fatalf("SaveAgent custom: %v", err)
	}
	agents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) < 2 || agents[0].ID != defaultAgent.ID {
		t.Fatalf("ListAgents order = %#v", agents)
	}
	if err := store.DeleteAgent(ctx, customAgent.ID); err != nil {
		t.Fatalf("DeleteAgent custom: %v", err)
	}
	activeAgents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents active after delete: %v", err)
	}
	for _, agent := range activeAgents {
		if agent.ID == customAgent.ID {
			t.Fatalf("deleted agent still active: %#v", activeAgents)
		}
	}
	allAgents, err := store.ListAllAgents(ctx)
	if err != nil {
		t.Fatalf("ListAllAgents after delete: %v", err)
	}
	foundDeleted := false
	for _, agent := range allAgents {
		if agent.ID == customAgent.ID && agent.DeletedAt != nil {
			foundDeleted = true
		}
	}
	if !foundDeleted {
		t.Fatalf("deleted agent missing from ListAllAgents: %#v", allAgents)
	}

	session, err := store.CreateSession(ctx, defaultAgent.ID, "Research A")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	runA := Run{ID: "run-a", SessionID: session.ID, AgentID: defaultAgent.ID, Status: RunStatusRunning}
	runB := Run{ID: "run-b", SessionID: session.ID, AgentID: defaultAgent.ID, Status: RunStatusCompleted}
	if err := store.SaveRun(ctx, runA); err != nil {
		t.Fatalf("SaveRun run-a: %v", err)
	}
	if err := store.SaveRun(ctx, runB); err != nil {
		t.Fatalf("SaveRun run-b: %v", err)
	}
	allRuns, err := store.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(allRuns) != 2 {
		t.Fatalf("ListRuns = %#v", allRuns)
	}
	pagedRuns, total, err := store.ListRunsPage(ctx, "", defaultAgent.ID, session.ID, 1, 1)
	if err != nil {
		t.Fatalf("ListRunsPage offset: %v", err)
	}
	if total != 2 || len(pagedRuns) != 1 {
		t.Fatalf("ListRunsPage offset total=%d runs=%#v", total, pagedRuns)
	}

	approvalA := Approval{ID: "approval-a", RunID: runA.ID, AgentID: defaultAgent.ID, Status: ApprovalStatusPending}
	approvalB := Approval{ID: "approval-b", RunID: runB.ID, AgentID: defaultAgent.ID, Status: ApprovalStatusDenied}
	if err := store.SaveApproval(ctx, approvalA); err != nil {
		t.Fatalf("SaveApproval approval-a: %v", err)
	}
	if err := store.SaveApproval(ctx, approvalB); err != nil {
		t.Fatalf("SaveApproval approval-b: %v", err)
	}
	approvals, err := store.ListApprovals(ctx)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 2 {
		t.Fatalf("ListApprovals = %#v", approvals)
	}
	if _, ok, err := store.ApprovalByConfirmationCallID(ctx, " "); err != nil || ok {
		t.Fatalf("blank ApprovalByConfirmationCallID ok=%v err=%v", ok, err)
	}

	workspaceMemory, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "Workspace Note", Value: "workspace", Scope: "workspace"})
	if err != nil {
		t.Fatalf("SaveMemory workspace: %v", err)
	}
	agentMemory, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "Agent Note", Value: "agent", Scope: "agent", AgentID: defaultAgent.ID})
	if err != nil {
		t.Fatalf("SaveMemory agent: %v", err)
	}
	if got, ok, err := store.Memory(ctx, workspaceMemory.ID); err != nil || !ok || got.Value != "workspace" {
		t.Fatalf("Memory workspace = %#v ok=%v err=%v", got, ok, err)
	}
	agentScoped, err := store.ListMemory(ctx, defaultAgent.ID)
	if err != nil {
		t.Fatalf("ListMemory agent scoped: %v", err)
	}
	if len(agentScoped) != 2 {
		t.Fatalf("ListMemory agent scoped = %#v", agentScoped)
	}
	workspaceOnly, err := store.ListMemoryFiltered(ctx, "workspace", defaultAgent.ID, workspaceMemory.Key)
	if err != nil {
		t.Fatalf("ListMemoryFiltered workspace/key: %v", err)
	}
	if len(workspaceOnly) != 1 || workspaceOnly[0].ID != workspaceMemory.ID {
		t.Fatalf("workspace filtered memory = %#v", workspaceOnly)
	}
	agentOnly, err := store.ListMemoryFiltered(ctx, "agent", defaultAgent.ID, agentMemory.Key)
	if err != nil {
		t.Fatalf("ListMemoryFiltered agent/key: %v", err)
	}
	if len(agentOnly) != 1 || agentOnly[0].ID != agentMemory.ID {
		t.Fatalf("agent filtered memory = %#v", agentOnly)
	}
	if _, err := store.SaveMemory(ctx, MemoryWriteRequest{Key: "Missing Agent", Scope: "agent", AgentID: "missing-agent"}); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("SaveMemory missing agent err = %v", err)
	}

	builtinSkill, ok, err := store.Skill(ctx, "jftrade-market")
	if err != nil || !ok {
		t.Fatalf("builtin Skill lookup ok=%v err=%v", ok, err)
	}
	if err := store.DeleteSkill(ctx, builtinSkill.ID); err != nil {
		t.Fatalf("DeleteSkill builtin should be ignored: %v", err)
	}
	if _, ok, err := store.Skill(ctx, builtinSkill.ID); err != nil || !ok {
		t.Fatalf("builtin Skill after delete ok=%v err=%v", ok, err)
	}
	customSkill, err := store.SaveSkill(ctx, Skill{ID: "zz-custom", DisplayName: "ZZ Custom", Source: "local", Enabled: true})
	if err != nil {
		t.Fatalf("SaveSkill custom: %v", err)
	}
	skills, err := store.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) < 2 || !skills[0].Builtin {
		t.Fatalf("ListSkills builtin ordering = %#v", skills)
	}
	if customSkill.CreatedAt == "" {
		t.Fatalf("custom skill missing CreatedAt: %#v", customSkill)
	}

	generatedOptimization, err := store.SaveOptimizationTask(ctx, OptimizationTask{Status: "QUEUED", Objective: "find robust params"})
	if err != nil {
		t.Fatalf("SaveOptimizationTask generated id: %v", err)
	}
	if !strings.HasPrefix(generatedOptimization.ID, "opt-") {
		t.Fatalf("generated optimization id = %q", generatedOptimization.ID)
	}
	if _, ok, err := store.OptimizationTask(ctx, "missing-opt"); err != nil || ok {
		t.Fatalf("missing OptimizationTask ok=%v err=%v", ok, err)
	}
}
