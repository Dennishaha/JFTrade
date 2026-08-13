package workflowexec

import (
	"context"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
)

func TestGoalWorkflowTerminalHelpersPersistCompletionContinuationAndStablePause(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "goal-terminal-agent", "goal terminal helpers")
	executor := (&WorkflowExecutor{runtime: runtime})
	parent := mustSaveRun(t, runtime, Run{
		ID: "goal-terminal-parent", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, Objective: "verify terminal behavior",
		CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "goal-terminal-task", Title: "Verify", Description: "collect result", Status: "DONE", RunID: parent.ID, Order: 1})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = assistantmodel.WorkflowPlanFromTasks([]Task{task}, nil)
	mustSaveRun(t, runtime, parent)

	if got := executor.CompleteGoalReply(ctx, parent, []Task{task}, workflowGoalDecisionSnapshot{Summary: " explicit summary "}, "visible reply"); got != "explicit summary" {
		t.Fatalf("explicit completion reply = %q", got)
	}
	if got := executor.CompleteGoalReply(ctx, parent, []Task{task}, workflowGoalDecisionSnapshot{}, " visible reply "); got != " visible reply " {
		t.Fatalf("visible completion reply = %q", got)
	}
	if got := executor.CompleteGoalReply(ctx, parent, []Task{task}, workflowGoalDecisionSnapshot{}, ""); !strings.Contains(got, "目标模式已完成") {
		t.Fatalf("task-summary completion reply = %q", got)
	}

	completed, response, done, prompt, err := executor.FinishCompleteGoalWorkflow(ctx, workflowRequest{Session: session}, parent, []Task{task}, jfadk.AssistantExecutionResult{Reply: "model reply"}, workflowGoalDecisionSnapshot{}, "model reply", 2)
	if err != nil {
		t.Fatalf("finish complete goal: %v", err)
	}
	if !done || prompt != "" || completed.Status != RunStatusCompleted || completed.WorkflowStatus != workflowStatusComplete || completed.Iteration != 2 || response.Run.Status != RunStatusCompleted {
		t.Fatalf("completed goal workflow = %+v response=%+v done=%v prompt=%q", completed, response, done, prompt)
	}

	continuingParent := mustSaveRun(t, runtime, Run{
		ID: "goal-continue-parent", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})
	continued, emptyResponse, paused, nudge, err := executor.FinishContinueGoalWorkflow(ctx, workflowRequest{Session: session}, continuingParent, jfadk.AssistantExecutionResult{}, workflowGoalDecisionSnapshot{Reason: "need one more verification"}, "", 3)
	if err != nil {
		t.Fatalf("finish continued goal: %v", err)
	}
	if paused || emptyResponse.Run.ID != "" || continued.Status != RunStatusRunning || continued.Iteration != 3 || !strings.Contains(nudge, "need one more verification") {
		t.Fatalf("continued goal workflow = %+v response=%+v paused=%v nudge=%q", continued, emptyResponse, paused, nudge)
	}

	decision := &workflowGoalDecision{}
	decision.SetComplete("done")
	_, _, snapshot, done, _, _, err := executor.ResolveGoalWorkflowDecision(ctx, workflowRequest{Session: session}, continuingParent, nil, &fakeWorkflowExecutionHandle{}, decision, jfadk.AssistantExecutionResult{Reply: "reply"}, "reply", "", 1, false)
	if err != nil {
		t.Fatalf("resolve recorded decision: %v", err)
	}
	if done || snapshot.Status != "complete" {
		t.Fatalf("already-recorded goal decision = %+v done=%v", snapshot, done)
	}

	pauseErr := assistantmodel.ErrUserGoalPauseRequested.Error()
	pauseRequestedAt := assistantmodel.NowString()
	pausedParent := mustSaveRun(t, runtime, Run{
		ID: "goal-already-paused", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusPaused,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused, PausedReason: "user", Message: "already paused",
		PauseRequestedAt: &pauseRequestedAt,
		ToolCalls:        []ToolCall{{ID: "drop", RunID: "goal-already-paused", ToolName: workflowTasksListTool, Status: "FAILED", Error: &pauseErr}},
		CreatedAt:        assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})
	stable, stableResponse, didPause, err := executor.PauseADKGoalWorkflowIfRequested(ctx, workflowRequest{Session: session}, pausedParent, 1, "")
	if err != nil {
		t.Fatalf("clean already-paused goal: %v", err)
	}
	if !didPause || stable.Status != RunStatusPaused || len(stable.ToolCalls) != 0 || stable.Message != "already paused" || stableResponse.Run.ID != stable.ID {
		t.Fatalf("already-paused goal cleanup = %+v response=%+v didPause=%v", stable, stableResponse, didPause)
	}
}
