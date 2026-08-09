package adk

import (
	"context"
	"testing"
)

func TestTaskWorkflowApprovalContinuesParentWorkflow(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeLoop)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-approval-agent", Name: "Task Approval", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		Objective:        "完成审批续跑测试",
		WorkModeOverride: WorkModeLoop,
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
	runtime, _ := newWorkflowApprovalRuntime(t, WorkModeLoop)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-deny-agent", Name: "Task Deny", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		WorkModeOverride: WorkModeLoop,
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
		return
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

func TestWorkflowParentReconcilesResolvedChildApproval(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeLoop)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-reconcile-agent", Name: "Task Reconcile", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		Objective:        "完成审批恢复测试",
		WorkModeOverride: WorkModeLoop,
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
