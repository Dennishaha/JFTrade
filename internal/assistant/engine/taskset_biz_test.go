package adk

import (
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"
)

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
	ready := assistantmodel.ExecutableWorkflowTasks(tasks, WorkModeLoop)
	if len(ready) != 1 || ready[0].ID != "ready-b" {
		t.Fatalf("ready workflow tasks = %#v", ready)
	}
	if assistantmodel.WorkflowTasksComplete(nil) || !assistantmodel.WorkflowTasksComplete([]Task{{Status: "DONE"}, {Status: "DONE"}}) {
		t.Fatal("workflowTasksComplete business status handling changed")
	}
	if task, ok := assistantmodel.FirstTerminalWorkflowTask([]Task{{Status: "TODO"}, {ID: "blocked", Status: "BLOCKED"}}); !ok || task.ID != "blocked" {
		t.Fatalf("firstTerminalWorkflowTask = %#v/%v", task, ok)
	}
	if !assistantmodel.WorkflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}}) {
		t.Fatal("workflowTasksHaveCycle missed direct cycle")
	}
	if assistantmodel.WorkflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"missing"}}}) {
		t.Fatal("workflowTasksHaveCycle treated missing dependency as a cycle")
	}

	state := assistantmodel.WorkflowStepFromTask(Task{
		ID: "task", Title: "Task", Description: "description\n\nAgent role: reviewer", Status: "TODO",
		Message: "", DependsOn: []string{"done"}, AgentRole: "reviewer", PlannerWarnings: []string{"warn"},
	})
	if state.Message != "description\n\nAgent role: reviewer" || state.Description != "description" || len(state.PlannerWarnings) != 1 {
		t.Fatalf("workflowStepFromTask = %#v", state)
	}
	if assistantmodel.WorkflowTaskIteration(Task{}) != 1 || assistantmodel.WorkflowTaskIteration(Task{Order: 7}) != 7 {
		t.Fatal("workflowTaskIteration no longer falls back to first iteration")
	}
	if !strings.Contains(assistantmodel.WorkflowSelfTaskSummary(Task{Title: "Review", Description: strings.Repeat("x", 140)}), "...") {
		t.Fatal("workflowSelfTaskSummary did not trim long task context")
	}
}
