package adk

import (
	"context"
	"testing"
)

func TestUserPausedGoalParentPreservesPauseWhileChildStateChanges(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "paused-parent-sync-agent", Name: "Paused Parent Sync", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "paused parent sync")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:             "run-paused-parent-sync",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusPaused,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusPaused,
		Message:        "目标已暂停。",
		Objective:      "暂停后等待子运行",
		PausedReason:   "user",
		PausedAt:       &now,
		ResumeState:    "user_paused",
		ChildRunIDs:    []string{"run-paused-parent-sync-child"},
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "task-paused-parent-sync", Title: "待审批子步骤", Status: "IN_PROGRESS", ChildRunID: "run-paused-parent-sync-child",
		}},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	approval := Approval{
		ID: "approval-paused-parent-sync", RunID: "run-paused-parent-sync-child", AgentID: agent.ID,
		ToolName: "approval.required", Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now,
	}
	child := mustSaveRun(t, runtime, Run{
		ID:               "run-paused-parent-sync-child",
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

	updated, err := runtime.syncParentWorkflowFromChild(ctx, child)
	if err != nil {
		t.Fatalf("syncParentWorkflowFromChild paused parent: %v", err)
	}
	if updated == nil || updated.Status != RunStatusPaused || updated.WorkflowStatus != workflowStatusPaused || updated.ResumeState != "user_paused" || updated.PausedReason != "user" {
		t.Fatalf("updated parent = %+v, want user-paused parent preserved", updated)
	}
	if len(updated.PendingApprovals) != 1 || updated.PendingApprovals[0].ID != approval.ID {
		t.Fatalf("updated approvals = %+v, want mirrored child approval", updated.PendingApprovals)
	}
	if got := updated.WorkflowPlan[0].Status; got != "BLOCKED" {
		t.Fatalf("workflow step status = %q, want BLOCKED while child awaits approval", got)
	}

	continued, err := runtime.continueParentWorkflowAfterChild(ctx, child)
	if err != nil {
		t.Fatalf("continueParentWorkflowAfterChild paused parent: %v", err)
	}
	if continued == nil || continued.Status != RunStatusPaused || continued.Message != "目标已暂停。" {
		t.Fatalf("continued parent = %+v, want user-paused parent unchanged", continued)
	}
}

func TestCompletedChildReopensPendingParentWorkflowToRunning(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "parent-reopen-completed-child-agent", Name: "Parent Reopen Completed Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "reopen completed child")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-pending-resume-from-child", SessionID: session.ID, AgentID: agent.ID,
		Status:         RunStatusPending,
		WorkMode:       WorkModeTask,
		WorkflowStatus: workflowStatusPaused,
		Message:        "等待用户审批后继续执行。",
		Objective:      "完成审批后的续跑",
		ChildRunIDs:    []string{"child-approved-complete"},
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "task-approved-complete", Title: "需要审批的步骤", Status: "BLOCKED", ChildRunID: "child-approved-complete",
		}},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID:          "child-approved-complete",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		ParentRunID: parent.ID,
		Status:      RunStatusCompleted,
		Message:     "子运行完成",
		PendingApprovals: []Approval{{
			ID: "approval-approved-history", RunID: "child-approved-complete", AgentID: agent.ID, Status: ApprovalStatusApproved,
		}},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})

	updated, err := runtime.syncParentWorkflowFromChild(ctx, child)
	if err != nil {
		t.Fatalf("syncParentWorkflowFromChild completed child: %v", err)
	}
	if updated == nil || updated.Status != RunStatusRunning || updated.WorkflowStatus != workflowStatusRunning || updated.Message != "workflow resumed" {
		t.Fatalf("updated parent = %+v, want reopened running workflow", updated)
	}
	if len(updated.PendingApprovals) != 0 {
		t.Fatalf("updated pending approvals = %+v, want cleared pending approvals", updated.PendingApprovals)
	}
	if got := updated.WorkflowPlan[0].Status; got != "DONE" {
		t.Fatalf("workflow step status = %q, want DONE after child completion", got)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusRunning || stored.WorkflowStatus != workflowStatusRunning || stored.Message != "workflow resumed" {
		t.Fatalf("stored parent = %+v, want reopened running workflow", stored)
	}
}

func TestCompletedChildTaskResumeFailsWhenSiblingTaskIsBlocked(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "parent-task-resume-blocked-agent", Name: "Parent Task Resume Blocked", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "task resume blocked")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:             "parent-task-resume-blocked",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusPending,
		WorkMode:       WorkModeTask,
		WorkflowStatus: workflowStatusPaused,
		Message:        "等待子运行完成",
		Objective:      "恢复后发现阻塞任务",
		UserMessage:    "继续工作流",
		ChildRunIDs:    []string{"child-task-resume-blocked"},
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-task-resume-completed-child", Title: "已完成子步骤", Status: "BLOCKED", ChildRunID: "child-task-resume-blocked"},
			{TaskID: "task-task-resume-blocked-sibling", Title: "缺数据步骤", Status: "BLOCKED"},
		},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-task-resume-completed-child", Title: "已完成子步骤", Status: "BLOCKED", AgentID: agent.ID,
		RunID: parent.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeTask, Objective: parent.Objective,
	}); err != nil {
		t.Fatalf("Save child task: %v", err)
	}
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-task-resume-blocked-sibling", Title: "缺数据步骤", Status: "BLOCKED", AgentID: agent.ID,
		RunID: parent.ID, WorkflowMode: WorkModeTask, Objective: parent.Objective, ResultSummary: "缺少行情数据",
	}); err != nil {
		t.Fatalf("Save blocked sibling task: %v", err)
	}
	completedAt := nowString()
	child := mustSaveRun(t, runtime, Run{
		ID:          "child-task-resume-blocked",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		ParentRunID: parent.ID,
		Status:      RunStatusCompleted,
		Message:     "子运行完成",
		CompletedAt: &completedAt,
		CreatedAt:   now,
		UpdatedAt:   completedAt,
		Usage:       &RunUsage{},
	})

	updated, err := runtime.continueParentWorkflowAfterChild(ctx, child)
	if err != nil {
		t.Fatalf("continueParentWorkflowAfterChild blocked sibling: %v", err)
	}
	if updated == nil || updated.Status != RunStatusFailed || updated.WorkflowStatus != workflowStatusFailed {
		t.Fatalf("updated parent = %+v, want failed parent after blocked sibling", updated)
	}
	if updated.ErrorCode != "WORKFLOW_TASK_BLOCKED" || updated.FailureReason != "缺少行情数据" {
		t.Fatalf("updated parent failure = %+v, want blocked sibling summary", updated)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusFailed || stored.FailureReason != "缺少行情数据" {
		t.Fatalf("stored parent = %+v, want persisted blocked sibling failure", stored)
	}
}

func TestNonWorkflowParentIgnoresChildWorkflowCallbacks(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "non-workflow-parent-agent", Name: "Non Workflow Parent", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat,
	})
	session := mustCreateSession(t, runtime, agent.ID, "non workflow parent")
	now := nowString()
	chatParent := mustSaveRun(t, runtime, Run{
		ID:        "run-chat-parent-ignore",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    RunStatusRunning,
		WorkMode:  WorkModeChat,
		Message:   "chat run",
		CreatedAt: now,
		UpdatedAt: now,
		Usage:     &RunUsage{},
	})
	blankWorkflowParent := mustSaveRun(t, runtime, Run{
		ID:        "run-blank-workflow-parent-ignore",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    RunStatusRunning,
		WorkMode:  WorkModeTask,
		Message:   "not marked as workflow",
		CreatedAt: now,
		UpdatedAt: now,
		Usage:     &RunUsage{},
	})

	for _, tc := range []struct {
		name     string
		parent   Run
		childRun string
	}{
		{name: "chat parent", parent: chatParent, childRun: "run-chat-parent-ignore-child"},
		{name: "blank workflow status", parent: blankWorkflowParent, childRun: "run-blank-workflow-parent-ignore-child"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := mustSaveRun(t, runtime, Run{
				ID:          tc.childRun,
				SessionID:   session.ID,
				AgentID:     agent.ID,
				ParentRunID: tc.parent.ID,
				Status:      RunStatusCompleted,
				Message:     "stray child callback",
				CreatedAt:   now,
				UpdatedAt:   now,
				Usage:       &RunUsage{},
			})

			updated, err := runtime.syncParentWorkflowFromChild(ctx, child)
			if err != nil {
				t.Fatalf("syncParentWorkflowFromChild ignore: %v", err)
			}
			if updated != nil {
				t.Fatalf("updated parent = %+v, want nil for non-workflow parent", updated)
			}

			continued, err := runtime.continueParentWorkflowAfterChild(ctx, child)
			if err != nil {
				t.Fatalf("continueParentWorkflowAfterChild ignore: %v", err)
			}
			if continued != nil {
				t.Fatalf("continued parent = %+v, want nil for non-workflow parent", continued)
			}

			stored, ok, err := runtime.Store().Run(ctx, tc.parent.ID)
			if err != nil || !ok {
				t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
			}
			if stored.Status != tc.parent.Status || stored.Message != tc.parent.Message || len(stored.ChildRunIDs) != len(tc.parent.ChildRunIDs) {
				t.Fatalf("stored parent = %+v, want unchanged parent %+v", stored, tc.parent)
			}
		})
	}
}

func TestReconcileWorkflowChildrenIgnoresMissingAndForeignRuns(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "reconcile-ignore-agent", Name: "Reconcile Ignore", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "reconcile ignore")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:             "run-reconcile-ignore",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusRunning,
		WorkMode:       WorkModeTask,
		WorkflowStatus: workflowStatusRunning,
		ChildRunIDs:    []string{"child-missing-ignore", "child-foreign-ignore"},
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-missing-ignore", Title: "缺失子步骤", Status: "IN_PROGRESS", ChildRunID: "child-missing-ignore"},
			{TaskID: "task-foreign-ignore", Title: "串错子步骤", Status: "IN_PROGRESS", ChildRunID: "child-foreign-ignore"},
		},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID:          "child-foreign-ignore",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		ParentRunID: "different-parent",
		Status:      RunStatusCompleted,
		Message:     "不属于这个父工作流",
		CreatedAt:   now, UpdatedAt: now, Usage: &RunUsage{},
	})

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
	if err != nil {
		t.Fatalf("reconcileWorkflowChildren ignore stale children: %v", err)
	}
	if blocked {
		t.Fatal("reconcileWorkflowChildren blocked = true, want false when only missing/foreign children remain")
	}
	if updated.Status != RunStatusRunning || updated.WorkflowStatus != workflowStatusRunning {
		t.Fatalf("updated parent = %+v, want unchanged running workflow", updated)
	}
	if got := updated.WorkflowPlan[0].Status; got != "IN_PROGRESS" {
		t.Fatalf("missing child step status = %q, want unchanged IN_PROGRESS", got)
	}
	if got := updated.WorkflowPlan[1].Status; got != "IN_PROGRESS" {
		t.Fatalf("foreign child step status = %q, want unchanged IN_PROGRESS", got)
	}
}
