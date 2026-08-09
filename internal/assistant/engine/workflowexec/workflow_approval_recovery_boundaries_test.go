package workflowexec

import (
	"strings"
	"testing"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowApprovalRecoveryReturnsPausedSaveFailure(t *testing.T) {
	ctx := t.Context()
	runtime, agent, session := newWorkflowApprovalFixture(t, "resume-paused")
	pausedAt := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "coverage98-resume-paused-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
		PausedAt: &pausedAt, PausedReason: "user", CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER coverage98_reject_paused_resume BEFORE UPDATE ON `+enginepersistence.TableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN SELECT RAISE(FAIL, 'paused resume write rejected'); END`); err != nil {
		t.Fatalf("create paused resume trigger: %v", err)
	}
	if _, err := (&WorkflowExecutor{runtime: runtime}).ResumeLoopWorkflow(ctx, session, parent); err == nil || !strings.Contains(err.Error(), "paused resume write rejected") {
		t.Fatalf("resume paused workflow error = %v", err)
	}
}

func TestWorkflowApprovalReconcileFailsClosedOnTaskAndRunPersistence(t *testing.T) {
	ctx := t.Context()

	t.Run("completed child does not become an unrecorded task completion", func(t *testing.T) {
		runtime, agent, session := newWorkflowApprovalFixture(t, "task-store")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-task-store-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "coverage98-missing-task", ChildRunID: "coverage98-task-store-child", Status: "IN_PROGRESS"}},
			CreatedAt:    jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		mustSaveRun(t, runtime, Run{
			ID: "coverage98-task-store-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
			Status: RunStatusCompleted, Message: "child result", CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableTasks); err != nil {
			t.Fatalf("drop task table: %v", err)
		}

		updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(ctx, parent)
		if err != nil || blocked || len(updated.WorkflowPlan) != 1 || updated.WorkflowPlan[0].Status != "DONE" {
			t.Fatalf("completed child reconciliation = %+v blocked=%v err=%v", updated, blocked, err)
		}
	})

	for _, childStatus := range []string{RunStatusPending, RunStatusRunning} {
		t.Run("active child persistence failure "+childStatus, func(t *testing.T) {
			runtime, agent, session := newWorkflowApprovalFixture(t, "active-"+strings.ToLower(childStatus))
			parent := mustSaveRun(t, runtime, Run{
				ID: "coverage98-active-parent-" + strings.ToLower(childStatus), SessionID: session.ID, AgentID: agent.ID,
				Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
				WorkflowPlan: []WorkflowStepState{{ChildRunID: "coverage98-active-child-" + strings.ToLower(childStatus), Status: "IN_PROGRESS"}},
				CreatedAt:    jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
			})
			mustSaveRun(t, runtime, Run{
				ID: "coverage98-active-child-" + strings.ToLower(childStatus), SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
				Status: childStatus, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
			})
			if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER coverage98_reject_active_parent_`+strings.ToLower(childStatus)+` BEFORE UPDATE ON `+enginepersistence.TableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN SELECT RAISE(FAIL, 'active child state write rejected'); END`); err != nil {
				t.Fatalf("create active child trigger: %v", err)
			}
			if _, _, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(ctx, parent); err == nil || !strings.Contains(err.Error(), "active child state write rejected") {
				t.Fatalf("reconcile %s child error = %v", childStatus, err)
			}
		})
	}
}
