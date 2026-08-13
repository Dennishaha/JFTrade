package workflowexec

import (
	"context"
	"errors"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"path/filepath"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	adksession "google.golang.org/adk/v2/session"
)

func newWorkflowApprovalFixture(t *testing.T, suffix string) (*jfadk.Runtime, Agent, Session) {
	t.Helper()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-approval-agent-" + suffix, Name: "Coverage Approval " + suffix,
		ProviderID: testProviderID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	return runtime, agent, mustCreateSession(t, runtime, agent.ID, "coverage approval "+suffix)
}

func TestGoalResumeSurfacesReconcileAndPersistenceFaults(t *testing.T) {
	ctx := context.Background()

	t.Run("reconciliation errors and user-paused children do not launch another model turn", func(t *testing.T) {
		runtime, agent, session := newWorkflowApprovalFixture(t, "resume-reconcile")
		brokenParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-reconcile-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{ChildRunID: "coverage98-resume-reconcile-child"}},
			CreatedAt:    assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableRuns); err != nil {
			t.Fatalf("drop runs table: %v", err)
		}
		if _, err := (&WorkflowExecutor{runtime: runtime}).ResumeLoopWorkflow(ctx, session, brokenParent); err == nil {
			t.Fatal("resumeLoopWorkflow swallowed a child-run lookup failure")
		}

		runtime, agent, session = newWorkflowApprovalFixture(t, "resume-paused")
		pausedAt := assistantmodel.NowString()
		pausedParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-blocked-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
			PausedAt: &pausedAt, PausedReason: "user", CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		resumed, err := (&WorkflowExecutor{runtime: runtime}).ResumeLoopWorkflow(ctx, session, pausedParent)
		if err != nil || resumed.Status != RunStatusPaused || resumed.PausedReason != "user" {
			t.Fatalf("already paused resume = %+v, %v", resumed, err)
		}
	})

	t.Run("resume exposes parent and task persistence failures", func(t *testing.T) {
		runtime, agent, session := newWorkflowApprovalFixture(t, "resume-save")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-save-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER coverage98_reject_resume_parent BEFORE UPDATE ON `+enginepersistence.TableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN SELECT RAISE(FAIL, 'resume parent write rejected'); END`); err != nil {
			t.Fatalf("create resume write trigger: %v", err)
		}
		if _, err := (&WorkflowExecutor{runtime: runtime}).ResumeADKGoalWorkflow(ctx, session, agent, parent); err == nil || !strings.Contains(err.Error(), "resume parent write rejected") {
			t.Fatalf("resume parent persistence error = %v", err)
		}

		runtime, agent, session = newWorkflowApprovalFixture(t, "resume-tasks")
		parent = mustSaveRun(t, runtime, Run{
			ID: "coverage98-resume-task-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableTasks); err != nil {
			t.Fatalf("drop tasks table: %v", err)
		}
		if _, err := (&WorkflowExecutor{runtime: runtime}).ResumeADKGoalWorkflow(ctx, session, agent, parent); err == nil {
			t.Fatal("resumeADKGoalWorkflow swallowed a task lookup failure")
		}
	})
}

func TestGoalDecisionErrorsAndTerminalFallbacksStayObservable(t *testing.T) {
	ctx := context.Background()

	t.Run("decision-phase model errors fail the parent without a second runner", func(t *testing.T) {
		runtime, agent, session := newWorkflowApprovalFixture(t, "decision-error")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-decision-error-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		execution := &fakeWorkflowExecutionHandle{runErr: errors.New("decision provider unavailable")}
		updated, _, _, done, response, _, err := (&WorkflowExecutor{runtime: runtime}).RunGoalWorkflowDecision(ctx, workflowRequest{Session: session}, parent, nil,
			execution, &workflowGoalDecision{}, parent, "visible reply", 1, false)
		if err != nil {
			t.Fatalf("run goal decision: %v", err)
		}
		if !done || updated.Status != RunStatusFailed || response.Run.Status != RunStatusFailed || !strings.Contains(response.Reply, "decision provider unavailable") {
			t.Fatalf("decision model failure = parent:%+v done:%v response:%+v", updated, done, response)
		}
	})

	t.Run("completion keeps a user pause and falls back when the assistant-message write fails", func(t *testing.T) {
		runtime, agent, session := newWorkflowApprovalFixture(t, "completion-pause")
		pauseRequestedAt := assistantmodel.NowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-completion-pause-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		updated, response, done, prompt, err := (&WorkflowExecutor{runtime: runtime}).FinishCompleteGoalWorkflow(ctx, workflowRequest{Session: session}, parent, nil,
			jfadk.AssistantExecutionResult{Reply: "complete reply"}, workflowGoalDecisionSnapshot{Summary: "complete reply"}, "complete reply", 1)
		if err != nil {
			t.Fatalf("finish paused completion: %v", err)
		}
		if !done || prompt != "" || updated.Status != RunStatusPaused || response.Run.Status != RunStatusPaused {
			t.Fatalf("completion pause = parent:%+v response:%+v done:%v prompt:%q", updated, response, done, prompt)
		}

		dir := t.TempDir()
		sessionService, err := enginepersistence.NewSQLiteSessionService(filepath.Join(dir, "adk-session.db"))
		if err != nil {
			t.Fatalf("NewSQLiteSessionService: %v", err)
		}
		if err := enginepersistence.ValidateSQLiteSessionService(sessionService); err != nil {
			t.Fatalf("ValidateSQLiteSessionService: %v", err)
		}
		service := &failAfterSessionService{Service: sessionService}
		runtime, agent, session = newRuntimeAndApprovalFixtureWithSessionService(t, "completion-message", service)
		parent = mustSaveRun(t, runtime, Run{
			ID: "coverage98-completion-message-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: assistantmodel.NowString(), UpdatedAt: assistantmodel.NowString(), Usage: &RunUsage{},
		})
		service.fail = true
		updated, response, done, prompt, err = (&WorkflowExecutor{runtime: runtime}).FinishCompleteGoalWorkflow(ctx, workflowRequest{Session: session}, parent, nil,
			jfadk.AssistantExecutionResult{Reply: "message fallback"}, workflowGoalDecisionSnapshot{Summary: "message fallback"}, "message fallback", 1)
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
		runtime, agent, session := newWorkflowApprovalFixture(t, "execution-engine")
		parent := Run{ID: "coverage98-engine-parent", SessionID: session.ID, AgentID: agent.ID, WorkMode: WorkModeLoop, Status: RunStatusRunning, Usage: &RunUsage{}}
		taskTools := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, parent.ID, "")
		taskTools.Req = workflowRequest{Mode: WorkModeLoop}
		execution, err := runtime.NewGoogleADKTaskExecution(ctx, agent, session, parent, workflowRequest{Mode: WorkModeLoop}, taskTools, nil)
		if err != nil || execution == nil || execution.RunID() != parent.ID {
			t.Fatalf("new goal execution = %+v, %v", execution, err)
		}
	})
}

func newRuntimeAndApprovalFixtureWithSessionService(t *testing.T, suffix string, service adksession.Service) (*jfadk.Runtime, Agent, Session) {
	t.Helper()
	runtime := newTestRuntimeWithSessionService(t, service)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-approval-agent-" + suffix, Name: "Coverage Approval " + suffix,
		ProviderID: testProviderID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	return runtime, agent, mustCreateSession(t, runtime, agent.ID, "coverage approval "+suffix)
}
