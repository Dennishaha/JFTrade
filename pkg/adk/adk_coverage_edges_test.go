package adk

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adkagent "google.golang.org/adk/v2/agent"
	adkmemory "google.golang.org/adk/v2/memory"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"

	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
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
		Name: "Coverage Workflow", Status: WorkflowStatusEnabled, AgentID: "agent", WorkMode: WorkModeTask,
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

func TestSmallADKBoundaryTailBranches(t *testing.T) {
	if _, err := googleADKJSONSchemaFromMap(map[string]any{"type": 123}); err == nil {
		t.Fatal("googleADKJSONSchemaFromMap invalid schema err = nil, want decode error")
	}

	runtime := newTestRuntime(t)
	for index := range 2 {
		mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID: fmt.Sprintf("limit-provider-%d", index), DisplayName: fmt.Sprintf("Limit Provider %d", index),
			BaseURL: "https://example.test/v1", Model: fmt.Sprintf("model-%d", index), APIKey: "sk-limit", Enabled: true,
		})
	}
	raw, err := runtime.modelsListTool(t.Context(), map[string]any{"limit": 1, "callableOnly": "yes"})
	if err != nil {
		t.Fatalf("modelsListTool limit: %v", err)
	}
	payload := raw.(map[string]any)
	if payload["totalReturned"] != 1 {
		t.Fatalf("modelsListTool limit payload = %#v", payload)
	}
	if !toolBoolValue(map[string]any{"flag": "true"}, "flag", false) {
		t.Fatal("toolBoolValue true string = false, want true")
	}

	var splitter legacyAssistantContentSplitter
	reply, reasoning := splitter.Push("visible <not-a-tag> tail")
	if !strings.Contains(reply, "<not-a-tag>") || reasoning != "" {
		t.Fatalf("legacy splitter unknown tag = reply:%q reasoning:%q", reply, reasoning)
	}

	execution := &googleADKExecution{
		runID: "plugin-run",
		agent: Agent{PermissionMode: PermissionModeApproval},
		descriptors: map[string]ToolDescriptor{
			"live.trade": {Name: "live.trade", Permission: "live_trading", AllowedModes: []string{PermissionModeAll}},
		},
	}
	ctx := newGoogleADKToolTestContext()
	if result, err := execution.beforeToolCallback(ctx, boundaryGoogleTool{name: "unknown.tool"}, map[string]any{}); result != nil || err != nil {
		t.Fatalf("beforeToolCallback unknown = %#v/%v, want nil/nil", result, err)
	}
	if result, err := execution.beforeToolCallback(ctx, boundaryGoogleTool{name: "live.trade"}, map[string]any{}); result != nil || err == nil || !strings.Contains(err.Error(), "permission mode") {
		t.Fatalf("beforeToolCallback disallowed = %#v/%v, want permission error", result, err)
	}
	if result, err := execution.afterToolCallback(ctx, boundaryGoogleTool{name: "unknown.tool"}, map[string]any{}, nil, nil); result != nil || err != nil {
		t.Fatalf("afterToolCallback unknown = %#v/%v, want nil/nil", result, err)
	}

	nodes := []adkworkflow.Node{newWorkflowCompilerTestNode("first")}
	edges, err := newWorkflowCompiler().CompileEdges([]workflowStep{}, nodes)
	if err != nil {
		t.Fatalf("CompileEdges fallback: %v", err)
	}
	if len(edges) != 1 || edges[0].To.Name() != "first" {
		t.Fatalf("CompileEdges fallback edges = %+v", edges)
	}
	edges, err = newWorkflowCompiler().CompileEdges([]workflowStep{{DependencyID: "first"}, {DependencyID: "ignored"}}, nodes)
	if err != nil || len(edges) != 1 || edges[0].To.Name() != "first" {
		t.Fatalf("CompileEdges fewer nodes = %+v/%v", edges, err)
	}
}

func TestProviderHTTPBoundaryTailBranches(t *testing.T) {
	if err := validateProviderHostname(" "); err == nil || !strings.Contains(err.Error(), "host is required") {
		t.Fatalf("blank provider host err = %v, want required", err)
	}
	if err := validateProviderIP(netip.Addr{}); err == nil || !strings.Contains(err.Error(), "unspecified") {
		t.Fatalf("invalid provider IP err = %v, want unspecified", err)
	}
	if err := validateProviderIP(netip.MustParseAddr("224.0.0.1")); err == nil || !strings.Contains(err.Error(), "multicast") {
		t.Fatalf("multicast provider IP err = %v, want multicast", err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("jftradeCheckedTypeAssertion panic = nil, want panic")
			}
		}()
		_ = jftradeCheckedTypeAssertion[*http.Transport]("not a transport")
	}()

	lookupErr := errors.New("lookup failed")
	client := newProviderHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, lookupErr
	})
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(t.Context(), "tcp", "missing-port"); err == nil {
		t.Fatal("provider DialContext split host port err = nil, want error")
	}
	if _, err := transport.DialContext(t.Context(), "tcp", "metadata:443"); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("provider DialContext metadata err = %v, want metadata blocked", err)
	}
	if _, err := transport.DialContext(t.Context(), "tcp", "example.test:443"); !errors.Is(err, lookupErr) {
		t.Fatalf("provider DialContext lookup err = %v, want lookupErr", err)
	}

	blockedClient := newProviderHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
	})
	if _, err := blockedClient.Transport.(*http.Transport).DialContext(t.Context(), "tcp", "example.test:443"); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("provider DialContext blocked IP err = %v, want blocked address", err)
	}
	emptyClient := newProviderHTTPClientWithResolver(time.Second, func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, nil
	})
	if _, err := emptyClient.Transport.(*http.Transport).DialContext(t.Context(), "tcp", "example.test:443"); err == nil || !strings.Contains(err.Error(), "no usable addresses") {
		t.Fatalf("provider DialContext empty addresses err = %v, want no usable", err)
	}
	if err := emptyClient.CheckRedirect(&http.Request{URL: &url.URL{Host: "example.test"}}, make([]*http.Request, 5)); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("provider redirect limit err = %v, want redirect limit", err)
	}
	if err := emptyClient.CheckRedirect(&http.Request{URL: &url.URL{Host: "metadata"}}, nil); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("provider redirect metadata err = %v, want metadata host", err)
	}
}

func TestProjectionAndReasoningHelperBoundaryBranches(t *testing.T) {
	if got := projectionRunID(nil); got != "" {
		t.Fatalf("nil projectionRunID = %q, want empty", got)
	}
	if got := projectionRunID(&adksession.Event{InvocationID: " invocation "}); got != "invocation" {
		t.Fatalf("invocation projectionRunID = %q", got)
	}
	if got := projectionRunID(&adksession.Event{ID: " event-id "}); got != "event-id" {
		t.Fatalf("event projectionRunID = %q", got)
	}
	timestamp := time.Date(2026, 7, 5, 1, 2, 3, 4, time.FixedZone("CST", 8*60*60))
	if got := projectionRunID(&adksession.Event{Timestamp: timestamp}); got != timestamp.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("timestamp projectionRunID = %q", got)
	}
	if got := eventTimeString(&adksession.Event{}); got == "" {
		t.Fatal("zero eventTimeString should fall back to nowString")
	}
	var builder strings.Builder
	mergeProjectedText(&builder, "hello", false)
	mergeProjectedText(&builder, "hello world", false)
	if got := builder.String(); got != "hello world" {
		t.Fatalf("prefix merge = %q", got)
	}
	mergeProjectedText(&builder, "world", false)
	if got := builder.String(); got != "hello world" {
		t.Fatalf("suffix merge = %q", got)
	}
	mergeProjectedText(&builder, "!", false)
	if got := builder.String(); got != "hello world!" {
		t.Fatalf("append merge = %q", got)
	}

	var splitter legacyAssistantContentSplitter
	splitter.tagBuffer.WriteString("<think>")
	if reply, reasoning := splitter.Flush(); reply != "" || reasoning != "" || splitter.mode != reasoningModeReasoning {
		t.Fatalf("flush opening = (%q,%q) mode=%v", reply, reasoning, splitter.mode)
	}
	splitter.tagBuffer.WriteString("</think>")
	if reply, reasoning := splitter.Flush(); reply != "" || reasoning != "" || splitter.mode != reasoningModeReply {
		t.Fatalf("flush closing = (%q,%q) mode=%v", reply, reasoning, splitter.mode)
	}
	splitter = legacyAssistantContentSplitter{mode: reasoningModeReasoning}
	splitter.tagBuffer.WriteString("<partial")
	if reply, reasoning := splitter.Flush(); reply != "" || reasoning != "<partial" {
		t.Fatalf("flush reasoning partial = (%q,%q)", reply, reasoning)
	}
}

func TestNormalizeAndWorkflowModelToolBoundaryBranches(t *testing.T) {
	run := Run{ID: "run-resolution", ToolCalls: nil}
	parent := Run{ID: "parent-resolution", ToolCalls: nil}
	resolution := NormalizeApprovalResolution(ApprovalResolution{Run: &run, ParentRun: &parent})
	if resolution.Run == &run || resolution.ParentRun == &parent {
		t.Fatal("NormalizeApprovalResolution should copy run pointers")
	}
	if got := normalizeAnyMap(map[string]any{" ": "ignored"}); len(got) != 0 {
		t.Fatalf("normalizeAnyMap blank-only = %#v, want empty", got)
	}
	runtime := newTestRuntime(t)
	toolset := &workflowTaskToolset{executor: runtime.workflowExecutor()}
	modelTool, err := toolset.modelsListTool()
	if err != nil {
		t.Fatalf("modelsListTool: %v", err)
	}
	if modelTool.Name() != workflowModelsListTool {
		t.Fatalf("models tool name = %q", modelTool.Name())
	}
	jftradeCheckTestError(t, runtime.Store().Close())
	if _, err := toolset.modelsList(map[string]any{"query": "test"}); err == nil {
		t.Fatal("modelsList closed runtime err = nil, want error")
	}
}

func TestWorkflowTaskLocalHelperBoundaryBranches(t *testing.T) {
	var nilDecision *workflowGoalDecision
	nilDecision.reset()
	nilDecision.beginDecision()
	nilDecision.setComplete("ignored")
	nilDecision.setContinue("ignored")
	if nilDecision.decisionPhase() {
		t.Fatal("nil decision should not be in decision phase")
	}
	if snap := nilDecision.snapshot(); snap.status != "" || snap.summary != "" || snap.reason != "" {
		t.Fatalf("nil decision snapshot status=%q summary=%q reason=%q, want empty", snap.status, snap.summary, snap.reason)
	}
	decision := &workflowGoalDecision{}
	decision.beginDecision()
	if !decision.decisionPhase() {
		t.Fatal("decision should be in decision phase")
	}
	decision.setComplete(" complete summary ")
	if snap := decision.snapshot(); snap.status != "complete" || snap.summary != "complete summary" || snap.reason != "" {
		t.Fatalf("complete decision snapshot status=%q summary=%q reason=%q", snap.status, snap.summary, snap.reason)
	}
	decision.setContinue(" continue reason ")
	if snap := decision.snapshot(); snap.status != "continue" || snap.reason != "continue reason" || snap.summary != "" {
		t.Fatalf("continue decision snapshot status=%q summary=%q reason=%q", snap.status, snap.summary, snap.reason)
	}
	decision.reset()
	if decision.decisionPhase() {
		t.Fatal("reset decision should leave decision phase")
	}

	if run, changed := pruneInterruptedGoalWorkflowToolCalls(Run{}); changed || len(run.ToolCalls) != 0 {
		t.Fatalf("empty prune = %+v changed=%v", run, changed)
	}
	pauseErr := errUserGoalPauseRequested.Error()
	run, changed := pruneInterruptedGoalWorkflowToolCalls(Run{
		ID: "parent-run",
		ToolCalls: []ToolCall{
			{ID: "keep-other-run", RunID: "child-run", ToolName: workflowTasksListTool, Status: "RUNNING"},
			{ID: "keep-business", RunID: "parent-run", ToolName: "market.candles", Status: "RUNNING"},
			{ID: "keep-failed-other", RunID: "parent-run", ToolName: workflowTasksListTool, Status: "FAILED"},
			{ID: "drop-running", RunID: "parent-run", ToolName: workflowTasksListTool, Status: "RUNNING"},
			{ID: "drop-pending", ToolName: workflowTaskAddTool, Status: "PENDING"},
			{ID: "drop-failed-pause", ToolName: workflowTaskClaimTool, Status: "FAILED", Error: &pauseErr},
		},
	})
	if !changed {
		t.Fatal("workflow tool prune changed = false, want true")
	}
	if len(run.ToolCalls) != 3 {
		t.Fatalf("pruned tool calls = %+v, want three kept calls", run.ToolCalls)
	}
	for _, call := range run.ToolCalls {
		if strings.HasPrefix(call.ID, "drop-") {
			t.Fatalf("interrupted call was not pruned: %+v", run.ToolCalls)
		}
	}
}

func TestWorkflowTaskToolsetLookupBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-helper-parent", SessionID: "workflow-helper-session", AgentID: "workflow-helper-agent",
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-current", ChildRunID: ""},
			{TaskID: "task-missing-child", ChildRunID: "missing-child"},
			{TaskID: "task-foreign-child", ChildRunID: "foreign-child"},
			{TaskID: "task-pending-child", ChildRunID: "pending-child"},
		},
		ChildRunIDs: []string{"", "workflow-helper-parent"},
		CreatedAt:   now, UpdatedAt: now,
	})
	current, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-current", Title: "Current", Status: "IN_PROGRESS", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask current: %v", err)
	}
	ready, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-ready", Title: "Ready", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask ready: %v", err)
	}
	mustSaveRun(t, runtime, Run{
		ID: "foreign-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: "other-parent",
		Status: RunStatusRunning, CreatedAt: now, UpdatedAt: now,
	})
	mustSaveRun(t, runtime, Run{
		ID: "pending-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusPending, CreatedAt: now, UpdatedAt: now,
	})
	toolset := &workflowTaskToolset{
		executor:      runtime.workflowExecutor(),
		parentID:      parent.ID,
		currentTaskID: current.ID,
		req:           workflowRequest{Mode: WorkModeTask},
	}
	if _, _, err := (&workflowTaskToolset{executor: runtime.workflowExecutor(), parentID: "missing-parent"}).parentAndTasks(ctx); err == nil || !strings.Contains(err.Error(), "parent run not found") {
		t.Fatalf("missing parentAndTasks err = %v", err)
	}
	if task, ok := toolset.taskByID(ctx, " "); ok || task.ID != "" {
		t.Fatalf("blank taskByID = %+v/%v, want missing", task, ok)
	}
	if task, err := toolset.resolveTask(ctx, parent, []Task{ready}, "missing-task", true); err == nil || task.ID != "" || !strings.Contains(err.Error(), "task not found") {
		t.Fatalf("explicit missing resolveTask = %+v/%v", task, err)
	}
	if task, err := toolset.resolveTask(ctx, parent, []Task{ready}, "", false); err != nil || task.ID != current.ID {
		t.Fatalf("current resolveTask = %+v/%v, want current", task, err)
	}
	toolset.currentTaskID = ""
	if task, err := toolset.resolveTask(ctx, parent, []Task{{ID: "in-progress", Status: "IN_PROGRESS"}}, "", false); err != nil || task.ID != "in-progress" {
		t.Fatalf("in-progress resolveTask = %+v/%v", task, err)
	}
	if task, err := toolset.resolveTask(ctx, parent, []Task{ready}, "", true); err != nil || task.ID != ready.ID {
		t.Fatalf("ready resolveTask = %+v/%v", task, err)
	}
	if _, err := toolset.resolveTask(ctx, parent, []Task{{ID: "blocked-ready", Status: "TODO", DependsOn: []string{"missing"}}}, "", true); err == nil || !strings.Contains(err.Error(), "no executable workflow task") {
		t.Fatalf("no executable resolveTask err = %v", err)
	}

	child, index, ok := runtime.workflowExecutor().firstBlockingTaskChild(ctx, parent)
	if !ok || child.ID != "pending-child" || index != 3 {
		t.Fatalf("firstBlockingTaskChild = %+v index=%d ok=%v, want pending child at index 3", child, index, ok)
	}
	cleanParent := parent
	cleanParent.WorkflowPlan = []WorkflowStepState{{TaskID: "blank-child"}, {TaskID: "done-child", ChildRunID: "done-child"}}
	mustSaveRun(t, runtime, Run{
		ID: "done-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusCompleted, CreatedAt: now, UpdatedAt: now,
	})
	if child, index, ok := runtime.workflowExecutor().firstBlockingTaskChild(ctx, cleanParent); ok || child.ID != "" || index != -1 {
		t.Fatalf("clean firstBlockingTaskChild = %+v index=%d ok=%v, want none", child, index, ok)
	}

	blockers := toolset.workflowCompletionBlockers(ctx, parent, []Task{{ID: "done", Status: "DONE"}})
	if len(blockers) != 0 {
		t.Fatalf("blank/self child IDs should not block completion: %+v", blockers)
	}

	dir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed workflow task lookup: %v", err)
	}
	closedRuntime := NewRuntime(closedStore, NewToolRegistry())
	closedToolset := &workflowTaskToolset{executor: closedRuntime.workflowExecutor(), parentID: parent.ID}
	jftradeCheckTestError(t, closedStore.Close())
	if _, _, err := closedToolset.parentAndTasks(ctx); err == nil {
		t.Fatal("closed parentAndTasks err = nil, want error")
	}
	if task, ok := closedToolset.taskByID(ctx, current.ID); ok || task.ID != "" {
		t.Fatalf("closed taskByID = %+v/%v, want missing", task, ok)
	}
	if err := closedToolset.saveParentPlan(ctx, parent, nil); err == nil {
		t.Fatal("closed saveParentPlan err = nil, want error")
	}
	for _, tc := range []struct {
		name string
		call func() (map[string]any, error)
	}{
		{name: "list", call: func() (map[string]any, error) { return closedToolset.list(nil) }},
		{name: "add", call: func() (map[string]any, error) { return closedToolset.add(map[string]any{"title": "x"}) }},
		{name: "claim", call: func() (map[string]any, error) { return closedToolset.claim(map[string]any{"taskId": current.ID}) }},
		{name: "complete", call: func() (map[string]any, error) { return closedToolset.complete(map[string]any{"taskId": current.ID}) }},
		{name: "block", call: func() (map[string]any, error) { return closedToolset.block(map[string]any{"taskId": current.ID}) }},
		{name: "delegate", call: func() (map[string]any, error) { return closedToolset.delegate(map[string]any{"taskId": current.ID}) }},
		{name: "goalComplete", call: func() (map[string]any, error) { return closedToolset.goalComplete(map[string]any{"summary": "done"}) }},
	} {
		t.Run("closed "+tc.name, func(t *testing.T) {
			if result, err := tc.call(); err == nil || result != nil {
				t.Fatalf("%s closed result = %#v err=%v, want nil/error", tc.name, result, err)
			}
		})
	}
}

func TestWorkflowTaskToolsetMethodErrorAndFallbackBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-method-branches-parent", SessionID: "workflow-method-branches-session", AgentID: "workflow-method-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	done, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-method-done", Title: "Done task", Status: "DONE", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask done: %v", err)
	}
	ready, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-method-ready", Title: "Ready task", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask ready: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{done, ready}, nil)
	mustSaveRun(t, runtime, parent)
	toolset := &workflowTaskToolset{
		executor: runtime.workflowExecutor(),
		parentID: parent.ID,
		req:      workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}},
	}
	for _, tc := range []struct {
		name string
		call func() (map[string]any, error)
	}{
		{name: "claim missing", call: func() (map[string]any, error) { return toolset.claim(map[string]any{"taskId": "missing-task"}) }},
		{name: "complete missing", call: func() (map[string]any, error) { return toolset.complete(map[string]any{"taskId": "missing-task"}) }},
		{name: "block missing", call: func() (map[string]any, error) { return toolset.block(map[string]any{"taskId": "missing-task"}) }},
		{name: "delegate missing", call: func() (map[string]any, error) { return toolset.delegate(map[string]any{"taskId": "missing-task"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if result, err := tc.call(); err == nil || result != nil || !strings.Contains(err.Error(), "task not found") {
				t.Fatalf("%s result = %#v err=%v, want task not found", tc.name, result, err)
			}
		})
	}
	complete, err := toolset.goalComplete(map[string]any{"resultSummary": "done via result summary"})
	if err != nil {
		t.Fatalf("goalComplete resultSummary: %v", err)
	}
	if complete["success"] != false || complete["status"] != "blocked" {
		t.Fatalf("goalComplete with open task = %#v, want blocked", complete)
	}
	doneStatus := "DONE"
	if _, err := runtime.Store().UpdateTask(ctx, ready.ID, TaskPatchRequest{Status: &doneStatus}); err != nil {
		t.Fatalf("UpdateTask ready done: %v", err)
	}
	complete, err = toolset.goalComplete(map[string]any{"resultSummary": "done via result summary"})
	if err != nil {
		t.Fatalf("goalComplete success resultSummary: %v", err)
	}
	if complete["success"] != true || complete["summary"] != "done via result summary" {
		t.Fatalf("goalComplete success = %#v, want resultSummary fallback", complete)
	}
	if snap := toolset.req.GoalDecision.snapshot(); snap.status != "complete" || snap.summary != "done via result summary" {
		t.Fatalf("goal decision = status:%q summary:%q", snap.status, snap.summary)
	}
}

func TestWorkflowPlannerAdditionalBoundaryBranches(t *testing.T) {
	tool, err := newWorkflowMapFunctionTool(workflowMapToolSpec{
		name:        "workflow.coverage.nil",
		description: "coverage",
		schema:      emptyObjectSchema(),
	})
	if err != nil {
		t.Fatalf("newWorkflowMapFunctionTool: %v", err)
	}
	runnable, ok := tool.(interface {
		Run(adkagent.Context, any) (map[string]any, error)
	})
	if !ok {
		t.Fatalf("workflow map tool type = %T, want runnable", tool)
	}
	mock := newGoogleADKToolTestContext()
	if result, err := runnable.Run(mock, map[string]any{}); err == nil || result != nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil workflow tool result = %#v err=%v, want unavailable", result, err)
	}
	if result, err := runnable.Run(mock, "bad"); err == nil || result != nil || !strings.Contains(err.Error(), "unexpected args type") {
		t.Fatalf("bad workflow tool args result = %#v err=%v, want args type error", result, err)
	}

	if got := plannerStringArg(map[string]any{"x": nil}, "x"); got != "" {
		t.Fatalf("plannerStringArg nil = %q, want empty", got)
	}
	if got := plannerStringArg(map[string]any{"x": "<nil>"}, "x"); got != "" {
		t.Fatalf("plannerStringArg <nil> = %q, want empty", got)
	}
	if got := plannerStringArg(map[string]any{"x": "  value  "}, "x"); got != "value" {
		t.Fatalf("plannerStringArg trim = %q, want value", got)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want int
	}{
		{name: "nil", args: nil, want: 0},
		{name: "int64", args: map[string]any{"x": int64(12)}, want: 12},
		{name: "float64", args: map[string]any{"x": float64(12.9)}, want: 12},
		{name: "float32", args: map[string]any{"x": float32(7.9)}, want: 7},
		{name: "string", args: map[string]any{"x": " 42 "}, want: 42},
		{name: "bad", args: map[string]any{"x": "not-a-number"}, want: 0},
		{name: "nil string", args: map[string]any{"x": "<nil>"}, want: 0},
	} {
		if got := plannerIntArg(tc.args, "x"); got != tc.want {
			t.Fatalf("plannerIntArg %s = %d, want %d", tc.name, got, tc.want)
		}
	}

	unfinished := workflowPlanDraft{Warnings: []string{"keep"}}
	if steps, warnings, err := compileWorkflowPlanDraft(unfinished, WorkModeTask, "msg", "msg", RunOptions{}); err == nil || steps != nil || len(warnings) != 1 {
		t.Fatalf("unfinished draft = steps:%#v warnings:%#v err:%v, want warning/error", steps, warnings, err)
	}
	empty := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{{Title: "empty"}}}
	if steps, _, err := compileWorkflowPlanDraft(empty, WorkModeTask, "msg", "msg", RunOptions{}); err == nil || steps != nil || !strings.Contains(err.Error(), "no valid steps") {
		t.Fatalf("empty draft = steps:%#v err:%v, want no valid steps", steps, err)
	}
	duplicate := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{
		{Order: 2, Title: "B", Message: "run B"},
		{Order: 2, Title: "A", Message: "run A"},
		{Order: 0, Title: "C", Message: "run C"},
	}}
	steps, warnings, err := compileWorkflowPlanDraft(duplicate, WorkModeTask, "user message", "different objective", RunOptions{})
	if err != nil {
		t.Fatalf("compile duplicate draft: %v", err)
	}
	if len(steps) != 3 || steps[0].Order != 1 || len(warnings) != 1 || !strings.Contains(warnings[0], "duplicated") {
		t.Fatalf("duplicate normalization steps=%#v warnings=%#v", steps, warnings)
	}
	if len(steps[1].DependsOn) != 1 || steps[1].DependsOn[0] != steps[0].DependencyID {
		t.Fatalf("sequential dependency = %#v, want previous id %q", steps[1].DependsOn, steps[0].DependencyID)
	}
	loop := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{
		{Title: "one", Message: "first"},
		{Title: "two", Message: "second"},
	}}
	loopSteps, loopWarnings, err := compileWorkflowPlanDraft(loop, WorkModeLoop, "msg", "msg", RunOptions{})
	if err != nil {
		t.Fatalf("compile loop draft: %v", err)
	}
	if len(loopSteps) != 1 || len(loopWarnings) != 1 || !strings.Contains(loopWarnings[0], "first planner step") {
		t.Fatalf("loop truncation steps=%#v warnings=%#v", loopSteps, loopWarnings)
	}
	depLoop := workflowPlanDraft{Finished: true, Steps: []workflowPlanDraftStep{{Title: "one", Message: "first", DependsOn: []string{"x"}}}}
	if _, _, err := compileWorkflowPlanDraft(depLoop, WorkModeLoop, "msg", "msg", RunOptions{}); err == nil || !strings.Contains(err.Error(), "must not depend") {
		t.Fatalf("loop dependency err = %v, want dependency error", err)
	}
	ambiguous := []workflowStep{
		{Title: "same", Message: "first", DependencyID: "a"},
		{Title: "same", Message: "second", DependencyID: "b", DependsOn: []string{"same"}},
	}
	if err := normalizeSequentialPlannerDependencies(ambiguous); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous dependency err = %v, want ambiguous", err)
	}
	aliases := map[string]int{"first": 0, "second": 1}
	resolved, err := resolveWorkflowStepDependencies([]string{" first ", "first", ""}, aliases, []workflowStep{{DependencyID: "dep-1"}, {DependencyID: "dep-2"}}, 1)
	if err != nil || len(resolved) != 1 || resolved[0] != "dep-1" {
		t.Fatalf("resolved duplicate deps = %#v err=%v, want dep-1", resolved, err)
	}
	if _, err := resolveWorkflowStepDependencies([]string{"missing"}, aliases, []workflowStep{{DependencyID: "dep-1"}}, 1); err == nil || !strings.Contains(err.Error(), "known step") {
		t.Fatalf("missing dep err = %v, want known step", err)
	}
	if _, err := resolveWorkflowStepDependencies([]string{"second"}, aliases, []workflowStep{{DependencyID: "dep-1"}, {DependencyID: "dep-2"}}, 1); err == nil || !strings.Contains(err.Error(), "earlier step") {
		t.Fatalf("future dep err = %v, want earlier step", err)
	}
}

func TestTimelineAdditionalBoundaryBranches(t *testing.T) {
	t1 := "2026-01-01T00:00:00Z"
	t2 := "2026-01-01T00:00:01Z"
	prompt := classifyWorkflowUserPrompt("请推进这个目标。\n总体目标：ship\n用户请求：build it")
	if !prompt.isInternal || prompt.isHidden || prompt.userMessage != "build it" || prompt.objective != "ship" {
		t.Fatalf("goal workflow prompt = %+v", prompt)
	}
	hidden := classifyWorkflowUserPrompt("请判断是否完成目标")
	if !hidden.isInternal || !hidden.isHidden {
		t.Fatalf("hidden prompt = %+v, want hidden internal", hidden)
	}
	if got := extractWorkflowPromptField("no marker", "missing:", ""); got != "" {
		t.Fatalf("missing prompt field = %q, want empty", got)
	}
	runs := []Run{
		{ID: "old", UserMessage: "build it", Objective: "ship", CreatedAt: t1, UpdatedAt: t1},
		{ID: "new", UserMessage: "build it", Objective: "ship", CreatedAt: t2, UpdatedAt: t2},
	}
	if run, ok := matchWorkflowPromptRun(prompt, runs); !ok || run.ID != "new" {
		t.Fatalf("matched run = %+v ok=%v, want newest", run, ok)
	}
	if _, ok := matchWorkflowPromptRun(workflowUserPrompt{isInternal: true, isHidden: true, userMessage: "build it"}, runs); ok {
		t.Fatal("hidden workflow prompt should not match")
	}
	session := Session{ID: "timeline-session"}
	messages := []TranscriptEntry{
		{ID: "hidden", SessionID: session.ID, Role: "user", Content: "请判断是否完成目标", CreatedAt: t1},
		{ID: "internal", SessionID: session.ID, Role: "user", Content: "请推进这个目标。\n总体目标：ship\n用户请求：build it", CreatedAt: t1},
		{ID: "dup-visible", SessionID: session.ID, RunID: "new", Role: "user", Content: "processed", CreatedAt: t2},
		{ID: "assistant-loose", SessionID: session.ID, Role: "assistant", Content: " loose final ", ReasoningContent: " loose reasoning ", CreatedAt: t2},
	}
	notice := TimelineEntry{ID: "notice", Kind: "", Text: "notice text", CreatedAt: t1, Status: "streaming"}
	entries := buildSessionTimeline(session, messages, runs, []TimelineEntry{notice, TimelineEntry{ID: "blank", Text: "   "}})
	var sawNotice, sawOriginal, sawLooseReasoning, sawLooseFinal bool
	for _, entry := range entries {
		switch {
		case entry.ID == "notice" && entry.Kind == TimelineKindContextNotice && entry.Status == "streaming":
			sawNotice = true
		case entry.Kind == TimelineKindUserMessage && entry.RunID == "new" && entry.Text == "build it" && entry.ProcessedText != "":
			sawOriginal = true
		case entry.ID == "assistant-loose:reasoning" && entry.Text == "loose reasoning":
			sawLooseReasoning = true
		case entry.ID == "assistant-loose" && entry.Text == "loose final":
			sawLooseFinal = true
		case entry.ID == "hidden":
			t.Fatal("hidden prompt should not be emitted")
		case entry.ID == "dup-visible":
			t.Fatal("duplicate visible user message should not be emitted")
		}
	}
	if !sawNotice || !sawOriginal || !sawLooseReasoning || !sawLooseFinal {
		t.Fatalf("timeline entries missing expected items: notice=%v original=%v reasoning=%v final=%v entries=%#v", sawNotice, sawOriginal, sawLooseReasoning, sawLooseFinal, entries)
	}
	run := Run{
		ID: "activity", CreatedAt: t2, UpdatedAt: t2,
		ToolCalls: []ToolCall{
			{ID: "tool-2", CreatedAt: t2, ToolName: "b"},
			{ID: "tool-1", CreatedAt: t1, ToolName: "a"},
		},
		PendingApprovals: []Approval{
			{ID: "approval-2", CreatedAt: t2, Status: ApprovalStatusPending},
			{ID: "approval-1", CreatedAt: t1, Status: ApprovalStatusPending},
			{ID: "approval-done", CreatedAt: t1, Status: ApprovalStatusApproved},
		},
		PreToolContent: "pre content", PreToolReasoning: "pre reasoning",
	}
	orphan := timelinePrimitivesForOrphanRun(session.ID, run)
	grouped := groupTimelinePrimitives(orphan)
	var toolGroup, approvalGroup *TimelineEntry
	for index := range grouped {
		switch grouped[index].Kind {
		case TimelineKindToolGroup:
			if toolGroup == nil {
				toolGroup = &grouped[index]
			}
		case TimelineKindApprovalGroup:
			if approvalGroup == nil {
				approvalGroup = &grouped[index]
			}
		}
	}
	if toolGroup == nil || len(toolGroup.ToolCalls) != 1 || toolGroup.ToolCalls[0].ID != "tool-1" {
		t.Fatalf("first tool group = %+v, want earliest tool call", toolGroup)
	}
	if approvalGroup == nil || len(approvalGroup.Approvals) != 1 || approvalGroup.Approvals[0].ID != "approval-1" {
		t.Fatalf("first approval group = %+v, want earliest pending approval", approvalGroup)
	}
	merged := groupTimelinePrimitives([]timelinePrimitive{
		{id: "tool:a", sessionID: session.ID, runID: "merge", kind: TimelineKindToolGroup, createdAt: t1, order: 40, toolCall: &ToolCall{ID: "a"}},
		{id: "tool:b", sessionID: session.ID, runID: "merge", kind: TimelineKindToolGroup, createdAt: t1, order: 40, toolCall: &ToolCall{ID: "b"}},
		{id: "approval:a", sessionID: session.ID, runID: "merge", kind: TimelineKindApprovalGroup, createdAt: t1, order: 50, approval: &Approval{ID: "a"}},
		{id: "approval:b", sessionID: session.ID, runID: "merge", kind: TimelineKindApprovalGroup, createdAt: t1, order: 50, approval: &Approval{ID: "b"}},
	})
	if len(merged) != 2 || len(merged[0].ToolCalls) != 2 || len(merged[1].Approvals) != 2 {
		t.Fatalf("merged primitives = %#v, want grouped tools and approvals", merged)
	}
	if got := runTextAnchor(Run{}, ""); got == "" {
		t.Fatal("empty runTextAnchor should fall back to nowString")
	}
	if got := stripTimelinePrefix("prefix rest", "prefix"); got != "rest" {
		t.Fatalf("stripTimelinePrefix partial = %q, want rest", got)
	}
	if got := stripTimelinePrefix("same", "same"); got != "" {
		t.Fatalf("stripTimelinePrefix exact = %q, want empty", got)
	}
	if !compareTimelineKeys("bad-a", 2, "b", "bad-b", 1, "a") {
		t.Fatal("invalid time keys should fall back to lexical time before order")
	}
	if compareTimelineKeys("", 1, "b", t1, 1, "a") {
		t.Fatal("valid right timestamp should sort before empty left timestamp")
	}
}

func TestGoogleADKWorkflowInputResponseBoundaryBranches(t *testing.T) {
	if got := googleADKWorkflowInputToUserContent(nil); got != nil {
		t.Fatalf("nil input content = %#v, want nil", got)
	}
	if got := googleADKWorkflowInputToUserContent(""); got != nil {
		t.Fatalf("empty input content = %#v, want nil", got)
	}
	content := genai.NewContentFromText("hello", genai.RoleUser)
	if got := googleADKWorkflowInputToUserContent(content); got != content {
		t.Fatal("content input should be returned unchanged")
	}
	if got := googleADKWorkflowInputToUserContent(func() {}); got != nil {
		t.Fatalf("unmarshalable input content = %#v, want nil", got)
	}
	jsonContent := googleADKWorkflowInputToUserContent(map[string]any{"a": 1})
	if jsonContent == nil || len(jsonContent.Parts) != 1 || !strings.Contains(jsonContent.Parts[0].Text, `"a":1`) {
		t.Fatalf("json input content = %+v", jsonContent)
	}
	mixed := genai.NewContentFromParts([]*genai.Part{
		nil,
		{Text: "ignore"},
		{FunctionResponse: &genai.FunctionResponse{ID: "", Name: adkworkflow.WorkflowInputFunctionCallName, Response: map[string]any{"response": "ignored"}}},
		{FunctionResponse: &genai.FunctionResponse{ID: "ask", Name: adkworkflow.WorkflowInputFunctionCallName, Response: map[string]any{"response": `{"ok":true}`}}},
		{FunctionResponse: &genai.FunctionResponse{ID: "approval", Name: toolconfirmation.FunctionCallName, Response: map[string]any{"payload": map[string]any{"confirmed": true}}}},
	}, genai.RoleUser)
	if !googleADKWorkflowHasFunctionResponse(mixed) {
		t.Fatal("mixed content should have function response")
	}
	inputs := googleADKWorkflowInputResponses(mixed)
	if decoded, ok := inputs["ask"].(map[string]any); !ok || decoded["ok"] != true {
		t.Fatalf("workflow input responses = %#v, want decoded ask response", inputs)
	}
	state := &adkworkflow.RunState{Nodes: map[string]*adkworkflow.NodeState{
		"nil":       nil,
		"completed": {Status: adkworkflow.NodeCompleted, Interrupts: []string{"done"}},
		"waiting":   {Status: adkworkflow.NodeWaiting, Interrupts: []string{"ask", ""}},
	}}
	resume := googleADKWorkflowResumeResponses(mixed, state, nil)
	if _, ok := resume["ask"]; !ok {
		t.Fatalf("resume responses = %#v, want ask", resume)
	}
	if _, ok := resume["approval"]; ok {
		t.Fatalf("resume responses = %#v, did not expect approval without open session call", resume)
	}
	ctx := context.Background()
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	request := adksession.NewEvent(ctx, "invocation")
	request.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{ID: "approval", Name: toolconfirmation.FunctionCallName}}}, genai.RoleModel)
	request.LongRunningToolIDs = []string{"approval", ""}
	if err := service.AppendEvent(ctx, created.Session, request); err != nil {
		t.Fatalf("Append request: %v", err)
	}
	sess := created.Session
	open := googleADKWorkflowOpenLongRunningCallIDs(sess)
	if _, ok := open["approval"]; !ok {
		t.Fatalf("open long-running ids = %#v, want approval", open)
	}
	resume = googleADKWorkflowResumeResponses(mixed, nil, sess)
	if decoded, ok := resume["approval"].(map[string]any); !ok || decoded["confirmed"] != true {
		t.Fatalf("long-running resume responses = %#v, want approval payload", resume)
	}
	answeredBefore := googleADKWorkflowAnsweredOpenInterrupts(sess)
	if answeredBefore["approval"] {
		t.Fatalf("answered before response = %#v, want false", answeredBefore)
	}
	response := adksession.NewEvent(ctx, "invocation")
	response.Content = genai.NewContentFromParts([]*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: "approval", Name: toolconfirmation.FunctionCallName, Response: map[string]any{"confirmed": true},
	}}}, genai.RoleUser)
	if err := service.AppendEvent(ctx, sess, response); err != nil {
		t.Fatalf("Append response: %v", err)
	}
	answered := googleADKWorkflowAnsweredOpenInterrupts(sess)
	if !answered["approval"] {
		t.Fatalf("answered ids = %#v, want approval", answered)
	}
	if open := googleADKWorkflowOpenLongRunningCallIDs(sess); len(open) != 0 {
		t.Fatalf("open long-running after response = %#v, want empty", open)
	}
	if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"response": "plain text"}}); got != "plain text" {
		t.Fatalf("plain response decode = %#v, want plain text", got)
	}
	if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"other": "value"}}); fmt.Sprint(got) == "" {
		t.Fatalf("fallback response decode = %#v, want response map", got)
	}
	if got := googleADKWorkflowInputResponses(nil); got != nil {
		t.Fatalf("nil input responses = %#v, want nil", got)
	}
	if got := googleADKWorkflowResumeResponses(genai.NewContentFromText("none", genai.RoleUser), nil, nil); got != nil {
		t.Fatalf("no pending resume responses = %#v, want nil", got)
	}
	var nilSession adksession.Session
	if open := googleADKWorkflowOpenLongRunningCallIDs(nilSession); len(open) != 0 {
		t.Fatalf("nil session open ids = %#v, want empty", open)
	}
	if answered := googleADKWorkflowAnsweredOpenInterrupts(nilSession); len(answered) != 0 {
		t.Fatalf("nil session answered ids = %#v, want empty", answered)
	}
}

func TestSkillRegistryArchiveAndFilesystemBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := (&SkillRegistry{}).InstallURL(ctx, "ftp://example.com/skill.md"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unavailable InstallURL err = %v, want unavailable", err)
	}
	dir := t.TempDir()
	registry := &SkillRegistry{skillsPath: dir}
	if _, err := registry.InstallURL(ctx, "ftp://example.com/skill.md"); err == nil || !strings.Contains(err.Error(), "valid http/https") {
		t.Fatalf("invalid InstallURL err = %v, want URL validation", err)
	}
	rawBundle, err := buildSingleFileBuiltinSkill("zip-skill", "Zip skill", "Use zip skill.", []string{"tool.one"}, "1")
	if err != nil {
		t.Fatalf("buildSingleFileBuiltinSkill: %v", err)
	}
	skill, err := registry.installArchive(ctx, "https://example.com/zip-skill.zip", zipSkillArchive(t, map[string]string{
		"bundle/SKILL.md": rawBundle["SKILL.md"],
		"bundle/data.txt": "resource",
	}))
	if err != nil {
		t.Fatalf("installArchive success: %v", err)
	}
	if skill.ID != "zip-skill" || skill.Source != "https://example.com/zip-skill.zip" {
		t.Fatalf("installed archive skill = %+v", skill)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/unsafe.zip", zipSkillArchive(t, map[string]string{"../SKILL.md": rawBundle["SKILL.md"]})); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unsafe archive err = %v, want unsafe path", err)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/missing.zip", zipSkillArchive(t, map[string]string{"README.md": "missing"})); err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("missing archive err = %v, want no SKILL.md", err)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/dupe.zip", zipSkillArchive(t, map[string]string{
		"a/SKILL.md": rawBundle["SKILL.md"],
		"b/SKILL.md": rawBundle["SKILL.md"],
	})); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("duplicate archive err = %v, want exactly one", err)
	}
	if _, err := registry.installArchive(ctx, "https://example.com/bad.zip", []byte("not zip")); err == nil || !strings.Contains(err.Error(), "parse skill archive") {
		t.Fatalf("bad archive err = %v, want parse error", err)
	}

	source := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(t.TempDir(), "dst")
	if err := copyDirectoryContents(source, target); err != nil {
		t.Fatalf("copyDirectoryContents success: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(target, "nested", "file.txt")); err != nil || string(raw) != "content" {
		t.Fatalf("copied file raw=%q err=%v", raw, err)
	}
	blockDirTarget := filepath.Join(t.TempDir(), "block-dir")
	if err := os.MkdirAll(blockDirTarget, 0o755); err != nil {
		t.Fatalf("mkdir block target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockDirTarget, "nested"), []byte("file blocks dir"), 0o644); err != nil {
		t.Fatalf("write block file: %v", err)
	}
	if err := copyDirectoryContents(source, blockDirTarget); err == nil {
		t.Fatal("copyDirectoryContents with target file blocking directory err = nil, want error")
	}
	sourceFile := filepath.Join(t.TempDir(), "src-file")
	if err := os.WriteFile(filepath.Join(sourceFile), []byte("not dir"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := copyDirectoryContents(sourceFile, filepath.Join(t.TempDir(), "dst-file")); err != nil {
		t.Fatalf("copyDirectoryContents source root file should be ignored: %v", err)
	}

	bundle := map[string]string{"SKILL.md": rawBundle["SKILL.md"], "resources/info.txt": "info"}
	replaceTarget := filepath.Join(t.TempDir(), "replace-skill")
	if err := replaceDirectoryWithBundle(replaceTarget, bundle); err != nil {
		t.Fatalf("replaceDirectoryWithBundle new: %v", err)
	}
	if !directoryMatchesBundle(replaceTarget, bundle) {
		t.Fatal("new replaced directory should match bundle")
	}
	changed := map[string]string{"SKILL.md": rawBundle["SKILL.md"], "resources/info.txt": "changed"}
	if err := replaceDirectoryWithBundle(replaceTarget, changed); err != nil {
		t.Fatalf("replaceDirectoryWithBundle existing: %v", err)
	}
	if !directoryMatchesBundle(replaceTarget, changed) || directoryMatchesBundle(replaceTarget, bundle) {
		t.Fatal("existing replaced directory should match changed bundle only")
	}
	if err := replaceDirectoryWithBundle(filepath.Join(t.TempDir(), "unsafe-bundle"), map[string]string{"../bad": "x"}); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe bundle err = %v, want unsafe", err)
	}
	if directoryMatchesBundle(filepath.Join(t.TempDir(), "missing"), bundle) {
		t.Fatal("missing directory should not match bundle")
	}

	if _, _, err := registry.installSkillDocument("", []byte("x")); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("blank installSkillDocument err = %v, want required", err)
	}
	docPath, existed, err := registry.installSkillDocument("doc-skill", []byte(rawBundle["SKILL.md"]))
	if err != nil || existed || !strings.HasSuffix(docPath, "SKILL.md") {
		t.Fatalf("installSkillDocument success path=%q existed=%v err=%v", docPath, existed, err)
	}
	if _, existed, err := registry.installSkillDocument("doc-skill", []byte(rawBundle["SKILL.md"])); err == nil || !existed {
		t.Fatalf("duplicate installSkillDocument existed=%v err=%v, want existed error", existed, err)
	}
	invalidLegacyDir := filepath.Join(dir, strategypinespec.LegacyBuiltinSkillName)
	if err := os.MkdirAll(invalidLegacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(invalidLegacyDir, "SKILL.md"), []byte("not frontmatter"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if err := registry.removeLegacyBuiltinSkill(strategypinespec.LegacyBuiltinSkillName); err != nil {
		t.Fatalf("invalid legacy removal should be ignored: %v", err)
	}
	if err := registry.syncBuiltinSkill("custom-preserved", map[string]string{"SKILL.md": rawBundle["SKILL.md"]}); err != nil {
		t.Fatalf("syncBuiltinSkill missing: %v", err)
	}
	customDir := filepath.Join(dir, "custom-preserved")
	customDoc := filepath.Join(customDir, "SKILL.md")
	customRaw := strings.Replace(rawBundle["SKILL.md"], "source: builtin", "source: user", 1)
	if err := os.WriteFile(customDoc, []byte(customRaw), 0o644); err != nil {
		t.Fatalf("write custom doc: %v", err)
	}
	if err := registry.syncBuiltinSkill("custom-preserved", map[string]string{"SKILL.md": rawBundle["SKILL.md"]}); err != nil {
		t.Fatalf("syncBuiltinSkill custom should be preserved: %v", err)
	}
	if raw, err := os.ReadFile(customDoc); err != nil || !strings.Contains(string(raw), "source: user") {
		t.Fatalf("custom doc raw=%q err=%v, want preserved user source", raw, err)
	}
}

func TestOpenAIMessageNormalizationAndStreamingBoundaryBranches(t *testing.T) {
	long := strings.Repeat("你", 10)
	truncated := truncateBytes(long, 8)
	if !strings.Contains(truncated, "truncated") || strings.Contains(truncated, "\ufffd") {
		t.Fatalf("truncateBytes multibyte = %q, want marker without replacement", truncated)
	}
	callA := openAITestToolCall("a", "tool_a")
	callB := openAITestToolCall("b", "tool_b")
	normalized := normalizeMessagesForProvider([]openAIChatMessage{
		{Role: " user ", Content: "hi"},
		{Role: "assistant", ToolCalls: []openAIToolCall{callA, callB}},
		{Role: "user", Content: "interrupt active pairing"},
		{Role: "tool", ToolCallID: "b", Content: "late b"},
		{Role: "tool", ToolCallID: "a", Content: "late a"},
		{Role: "tool", ToolCallID: "", Content: "drop blank"},
		{Role: "tool", ToolCallID: "missing", Content: "drop missing"},
		{Role: "assistant", ToolCalls: []openAIToolCall{openAITestToolCall("", "ignored")}},
	})
	if len(normalized) != 7 {
		t.Fatalf("normalized messages len=%d messages=%#v, want 7", len(normalized), normalized)
	}
	if normalized[0].Role != "user" {
		t.Fatalf("normalized user role = %q, want trimmed user", normalized[0].Role)
	}
	if normalized[1].Role != "user" || normalized[1].Content != "interrupt active pairing" {
		t.Fatalf("interrupt message = %#v", normalized[1])
	}
	if normalized[2].Role != "assistant" || len(normalized[2].ToolCalls) != 1 || normalized[2].ToolCalls[0].ID != "b" {
		t.Fatalf("inserted assistant for b = %#v", normalized[2])
	}
	if normalized[4].Role != "assistant" || len(normalized[4].ToolCalls) != 1 || normalized[4].ToolCalls[0].ID != "a" {
		t.Fatalf("inserted assistant for a = %#v", normalized[4])
	}
	trimmed := trimMessagesForProvider([]openAIChatMessage{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("x", 50000)},
		{Role: "assistant", Content: "older"},
		{Role: "user", Content: "newer"},
	}, 300)
	if len(trimmed) == 0 || trimmed[0].Role != "system" {
		t.Fatalf("trimmed messages = %#v, want system retained", trimmed)
	}
	c := openAIClient{}
	if _, err := c.readStreamingResponse(strings.NewReader("data: {bad-json}\n\n"), nil); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("bad stream err = %v, want decode", err)
	}
	if _, err := c.readStreamingResponse(strings.NewReader("data: {\"error\":{\"message\":\"boom\"}}\n\n"), nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error stream err = %v, want boom", err)
	}
	if _, err := c.readStreamingResponse(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"\"}}]}\n\ndata: [DONE]\n\n"), nil); err == nil || !strings.Contains(err.Error(), "empty reply") {
		t.Fatalf("empty stream err = %v, want empty reply", err)
	}
	var deltas []ChatDelta
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"think "}}]}`,
		`data: {"choices":[{"message":{"content":"answer"}}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	result, err := c.readStreamingResponse(strings.NewReader(stream), func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || result.Reply != "answer" || result.ReasoningContent != "think" || len(deltas) == 0 {
		t.Fatalf("stream result=%+v deltas=%#v err=%v", result, deltas, err)
	}
	if err := appendStreamChoice(&legacyAssistantContentSplitter{}, &strings.Builder{}, &strings.Builder{}, "x", "", "", func(ChatDelta) error {
		return errors.New("delta failed")
	}); err == nil || !strings.Contains(err.Error(), "delta failed") {
		t.Fatalf("appendStreamChoice err = %v, want delta failed", err)
	}
}

func TestWorkflowApprovalParentChildBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-approval-agent", Name: "Workflow Approval Agent", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop, PermissionMode: PermissionModeLessApproval,
	})
	session := mustCreateSession(t, runtime, agent.ID, "workflow approvals")
	now := nowString()
	newParent := func(id string, mode string) Run {
		return mustSaveRun(t, runtime, Run{
			ID: id, SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: mode, WorkflowStatus: workflowStatusRunning,
			PermissionMode: PermissionModeApproval, UserMessage: "do workflow", Objective: "finish workflow",
			WorkflowPlan: []WorkflowStepState{{Title: "child step", Status: "IN_PROGRESS", ChildRunID: id + "-child", TaskID: id + "-task"}},
			CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
		})
	}
	saveChild := func(parent Run, status string, message string) Run {
		return mustSaveRun(t, runtime, Run{
			ID: parent.ID + "-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
			Status: status, WorkMode: WorkModeChat, Message: message, FailureReason: "",
			PendingApprovals: []Approval{
				{ID: parent.ID + "-pending", RunID: parent.ID + "-child", Status: ApprovalStatusPending, ToolName: "write"},
				{ID: parent.ID + "-approved", RunID: parent.ID + "-child", Status: ApprovalStatusApproved, ToolName: "read"},
			},
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{ToolCallsTotal: 1},
		})
	}

	pendingParent := newParent("wf-pending-parent", WorkModeTask)
	pendingChild := saveChild(pendingParent, RunStatusPending, "waiting approval")
	synced, err := runtime.syncParentWorkflowFromChild(ctx, pendingChild)
	if err != nil || synced == nil || synced.Status != RunStatusPending || synced.WorkflowStatus != workflowStatusPaused || len(synced.PendingApprovals) != 1 {
		t.Fatalf("sync pending parent=%+v err=%v", synced, err)
	}
	continued, err := runtime.continueParentWorkflowAfterChild(ctx, pendingChild)
	if err != nil || continued == nil || continued.Status != RunStatusPending {
		t.Fatalf("continue pending parent=%+v err=%v", continued, err)
	}

	runningParent := newParent("wf-running-parent", WorkModeTask)
	runningChild := saveChild(runningParent, RunStatusRunning, "child running")
	synced, err = runtime.syncParentWorkflowFromChild(ctx, runningChild)
	if err != nil || synced == nil || synced.Status != RunStatusRunning || synced.WorkflowStatus != workflowStatusRunning {
		t.Fatalf("sync running parent=%+v err=%v", synced, err)
	}

	for _, tc := range []struct {
		status     string
		wantReason string
		wantCode   string
		cancelled  bool
	}{
		{status: RunStatusDenied, wantReason: "approval was denied", wantCode: "APPROVAL_DENIED"},
		{status: RunStatusCancelled, wantReason: "cancelled", cancelled: true},
		{status: RunStatusTimedOut, wantReason: "timed out"},
		{status: RunStatusFailed, wantReason: "failed"},
	} {
		parent := newParent("wf-terminal-"+tc.status, WorkModeTask)
		child := saveChild(parent, tc.status, "child "+tc.status)
		terminated := runtime.terminateParentWorkflowFromChild(ctx, parent, child)
		if terminated.Status != tc.status || !strings.Contains(terminated.FailureReason, tc.wantReason) || terminated.CompletedAt == nil {
			t.Fatalf("terminated %s = %+v", tc.status, terminated)
		}
		if tc.wantCode != "" && terminated.ErrorCode != tc.wantCode {
			t.Fatalf("terminated code %s = %q, want %q", tc.status, terminated.ErrorCode, tc.wantCode)
		}
		if tc.cancelled && terminated.CancelledAt == nil {
			t.Fatalf("cancelled parent = %+v, want CancelledAt", terminated)
		}
		continued, err = runtime.continueParentWorkflowAfterChild(ctx, child)
		if err != nil || continued == nil || continued.Status != tc.status {
			t.Fatalf("continue terminal %s parent=%+v err=%v", tc.status, continued, err)
		}
	}

	completedParent := newParent("wf-loop-complete-parent", WorkModeLoop)
	completedChild := saveChild(completedParent, RunStatusCompleted, "child done")
	completed, err := runtime.continueParentWorkflowAfterChild(ctx, completedChild)
	if err != nil || completed == nil || completed.Status != RunStatusCompleted || completed.WorkflowStatus != workflowStatusComplete || completed.FinalMessageID == "" {
		t.Fatalf("continue completed loop parent=%+v err=%v", completed, err)
	}
	resumeSession, resumeAgent, err := runtime.workflowResumeContext(ctx, completedParent)
	if err != nil || resumeSession.ID != session.ID || resumeAgent.ID != agent.ID || resumeAgent.WorkMode != WorkModeLoop || resumeAgent.PermissionMode != PermissionModeApproval {
		t.Fatalf("workflowResumeContext session=%+v agent=%+v err=%v", resumeSession, resumeAgent, err)
	}

	missingSessionParent := newParent("wf-missing-session-parent", WorkModeLoop)
	missingSessionParent.SessionID = "missing-session"
	mustSaveRun(t, runtime, missingSessionParent)
	missingChild := saveChild(missingSessionParent, RunStatusCompleted, "done with missing session")
	failed, err := runtime.continueParentWorkflowAfterChild(ctx, missingChild)
	if err != nil || failed == nil || failed.Status != RunStatusFailed || failed.ErrorCode == "" {
		t.Fatalf("continue missing session parent=%+v err=%v", failed, err)
	}

	pauseRequested := newParent("wf-pause-request-parent", WorkModeLoop)
	pauseAt := nowString()
	pauseRequested.PauseRequestedAt = &pauseAt
	mustSaveRun(t, runtime, pauseRequested)
	pauseChild := saveChild(pauseRequested, RunStatusCompleted, "done while pause requested")
	paused, err := runtime.syncParentWorkflowFromChild(ctx, pauseChild)
	if err != nil || paused == nil || paused.Status != RunStatusPaused || paused.ResumeState != "user_paused" {
		t.Fatalf("sync pause requested parent=%+v err=%v", paused, err)
	}
	continued, err = runtime.continueParentWorkflowAfterChild(ctx, pauseChild)
	if err != nil || continued == nil || continued.Status != RunStatusPaused || continued.PausedReason != "user" {
		t.Fatalf("continue pause requested parent=%+v err=%v", continued, err)
	}

	chatParent := mustSaveRun(t, runtime, Run{
		ID: "wf-chat-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now,
	})
	chatChild := saveChild(chatParent, RunStatusRunning, "chat child")
	if synced, err := runtime.syncParentWorkflowFromChild(ctx, chatChild); err != nil || synced != nil {
		t.Fatalf("sync chat parent=%+v err=%v, want nil", synced, err)
	}
	if synced, err := ((*Runtime)(nil)).syncParentWorkflowFromChild(ctx, chatChild); err != nil || synced != nil {
		t.Fatalf("nil runtime sync parent=%+v err=%v, want nil", synced, err)
	}
}

func TestRunnerChatAndStoreAdditionalBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := ((*Runtime)(nil)).prepareChatRequest(ctx, ChatRequest{Message: "hello"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil prepareChatRequest err = %v, want unavailable", err)
	}
	runtime := newTestRuntime(t)
	if _, err := runtime.prepareChatRequest(ctx, ChatRequest{Message: "   "}); err == nil || !strings.Contains(err.Error(), "message is required") {
		t.Fatalf("empty prepareChatRequest err = %v, want message required", err)
	}
	if _, err := runtime.prepareChatRequest(ctx, ChatRequest{Message: strings.Repeat("x", MaxMessageLength+1)}); err == nil || !strings.Contains(err.Error(), "maximum length") {
		t.Fatalf("long prepareChatRequest err = %v, want max length", err)
	}
	for range MaxConcurrentRuns {
		runtime.runSem <- struct{}{}
	}
	if _, err := runtime.prepareChatRequest(ctx, ChatRequest{Message: "busy"}); err == nil || !strings.Contains(err.Error(), "maximum concurrent runs") {
		t.Fatalf("busy prepareChatRequest err = %v, want maximum concurrent", err)
	}
	for range MaxConcurrentRuns {
		<-runtime.runSem
	}
	if _, err := runtime.runChat(ctx, ChatRequest{Message: "hello", WorkModeOverride: "bad-mode"}, nil, false); err == nil || !strings.Contains(err.Error(), "invalid work mode") {
		t.Fatalf("invalid work mode err = %v, want invalid", err)
	}
	if _, err := runtime.runChat(ctx, ChatRequest{Message: "hello", AgentID: "missing-agent"}, nil, false); err == nil {
		t.Fatal("missing agent runChat err = nil, want error")
	}

	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "runner-boundary-agent", Name: "Runner Boundary", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat, PermissionMode: PermissionModeLessApproval,
	})
	session := mustCreateSession(t, runtime, agent.ID, "runner boundary")
	now := nowString()
	baseRun := mustSaveRun(t, runtime, Run{
		ID: "runner-boundary-run", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	pendingApproval := Approval{ID: "runner-boundary-approval", RunID: baseRun.ID, AgentID: agent.ID, ToolName: "write", Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now}
	pendingResponse, err := runtime.completeChatRun(ctx, session, baseRun, "approve", toolExecutionContext{}, []Approval{pendingApproval}, openAIChatResult{}, nil)
	if err != nil || pendingResponse.Run.Status != RunStatusPending || len(pendingResponse.PendingApprovals) != 1 {
		t.Fatalf("pending completeChatRun response=%+v err=%v", pendingResponse, err)
	}
	failedRun := baseRun
	failedRun.ID = "runner-boundary-failed"
	failedRun.Status = RunStatusRunning
	mustSaveRun(t, runtime, failedRun)
	failedResponse, err := runtime.completeChatRun(ctx, session, failedRun, "fail", toolExecutionContext{}, nil, openAIChatResult{}, fmt.Errorf("model failed"))
	if err != nil || failedResponse.Run.Status != RunStatusFailed || failedResponse.Run.ErrorCode == "" || failedResponse.Reply == "" {
		t.Fatalf("failed completeChatRun response=%+v err=%v", failedResponse, err)
	}
	completedRun := baseRun
	completedRun.ID = "runner-boundary-completed"
	completedRun.Status = RunStatusRunning
	completedRun.ToolCalls = []ToolCall{{ID: "call-failed", RunID: completedRun.ID, ToolName: "tool", Status: "FAILED", Error: new("tool failed")}}
	mustSaveRun(t, runtime, completedRun)
	completedResponse, err := runtime.completeChatRun(ctx, session, completedRun, "done", toolExecutionContext{calls: completedRun.ToolCalls}, nil, openAIChatResult{}, nil)
	if err != nil || completedResponse.Run.Status != RunStatusCompleted || !completedResponse.Run.Degraded || !strings.Contains(completedResponse.Reply, "tool failed") {
		t.Fatalf("completed degraded response=%+v err=%v", completedResponse, err)
	}

	if err := runtime.Store().DeleteSession(ctx, ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteSession blank err = %v, want not exist", err)
	}
	created, createdNew, err := runtime.Store().SaveApprovalIfConfirmationAbsent(ctx, Approval{
		ID: "confirmation-dedup-1", RunID: baseRun.ID, AgentID: agent.ID, ToolName: "write",
		Status: ApprovalStatusPending, ConfirmationCallID: "confirm-1",
	})
	if err != nil || !createdNew || created.ID != "confirmation-dedup-1" {
		t.Fatalf("SaveApprovalIfConfirmationAbsent created=%+v new=%v err=%v", created, createdNew, err)
	}
	existing, createdNew, err := runtime.Store().SaveApprovalIfConfirmationAbsent(ctx, Approval{
		ID: "confirmation-dedup-2", RunID: baseRun.ID, AgentID: agent.ID, ToolName: "write",
		Status: ApprovalStatusPending, ConfirmationCallID: "confirm-1",
	})
	if err != nil || createdNew || existing.ID != "confirmation-dedup-1" {
		t.Fatalf("SaveApprovalIfConfirmationAbsent existing=%+v new=%v err=%v", existing, createdNew, err)
	}
	if _, ok, err := runtime.Store().ApprovalByConfirmationCallID(ctx, ""); err != nil || ok {
		t.Fatalf("blank ApprovalByConfirmationCallID ok=%v err=%v, want false nil", ok, err)
	}
	resolved, changed, err := runtime.Store().ResolvePendingApproval(ctx, created.ID, ApprovalStatusApproved)
	if err != nil || !changed || resolved.Status != ApprovalStatusApproved {
		t.Fatalf("ResolvePendingApproval changed=%v approval=%+v err=%v", changed, resolved, err)
	}
	resolvedAgain, changed, err := runtime.Store().ResolvePendingApproval(ctx, created.ID, ApprovalStatusDenied)
	if err != nil || changed || resolvedAgain.Status != ApprovalStatusApproved {
		t.Fatalf("ResolvePendingApproval again changed=%v approval=%+v err=%v", changed, resolvedAgain, err)
	}
}

func TestResumeGoogleADKFakeExecutionBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "resume-google-agent", Name: "Resume Google", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat, PermissionMode: PermissionModeApproval,
	})
	session := mustCreateSession(t, runtime, agent.ID, "resume google")
	appName := googleADKAppName(agent.ID)
	if _, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{AppName: appName, UserID: googleADKUserID, SessionID: session.ID}); err != nil {
		t.Fatalf("Create ADK session: %v", err)
	}
	newExecution := func(runID string, runBlocking func(*googleADKExecution, context.Context, *genai.Content) error) *googleADKExecution {
		execution := &googleADKExecution{
			sessionID: session.ID,
			appName:   appName,
			agent:     agent,
			runID:     runID,
			runIDByAgentName: map[string]string{
				googleADKAgentName(agent.ID): runID,
			},
			runSnapshotBaseByID: map[string]Run{
				runID: {ID: runID, SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning, Usage: &RunUsage{}},
			},
			descriptors:              map[string]ToolDescriptor{},
			calls:                    []ToolCall{},
			summaries:                []string{},
			replyByRunID:             map[string]*strings.Builder{},
			reasoningByRunID:         map[string]*strings.Builder{},
			bufferedReplyByRunID:     map[string]*strings.Builder{},
			bufferedReasoningByRunID: map[string]*strings.Builder{},
			toolResponseSeenByRunID:  map[string]bool{},
			postToolTextByRunID:      map[string]bool{},
			toolResponseSeqByRunID:   map[string]int{},
			postToolTextSeqByRunID:   map[string]int{},
			sessionService:           runtime.rawSessionService,
			loadRun: func(ctx context.Context, id string) (Run, bool, error) {
				return runtime.Store().Run(ctx, id)
			},
			persistRunSnapshot: func(snapshot Run) (Run, error) {
				return runtime.persistRunActivitySnapshot(context.Background(), snapshot)
			},
		}
		execution.runBlocking = func(ctx context.Context, content *genai.Content) error {
			return runBlocking(execution, ctx, content)
		}
		return execution
	}
	saveResumeRun := func(id string, status string) Run {
		return mustSaveRun(t, runtime, Run{
			ID: id, SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPending,
			UserMessage: "please resume", ResumeState: "waiting_approval",
			PendingApprovals: []Approval{{
				ID: id + "-approval", RunID: id, AgentID: agent.ID, ToolName: "write",
				Status: status, FunctionCallID: id + "-call", ConfirmationCallID: id + "-confirmation",
				Input: map[string]any{"x": id}, CreatedAt: nowString(), UpdatedAt: nowString(),
			}},
			ToolCalls: []ToolCall{{ID: id + "-call", RunID: id, ToolName: "write", Status: "PENDING"}},
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
	}

	noPartsRun := mustSaveRun(t, runtime, Run{
		ID: "resume-google-no-parts", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPending,
		PendingApprovals: []Approval{{ID: "resume-google-no-parts-approval", Status: ApprovalStatusApproved}},
		CreatedAt:        nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	runtime.adkRuns[noPartsRun.ID] = newExecution(noPartsRun.ID, func(*googleADKExecution, context.Context, *genai.Content) error {
		t.Fatal("runBlocking should not be called without approval response parts")
		return nil
	})
	unchanged, message, handled, err := runtime.resumeGoogleADK(ctx, noPartsRun)
	if err != nil || handled || message != nil || unchanged.ID != noPartsRun.ID {
		t.Fatalf("resume no parts run=%+v message=%+v handled=%v err=%v", unchanged, message, handled, err)
	}

	approvedRun := saveResumeRun("resume-google-approved", ApprovalStatusApproved)
	runtime.adkRuns[approvedRun.ID] = newExecution(approvedRun.ID, func(execution *googleADKExecution, ctx context.Context, content *genai.Content) error {
		if !googleADKWorkflowHasFunctionResponse(content) {
			t.Fatalf("approved resume content = %+v, want function response", content)
		}
		execution.markToolResponseSeenForRun(approvedRun.ID)
		execution.markPostToolTextForRun(approvedRun.ID)
		return execution.appendVisibleTextForRun(approvedRun.ID, "approved final", "")
	})
	completed, message, handled, err := runtime.resumeGoogleADK(ctx, approvedRun)
	if err != nil || !handled || message == nil || completed.Status != RunStatusCompleted {
		t.Fatalf("resume approved run=%+v message=%+v handled=%v err=%v", completed, message, handled, err)
	}
	storedCompleted, ok, err := runtime.Store().Run(ctx, approvedRun.ID)
	if err != nil || !ok || storedCompleted.FinalMessageID == "" {
		t.Fatalf("stored approved run=%+v ok=%v err=%v, want final message id", storedCompleted, ok, err)
	}
	if _, ok := runtime.adkRuns[approvedRun.ID]; ok {
		t.Fatal("approved execution should be removed from active ADK runs")
	}

	deniedRun := saveResumeRun("resume-google-denied", ApprovalStatusDenied)
	runtime.adkRuns[deniedRun.ID] = newExecution(deniedRun.ID, func(execution *googleADKExecution, ctx context.Context, content *genai.Content) error {
		execution.markToolResponseSeenForRun(deniedRun.ID)
		return nil
	})
	denied, message, handled, err := runtime.resumeGoogleADK(ctx, deniedRun)
	if err != nil || !handled || message == nil || denied.Status != RunStatusDenied || !strings.Contains(message.Content, "拒绝") {
		t.Fatalf("resume denied run=%+v message=%+v handled=%v err=%v", denied, message, handled, err)
	}

	errorRun := saveResumeRun("resume-google-error", ApprovalStatusApproved)
	runtime.adkRuns[errorRun.ID] = newExecution(errorRun.ID, func(*googleADKExecution, context.Context, *genai.Content) error {
		return errors.New("resume failed")
	})
	errored, message, handled, err := runtime.resumeGoogleADK(ctx, errorRun)
	if err == nil || !handled || message != nil || errored.ID != errorRun.ID {
		t.Fatalf("resume error run=%+v message=%+v handled=%v err=%v", errored, message, handled, err)
	}
}

func TestWorkflowExecutorAdditionalBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	if _, err := ((*WorkflowExecutor)(nil)).Run(ctx, workflowRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil workflow executor err = %v, want unavailable", err)
	}
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	if _, err := executor.Run(ctx, workflowRequest{Mode: WorkModeChat}); err == nil || !strings.Contains(err.Error(), "workflow mode") {
		t.Fatalf("chat workflow err = %v, want workflow mode required", err)
	}
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-executor-agent", Name: "Workflow Executor", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "workflow executor")
	if _, err := executor.Run(ctx, workflowRequest{
		Agent: agent, Session: session, Mode: WorkModeLoop, Message: "loop objective", Objective: "loop objective", EmitRun: true,
		OnDelta: func(ChatDelta) error { return errors.New("emit failed") },
	}); err == nil || !strings.Contains(err.Error(), "emit failed") {
		t.Fatalf("emit workflow err = %v, want emit failed", err)
	}
	if _, _, err := executor.planWorkflowSteps(ctx, workflowRequest{Agent: Agent{ID: "missing", ProviderID: "missing"}}, WorkModeTask, "objective"); err == nil || !strings.Contains(err.Error(), "workflow planner failed") {
		t.Fatalf("planWorkflowSteps err = %v, want planner failed", err)
	}

	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-executor-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	steps := []workflowStep{
		{Title: "First", Description: "Desc", Message: "First message", DependencyID: "first", Order: 1, AgentRole: "researcher", ModeHint: WorkModeChat, PlanSource: workflowPlanSourcePlanner, WorkflowMode: WorkModeTask},
		{Title: "Second", Message: "Second message", DependsOn: []string{"first", "__previous_step_1"}, DependencyID: "second", Order: 2, PlanSource: workflowPlanSourcePlanner, WorkflowMode: WorkModeTask},
	}
	tasks, err := executor.persistWorkflowTasks(ctx, parent, agent, steps)
	if err != nil {
		t.Fatalf("persistWorkflowTasks: %v", err)
	}
	if len(tasks) != 2 || tasks[1].DependsOn[0] != tasks[0].ID || !strings.Contains(tasks[0].Description, "Agent role: researcher") {
		t.Fatalf("persisted tasks = %+v", tasks)
	}
	failingParent := parent
	failingParent.ID = "workflow-executor-failing-parent"
	mustSaveRun(t, runtime, failingParent)
	response, err := executor.runPlannedGoogleADKWorkflow(ctx, workflowRequest{
		Agent:   Agent{ID: "bad-child-agent", Name: "Bad Child", ProviderID: "missing-provider"},
		Session: session, Message: "run children", Mode: WorkModeTask,
	}, failingParent, []workflowStep{{Title: "Bad", Message: "bad child"}}, nil)
	if err != nil || response.Run.Status != RunStatusFailed || response.Run.FailureReason == "" {
		t.Fatalf("runPlannedGoogleADKWorkflow response=%+v err=%v, want failed response", response, err)
	}
	if _, _, err := executor.startWorkflowChildRuns(ctx, workflowRequest{
		Agent:   Agent{ID: "bad-child-agent", Name: "Bad Child", ProviderID: "missing-provider"},
		Session: session, Message: "run child", Mode: WorkModeTask,
	}, parent, []workflowStep{{Title: "Bad", Message: "bad child"}}, nil); err == nil {
		t.Fatal("startWorkflowChildRuns bad provider err = nil, want error")
	}
	ordered := []Task{
		{ID: "zero", Order: 0, CreatedAt: "b"},
		{ID: "two", Order: 2, CreatedAt: "a"},
		{ID: "one", Order: 1, CreatedAt: "c"},
		{ID: "zero-a", Order: 0, CreatedAt: "a"},
	}
	sortWorkflowTasks(ordered)
	if got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID}; strings.Join(got, ",") != "one,two,zero-a,zero" {
		t.Fatalf("sortWorkflowTasks order = %v", got)
	}
	if got := workflowDescriptionWithoutAgentRole("Agent role: only role"); got != "" {
		t.Fatalf("workflowDescriptionWithoutAgentRole prefix = %q, want empty", got)
	}
	if got := workflowDescriptionWithoutAgentRole("body\n\nAgent role: worker"); got != "body" {
		t.Fatalf("workflowDescriptionWithoutAgentRole suffix = %q, want body", got)
	}
	if got := workflowDescriptionWithoutAgentRole("body"); got != "body" {
		t.Fatalf("workflowDescriptionWithoutAgentRole plain = %q, want body", got)
	}
}

func TestRunnerApprovalStateMachineAdditionalBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "approval-state-agent", Name: "Approval State", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat, PermissionMode: PermissionModeApproval,
	})
	session := mustCreateSession(t, runtime, agent.ID, "approval state")
	now := nowString()
	baseRun := func(id string, approvals []Approval) Run {
		return mustSaveRun(t, runtime, Run{
			ID: id, SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPending,
			ResumeState: "waiting_approval", UserMessage: "approval state",
			PendingApprovals: approvals,
			ToolCalls: []ToolCall{
				{ID: id + "-call-a", RunID: id, ToolName: "write.a", Status: "PENDING_APPROVAL", RequiresUser: true, CreatedAt: now, UpdatedAt: now},
				{ID: id + "-call-b", RunID: id, ToolName: "write.b", Status: "PENDING_APPROVAL", RequiresUser: true, CreatedAt: now, UpdatedAt: now},
			},
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})
	}
	saveApprovals := func(runID string) []Approval {
		items := []Approval{
			{ID: runID + "-approval-a", RunID: runID, AgentID: agent.ID, ToolName: "write.a", Status: ApprovalStatusPending, FunctionCallID: runID + "-call-a", ConfirmationCallID: runID + "-confirmation-a", CreatedAt: now, UpdatedAt: now},
			{ID: runID + "-approval-b", RunID: runID, AgentID: agent.ID, ToolName: "write.b", Status: ApprovalStatusPending, FunctionCallID: runID + "-call-b", ConfirmationCallID: runID + "-confirmation-b", CreatedAt: now, UpdatedAt: now},
		}
		for _, item := range items {
			if err := runtime.Store().SaveApproval(ctx, item); err != nil {
				t.Fatalf("SaveApproval %s: %v", item.ID, err)
			}
		}
		return items
	}

	if resolution, shouldContinue, err := runtime.stageResolvedApproval(ctx, Approval{ID: "missing", RunID: "missing-run"}, true); err != nil || shouldContinue || resolution.Run != nil {
		t.Fatalf("stage missing run resolution=%+v continue=%v err=%v", resolution, shouldContinue, err)
	}
	nonPending := mustSaveRun(t, runtime, Run{ID: "approval-non-pending", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusCompleted, CreatedAt: now, UpdatedAt: now})
	if resolution, shouldContinue, err := runtime.stageResolvedApproval(ctx, Approval{ID: "x", RunID: nonPending.ID}, true); err != nil || shouldContinue || resolution.Run != nil {
		t.Fatalf("stage non-pending resolution=%+v continue=%v err=%v", resolution, shouldContinue, err)
	}

	approvedItems := saveApprovals("approval-pending-sibling")
	parent := baseRun("approval-pending-sibling", approvedItems)
	approvedItems[0].Status = ApprovalStatusApproved
	resolution, shouldContinue, err := runtime.stageResolvedApproval(ctx, approvedItems[0], true)
	if err != nil || shouldContinue || resolution.Run == nil || resolution.Run.Status != RunStatusPending || !runHasPendingApproval(resolution.Run.PendingApprovals) {
		t.Fatalf("stage approved with pending sibling resolution=%+v continue=%v err=%v", resolution, shouldContinue, err)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok || stored.ToolCalls[0].Status != "PENDING_APPROVAL" {
		t.Fatalf("stored approved pending run=%+v ok=%v err=%v", stored, ok, err)
	}

	deniedItems := saveApprovals("approval-deny-siblings")
	deniedParent := baseRun("approval-deny-siblings", deniedItems)
	deniedItems[0].Status = ApprovalStatusDenied
	resolution, shouldContinue, err = runtime.stageResolvedApproval(ctx, deniedItems[0], false)
	if err != nil || !shouldContinue || resolution.Run == nil || resolution.Run.ResumeState != "approval_resuming" {
		t.Fatalf("stage denied resolution=%+v continue=%v err=%v", resolution, shouldContinue, err)
	}
	stored, ok, err = runtime.Store().Run(ctx, deniedParent.ID)
	if err != nil || !ok || stored.ToolCalls[0].Status != "DENIED" || stored.ToolCalls[1].Status != "DENIED" {
		t.Fatalf("stored denied run=%+v ok=%v err=%v", stored, ok, err)
	}
	sibling, ok, err := runtime.Store().Approval(ctx, deniedItems[1].ID)
	if err != nil || !ok || sibling.Status != ApprovalStatusDenied {
		t.Fatalf("denied sibling approval=%+v ok=%v err=%v", sibling, ok, err)
	}

	noMatchRun := baseRun("approval-no-match", nil)
	resolution, err = runtime.continueResolvedApproval(ctx, Approval{ID: "unknown", RunID: noMatchRun.ID, Status: ApprovalStatusApproved}, true)
	if err != nil || resolution.Run == nil || resolution.Run.ID != noMatchRun.ID {
		t.Fatalf("continue no match resolution=%+v err=%v", resolution, err)
	}
	unavailableItems := saveApprovals("approval-unavailable-context")
	unavailableRun := baseRun("approval-unavailable-context", unavailableItems[:1])
	unavailableItems[0].Status = ApprovalStatusApproved
	resolution, err = runtime.continueResolvedApproval(ctx, unavailableItems[0], true)
	if err != nil || resolution.Run == nil || resolution.Run.Status != RunStatusCompleted || resolution.Message == nil {
		t.Fatalf("continue rehydrated context resolution=%+v err=%v", resolution, err)
	}
	stored, ok, err = runtime.Store().Run(ctx, unavailableRun.ID)
	if err != nil || !ok || stored.Status != RunStatusCompleted || stored.PendingApprovals[0].Status != ApprovalStatusApproved {
		t.Fatalf("stored unavailable run=%+v ok=%v err=%v", stored, ok, err)
	}

	failedRun := mustSaveRun(t, runtime, Run{
		ID: "approval-continuation-failed", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		ResumeState:      "approval_resuming",
		PendingApprovals: []Approval{{ID: "approval-continuation-failed-a", Status: ApprovalStatusApproved, FunctionCallID: "call", ConfirmationCallID: "confirmation"}},
		CreatedAt:        now, UpdatedAt: now, Usage: &RunUsage{},
	})
	runtime.markApprovalContinuationFailed(ctx, failedRun.ID, errors.New("append event to SessionService: database is locked"))
	stored, ok, err = runtime.Store().Run(ctx, failedRun.ID)
	if err != nil || !ok || stored.Status != RunStatusFailed || stored.FinalMessageID == "" || stored.ErrorCode != "APPROVAL_CONTINUATION_FAILED" {
		t.Fatalf("continuation failed stored=%+v ok=%v err=%v", stored, ok, err)
	}
	unchangedPending := mustSaveRun(t, runtime, Run{
		ID: "approval-continuation-still-pending", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		ResumeState:      "approval_resuming",
		PendingApprovals: []Approval{{ID: "still-pending", Status: ApprovalStatusPending, FunctionCallID: "call", ConfirmationCallID: "confirmation"}},
		CreatedAt:        now, UpdatedAt: now, Usage: &RunUsage{},
	})
	runtime.markApprovalContinuationFailed(ctx, unchangedPending.ID, errors.New("ignored"))
	stored, ok, err = runtime.Store().Run(ctx, unchangedPending.ID)
	if err != nil || !ok || stored.Status != RunStatusRunning {
		t.Fatalf("still pending continuation stored=%+v ok=%v err=%v", stored, ok, err)
	}

	if err := runtime.continueResolvedApprovalRun(ctx, "missing-run"); err != nil {
		t.Fatalf("continueResolvedApprovalRun missing should return nil err, got %v", err)
	}
	noResolved := mustSaveRun(t, runtime, Run{ID: "approval-no-resolved", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPending, CreatedAt: now, UpdatedAt: now})
	if err := runtime.continueResolvedApprovalRun(ctx, noResolved.ID); err != nil {
		t.Fatalf("continueResolvedApprovalRun no approval: %v", err)
	}
	((*Runtime)(nil)).ReconcileResolvedApprovals(ctx)
}

func zipSkillArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := io.WriteString(writer, content); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func openAITestToolCall(id string, name string) openAIToolCall {
	call := openAIToolCall{ID: id}
	call.Function.Name = name
	call.Function.Arguments = "{}"
	return call
}
