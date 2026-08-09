package adk

import (
	"strings"
	"testing"
)

func TestWorkflowApprovalRecoveryPreservesUserPauseAndContextFailures(t *testing.T) {
	ctx := t.Context()

	t.Run("a second pause write must report persistence failure", func(t *testing.T) {
		runtime, agent, session := newWorkflowApprovalFixture(t, "pause-write")
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-approval-pause-write-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			PauseRequestedAt: &pauseRequestedAt, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER coverage98_reject_second_pause_write BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+parent.ID+`' AND OLD.status = '`+RunStatusPaused+`' BEGIN SELECT RAISE(FAIL, 'second pause write rejected'); END`); err != nil {
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
		runtime, _, session := newWorkflowApprovalFixture(t, "resume-context")
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
}

func newWorkflowApprovalFixture(t *testing.T, suffix string) (*Runtime, Agent, Session) {
	t.Helper()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-approval-agent-" + suffix, Name: "Coverage Approval " + suffix,
		ProviderID: testProviderID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	return runtime, agent, mustCreateSession(t, runtime, agent.ID, "coverage approval "+suffix)
}
