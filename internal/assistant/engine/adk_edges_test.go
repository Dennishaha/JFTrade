package adk

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adkmemory "google.golang.org/adk/v2/memory"
	adksession "google.golang.org/adk/v2/session"
)

type invalidArtifactPathSessionService struct {
	adksession.Service
	path string
}

func (s invalidArtifactPathSessionService) DatabasePath() string {
	return s.path
}

func TestGoogleADKMemoryServiceBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if service := newGoogleADKMemoryService(nil); service != nil {
		t.Fatalf("newGoogleADKMemoryService(nil) = %#v, want nil", service)
	}
	service := &googleADKMemoryService{}
	if err := service.AddSessionToMemory(ctx, nil); err != nil {
		t.Fatalf("AddSessionToMemory: %v", err)
	}
	response, err := service.SearchMemory(ctx, nil)
	if err != nil {
		t.Fatalf("nil SearchMemory: %v", err)
	}
	if response == nil || len(response.Memories) != 0 {
		t.Fatalf("nil SearchMemory response = %+v, want empty", response)
	}
	runtime := newTestRuntime(t)
	if !googleADKMemoryMatches(MemoryEntry{Key: "Risk", Value: "small", Scope: "agent"}, "risk missing") {
		t.Fatal("memory query should match any token")
	}
	if googleADKMemoryMatches(MemoryEntry{Key: "Risk", Value: "small", Scope: "agent"}, "macro") {
		t.Fatal("memory query matched unrelated token")
	}
	if got := googleADKAppName(""); got != "jftrade-default" {
		t.Fatalf("googleADKAppName(empty) = %q", got)
	}
	if got := googleADKAgentIDFromAppName("  "); got != "" {
		t.Fatalf("blank app name agent id = %q", got)
	}
	jftradeCheckTestError(t, runtime.Store().Close())
	_, err = runtime.memoryService.SearchMemory(ctx, &adkmemory.SearchRequest{AppName: "jftrade-default"})
	if err == nil {
		t.Fatal("SearchMemory on closed store err = nil, want error")
	}
}

func TestModelCatalogToolBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	var nilRuntime *Runtime
	nilRuntime.registerModelCatalogTool()
	if _, err := nilRuntime.modelsListTool(ctx, nil); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil runtime modelsListTool err = %v", err)
	}
	(&Runtime{}).registerModelCatalogTool()
	if got := capabilityNames(map[string]bool{}); got != "" {
		t.Fatalf("empty capabilityNames = %q", got)
	}
	if got := toolBoolValue(nil, "callableOnly", false); got {
		t.Fatal("nil bool value should return false default")
	}
	for _, input := range []map[string]any{
		{"value": false},
		{"value": "false"},
		{"value": "0"},
		{"value": "no"},
		{"value": "n"},
	} {
		if got := toolBoolValue(input, "value", true); got {
			t.Fatalf("toolBoolValue(%#v) = true, want false", input)
		}
	}
	if got := toolBoolValue(map[string]any{"value": "maybe"}, "value", true); !got {
		t.Fatal("unknown bool string should keep true default")
	}

	runtime := newTestRuntime(t)
	raw, err := runtime.modelsListTool(ctx, map[string]any{"query": "does-not-match", "limit": 0})
	if err != nil {
		t.Fatalf("modelsListTool unmatched query: %v", err)
	}
	payload := raw.(map[string]any)
	if payload["totalReturned"] != 0 {
		t.Fatalf("unmatched models payload = %#v, want empty", payload)
	}
	jftradeCheckTestError(t, runtime.Store().Close())
	if _, err := runtime.modelsListTool(ctx, nil); err == nil {
		t.Fatal("modelsListTool on closed store err = nil, want error")
	}
}

func TestContextCompactionNoticeBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	if notice := runtime.updateContextCompactionNotice(ctx, TimelineEntry{}, TimelineStatusFinal, "done"); notice.ID != "" {
		t.Fatalf("empty notice update = %+v, want empty", notice)
	}
	if notice := (*Runtime)(nil).saveContextCompactionNotice(ctx, TimelineEntry{SessionID: "session"}); notice.ID != "" {
		t.Fatalf("nil runtime save notice = %+v, want empty", notice)
	}
	dir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed notice: %v", err)
	}
	jftradeCheckTestError(t, closedStore.Close())
	if notice := (&Runtime{store: closedStore}).saveContextCompactionNotice(ctx, TimelineEntry{SessionID: "session"}); notice.ID != "" {
		t.Fatalf("closed store save notice = %+v, want empty", notice)
	}
	if err := emitContextCompactionNotice(nil, TimelineEntry{ID: "notice"}); err != nil {
		t.Fatalf("nil delta notice err = %v", err)
	}
	if err := emitContextCompactionNotice(func(ChatDelta) error {
		t.Fatal("empty notice id should not emit")
		return nil
	}, TimelineEntry{}); err != nil {
		t.Fatalf("empty id notice err = %v", err)
	}
	session := mustCreateSession(t, runtime, testProviderID, "Context notice")
	notice := runtime.createContextCompactionNotice(ctx, session.ID)
	if notice.ID == "" || notice.Text != contextCompactionStartedText || notice.Status != TimelineStatusStreaming {
		t.Fatalf("created compaction notice = %+v", notice)
	}
	updated := runtime.updateContextCompactionNotice(ctx, notice, TimelineStatusFinal, contextCompactionDoneText)
	if updated.ID != notice.ID || updated.Status != TimelineStatusFinal || updated.Text != contextCompactionDoneText {
		t.Fatalf("updated compaction notice = %+v", updated)
	}
	var emitted *TimelineEntry
	if err := emitContextCompactionNotice(func(delta ChatDelta) error {
		emitted = delta.Timeline
		return errors.New("emit failed")
	}, updated); err == nil || err.Error() != "emit failed" {
		t.Fatalf("emit error = %v, want emit failed", err)
	}
	if emitted == nil || emitted.ID != updated.ID {
		t.Fatalf("emitted notice = %+v", emitted)
	}
}

func TestPauseGuardBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	run := Run{ID: "chat-run", Status: RunStatusRunning, WorkMode: WorkModeChat}
	if saved, err := (*Runtime)(nil).saveRunPreservingUserGoalPause(ctx, run); err != nil || saved.ID != run.ID {
		t.Fatalf("nil runtime save = %+v/%v", saved, err)
	}
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, testProviderID, "Pause guard")
	if saved, err := runtime.saveRunPreservingUserGoalPause(ctx, Run{
		ID: "chat-save", SessionID: session.ID, AgentID: testProviderID, Status: RunStatusRunning, WorkMode: WorkModeChat,
	}); err != nil || saved.ID != "chat-save" {
		t.Fatalf("non-loop save = %+v/%v", saved, err)
	}
	pausedAt := nowString()
	latest := Run{
		ID: "goal", Status: RunStatusPaused, WorkMode: WorkModeLoop, PauseRequestedAt: &pausedAt,
		PausedAt: &pausedAt, PausedReason: "user", ResumeState: "user_paused", Message: "paused by user",
	}
	if got := preserveUserGoalPauseLifecycle(latest, Run{ID: "other", Status: RunStatusRunning, WorkMode: WorkModeLoop}); got.ID != "other" || got.Status != RunStatusRunning {
		t.Fatalf("different run preserved = %+v", got)
	}
	if got := preserveUserGoalPauseLifecycle(latest, Run{ID: "goal", Status: RunStatusRunning, WorkMode: WorkModeChat}); got.Status != RunStatusRunning {
		t.Fatalf("non-loop candidate preserved = %+v", got)
	}
	if got := preserveUserGoalPauseLifecycle(latest, Run{ID: "goal", Status: RunStatusRunning, WorkMode: WorkModeLoop, ResumeState: "user_resuming"}); got.Status != RunStatusRunning {
		t.Fatalf("resuming candidate = %+v, want running", got)
	}
	cancelled := preserveUserGoalPauseLifecycle(latest, Run{ID: "goal", Status: RunStatusCancelled, WorkMode: WorkModeLoop})
	if cancelled.Status != RunStatusCancelled {
		t.Fatalf("cancelled candidate = %+v, want cancelled", cancelled)
	}
	requested := preserveUserGoalPauseLifecycle(
		Run{ID: "goal", Status: RunStatusRunning, WorkMode: WorkModeLoop, PauseRequestedAt: &pausedAt, ResumeState: ""},
		Run{ID: "goal", Status: RunStatusRunning, WorkMode: WorkModeLoop},
	)
	if requested.PauseRequestedAt == nil || requested.ResumeState != "user_pause_requested" {
		t.Fatalf("pause requested candidate = %+v", requested)
	}
	terminal := preserveUserGoalPauseLifecycle(
		Run{ID: "goal", Status: RunStatusRunning, WorkMode: WorkModeLoop, PauseRequestedAt: &pausedAt},
		Run{ID: "goal", Status: RunStatusCompleted, WorkMode: WorkModeLoop},
	)
	if terminal.PauseRequestedAt != nil {
		t.Fatalf("terminal candidate inherited pause request: %+v", terminal)
	}
	if isRootLoopGoalRun(Run{ID: "child", ParentRunID: "parent", WorkMode: WorkModeLoop}) {
		t.Fatal("child loop run should not be root goal run")
	}
}

func TestSchemaConversionReportsMarshalError(t *testing.T) {
	if schema, err := googleADKJSONSchemaFromMap(map[string]any{"bad": func() {}}); err == nil || schema != nil || !strings.Contains(err.Error(), "encode") {
		t.Fatalf("schema marshal error = schema:%+v err:%v", schema, err)
	}
}

func TestSQLiteGormPoolBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	store := newBusinessStore(t)
	pool := newSQLiteGormPool(store.db)
	stmt, err := pool.PrepareContext(ctx, `SELECT 1`)
	if err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
	jftradeCheckTestError(t, stmt.Close())
	txPool, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	tx := txPool.(*sqliteGormTx)
	jftradeCheckTestError(t, tx.Rollback())

	dir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	closedPool := newSQLiteGormPool(closedStore.db)
	jftradeCheckTestError(t, closedStore.Close())
	if _, err := closedPool.BeginTx(ctx, nil); err == nil {
		t.Fatal("BeginTx on closed store err = nil, want error")
	}
}

func TestStoreMaintenanceBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := (*Store)(nil).PurgeDeletedConfigs(ctx, DeletedConfigIDs{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil PurgeDeletedConfigs err = %v", err)
	}
	if active, err := (*Store)(nil).HasDatabaseActivity(ctx); err != nil || active {
		t.Fatalf("nil HasDatabaseActivity = %v/%v, want false/nil", active, err)
	}
	if err := (*Store)(nil).CompactDatabase(ctx); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil CompactDatabase err = %v", err)
	}

	store := newBusinessStore(t)
	if deleted, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Agents: []string{"missing"}}); !errors.Is(err, ErrCleanupCandidatesChanged) || deleted != 0 {
		t.Fatalf("missing agent purge = %d/%v, want candidate changed", deleted, err)
	}
	workflow, err := store.SaveWorkflowDefinition(ctx, WorkflowDefinition{ID: "not-deleted-workflow", Name: "Workflow", Status: WorkflowStatusEnabled})
	if err != nil {
		t.Fatalf("SaveWorkflowDefinition: %v", err)
	}
	if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Workflows: []string{workflow.ID}}); !errors.Is(err, ErrCleanupCandidatesChanged) {
		t.Fatalf("active workflow purge err = %v, want candidate changed", err)
	}
	trigger, err := store.SaveWorkflowTrigger(ctx, WorkflowTrigger{ID: "active-trigger", WorkflowID: workflow.ID, Type: WorkflowTriggerTypeWebhook, Status: WorkflowTriggerStatusEnabled})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}
	if _, err := store.PurgeDeletedConfigs(ctx, DeletedConfigIDs{Triggers: []string{trigger.ID}}); !errors.Is(err, ErrCleanupCandidatesChanged) {
		t.Fatalf("active trigger purge err = %v, want candidate changed", err)
	}
	if active, err := store.HasDatabaseActivity(ctx); err != nil || active {
		t.Fatalf("empty HasDatabaseActivity = %v/%v, want false/nil", active, err)
	}
	if err := store.SaveRun(ctx, Run{ID: "active-run", SessionID: "session", AgentID: "agent", Status: RunStatusRunning}); err != nil {
		t.Fatalf("SaveRun active: %v", err)
	}
	if active, err := store.HasDatabaseActivity(ctx); err != nil || !active {
		t.Fatalf("run HasDatabaseActivity = %v/%v, want true/nil", active, err)
	}

	dir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed: %v", err)
	}
	jftradeCheckTestError(t, closedStore.Close())
	if _, err := closedStore.PurgeDeletedConfigs(ctx, DeletedConfigIDs{}); err == nil {
		t.Fatal("PurgeDeletedConfigs on closed store err = nil, want error")
	}
	if _, err := closedStore.HasDatabaseActivity(ctx); err == nil {
		t.Fatal("HasDatabaseActivity on closed store err = nil, want error")
	}
	if err := closedStore.CompactDatabase(ctx); err == nil {
		t.Fatal("CompactDatabase on closed store err = nil, want error")
	}
}

func TestRuntimeFacadeBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	var nilRuntime *Runtime
	nilRuntime.SetRuntimeLimitsProvider(func() RuntimeLimits {
		return RuntimeLimits{RunTimeout: 99}
	})
	if got := nilRuntime.runtimeLimits().RunTimeout; got != DefaultRunTimeout {
		t.Fatalf("nil runtime timeout = %v, want default", got)
	}
	if store := nilRuntime.Store(); store != nil {
		t.Fatalf("nil runtime store = %#v, want nil", store)
	}
	if active, err := nilRuntime.HasDatabaseActivity(ctx); err != nil || active {
		t.Fatalf("nil runtime activity = %v/%v, want false/nil", active, err)
	}
	if err := nilRuntime.CompactSessionDatabase(ctx); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil CompactSessionDatabase err = %v", err)
	}
	runtime := newTestRuntime(t)
	runtime.SetRuntimeLimitsProvider(func() RuntimeLimits {
		return RuntimeLimits{RunTimeout: 42}
	})
	if got := runtime.runtimeLimits().RunTimeout; got != 42 {
		t.Fatalf("runtime timeout = %v, want provider override", got)
	}
	runtime.SetRuntimeLimitsProvider(func() RuntimeLimits {
		return RuntimeLimits{}
	})
	if got := runtime.runtimeLimits().RunTimeout; got != DefaultRunTimeout {
		t.Fatalf("zero runtime timeout = %v, want default", got)
	}
	runtime.activeMu.Lock()
	runtime.activeRuns["active-run"] = func() {}
	runtime.activeMu.Unlock()
	if active, err := runtime.HasDatabaseActivity(ctx); err != nil || !active {
		t.Fatalf("active runtime activity = %v/%v, want true/nil", active, err)
	}
	runtime.activeMu.Lock()
	delete(runtime.activeRuns, "active-run")
	runtime.activeMu.Unlock()
	if err := runtime.CompactSessionDatabase(ctx); err != nil {
		t.Fatalf("CompactSessionDatabase: %v", err)
	}
	if active, err := runtime.HasDatabaseActivity(ctx); err != nil || active {
		t.Fatalf("inactive runtime activity = %v/%v, want false/nil", active, err)
	}
	if tools := nilRuntime.Tools(); tools != nil {
		t.Fatalf("nil runtime tools = %#v, want nil", tools)
	}
	if skills := nilRuntime.Skills(); skills != nil {
		t.Fatalf("nil runtime skills = %#v, want nil", skills)
	}
	if err := nilRuntime.Close(); err != nil {
		t.Fatalf("nil runtime close: %v", err)
	}
}

func TestRuntimeConstructionCompactionAndCloseBoundaryBranches(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "artifact-parent-file")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	serviceWithBadArtifactPath := invalidArtifactPathSessionService{
		Service: adksession.InMemoryService(),
		path:    filepath.Join(blocker, "adk-session.db"),
	}
	runtimeWithFallbackArtifact := NewRuntimeWithSessionService(nil, nil, serviceWithBadArtifactPath)
	if runtimeWithFallbackArtifact.artifactService == nil {
		t.Fatal("fallback artifact service = nil, want in-memory fallback")
	}
	if err := runtimeWithFallbackArtifact.CloseSessionServices(); err != nil {
		t.Fatalf("CloseSessionServices fallback artifact: %v", err)
	}

	var nilRuntime *Runtime
	release, acquired := nilRuntime.beginSessionCompaction(" ")
	if !acquired {
		t.Fatal("nil/blank beginSessionCompaction acquired = false, want true no-op")
	}
	release()
	lockRuntime := &Runtime{}
	releaseFirst, acquiredFirst := lockRuntime.beginSessionCompaction("session-one")
	if !acquiredFirst {
		t.Fatal("first beginSessionCompaction acquired = false, want true")
	}
	releaseDuplicate, acquiredDuplicate := lockRuntime.beginSessionCompaction(" session-one ")
	if acquiredDuplicate {
		t.Fatal("duplicate beginSessionCompaction acquired = true, want false")
	}
	releaseDuplicate()
	releaseFirst()
	releaseAgain, acquiredAgain := lockRuntime.beginSessionCompaction("session-one")
	if !acquiredAgain {
		t.Fatal("beginSessionCompaction after release acquired = false, want true")
	}
	releaseAgain()

	closeDir := t.TempDir()
	store, err := NewStore(filepath.Join(closeDir, "adk.db"), filepath.Join(closeDir, "secrets", "adk.json"), filepath.Join(closeDir, "skills"))
	if err != nil {
		t.Fatalf("NewStore close runtime: %v", err)
	}
	sessionService, err := NewSQLiteSessionService(filepath.Join(closeDir, "adk-session.db"))
	if err != nil {
		t.Fatalf("NewSQLiteSessionService close runtime: %v", err)
	}
	closeRuntime := NewRuntimeWithSessionService(store, NewToolRegistry(), sessionService)
	cancelled := false
	closeRuntime.activeMu.Lock()
	closeRuntime.activeRuns["cancel-me"] = func() {
		cancelled = true
	}
	closeRuntime.activeMu.Unlock()
	if err := closeRuntime.Close(); err != nil {
		t.Fatalf("Runtime.Close: %v", err)
	}
	if !cancelled {
		t.Fatal("Runtime.Close did not cancel active run")
	}
}

func TestRuntimeSessionContextBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	var nilRuntime *Runtime
	if _, err := nilRuntime.SessionContext(ctx, "session"); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil SessionContext err = %v, want runtime error", err)
	}
	if _, err := nilRuntime.CompactSessionContext(ctx, "session", "", "", ""); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil CompactSessionContext err = %v, want runtime error", err)
	}

	runtime := newTestRuntime(t)
	if _, err := runtime.SessionContext(ctx, "missing-session"); err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("missing SessionContext err = %v, want session not found", err)
	}
	if _, err := runtime.CompactSessionContext(ctx, "missing-session", "", "", ""); err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("missing CompactSessionContext err = %v, want session not found", err)
	}

	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "context-agent", Name: "Context Agent", ProviderID: testProviderID, Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context session")
	release, acquired := runtime.beginSessionCompaction(session.ID)
	if !acquired {
		t.Fatal("manual compaction lock acquired = false, want true")
	}
	if _, err := runtime.CompactSessionContext(ctx, session.ID, "", "", ""); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("locked CompactSessionContext err = %v, want already running", err)
	}
	release()

	mustSaveRun(t, runtime, Run{
		ID: "active-context-run", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		CreatedAt: nowString(), UpdatedAt: nowString(),
	})
	if _, err := runtime.CompactSessionContext(ctx, session.ID, "", "", ""); err == nil || !strings.Contains(err.Error(), "active run") {
		t.Fatalf("active CompactSessionContext err = %v, want active run", err)
	}

	providerOverride := "missing-provider"
	modelOverride := "bad-model"
	if _, err := runtime.Store().SaveSessionComposerState(ctx, session.ID, SessionComposerStatePatch{
		ProviderIDOverride: &providerOverride,
		ModelOverride:      &modelOverride,
	}); err != nil {
		t.Fatalf("SaveSessionComposerState override: %v", err)
	}
	resolved, err := runtime.resolveSessionContextAgent(ctx, session)
	if err != nil {
		t.Fatalf("resolveSessionContextAgent fallback: %v", err)
	}
	if resolved.ProviderID != testProviderID || resolved.Model == modelOverride {
		t.Fatalf("fallback resolved agent = %+v, want base provider/model", resolved)
	}

	noBaseAgent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "context-agent-no-base-provider", Name: "No Base Provider", ProviderID: " ", Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent no base provider: %v", err)
	}
	noBaseSession := mustCreateSession(t, runtime, noBaseAgent.ID, "No base provider")
	if _, err := runtime.Store().SaveSessionComposerState(ctx, noBaseSession.ID, SessionComposerStatePatch{
		ProviderIDOverride: &providerOverride,
	}); err != nil {
		t.Fatalf("SaveSessionComposerState no base override: %v", err)
	}
	if _, err := runtime.resolveSessionContextAgent(ctx, noBaseSession); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("resolveSessionContextAgent no fallback err = %v, want provider error", err)
	}

	if overridden, ok := nilRuntime.applySessionComposerModelOverride(ctx, session.ID, agent); ok || overridden.ID != agent.ID {
		t.Fatalf("nil applySessionComposerModelOverride = %+v/%v, want original false", overridden, ok)
	}
	if overridden, ok := runtime.applySessionComposerModelOverride(ctx, " ", agent); ok || overridden.ID != agent.ID {
		t.Fatalf("blank applySessionComposerModelOverride = %+v/%v, want original false", overridden, ok)
	}

	errorRuntime := newTestRuntime(t)
	errorAgent := mustSaveAgent(t, errorRuntime, AgentWriteRequest{
		ID: "context-error-agent", Name: "Context Error Agent", ProviderID: testProviderID, Status: AgentStatusEnabled,
	})
	errorSession := mustCreateSession(t, errorRuntime, errorAgent.ID, "Context errors")
	if _, err := errorRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessions); err != nil {
		t.Fatalf("drop sessions for context errors: %v", err)
	}
	if _, err := errorRuntime.SessionContext(ctx, errorSession.ID); err == nil {
		t.Fatal("SessionContext missing sessions table err = nil, want error")
	}
	if _, err := errorRuntime.CompactSessionContext(ctx, errorSession.ID, "", "", ""); err == nil {
		t.Fatal("CompactSessionContext missing sessions table err = nil, want error")
	}

	skillRuntime := newTestRuntime(t)
	skillAgent := mustSaveAgent(t, skillRuntime, AgentWriteRequest{
		ID: "context-skill-error-agent", Name: "Context Skill Error Agent", ProviderID: testProviderID, Status: AgentStatusEnabled,
		Skills: []string{"missing-skill"},
	})
	skillSession := mustCreateSession(t, skillRuntime, skillAgent.ID, "Context skill error")
	if _, err := skillRuntime.SessionContext(ctx, skillSession.ID); err == nil || !strings.Contains(err.Error(), "skill not found") {
		t.Fatalf("SessionContext missing skill err = %v, want skill not found", err)
	}

	composerErrorRuntime := newTestRuntime(t)
	composerAgent := mustSaveAgent(t, composerErrorRuntime, AgentWriteRequest{
		ID: "composer-error-agent", Name: "Composer Error Agent", ProviderID: testProviderID, Status: AgentStatusEnabled,
	})
	composerSession := mustCreateSession(t, composerErrorRuntime, composerAgent.ID, "Composer error")
	if _, err := composerErrorRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessionComposer); err != nil {
		t.Fatalf("drop session composer: %v", err)
	}
	if overridden, ok := composerErrorRuntime.applySessionComposerModelOverride(ctx, composerSession.ID, composerAgent); ok || overridden.ID != composerAgent.ID {
		t.Fatalf("composer table error override = %+v/%v, want original false", overridden, ok)
	}
}

func TestRuntimeAgentProviderSessionAndMemoryBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if provider, err := (*Runtime)(nil).effectiveProvider(ctx, ""); err == nil || provider.ID != "" || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil effectiveProvider = %+v/%v, want runtime error", provider, err)
	}
	if agent, err := (*Runtime)(nil).resolveAgentProvider(ctx, Agent{}); err == nil || agent.ID != "" || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil resolveAgentProvider = %+v/%v, want runtime error", agent, err)
	}
	if prompt, err := (*Runtime)(nil).agentMemoryPrompt(ctx, "agent"); err != nil || prompt != "" {
		t.Fatalf("nil agentMemoryPrompt = %q/%v, want empty/nil", prompt, err)
	}
	(*Runtime)(nil).audit(ctx, "kind", "subject", "detail", nil)

	runtime := newTestRuntime(t)
	if _, err := runtime.resolveAgentDefinition(ctx, "missing-agent"); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("missing resolveAgentDefinition err = %v, want agent not found", err)
	}
	disabled := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "disabled-agent", Name: "Disabled Agent", Status: AgentStatusDisabled,
	})
	if _, err := runtime.resolveAgentDefinition(ctx, disabled.ID); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled resolveAgentDefinition err = %v, want disabled", err)
	}
	deleted := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "deleted-agent", Name: "Deleted Agent", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, deleted.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if _, err := runtime.Store().db.ExecContext(ctx, `UPDATE `+tableAgents+` SET payload_json = json_set(payload_json, '$.status', ?, '$.deletedAt', ?), updated_at = ? WHERE id = ?`,
		AgentStatusEnabled, nowString(), nowString(), deleted.ID); err != nil {
		t.Fatalf("force enabled deleted agent: %v", err)
	}
	if _, err := runtime.resolveAgentDefinition(ctx, deleted.ID); err == nil || !strings.Contains(err.Error(), "deleted") {
		t.Fatalf("deleted resolveAgentDefinition err = %v, want deleted", err)
	}
	disabledProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "disabled-provider", DisplayName: "Disabled Provider", BaseURL: "http://127.0.0.1:9/v1", Model: "model", APIKey: "sk-test", Enabled: false,
	})
	if _, err := runtime.resolveAgentProvider(ctx, Agent{ProviderID: disabledProvider.ID}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("disabled resolveAgentProvider err = %v, want unavailable", err)
	}
	noKeyProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "no-key-provider", DisplayName: "No Key Provider", BaseURL: "http://127.0.0.1:9/v1", Model: "model", Enabled: true,
	})
	if _, err := runtime.resolveAgentProvider(ctx, Agent{ProviderID: noKeyProvider.ID}); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("no-key resolveAgentProvider err = %v, want API key", err)
	}
	if _, err := runtime.effectiveProvider(ctx, "missing-provider"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing effectiveProvider err = %v, want unavailable", err)
	}
	if err := os.WriteFile(runtime.Store().secrets.path, []byte("{"), 0o600); err != nil {
		t.Fatalf("write bad provider secrets: %v", err)
	}
	if _, err := runtime.resolveAgentProvider(ctx, Agent{ProviderID: testProviderID}); err == nil {
		t.Fatal("resolveAgentProvider bad secrets err = nil, want error")
	}

	agentOne := Agent{ID: "agent-one"}
	agentTwo := Agent{ID: "agent-two"}
	session := mustCreateSession(t, runtime, agentOne.ID, "Existing session")
	if _, err := runtime.resolveSession(ctx, session.ID, agentTwo, "text"); err == nil || !strings.Contains(err.Error(), "different agent") {
		t.Fatalf("different-agent resolveSession err = %v, want different agent", err)
	}
	if _, err := runtime.resolveSession(ctx, "missing-session", agentOne, "text"); err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("missing resolveSession err = %v, want session not found", err)
	}
	created, err := runtime.resolveSession(ctx, "", agentOne, strings.Repeat("长", 40))
	if err != nil {
		t.Fatalf("resolveSession create: %v", err)
	}
	if got := len([]rune(created.Title)); got != 28 {
		t.Fatalf("created session title length = %d, want 28", got)
	}

	if _, err := runtime.prepareAgent(ctx, Agent{ID: "skill-agent", Skills: []string{"missing-skill"}}); err == nil || !strings.Contains(err.Error(), "skill not found") {
		t.Fatalf("missing skill prepareAgent err = %v, want skill not found", err)
	}
	memoryAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "memory-agent", Name: "Memory Agent", Status: AgentStatusEnabled, MemoryEnabled: true,
	})
	for index := range 3 {
		if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{
			AgentID: memoryAgent.ID,
			Scope:   "agent",
			Key:     fmt.Sprintf("key-%d", index),
			Value:   strings.Repeat("x", 2000),
		}); err != nil {
			t.Fatalf("SaveMemory %d: %v", index, err)
		}
	}
	prepared, err := runtime.prepareAgent(ctx, memoryAgent)
	if err != nil {
		t.Fatalf("prepareAgent memory: %v", err)
	}
	if !strings.Contains(prepared.Instruction, "JFTrade memory:") {
		t.Fatalf("prepared memory instruction missing memory block: %q", prepared.Instruction)
	}

	closedDir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(closedDir, "adk.db"), filepath.Join(closedDir, "secrets", "adk.json"), filepath.Join(closedDir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed memory: %v", err)
	}
	jftradeCheckTestError(t, closedStore.Close())
	if _, err := (&Runtime{store: closedStore}).agentMemoryPrompt(ctx, "agent"); err == nil {
		t.Fatal("closed agentMemoryPrompt err = nil, want error")
	}
	if _, err := (&Runtime{store: closedStore, skills: NewSkillRegistry(closedStore.SkillsPath())}).prepareAgent(ctx, Agent{ID: "memory-closed", MemoryEnabled: true}); err == nil {
		t.Fatal("prepareAgent closed memory err = nil, want error")
	}
	if prompt, err := runtime.agentMemoryPrompt(ctx, "agent-without-memory"); err != nil || prompt != "" {
		t.Fatalf("empty agentMemoryPrompt = %q/%v, want empty nil", prompt, err)
	}

	emptyDir := t.TempDir()
	emptyStore, err := NewStore(filepath.Join(emptyDir, "adk.db"), filepath.Join(emptyDir, "secrets", "adk.json"), filepath.Join(emptyDir, "skills"))
	if err != nil {
		t.Fatalf("NewStore empty providers: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, emptyStore.Close()) })
	emptyRuntime := NewRuntimeWithSessionService(emptyStore, NewToolRegistry(), adksession.InMemoryService())
	if _, err := emptyRuntime.effectiveProvider(ctx, ""); err == nil || !strings.Contains(err.Error(), "default agent provider") {
		t.Fatalf("empty default effectiveProvider err = %v, want default provider error", err)
	}

	tableErrorRuntime := newTestRuntime(t)
	if _, err := tableErrorRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableAgents); err != nil {
		t.Fatalf("drop agents table: %v", err)
	}
	if _, err := tableErrorRuntime.resolveAgentDefinition(ctx, "agent"); err == nil {
		t.Fatal("resolveAgentDefinition missing agents table err = nil, want error")
	}
	providerErrorRuntime := newTestRuntime(t)
	if _, err := providerErrorRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableProviders); err != nil {
		t.Fatalf("drop providers table: %v", err)
	}
	if _, err := providerErrorRuntime.effectiveProvider(ctx, ""); err == nil {
		t.Fatal("effectiveProvider missing providers table err = nil, want error")
	}
	if _, err := providerErrorRuntime.effectiveProvider(ctx, testProviderID); err == nil {
		t.Fatal("effectiveProvider explicit missing providers table err = nil, want error")
	}
	sessionErrorRuntime := newTestRuntime(t)
	if _, err := sessionErrorRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableSessions); err != nil {
		t.Fatalf("drop sessions table: %v", err)
	}
	if _, err := sessionErrorRuntime.resolveSession(ctx, "session", Agent{ID: "agent"}, "text"); err == nil {
		t.Fatal("resolveSession missing sessions table err = nil, want error")
	}
}

func TestRuntimeSnapshotAndProviderTestBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	runtime.skills = &SkillRegistry{}
	if _, err := runtime.Snapshot(ctx); err == nil || !strings.Contains(err.Error(), "skill registry") {
		t.Fatalf("Snapshot skill error = %v, want skill registry error", err)
	}

	agentTableDir := t.TempDir()
	agentTableStore, err := NewStore(filepath.Join(agentTableDir, "adk.db"), filepath.Join(agentTableDir, "secrets", "adk.json"), filepath.Join(agentTableDir, "skills"))
	if err != nil {
		t.Fatalf("NewStore agent table: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, agentTableStore.Close()) })
	if _, err := agentTableStore.db.ExecContext(ctx, `DROP TABLE `+tableAgents); err != nil {
		t.Fatalf("drop agents: %v", err)
	}
	agentTableRuntime := &Runtime{store: agentTableStore, skills: NewSkillRegistry(agentTableStore.SkillsPath()), tools: NewToolRegistry()}
	if _, err := agentTableRuntime.Snapshot(ctx); err == nil {
		t.Fatal("Snapshot missing agents table err = nil, want error")
	}

	closedDir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(closedDir, "adk.db"), filepath.Join(closedDir, "secrets", "adk.json"), filepath.Join(closedDir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed snapshot: %v", err)
	}
	closedRuntime := &Runtime{store: closedStore, skills: NewSkillRegistry(closedStore.SkillsPath()), tools: NewToolRegistry()}
	jftradeCheckTestError(t, closedStore.Close())
	if _, err := closedRuntime.Snapshot(ctx); err == nil {
		t.Fatal("Snapshot closed store err = nil, want error")
	}
	if _, err := closedRuntime.TestProvider(ctx, testProviderID); err == nil {
		t.Fatal("TestProvider closed store err = nil, want error")
	}
	if _, err := runtime.TestProvider(ctx, "missing-provider"); err == nil || !strings.Contains(err.Error(), "provider not found") {
		t.Fatalf("TestProvider missing err = %v, want provider not found", err)
	}
	badProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "bad-chat-provider", DisplayName: "Bad Chat Provider", BaseURL: "http://127.0.0.1:1/v1", Model: "model", APIKey: "sk-test", Enabled: true,
	})
	if _, err := runtime.TestProvider(ctx, badProvider.ID); err == nil {
		t.Fatal("TestProvider bad chat err = nil, want error")
	}
}
