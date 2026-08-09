package workflowexec

import (
	"path/filepath"
	"strings"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowTaskToolsetLookupBoundaryBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	now := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-helper-parent", SessionID: "workflow-helper-session", AgentID: "workflow-helper-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-current", ChildRunID: ""},
			{TaskID: "task-missing-child", ChildRunID: "missing-child"},
			{TaskID: "task-foreign-child", ChildRunID: "foreign-child"},
			{TaskID: "task-pending-child", ChildRunID: "pending-child"},
		},
		ChildRunIDs: []string{"", "workflow-helper-parent"},
		CreatedAt:   now, UpdatedAt: now,
	})
	current, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-current", Title: "Current", Status: "IN_PROGRESS", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask current: %v", err)
	}
	ready, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-ready", Title: "Ready", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask ready: %v", err)
	}
	mustSaveRun(t, runtime, Run{
		ID: "foreign-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: "other-parent",
		Status: RunStatusRunning, CreatedAt: now, UpdatedAt: now,
	})
	mustSaveRun(t, runtime, Run{
		ID: "pending-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusPending, CreatedAt: now, UpdatedAt: now,
	})
	toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, parent.ID, current.ID)
	toolset.Req = workflowRequest{Mode: WorkModeLoop}
	if _, _, err := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, "missing-parent", "").ParentAndTasks(ctx); err == nil || !strings.Contains(err.Error(), "parent run not found") {
		t.Fatalf("missing parentAndTasks err = %v", err)
	}
	if task, ok, err := toolset.TaskByID(ctx, " "); err != nil || ok || task.ID != "" {
		t.Fatalf("blank taskByID = %+v/%v err=%v, want missing", task, ok, err)
	}
	if task, err := toolset.ResolveTask(ctx, parent, []Task{ready}, "missing-task", true); err == nil || task.ID != "" || !strings.Contains(err.Error(), "task not found") {
		t.Fatalf("explicit missing resolveTask = %+v/%v", task, err)
	}
	if task, err := toolset.ResolveTask(ctx, parent, []Task{ready}, "", false); err != nil || task.ID != current.ID {
		t.Fatalf("current resolveTask = %+v/%v, want current", task, err)
	}
	toolset.CurrentTaskID = ""
	if task, err := toolset.ResolveTask(ctx, parent, []Task{{ID: "in-progress", Status: "IN_PROGRESS"}}, "", false); err != nil || task.ID != "in-progress" {
		t.Fatalf("in-progress resolveTask = %+v/%v", task, err)
	}
	if task, err := toolset.ResolveTask(ctx, parent, []Task{ready}, "", true); err != nil || task.ID != ready.ID {
		t.Fatalf("ready resolveTask = %+v/%v", task, err)
	}
	if _, err := toolset.ResolveTask(ctx, parent, []Task{{ID: "blocked-ready", Status: "TODO", DependsOn: []string{"missing"}}}, "", true); err == nil || !strings.Contains(err.Error(), "no executable workflow task") {
		t.Fatalf("no executable resolveTask err = %v", err)
	}

	child, index, ok := (&WorkflowExecutor{runtime: runtime}).FirstBlockingTaskChild(ctx, parent)
	if !ok || child.ID != "pending-child" || index != 3 {
		t.Fatalf("FirstBlockingTaskChild = %+v index=%d ok=%v, want pending child at index 3", child, index, ok)
	}
	cleanParent := parent
	cleanParent.WorkflowPlan = []WorkflowStepState{{TaskID: "blank-child"}, {TaskID: "done-child", ChildRunID: "done-child"}}
	mustSaveRun(t, runtime, Run{
		ID: "done-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusCompleted, CreatedAt: now, UpdatedAt: now,
	})
	if child, index, ok := (&WorkflowExecutor{runtime: runtime}).FirstBlockingTaskChild(ctx, cleanParent); ok || child.ID != "" || index != -1 {
		t.Fatalf("clean FirstBlockingTaskChild = %+v index=%d ok=%v, want none", child, index, ok)
	}

	blockers, err := toolset.WorkflowCompletionBlockers(ctx, parent, []Task{{ID: "done", Status: "DONE"}})
	if err != nil {
		t.Fatalf("workflowCompletionBlockers: %v", err)
	}
	if len(blockers) != 0 {
		t.Fatalf("blank/self child IDs should not block completion: %+v", blockers)
	}

	dir := t.TempDir()
	closedStore, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore closed workflow task lookup: %v", err)
	}
	closedRuntime := NewRuntime(closedStore, NewToolRegistry())
	closedToolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: closedRuntime}, parent.ID, "")
	if err := closedStore.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}
	if _, _, err := closedToolset.ParentAndTasks(ctx); err == nil {
		t.Fatal("closed parentAndTasks err = nil, want error")
	}
	if task, ok, err := closedToolset.TaskByID(ctx, current.ID); err == nil || ok || task.ID != "" {
		t.Fatalf("closed taskByID = %+v/%v err=%v, want read error", task, ok, err)
	}
	if task, err := closedToolset.ResolveTask(ctx, parent, nil, current.ID, true); err == nil || task.ID != "" {
		t.Fatalf("closed resolveTask = %+v/%v, want read error", task, err)
	}
	if err := closedToolset.SaveParentPlan(ctx, parent, nil); err == nil {
		t.Fatal("closed saveParentPlan err = nil, want error")
	}
	for _, tc := range []struct {
		name string
		call func() (map[string]any, error)
	}{
		{name: "list", call: func() (map[string]any, error) { return closedToolset.List(nil) }},
		{name: "add", call: func() (map[string]any, error) { return closedToolset.Add(map[string]any{"title": "x"}) }},
		{name: "claim", call: func() (map[string]any, error) { return closedToolset.Claim(map[string]any{"taskId": current.ID}) }},
		{name: "complete", call: func() (map[string]any, error) { return closedToolset.Complete(map[string]any{"taskId": current.ID}) }},
		{name: "block", call: func() (map[string]any, error) { return closedToolset.Block(map[string]any{"taskId": current.ID}) }},
		{name: "delegate", call: func() (map[string]any, error) { return closedToolset.Delegate(map[string]any{"taskId": current.ID}) }},
		{name: "goalComplete", call: func() (map[string]any, error) { return closedToolset.GoalComplete(map[string]any{"summary": "done"}) }},
	} {
		t.Run("closed "+tc.name, func(t *testing.T) {
			if result, err := tc.call(); err == nil || result != nil {
				t.Fatalf("%s closed result = %#v err=%v, want nil/error", tc.name, result, err)
			}
		})
	}
}

func TestWorkflowTaskToolsetMethodErrorAndFallbackBranches(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	now := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-method-branches-parent", SessionID: "workflow-method-branches-session", AgentID: "workflow-method-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	})
	done, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-method-done", Title: "Done task", Status: "DONE", AgentID: parent.AgentID, RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask done: %v", err)
	}
	ready, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-method-ready", Title: "Ready task", Status: "TODO", AgentID: parent.AgentID, RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode,
	})
	if err != nil {
		t.Fatalf("SaveTask ready: %v", err)
	}
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks([]Task{done, ready}, nil)
	mustSaveRun(t, runtime, parent)
	toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, parent.ID, "")
	toolset.Req = workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}}
	for _, tc := range []struct {
		name string
		call func() (map[string]any, error)
	}{
		{name: "claim missing", call: func() (map[string]any, error) { return toolset.Claim(map[string]any{"taskId": "missing-task"}) }},
		{name: "complete missing", call: func() (map[string]any, error) { return toolset.Complete(map[string]any{"taskId": "missing-task"}) }},
		{name: "block missing", call: func() (map[string]any, error) { return toolset.Block(map[string]any{"taskId": "missing-task"}) }},
		{name: "delegate missing", call: func() (map[string]any, error) { return toolset.Delegate(map[string]any{"taskId": "missing-task"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if result, err := tc.call(); err == nil || result != nil || !strings.Contains(err.Error(), "task not found") {
				t.Fatalf("%s result = %#v err=%v, want task not found", tc.name, result, err)
			}
		})
	}
	complete, err := toolset.GoalComplete(map[string]any{"resultSummary": "done via result summary"})
	if err != nil {
		t.Fatalf("goalComplete resultSummary: %v", err)
	}
	if complete["success"] != false || complete["status"] != "blocked" {
		t.Fatalf("goalComplete with open task = %#v, want blocked", complete)
	}
	doneStatus := "DONE"
	if _, err := runtime.Store().UpdateTask(ctx, ready.ID, TaskPatchRequest{Status: &doneStatus}); err != nil {
		t.Fatalf("UpdateTask ready done: %v", err)
	}
	complete, err = toolset.GoalComplete(map[string]any{"resultSummary": "done via result summary"})
	if err != nil {
		t.Fatalf("goalComplete success resultSummary: %v", err)
	}
	if complete["success"] != true || complete["summary"] != "done via result summary" {
		t.Fatalf("goalComplete success = %#v, want resultSummary fallback", complete)
	}
	if snap := toolset.Req.GoalDecision.Snapshot(); snap.Status != "complete" || snap.Summary != "done via result summary" {
		t.Fatalf("goal decision = status:%q summary:%q", snap.Status, snap.Summary)
	}
}
