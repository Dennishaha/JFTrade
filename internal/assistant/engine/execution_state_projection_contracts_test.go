package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adktool "google.golang.org/adk/v2/tool"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func newBareGoogleADKExecution(runID string) *googleADKExecution {
	return &googleADKExecution{
		sessionID: "session-" + runID,
		appName:   googleADKAppName("coverage-agent"),
		agent:     Agent{ID: "coverage-agent"},
		runID:     runID,
		runIDByAgentName: map[string]string{
			"coverage-agent": runID,
		},
		runSnapshotBaseByID: map[string]Run{
			runID: {ID: runID, SessionID: "session-" + runID, AgentID: "coverage-agent", Status: RunStatusRunning, Usage: &RunUsage{}},
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
	}
}

func nowUTCForCoverage() time.Time {
	return time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
}

func TestGoogleADKExecutionProjectsToolResponseLifecycle(t *testing.T) {
	execution := newBareGoogleADKExecution("root-run")
	execution.runIDByAgentName["child-agent"] = "child-run"
	execution.runSnapshotBaseByID["child-run"] = Run{ID: "child-run", SessionID: execution.sessionID, AgentID: execution.agent.ID, Status: RunStatusRunning, Usage: &RunUsage{}}
	execution.reply.WriteString("before tools")
	execution.reasoning.WriteString("initial reasoning")

	if err := execution.consumeEvent(newProjectionEvent("call-event", "root-run", "child-agent", genai.RoleModel, []*genai.Part{{
		FunctionCall: &genai.FunctionCall{ID: "price", Name: "market.price", Args: map[string]any{"symbol": "AAPL"}},
	}}, nowUTCForCoverage(), false)); err != nil {
		t.Fatalf("consume tool call event: %v", err)
	}
	if len(execution.calls) != 1 || execution.calls[0].RunID != "child-run" || execution.calls[0].Status != "RUNNING" {
		t.Fatalf("tool call projection = %+v", execution.calls)
	}
	if reply, reasoning := execution.preToolState(); reply != "before tools" || reasoning != "initial reasoning" {
		t.Fatalf("pre-tool state = %q/%q", reply, reasoning)
	}
	if err := execution.appendVisibleTextForRun("child-run", "final answer", "final reasoning"); err != nil {
		t.Fatalf("buffer child text: %v", err)
	}
	execution.consumeFunctionResponse(&genai.FunctionResponse{ID: "price", Name: "market.price", Response: map[string]any{"last": 221.5}})
	if call := execution.calls[0]; call.Status != "SUCCEEDED" || call.Output == nil || call.CompletedAt == nil || call.Error != nil {
		t.Fatalf("successful response call = %+v", call)
	}
	if result := execution.resultForRun("child-run"); result.Reply != "final answer" || result.ReasoningContent != "final reasoning" || !execution.runHasPostToolText("child-run") {
		t.Fatalf("post-tool child result = %+v, hasPost=%v", result, execution.runHasPostToolText("child-run"))
	}

	timedOut := execution.ensureCallForRun("timeout", ToolDescriptor{Name: "market.history"}, nil, "root-run")
	execution.consumeFunctionResponse(&genai.FunctionResponse{ID: "timeout", Name: "market.history", Response: map[string]any{"error": "context deadline exceeded"}})
	if timedOut.Status != "TIMED_OUT" || timedOut.Error == nil || !strings.Contains(*timedOut.Error, "timed out") || timedOut.CompletedAt == nil {
		t.Fatalf("timeout response = %+v", timedOut)
	}

	cancelled := execution.ensureCallForRun("cancel", ToolDescriptor{Name: "market.stream"}, nil, "root-run")
	execution.consumeFunctionResponse(&genai.FunctionResponse{ID: "cancel", Name: "market.stream", Response: map[string]any{"error": "context canceled by caller"}})
	if cancelled.Status != "CANCELLED" || cancelled.Error == nil || !strings.Contains(*cancelled.Error, "cancelled") {
		t.Fatalf("cancelled response = %+v", cancelled)
	}

	pending := execution.ensureCallForRun("approval", ToolDescriptor{Name: "trade.submit"}, nil, "root-run")
	execution.consumeFunctionResponse(&genai.FunctionResponse{ID: "approval", Name: "trade.submit", Response: map[string]any{"error": adktool.ErrConfirmationRequired.Error()}})
	if pending.Status != "PENDING_APPROVAL" || !pending.RequiresUser || pending.CompletedAt != nil {
		t.Fatalf("confirmation-required response = %+v", pending)
	}

	if err := execution.consumeEvent(newProjectionEvent("input-event", "root-run", "child-agent", genai.RoleModel, []*genai.Part{{
		FunctionCall: &genai.FunctionCall{ID: "input", Name: adkworkflow.WorkflowInputFunctionCallName},
	}}, nowUTCForCoverage(), false)); !errors.Is(err, errADKInputUnsupported) {
		t.Fatalf("workflow input event err = %v, want unsupported input", err)
	}
	if err := execution.consumeEvent(nil); err != nil {
		t.Fatalf("nil event should be harmless: %v", err)
	}
}

func TestGoogleADKExecutionStateModelsVisibleAndPersistedRunStatus(t *testing.T) {
	state := newBareGoogleADKExecution("state-run")

	assertStatus := func(name string, calls []ToolCall, want string) {
		t.Helper()
		state.calls = calls
		state.toolResponseSeenByRunID = map[string]bool{}
		state.postToolTextByRunID = map[string]bool{}
		state.toolResponseSeqByRunID = map[string]int{}
		state.postToolTextSeqByRunID = map[string]int{}
		if got := state.derivedRunStatusForRunLocked("state-run"); got != want {
			t.Fatalf("%s derived status = %q, want %q", name, got, want)
		}
	}

	assertStatus("empty", nil, RunStatusRunning)
	state.reply.WriteString("plain model response")
	if got := state.derivedRunStatusForRunLocked("state-run"); got != RunStatusCompleted {
		t.Fatalf("text-only derived status = %q, want completed", got)
	}
	state.reply.Reset()
	assertStatus("pending approval", []ToolCall{{RunID: "state-run", Status: "PENDING_APPROVAL"}}, RunStatusPending)
	assertStatus("active", []ToolCall{{RunID: "state-run", Status: "RUNNING"}}, RunStatusRunning)
	assertStatus("cancelled", []ToolCall{{RunID: "state-run", Status: "CANCELLED"}}, RunStatusCancelled)
	assertStatus("failed", []ToolCall{{RunID: "state-run", Status: "FAILED"}}, RunStatusRunning)
	assertStatus("unknown", []ToolCall{{RunID: "state-run", Status: "OTHER"}}, RunStatusRunning)

	state.calls = []ToolCall{{ID: "finished", RunID: "state-run", ToolName: "market.price", Status: "SUCCEEDED", Output: map[string]any{"last": 1}}}
	if got := state.derivedRunStatusForRunLocked("state-run"); got != RunStatusRunning {
		t.Fatalf("finished tool without a final reply = %q, want running", got)
	}
	state.markToolResponseSeenForRun("state-run")
	state.markPostToolTextForRun("state-run")
	if got := state.derivedRunStatusForRunLocked("state-run"); got != RunStatusCompleted {
		t.Fatalf("finished tool with final reply = %q, want completed", got)
	}
	if got := state.persistedRunStatusForRunLocked("state-run"); got != RunStatusRunning {
		t.Fatalf("persisted completed tool status = %q, want running snapshot", got)
	}
	state.calls[0].Status = "PENDING_APPROVAL"
	if got := state.persistedRunStatusForRunLocked("state-run"); got != RunStatusPending {
		t.Fatalf("persisted approval status = %q, want pending", got)
	}

	childReasoning := state.builderForRun(state.reasoningByRunID, "child-state")
	childReasoning.WriteString("child reasoning")
	if !state.runHasTextLocked("child-state") || state.runHasTextLocked("missing-child") {
		t.Fatal("run-scoped visible text detection did not distinguish child result buffers")
	}
	state.runSnapshotBaseByID["child-state"] = Run{ID: "child-state", SessionID: state.sessionID, AgentID: state.agent.ID}
	state.calls = append(state.calls, ToolCall{ID: "child-call", RunID: "child-state", ToolName: "child.tool", Status: "SUCCEEDED"})
	ids := state.snapshotRunIDsLocked()
	if len(ids) != 2 || !strings.Contains(strings.Join(ids, ","), "state-run") || !strings.Contains(strings.Join(ids, ","), "child-state") {
		t.Fatalf("snapshot run ids = %#v", ids)
	}
	snapshot := state.runSnapshotLocked("", true)
	if snapshot.ID != "state-run" || snapshot.Status != RunStatusPending || len(snapshot.ToolCalls) != 1 {
		t.Fatalf("persisted root snapshot = %+v", snapshot)
	}

	if googleADKAgentName("") != "jftrade_agent" || googleADKAgentName("user") != "jftrade_user_agent" || googleADKWorkflowRootName("") != "workflow_root" {
		t.Fatal("ADK agent/workflow names must remain valid for blank and user identities")
	}
	instruction := workflowChildInstructionTask(workflowStep{Objective: "research", Message: "inspect price", Description: "use verified data", AgentRole: "analyst"})
	if !strings.Contains(instruction, "总体目标：research") || !strings.Contains(instruction, "当前子任务：inspect price") || !strings.Contains(instruction, "子 Agent 角色：analyst") {
		t.Fatalf("child instruction = %q", instruction)
	}
	if got := workflowChildInstruction("base", ""); got != "base" {
		t.Fatalf("child instruction without task = %q", got)
	}
}

func TestGoogleADKRunnerHelpersPreserveRecoveryRules(t *testing.T) {
	if status, message := classifyToolExecutionError(nil); status != "SUCCEEDED" || message != "" {
		t.Fatalf("nil tool result = %q/%q", status, message)
	}
	if status, message := classifyToolErrorText(" context deadline exceeded "); status != "TIMED_OUT" || !strings.Contains(message, "timed out") {
		t.Fatalf("deadline classification = %q/%q", status, message)
	}
	if status, message := classifyToolErrorText("context canceled"); status != "CANCELLED" || !strings.Contains(message, "cancelled") {
		t.Fatalf("cancellation classification = %q/%q", status, message)
	}
	if got := prefixedToolError("tool execution timed out: upstream", "tool execution timed out"); got != "tool execution timed out: upstream" {
		t.Fatalf("already prefixed tool error = %q", got)
	}
	errText := "broker rejected order"
	if got := firstToolCallFailure(&Run{ToolCalls: []ToolCall{{Status: "SUCCEEDED"}, {Status: "FAILED", Error: &errText}}}); got != errText {
		t.Fatalf("first tool failure = %q", got)
	}
	if got := toolCallFailureMessage(&ToolCall{Status: "TIMED_OUT"}); got != "tool execution timed out" {
		t.Fatalf("timeout fallback failure message = %q", got)
	}

	execution := newBareGoogleADKExecution("resume-seed")
	execution.calls = []ToolCall{{ID: "existing", IdempotencyKey: "existing-key", Status: "SUCCEEDED"}}
	seedResumedExecutionState(execution, Run{
		ID: "resume-seed", ToolCalls: []ToolCall{
			{ID: "existing", Status: "SUCCEEDED"},
			{IdempotencyKey: "existing-key", Status: "SUCCEEDED"},
			{ID: "new-call", IdempotencyKey: "new-key", Status: "FAILED"},
		}, ToolSummaries: []string{"saved summary"},
	})
	if len(execution.calls) != 2 || execution.calls[1].ID != "new-call" || len(execution.summaries) != 1 {
		t.Fatalf("seeded resumed execution = calls:%+v summaries:%+v", execution.calls, execution.summaries)
	}

	bareRuntime := &Runtime{}
	loader := bareRuntime.googleADKExecutionRunLoader()
	if run, ok, err := loader(context.Background(), "missing"); err != nil || ok || run.ID != "" {
		t.Fatalf("nil-store run loader = %+v/%v/%v", run, ok, err)
	}
	if nodes, err := bareRuntime.newGoogleADKWorkflowChildNodes(context.Background(), Agent{}, "parent", []Run{{ID: "unplanned-child"}}, nil, newBareGoogleADKExecution("parent")); err != nil || len(nodes) != 0 {
		t.Fatalf("unplanned child nodes = %#v, err=%v", nodes, err)
	}
	cancelExecution := newBareGoogleADKExecution("cancel-run")
	cancelExecution.runBlocking = func(ctx context.Context, _ *genai.Content) error {
		<-ctx.Done()
		return ctx.Err()
	}
	if err := cancelExecution.run(cancelledCoverageContext(t), genai.NewContentFromText("wait", genai.RoleUser)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled execution run err = %v", err)
	}
}

func cancelledCoverageContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestDirectApprovalCompletionPersistsAssistantTimelineAndTerminalState(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{ID: "direct-resume-agent", Name: "Direct Resume Agent", Status: AgentStatusEnabled})
	session := mustCreateSession(t, runtime, agent.ID, "direct resume")

	saveRun := func(id string, approvalStatus string) Run {
		return mustSaveRun(t, runtime, Run{
			ID: id, SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPending, UserMessage: "resume the operation",
			PendingApprovals: []Approval{{
				ID: id + "-approval", RunID: id, AgentID: agent.ID, ToolName: "trade.submit", Status: approvalStatus,
				FunctionCallID: id + "-call", ConfirmationCallID: id + "-confirmation", CreatedAt: nowString(), UpdatedAt: nowString(),
			}}, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
	}

	approved := saveRun("direct-approved", ApprovalStatusApproved)
	approvedExecution := newBareGoogleADKExecution(approved.ID)
	approvedExecution.sessionID = session.ID
	approvedExecution.agent = agent
	approvedExecution.reply.WriteString("order submitted after approval")
	approvedExecution.calls = []ToolCall{{ID: "approved-tool", RunID: approved.ID, ToolName: "trade.submit", Status: "SUCCEEDED"}}
	runtime.adkRuns[approved.ID] = approvedExecution
	completed, message, handled, err := runtime.completeDirectResumedExecution(ctx, approved, approvedExecution)
	if err != nil || !handled || message == nil || completed.Status != RunStatusCompleted {
		t.Fatalf("direct approved completion = %+v message=%+v handled=%v err=%v", completed, message, handled, err)
	}
	if _, active := runtime.adkRuns[approved.ID]; active {
		t.Fatal("completed direct approval execution remained active")
	}
	stored, ok, err := runtime.Store().Run(ctx, approved.ID)
	if err != nil || !ok || stored.FinalMessageID == "" || stored.FinalMessageID != message.ID {
		t.Fatalf("stored direct approved run = %+v ok=%v err=%v", stored, ok, err)
	}

	denied := saveRun("direct-denied", ApprovalStatusDenied)
	deniedExecution := newBareGoogleADKExecution(denied.ID)
	deniedExecution.sessionID = session.ID
	deniedExecution.agent = agent
	rejected := adktool.ErrConfirmationRejected.Error()
	deniedExecution.calls = []ToolCall{{ID: "denied-tool", RunID: denied.ID, ToolName: "trade.submit", Status: "FAILED", Error: &rejected}}
	completedDenied, deniedMessage, handled, err := runtime.completeDirectResumedExecution(ctx, denied, deniedExecution)
	if err != nil || !handled || deniedMessage == nil || completedDenied.Status != RunStatusDenied || len(completedDenied.ToolCalls) != 1 || completedDenied.ToolCalls[0].Status != "DENIED" {
		t.Fatalf("direct denied completion = %+v message=%+v handled=%v err=%v", completedDenied, deniedMessage, handled, err)
	}

	failed := saveRun("direct-failed", ApprovalStatusApproved)
	failedExecution := newBareGoogleADKExecution(failed.ID)
	failedExecution.sessionID = session.ID
	failedExecution.agent = agent
	runtime.adkRuns[failed.ID] = failedExecution
	failedResult, failedMessage, handled, err := runtime.failResumedExecution(ctx, failed, failedExecution, errors.New("resume transport failed"))
	if err != nil || !handled || failedMessage != nil || failedResult.Status != RunStatusFailed || failedResult.FailureReason == "" {
		t.Fatalf("failed resumed execution = %+v message=%+v handled=%v err=%v", failedResult, failedMessage, handled, err)
	}

	noParts := saveRun("direct-no-parts", ApprovalStatusApproved)
	noParts.PendingApprovals = nil
	if _, message, handled, err := runtime.resumeGoogleADKDirect(ctx, noParts); err != nil || handled || message != nil {
		t.Fatalf("direct resume without confirmation parts = message:%+v handled:%v err:%v", message, handled, err)
	}
}
