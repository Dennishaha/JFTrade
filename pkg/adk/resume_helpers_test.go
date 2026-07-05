package adk

import (
	"context"
	"testing"
)

func TestWorkflowResumeContextValidatesResourcesAndAppliesParentOverrides(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	t.Run("missing session fails", func(t *testing.T) {
		_, _, err := runtime.workflowResumeContext(ctx, Run{
			SessionID: "session-missing", AgentID: "agent-missing", WorkMode: WorkModeLoop,
		})
		if err == nil || err.Error() != "session not found" {
			t.Fatalf("workflowResumeContext missing session err = %v, want session not found", err)
		}
	})

	t.Run("parent permission and work mode override resolved agent", func(t *testing.T) {
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:             "resume-context-agent",
			Name:           "Resume Context Agent",
			ProviderID:     testProviderID,
			Status:         AgentStatusEnabled,
			PermissionMode: PermissionModeApproval,
			WorkMode:       WorkModeChat,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume context")

		resumedSession, resumedAgent, err := runtime.workflowResumeContext(ctx, Run{
			SessionID:        session.ID,
			AgentID:          agent.ID,
			PermissionMode:   PermissionModeLessApproval,
			WorkMode:         WorkModeLoop,
			WorkflowStatus:   workflowStatusPaused,
			Status:           RunStatusPaused,
			ResumeState:      "user_paused",
			PauseRequestedAt: nil,
		})
		if err != nil {
			t.Fatalf("workflowResumeContext: %v", err)
		}
		if resumedSession.ID != session.ID {
			t.Fatalf("resumed session = %+v, want %s", resumedSession, session.ID)
		}
		if resumedAgent.ID != agent.ID || resumedAgent.PermissionMode != PermissionModeLessApproval || resumedAgent.WorkMode != WorkModeLoop {
			t.Fatalf("resumed agent = %+v, want inherited agent with parent overrides", resumedAgent)
		}
	})

	t.Run("disabled agent fails resume context", func(t *testing.T) {
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:       "resume-context-disabled-agent",
			Name:     "Resume Context Disabled Agent",
			Status:   AgentStatusDisabled,
			WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume context disabled")

		_, _, err := runtime.workflowResumeContext(ctx, Run{
			SessionID: session.ID,
			AgentID:   agent.ID,
			WorkMode:  WorkModeLoop,
		})
		if err == nil || err.Error() != "agent is disabled" {
			t.Fatalf("workflowResumeContext disabled agent err = %v, want agent is disabled", err)
		}
	})
}

func TestReconcileWorkflowChildrenShortCircuitsPausedAndCompletesFinishedChildren(t *testing.T) {
	ctx := context.Background()

	t.Run("user paused goal short-circuits as blocked", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "reconcile-paused-agent", Name: "Reconcile Paused", Status: AgentStatusEnabled,
			WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "reconcile paused")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-reconcile-paused", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
			PausedReason: "user", ResumeState: "user_paused",
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-reconcile-paused", Title: "暂停任务", Status: "DONE"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})

		updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
		if err != nil {
			t.Fatalf("reconcileWorkflowChildren paused: %v", err)
		}
		if !blocked {
			t.Fatal("reconcileWorkflowChildren blocked = false, want true for user-paused goal")
		}
		if updated.Status != RunStatusPaused || updated.ResumeState != "user_paused" || updated.PausedReason != "user" {
			t.Fatalf("updated parent = %+v, want still user-paused", updated)
		}
	})

	t.Run("completed children update tasks and do not block resume", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "reconcile-completed-agent", Name: "Reconcile Completed", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "reconcile completed")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-reconcile-completed", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			ChildRunIDs: []string{"child-reconcile-completed"},
			WorkflowPlan: []WorkflowStepState{{
				TaskID: "task-reconcile-completed", Title: "已完成子步骤", Status: "IN_PROGRESS", ChildRunID: "child-reconcile-completed",
			}},
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-reconcile-completed", Title: "已完成子步骤", Status: "IN_PROGRESS", AgentID: agent.ID,
			RunID: parent.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeTask,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		mustSaveRun(t, runtime, Run{
			ID: "child-reconcile-completed", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
			Status: RunStatusCompleted, Message: "child finished summary", UserMessage: "执行子步骤",
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})

		updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
		if err != nil {
			t.Fatalf("reconcileWorkflowChildren completed child: %v", err)
		}
		if blocked {
			t.Fatal("reconcileWorkflowChildren blocked = true, want false when all children completed")
		}
		if got := updated.WorkflowPlan[0].Status; got != "DONE" {
			t.Fatalf("workflow step status = %q, want DONE", got)
		}
		storedTask, ok, err := runtime.Store().Task(ctx, "task-reconcile-completed")
		if err != nil || !ok {
			t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "DONE" || storedTask.RunID != "child-reconcile-completed" || storedTask.ResultSummary != "child finished summary" {
			t.Fatalf("stored task = %+v, want completed child task projection", storedTask)
		}
	})
}

func TestReconcileWorkflowParentResolvesPendingAndTerminalChildren(t *testing.T) {
	ctx := context.Background()

	t.Run("resolved child approval resumes parent workflow", func(t *testing.T) {
		runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "reconcile-parent-approval-agent", Name: "Reconcile Parent Approval", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
		})
		response, err := runtime.Chat(ctx, ChatRequest{
			AgentID:          agent.ID,
			Message:          "请创建子智能体并 @approval.required 保存策略",
			Objective:        "验证父工作流对子审批恢复",
			WorkModeOverride: WorkModeTask,
		})
		if err != nil {
			t.Fatalf("Chat task approval workflow: %v", err)
		}
		if len(response.PendingApprovals) != 1 {
			t.Fatalf("pending approvals = %+v, want one child approval", response.PendingApprovals)
		}
		approvalID := response.PendingApprovals[0].ID
		if _, changed, err := runtime.Store().ResolvePendingApproval(ctx, approvalID, ApprovalStatusApproved); err != nil || !changed {
			t.Fatalf("ResolvePendingApproval changed=%v err=%v", changed, err)
		}
		parent, ok, err := runtime.Store().Run(ctx, response.Run.ID)
		if err != nil || !ok {
			t.Fatalf("parent run lookup ok=%v err=%v", ok, err)
		}

		runtime.reconcileWorkflowParent(ctx, parent)

		child := waitForRunStatus(t, runtime, response.PendingApprovals[0].RunID, RunStatusCompleted)
		if child.ResumeState != "adk_confirmation_resolved" {
			t.Fatalf("child run = %+v, want resumed child completion", child)
		}
		parent = waitForRunStatus(t, runtime, response.Run.ID, RunStatusCompleted)
		if parent.WorkflowStatus != workflowStatusComplete {
			t.Fatalf("parent run = %+v, want completed workflow", parent)
		}
		if executions.Load() != 1 {
			t.Fatalf("tool executions = %d, want 1", executions.Load())
		}
	})

	t.Run("terminal child failure propagates to parent workflow", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "reconcile-parent-terminal-agent", Name: "Reconcile Parent Terminal", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "reconcile parent terminal")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-reconcile-parent-terminal",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "处理失败的子运行",
			ChildRunIDs:    []string{"run-reconcile-parent-terminal-child"},
			WorkflowPlan: []WorkflowStepState{{
				TaskID: "task-reconcile-parent-terminal", Title: "失败子步骤", Status: "IN_PROGRESS", ChildRunID: "run-reconcile-parent-terminal-child",
			}},
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})
		child := mustSaveRun(t, runtime, Run{
			ID:            "run-reconcile-parent-terminal-child",
			SessionID:     session.ID,
			AgentID:       agent.ID,
			ParentRunID:   parent.ID,
			Status:        RunStatusFailed,
			Message:       "子运行失败",
			FailureReason: "provider failed",
			ErrorCode:     "MODEL_CALL_FAILED",
			CreatedAt:     now,
			UpdatedAt:     now,
			Usage:         &RunUsage{},
		})

		runtime.reconcileWorkflowParent(ctx, parent)

		stored, ok, err := runtime.Store().Run(ctx, parent.ID)
		if err != nil || !ok {
			t.Fatalf("parent run lookup ok=%v err=%v", ok, err)
		}
		if stored.Status != RunStatusFailed || stored.WorkflowStatus != workflowStatusFailed {
			t.Fatalf("stored parent = %+v, want failed parent workflow", stored)
		}
		if stored.FailureReason != child.FailureReason || stored.ErrorCode != child.ErrorCode {
			t.Fatalf("stored parent failure = %+v, want propagated child failure", stored)
		}
	})
}

func TestApprovalRecoveryContextHelpers(t *testing.T) {
	run := Run{
		Status:      RunStatusPending,
		ResumeState: "approval_resuming",
		PendingApprovals: []Approval{
			{ID: "approval-with-context", Status: ApprovalStatusPending, FunctionCallID: "call-1", ConfirmationCallID: "confirm-1"},
			{ID: "approval-resolved", Status: ApprovalStatusApproved, FunctionCallID: "call-2", ConfirmationCallID: "confirm-2"},
		},
	}
	if !runHasRecoverableApprovalContext(run) {
		t.Fatalf("runHasRecoverableApprovalContext(%+v) = false, want true", run)
	}
	if !runHasRecoverableResolvedApprovalContext(run) {
		t.Fatalf("runHasRecoverableResolvedApprovalContext(%+v) = false, want true", run)
	}
	if !runCanContinueResolvedApproval(run) {
		t.Fatalf("runCanContinueResolvedApproval(%+v) = false, want true", run)
	}

	run.WorkMode = WorkModeTask
	run.WorkflowStatus = workflowStatusPaused
	if runCanContinueResolvedApproval(run) {
		t.Fatalf("runCanContinueResolvedApproval(%+v) = true, want false for workflow parent", run)
	}

	run.WorkflowStatus = ""
	run.Status = RunStatusRunning
	run.PendingApprovals = []Approval{{ID: "approval-missing-context", Status: ApprovalStatusApproved}}
	if runHasRecoverableApprovalContext(run) {
		t.Fatalf("runHasRecoverableApprovalContext(%+v) = true, want false without confirmation context", run)
	}
	if runHasRecoverableResolvedApprovalContext(run) {
		t.Fatalf("runHasRecoverableResolvedApprovalContext(%+v) = true, want false without confirmation context", run)
	}
	if runCanContinueResolvedApproval(run) {
		t.Fatalf("runCanContinueResolvedApproval(%+v) = true, want false without recoverable context", run)
	}
}
