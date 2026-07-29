package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	"google.golang.org/genai"
)

func TestGoogleADKExecutionDescriptorRunMappingAndContentBoundaries(t *testing.T) {
	execution := &googleADKExecution{
		runID:            "root-run",
		runIDByAgentName: map[string]string{"child-agent": "child-run"},
		descriptors: map[string]ToolDescriptor{
			"market.candles": {Name: "market.candles", Permission: "read"},
		},
	}

	if contentHasText(nil) {
		t.Fatal("nil content unexpectedly has text")
	}
	if contentHasText(genai.NewContentFromParts([]*genai.Part{{Text: ""}, nil}, genai.RoleModel)) {
		t.Fatal("empty content unexpectedly has text")
	}
	if !contentHasText(genai.NewContentFromText("ready", genai.RoleModel)) {
		t.Fatal("text content was not detected")
	}

	if descriptor, ok := execution.descriptorForTool(boundaryGoogleTool{name: "market.candles"}); !ok || descriptor.Permission != "read" {
		t.Fatalf("descriptor fallback = %#v/%v", descriptor, ok)
	}
	if descriptor, ok := execution.descriptorForTool(boundaryGoogleTool{name: "missing"}); ok || descriptor.Name != "" {
		t.Fatalf("missing descriptor = %#v/%v", descriptor, ok)
	}
	if descriptor, ok := (&googleADKExecution{}).descriptorForTool(nil); ok || descriptor.Name != "" {
		t.Fatalf("nil descriptor = %#v/%v", descriptor, ok)
	}

	if got := execution.runIDForAgentName(" child-agent "); got != "child-run" {
		t.Fatalf("runIDForAgentName(child-agent) = %q", got)
	}
	if got := execution.runIDForAgentName("unknown"); got != "root-run" {
		t.Fatalf("runIDForAgentName(unknown) = %q", got)
	}
	if got := execution.agentNameForRunID(" child-run "); got != "child-agent" {
		t.Fatalf("agentNameForRunID(child-run) = %q", got)
	}
	if got := execution.agentNameForRunID(" "); got != "" {
		t.Fatalf("agentNameForRunID(empty) = %q", got)
	}
}

func TestGoogleADKExecutionToolCallReuseAndCompletionBoundaries(t *testing.T) {
	execution := &googleADKExecution{
		runID:            "root-run",
		runIDByAgentName: map[string]string{"child-agent": "child-run"},
	}
	execution.reply.WriteString("pre tool reply")
	execution.reasoning.WriteString("pre tool reasoning")

	call := execution.ensureCallForAgent("function-1", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"}, "child-agent")
	if call.RunID != "child-run" || call.ToolName != "market.candles" || call.Status != "RUNNING" {
		t.Fatalf("created call = %#v", call)
	}
	reused := execution.ensureCallForAgent("function-1", ToolDescriptor{Name: "ignored"}, map[string]any{"symbol": "BABA"}, "child-agent")
	if reused.ID != call.ID || len(execution.calls) != 1 {
		t.Fatalf("reused call = %#v calls=%#v", reused, execution.calls)
	}
	if execution.preToolContent.String() != "pre tool reply" || execution.preToolReasoning.String() != "pre tool reasoning" {
		t.Fatalf("pre-tool buffers = %q/%q", execution.preToolContent.String(), execution.preToolReasoning.String())
	}

	execution.finishCall("missing-call", nil, errors.New("ignored"))
	if execution.calls[0].Status != "RUNNING" {
		t.Fatalf("missing finish mutated calls: %#v", execution.calls)
	}
	execution.finishCall(call.ID, map[string]any{"close": 10.2}, nil)
	if execution.calls[0].Status != "SUCCEEDED" || execution.calls[0].Output == nil || len(execution.summaries) == 0 {
		t.Fatalf("successful finish = calls %#v summaries %#v", execution.calls, execution.summaries)
	}

	failed := execution.ensureCallForRun("function-2", ToolDescriptor{Name: "strategy.backtest"}, nil, "")
	execution.finishCall(failed.ID, nil, context.DeadlineExceeded)
	if execution.calls[1].Status != "TIMED_OUT" || execution.calls[1].Error == nil {
		t.Fatalf("timeout finish = %#v", execution.calls[1])
	}
}

func TestGoogleADKExecutionRunAndEventErrorBoundaries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	execution := &googleADKExecution{
		runID: "run-cancel",
		runBlocking: func(ctx context.Context, _ *genai.Content) error {
			<-ctx.Done()
			return nil
		},
	}
	if err := execution.run(ctx, genai.NewContentFromText("cancel", genai.RoleUser)); !errors.Is(err, context.Canceled) {
		t.Fatalf("run canceled err = %v, want context.Canceled", err)
	}

	execution = &googleADKExecution{runID: "run-event", sawPartialText: true}
	if err := execution.consumeEvent(nil); err != nil {
		t.Fatalf("consume nil event: %v", err)
	}
	emptyFinal := adksession.NewEvent(context.Background(), "inv-empty")
	if err := execution.consumeEvent(emptyFinal); err != nil {
		t.Fatalf("consume empty final event: %v", err)
	}
	if execution.sawPartialText {
		t.Fatal("empty final event should reset sawPartialText")
	}

	var deltaCalls int
	execution = &googleADKExecution{
		runID: "run-delta",
		onDelta: func(ChatDelta) error {
			deltaCalls++
			return errors.New("delta failed")
		},
	}
	textEvent := adksession.NewEvent(context.Background(), "inv-text")
	textEvent.Author = "agent"
	textEvent.Content = genai.NewContentFromText("visible reply", genai.RoleModel)
	if err := execution.consumeEvent(textEvent); err == nil || !strings.Contains(err.Error(), "delta failed") {
		t.Fatalf("consume text event err = %v, want delta failed", err)
	}
	if deltaCalls != 1 {
		t.Fatalf("delta calls = %d, want 1", deltaCalls)
	}

	execution = &googleADKExecution{runID: "run-partial", sawPartialText: true}
	finalText := adksession.NewEvent(context.Background(), "inv-final-text")
	finalText.Content = genai.NewContentFromText("duplicate final", genai.RoleModel)
	if err := execution.consumeEvent(finalText); err != nil {
		t.Fatalf("consume final after partial: %v", err)
	}
	if got := execution.result().Reply; got != "" {
		t.Fatalf("final text after partial reply = %q, want suppressed", got)
	}
}

func TestGoogleADKExecutionPauseAndBufferedErrorBoundaries(t *testing.T) {
	pausedAt := nowString()
	for _, tc := range []struct {
		name string
		exec *googleADKExecution
		want bool
	}{
		{name: "blank run", exec: &googleADKExecution{runID: "run"}, want: false},
		{name: "different run", exec: &googleADKExecution{runID: "run", loadRun: func(context.Context, string) (Run, bool, error) {
			return Run{}, false, nil
		}}, want: false},
		{name: "load error", exec: &googleADKExecution{runID: "run", loadRun: func(context.Context, string) (Run, bool, error) {
			return Run{}, false, errors.New("load failed")
		}}, want: false},
		{name: "not found", exec: &googleADKExecution{runID: "run", loadRun: func(context.Context, string) (Run, bool, error) {
			return Run{}, false, nil
		}}, want: false},
		{name: "pause requested", exec: &googleADKExecution{runID: "run", loadRun: func(context.Context, string) (Run, bool, error) {
			return Run{ID: "run", WorkMode: WorkModeLoop, PauseRequestedAt: &pausedAt}, true, nil
		}}, want: true},
		{name: "user paused", exec: &googleADKExecution{runID: "run", loadRun: func(context.Context, string) (Run, bool, error) {
			return Run{ID: "run", WorkMode: WorkModeLoop, Status: RunStatusPaused, PausedReason: "user"}, true, nil
		}}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runID := "run"
			switch tc.name {
			case "blank run":
				runID = " "
			case "different run":
				runID = "other"
			}
			if got := tc.exec.shouldInterruptForUserGoalPause(runID); got != tc.want {
				t.Fatalf("shouldInterruptForUserGoalPause = %v, want %v", got, tc.want)
			}
		})
	}

	execution := &googleADKExecution{
		runID: "root-run",
		onDelta: func(ChatDelta) error {
			return errors.New("flush failed")
		},
	}
	execution.ensureTextMaps()
	execution.builderForRun(execution.bufferedReplyByRunID, "root-run").WriteString(" root buffered ")
	if err := execution.flushBufferedTextIfReady(); err == nil || !strings.Contains(err.Error(), "flush failed") {
		t.Fatalf("flushBufferedTextIfReady err = %v, want flush failed", err)
	}

	execution = &googleADKExecution{
		runID: "root-run",
		onDelta: func(ChatDelta) error {
			return errors.New("direct flush failed")
		},
	}
	execution.ensureTextMaps()
	execution.builderForRun(execution.bufferedReplyByRunID, "root-run").WriteString(" direct buffered ")
	if err := execution.flushBufferedTextForRunIfReady("root-run"); err == nil || !strings.Contains(err.Error(), "direct flush failed") {
		t.Fatalf("flushBufferedTextForRunIfReady err = %v, want direct flush failed", err)
	}
}

func TestGoogleADKFinalReplySynthesisBoundaries(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	session, err := runtime.Store().CreateSession(ctx, "agent-final", "final synthesis")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	definition := Agent{
		ID: "agent-final", Name: "Final Agent", ProviderID: "missing-provider",
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	}
	execution := &googleADKExecution{
		runID:                   "run-final",
		runIDByAgentName:        map[string]string{googleADKAgentName(definition.ID): "run-final"},
		calls:                   []ToolCall{},
		postToolTextByRunID:     map[string]bool{},
		toolResponseSeenByRunID: map[string]bool{},
	}

	if err := runtime.ensureGoogleADKFinalReply(ctx, definition, session, execution, "run-final", "summarize the result"); err != nil {
		t.Fatalf("ensureGoogleADKFinalReply without tool calls = %v", err)
	}

	execution.calls = []ToolCall{{ID: "call-1", RunID: "run-final", ToolName: "market.candles", Status: "SUCCEEDED"}}
	execution.markToolResponseSeenForRun("run-final")
	execution.markPostToolTextForRun("run-final")
	if err := runtime.ensureGoogleADKFinalReply(ctx, definition, session, execution, "run-final", "summarize the result"); err != nil {
		t.Fatalf("ensureGoogleADKFinalReply with post-tool text = %v", err)
	}

	execution.postToolTextByRunID["run-final"] = false
	err = runtime.ensureGoogleADKFinalReply(ctx, definition, session, execution, "run-final", "summarize the result")
	if err == nil || !strings.Contains(err.Error(), "agent provider is unavailable") {
		t.Fatalf("ensureGoogleADKFinalReply missing provider err = %v", err)
	}
}

func TestGoogleADKExecutionApprovalResolutionAndPendingBoundaries(t *testing.T) {
	parts := approvalResolutionParts([]Approval{
		{ConfirmationCallID: "", Status: ApprovalStatusApproved},
		{ConfirmationCallID: "confirmation-ok", Status: ApprovalStatusDenied},
	})
	if len(parts) != 1 || parts[0].FunctionResponse == nil || parts[0].FunctionResponse.ID != "confirmation-ok" {
		t.Fatalf("approvalResolutionParts = %#v, want only the valid confirmation", parts)
	}
	if confirmed, _ := parts[0].FunctionResponse.Response["confirmed"].(bool); confirmed {
		t.Fatalf("denied approval response confirmed = %v, want false", confirmed)
	}

	ctx := context.Background()
	getErr := errors.New("session get failed")
	_, err := (&googleADKExecution{
		sessionService: getErrorADKSessionService{err: getErr},
		appName:        "app",
		sessionID:      "missing-session",
	}).pendingApprovals(ctx, newTestRuntime(t).Store())
	if !errors.Is(err, getErr) {
		t.Fatalf("pendingApprovals get err = %v, want %v", err, getErr)
	}

	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: googleADKUserID, SessionID: "approval-errors",
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	appendConfirmationEvent := func(invocationID string, call *genai.FunctionCall) {
		t.Helper()
		event := adksession.NewEvent(ctx, invocationID)
		event.Author = "agent"
		event.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: call}}, genai.RoleModel)
		if err := service.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("AppendEvent(%s): %v", invocationID, err)
		}
	}
	appendConfirmationEvent("bad-original", &genai.FunctionCall{
		ID: "confirmation-bad-original", Name: toolconfirmation.FunctionCallName,
		Args: map[string]any{"originalFunctionCall": "not a function call"},
	})
	_, err = (&googleADKExecution{
		sessionService: service,
		appName:        "app",
		sessionID:      "approval-errors",
	}).pendingApprovals(ctx, newTestRuntime(t).Store())
	if err == nil || !strings.Contains(err.Error(), `argument "originalFunctionCall" has invalid type`) {
		t.Fatalf("pendingApprovals original-call err = %v, want invalid originalFunctionCall type", err)
	}

	validService := adksession.InMemoryService()
	validSession, err := validService.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: googleADKUserID, SessionID: "approval-save-error",
	})
	if err != nil {
		t.Fatalf("Create valid session: %v", err)
	}
	original := &genai.FunctionCall{ID: "tool-call-save-error", Name: "strategy.research_backtest", Args: map[string]any{"symbol": "TME"}}
	event := adksession.NewEvent(ctx, "save-error")
	event.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
		ID: "confirmation-save-error", Name: toolconfirmation.FunctionCallName,
		Args: map[string]any{
			"originalFunctionCall": original,
			"toolConfirmation":     toolconfirmation.ToolConfirmation{Hint: "approve"},
		},
	}}}, genai.RoleModel)
	if err := validService.AppendEvent(ctx, validSession.Session, event); err != nil {
		t.Fatalf("AppendEvent(save-error): %v", err)
	}
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableApprovals); err != nil {
		t.Fatalf("drop approvals table: %v", err)
	}
	execution := &googleADKExecution{
		sessionService: validService,
		appName:        "app",
		sessionID:      "approval-save-error",
		agent:          Agent{ID: "agent-save-error"},
		runID:          "run-save-error",
	}
	execution.ensureCall(original.ID, ToolDescriptor{Name: original.Name}, original.Args)
	_, err = execution.pendingApprovals(ctx, runtime.Store())
	if err == nil || !strings.Contains(err.Error(), tableApprovals) {
		t.Fatalf("pendingApprovals save err = %v, want approvals table error", err)
	}
}

func TestGoogleADKExecutionRehydrateBoundaries(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-rehydrate-boundary", Name: "Rehydrate Boundary", Status: AgentStatusEnabled,
	})

	_, err := runtime.rehydrateGoogleADKExecution(ctx, Run{
		ID: "run-missing-session", AgentID: agent.ID, ProviderID: testProviderID, Model: "test-model", SessionID: "missing-session",
	})
	if err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("rehydrate missing session err = %v, want session not found", err)
	}

	session := mustCreateSession(t, runtime, agent.ID, "rehydrate empty approvals")
	execution, err := runtime.rehydrateGoogleADKExecution(ctx, Run{
		ID: "run-empty-approvals", AgentID: agent.ID, ProviderID: testProviderID, Model: "test-model", SessionID: session.ID,
		PendingApprovals: []Approval{
			{ConfirmationCallID: "", FunctionCallID: "tool-call"},
			{ConfirmationCallID: "confirmation", FunctionCallID: ""},
		},
	})
	if err != nil {
		t.Fatalf("rehydrate empty approvals err = %v", err)
	}
	if execution != nil {
		t.Fatalf("rehydrate empty approvals execution = %#v, want nil", execution)
	}
}

type boundaryGoogleTool struct {
	name string
}

func (tool boundaryGoogleTool) Name() string        { return tool.name }
func (tool boundaryGoogleTool) Description() string { return "boundary test tool" }
func (tool boundaryGoogleTool) IsLongRunning() bool { return false }

type getErrorADKSessionService struct {
	adksession.Service
	err error
}

func (service getErrorADKSessionService) Get(context.Context, *adksession.GetRequest) (*adksession.GetResponse, error) {
	return nil, service.err
}
