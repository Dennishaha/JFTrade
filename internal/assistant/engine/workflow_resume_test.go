package adk

import (
	"context"
	"strings"
	"testing"
)

func TestResumeLoopWorkflowHonorsUserPauseAndCompletedChild(t *testing.T) {
	ctx := context.Background()

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
			return
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
