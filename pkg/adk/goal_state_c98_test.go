package adk

import (
	"context"
	"strings"
	"testing"
)

func TestCoverage98GoalWorkflowStateBoundariesFailClosedAndRemainResumable(t *testing.T) {
	ctx := context.Background()

	t.Run("model bootstrap failures become a durable failed goal response", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "coverage98-goal-missing-provider", "goal bootstrap failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-bootstrap-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})

		response, err := runtime.workflowExecutor().continueADKGoalWorkflow(ctx, workflowRequest{
			Agent:   Agent{ID: session.AgentID, Name: "Unavailable Model", ProviderID: "coverage98-missing-provider", Status: AgentStatusEnabled},
			Session: session, Mode: WorkModeLoop,
		}, parent, nil, "continue", 1, 1)
		if err != nil {
			t.Fatalf("continueADKGoalWorkflow returned transport error: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed || strings.TrimSpace(response.Reply) == "" {
			t.Fatalf("model bootstrap failure response = %+v", response)
		}
	})

	t.Run("invalid resume iteration is normalized into a durable iteration-limit pause", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "coverage98-goal-iteration-agent", Name: "Goal Iteration Boundary", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal iteration boundary")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-iteration-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})

		response, err := runtime.workflowExecutor().continueADKGoalWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, nil, "continue", 0, 0)
		if err != nil {
			t.Fatalf("continueADKGoalWorkflow iteration boundary: %v", err)
		}
		if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "iteration_limit" || response.Run.PausedReason != "iteration_limit" || response.Run.WorkflowEngine != WorkflowEngineADK2Loop {
			t.Fatalf("iteration-limit response = %+v", response.Run)
		}
	})

	t.Run("missing final reply asks for a reply while a user pause wins immediately", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "coverage98-goal-decision-agent", Name: "Goal Decision Boundary", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal decision boundary")
		executor := runtime.workflowExecutor()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-no-final-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})

		executionWithoutPostToolReply := newBareGoogleADKExecution(parent.ID)
		executionWithoutPostToolReply.calls = []ToolCall{{ID: "coverage98-finished-tool", RunID: parent.ID, ToolName: workflowTasksListTool, Status: "SUCCEEDED"}}
		updated, _, snapshot, done, response, prompt := executor.resolveGoalWorkflowDecision(ctx, workflowRequest{Session: session}, parent, nil,
			executionWithoutPostToolReply, &workflowGoalDecision{}, openAIChatResult{}, "visible progress", "", 1, false)
		if done || snapshot.status != "" || response.Run.ID != "" || !strings.Contains(prompt, "最终可见答复") || updated.ID != parent.ID {
			t.Fatalf("missing final reply resolution = parent:%+v decision:%+v done:%v response:%+v prompt:%q", updated, snapshot, done, response, prompt)
		}

		pauseRequestedAt := nowString()
		pausedParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-pause-decision-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		updated, _, _, done, response, prompt = executor.resolveGoalWorkflowDecision(ctx, workflowRequest{Session: session}, pausedParent, nil,
			newBareGoogleADKExecution(pausedParent.ID), &workflowGoalDecision{}, openAIChatResult{}, "progress before pause", "", 1, false)
		if !done || prompt != "" || updated.Status != RunStatusPaused || response.Run.Status != RunStatusPaused || response.Reply != "progress before pause" {
			t.Fatalf("pause-first resolution = parent:%+v done:%v response:%+v prompt:%q", updated, done, response, prompt)
		}

		unchanged, replyResult, pauseDone, pauseResponse := executor.pauseAfterMissingGoalDecision(ctx, workflowRequest{Session: session}, parent,
			openAIChatResult{}, "visible fallback", workflowGoalDecisionSnapshot{}, 1)
		if pauseDone || pauseResponse.Run.ID != "" || replyResult.Reply != "" || unchanged.ID != parent.ID {
			t.Fatalf("missing-decision fallback = parent:%+v reply:%+v done:%v response:%+v", unchanged, replyResult, pauseDone, pauseResponse)
		}
	})

	t.Run("continuation honors a concurrent pause and interrupted call filtering only removes internal calls", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "coverage98-goal-continue-agent", Name: "Goal Continue Boundary", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal continue boundary")
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-continue-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		continued, response, paused, prompt := runtime.workflowExecutor().finishContinueGoalWorkflow(ctx, workflowRequest{Session: session}, parent,
			openAIChatResult{}, workflowGoalDecisionSnapshot{reason: "await review"}, "", 2)
		if !paused || prompt != "" || continued.Status != RunStatusPaused || response.Run.Status != RunStatusPaused {
			t.Fatalf("continue while pausing = parent:%+v response:%+v paused:%v prompt:%q", continued, response, paused, prompt)
		}

		pauseErr := errUserGoalPauseRequested.Error()
		if interruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: workflowTaskClaimTool, Status: "FAILED"}) {
			t.Fatal("failed workflow call without an interruption error must remain visible")
		}
		if interruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: workflowTaskClaimTool, Status: "FAILED", Error: new("ordinary failure")}) {
			t.Fatal("ordinary workflow failure must remain visible")
		}
		if interruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: "market.snapshot", Status: "FAILED", Error: &pauseErr}) {
			t.Fatal("non-workflow interruption must remain visible")
		}
	})
}

func TestCoverage98WorkflowApprovalStateTransitionsPersistTheObservableOutcome(t *testing.T) {
	ctx := context.Background()

	t.Run("child pending states project their exact parent lifecycle", func(t *testing.T) {
		for _, status := range []string{RunStatusPendingInput, RunStatusPending, RunStatusRunning} {
			t.Run(strings.ToLower(status), func(t *testing.T) {
				runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "sync-"+strings.ToLower(status))
				parent := mustSaveRun(t, runtime, Run{
					ID: "coverage98-sync-parent-" + strings.ToLower(status), SessionID: session.ID, AgentID: agent.ID,
					Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
					CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
				})
				child := Run{ID: "coverage98-sync-child-" + strings.ToLower(status), ParentRunID: parent.ID, Status: status, Message: "child " + status}
				updated, err := runtime.syncParentWorkflowFromChild(ctx, child)
				if err != nil || updated == nil || updated.Status != status || updated.Message != child.Message {
					t.Fatalf("sync %s parent=%+v err=%v", status, updated, err)
				}
				wantWorkflow := workflowStatusPaused
				if status == RunStatusRunning {
					wantWorkflow = workflowStatusRunning
				}
				if updated.WorkflowStatus != wantWorkflow {
					t.Fatalf("sync %s workflow status = %q, want %q", status, updated.WorkflowStatus, wantWorkflow)
				}
			})
		}
	})

	t.Run("pause requests and terminal persistence failures do not become silent success", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "pause-and-terminal")
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-sync-pause-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		updated, err := runtime.syncParentWorkflowFromChild(ctx, Run{ID: "coverage98-sync-pause-child", ParentRunID: parent.ID, Status: RunStatusCompleted})
		if err != nil || updated == nil || updated.Status != RunStatusPaused || updated.PausedReason != "user" {
			t.Fatalf("requested pause projection = %+v, %v", updated, err)
		}

		terminalParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-terminal-save-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_terminal_parent BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+terminalParent.ID+`' BEGIN SELECT RAISE(FAIL, 'terminal parent write rejected'); END`); err != nil {
			t.Fatalf("create terminal failure trigger: %v", err)
		}
		terminated := runtime.terminateParentWorkflowFromChild(ctx, terminalParent, Run{ID: "coverage98-terminal-child", ParentRunID: terminalParent.ID, Status: RunStatusFailed, Message: "child failed"})
		if terminated.Status != RunStatusFailed || terminated.WorkflowStatus != workflowStatusFailed {
			t.Fatalf("terminal projection = %+v", terminated)
		}
		stored, ok, err := runtime.Store().Run(ctx, terminalParent.ID)
		if err != nil || !ok || stored.Status != RunStatusRunning {
			t.Fatalf("failed terminal persistence must not be reported as stored: %+v/%v/%v", stored, ok, err)
		}
	})

	t.Run("resume reconciler pauses on request and exposes run-store failures", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "resume-and-store")
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-requested-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		resumed, err := runtime.workflowExecutor().resumeLoopWorkflow(ctx, session, parent)
		if err != nil || resumed.Status != RunStatusPaused || resumed.PausedReason != "user" {
			t.Fatalf("resume requested pause = %+v, %v", resumed, err)
		}

		brokenParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-reconcile-store-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{ChildRunID: "coverage98-reconcile-store-child"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop run table: %v", err)
		}
		if _, _, err := runtime.workflowExecutor().reconcileWorkflowChildren(ctx, brokenParent); err == nil {
			t.Fatal("child run storage failure was swallowed")
		}
	})
}
