package adk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

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

	pendingParent := newParent("wf-pending-parent", WorkModeLoop)
	pendingChild := saveChild(pendingParent, RunStatusPending, "waiting approval")
	synced, err := runtime.syncParentWorkflowFromChild(ctx, pendingChild)
	if err != nil || synced == nil || synced.Status != RunStatusPending || synced.WorkflowStatus != workflowStatusPaused || len(synced.PendingApprovals) != 1 {
		t.Fatalf("sync pending parent=%+v err=%v", synced, err)
	}
	continued, err := runtime.continueParentWorkflowAfterChild(ctx, pendingChild)
	if err != nil || continued == nil || continued.Status != RunStatusPending {
		t.Fatalf("continue pending parent=%+v err=%v", continued, err)
	}

	runningParent := newParent("wf-running-parent", WorkModeLoop)
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
		parent := newParent("wf-terminal-"+tc.status, WorkModeLoop)
		child := saveChild(parent, tc.status, "child "+tc.status)
		terminated, err := runtime.terminateParentWorkflowFromChild(ctx, parent, child)
		if err != nil {
			t.Fatalf("terminate %s parent: %v", tc.status, err)
		}
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
	if _, _, err := executor.planWorkflowSteps(ctx, workflowRequest{Agent: Agent{ID: "missing", ProviderID: "missing"}}, WorkModeLoop, "objective"); err == nil || !strings.Contains(err.Error(), "workflow planner failed") {
		t.Fatalf("planWorkflowSteps err = %v, want planner failed", err)
	}

	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-executor-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	steps := []workflowStep{
		{Title: "First", Description: "Desc", Message: "First message", DependencyID: "first", Order: 1, AgentRole: "researcher", ModeHint: WorkModeChat, PlanSource: workflowPlanSourcePlanner, WorkflowMode: WorkModeLoop},
		{Title: "Second", Message: "Second message", DependsOn: []string{"first", "__previous_step_1"}, DependencyID: "second", Order: 2, PlanSource: workflowPlanSourcePlanner, WorkflowMode: WorkModeLoop},
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
		Session: session, Message: "run children", Mode: WorkModeLoop,
	}, failingParent, []workflowStep{{Title: "Bad", Message: "bad child"}}, nil)
	if err != nil || response.Run.Status != RunStatusFailed || response.Run.FailureReason == "" {
		t.Fatalf("runPlannedGoogleADKWorkflow response=%+v err=%v, want failed response", response, err)
	}
	if _, _, err := executor.startWorkflowChildRuns(ctx, workflowRequest{
		Agent:   Agent{ID: "bad-child-agent", Name: "Bad Child", ProviderID: "missing-provider"},
		Session: session, Message: "run child", Mode: WorkModeLoop,
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
	if err := runtime.markApprovalContinuationFailed(ctx, failedRun.ID, errors.New("append event to SessionService: database is locked")); err != nil {
		t.Fatalf("mark approval continuation failed: %v", err)
	}
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
	if err := runtime.markApprovalContinuationFailed(ctx, unchangedPending.ID, errors.New("ignored")); err != nil {
		t.Fatalf("ignore still-pending approval continuation: %v", err)
	}
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
