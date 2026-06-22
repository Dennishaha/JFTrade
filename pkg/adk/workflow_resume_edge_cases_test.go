package adk

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestResumeLoopWorkflowHonorsUserPauseAndCompletedChild(t *testing.T) {
	ctx := context.Background()

	t.Run("user paused parent stays paused on resume", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-loop-paused-agent", Name: "Resume Loop Paused", Status: AgentStatusEnabled,
			WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume loop paused")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-resume-loop-paused", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
			Objective: "暂停中的目标", PausedReason: "user", ResumeState: "user_paused",
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-resume-loop-paused", Title: "暂停任务", Status: "DONE"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})

		updated, err := (&WorkflowExecutor{runtime: runtime}).resumeLoopWorkflow(ctx, session, parent)
		if err != nil {
			t.Fatalf("resumeLoopWorkflow paused: %v", err)
		}
		if updated.Status != RunStatusPaused || updated.WorkflowStatus != workflowStatusPaused || updated.ResumeState != "user_paused" || updated.PausedReason != "user" {
			t.Fatalf("updated parent = %+v, want still user-paused", updated)
		}
	})

	t.Run("completed child resumes and completes loop parent", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-loop-complete-agent", Name: "Resume Loop Complete", Status: AgentStatusEnabled,
			WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume loop complete")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-resume-loop-complete", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, Message: "等待子运行", UserMessage: "推进目标", WorkMode: WorkModeLoop,
			Objective: "推进目标", Iteration: 1, WorkflowStatus: workflowStatusRunning,
			ChildRunIDs: []string{"run-resume-loop-child"},
			WorkflowPlan: []WorkflowStepState{{
				TaskID: "task-resume-loop-child", Title: "子步骤", Message: "执行子步骤", Status: "IN_PROGRESS", ChildRunID: "run-resume-loop-child",
			}},
			CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-resume-loop-child", Title: "子步骤", Status: "IN_PROGRESS", AgentID: agent.ID,
			RunID: parent.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeLoop,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		completedAt := nowString()
		child := mustSaveRun(t, runtime, Run{
			ID: "run-resume-loop-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
			Status: RunStatusCompleted, Message: "子运行完成", UserMessage: "执行子步骤",
			CompletedAt: &completedAt,
			CreatedAt:   now, StartedAt: now, UpdatedAt: completedAt, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
		})

		updated, err := runtime.continueParentWorkflowAfterChild(ctx, child)
		if err != nil {
			t.Fatalf("continueParentWorkflowAfterChild completed loop child: %v", err)
		}
		if updated == nil || updated.Status != RunStatusCompleted || updated.WorkflowStatus != workflowStatusComplete || updated.FinalMessageID == "" {
			t.Fatalf("updated parent = %+v, want completed resumed loop workflow", updated)
		}
		if !strings.Contains(updated.Message, "workflow completed") {
			t.Fatalf("updated message = %q, want workflow completed", updated.Message)
		}
		storedTask, ok, err := runtime.Store().Task(ctx, "task-resume-loop-child")
		if err != nil || !ok {
			t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "DONE" || storedTask.ResultSummary != "子运行完成" {
			t.Fatalf("stored task = %+v, want DONE with child summary", storedTask)
		}
	})
}

func TestRunChildAndWorkflowResumeEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("runChild blocks task when delegated child needs approval", func(t *testing.T) {
		runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID:             "run-child-approval-agent",
			Name:           "Run Child Approval",
			ProviderID:     testProviderID,
			Status:         AgentStatusEnabled,
			WorkMode:       WorkModeTask,
			PermissionMode: PermissionModeApproval,
			Tools:          []string{"approval.required"},
		})
		session := mustCreateSession(t, runtime, agent.ID, "run child approval")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-child-approval-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "保存策略草稿",
			CreatedAt:      now,
			StartedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID:           "task-run-child-approval",
			Title:        "保存策略草稿",
			Status:       "IN_PROGRESS",
			AgentID:      agent.ID,
			RunID:        parent.ID,
			Order:        1,
			WorkflowMode: WorkModeTask,
			Objective:    parent.Objective,
			Message:      "请 @approval.required 保存策略",
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
		if err := runtime.Store().SaveRun(ctx, parent); err != nil {
			t.Fatalf("SaveRun parent with plan: %v", err)
		}

		result := (&WorkflowExecutor{runtime: runtime}).runChild(ctx, workflowRequest{
			Agent:     agent,
			Session:   session,
			Objective: parent.Objective,
		}, parent, workflowStep{
			Title:   task.Title,
			Message: "请 @approval.required 保存策略",
		}, task, 1)
		if result.Err != nil {
			t.Fatalf("runChild approval path: %v", result.Err)
		}
		if result.Response.Run.Status != RunStatusPending || len(result.Response.PendingApprovals) != 1 {
			t.Fatalf("child response = %+v, want pending approval child response", result.Response)
		}
		if executions.Load() != 0 {
			t.Fatalf("tool executions = %d, want 0 before approval is granted", executions.Load())
		}
		storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
		if err != nil || !ok {
			t.Fatalf("Task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "BLOCKED" || storedTask.Executor != workflowTaskExecutorChild || storedTask.RunID == "" {
			t.Fatalf("stored task = %+v, want blocked child task with run id", storedTask)
		}
		if !strings.Contains(storedTask.ResultSummary, "审批队列") {
			t.Fatalf("task result summary = %q, want approval guidance", storedTask.ResultSummary)
		}
		child, ok, err := runtime.Store().Run(ctx, storedTask.RunID)
		if err != nil || !ok {
			t.Fatalf("child run lookup ok=%v err=%v", ok, err)
		}
		if child.ParentRunID != parent.ID || child.Status != RunStatusPending || len(child.PendingApprovals) != 1 {
			t.Fatalf("stored child = %+v, want pending child linked to parent", child)
		}
		storedParent, ok, err := runtime.Store().Run(ctx, parent.ID)
		if err != nil || !ok {
			t.Fatalf("parent run lookup ok=%v err=%v", ok, err)
		}
		if len(storedParent.ChildRunIDs) != 1 || storedParent.ChildRunIDs[0] != child.ID {
			t.Fatalf("stored parent child runs = %+v, want child %q", storedParent.ChildRunIDs, child.ID)
		}
		if got := storedParent.WorkflowPlan[0].ChildRunID; got != child.ID {
			t.Fatalf("stored workflow plan child run id = %q, want %q", got, child.ID)
		}
	})

	t.Run("runChild returns delta sink errors after child launch snapshot", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "run-child-delta-error-agent", Name: "Run Child Delta Error", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "run child delta error")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-child-delta-error-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "整理一个子任务",
			CreatedAt:      now,
			StartedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID:           "task-run-child-delta-error",
			Title:        "整理一个子任务",
			Status:       "IN_PROGRESS",
			AgentID:      agent.ID,
			RunID:        parent.ID,
			Order:        1,
			WorkflowMode: WorkModeTask,
			Objective:    parent.Objective,
			Message:      "整理这个子任务",
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
		if err := runtime.Store().SaveRun(ctx, parent); err != nil {
			t.Fatalf("SaveRun parent with plan: %v", err)
		}
		wantErr := fmt.Errorf("delta sink closed")

		result := (&WorkflowExecutor{runtime: runtime}).runChild(ctx, workflowRequest{
			Agent:   agent,
			Session: session,
			Mode:    WorkModeTask,
			EmitRun: true,
			OnDelta: func(ChatDelta) error { return wantErr },
		}, parent, workflowStep{
			Title:   task.Title,
			Message: "整理这个子任务",
		}, task, 1)
		if result.Err == nil || result.Err.Error() != wantErr.Error() {
			t.Fatalf("runChild delta error = %v, want %v", result.Err, wantErr)
		}
		storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
		if err != nil || !ok {
			t.Fatalf("Task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "IN_PROGRESS" || storedTask.RunID == "" {
			t.Fatalf("stored task = %+v, want child claimed with run id before delta error", storedTask)
		}
		child, ok, err := runtime.Store().Run(ctx, storedTask.RunID)
		if err != nil || !ok {
			t.Fatalf("child run lookup ok=%v err=%v", ok, err)
		}
		if child.ParentRunID != parent.ID || child.Status != RunStatusRunning {
			t.Fatalf("stored child = %+v, want still-running child after snapshot failure", child)
		}
	})

	t.Run("runChild marks task blocked when child execution fails immediately", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "run-child-immediate-fail-agent", Name: "Run Child Immediate Fail", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "run child immediate fail")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-child-immediate-fail-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "触发子运行立即失败",
			CreatedAt:      now,
			StartedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID:           "task-run-child-immediate-fail",
			Title:        "失败子任务",
			Status:       "IN_PROGRESS",
			AgentID:      agent.ID,
			RunID:        parent.ID,
			Order:        1,
			WorkflowMode: WorkModeTask,
			Objective:    parent.Objective,
			Message:      "执行会失败的子任务",
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
		if err := runtime.Store().SaveRun(ctx, parent); err != nil {
			t.Fatalf("SaveRun parent with plan: %v", err)
		}
		badAgent := agent
		badAgent.ProviderID = ""

		result := (&WorkflowExecutor{runtime: runtime}).runChild(ctx, workflowRequest{
			Agent:     badAgent,
			Session:   session,
			Mode:      WorkModeTask,
			Objective: parent.Objective,
		}, parent, workflowStep{
			Title:   task.Title,
			Message: "执行会失败的子任务",
		}, task, 1)
		if result.Err != nil {
			t.Fatalf("runChild immediate fail err = %v, want nil with failed child response", result.Err)
		}
		if result.Response.Run.Status != RunStatusFailed || result.Response.Reply != "agent provider is required" {
			t.Fatalf("child response = %+v, want failed child run with provider error", result.Response)
		}
		storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
		if err != nil || !ok {
			t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "BLOCKED" || storedTask.ResultSummary != "agent provider is required" {
			t.Fatalf("stored task = %+v, want blocked task with child failure summary", storedTask)
		}
	})

	t.Run("resumeLoopWorkflow honors pause request after child completion", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-loop-pause-request-agent", Name: "Resume Loop Pause Request", Status: AgentStatusEnabled,
			WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume loop pause request")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:               "run-resume-loop-pause-request",
			SessionID:        session.ID,
			AgentID:          agent.ID,
			Status:           RunStatusRunning,
			Message:          "等待子运行",
			UserMessage:      "推进目标",
			WorkMode:         WorkModeLoop,
			Objective:        "推进目标",
			Iteration:        1,
			WorkflowStatus:   workflowStatusRunning,
			PauseRequestedAt: &now,
			ChildRunIDs:      []string{"run-resume-loop-pause-request-child"},
			WorkflowPlan: []WorkflowStepState{{
				TaskID: "task-resume-loop-pause-request", Title: "子步骤", Message: "执行子步骤", Status: "IN_PROGRESS",
				ChildRunID: "run-resume-loop-pause-request-child",
			}},
			CreatedAt: now, StartedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-resume-loop-pause-request", Title: "子步骤", Status: "IN_PROGRESS", AgentID: agent.ID,
			RunID: parent.ID, Executor: workflowTaskExecutorChild, WorkflowMode: WorkModeLoop,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		completedAt := nowString()
		mustSaveRun(t, runtime, Run{
			ID:          "run-resume-loop-pause-request-child",
			SessionID:   session.ID,
			AgentID:     agent.ID,
			ParentRunID: parent.ID,
			Status:      RunStatusCompleted,
			Message:     "子运行完成",
			UserMessage: "执行子步骤",
			CompletedAt: &completedAt,
			CreatedAt:   now,
			StartedAt:   now,
			UpdatedAt:   completedAt,
			Usage:       &RunUsage{},
		})

		updated, err := (&WorkflowExecutor{runtime: runtime}).resumeLoopWorkflow(ctx, session, parent)
		if err != nil {
			t.Fatalf("resumeLoopWorkflow pause request: %v", err)
		}
		if updated.Status != RunStatusPaused || updated.WorkflowStatus != workflowStatusPaused || updated.ResumeState != "user_paused" || updated.PausedReason != "user" {
			t.Fatalf("updated parent = %+v, want user-paused parent", updated)
		}
		if updated.CompletedAt != nil {
			t.Fatalf("updated completed at = %v, want nil while pause request wins", *updated.CompletedAt)
		}
		if got := updated.WorkflowPlan[0].Status; got != "DONE" {
			t.Fatalf("workflow step status = %q, want DONE before parent pauses", got)
		}
		storedTask, ok, err := runtime.Store().Task(ctx, "task-resume-loop-pause-request")
		if err != nil || !ok {
			t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "DONE" || storedTask.ResultSummary != "子运行完成" {
			t.Fatalf("stored task = %+v, want completed child summary before pause", storedTask)
		}
	})

	t.Run("continueParentWorkflowAfterChild terminates denied child and fails missing resume context", func(t *testing.T) {
		t.Run("denied child terminates parent workflow", func(t *testing.T) {
			runtime := newTestRuntime(t)
			agent := mustSaveAgent(t, runtime, AgentWriteRequest{
				ID: "continue-parent-denied-agent", Name: "Continue Parent Denied", Status: AgentStatusEnabled,
				WorkMode: WorkModeLoop,
			})
			session := mustCreateSession(t, runtime, agent.ID, "continue parent denied")
			now := nowString()
			parent := mustSaveRun(t, runtime, Run{
				ID:             "run-continue-parent-denied",
				SessionID:      session.ID,
				AgentID:        agent.ID,
				Status:         RunStatusRunning,
				WorkMode:       WorkModeLoop,
				WorkflowStatus: workflowStatusRunning,
				Objective:      "等待审批结果",
				ChildRunIDs:    []string{"run-continue-parent-denied-child"},
				WorkflowPlan: []WorkflowStepState{{
					TaskID: "task-continue-parent-denied", Title: "审批步骤", Status: "IN_PROGRESS", ChildRunID: "run-continue-parent-denied-child",
				}},
				CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
			})
			child := mustSaveRun(t, runtime, Run{
				ID:            "run-continue-parent-denied-child",
				SessionID:     session.ID,
				AgentID:       agent.ID,
				ParentRunID:   parent.ID,
				Status:        RunStatusDenied,
				Message:       "审批被拒绝",
				FailureReason: "",
				CreatedAt:     now,
				UpdatedAt:     now,
				Usage:         &RunUsage{},
			})

			updated, err := runtime.continueParentWorkflowAfterChild(ctx, child)
			if err != nil {
				t.Fatalf("continueParentWorkflowAfterChild denied: %v", err)
			}
			if updated == nil || updated.Status != RunStatusDenied || updated.WorkflowStatus != workflowStatusFailed {
				t.Fatalf("updated parent = %+v, want denied failed parent", updated)
			}
			if updated.ErrorCode != "APPROVAL_DENIED" || !strings.Contains(updated.FailureReason, "approval was denied") {
				t.Fatalf("updated parent failure = %+v, want approval denied failure", updated)
			}
			if updated.CompletedAt == nil {
				t.Fatal("updated completed at is nil, want terminal timestamp")
			}
		})

		t.Run("missing session during resume fails parent", func(t *testing.T) {
			runtime := newTestRuntime(t)
			agent := mustSaveAgent(t, runtime, AgentWriteRequest{
				ID: "continue-parent-missing-session-agent", Name: "Continue Parent Missing Session", Status: AgentStatusEnabled,
				WorkMode: WorkModeLoop,
			})
			now := nowString()
			parent := mustSaveRun(t, runtime, Run{
				ID:             "run-continue-parent-missing-session",
				SessionID:      "session-missing-for-resume",
				AgentID:        agent.ID,
				Status:         RunStatusRunning,
				WorkMode:       WorkModeLoop,
				WorkflowStatus: workflowStatusRunning,
				Objective:      "恢复目标",
				ChildRunIDs:    []string{"run-continue-parent-missing-session-child"},
				WorkflowPlan: []WorkflowStepState{{
					TaskID: "task-continue-parent-missing-session", Title: "子步骤", Status: "IN_PROGRESS", ChildRunID: "run-continue-parent-missing-session-child",
				}},
				CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
			})
			child := mustSaveRun(t, runtime, Run{
				ID:          "run-continue-parent-missing-session-child",
				SessionID:   "session-missing-for-resume",
				AgentID:     agent.ID,
				ParentRunID: parent.ID,
				Status:      RunStatusCompleted,
				Message:     "子运行完成",
				CreatedAt:   now,
				UpdatedAt:   now,
				Usage:       &RunUsage{},
			})

			updated, err := runtime.continueParentWorkflowAfterChild(ctx, child)
			if err != nil {
				t.Fatalf("continueParentWorkflowAfterChild missing session: %v", err)
			}
			if updated == nil || updated.Status != RunStatusFailed || updated.WorkflowStatus != workflowStatusFailed {
				t.Fatalf("updated parent = %+v, want failed parent after missing resume context", updated)
			}
			if updated.FailureReason != "session not found" || updated.ErrorCode != "MODEL_CALL_FAILED" {
				t.Fatalf("updated failure = %+v, want session not found failure", updated)
			}
			stored, ok, err := runtime.Store().Run(ctx, parent.ID)
			if err != nil || !ok {
				t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
			}
			if stored.Status != RunStatusFailed || stored.FailureReason != "session not found" {
				t.Fatalf("stored parent = %+v, want persisted session-not-found failure", stored)
			}
		})
	})
}
