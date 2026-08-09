package workflowexec

import (
	"context"
	"strings"
	"testing"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowResumeAndCompletePersistenceFailures(t *testing.T) {
	ctx := context.Background()

	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	now := jfadkmodel.NowString()
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
	installFailTrigger(t, runtime, "fail_runs_update_resume_loop", enginepersistence.TableRuns, "UPDATE", "resume loop save failed")
	if _, err := executor.ResumeLoopWorkflow(ctx, session, parent); err == nil || !strings.Contains(err.Error(), "resume loop save failed") {
		t.Fatalf("resumeLoopWorkflow err = %v", err)
	}

	runtime2 := newTestRuntime(t)
	executor2 := (&WorkflowExecutor{runtime: runtime2})
	session2 := mustCreateSession(t, runtime2, "agent", "complete resumed failure")
	parent2 := mustSaveRun(t, runtime2, Run{
		ID:             "goal-parent-complete-fail",
		SessionID:      session2.ID,
		AgentID:        "agent",
		Status:         RunStatusRunning,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		CreatedAt:      jfadkmodel.NowString(),
		UpdatedAt:      jfadkmodel.NowString(),
		Usage:          &RunUsage{},
	})
	installFailTrigger(t, runtime2, "fail_runs_update_complete_resumed", enginepersistence.TableRuns, "UPDATE", "complete resumed save failed")
	if _, err := executor2.CompleteResumedWorkflow(ctx, session2, parent2, "done"); err == nil || !strings.Contains(err.Error(), "complete resumed save failed") {
		t.Fatalf("completeResumedWorkflow err = %v", err)
	}
}
