package adk

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowApprovalStateTransitionsPersistTheObservableOutcome(t *testing.T) {
	ctx := context.Background()

	t.Run("child pending states project their exact parent lifecycle", func(t *testing.T) {
		for _, status := range []string{RunStatusPendingInput, RunStatusPending, RunStatusRunning} {
			t.Run(strings.ToLower(status), func(t *testing.T) {
				runtime, agent, session := newWorkflowApprovalFixture(t, "sync-"+strings.ToLower(status))
				parent := mustSaveRun(t, runtime, Run{
					ID: "coverage98-sync-parent-" + strings.ToLower(status), SessionID: session.ID, AgentID: agent.ID,
					Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
					CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
				})
				child := Run{ID: "coverage98-sync-child-" + strings.ToLower(status), ParentRunID: parent.ID, Status: status, Message: "child " + status}
				updated, err := runtime.syncParentWorkflowFromChild(ctx, child)
				if err != nil || updated == nil || updated.Status != status || updated.Message != child.Message {
					t.Fatalf("sync %s parent=%+v err=%v", status, updated, err)
					return
				}
				wantWorkflow := workflowStatusPaused
				if status == RunStatusRunning {
					wantWorkflow = workflowStatusRunning
				}
				if updated.WorkflowStatus != wantWorkflow {
					t.Fatalf("sync %s workflow status = %q, want %q", status, updated.WorkflowStatus, wantWorkflow)
				}
			})
		}
	})

	t.Run("pause requests and terminal persistence failures do not become silent success", func(t *testing.T) {
		runtime, agent, session := newWorkflowApprovalFixture(t, "pause-and-terminal")
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-sync-pause-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		updated, err := runtime.syncParentWorkflowFromChild(ctx, Run{ID: "coverage98-sync-pause-child", ParentRunID: parent.ID, Status: RunStatusCompleted})
		if err != nil || updated == nil || updated.Status != RunStatusPaused || updated.PausedReason != "user" {
			t.Fatalf("requested pause projection = %+v, %v", updated, err)
		}

		terminalParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-terminal-save-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER coverage98_reject_terminal_parent BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+terminalParent.ID+`' BEGIN SELECT RAISE(FAIL, 'terminal parent write rejected'); END`); err != nil {
			t.Fatalf("create terminal failure trigger: %v", err)
		}
		terminated, terminateErr := runtime.TerminateParentWorkflowFromChild(ctx, terminalParent, Run{ID: "coverage98-terminal-child", ParentRunID: terminalParent.ID, Status: RunStatusFailed, Message: "child failed"})
		if terminateErr == nil || !strings.Contains(terminateErr.Error(), "terminal parent write rejected") {
			t.Fatalf("terminate parent error = %v, want persistence failure", terminateErr)
		}
		if terminated.Status != RunStatusFailed || terminated.WorkflowStatus != workflowStatusFailed {
			t.Fatalf("terminal projection = %+v", terminated)
		}
		stored, ok, err := runtime.Store().Run(ctx, terminalParent.ID)
		if err != nil || !ok || stored.Status != RunStatusRunning {
			t.Fatalf("failed terminal persistence must not be reported as stored: %+v/%v/%v", stored, ok, err)
		}
	})
}
