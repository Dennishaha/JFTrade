package workflowexec

import (
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"
)

func TestWorkflowTaskToolsetBusinessLifecycle(t *testing.T) {
	runtime := newTestRuntime(t)
	ctx := t.Context()
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-toolset-business-agent", Name: "Workflow Toolset Business", Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "workflow toolset lifecycle")
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-toolset-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, Objective: "finish rollout",
	})
	first, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-first", Title: "Inspect state", Status: "TODO", AgentID: agent.ID, RunID: parent.ID, Order: 1,
	})
	if err != nil {
		t.Fatalf("SaveTask first: %v", err)
	}
	second, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-second", Title: "Apply fix", Status: "TODO", AgentID: agent.ID, RunID: parent.ID, Order: 2, DependsOn: []string{first.ID},
	})
	if err != nil {
		t.Fatalf("SaveTask second: %v", err)
	}
	parent.WorkflowPlan = assistantmodel.WorkflowPlanFromTasks([]Task{first, second}, nil)
	mustSaveRun(t, runtime, parent)

	decision := &workflowGoalDecision{}
	toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, parent.ID, "")
	toolset.Req = workflowRequest{Agent: agent, Session: session, GoalDecision: decision}

	listed, err := toolset.List(nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if ready, ok := listed["readyTasks"].([]map[string]any); !ok || len(ready) != 1 || ready[0]["id"] != first.ID {
		t.Fatalf("ready tasks = %#v", listed["readyTasks"])
	}
	if _, err := toolset.Add(map[string]any{"title": "Bad dependency", "dependsOn": []any{"missing-task"}}); err == nil || !strings.Contains(err.Error(), "dependency not found") {
		t.Fatalf("add missing dependency err = %v", err)
	}

	claim, err := toolset.Claim(map[string]any{"taskId": first.ID, "executor": workflowTaskExecutorSelf})
	if err != nil || claim["success"] != true {
		t.Fatalf("claim = %#v err=%v", claim, err)
	}
	complete, err := toolset.Complete(map[string]any{"taskId": first.ID})
	if err != nil || complete["success"] != true {
		t.Fatalf("complete = %#v err=%v", complete, err)
	}
	doneFirst, ok, err := runtime.Store().Task(ctx, first.ID)
	if err != nil || !ok || doneFirst.Status != "DONE" || !strings.Contains(doneFirst.ResultSummary, "Inspect state") {
		t.Fatalf("completed first task = %#v ok=%v err=%v", doneFirst, ok, err)
	}

	added, err := toolset.Add(map[string]any{
		"title": "Verify rollout", "message": "Verify the changed behavior", "dependsOn": []any{first.ID},
		"agentRole": "reviewer", "modeHint": WorkModeLoop,
	})
	if err != nil || added["success"] != true {
		t.Fatalf("add runtime task = %#v err=%v", added, err)
	}
	runtimeTask := added["task"].(map[string]any)
	if runtimeTask["planSource"] != workflowPlanSourceRuntime || runtimeTask["agentRole"] != "reviewer" {
		t.Fatalf("runtime task summary = %#v", runtimeTask)
	}

	blocked, err := toolset.Block(map[string]any{"taskId": second.ID})
	if err != nil || blocked["success"] != true {
		t.Fatalf("block = %#v err=%v", blocked, err)
	}
	completion, err := toolset.GoalComplete(map[string]any{"summary": "done"})
	if err != nil {
		t.Fatalf("goalComplete with blockers: %v", err)
	}
	if completion["success"] != false || completion["status"] != "blocked" {
		t.Fatalf("goalComplete blockers = %#v", completion)
	}
	if cont, err := toolset.GoalContinue(map[string]any{}); err != nil || cont["status"] != "continue" {
		t.Fatalf("goalContinue = %#v err=%v", cont, err)
	}
	if snap := decision.Snapshot(); snap.Status != "continue" || snap.Reason == "" {
		t.Fatalf("goal decision after continue = status:%q reason:%q", snap.Status, snap.Reason)
	}

	done := "DONE"
	if _, err := runtime.Store().UpdateTask(ctx, second.ID, TaskPatchRequest{Status: &done}); err != nil {
		t.Fatalf("UpdateTask second done: %v", err)
	}
	runtimeTaskID := runtimeTask["id"].(string)
	if _, err := runtime.Store().UpdateTask(ctx, runtimeTaskID, TaskPatchRequest{Status: &done}); err != nil {
		t.Fatalf("UpdateTask runtime done: %v", err)
	}
	completion, err = toolset.GoalComplete(map[string]any{"summary": "workflow complete"})
	if err != nil || completion["success"] != true {
		t.Fatalf("goalComplete success = %#v err=%v", completion, err)
	}
	if snap := decision.Snapshot(); snap.Status != "complete" || snap.Summary != "workflow complete" {
		t.Fatalf("goal decision after complete = status:%q summary:%q", snap.Status, snap.Summary)
	}
}
