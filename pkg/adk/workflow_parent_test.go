package adk

import (
	"context"
	"strings"
	"testing"
)

func TestContinueParentWorkflowAfterChildKeepsParentWaitingForPendingAndRunningChildren(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "continue-parent-waiting-agent", Name: "Continue Parent Waiting", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "continue parent waiting")
	now := nowString()

	t.Run("pending child keeps parent pending", func(t *testing.T) {
		approval := Approval{
			ID: "approval-continue-parent-pending", RunID: "run-continue-parent-pending-child", AgentID: agent.ID,
			ToolName: "approval.required", Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now,
		}
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-continue-parent-pending",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "等待审批",
			ChildRunIDs:    []string{"run-continue-parent-pending-child"},
			WorkflowPlan: []WorkflowStepState{{
				TaskID: "task-continue-parent-pending", Title: "审批子步骤", Status: "IN_PROGRESS", ChildRunID: "run-continue-parent-pending-child",
			}},
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})
		child := mustSaveRun(t, runtime, Run{
			ID:               "run-continue-parent-pending-child",
			SessionID:        session.ID,
			AgentID:          agent.ID,
			ParentRunID:      parent.ID,
			Status:           RunStatusPending,
			Message:          "等待用户审批后继续执行。",
			PendingApprovals: []Approval{approval},
			CreatedAt:        now,
			UpdatedAt:        now,
			Usage:            &RunUsage{},
		})
		if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}

		updated, err := runtime.continueParentWorkflowAfterChild(ctx, child)
		if err != nil {
			t.Fatalf("continueParentWorkflowAfterChild pending: %v", err)
		}
		if updated == nil || updated.Status != RunStatusPending || updated.WorkflowStatus != workflowStatusPaused {
			t.Fatalf("updated parent = %+v, want pending paused workflow", updated)
		}
		if len(updated.PendingApprovals) != 1 || updated.PendingApprovals[0].ID != approval.ID {
			t.Fatalf("updated approvals = %+v, want mirrored pending approval", updated.PendingApprovals)
		}
	})

	t.Run("running child keeps parent running", func(t *testing.T) {
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-continue-parent-running",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusPending,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusPaused,
			Objective:      "等待子运行结束",
			ChildRunIDs:    []string{"run-continue-parent-running-child"},
			WorkflowPlan: []WorkflowStepState{{
				TaskID: "task-continue-parent-running", Title: "运行子步骤", Status: "BLOCKED", ChildRunID: "run-continue-parent-running-child",
			}},
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})
		child := mustSaveRun(t, runtime, Run{
			ID:          "run-continue-parent-running-child",
			SessionID:   session.ID,
			AgentID:     agent.ID,
			ParentRunID: parent.ID,
			Status:      RunStatusRunning,
			Message:     "子运行仍在执行",
			CreatedAt:   now,
			UpdatedAt:   now,
			Usage:       &RunUsage{},
		})

		updated, err := runtime.continueParentWorkflowAfterChild(ctx, child)
		if err != nil {
			t.Fatalf("continueParentWorkflowAfterChild running: %v", err)
		}
		if updated == nil || updated.Status != RunStatusRunning || updated.WorkflowStatus != workflowStatusRunning || updated.Message != "子运行仍在执行" {
			t.Fatalf("updated parent = %+v, want running workflow mirroring child", updated)
		}
		if len(updated.PendingApprovals) != 0 {
			t.Fatalf("updated approvals = %+v, want no pending approvals mirrored from running child", updated.PendingApprovals)
		}
	})
}

func TestTaskWorkflowAsyncApprovalFailurePropagatesToParent(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-async-approval-failure-agent", Name: "Task Async Approval Failure", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @approval.required 保存策略",
		Objective:        "验证审批异步失败",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task async approval workflow: %v", err)
	}
	if response.Run.Status != RunStatusPending || len(response.PendingApprovals) != 1 {
		t.Fatalf("initial response = %+v, want pending parent with one child approval", response)
	}
	childRunID := response.PendingApprovals[0].RunID
	if childRunID == "" || childRunID == response.Run.ID {
		t.Fatalf("pending approval = %+v, want child run id", response.PendingApprovals[0])
	}
	if _, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "seq-async-approval-failure-agent", Name: "Task Async Approval Failure", ProviderID: testProviderID,
		Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval, Status: AgentStatusDisabled, WorkMode: WorkModeTask,
	}); err != nil {
		t.Fatalf("disable agent: %v", err)
	}
	restarted := newRuntimeWithRegistry(t, runtime.Store(), runtime.Tools())

	resolution, err := restarted.ResolveApprovalAsync(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApprovalAsync: %v", err)
	}
	if resolution.Run == nil || resolution.Run.ID != childRunID || resolution.Run.Status != RunStatusRunning || resolution.Run.ResumeState != "approval_resuming" {
		t.Fatalf("child resolution = %+v, want running approval_resuming child", resolution.Run)
	}
	if resolution.ParentRun == nil || resolution.ParentRun.ID != response.Run.ID || resolution.ParentRun.Status != RunStatusRunning || resolution.ParentRun.WorkflowStatus != workflowStatusRunning {
		t.Fatalf("parent resolution = %+v, want running resumed parent", resolution.ParentRun)
	}

	child := waitForRunStatus(t, restarted, childRunID, RunStatusFailed)
	if child.ResumeState != "approval_continuation_failed" || child.ErrorCode != "APPROVAL_CONTINUATION_FAILED" {
		t.Fatalf("child run = %+v, want approval continuation failure state", child)
	}
	if child.CompletedAt == nil || !strings.Contains(child.FailureReason, "agent is disabled") {
		t.Fatalf("child terminal state = %+v, want disabled-agent failure", child)
	}

	parent := waitForRunStatus(t, restarted, response.Run.ID, RunStatusFailed)
	if parent.WorkflowStatus != workflowStatusFailed || parent.ErrorCode != "APPROVAL_CONTINUATION_FAILED" {
		t.Fatalf("parent run = %+v, want failed workflow with propagated continuation error", parent)
	}
	if parent.CompletedAt == nil || parent.FailureReason != child.FailureReason || parent.Message != child.Message {
		t.Fatalf("parent failure projection = %+v, child = %+v", parent, child)
	}
	if len(parent.PendingApprovals) != 0 {
		t.Fatalf("parent pending approvals = %+v, want none after propagated failure", parent.PendingApprovals)
	}
	if executions.Load() != 0 {
		t.Fatalf("tool executions = %d, want 0 because resume failed before tool rerun", executions.Load())
	}

	events := mustAuditEvents(t, restarted)
	found := false
	for _, event := range events {
		if event.SubjectID == child.ID && event.Kind == "run.failed" && toString(event.Metadata["resumeState"]) == "approval_continuation_failed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("audit events = %+v, want child run.failed with approval_continuation_failed", events)
	}
}
