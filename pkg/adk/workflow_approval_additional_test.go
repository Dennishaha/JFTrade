package adk

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowApprovalAdditionalBoundaryBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("sync and resume surface save failures and missing parents", func(t *testing.T) {
		runtime := newTestRuntime(t)
		child := Run{ID: "child-missing-parent", ParentRunID: "missing-parent", Status: RunStatusCompleted}
		if synced, err := runtime.syncParentWorkflowFromChild(ctx, child); err != nil || synced != nil {
			t.Fatalf("syncParentWorkflowFromChild missing parent = %#v err=%v, want nil nil", synced, err)
		}

		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "goal-parent-sync-fail",
			SessionID:      "session-sync-fail",
			AgentID:        "agent",
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			CreatedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		installFailTrigger(t, runtime, "fail_runs_update_sync_parent", tableRuns, "UPDATE", "sync parent failed")
		if _, err := runtime.syncParentWorkflowFromChild(ctx, Run{ID: "child", ParentRunID: parent.ID, Status: RunStatusCompleted}); err == nil || !strings.Contains(err.Error(), "sync parent failed") {
			t.Fatalf("syncParentWorkflowFromChild save err = %v", err)
		}
	})

	t.Run("resume loop and completed workflow surface persistence failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		now := nowString()
		session := mustCreateSession(t, runtime, "agent", "resume loop failure")
		parent := mustSaveRun(t, runtime, Run{
			ID:               "goal-parent-resume-fail",
			SessionID:        session.ID,
			AgentID:          "agent",
			Status:           RunStatusRunning,
			WorkMode:         WorkModeLoop,
			WorkflowStatus:   workflowStatusRunning,
			PauseRequestedAt: new(string),
			CreatedAt:        now,
			UpdatedAt:        now,
			Usage:            &RunUsage{},
		})
		*parent.PauseRequestedAt = now
		if err := runtime.Store().SaveRun(ctx, parent); err != nil {
			t.Fatalf("SaveRun parent pause requested: %v", err)
		}
		installFailTrigger(t, runtime, "fail_runs_update_resume_loop", tableRuns, "UPDATE", "resume loop save failed")
		if _, err := executor.resumeLoopWorkflow(ctx, session, parent); err == nil || !strings.Contains(err.Error(), "resume loop save failed") {
			t.Fatalf("resumeLoopWorkflow err = %v", err)
		}

		runtime2 := newTestRuntime(t)
		executor2 := &WorkflowExecutor{runtime: runtime2}
		session2 := mustCreateSession(t, runtime2, "agent", "complete resumed failure")
		parent2 := mustSaveRun(t, runtime2, Run{
			ID:             "goal-parent-complete-fail",
			SessionID:      session2.ID,
			AgentID:        "agent",
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			CreatedAt:      nowString(),
			UpdatedAt:      nowString(),
			Usage:          &RunUsage{},
		})
		installFailTrigger(t, runtime2, "fail_runs_update_complete_resumed", tableRuns, "UPDATE", "complete resumed save failed")
		if _, err := executor2.completeResumedWorkflow(ctx, session2, parent2, "done"); err == nil || !strings.Contains(err.Error(), "complete resumed save failed") {
			t.Fatalf("completeResumedWorkflow err = %v", err)
		}
	})
}
