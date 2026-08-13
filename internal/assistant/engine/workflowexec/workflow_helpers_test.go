package workflowexec

import (
	"context"
	"errors"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"
)

func TestWorkflowHelperBranches(t *testing.T) {
	t.Run("plan and description helpers preserve fallback fields", func(t *testing.T) {
		existing := []WorkflowStepState{{
			TaskID: "task-1", Title: "Prior Title", Description: "Prior Desc", Message: "Prior Message",
			PlanSource: "prior", WorkflowMode: WorkModeLoop, Objective: "Prior Objective", ChildRunID: "child-1",
		}}
		plan := assistantmodel.WorkflowPlanFromTasks([]Task{{ID: "task-1", Status: "TODO"}}, existing)
		if len(plan) != 1 || plan[0].Title != "Prior Title" || plan[0].Description != "Prior Desc" || plan[0].Message != "Prior Message" || plan[0].ChildRunID != "child-1" {
			t.Fatalf("workflowPlanFromTasks fallback = %+v", plan)
		}

		if got := assistantmodel.WorkflowStepDescription(workflowStep{AgentRole: "Research Agent"}); got != "Agent role: Research Agent" {
			t.Fatalf("workflowStepDescription role-only = %q", got)
		}
		if got := assistantmodel.WorkflowDescriptionWithoutAgentRole("Agent role: Research Agent"); got != "" {
			t.Fatalf("workflowDescriptionWithoutAgentRole prefix-only = %q, want empty", got)
		}
		if got := assistantmodel.WorkflowDescriptionWithoutAgentRole("Desc\n\nAgent role: Research Agent"); got != "Desc" {
			t.Fatalf("workflowDescriptionWithoutAgentRole suffix strip = %q", got)
		}
	})

	t.Run("runtime task helpers cover validation cycle and emission errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		parent := mustSaveRun(t, runtime, Run{
			ID:             "workflow-helper-runtime-parent",
			SessionID:      "session-runtime-parent",
			AgentID:        "agent",
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "推进目标",
			CreatedAt:      assistantmodel.NowString(),
			UpdatedAt:      assistantmodel.NowString(),
		})
		seed, err := runtime.Store().SaveTask(context.Background(), TaskWriteRequest{
			ID:           "seed-task",
			Title:        "Seed Task",
			Status:       "DONE",
			AgentID:      parent.AgentID,
			RunID:        parent.ID,
			Order:        1,
			PlanSource:   workflowPlanSourceRuntime,
			WorkflowMode: parent.WorkMode,
			Objective:    parent.Objective,
		})
		if err != nil {
			t.Fatalf("SaveTask seed: %v", err)
		}
		if _, err := executor.AddRuntimeWorkflowTask(context.Background(), parent, Task{}, workflowRuntimeTaskRequest{}); err == nil || !strings.Contains(err.Error(), "title is required") {
			t.Fatalf("AddRuntimeWorkflowTask blank err = %v", err)
		}
		if _, err := executor.AddRuntimeWorkflowTask(context.Background(), parent, Task{}, workflowRuntimeTaskRequest{Title: "Depends", DependsOn: []string{"missing"}}); err == nil || !strings.Contains(err.Error(), "dependency not found") {
			t.Fatalf("AddRuntimeWorkflowTask missing dependency err = %v", err)
		}
		added, err := executor.AddRuntimeWorkflowTask(context.Background(), parent, Task{}, workflowRuntimeTaskRequest{
			Title:       "Follow-up",
			Description: "Derived from summary",
			DependsOn:   []string{seed.ID},
		})
		if err != nil {
			t.Fatalf("AddRuntimeWorkflowTask success: %v", err)
		}
		if added.Message != "Derived from summary" {
			t.Fatalf("AddRuntimeWorkflowTask message fallback = %+v", added)
		}

		if !assistantmodel.WorkflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}}) {
			t.Fatal("workflowTasksHaveCycle missed simple cycle")
		}
		if assistantmodel.WorkflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"missing"}}}) {
			t.Fatal("workflowTasksHaveCycle treated missing dependency as cycle")
		}

		run := Run{ID: "run-emit"}
		if err := emitWorkflowRunSnapshot(context.Background(), nil, workflowRequest{EmitRun: true, OnDelta: func(ChatDelta) error { return context.Canceled }}, run); !errors.Is(err, context.Canceled) {
			t.Fatalf("emitWorkflowRunSnapshot err = %v, want context.Canceled", err)
		}
	})

	t.Run("child state and approval filters cover no-op branches", func(t *testing.T) {
		parent := Run{
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-1", ChildRunID: "child-1", Status: "TODO"}},
			ChildRunIDs:  []string{"child-1"},
		}
		noChange := assistantmodel.UpdateWorkflowPlanForChildAt(parent, Run{}, 0)
		if noChange.WorkflowPlan[0].ChildRunID != "child-1" {
			t.Fatalf("updateWorkflowPlanForChildAt empty child mutated plan = %+v", noChange.WorkflowPlan)
		}

		paused := assistantmodel.PauseParentForChild(Run{WorkflowPlan: []WorkflowStepState{{TaskID: "task-1"}}}, Run{
			ID:               "child-2",
			Status:           RunStatusPending,
			Message:          "waiting approval",
			PendingApprovals: []Approval{{ID: "approval-1", Status: ApprovalStatusPending}, {ID: "approval-2", Status: ApprovalStatusApproved}},
		}, 0)
		if paused.Status != RunStatusPending || len(paused.PendingApprovals) != 1 || paused.PendingApprovals[0].ID != "approval-1" {
			t.Fatalf("pauseParentForChild = %+v", paused)
		}

		filtered := assistantmodel.ApprovalsForRun([]Approval{
			{ID: "pending", RunID: "run", Status: ApprovalStatusPending},
			{ID: "done", RunID: "run", Status: ApprovalStatusApproved},
			{ID: "other", RunID: "other", Status: ApprovalStatusPending},
		}, "run")
		if len(filtered) != 1 || filtered[0].ID != "pending" {
			t.Fatalf("approvalsForRun pending-only = %+v", filtered)
		}
	})
}
