package adk

import (
	"context"
	"testing"
)

func TestCoverage98LifecycleFailsClosedWhenStorageStopsDuringReconciliation(t *testing.T) {
	ctx := context.Background()
	runtime := &Runtime{store: newClosedStoreForLifecycle(t)}

	// Shutdown can race with the periodic reconciler. It must abandon the
	// pass rather than treating an unreadable run list as an empty list.
	runtime.reconcileStaleRuns(ctx)

	// A cancellation already in progress must surface the persistence failure
	// instead of reporting a cancelled run that was never durably recorded.
	if _, err := runtime.cancelRunTree(ctx, Run{ID: "shutdown-parent", Status: RunStatusRunning}, "server shutdown", "SERVER_STOPPED", "cancelled during shutdown", "run.shutdown"); err == nil {
		t.Fatal("cancelRunTree after store shutdown returned nil")
	}

	// Parent cleanup is best-effort after a terminal transition. If storage
	// closes between parent and child cleanup, it must simply skip unreadable
	// children; it must not manufacture terminal child records.
	runtime.cancelUnfinishedWorkflowChildren(ctx, Run{
		ID:           "shutdown-parent",
		ChildRunIDs:  []string{"shutdown-child"},
		WorkflowPlan: []WorkflowStepState{{ChildRunID: "shutdown-child"}},
	})
	if runtime.isDormantWorkflowChildRun(ctx, Run{ID: "shutdown-child", ParentRunID: "shutdown-parent", Status: RunStatusRunning}) {
		t.Fatal("child with an unreadable parent was treated as dormant")
	}
}
