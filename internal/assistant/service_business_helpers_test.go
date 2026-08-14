package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestServiceProviderChatAndSkillWrappers(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()

	providerResult, err := service.TestProvider(ctx, "test-provider")
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	resultMap := asMap(t, providerResult)
	if resultMap["ok"] != true {
		t.Fatalf("provider test result = %#v", resultMap)
	}
	if _, err := service.TestProvider(ctx, "provider-missing"); err == nil || !strings.Contains(err.Error(), "provider not found") {
		t.Fatalf("TestProvider missing err = %v, want provider not found", err)
	}

	agent, err := runtime.Store().SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "agent-chat-wrapper", Name: "Chat Wrapper", Status: assistantmodel.AgentStatusEnabled, ProviderID: "test-provider",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := service.Chat(ctx, assistantmodel.ChatRequest{AgentID: agent.ID, Message: "hello"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Reply == "" || response.Run.ID == "" || response.Session.ID == "" {
		t.Fatalf("chat response = %+v", response)
	}

	deltaCount := 0
	streamResponse, err := service.ChatStream(ctx, assistantmodel.ChatRequest{AgentID: agent.ID, Message: "hello stream"}, func(delta assistantmodel.ChatDelta) error {
		deltaCount++
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if deltaCount == 0 || streamResponse.Run.ID == "" {
		t.Fatalf("stream response = %+v deltaCount=%d", streamResponse, deltaCount)
	}

	skills, err := service.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("ListSkills() returned no skills")
	}

	externalDir := filepath.Join(runtime.Store().SkillsPath(), "external-skill")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll external skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, "SKILL.md"), []byte("---\nname: external-skill\ndescription: external skill\nmetadata:\n  source: https://example.com/SKILL.md\n---\nUse external references carefully.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile external skill: %v", err)
	}
	skills, err = service.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills after external add: %v", err)
	}
	foundExternal := false
	for _, skill := range skills {
		if skill.ID == "external-skill" {
			foundExternal = true
			break
		}
	}
	if !foundExternal {
		t.Fatalf("external skill not found in list: %+v", skills)
	}
	if err := service.DeleteSkill(ctx, "external-skill"); err != nil {
		t.Fatalf("DeleteSkill external: %v", err)
	}
	if _, err := os.Stat(externalDir); !os.IsNotExist(err) {
		t.Fatalf("external skill dir stat err = %v, want not exist", err)
	}
	if _, err := service.InstallSkill(ctx, "ftp://invalid-skill"); err == nil || !strings.Contains(err.Error(), "valid http/https skill URL is required") {
		t.Fatalf("InstallSkill invalid err = %v, want invalid url", err)
	}
}

func TestServiceSessionContextCompactionWrapper(t *testing.T) {
	runtime, service, sessionService := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "agent-context-wrapper", Name: "Context Wrapper", Status: assistantmodel.AgentStatusEnabled, ProviderID: "test-provider",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Context Wrapper Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := sessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   "jftrade-" + agent.ID,
		UserID:    "jftrade-user",
		SessionID: session.ID,
	}); err != nil {
		t.Fatalf("Create raw session: %v", err)
	}

	snapshot, err := service.GetSessionContext(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSessionContext: %v", err)
	}
	if snapshot.SessionID != session.ID {
		t.Fatalf("initial context snapshot = %+v", snapshot)
	}

	compacted, err := service.CompactSessionContext(ctx, session.ID, "normal", "manual", "unit test compaction")
	if err != nil {
		t.Fatalf("CompactSessionContext: %v", err)
	}
	if compacted.SessionID != session.ID || compacted.LastCompactionMode == "" {
		t.Fatalf("compacted snapshot = %+v", compacted)
	}
}

func TestServiceRuntimeUnavailableErrorBranches(t *testing.T) {
	service := NewService(nil)
	ctx := t.Context()

	if _, err := service.GetSessionContext(ctx, "session-1"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("GetSessionContext err = %v, want unavailable", err)
	}
	if _, err := service.CompactSessionContext(ctx, "session-1", "normal", "manual", "reason"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("CompactSessionContext err = %v, want unavailable", err)
	}
	if _, err := service.Chat(ctx, assistantmodel.ChatRequest{Message: "hello"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Chat err = %v, want unavailable", err)
	}
	if _, err := service.ChatStream(ctx, assistantmodel.ChatRequest{Message: "hello"}, nil); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("ChatStream err = %v, want unavailable", err)
	}
	if _, err := service.CancelRun(ctx, "run-1"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("CancelRun err = %v, want unavailable", err)
	}
	if _, err := service.PauseGoalRun(ctx, "run-1"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("PauseGoalRun err = %v, want unavailable", err)
	}
	if _, err := service.ResumeGoalRun(ctx, "run-1"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("ResumeGoalRun err = %v, want unavailable", err)
	}
	if _, err := service.ResolveApproval(ctx, "approval-1", true); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("ResolveApproval err = %v, want unavailable", err)
	}
	if _, err := service.ResolveApprovalAsync(ctx, "approval-1", false); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("ResolveApprovalAsync err = %v, want unavailable", err)
	}
	if _, err := service.InstallSkill(ctx, "https://example.com/skill"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("InstallSkill err = %v, want unavailable", err)
	}
	if err := service.DeleteSkill(ctx, "external-skill"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("DeleteSkill err = %v, want unavailable", err)
	}

	if err := service.ReconcileExpiredRuns(ctx); err != nil {
		t.Fatalf("ReconcileExpiredRuns: %v", err)
	}
	service.ReconcileResolvedApprovals(ctx)
}

func TestServiceOptimizationTaskLifecycleAndMetrics(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t, WithOptimizationRuns(newStubOptimizationRuns(
		map[string]OptimizationRun{
			"run-opt-running":   {Status: "running"},
			"run-opt-completed": {Status: "completed", Result: map[string]any{"return": 12.5}},
			"run-opt-cancelled": {Status: "cancelled"},
		},
	)))
	ctx := t.Context()

	provider, err := runtime.Store().SaveProvider(ctx, assistantmodel.ProviderWriteRequest{
		ID: "provider-metrics", DisplayName: "Metrics Provider", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	agentBound, err := runtime.Store().SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "agent-metrics-bound", Name: "Metrics Bound", Status: assistantmodel.AgentStatusEnabled, ProviderID: provider.ID,
	})
	if err != nil {
		t.Fatalf("SaveAgent bound: %v", err)
	}
	agentUnbound, err := runtime.Store().SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "agent-metrics-unbound", Name: "Metrics Unbound", Status: assistantmodel.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent unbound: %v", err)
	}

	now := time.Now().UTC()
	runCompleted := assistantmodel.Run{
		ID:          "run-metrics-completed",
		SessionID:   "session-metrics-1",
		AgentID:     agentBound.ID,
		Status:      assistantmodel.RunStatusCompleted,
		ResumeState: "adk_confirmation_resolved",
		ToolCalls: []assistantmodel.ToolCall{
			{ID: "tool-1", RunID: "run-metrics-completed", ToolName: "market.read", Status: "SUCCEEDED", DurationMs: 120, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)},
			{ID: "tool-2", RunID: "run-metrics-completed", ToolName: "strategy.write", Status: "FAILED", Output: map[string]any{
				"truncated": true,
				"error":     map[string]any{"code": "timeout", "retryable": true},
			}, DurationMs: 30, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)},
			{ID: "tool-3", RunID: "run-metrics-completed", ToolName: "market.pending", Status: "RUNNING", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)},
			{ID: "tool-4", RunID: "run-metrics-completed", ToolName: "strategy.pending", Status: "PENDING_APPROVAL", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)},
		},
		Usage:     &assistantmodel.RunUsage{TokensIn: 120, TokensOut: 60},
		CreatedAt: now.Format(time.RFC3339Nano),
		UpdatedAt: now.Format(time.RFC3339Nano),
	}
	runFailed := assistantmodel.Run{
		ID:         "run-metrics-failed",
		SessionID:  "session-metrics-2",
		AgentID:    agentBound.ID,
		Status:     assistantmodel.RunStatusFailed,
		ErrorCode:  "RUN_ORPHANED",
		ProviderID: provider.ID,
		CreatedAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
		UpdatedAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
	}
	runCancelled := assistantmodel.Run{
		ID:        "run-metrics-cancelled",
		SessionID: "session-metrics-3",
		AgentID:   agentUnbound.ID,
		Status:    assistantmodel.RunStatusCancelled,
		CreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
	}
	for _, run := range []assistantmodel.Run{runCompleted, runFailed, runCancelled} {
		if err := runtime.Store().SaveRun(ctx, run); err != nil {
			t.Fatalf("SaveRun(%s): %v", run.ID, err)
		}
	}

	pendingApproval := assistantmodel.Approval{
		ID: "approval-pending", RunID: runCompleted.ID, AgentID: agentBound.ID, ToolName: "strategy.write",
		Status: assistantmodel.ApprovalStatusPending, FunctionCallID: "call-1", ConfirmationCallID: "confirm-1",
		CreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339Nano), UpdatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
	}
	approvedApproval := assistantmodel.Approval{
		ID: "approval-approved", RunID: runCompleted.ID, AgentID: agentBound.ID, ToolName: "strategy.write",
		Status:    assistantmodel.ApprovalStatusApproved,
		CreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339Nano), UpdatedAt: now.Add(-8 * time.Minute).Format(time.RFC3339Nano),
	}
	deniedApproval := assistantmodel.Approval{
		ID: "approval-denied", RunID: runFailed.ID, AgentID: agentBound.ID, ToolName: "strategy.write",
		Status:    assistantmodel.ApprovalStatusDenied,
		CreatedAt: now.Add(-7 * time.Minute).Format(time.RFC3339Nano), UpdatedAt: now.Add(-6 * time.Minute).Format(time.RFC3339Nano),
	}
	for _, approval := range []assistantmodel.Approval{pendingApproval, approvedApproval, deniedApproval} {
		if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval(%s): %v", approval.ID, err)
		}
	}

	task, err := runtime.Store().SaveOptimizationTask(ctx, assistantmodel.OptimizationTask{
		ID: "optimization-lifecycle", Status: "queued", Objective: "maximize sharpe",
		Runs: []assistantmodel.OptimizationRunRef{
			{DefinitionID: "def-running", RunID: "run-opt-running"},
			{DefinitionID: "def-completed", RunID: "run-opt-completed"},
			{DefinitionID: "def-missing", RunID: "run-opt-missing"},
		},
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("SaveOptimizationTask: %v", err)
	}

	listed, err := service.ListOptimizationTasks(ctx)
	if err != nil {
		t.Fatalf("ListOptimizationTasks: %v", err)
	}
	if len(listed.Tasks) != 1 {
		t.Fatalf("optimization tasks len = %d, want 1", len(listed.Tasks))
	}
	taskResponse := listed.Tasks[0]
	if taskResponse["status"] != "running" {
		t.Fatalf("optimization task status = %v, want running", taskResponse["status"])
	}
	progress := asMap(t, taskResponse["progress"])
	if progress["running"] != 1 || progress["completed"] != 1 || progress["failed"] != 1 {
		t.Fatalf("optimization progress = %#v, want running=1 completed=1 failed=1", progress)
	}
	loadedTask, err := service.GetOptimizationTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetOptimizationTask: %v", err)
	}
	if asMap(t, loadedTask)["status"] != "running" {
		t.Fatalf("loaded optimization task = %#v, want running status", loadedTask)
	}

	cancelled, err := service.CancelOptimizationTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("CancelOptimizationTask: %v", err)
	}
	cancelledTask := asMap(t, cancelled)
	if cancelledTask["status"] != "cancelled" {
		t.Fatalf("cancelled optimization status = %v, want cancelled", cancelledTask["status"])
	}
	runsPort := service.optimizationRuns.(*stubOptimizationRuns)
	if got, want := strings.Join(runsPort.cancelled, ","), "run-opt-running,run-opt-completed,run-opt-missing"; got != want {
		t.Fatalf("cancelled run ids = %q, want %q", got, want)
	}

	metricsAny, err := service.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	metrics := asMap(t, metricsAny)
	runsMetrics := asMap(t, metrics["runs"])
	if runsMetrics["total"] != 3 {
		t.Fatalf("runs total = %v, want 3", runsMetrics["total"])
	}
	byProvider := asMap(t, runsMetrics["byProvider"])
	if byProvider[provider.ID] != 2 || byProvider["unbound"] != 1 {
		t.Fatalf("runs byProvider = %#v, want provider=%d unbound=%d", byProvider, 2, 1)
	}
	lifecycle := asMap(t, runsMetrics["lifecycle"])
	if lifecycle["failed"] != 1 || lifecycle["cancelled"] != 1 || lifecycle["resumed"] != 1 || lifecycle["orphaned"] != 1 {
		t.Fatalf("runs lifecycle = %#v", lifecycle)
	}

	toolsMetrics := asMap(t, metrics["tools"])
	if toolsMetrics["total"] != 4 || toolsMetrics["successful"] != 1 || toolsMetrics["averageDurationMs"] != int64(75) {
		t.Fatalf("tools metrics = %#v", toolsMetrics)
	}
	if toolsMetrics["outputBytesTotal"] == int64(0) || toolsMetrics["outputBytesMax"] == int64(0) || toolsMetrics["truncated"] != 1 || toolsMetrics["errorCount"] != 1 || toolsMetrics["retryableErrors"] != 1 {
		t.Fatalf("tool payload/error metrics = %#v", toolsMetrics)
	}
	byErrorCode := asMap(t, toolsMetrics["byErrorCode"])
	if byErrorCode["TIMEOUT"] != 1 {
		t.Fatalf("tool error codes = %#v, want one TIMEOUT without duplicate status count", byErrorCode)
	}
	if byErrorCode["RUNNING"] != nil || byErrorCode["PENDING_APPROVAL"] != nil {
		t.Fatalf("nonterminal tool calls counted as errors: %#v", byErrorCode)
	}

	approvalsMetrics := asMap(t, metrics["approvals"])
	if approvalsMetrics["pending"] != 1 || approvalsMetrics["approved"] != 1 || approvalsMetrics["denied"] != 1 || approvalsMetrics["recoverablePending"] != 1 {
		t.Fatalf("approvals metrics = %#v", approvalsMetrics)
	}
	resolutionWait := asMap(t, approvalsMetrics["resolutionWaitMs"])
	if resolutionWait["count"] != int64(2) {
		t.Fatalf("resolution wait metrics = %#v, want count=2", resolutionWait)
	}

	usageMetrics := asMap(t, metrics["usage"])
	if usageMetrics["samples"] != 1 || usageMetrics["tokensInTotal"] != 120 || usageMetrics["tokensOutAverage"] != 60 {
		t.Fatalf("usage metrics = %#v", usageMetrics)
	}
}

func newAssistantServiceHarness(t *testing.T, options ...Option) (*jfadkruntime.Runtime, *Service, adksession.Service) {
	t.Helper()
	root := t.TempDir()
	store, err := jfadkruntime.NewStore(root+"/adk.db", root+"/secrets.json", root+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sessionService := adksession.InMemoryService()
	runtime := jfadkruntime.NewRuntimeWithSessionService(store, jfadkruntime.NewToolRegistry(), sessionService)
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("runtime.Close(): %v", err)
		}
	})
	return runtime, NewService(runtime, options...), sessionService
}

func assistantServiceProvider(t *testing.T, runtime *jfadkruntime.Runtime) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/responses") {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if streaming, _ := payload["stream"].(bool); streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			_, err := w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-test\",\"model\":\"test-model\"}}\n\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-test\",\"model\":\"test-model\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n" +
				"data: [DONE]\n\n"))
			if err != nil {
				t.Fatalf("encode streaming provider response: %v", err)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id": "resp-test", "model": "test-model",
			"output": []map[string]any{{
				"type": "message", "role": "assistant",
				"content": []map[string]any{{"type": "output_text", "text": "ok", "annotations": []any{}}},
			}},
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		}); err != nil {
			t.Fatalf("encode provider response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	if _, err := runtime.Store().SaveProvider(context.Background(), assistantmodel.ProviderWriteRequest{
		ID: "test-provider", DisplayName: "Test Provider", BaseURL: server.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider test provider: %v", err)
	}
}

func appendAssistantSessionEvent(t *testing.T, sessionService adksession.Service, agentID string, sessionID string, event *adksession.Event) {
	t.Helper()
	response, err := sessionService.Get(context.Background(), &adksession.GetRequest{
		AppName:   "jftrade-" + agentID,
		UserID:    "jftrade-user",
		SessionID: sessionID,
	})
	if err != nil {
		created, createErr := sessionService.Create(context.Background(), &adksession.CreateRequest{
			AppName:   "jftrade-" + agentID,
			UserID:    "jftrade-user",
			SessionID: sessionID,
		})
		if createErr != nil {
			t.Fatalf("Create session service session: %v", createErr)
		}
		response = &adksession.GetResponse{Session: created.Session}
	}
	if err := sessionService.AppendEvent(context.Background(), response.Session, event); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
}

func newUserSessionEvent(runID string, text string, ts time.Time) *adksession.Event {
	event := adksession.NewEvent(context.Background(), runID)
	event.ID = "user-" + runID + "-" + ts.UTC().Format(time.RFC3339Nano)
	event.Author = "user"
	event.Timestamp = ts
	event.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText(text, genai.RoleUser),
	}
	return event
}

func newAssistantSessionEvent(runID string, eventID string, reply string, reasoning string, ts time.Time) *adksession.Event {
	parts := make([]*genai.Part, 0, 2)
	if reasoning != "" {
		parts = append(parts, &genai.Part{Text: reasoning, Thought: true})
	}
	if reply != "" {
		parts = append(parts, &genai.Part{Text: reply})
	}
	event := adksession.NewEvent(context.Background(), runID)
	event.ID = eventID
	event.Author = "assistant"
	event.Timestamp = ts
	event.LLMResponse = adkmodel.LLMResponse{
		Content:      genai.NewContentFromParts(parts, genai.RoleModel),
		TurnComplete: true,
	}
	return event
}

type stubOptimizationRuns struct {
	cancelled []string
	runs      map[string]OptimizationRun
}

func newStubOptimizationRuns(runs map[string]OptimizationRun) *stubOptimizationRuns {
	return &stubOptimizationRuns{runs: runs, cancelled: []string{}}
}

func (s *stubOptimizationRuns) Get(runID string) (OptimizationRun, bool) {
	run, ok := s.runs[runID]
	return run, ok
}

func (s *stubOptimizationRuns) Cancel(runID string) {
	s.cancelled = append(s.cancelled, runID)
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]int:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			result[key] = entry
		}
		return result
	case map[string]int64:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			result[key] = entry
		}
		return result
	default:
		t.Fatalf("value %T is not a supported map type", value)
		return nil
	}
}
