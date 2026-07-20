package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/genai"
)

func TestCoverage98GoalResumeSurfacesReconcileAndPersistenceFaults(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciliation errors and user-paused children do not launch another model turn", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "resume-reconcile")
		brokenParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-reconcile-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{ChildRunID: "coverage98-resume-reconcile-child"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := runtime.workflowExecutor().resumeLoopWorkflow(ctx, session, brokenParent); err == nil {
			t.Fatal("resumeLoopWorkflow swallowed a child-run lookup failure")
		}

		runtime, agent, session = newCoverage98WorkflowApprovalFixture(t, "resume-paused")
		pausedAt := nowString()
		pausedParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-blocked-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
			PausedAt: &pausedAt, PausedReason: "user", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		resumed, err := runtime.workflowExecutor().resumeLoopWorkflow(ctx, session, pausedParent)
		if err != nil || resumed.Status != RunStatusPaused || resumed.PausedReason != "user" {
			t.Fatalf("already paused resume = %+v, %v", resumed, err)
		}
	})

	t.Run("resume exposes parent and task persistence failures", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "resume-save")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-save-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_resume_parent BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN SELECT RAISE(FAIL, 'resume parent write rejected'); END`); err != nil {
			t.Fatalf("create resume write trigger: %v", err)
		}
		if _, err := runtime.workflowExecutor().resumeADKGoalWorkflow(ctx, session, agent, parent); err == nil || !strings.Contains(err.Error(), "resume parent write rejected") {
			t.Fatalf("resume parent persistence error = %v", err)
		}

		runtime, agent, session = newCoverage98WorkflowApprovalFixture(t, "resume-tasks")
		parent = mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-task-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableTasks); err != nil {
			t.Fatalf("drop tasks table: %v", err)
		}
		if _, err := runtime.workflowExecutor().resumeADKGoalWorkflow(ctx, session, agent, parent); err == nil {
			t.Fatal("resumeADKGoalWorkflow swallowed a task lookup failure")
		}
	})
}

func TestCoverage98GoalDecisionErrorsAndTerminalFallbacksStayObservable(t *testing.T) {
	ctx := context.Background()

	t.Run("decision-phase model errors fail the parent without a second runner", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "decision-error")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-decision-error-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		execution := newBareGoogleADKExecution(parent.ID)
		execution.runBlocking = func(context.Context, *genai.Content) error { return errors.New("decision provider unavailable") }
		updated, _, _, done, response, _, err := runtime.workflowExecutor().runGoalWorkflowDecision(ctx, workflowRequest{Session: session}, parent, nil,
			execution, &workflowGoalDecision{}, parent, "visible reply", 1, false)
		if err != nil {
			t.Fatalf("run goal decision: %v", err)
		}
		if !done || updated.Status != RunStatusFailed || response.Run.Status != RunStatusFailed || !strings.Contains(response.Reply, "decision provider unavailable") {
			t.Fatalf("decision model failure = parent:%+v done:%v response:%+v", updated, done, response)
		}
	})

	t.Run("completion keeps a user pause and falls back when the assistant-message write fails", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "completion-pause")
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-completion-pause-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		updated, response, done, prompt, err := runtime.workflowExecutor().finishCompleteGoalWorkflow(ctx, workflowRequest{Session: session}, parent, nil,
			openAIChatResult{Reply: "complete reply"}, workflowGoalDecisionSnapshot{summary: "complete reply"}, "complete reply", 1)
		if err != nil {
			t.Fatalf("finish paused completion: %v", err)
		}
		if !done || prompt != "" || updated.Status != RunStatusPaused || response.Run.Status != RunStatusPaused {
			t.Fatalf("completion pause = parent:%+v response:%+v done:%v prompt:%q", updated, response, done, prompt)
		}

		runtime, agent, session = newCoverage98WorkflowApprovalFixture(t, "completion-message")
		runtime.rawSessionService = createErrorSessionService{err: errors.New("assistant message storage unavailable")}
		parent = mustSaveRun(t, runtime, Run{
			ID: "coverage98-completion-message-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		updated, response, done, prompt, err = runtime.workflowExecutor().finishCompleteGoalWorkflow(ctx, workflowRequest{Session: session}, parent, nil,
			openAIChatResult{Reply: "message fallback"}, workflowGoalDecisionSnapshot{summary: "message fallback"}, "message fallback", 1)
		if err != nil {
			t.Fatalf("finish completion fallback: %v", err)
		}
		if !done || prompt != "" || updated.Status != RunStatusCompleted || updated.FinalMessageID != "" || response.Run.FinalMessageID != "" {
			t.Fatalf("completion message fallback = parent:%+v response:%+v done:%v prompt:%q", updated, response, done, prompt)
		}
		stored, ok, err := runtime.Store().Run(ctx, parent.ID)
		if err != nil || !ok || stored.Status != RunStatusCompleted || stored.FinalMessageID != "" {
			t.Fatalf("fallback completion storage = %+v/%v/%v", stored, ok, err)
		}
	})

	t.Run("goal execution construction supplies a default engine before attaching a runner", func(t *testing.T) {
		runtime, agent, session := newCoverage98WorkflowApprovalFixture(t, "execution-engine")
		parent := Run{ID: "coverage98-engine-parent", SessionID: session.ID, AgentID: agent.ID, WorkMode: WorkModeLoop, Status: RunStatusRunning, Usage: &RunUsage{}}
		execution, err := runtime.newGoogleADKTaskExecution(ctx, agent, session, parent, workflowRequest{Mode: WorkModeLoop}, nil)
		if err != nil || execution == nil || execution.runID != parent.ID {
			t.Fatalf("new goal execution = %+v, %v", execution, err)
		}
	})
}
