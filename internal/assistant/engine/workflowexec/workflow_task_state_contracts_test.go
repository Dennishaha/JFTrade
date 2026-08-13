package workflowexec

import (
	"context"
	"errors"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"
)

func TestPauseGoalWorkflowPrunesInterruptedInternalToolCalls(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	pauseRequestedAt := assistantmodel.NowString()
	pauseError := assistantmodel.ErrUserGoalPauseRequested.Error()
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
		CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	}
	mustSaveRun(t, runtime, parent)

	paused, response, didPause, err := executor.PauseADKGoalWorkflowIfRequested(ctx, workflowRequest{Session: Session{ID: parent.SessionID}}, parent, 2, "当前进度")
	if err != nil {
		t.Fatalf("pause goal workflow: %v", err)
	}
	if !didPause || paused.Status != RunStatusPaused || paused.PausedReason != "user" || paused.Iteration != 2 || response.Reply != "当前进度" {
		t.Fatalf("paused goal = %+v response=%+v", paused, response)
	}
	if len(paused.ToolCalls) != 2 || paused.ToolCalls[0].ID != "running-child" || paused.ToolCalls[1].ID != "finished" {
		t.Fatalf("paused calls = %+v", paused.ToolCalls)
	}
	if _, changed := assistantmodel.PruneInterruptedGoalWorkflowToolCalls(paused); changed {
		t.Fatal("already pruned goal should not change a second time")
	}
	if assistantmodel.InterruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: "market.snapshot", Status: "RUNNING"}) {
		t.Fatal("non-workflow tool must not be pruned during a user pause")
	}
}

func TestPrepareGoalWorkflowTurnHandlesPendingChildrenBlockedTasksAndErrors(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	session := Session{ID: "goal-turn-session", AgentID: "goal-turn-agent"}
	now := assistantmodel.NowString()

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
	updated, reply, done, _, err := executor.PrepareGoalWorkflowTurn(ctx, workflowRequest{Session: session}, pendingParent, []Task{pendingTask}, &fakeWorkflowExecutionHandle{}, nil, 1)
	if err != nil {
		t.Fatalf("prepare pending-child turn: %v", err)
	}
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
	updated, reply, done, _, err = executor.PrepareGoalWorkflowTurn(ctx, workflowRequest{Session: session}, blockedParent, []Task{blockedTask}, &fakeWorkflowExecutionHandle{}, nil, 1)
	if err != nil {
		t.Fatalf("prepare blocked-task turn: %v", err)
	}
	if !done || updated.Status != RunStatusFailed || updated.ErrorCode != "WORKFLOW_TASK_BLOCKED" || !strings.Contains(reply.Reply, "dependency unavailable") {
		t.Fatalf("blocked-task turn = %+v reply=%+v", updated, reply)
	}

	errorParent := Run{ID: "goal-turn-parent-error", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{}}
	mustSaveRun(t, runtime, errorParent)
	updated, _, done, _, err = executor.PrepareGoalWorkflowTurn(ctx, workflowRequest{Session: session}, errorParent, nil, &fakeWorkflowExecutionHandle{}, errors.New("model failed"), 1)
	if err != nil {
		t.Fatalf("prepare failed-model turn: %v", err)
	}
	if !done || updated.Status != RunStatusFailed || updated.ErrorCode == "" {
		t.Fatalf("error turn = %+v", updated)
	}
}
