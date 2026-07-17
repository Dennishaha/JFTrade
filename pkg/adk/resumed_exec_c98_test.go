package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func TestCoverage98ResumedExecutionFailurePersistenceIsObservable(t *testing.T) {
	ctx := t.Context()

	newApprovalRun := func(t *testing.T, runtime *Runtime, id string) (Agent, Session, Run, *googleADKExecution) {
		t.Helper()
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: id + "-agent", Name: "Approval Recovery", Status: AgentStatusEnabled, WorkMode: WorkModeChat,
		})
		session := mustCreateSession(t, runtime, agent.ID, "approval recovery "+id)
		run := mustSaveRun(t, runtime, Run{
			ID: id, SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPending, UserMessage: "resume confirmed action",
			PendingApprovals: []Approval{{
				ID: id + "-approval", RunID: id, AgentID: agent.ID, ToolName: "trade.submit", Status: ApprovalStatusApproved,
				FunctionCallID: id + "-call", ConfirmationCallID: id + "-confirmation", CreatedAt: nowString(), UpdatedAt: nowString(),
			}},
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		execution := newBareGoogleADKExecution(run.ID)
		execution.sessionID = session.ID
		execution.agent = agent
		return agent, session, run, execution
	}

	t.Run("interim blank messages and fallback approval summaries preserve truthful state", func(t *testing.T) {
		runtime := newTestRuntime(t)
		_, _, run, execution := newApprovalRun(t, runtime, "coverage98-resume-blank")
		if updated := persistResumedApprovalMessage(ctx, runtime, run, execution); updated.FinalMessageID != "" {
			t.Fatalf("blank interim message created final ID: %+v", updated)
		}
		approved := finalizeResumedResult(run, openAIChatResult{}, false)
		if strings.TrimSpace(approved.Reply) == "" {
			t.Fatal("approved resume must retain a visible fallback confirmation summary")
		}
		deniedRun := run
		deniedRun.PendingApprovals[0].Status = ApprovalStatusDenied
		denied := finalizeResumedResult(deniedRun, openAIChatResult{Reply: "stale", ReasoningContent: "private"}, true)
		if !strings.Contains(denied.Reply, "拒绝") || denied.ReasoningContent != "" {
			t.Fatalf("denied finalization = %+v", denied)
		}
	})

	t.Run("new approval lookup and final run writes propagate storage failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		_, _, run, execution := newApprovalRun(t, runtime, "coverage98-resume-pending-read")
		execution.sessionService = getErrorADKSessionService{Service: adksession.InMemoryService(), err: errors.New("approval session read failed")}
		if _, waiting, err := runtime.handleResumedApprovals(ctx, run, execution); err == nil || waiting || !strings.Contains(err.Error(), "approval session read failed") {
			t.Fatalf("handleResumedApprovals read failure = waiting:%v err:%v", waiting, err)
		}

		runtime = newTestRuntime(t)
		_, _, run, execution = newApprovalRun(t, runtime, "coverage98-resume-direct-save")
		execution.reply.WriteString("the confirmed action completed")
		if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_fail_direct_resumed_save BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+run.ID+`' BEGIN SELECT RAISE(FAIL, 'direct resumed save failed'); END`); err != nil {
			t.Fatalf("create direct resume trigger: %v", err)
		}
		updated, message, handled, err := runtime.completeDirectResumedExecution(ctx, run, execution)
		if err == nil || !handled || message != nil || !strings.Contains(err.Error(), "direct resumed save failed") {
			t.Fatalf("direct resumed save failure = run:%+v message:%+v handled:%v err:%v", updated, message, handled, err)
		}
	})

	t.Run("terminal failure does not disappear when its persistence fails", func(t *testing.T) {
		runtime := newTestRuntime(t)
		_, _, run, execution := newApprovalRun(t, runtime, "coverage98-resume-terminal-save")
		runtime.adkRuns[run.ID] = execution
		if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_fail_resumed_terminal_save BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+run.ID+`' BEGIN SELECT RAISE(FAIL, 'terminal resumed save failed'); END`); err != nil {
			t.Fatalf("create terminal resume trigger: %v", err)
		}
		updated, message, handled, err := runtime.failResumedExecution(ctx, run, execution, errors.New("provider resume failed"))
		if err == nil || !handled || message != nil || !strings.Contains(err.Error(), "terminal resumed save failed") || updated.Status != RunStatusFailed {
			t.Fatalf("terminal persistence failure = run:%+v message:%+v handled:%v err:%v", updated, message, handled, err)
		}
		if _, stillTracked := runtime.adkRuns[run.ID]; !stillTracked {
			t.Fatal("execution must remain recoverable when terminal persistence fails")
		}
	})
}

func TestCoverage98DirectResumeSurfacesRehydrationFailure(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-direct-rehydrate-agent", Name: "Direct Rehydrate", Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	run := Run{
		ID: "coverage98-direct-rehydrate-missing-session", SessionID: "coverage98-session-missing", AgentID: agent.ID, Status: RunStatusPending,
		PendingApprovals: []Approval{{
			ID: "coverage98-direct-rehydrate-approval", ToolName: "trade.submit", Status: ApprovalStatusApproved,
			FunctionCallID: "coverage98-direct-rehydrate-call", ConfirmationCallID: "coverage98-direct-rehydrate-confirmation",
		}},
	}
	updated, message, handled, err := runtime.resumeGoogleADKDirect(ctx, run)
	if err == nil || !handled || message != nil || updated.ID != run.ID || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("direct resume rehydration failure = run:%+v message:%+v handled:%v err:%v", updated, message, handled, err)
	}
}

func TestCoverage98ChildApprovalResumeFallsBackToDirectRecovery(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-direct-fallback-agent", Name: "Direct Fallback", Status: AgentStatusEnabled,
		WorkMode: WorkModeChat, PermissionMode: PermissionModeAll,
	})
	session := mustCreateSession(t, runtime, agent.ID, "direct fallback after workflow restart")
	run := mustSaveRun(t, runtime, Run{
		ID: "coverage98-direct-fallback-run", SessionID: session.ID, AgentID: agent.ID,
		ParentRunID: "coverage98-parent-workflow", ProviderID: testProviderID, Model: "test-model",
		PermissionMode: PermissionModeAll, Status: RunStatusPending, ResumeState: "approval_resuming",
		UserMessage: "resume the completed child action",
		PendingApprovals: []Approval{{
			ID: "coverage98-direct-fallback-approval", RunID: "coverage98-direct-fallback-run", AgentID: agent.ID,
			ToolName: "market.snapshot", Status: ApprovalStatusApproved,
			FunctionCallID: "coverage98-direct-fallback-call", ConfirmationCallID: "coverage98-direct-fallback-confirmation",
			Input: map[string]any{"symbol": "US.AAPL"}, CreatedAt: nowString(), UpdatedAt: nowString(),
		}},
		ToolCalls: []ToolCall{{
			ID: "coverage98-direct-fallback-call", RunID: "coverage98-direct-fallback-run", ToolName: "market.snapshot", Status: "SUCCEEDED",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})

	// A workflow child can outlive the in-memory graph after a process restart.
	// The graph runner reports that it has nothing to resume; the persisted child
	// still has enough terminal tool state for the direct recovery path to finish
	// it truthfully instead of leaving it permanently pending.
	live := newBareGoogleADKExecution(run.ID)
	live.sessionID = session.ID
	live.appName = googleADKAppName(agent.ID)
	live.agent = agent
	live.runBlocking = func(_ context.Context, _ *genai.Content) error {
		return adkworkflow.ErrNothingToResume
	}
	runtime.adkRuns[run.ID] = live

	updated, message, handled, err := runtime.resumeGoogleADK(ctx, run)
	if err != nil || !handled || message == nil || updated.Status != RunStatusCompleted || updated.FinalMessageID == "" {
		t.Fatalf("direct fallback resume = run:%+v message:%+v handled:%v err:%v", updated, message, handled, err)
	}
	if _, stillLive := runtime.adkRuns[run.ID]; stillLive {
		t.Fatal("completed direct recovery must remove the stale in-memory graph execution")
	}
	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok || stored.Status != RunStatusCompleted || stored.FinalMessageID != message.ID {
		t.Fatalf("stored direct fallback run = %+v ok=%v err=%v", stored, ok, err)
	}
}
