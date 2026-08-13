package workflowexec

import (
	"context"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"testing"
)

func TestTaskResumeUsesStoredPendingChildBeforeCompletingParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-stale-child-agent", Name: "Task Stale Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "stale child")
	approval := Approval{
		ID: "approval-stale-child", RunID: "child-stale-pending", AgentID: agent.ID,
		ToolName: "strategy.save_draft", Status: ApprovalStatusPending,
		CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(),
	}
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-stale-plan", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Objective: "等待子审批", ChildRunIDs: []string{"child-stale-pending"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "需要审批的步骤", Message: "保存策略", Status: "DONE", ChildRunID: "child-stale-pending",
		}},
		CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-stale-pending", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusPending, Message: "等待用户审批后继续执行。", UserMessage: "保存策略",
		PendingApprovals: []Approval{approval},
		CreatedAt:        assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(ctx, parent)
	if err != nil {
		t.Fatalf("reconcileWorkflowChildren: %v", err)
	}
	if !blocked {
		t.Fatal("reconcileWorkflowChildren blocked = false, want true")
	}
	if updated.Status != RunStatusPending || updated.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("parent run = %+v, want paused pending workflow", updated)
	}
	if len(updated.PendingApprovals) != 1 || updated.PendingApprovals[0].ID != approval.ID {
		t.Fatalf("parent pending approvals = %+v, want child approval", updated.PendingApprovals)
	}
	if got := updated.WorkflowPlan[0].Status; got != "BLOCKED" {
		t.Fatalf("workflow step status = %q, want BLOCKED", got)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("parent completed at = %v, want nil while child waits approval", *updated.CompletedAt)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPending || stored.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("stored parent = %+v, want paused pending workflow", stored)
	}
}

func TestTaskResumeUsesStoredRunningChildBeforeCompletingParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-running-child-agent", Name: "Task Running Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "running child")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-running-plan", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Objective: "等待子运行", ChildRunIDs: []string{"child-still-running"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "仍在运行的步骤", Message: "继续运行", Status: "DONE", ChildRunID: "child-still-running",
		}},
		CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-still-running", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusRunning, Message: "子运行仍在执行。", UserMessage: "继续运行",
		CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(ctx, parent)
	if err != nil {
		t.Fatalf("reconcileWorkflowChildren: %v", err)
	}
	if !blocked {
		t.Fatal("reconcileWorkflowChildren blocked = false, want true")
	}
	if updated.Status != RunStatusRunning || updated.WorkflowStatus != workflowStatusRunning {
		t.Fatalf("parent run = %+v, want running workflow", updated)
	}
	if got := updated.WorkflowPlan[0].Status; got != "IN_PROGRESS" {
		t.Fatalf("workflow step status = %q, want IN_PROGRESS", got)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("parent completed at = %v, want nil while child is running", *updated.CompletedAt)
	}
}

func TestTaskResumeTerminatesParentForStoredTerminalChild(t *testing.T) {
	cases := []struct {
		name   string
		status string
	}{
		{name: "failed", status: RunStatusFailed},
		{name: "denied", status: RunStatusDenied},
		{name: "cancelled", status: RunStatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			runtime := newTestRuntime(t)
			agent := mustSaveAgent(t, runtime, AgentWriteRequest{
				ID: "seq-terminal-child-agent-" + tc.name, Name: "Task Terminal Child", Status: AgentStatusEnabled,
				WorkMode: WorkModeLoop,
			})
			session := mustCreateSession(t, runtime, agent.ID, "terminal child "+tc.name)
			childID := "child-terminal-" + tc.name
			parent := mustSaveRun(t, runtime, Run{
				ID: "parent-terminal-plan-" + tc.name, SessionID: session.ID, AgentID: agent.ID,
				Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
				Objective: "处理终止子运行", ChildRunIDs: []string{childID},
				WorkflowPlan: []WorkflowStepState{{
					Title: "终止步骤", Message: "终止", Status: "DONE", ChildRunID: childID,
				}},
				CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
			})
			mustSaveRun(t, runtime, Run{
				ID: childID, SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
				Status: tc.status, Message: "child terminal", FailureReason: "child terminal failure",
				CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
			})

			updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(ctx, parent)
			if err != nil {
				t.Fatalf("reconcileWorkflowChildren: %v", err)
			}
			if !blocked {
				t.Fatal("reconcileWorkflowChildren blocked = false, want true")
			}
			if updated.Status != tc.status || updated.WorkflowStatus != workflowStatusFailed {
				t.Fatalf("parent run = %+v, want status %q failed workflow", updated, tc.status)
			}
			if updated.CompletedAt == nil {
				t.Fatal("parent completed at is nil, want terminal timestamp")
			}
			if got := updated.WorkflowPlan[0].Status; got != "BLOCKED" {
				t.Fatalf("workflow step status = %q, want BLOCKED", got)
			}
		})
	}
}

func TestCompleteResumedWorkflowClearsTerminalPendingApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "complete-resumed-clear-agent", Name: "Complete Resumed Clear", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "complete resumed clear")
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-complete-resumed-clear", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Objective: "完成恢复工作流", PendingApprovals: []Approval{
			{ID: "approval-stale-pending-on-parent", RunID: "run-complete-resumed-clear", AgentID: agent.ID, Status: ApprovalStatusPending},
			{ID: "approval-resolved-on-parent", RunID: "run-complete-resumed-clear", AgentID: agent.ID, Status: ApprovalStatusApproved},
		},
		WorkflowPlan: []WorkflowStepState{{TaskID: "task-complete-resumed-clear", Title: "完成", Status: "DONE"}},
		CreatedAt:    assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
	})

	completed, err := (&WorkflowExecutor{runtime: runtime}).CompleteResumedWorkflow(ctx, session, parent, "done")
	if err != nil {
		t.Fatalf("completeResumedWorkflow: %v", err)
	}
	if completed.Status != RunStatusCompleted || len(completed.PendingApprovals) != 0 {
		t.Fatalf("completed parent = %+v, want terminal parent without pending approvals", completed)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusCompleted || len(stored.PendingApprovals) != 0 {
		t.Fatalf("stored completed parent = %+v, want no pending approvals", stored)
	}
}
