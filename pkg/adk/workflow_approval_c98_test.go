package adk

import (
	"strings"
	"testing"
)

func TestCoverage98WorkflowApprovalRecoveryPreservesUserPauseAndContextFailures(t *testing.T) {
	ctx := t.Context()

	t.Run("a second pause write must report persistence failure", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "pause-write")
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-approval-pause-write-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			PauseRequestedAt: &pauseRequestedAt, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_second_pause_write BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+parent.ID+`' AND OLD.status = '`+RunStatusPaused+`' BEGIN SELECT RAISE(FAIL, 'second pause write rejected'); END`); err != nil {
			t.Fatalf("create pause trigger: %v", err)
		}

		child := Run{ID: "coverage98-approval-pause-write-child", ParentRunID: parent.ID, Status: RunStatusCompleted, Message: "child finished"}
		if resumed, err := runtime.continueParentWorkflowAfterChild(ctx, child); err == nil || resumed != nil || !strings.Contains(err.Error(), "second pause write rejected") {
			t.Fatalf("continue paused parent = %+v, %v", resumed, err)
		}
		stored, ok, err := runtime.Store().Run(ctx, parent.ID)
		if err != nil || !ok || stored.Status != RunStatusPaused || stored.PausedReason != "user" {
			t.Fatalf("first pause was not durably retained: %+v/%v/%v", stored, ok, err)
		}
	})

	t.Run("resumption rejects missing agents and unavailable skills", func(t *testing.T) {
		runtime, _, session := newCoverage98WorkflowApprovalFixture(t, "resume-context")
		if _, _, err := runtime.workflowResumeContext(ctx, Run{SessionID: session.ID, AgentID: "coverage98-missing-agent"}); err == nil || !strings.Contains(err.Error(), "agent not found") {
			t.Fatalf("missing workflow agent error = %v", err)
		}
		unavailableSkillAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "coverage98-resume-missing-skill", Name: "Resume Missing Skill", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop, Skills: []string{"coverage98-not-installed"},
		})
		if _, _, err := runtime.workflowResumeContext(ctx, Run{SessionID: session.ID, AgentID: unavailableSkillAgent.ID}); err == nil || !strings.Contains(err.Error(), "skill not found") {
			t.Fatalf("missing workflow skill error = %v", err)
		}
	})

	t.Run("already paused workflow returns its save failure", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "resume-paused")
		pausedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-paused-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
			PausedAt: &pausedAt, PausedReason: "user", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_paused_resume BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN SELECT RAISE(FAIL, 'paused resume write rejected'); END`); err != nil {
			t.Fatalf("create paused resume trigger: %v", err)
		}
		if _, err := runtime.workflowExecutor().resumeLoopWorkflow(ctx, session, parent); err == nil || !strings.Contains(err.Error(), "paused resume write rejected") {
			t.Fatalf("resume paused workflow error = %v", err)
		}
	})
}

func TestCoverage98WorkflowApprovalReconcileFailsClosedOnTaskAndRunPersistence(t *testing.T) {
	ctx := t.Context()

	t.Run("completed child does not become an unrecorded task completion", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "task-store")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-task-store-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "coverage98-missing-task", ChildRunID: "coverage98-task-store-child", Status: "IN_PROGRESS"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		mustSaveRun(t, runtime, Run{
			ID: "coverage98-task-store-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
			Status: RunStatusCompleted, Message: "child result", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableTasks); err != nil {
			t.Fatalf("drop task table: %v", err)
		}

		updated, blocked, err := runtime.workflowExecutor().reconcileWorkflowChildren(ctx, parent)
		if err != nil || blocked || len(updated.WorkflowPlan) != 1 || updated.WorkflowPlan[0].Status != "DONE" {
			t.Fatalf("completed child reconciliation = %+v blocked=%v err=%v", updated, blocked, err)
		}
	})

	for _, childStatus := range []string{RunStatusPending, RunStatusRunning} {
		t.Run("active child persistence failure "+childStatus, func(t *testing.T) {
			runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "active-"+strings.ToLower(childStatus))
			parent := mustSaveRun(t, runtime, Run{
				ID: "coverage98-active-parent-" + strings.ToLower(childStatus), SessionID: session.ID, AgentID: agent.ID,
				Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
				WorkflowPlan: []WorkflowStepState{{ChildRunID: "coverage98-active-child-" + strings.ToLower(childStatus), Status: "IN_PROGRESS"}},
				CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
			})
			mustSaveRun(t, runtime, Run{
				ID: "coverage98-active-child-" + strings.ToLower(childStatus), SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
				Status: childStatus, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
			})
			if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_active_parent_`+strings.ToLower(childStatus)+` BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN SELECT RAISE(FAIL, 'active child state write rejected'); END`); err != nil {
				t.Fatalf("create active child trigger: %v", err)
			}
			if _, _, err := runtime.workflowExecutor().reconcileWorkflowChildren(ctx, parent); err == nil || !strings.Contains(err.Error(), "active child state write rejected") {
				t.Fatalf("reconcile %s child error = %v", childStatus, err)
			}
		})
	}
}

func newCoverage98WorkflowApprovalFixture(t *testing.T, suffix string) (*Runtime, Agent, Session) {
	t.Helper()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-approval-agent-" + suffix, Name: "Coverage Approval " + suffix,
		ProviderID: testProviderID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	return runtime, agent, mustCreateSession(t, runtime, agent.ID, "coverage approval "+suffix)
}
