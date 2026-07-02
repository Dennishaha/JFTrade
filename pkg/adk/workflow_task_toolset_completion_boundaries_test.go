package adk

import (
	"context"
	"testing"
)

func TestWorkflowTaskToolsetCompleteHonorsTaskAndChildRunBoundaries(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-complete-boundaries", SessionID: "session-complete-boundaries", AgentID: "agent-complete",
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-already-done", Title: "Already done"},
			{TaskID: "task-self-complete", Title: "Self complete"},
			{TaskID: "task-missing-child", Title: "Missing child"},
			{TaskID: "task-running-child", Title: "Running child"},
			{TaskID: "task-completed-child", Title: "Completed child"},
		},
		CreatedAt: now, UpdatedAt: now,
	})
	saveTask := func(req TaskWriteRequest) Task {
		t.Helper()
		task, err := runtime.Store().SaveTask(ctx, req)
		if err != nil {
			t.Fatalf("SaveTask(%s): %v", req.ID, err)
		}
		return task
	}
	saveTask(TaskWriteRequest{ID: "task-already-done", Title: "Already done", Status: "DONE", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode})
	selfTask := saveTask(TaskWriteRequest{
		ID: "task-self-complete", Title: "Review exposure", Description: "Check concentrated positions",
		Status: "IN_PROGRESS", AgentID: parent.AgentID, RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode,
	})
	saveTask(TaskWriteRequest{
		ID: "task-missing-child", Title: "Missing child", Status: "IN_PROGRESS", AgentID: parent.AgentID,
		RunID: "child-missing", Executor: workflowTaskExecutorChild, Order: 3, WorkflowMode: parent.WorkMode,
	})
	saveTask(TaskWriteRequest{
		ID: "task-running-child", Title: "Running child", Status: "IN_PROGRESS", AgentID: parent.AgentID,
		RunID: "child-running", Executor: workflowTaskExecutorChild, Order: 4, WorkflowMode: parent.WorkMode,
	})
	saveTask(TaskWriteRequest{
		ID: "task-completed-child", Title: "Completed child", Status: "IN_PROGRESS", AgentID: parent.AgentID,
		RunID: "child-completed", Executor: workflowTaskExecutorChild, Order: 5, WorkflowMode: parent.WorkMode,
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-running", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusRunning, Message: "child still working", CreatedAt: now, UpdatedAt: now,
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-completed", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusCompleted, Message: "child done", CreatedAt: now, UpdatedAt: now,
	})
	toolset := &workflowTaskToolset{
		executor:      &WorkflowExecutor{runtime: runtime},
		parentID:      parent.ID,
		currentTaskID: selfTask.ID,
		req:           workflowRequest{Mode: WorkModeTask},
	}

	selfResult, err := toolset.complete(nil)
	if err != nil {
		t.Fatalf("complete current self task: %v", err)
	}
	selfPayload := selfResult["task"].(map[string]any)
	if selfPayload["status"] != "DONE" || selfPayload["executor"] != workflowTaskExecutorSelf {
		t.Fatalf("self completion payload = %#v, want done self task", selfPayload)
	}
	if summary, _ := selfPayload["resultSummary"].(string); summary != "Review exposure 已由父智能体完成：Check concentrated positions" {
		t.Fatalf("self completion summary = %q", summary)
	}
	if toolset.currentTaskID != "" {
		t.Fatalf("current task id = %q, want cleared after completion", toolset.currentTaskID)
	}

	doneResult, err := toolset.complete(map[string]any{"taskId": "task-already-done"})
	if err != nil {
		t.Fatalf("complete already done: %v", err)
	}
	if doneResult["success"] != false || doneResult["status"] != "DONE" {
		t.Fatalf("already done result = %#v, want non-completable status", doneResult)
	}
	missingResult, err := toolset.complete(map[string]any{"taskId": "task-missing-child"})
	if err != nil {
		t.Fatalf("complete missing child: %v", err)
	}
	if missingResult["success"] != false || missingResult["message"] != "delegated child run is unavailable" {
		t.Fatalf("missing child result = %#v", missingResult)
	}
	runningResult, err := toolset.complete(map[string]any{"taskId": "task-running-child"})
	if err != nil {
		t.Fatalf("complete running child: %v", err)
	}
	if runningResult["success"] != false || runningResult["childStatus"] != RunStatusRunning {
		t.Fatalf("running child result = %#v", runningResult)
	}
	completedResult, err := toolset.complete(map[string]any{"taskId": "task-completed-child", "resultSummary": "Child result accepted"})
	if err != nil {
		t.Fatalf("complete finished child: %v", err)
	}
	completedPayload := completedResult["task"].(map[string]any)
	if completedPayload["status"] != "DONE" || completedPayload["resultSummary"] != "Child result accepted" {
		t.Fatalf("completed child payload = %#v, want accepted child result", completedPayload)
	}
}

func TestWorkflowGoalCompleteBlocksUnfinishedChildrenAndApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-goal-completion-blockers", SessionID: "session-goal-completion-blockers", AgentID: "agent-goal",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		ChildRunIDs: []string{"child-orphan-running", "child-orphan-completed", "child-orphan-missing"},
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-goal-todo", Title: "Open task"},
			{TaskID: "task-goal-child-running", Title: "Running delegated task"},
			{TaskID: "task-goal-child-completed-pending", Title: "Completed child with pending approval"},
			{TaskID: "task-goal-child-ok", Title: "Completed child"},
		},
		CreatedAt: now, UpdatedAt: now,
	})
	saveTask := func(req TaskWriteRequest) Task {
		t.Helper()
		task, err := runtime.Store().SaveTask(ctx, req)
		if err != nil {
			t.Fatalf("SaveTask(%s): %v", req.ID, err)
		}
		return task
	}
	saveTask(TaskWriteRequest{ID: "task-goal-todo", Title: "Open task", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode})
	saveTask(TaskWriteRequest{
		ID: "task-goal-child-running", Title: "Running delegated task", Status: "DONE", AgentID: parent.AgentID,
		RunID: "child-running-blocker", Executor: workflowTaskExecutorChild, Order: 2, WorkflowMode: parent.WorkMode,
	})
	saveTask(TaskWriteRequest{
		ID: "task-goal-child-completed-pending", Title: "Completed child with pending approval", Status: "DONE", AgentID: parent.AgentID,
		RunID: "child-completed-pending", Executor: workflowTaskExecutorChild, Order: 3, WorkflowMode: parent.WorkMode,
	})
	saveTask(TaskWriteRequest{
		ID: "task-goal-child-ok", Title: "Completed child", Status: "DONE", AgentID: parent.AgentID,
		RunID: "child-completed-ok", Executor: workflowTaskExecutorChild, Order: 4, WorkflowMode: parent.WorkMode,
	})
	for _, child := range []Run{
		{ID: "child-running-blocker", Status: RunStatusRunning, Message: "still working"},
		{ID: "child-completed-pending", Status: RunStatusCompleted, Message: "done but approval remains"},
		{ID: "child-completed-ok", Status: RunStatusCompleted, Message: "done"},
		{ID: "child-orphan-running", Status: RunStatusRunning, Message: "untracked child still running"},
		{ID: "child-orphan-completed", Status: RunStatusCompleted, Message: "untracked child done"},
	} {
		child.SessionID = parent.SessionID
		child.AgentID = parent.AgentID
		child.ParentRunID = parent.ID
		child.CreatedAt = now
		child.UpdatedAt = now
		mustSaveRun(t, runtime, child)
	}
	if err := runtime.Store().SaveApproval(ctx, Approval{
		ID: "approval-child-completed-pending", RunID: "child-completed-pending", AgentID: parent.AgentID,
		Status: ApprovalStatusPending, ToolName: "strategy.publish", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	toolset := &workflowTaskToolset{
		executor: &WorkflowExecutor{runtime: runtime},
		parentID: parent.ID,
		req:      workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}},
	}
	parent, tasks, err := toolset.parentAndTasks(ctx)
	if err != nil {
		t.Fatalf("parentAndTasks: %v", err)
	}
	blockers := toolset.workflowCompletionBlockers(ctx, parent, tasks)
	if !hasWorkflowBlocker(blockers, "task-goal-todo", "TODO") {
		t.Fatalf("blockers = %+v, want open task blocker", blockers)
	}
	if !hasWorkflowBlocker(blockers, "child-running-blocker", RunStatusRunning) {
		t.Fatalf("blockers = %+v, want running child task blocker", blockers)
	}
	if !hasWorkflowBlocker(blockers, "child-completed-pending", "STILL_ACTIVE") {
		t.Fatalf("blockers = %+v, want completed child with pending approval blocker", blockers)
	}
	if !hasWorkflowBlocker(blockers, "child-orphan-running", RunStatusRunning) || !hasWorkflowBlocker(blockers, "child-orphan-missing", "MISSING") {
		t.Fatalf("blockers = %+v, want orphan running and missing child blockers", blockers)
	}
	if hasWorkflowBlocker(blockers, "child-completed-ok", RunStatusCompleted) || hasWorkflowBlocker(blockers, "child-orphan-completed", RunStatusCompleted) {
		t.Fatalf("blockers = %+v, completed children without pending work should not block", blockers)
	}

	result, err := toolset.goalComplete(map[string]any{"summary": "Goal is done"})
	if err != nil {
		t.Fatalf("goalComplete: %v", err)
	}
	if result["success"] != false || result["status"] != "blocked" {
		t.Fatalf("goalComplete result = %#v, want blocked completion", result)
	}
	if snapshot := toolset.req.GoalDecision.snapshot(); snapshot.status != "" {
		t.Fatalf("goal decision snapshot status = %q, blocked completion must not mark complete", snapshot.status)
	}
}

func hasWorkflowBlocker(blockers []map[string]any, id string, status string) bool {
	for _, blocker := range blockers {
		if blocker["id"] == id && blocker["status"] == status {
			return true
		}
	}
	return false
}
