package adk

import (
	"context"
	"testing"
)

func TestTaskWorkflowApprovalContinuesParentWorkflow(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-approval-agent", Name: "Task Approval", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		Objective:        "完成审批续跑测试",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task approval workflow: %v", err)
	}
	if response.Run.Status != RunStatusPending || response.Run.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("parent run = %+v, want paused pending workflow", response.Run)
	}
	if len(response.PendingApprovals) != 1 || response.PendingApprovals[0].RunID == response.Run.ID {
		t.Fatalf("pending approvals = %+v, want child-run approval", response.PendingApprovals)
	}

	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	if resolution.Run == nil || resolution.Run.ParentRunID != response.Run.ID || resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("child resolution run = %+v, want completed child", resolution.Run)
	}
	if resolution.ParentRun == nil || resolution.ParentRun.ID != response.Run.ID || resolution.ParentRun.Status != RunStatusCompleted {
		t.Fatalf("parent resolution run = %+v, want completed parent workflow", resolution.ParentRun)
	}
	if len(resolution.ParentRun.ChildRunIDs) != 1 {
		t.Fatalf("child run ids = %+v, want approved child", resolution.ParentRun.ChildRunIDs)
	}
	if executions.Load() != 1 {
		t.Fatalf("tool executions = %d, want 1", executions.Load())
	}
}

func TestTaskWorkflowApprovalDeniedTerminatesParentWorkflow(t *testing.T) {
	ctx := context.Background()
	runtime, _ := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-deny-agent", Name: "Task Deny", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task denial workflow: %v", err)
	}
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, false)
	if err != nil {
		t.Fatalf("ResolveApproval deny: %v", err)
	}
	if resolution.ParentRun == nil || resolution.ParentRun.Status != RunStatusDenied || resolution.ParentRun.WorkflowStatus != workflowStatusFailed {
		t.Fatalf("parent resolution run = %+v, want denied failed workflow", resolution.ParentRun)
	}
}

func TestTaskResumeUsesStoredPendingChildBeforeCompletingParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-stale-child-agent", Name: "Task Stale Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "stale child")
	approval := Approval{
		ID: "approval-stale-child", RunID: "child-stale-pending", AgentID: agent.ID,
		ToolName: "strategy.save_draft", Status: ApprovalStatusPending,
		CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-stale-plan", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "等待子审批", ChildRunIDs: []string{"child-stale-pending"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "需要审批的步骤", Message: "保存策略", Status: "DONE", ChildRunID: "child-stale-pending",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-stale-pending", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusPending, Message: "等待用户审批后继续执行。", UserMessage: "保存策略",
		PendingApprovals: []Approval{approval},
		CreatedAt:        nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
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

func TestPendingChildCanReopenCompletedRunningParentWorkflow(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "parent-reopen-pending-child-agent", Name: "Parent Reopen Pending Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "reopen pending child")
	approval := Approval{
		ID: "approval-reopen-pending-child", RunID: "child-reopen-pending", AgentID: agent.ID,
		ToolName: "strategy.research_backtest", Status: ApprovalStatusPending,
		CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-completed-running-reopen", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusCompleted, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Message: "running", Objective: "等待子审批", ChildRunIDs: []string{"child-reopen-pending"},
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "task-reopen-pending-child", Title: "需要审批的步骤", Status: "DONE", ChildRunID: "child-reopen-pending",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "child-reopen-pending", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusPending, Message: "等待用户审批后继续执行。", UserMessage: "保存策略",
		PendingApprovals: []Approval{approval},
		CreatedAt:        nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	updated, err := runtime.syncParentWorkflowFromChild(ctx, child)
	if err != nil {
		t.Fatalf("syncParentWorkflowFromChild: %v", err)
	}
	if updated == nil || updated.Status != RunStatusPending || updated.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("updated parent = %+v, want pending paused parent", updated)
	}
	if len(updated.PendingApprovals) != 1 || updated.PendingApprovals[0].ID != approval.ID {
		t.Fatalf("updated pending approvals = %+v, want child approval", updated.PendingApprovals)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPending || stored.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("stored parent = %+v, want reopened pending workflow", stored)
	}
	if got := stored.WorkflowPlan[0].Status; got != "BLOCKED" {
		t.Fatalf("workflow step status = %q, want BLOCKED", got)
	}
}

func TestTaskResumeUsesStoredRunningChildBeforeCompletingParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-running-child-agent", Name: "Task Running Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "running child")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-running-plan", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "等待子运行", ChildRunIDs: []string{"child-still-running"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "仍在运行的步骤", Message: "继续运行", Status: "DONE", ChildRunID: "child-still-running",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-still-running", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusRunning, Message: "子运行仍在执行。", UserMessage: "继续运行",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
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
				WorkMode: WorkModeTask,
			})
			session := mustCreateSession(t, runtime, agent.ID, "terminal child "+tc.name)
			childID := "child-terminal-" + tc.name
			parent := mustSaveRun(t, runtime, Run{
				ID: "parent-terminal-plan-" + tc.name, SessionID: session.ID, AgentID: agent.ID,
				Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
				Objective: "处理终止子运行", ChildRunIDs: []string{childID},
				WorkflowPlan: []WorkflowStepState{{
					Title: "终止步骤", Message: "终止", Status: "DONE", ChildRunID: childID,
				}},
				CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
			})
			mustSaveRun(t, runtime, Run{
				ID: childID, SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
				Status: tc.status, Message: "child terminal", FailureReason: "child terminal failure",
				CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
			})

			updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
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

func TestWorkflowParentReconcilesResolvedChildApproval(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-reconcile-agent", Name: "Task Reconcile", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		Objective:        "完成审批恢复测试",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task approval workflow: %v", err)
	}
	if _, changed, err := runtime.Store().ResolvePendingApproval(ctx, response.PendingApprovals[0].ID, ApprovalStatusApproved); err != nil || !changed {
		t.Fatalf("ResolvePendingApproval changed=%v err=%v", changed, err)
	}
	runtime.ReconcileResolvedApprovals(ctx)
	parent := waitForRunStatus(t, runtime, response.Run.ID, RunStatusCompleted)
	if parent.WorkflowStatus != workflowStatusComplete {
		t.Fatalf("parent workflow status = %q, want %q", parent.WorkflowStatus, workflowStatusComplete)
	}
	if executions.Load() != 1 {
		t.Fatalf("tool executions = %d, want 1", executions.Load())
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
		CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})

	completed, err := (&WorkflowExecutor{runtime: runtime}).completeResumedWorkflow(ctx, session, parent, "done")
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
