package adk

import (
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
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{first, second}, nil)
	mustSaveRun(t, runtime, parent)

	decision := &workflowGoalDecision{}
	toolset := &workflowTaskToolset{
		executor: runtime.workflowExecutor(),
		req:      workflowRequest{Agent: agent, Session: session, GoalDecision: decision},
		parentID: parent.ID,
	}

	listed, err := toolset.list(nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if ready, ok := listed["readyTasks"].([]map[string]any); !ok || len(ready) != 1 || ready[0]["id"] != first.ID {
		t.Fatalf("ready tasks = %#v", listed["readyTasks"])
	}
	if _, err := toolset.add(map[string]any{"title": "Bad dependency", "dependsOn": []any{"missing-task"}}); err == nil || !strings.Contains(err.Error(), "dependency not found") {
		t.Fatalf("add missing dependency err = %v", err)
	}

	claim, err := toolset.claim(map[string]any{"taskId": first.ID, "executor": workflowTaskExecutorSelf})
	if err != nil || claim["success"] != true {
		t.Fatalf("claim = %#v err=%v", claim, err)
	}
	complete, err := toolset.complete(map[string]any{"taskId": first.ID})
	if err != nil || complete["success"] != true {
		t.Fatalf("complete = %#v err=%v", complete, err)
	}
	doneFirst, ok, err := runtime.Store().Task(ctx, first.ID)
	if err != nil || !ok || doneFirst.Status != "DONE" || !strings.Contains(doneFirst.ResultSummary, "Inspect state") {
		t.Fatalf("completed first task = %#v ok=%v err=%v", doneFirst, ok, err)
	}

	added, err := toolset.add(map[string]any{
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

	blocked, err := toolset.block(map[string]any{"taskId": second.ID})
	if err != nil || blocked["success"] != true {
		t.Fatalf("block = %#v err=%v", blocked, err)
	}
	completion, err := toolset.goalComplete(map[string]any{"summary": "done"})
	if err != nil {
		t.Fatalf("goalComplete with blockers: %v", err)
	}
	if completion["success"] != false || completion["status"] != "blocked" {
		t.Fatalf("goalComplete blockers = %#v", completion)
	}
	if cont, err := toolset.goalContinue(map[string]any{}); err != nil || cont["status"] != "continue" {
		t.Fatalf("goalContinue = %#v err=%v", cont, err)
	}
	if snap := decision.snapshot(); snap.status != "continue" || snap.reason == "" {
		t.Fatalf("goal decision after continue = status:%q reason:%q", snap.status, snap.reason)
	}

	done := "DONE"
	if _, err := runtime.Store().UpdateTask(ctx, second.ID, TaskPatchRequest{Status: &done}); err != nil {
		t.Fatalf("UpdateTask second done: %v", err)
	}
	runtimeTaskID := runtimeTask["id"].(string)
	if _, err := runtime.Store().UpdateTask(ctx, runtimeTaskID, TaskPatchRequest{Status: &done}); err != nil {
		t.Fatalf("UpdateTask runtime done: %v", err)
	}
	completion, err = toolset.goalComplete(map[string]any{"summary": "workflow complete"})
	if err != nil || completion["success"] != true {
		t.Fatalf("goalComplete success = %#v err=%v", completion, err)
	}
	if snap := decision.snapshot(); snap.status != "complete" || snap.summary != "workflow complete" {
		t.Fatalf("goal decision after complete = status:%q summary:%q", snap.status, snap.summary)
	}
}

func TestWorkflowPlanningHelpersPreserveBusinessOrdering(t *testing.T) {
	steps := applyWorkflowStepPlanningMetadata([]workflowStep{
		{Title: "Ship feature", Description: "Ship feature", Message: "Ship feature"},
		{Order: 9, Title: "Verify", PlanSource: workflowPlanSourceRuntime, WorkflowMode: WorkModeLoop},
	}, WorkModeLoop, "Ship feature", []string{"planner used fallback", ""})
	if steps[0].Order != 1 || steps[0].Title == "Ship feature" || steps[0].Message == "Ship feature" {
		t.Fatalf("sanitized first step = %#v", steps[0])
	}
	if steps[1].Order != 9 || steps[1].PlanSource != workflowPlanSourceRuntime || steps[1].WorkflowMode != WorkModeLoop {
		t.Fatalf("metadata-preserving second step = %#v", steps[1])
	}
	if len(steps[0].PlannerWarnings) != 1 || steps[0].PlannerWarnings[0] != "planner used fallback" {
		t.Fatalf("planner warnings = %#v", steps[0].PlannerWarnings)
	}

	tasks := []Task{
		{ID: "done", Title: "Done", Status: "DONE", Order: 1},
		{ID: "ready-a", Title: "Ready A", Status: "TODO", DependsOn: []string{"done"}, Order: 3},
		{ID: "ready-b", Title: "Ready B", Status: "TODO", DependsOn: []string{"done"}, Order: 2},
		{ID: "blocked", Title: "Blocked", Status: "TODO", DependsOn: []string{"missing"}, Order: 4},
	}
	ready := executableWorkflowTasks(tasks, WorkModeLoop)
	if len(ready) != 1 || ready[0].ID != "ready-b" {
		t.Fatalf("ready workflow tasks = %#v", ready)
	}
	if workflowTasksComplete(nil) || !workflowTasksComplete([]Task{{Status: "DONE"}, {Status: "DONE"}}) {
		t.Fatal("workflowTasksComplete business status handling changed")
	}
	if task, ok := firstTerminalWorkflowTask([]Task{{Status: "TODO"}, {ID: "blocked", Status: "BLOCKED"}}); !ok || task.ID != "blocked" {
		t.Fatalf("firstTerminalWorkflowTask = %#v/%v", task, ok)
	}
	if !workflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}}) {
		t.Fatal("workflowTasksHaveCycle missed direct cycle")
	}
	if workflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"missing"}}}) {
		t.Fatal("workflowTasksHaveCycle treated missing dependency as a cycle")
	}

	state := workflowStepFromTask(Task{
		ID: "task", Title: "Task", Description: "description\n\nAgent role: reviewer", Status: "TODO",
		Message: "", DependsOn: []string{"done"}, AgentRole: "reviewer", PlannerWarnings: []string{"warn"},
	})
	if state.Message != "description\n\nAgent role: reviewer" || state.Description != "description" || len(state.PlannerWarnings) != 1 {
		t.Fatalf("workflowStepFromTask = %#v", state)
	}
	if workflowTaskIteration(Task{}) != 1 || workflowTaskIteration(Task{Order: 7}) != 7 {
		t.Fatal("workflowTaskIteration no longer falls back to first iteration")
	}
	if !strings.Contains(workflowSelfTaskSummary(Task{Title: "Review", Description: strings.Repeat("x", 140)}), "...") {
		t.Fatal("workflowSelfTaskSummary did not trim long task context")
	}
}
