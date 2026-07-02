package adk

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

type flakyAppendSessionService struct {
	delegate             adksession.Service
	failAppendRemaining  atomic.Int32
	failedAppendAttempts atomic.Int32
	appendCalls          atomic.Int32
}

func (s *flakyAppendSessionService) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	return s.delegate.Create(ctx, req)
}

func (s *flakyAppendSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	return s.delegate.Get(ctx, req)
}

func (s *flakyAppendSessionService) List(ctx context.Context, req *adksession.ListRequest) (*adksession.ListResponse, error) {
	return s.delegate.List(ctx, req)
}

func (s *flakyAppendSessionService) Delete(ctx context.Context, req *adksession.DeleteRequest) error {
	return s.delegate.Delete(ctx, req)
}

func (s *flakyAppendSessionService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	s.appendCalls.Add(1)
	for {
		remaining := s.failAppendRemaining.Load()
		if remaining <= 0 {
			return s.delegate.AppendEvent(ctx, session, event)
		}
		if s.failAppendRemaining.CompareAndSwap(remaining, remaining-1) {
			s.failedAppendAttempts.Add(1)
			return errors.New("append event to sessionservice failed: database is locked")
		}
	}
}

func TestPendingApprovalsOnlyClaimsConfirmationCallsOwnedByExecution(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: googleADKUserID, SessionID: "session-approval-owner"})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	appendConfirmation := func(invocationID, confirmationID, functionCallID string) {
		t.Helper()
		event := adksession.NewEvent(invocationID)
		event.Author = "agent"
		event.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: confirmationID, Name: toolconfirmation.FunctionCallName,
			Args: map[string]any{
				"originalFunctionCall": &genai.FunctionCall{ID: functionCallID, Name: "strategy.research_backtest", Args: map[string]any{"symbol": "TME"}},
				"toolConfirmation":     toolconfirmation.ToolConfirmation{Hint: "approve"},
			},
		}}}, genai.RoleModel)
		if err := service.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("Append confirmation: %v", err)
		}
	}
	appendConfirmation("inv-foreign", "confirmation-foreign", "call-foreign")
	appendConfirmation("inv-owned", "confirmation-owned", "call-owned")

	execution := &googleADKExecution{
		sessionID: "session-approval-owner", appName: "app", sessionService: service,
		agent: Agent{ID: "agent-1"}, runID: "run-owned",
	}
	execution.ensureCall("call-owned", ToolDescriptor{Name: "strategy.research_backtest"}, map[string]any{"symbol": "TME"})
	approvals, err := execution.pendingApprovals(ctx, runtime.Store())
	if err != nil {
		t.Fatalf("pendingApprovals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].ConfirmationCallID != "confirmation-owned" || approvals[0].RunID != "run-owned" {
		t.Fatalf("approvals = %+v, want only owned confirmation", approvals)
	}
	again, err := execution.pendingApprovals(ctx, runtime.Store())
	if err != nil {
		t.Fatalf("pendingApprovals second pass: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second pass approvals = %+v, want none", again)
	}

	recovery := &googleADKExecution{
		sessionID: "session-approval-owner", appName: "app", sessionService: service,
		agent: Agent{ID: "agent-1"}, runID: "run-recovery",
	}
	recovery.ensureCall("call-owned", ToolDescriptor{Name: "strategy.research_backtest"}, map[string]any{"symbol": "TME"})
	recovered, err := recovery.pendingApprovals(ctx, runtime.Store())
	if err != nil {
		t.Fatalf("pendingApprovals recovery pass: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovery approvals = %+v, want globally idempotent confirmation", recovered)
	}
	all, err := runtime.Store().ListApprovals(ctx)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("stored approvals = %d, want 1", len(all))
	}
}

func TestGoogleRunnerHelperStateBoundaries(t *testing.T) {
	status, message := classifyToolExecutionError(nil)
	if status != "SUCCEEDED" || message != "" {
		t.Fatalf("nil tool error = %q/%q", status, message)
	}
	status, message = classifyToolExecutionError(context.DeadlineExceeded)
	if status != "TIMED_OUT" || !strings.Contains(message, "tool execution timed out") {
		t.Fatalf("deadline tool error = %q/%q", status, message)
	}
	status, message = classifyToolExecutionError(context.Canceled)
	if status != "CANCELLED" || !strings.Contains(message, "tool execution cancelled") {
		t.Fatalf("cancelled tool error = %q/%q", status, message)
	}
	if got := prefixedToolError("tool execution timed out: provider exceeded budget", "tool execution timed out"); got != "tool execution timed out: provider exceeded budget" {
		t.Fatalf("prefixed timeout = %q", got)
	}
	if got := prefixedToolError("  ", "tool execution failed"); got != "tool execution failed" {
		t.Fatalf("empty prefixed error = %q", got)
	}

	explicitErr := "broker denied order"
	for _, tc := range []struct {
		call *ToolCall
		want string
	}{
		{call: nil, want: ""},
		{call: &ToolCall{Status: "FAILED", Error: &explicitErr}, want: explicitErr},
		{call: &ToolCall{Status: "TIMED_OUT"}, want: "tool execution timed out"},
		{call: &ToolCall{Status: "CANCELLED"}, want: "tool execution cancelled"},
		{call: &ToolCall{Status: "FAILED"}, want: "tool execution failed"},
	} {
		if got := toolCallFailureMessage(tc.call); got != tc.want {
			t.Fatalf("toolCallFailureMessage(%#v) = %q, want %q", tc.call, got, tc.want)
		}
	}
	if got := firstToolCallFailure(&Run{ToolCalls: []ToolCall{{Status: "SUCCEEDED"}, {Status: "TIMED_OUT"}}}); got != "tool execution timed out" {
		t.Fatalf("firstToolCallFailure = %q", got)
	}
	if got := firstToolCallFailure(nil); got != "" {
		t.Fatalf("nil firstToolCallFailure = %q", got)
	}

	execution := &googleADKExecution{runID: "root-run"}
	if execution.runHasTextLocked("") {
		t.Fatal("empty execution unexpectedly has text")
	}
	execution.reply.WriteString(" root reply ")
	if !execution.runHasTextLocked("") || !execution.runHasTextLocked("root-run") {
		t.Fatal("root run text was not detected")
	}
	execution.replyByRunID = map[string]*strings.Builder{"child-run": {}}
	execution.replyByRunID["child-run"].WriteString(" child reply ")
	if !execution.runHasTextLocked("child-run") {
		t.Fatal("child reply text was not detected")
	}
	execution.reasoningByRunID = map[string]*strings.Builder{"reasoning-run": {}}
	execution.reasoningByRunID["reasoning-run"].WriteString(" child reasoning ")
	if !execution.runHasTextLocked("reasoning-run") {
		t.Fatal("child reasoning text was not detected")
	}
	if execution.runHasTextLocked("missing-run") {
		t.Fatal("missing run unexpectedly has text")
	}

	statusExecution := &googleADKExecution{
		runID: "status-run",
		calls: []ToolCall{{RunID: "status-run", Status: "SUCCEEDED"}},
	}
	if got := statusExecution.derivedRunStatusForRunLocked("status-run"); got != RunStatusRunning {
		t.Fatalf("completed tool without post-tool text status = %q", got)
	}
	statusExecution.markToolResponseSeenLocked("status-run")
	statusExecution.markPostToolTextForRun("status-run")
	if got := statusExecution.derivedRunStatusForRunLocked("status-run"); got != RunStatusCompleted {
		t.Fatalf("completed tool with post-tool text status = %q", got)
	}
	statusExecution.calls = []ToolCall{{RunID: "status-run", Status: "CANCELLED"}}
	if got := statusExecution.derivedRunStatusForRunLocked("status-run"); got != RunStatusCancelled {
		t.Fatalf("cancelled tool status = %q", got)
	}
	statusExecution.calls = []ToolCall{{RunID: "status-run", Status: "PENDING_APPROVAL"}}
	if got := statusExecution.persistedRunStatusForRunLocked("status-run"); got != RunStatusPending {
		t.Fatalf("persisted pending status = %q", got)
	}
	statusExecution.calls = []ToolCall{{RunID: "status-run", Status: "SUCCEEDED"}}
	if got := statusExecution.persistedRunStatusForRunLocked("status-run"); got != RunStatusRunning {
		t.Fatalf("persisted nonterminal status = %q", got)
	}

	finalExecution := &googleADKExecution{runID: "final-run", calls: []ToolCall{{RunID: "final-run", Status: "RUNNING"}}}
	if finalExecution.runNeedsFinalSynthesis("final-run") {
		t.Fatal("running tool should not need final synthesis")
	}
	finalExecution.calls[0].Status = "SUCCEEDED"
	if !finalExecution.runNeedsFinalSynthesis("final-run") {
		t.Fatal("finished tool without post-tool text should need final synthesis")
	}
	finalExecution.markToolResponseSeenForRun("final-run")
	finalExecution.markPostToolTextForRun("final-run")
	if finalExecution.runNeedsFinalSynthesis("final-run") {
		t.Fatal("post-tool text should satisfy final synthesis")
	}

	if got := googleADKAgentName("user"); got != "jftrade_user_agent" {
		t.Fatalf("googleADKAgentName(user) = %q", got)
	}
	if got := googleADKAgentName("Research-Agent"); got != "research_agent" {
		t.Fatalf("googleADKAgentName(custom) = %q", got)
	}
	if got := googleADKAgentName(" "); got != "jftrade_agent" {
		t.Fatalf("googleADKAgentName(empty) = %q", got)
	}
}

func TestGoogleRunnerHelperRecoveryBoundaries(t *testing.T) {
	execution := &googleADKExecution{
		runID:            "root-run",
		runIDByAgentName: map[string]string{"child_agent": "child-run"},
	}
	if got := execution.agentNameForRunID("missing-run"); got != "" {
		t.Fatalf("agentNameForRunID missing = %q", got)
	}
	if !execution.hasApprovalForConfirmation("") {
		t.Fatal("empty confirmation id should be treated as already processed")
	}
	execution.markConfirmationProcessed("")
	if execution.processedConfirmationIDs != nil {
		t.Fatalf("empty confirmation id should not initialize processed map: %#v", execution.processedConfirmationIDs)
	}

	call := execution.ensureCallForRun("call-1", ToolDescriptor{Permission: "read"}, nil, "")
	call.ToolName = ""
	call.Permission = ""
	again := execution.ensureCallForRun("call-1", ToolDescriptor{Name: "market.snapshot", Permission: "read"}, nil, "child-run")
	if again.ToolName != "market.snapshot" || again.Permission != "read" || again.RunID != "root-run" {
		t.Fatalf("existing call after descriptor hydration = %+v", again)
	}
	execution.consumeFunctionResponse(nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID: "call-1",
		Response: map[string]any{
			"success": false,
			"error":   "broker rejected order",
		},
	})
	if execution.calls[0].Status != "FAILED" || execution.calls[0].Error == nil || !strings.Contains(*execution.calls[0].Error, "broker rejected order") {
		t.Fatalf("failed function response call = %+v", execution.calls[0])
	}

	if err := execution.appendVisibleTextForRun("child-run", "", ""); err != nil {
		t.Fatalf("append empty text: %v", err)
	}
	if got := execution.builderForRun(nil, "orphan"); got == nil || got.String() != "" {
		t.Fatalf("nil builder store returned %#v", got)
	}
	execution.markToolResponseSeenForRun("")
	execution.markPostToolTextForRun("")
	if !execution.runHasPostToolText("") {
		t.Fatal("root post-tool text should be visible through empty run id")
	}
	if execution.runNeedsFinalSynthesis("") {
		t.Fatal("root run with post-tool text should not need synthesis")
	}

	execution.calls = append(execution.calls, ToolCall{ID: "blank-run", Status: "RUNNING"})
	ids := execution.snapshotRunIDsLocked()
	for _, id := range ids {
		if id == "" {
			t.Fatalf("snapshotRunIDsLocked contained blank id: %#v", ids)
		}
	}
	if snapshot := execution.runSnapshotLocked("", false); snapshot.ID != "root-run" {
		t.Fatalf("empty run snapshot = %+v", snapshot)
	}
	execution.calls = []ToolCall{{RunID: "root-run", Status: "MYSTERY"}}
	if got := execution.derivedRunStatusForRunLocked("root-run"); got != RunStatusRunning {
		t.Fatalf("unknown tool status derived run status = %q", got)
	}

	if got := googleADKWorkflowRootName(" "); got != "workflow_root" {
		t.Fatalf("googleADKWorkflowRootName(empty) = %q", got)
	}
	if got := workflowChildInstruction("", "inspect fills"); got != "JFTRADE_WORKFLOW_TASK: inspect fills" {
		t.Fatalf("workflowChildInstruction(empty base) = %q", got)
	}
	if got := workflowChildInstruction("base instruction", " "); got != "base instruction" {
		t.Fatalf("workflowChildInstruction(empty task) = %q", got)
	}
	if got := workflowChildInstructionTask(workflowStep{}); got != "" {
		t.Fatalf("workflowChildInstructionTask(empty) = %q", got)
	}
	if got := googleADKAppName(" "); got != "jftrade-default" {
		t.Fatalf("googleADKAppName(empty) = %q", got)
	}
}

func TestHydrateResumedRunWithApprovalsKeepsPendingRoundState(t *testing.T) {
	execution := &googleADKExecution{runID: "run-multi-approval"}
	execution.calls = []ToolCall{{
		ID: "call-follow-up-approval", RunID: "run-multi-approval", ToolName: "approval.required", Status: "RUNNING",
	}}
	execution.summaries = []string{"等待第二轮审批"}
	execution.preToolContent.WriteString("第一轮审批后的分析")
	execution.preToolReasoning.WriteString("继续推理")

	run := Run{
		ID:          "run-multi-approval",
		Status:      RunStatusRunning,
		ResumeState: "approval_resuming",
		PendingApprovals: []Approval{{
			ID: "approval-approved", Status: ApprovalStatusApproved, ToolName: "approval.required",
		}},
	}
	newApprovals := []Approval{{
		ID: "approval-follow-up", Status: ApprovalStatusPending, ToolName: "approval.required",
	}}

	hydrated := hydrateResumedRunWithApprovals(run, execution, newApprovals)
	if hydrated.Status != RunStatusRunning || hydrated.ResumeState != "waiting_approval" {
		t.Fatalf("hydrated run = %+v, want running waiting_approval state", hydrated)
	}
	if len(hydrated.ToolCalls) != 1 || hydrated.ToolCalls[0].ID != "call-follow-up-approval" {
		t.Fatalf("hydrated tool calls = %+v, want resumed execution calls", hydrated.ToolCalls)
	}
	if len(hydrated.ToolSummaries) != 0 {
		t.Fatalf("hydrated tool summaries = %+v, want no per-run summary until the follow-up tool finishes", hydrated.ToolSummaries)
	}
	if hydrated.PreToolContent != "第一轮审批后的分析" || hydrated.PreToolReasoning != "继续推理" {
		t.Fatalf("hydrated pre-tool state = %q / %q", hydrated.PreToolContent, hydrated.PreToolReasoning)
	}
	if len(hydrated.PendingApprovals) != 1 || hydrated.PendingApprovals[0].ID != "approval-follow-up" || hydrated.PendingApprovals[0].Status != ApprovalStatusPending {
		t.Fatalf("hydrated approvals = %+v, want fresh pending approval round", hydrated.PendingApprovals)
	}
}

func TestIsRetryableADKSessionBusyRecognizesSQLiteBusyAppendErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "sqlite busy append", err: errors.New("append event to sessionservice failed: database is locked"), want: true},
		{name: "sqlite busy token", err: errors.New("append event to sessionservice failed: SQLITE_BUSY"), want: true},
		{name: "other append failure", err: errors.New("append event to sessionservice failed: permission denied"), want: false},
		{name: "busy without append prefix", err: errors.New("database is locked"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableADKSessionBusy(tc.err); got != tc.want {
				t.Fatalf("isRetryableADKSessionBusy(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestResolveApprovalRetriesRetryableSessionBusyDuringResume(t *testing.T) {
	ctx := context.Background()
	base := newTestRuntime(t)
	registry := NewToolRegistry()
	var executions atomic.Int32
	registry.Register(ToolDescriptor{
		Name:         "approval.required",
		DisplayName:  "Save draft",
		Description:  "test write tool",
		Category:     "strategy",
		Permission:   "write_strategy",
		AllowedModes: []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
	}, func(context.Context, map[string]any) (any, error) {
		executions.Add(1)
		return map[string]any{"saved": true}, nil
	})
	service := &flakyAppendSessionService{delegate: adksession.InMemoryService()}
	runtime := NewRuntimeWithSessionService(base.Store(), registry, service)
	ensureTestProvider(t, runtime)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent-approval-busy-retry",
		Name:           "Agent Busy Retry",
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

	service.failAppendRemaining.Store(1)
	appendCallsBeforeResume := service.appendCalls.Load()
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	if service.failedAppendAttempts.Load() != 1 {
		t.Fatalf("failed append attempts = %d, want 1 retryable failure", service.failedAppendAttempts.Load())
	}
	if service.appendCalls.Load() <= appendCallsBeforeResume+1 {
		t.Fatalf("append calls after resume = %d, want retry to append more than once", service.appendCalls.Load())
	}
	if executions.Load() != 1 {
		t.Fatalf("tool executions = %d, want exactly 1 execution after retry", executions.Load())
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("resolution run = %+v, want completed run", resolution.Run)
	}
	if len(resolution.Run.PendingApprovals) != 1 || resolution.Run.PendingApprovals[0].Status != ApprovalStatusApproved {
		t.Fatalf("resolution approvals = %+v, want embedded approved approval", resolution.Run.PendingApprovals)
	}
	persistedRun, ok, err := runtime.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok {
		t.Fatalf("persisted run ok=%v err=%v", ok, err)
	}
	if persistedRun.Status != RunStatusCompleted || persistedRun.FinalMessageID == "" {
		t.Fatalf("persisted run = %+v, want completed run with assistant message", persistedRun)
	}
}

func TestGoogleADKExecutionBuffersTextUntilToolsFinish(t *testing.T) {
	t.Parallel()

	var replies []string
	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta: func(delta ChatDelta) error {
			if delta.Reply != "" {
				replies = append(replies, delta.Reply)
			}
			return nil
		},
	}

	if err := execution.appendVisibleText("先给前置结论。", ""); err != nil {
		t.Fatalf("append pre-tool text: %v", err)
	}
	call := execution.ensureCall("call-1", ToolDescriptor{Name: "portfolio.summary", Permission: "read"}, map[string]any{"scope": "all"})
	if call == nil {
		t.Fatal("expected tool call to be created")
	}
	if err := execution.appendVisibleText("这段应该等工具结束后再出来。", ""); err != nil {
		t.Fatalf("append buffered text: %v", err)
	}

	if len(replies) != 1 || replies[0] != "先给前置结论。" {
		t.Fatalf("replies before finish = %#v, want only pre-tool text", replies)
	}

	execution.finishCall(call.ID, map[string]any{"ok": true}, nil)

	if len(replies) != 2 {
		t.Fatalf("replies after finish = %#v, want buffered text flushed", replies)
	}
	if replies[1] != "这段应该等工具结束后再出来。" {
		t.Fatalf("flushed reply = %q, want buffered post-tool text", replies[1])
	}

	preToolContent, preToolReasoning := execution.preToolState()
	if preToolContent != "先给前置结论。" || preToolReasoning != "" {
		t.Fatalf("preToolState = (%q, %q)", preToolContent, preToolReasoning)
	}
}

func TestGoogleADKExecutionRequiresPostToolTextBeforeCompletedStatus(t *testing.T) {
	t.Parallel()

	var snapshots []*Run
	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta: func(delta ChatDelta) error {
			if delta.Run != nil {
				snapshots = append(snapshots, new(*delta.Run))
			}
			return nil
		},
	}

	if err := execution.appendVisibleText("先检查数据。", ""); err != nil {
		t.Fatalf("append pre-tool text: %v", err)
	}
	call := execution.ensureCall("call-1", ToolDescriptor{Name: "portfolio.summary", Permission: "read"}, map[string]any{"scope": "all"})
	execution.finishCall(call.ID, map[string]any{"ok": true}, nil)

	if len(snapshots) < 2 {
		t.Fatalf("snapshots = %d, want at least start + finish", len(snapshots))
	}
	if got := snapshots[len(snapshots)-1].Status; got != RunStatusRunning {
		t.Fatalf("tool-only snapshot status = %q, want %q", got, RunStatusRunning)
	}
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-1",
		Name:     "portfolio.summary",
		Response: map[string]any{"ok": true},
	})
	execution.mu.Lock()
	statusAfterResponse := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if statusAfterResponse != RunStatusRunning {
		t.Fatalf("status after function response = %q, want %q", statusAfterResponse, RunStatusRunning)
	}
	if err := execution.appendVisibleText("基于数据，最终结论如下。", ""); err != nil {
		t.Fatalf("append post-tool text: %v", err)
	}
	execution.mu.Lock()
	finalStatus := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if finalStatus != RunStatusCompleted {
		t.Fatalf("status after post-tool text = %q, want %q", finalStatus, RunStatusCompleted)
	}
}

func TestGoogleADKExecutionRequiresPostToolTextAfterLatestToolResponse(t *testing.T) {
	t.Parallel()

	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta:   func(ChatDelta) error { return nil },
	}

	first := execution.ensureCall("call-1", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"})
	execution.finishCall(first.ID, map[string]any{"ok": true}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-1",
		Name:     "market.candles",
		Response: map[string]any{"ok": true},
	})
	if err := execution.appendVisibleText("第一轮工具后的分析。", ""); err != nil {
		t.Fatalf("append first post-tool text: %v", err)
	}
	second := execution.ensureCall("call-2", ToolDescriptor{Name: "strategy.definitions", Permission: "read"}, map[string]any{"query": "TME"})
	execution.finishCall(second.ID, map[string]any{"ok": true}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-2",
		Name:     "strategy.definitions",
		Response: map[string]any{"ok": true},
	})

	execution.mu.Lock()
	statusAfterSecondTool := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if statusAfterSecondTool != RunStatusRunning {
		t.Fatalf("status after second tool response = %q, want %q", statusAfterSecondTool, RunStatusRunning)
	}
	if !execution.runNeedsFinalSynthesis("run-1") {
		t.Fatal("run should need final synthesis after latest tool response")
	}

	if err := execution.appendVisibleText("第二轮工具后的最终结论。", ""); err != nil {
		t.Fatalf("append final post-tool text: %v", err)
	}
	execution.mu.Lock()
	finalStatus := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if finalStatus != RunStatusCompleted {
		t.Fatalf("status after latest post-tool text = %q, want %q", finalStatus, RunStatusCompleted)
	}
}

func TestGoogleADKExecutionFlushBufferedTextWithoutDeadlock(t *testing.T) {
	t.Parallel()

	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta:   func(ChatDelta) error { return nil },
	}

	call := execution.ensureCall("call-1", ToolDescriptor{Name: "portfolio.summary", Permission: "read"}, map[string]any{"scope": "all"})
	if err := execution.appendVisibleText("buffered", ""); err != nil {
		t.Fatalf("appendVisibleText: %v", err)
	}
	execution.finishCall(call.ID, map[string]any{"ok": true}, nil)

	if err := execution.flushBufferedTextIfReady(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("flushBufferedTextIfReady: %v", err)
	}
}

func TestGoogleADKExecutionMarksToolsetFunctionResponseAsSucceeded(t *testing.T) {
	t.Parallel()

	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta:   func(ChatDelta) error { return nil },
	}

	call := execution.ensureCall("call-1", ToolDescriptor{Name: "load_skill", Permission: "read"}, map[string]any{"skill_name": "portfolio"})
	if call == nil {
		t.Fatal("expected tool call to be created")
	}

	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-1",
		Name:     "load_skill",
		Response: map[string]any{"result": "ok"},
	})

	toolContext := execution.toolContext()
	if len(toolContext.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(toolContext.calls))
	}
	if toolContext.calls[0].Status != "SUCCEEDED" {
		t.Fatalf("tool call status = %q, want SUCCEEDED", toolContext.calls[0].Status)
	}
	if toolContext.calls[0].CompletedAt == nil {
		t.Fatal("expected completed timestamp to be recorded")
	}
}

func TestGoogleADKExecutionPersistsTimedOutToolFailureOnRunningSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newTestRuntime(t)
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-tool-timeout",
		SessionID: "session-1",
		AgentID:   "agent-1",
		Status:    RunStatusRunning,
		Message:   "running",
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
		StartedAt: nowString(),
		Usage:     &RunUsage{},
	})
	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     run.ID,
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			return runtime.persistRunActivitySnapshot(ctx, snapshot)
		},
	}

	call := execution.ensureCall("call-timeout", ToolDescriptor{
		Name:       "portfolio.summary",
		Permission: "read",
	}, map[string]any{"scope": "all"})
	execution.finishCall(call.ID, nil, context.DeadlineExceeded)

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusRunning {
		t.Fatalf("stored status = %q, want %q", stored.Status, RunStatusRunning)
	}
	if stored.ErrorCode != "" {
		t.Fatalf("stored error code = %q, want empty for activity snapshot", stored.ErrorCode)
	}
	if len(stored.ToolCalls) != 1 || stored.ToolCalls[0].Status != "TIMED_OUT" {
		t.Fatalf("stored tool calls = %+v, want timed out call", stored.ToolCalls)
	}
	if stored.ToolCalls[0].Error == nil || *stored.ToolCalls[0].Error != "tool execution timed out: context deadline exceeded" {
		t.Fatalf("stored tool error = %#v, want explicit timeout message", stored.ToolCalls[0].Error)
	}
	if stored.FailureReason != "" {
		t.Fatalf("stored failure reason = %q, want empty for activity snapshot", stored.FailureReason)
	}
	if stored.Degraded {
		t.Fatalf("stored degraded = %v, want false for activity snapshot", stored.Degraded)
	}
}

func TestGoogleADKExecutionDoesNotPersistCompletedActivitySnapshot(t *testing.T) {
	t.Parallel()
	execution := &googleADKExecution{
		runID:                   "run-activity",
		calls:                   []ToolCall{{RunID: "run-activity", ToolName: "strategy.inspect", Status: "SUCCEEDED"}},
		toolResponseSeenByRunID: map[string]bool{"run-activity": true},
		postToolTextByRunID:     map[string]bool{"run-activity": true},
		toolResponseSeqByRunID:  map[string]int{"run-activity": 1},
		postToolTextSeqByRunID:  map[string]int{"run-activity": 1},
	}
	if status := execution.derivedRunStatusForRunLocked("run-activity"); status != RunStatusCompleted {
		t.Fatalf("derived display status = %q, want completed after post-tool text", status)
	}
	if status := execution.persistedRunStatusForRunLocked("run-activity"); status != RunStatusRunning {
		t.Fatalf("persisted activity status = %q, want running until invocation returns", status)
	}
}

func TestGoogleADKExecutionEmitsAuthoritativePauseRequestedSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	storedRun := mustSaveRun(t, runtime, Run{
		ID:               "run-goal-pause-stream",
		SessionID:        "session-1",
		AgentID:          "agent-1",
		Status:           RunStatusRunning,
		Message:          "目标将在当前轮结束后暂停。",
		WorkMode:         WorkModeLoop,
		Objective:        "推进目标",
		WorkflowStatus:   workflowStatusRunning,
		PauseRequestedAt: &now,
		ResumeState:      "user_pause_requested",
		CreatedAt:        now,
		UpdatedAt:        now,
		StartedAt:        now,
		Usage:            &RunUsage{},
	})
	staleSnapshot := storedRun
	staleSnapshot.Message = "goal running"
	staleSnapshot.PauseRequestedAt = nil
	staleSnapshot.ResumeState = ""

	var emitted Run
	execution := &googleADKExecution{
		sessionID:           storedRun.SessionID,
		agent:               Agent{ID: storedRun.AgentID},
		runID:               storedRun.ID,
		runSnapshotBaseByID: map[string]Run{storedRun.ID: staleSnapshot},
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			return runtime.persistRunActivitySnapshot(ctx, snapshot)
		},
		onDelta: func(delta ChatDelta) error {
			if delta.Run != nil {
				emitted = *delta.Run
			}
			return nil
		},
	}

	execution.emitRunSnapshotLocked()

	if emitted.PauseRequestedAt == nil || emitted.ResumeState != "user_pause_requested" {
		t.Fatalf("emitted run = %+v, want authoritative pause request fields", emitted)
	}
}
