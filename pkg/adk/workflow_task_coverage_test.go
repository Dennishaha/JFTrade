package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWorkflowGoalDecisionAndUtilityContracts(t *testing.T) {
	var nilDecision *workflowGoalDecision
	nilDecision.reset()
	nilDecision.beginDecision()
	nilDecision.setComplete("ignored")
	nilDecision.setContinue("ignored")
	if nilDecision.decisionPhase() || nilDecision.snapshot().status != "" {
		t.Fatal("nil goal decision must remain inert")
	}

	decision := &workflowGoalDecision{}
	decision.reset()
	if decision.decisionPhase() || decision.snapshot().status != "" {
		t.Fatalf("reset decision = %+v", decision.snapshot())
	}
	decision.beginDecision()
	if !decision.decisionPhase() {
		t.Fatal("decision phase was not entered")
	}
	decision.setComplete(" complete summary ")
	if snapshot := decision.snapshot(); snapshot.status != "complete" || snapshot.summary != "complete summary" || snapshot.reason != "" {
		t.Fatalf("complete snapshot = %+v", snapshot)
	}
	decision.setContinue(" keep researching ")
	if snapshot := decision.snapshot(); snapshot.status != "continue" || snapshot.reason != "keep researching" || snapshot.summary != "" {
		t.Fatalf("continue snapshot = %+v", snapshot)
	}

	parent := Run{ID: "goal", Objective: "完成研究", UserMessage: "分析市场", WorkMode: WorkModeLoop}
	if !strings.Contains(goalOrchestratorUserMessage(parent), "总体目标：完成研究") || !strings.Contains(goalDecisionPrompt(parent, "已有回复", true), workflowGoalCompleteTool) {
		t.Fatal("goal prompts lost their user objective or decision contract")
	}
	if !strings.Contains(goalFinalReplyPrompt(parent), "完成研究") || !strings.Contains(goalOrchestratorContinueNudge(parent, ""), "目标尚未完成") {
		t.Fatal("goal follow-up prompts lost their state")
	}
	if got := workflowTaskResultSummaries([]Task{{ResultSummary: " first "}, {ResultSummary: " "}, {ResultSummary: "second"}}); strings.Join(got, ",") != "first,second" {
		t.Fatalf("task result summaries = %v", got)
	}
	if got := plannerStringSliceArg(map[string]any{"items": []any{" first ", 2, ""}}, "items"); strings.Join(got, ",") != "first,2" {
		t.Fatalf("planner string slice = %v", got)
	}
	if plannerStringSliceArg(map[string]any{"items": []string{"not-any"}}, "items") != nil {
		t.Fatal("typed non-any slice should not be accepted as tool arguments")
	}
}

func TestPauseGoalWorkflowPrunesInterruptedInternalToolCalls(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	pauseRequestedAt := nowString()
	pauseError := errUserGoalPauseRequested.Error()
	parent := Run{
		ID: "pause-prune-parent", SessionID: "pause-prune-session", AgentID: "pause-prune-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &pauseRequestedAt,
		ToolCalls: []ToolCall{
			{ID: "running-internal", RunID: "pause-prune-parent", ToolName: workflowTasksListTool, Status: "RUNNING"},
			{ID: "running-child", RunID: "child", ToolName: workflowTasksListTool, Status: "RUNNING"},
			{ID: "failed-goal", RunID: "pause-prune-parent", ToolName: workflowGoalCompleteTool, Status: "FAILED", Error: &pauseError},
			{ID: "finished", RunID: "pause-prune-parent", ToolName: workflowTasksListTool, Status: "SUCCEEDED"},
		},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	}
	mustSaveRun(t, runtime, parent)

	paused, response, didPause := executor.pauseADKGoalWorkflowIfRequested(ctx, workflowRequest{Session: Session{ID: parent.SessionID}}, parent, 2, "当前进度")
	if !didPause || paused.Status != RunStatusPaused || paused.PausedReason != "user" || paused.Iteration != 2 || response.Reply != "当前进度" {
		t.Fatalf("paused goal = %+v response=%+v", paused, response)
	}
	if len(paused.ToolCalls) != 2 || paused.ToolCalls[0].ID != "running-child" || paused.ToolCalls[1].ID != "finished" {
		t.Fatalf("paused calls = %+v", paused.ToolCalls)
	}
	if _, changed := pruneInterruptedGoalWorkflowToolCalls(paused); changed {
		t.Fatal("already pruned goal should not change a second time")
	}
	if interruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: "market.snapshot", Status: "RUNNING"}) {
		t.Fatal("non-workflow tool must not be pruned during a user pause")
	}
}

func TestPrepareGoalWorkflowTurnHandlesPendingChildrenBlockedTasksAndErrors(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	session := Session{ID: "goal-turn-session", AgentID: "goal-turn-agent"}
	now := nowString()

	pendingParent := Run{
		ID: "goal-turn-parent-pending", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "goal-turn-task-pending", ChildRunID: "goal-turn-child-pending", Status: "IN_PROGRESS"}},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	}
	pendingTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "goal-turn-task-pending", Title: "Pending", Status: "IN_PROGRESS", RunID: pendingParent.ID})
	if err != nil {
		t.Fatalf("SaveTask pending: %v", err)
	}
	mustSaveRun(t, runtime, pendingParent)
	mustSaveRun(t, runtime, Run{
		ID: "goal-turn-child-pending", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: pendingParent.ID,
		Status: RunStatusPendingInput, Message: "需要用户回答", InputRequest: &InputRequest{ID: "input", Status: InputRequestStatusPending},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	pendingExecution := &googleADKExecution{runID: pendingParent.ID, runSnapshotBaseByID: map[string]Run{pendingParent.ID: pendingParent}}
	updated, reply, done, _ := executor.prepareGoalWorkflowTurn(ctx, workflowRequest{Session: session}, pendingParent, []Task{pendingTask}, pendingExecution, nil, 1)
	if !done || updated.Status != RunStatusPendingInput || updated.WorkflowStatus != workflowStatusPaused || reply.Reply != "工作流正在等待用户回答。" {
		t.Fatalf("pending-child turn = %+v reply=%+v", updated, reply)
	}

	blockedParent := Run{
		ID: "goal-turn-parent-blocked", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "goal-turn-task-blocked", Status: "BLOCKED"}},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	}
	blockedTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "goal-turn-task-blocked", Title: "Blocked", Description: "dependency unavailable", Status: "BLOCKED", RunID: blockedParent.ID})
	if err != nil {
		t.Fatalf("SaveTask blocked: %v", err)
	}
	mustSaveRun(t, runtime, blockedParent)
	updated, reply, done, _ = executor.prepareGoalWorkflowTurn(ctx, workflowRequest{Session: session}, blockedParent, []Task{blockedTask}, &googleADKExecution{runID: blockedParent.ID}, nil, 1)
	if !done || updated.Status != RunStatusFailed || updated.ErrorCode != "WORKFLOW_TASK_BLOCKED" || !strings.Contains(reply.Reply, "dependency unavailable") {
		t.Fatalf("blocked-task turn = %+v reply=%+v", updated, reply)
	}

	errorParent := Run{ID: "goal-turn-parent-error", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{}}
	mustSaveRun(t, runtime, errorParent)
	updated, _, done, _ = executor.prepareGoalWorkflowTurn(ctx, workflowRequest{Session: session}, errorParent, nil, &googleADKExecution{runID: errorParent.ID}, errors.New("model failed"), 1)
	if !done || updated.Status != RunStatusFailed || updated.ErrorCode == "" {
		t.Fatalf("error turn = %+v", updated)
	}
}
