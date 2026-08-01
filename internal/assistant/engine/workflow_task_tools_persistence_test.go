package adk

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowTaskToolsReturnParentPlanPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	newFixture := func(t *testing.T, suffix string, taskStatus string) (*Runtime, Run, Task, *workflowTaskToolset) {
		t.Helper()
		runtime := newTestRuntime(t)
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "task-plan-persistence-parent-" + suffix, SessionID: "task-plan-persistence-session-" + suffix, AgentID: "agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-plan-persistence-task-" + suffix, Title: "task", Status: taskStatus, AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		})
		if err != nil {
			t.Fatalf("SaveTask %s: %v", suffix, err)
		}
		return runtime, parent, task, &workflowTaskToolset{
			executor: runtime.workflowExecutor(), parentID: parent.ID, currentTaskID: "current-before-" + suffix,
			req: workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}},
		}
	}

	for _, tc := range []struct {
		name           string
		taskStatus     string
		mutatesTask    bool
		expectedStatus string
		call           func(*workflowTaskToolset, Task) (map[string]any, error)
	}{
		{name: "list", taskStatus: "TODO", call: func(toolset *workflowTaskToolset, _ Task) (map[string]any, error) {
			return toolset.list(nil)
		}},
		{name: "add", taskStatus: "TODO", mutatesTask: true, expectedStatus: "TODO", call: func(toolset *workflowTaskToolset, _ Task) (map[string]any, error) {
			return toolset.add(map[string]any{"title": "runtime task"})
		}},
		{name: "claim", taskStatus: "TODO", mutatesTask: true, expectedStatus: "IN_PROGRESS", call: func(toolset *workflowTaskToolset, task Task) (map[string]any, error) {
			return toolset.claim(map[string]any{"taskId": task.ID})
		}},
		{name: "complete", taskStatus: "TODO", mutatesTask: true, expectedStatus: "DONE", call: func(toolset *workflowTaskToolset, task Task) (map[string]any, error) {
			return toolset.complete(map[string]any{"taskId": task.ID, "resultSummary": "done"})
		}},
		{name: "block", taskStatus: "TODO", mutatesTask: true, expectedStatus: "BLOCKED", call: func(toolset *workflowTaskToolset, task Task) (map[string]any, error) {
			return toolset.block(map[string]any{"taskId": task.ID, "reason": "blocked"})
		}},
		{name: "goal_complete", taskStatus: "DONE", call: func(toolset *workflowTaskToolset, _ Task) (map[string]any, error) {
			return toolset.goalComplete(map[string]any{"summary": "complete"})
		}},
		{name: "goal_continue", taskStatus: "TODO", call: func(toolset *workflowTaskToolset, _ Task) (map[string]any, error) {
			return toolset.goalContinue(map[string]any{"reason": "continue"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, parent, task, toolset := newFixture(t, tc.name, tc.taskStatus)
			triggerName := "reject_task_plan_persistence_" + tc.name
			installRunUpdateRejectTrigger(t, runtime, parent.ID, triggerName)

			result, err := tc.call(toolset, task)
			if tc.mutatesTask {
				if err != nil {
					t.Fatalf("%s error = %v, want committed partial result", tc.name, err)
				}
				assertTaskMutationProjectionFailure(t, runtime, result, tc.expectedStatus)
				if tc.name == "claim" && toolset.currentTaskID != task.ID {
					t.Fatalf("claim current task = %q, want %q", toolset.currentTaskID, task.ID)
				}
				if tc.name == "complete" && toolset.currentTaskID != "" {
					t.Fatalf("complete current task = %q, want empty", toolset.currentTaskID)
				}
				return
			}
			if err == nil || result != nil || !strings.Contains(err.Error(), triggerName) {
				t.Fatalf("%s result = %#v err=%v, want nil/%s", tc.name, result, err, triggerName)
			}
			if toolset.currentTaskID != "current-before-"+tc.name {
				t.Fatalf("%s current task = %q after failed parent save", tc.name, toolset.currentTaskID)
			}
			if snapshot := toolset.req.GoalDecision.snapshot(); snapshot.status != "" {
				t.Fatalf("%s goal decision = %+v after failed parent save", tc.name, snapshot)
			}
		})
	}

	t.Run("goal complete approval lookup", func(t *testing.T) {
		runtime, _, _, toolset := newFixture(t, "goal-complete-approval-read", "DONE")
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableApprovals); err != nil {
			t.Fatalf("drop approval table: %v", err)
		}

		result, err := toolset.goalComplete(map[string]any{"summary": "complete"})
		if err == nil || result != nil {
			t.Fatalf("goalComplete result = %#v err=%v, want nil/read error", result, err)
		}
		if snapshot := toolset.req.GoalDecision.snapshot(); snapshot.status != "" {
			t.Fatalf("goalComplete decision = %+v after failed approval read", snapshot)
		}
	})
}

func TestWorkflowTaskToolsReturnParentPlanRefreshFailures(t *testing.T) {
	ctx := context.Background()
	newFixture := func(t *testing.T, suffix string) (*Runtime, Run, Task, *workflowTaskToolset) {
		t.Helper()
		runtime := newTestRuntime(t)
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "task-plan-refresh-parent-" + suffix, SessionID: "task-plan-refresh-session-" + suffix, AgentID: "agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: now, UpdatedAt: now,
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-plan-refresh-task-" + suffix, Title: "task", Status: "TODO", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		})
		if err != nil {
			t.Fatalf("SaveTask %s: %v", suffix, err)
		}
		return runtime, parent, task, &workflowTaskToolset{
			executor: runtime.workflowExecutor(), parentID: parent.ID,
			req: workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}},
		}
	}

	for _, tc := range []struct {
		name           string
		triggerEvent   string
		expectedStatus string
		call           func(*workflowTaskToolset, Task) (map[string]any, error)
	}{
		{name: "add", triggerEvent: "INSERT", expectedStatus: "TODO", call: func(toolset *workflowTaskToolset, _ Task) (map[string]any, error) {
			return toolset.add(map[string]any{"title": "runtime task"})
		}},
		{name: "claim", triggerEvent: "UPDATE", expectedStatus: "IN_PROGRESS", call: func(toolset *workflowTaskToolset, task Task) (map[string]any, error) {
			return toolset.claim(map[string]any{"taskId": task.ID})
		}},
		{name: "complete", triggerEvent: "UPDATE", expectedStatus: "DONE", call: func(toolset *workflowTaskToolset, task Task) (map[string]any, error) {
			return toolset.complete(map[string]any{"taskId": task.ID, "resultSummary": "done"})
		}},
		{name: "block", triggerEvent: "UPDATE", expectedStatus: "BLOCKED", call: func(toolset *workflowTaskToolset, task Task) (map[string]any, error) {
			return toolset.block(map[string]any{"taskId": task.ID, "reason": "blocked"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, parent, task, toolset := newFixture(t, tc.name)
			if _, err := runtime.Store().db.ExecContext(ctx, `
				CREATE TRIGGER corrupt_parent_after_task_`+tc.name+`
				AFTER `+tc.triggerEvent+` ON `+tableTasks+`
				WHEN NEW.run_id = '`+parent.ID+`'
				BEGIN UPDATE `+tableRuns+` SET payload_json = '{' WHERE id = '`+parent.ID+`'; END
			`); err != nil {
				t.Fatalf("create %s refresh trigger: %v", tc.name, err)
			}

			result, err := tc.call(toolset, task)
			if err != nil {
				t.Fatalf("%s error = %v, want committed partial result", tc.name, err)
			}
			assertTaskMutationProjectionFailure(t, runtime, result, tc.expectedStatus)
		})
	}

	t.Run("goal continue", func(t *testing.T) {
		runtime, _, _, toolset := newFixture(t, "goal-continue")
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableTasks); err != nil {
			t.Fatalf("drop task table: %v", err)
		}

		result, err := toolset.goalContinue(map[string]any{"reason": "continue"})
		if err == nil || result != nil {
			t.Fatalf("goalContinue result = %#v err=%v, want nil/read error", result, err)
		}
		if snapshot := toolset.req.GoalDecision.snapshot(); snapshot.status != "" {
			t.Fatalf("goalContinue decision = %+v after failed parent read", snapshot)
		}
	})
}

func assertTaskMutationProjectionFailure(t *testing.T, runtime *Runtime, result map[string]any, expectedStatus string) {
	t.Helper()
	if result["success"] != false || result["committed"] != true || result["partial"] != true ||
		result["retryable"] != false || result["parentPlanSynced"] != false {
		t.Fatalf("projection failure result = %#v", result)
	}
	summary, ok := result["task"].(map[string]any)
	if !ok {
		t.Fatalf("projection failure task = %#v, want summary", result["task"])
	}
	taskID, ok := summary["id"].(string)
	if !ok || taskID == "" {
		t.Fatalf("projection failure task id = %#v", summary["id"])
	}
	task, found, err := runtime.Store().Task(t.Context(), taskID)
	if err != nil || !found || task.Status != expectedStatus {
		t.Fatalf("committed task = %#v/%v err=%v, want status %s", task, found, err, expectedStatus)
	}
}
