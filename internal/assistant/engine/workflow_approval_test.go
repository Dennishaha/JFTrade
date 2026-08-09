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

}
