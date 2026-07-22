package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestCoverage98AnsweredInputFailuresRemainObservable(t *testing.T) {
	ctx := t.Context()

	t.Run("model resume and session history failures terminate the answered run", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			setup func(*googleADKExecution)
			want  string
		}{
			{
				name: "model resume failure",
				setup: func(execution *googleADKExecution) {
					execution.runBlocking = func(context.Context, *genai.Content) error {
						return errors.New("input continuation provider disconnected")
					}
				},
				want: "provider disconnected",
			},
			{
				name: "session history failure after model response",
				setup: func(execution *googleADKExecution) {
					execution.runBlocking = func(context.Context, *genai.Content) error { return nil }
					execution.sessionService = getErrorADKSessionService{err: errors.New("input session history is unavailable")}
				},
				want: "session history is unavailable",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runtime, agent, session, run := newCoverage98AnsweredInputRun(t, tc.name)
				execution := newBareGoogleADKExecution(run.ID)
				execution.sessionID = session.ID
				execution.appName = googleADKAppName(agent.ID)
				execution.agent = agent
				tc.setup(execution)
				runtime.adkRuns[run.ID] = execution

				if err := runtime.continueResolvedInput(ctx, run.ID); err != nil {
					t.Fatalf("continueResolvedInput: %v", err)
				}
				stored, ok, err := runtime.Store().Run(ctx, run.ID)
				if err != nil || !ok || stored.Status != RunStatusFailed || stored.ResumeState != "input_resume_failed" || !strings.Contains(stored.FailureReason, tc.want) {
					t.Fatalf("%s persisted run = %+v/%v/%v", tc.name, stored, ok, err)
				}
				if _, active := runtime.adkRuns[run.ID]; active {
					t.Fatalf("%s left a stale active execution", tc.name)
				}
			})
		}
	})

	t.Run("completion retains a truthful fallback and exposes message or run persistence failures", func(t *testing.T) {
		t.Run("empty visible model text receives the user-facing completion fallback", func(t *testing.T) {
			runtime, agent, session, run := newCoverage98AnsweredInputRun(t, "input-fallback")
			execution := newBareGoogleADKExecution(run.ID)
			execution.sessionID = session.ID
			execution.appName = googleADKAppName(agent.ID)
			execution.agent = agent

			if err := runtime.completeInputContinuation(ctx, run, execution); err != nil {
				t.Fatalf("completeInputContinuation: %v", err)
			}
			stored, ok, err := runtime.Store().Run(ctx, run.ID)
			if err != nil || !ok || stored.Status != RunStatusCompleted || stored.FinalMessageID == "" {
				t.Fatalf("fallback completion = %+v/%v/%v", stored, ok, err)
			}
			messages := mustAssistantMessages(t, runtime, session.ID)
			if len(messages) == 0 || !strings.Contains(messages[len(messages)-1].Content, "已根据你的选择继续执行") {
				t.Fatalf("fallback assistant messages = %+v", messages)
			}
		})

		t.Run("assistant event and terminal run write failures are returned to the caller", func(t *testing.T) {
			runtime, agent, session, run := newCoverage98AnsweredInputRun(t, "input-message-write")
			execution := newBareGoogleADKExecution(run.ID)
			execution.sessionID = session.ID
			execution.appName = googleADKAppName(agent.ID)
			execution.agent = agent
			runtime.rawSessionService = createErrorSessionService{err: errors.New("assistant input completion event rejected")}
			if err := runtime.completeInputContinuation(ctx, run, execution); err == nil || !strings.Contains(err.Error(), "assistant input completion event rejected") {
				t.Fatalf("assistant event write error = %v", err)
			}

			runtime, agent, session, run = newCoverage98AnsweredInputRun(t, "input-run-write")
			execution = newBareGoogleADKExecution(run.ID)
			execution.sessionID = session.ID
			execution.appName = googleADKAppName(agent.ID)
			execution.agent = agent
			if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_input_terminal_run BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+run.ID+`' BEGIN SELECT RAISE(FAIL, 'input terminal run write rejected'); END`); err != nil {
				t.Fatalf("create terminal run trigger: %v", err)
			}
			if err := runtime.completeInputContinuation(ctx, run, execution); err == nil || !strings.Contains(err.Error(), "input terminal run write rejected") {
				t.Fatalf("terminal run write error = %v", err)
			}
		})
	})
}

func TestCoverage98InputResolutionExposesParentProjectionWriteFailure(t *testing.T) {
	ctx := t.Context()
	runtime, agent, session, child := newCoverage98PendingInputRun(t, "input-parent-projection")
	parent := mustSaveRun(t, runtime, Run{
		ID: "coverage98-input-parent-projection", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "projection-child", ChildRunID: child.ID, Status: "IN_PROGRESS"}},
		CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child.ParentRunID = parent.ID
	if err := runtime.Store().SaveRun(ctx, child); err != nil {
		t.Fatalf("link child to parent: %v", err)
	}
	if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage98_reject_input_parent_projection BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+parent.ID+`' BEGIN SELECT RAISE(FAIL, 'input parent projection rejected'); END`); err != nil {
		t.Fatalf("create parent trigger: %v", err)
	}

	_, err := runtime.ResolveInputAsync(ctx, child.ID, InputResponseRequest{
		RequestID: child.InputRequest.ID,
		Answers:   []InputAnswer{{QuestionID: child.InputRequest.Questions[0].ID, OptionID: child.InputRequest.Questions[0].Options[0].ID}},
	})
	if err == nil || !strings.Contains(err.Error(), "input parent projection rejected") {
		t.Fatalf("ResolveInputAsync parent projection error = %v", err)
	}
	storedChild, ok, childErr := runtime.Store().Run(ctx, child.ID)
	if childErr != nil || !ok || storedChild.InputRequest == nil || storedChild.InputRequest.Status != InputRequestStatusAnswered {
		t.Fatalf("answered child must remain recoverable = %+v/%v/%v", storedChild, ok, childErr)
	}
}

func TestAnsweredInputCrashRecoveryHonorsDurableRunLease(t *testing.T) {
	resolveWithoutStartingContinuation := func(t *testing.T, runtime *Runtime, run Run, request InputRequest) {
		t.Helper()
		resolved, changed, err := runtime.Store().ResolveRunInput(t.Context(), run.ID, InputResponseRequest{
			RequestID: request.ID,
			Answers: []InputAnswer{
				{QuestionID: "q1", OptionID: "q1-o1"},
				{QuestionID: "q2", OptionID: "q2-o2"},
			},
		})
		if err != nil || !changed || !runHasRecoverableAnsweredInputContext(resolved) {
			t.Fatalf("persist crash-window answer = %+v, changed=%v err=%v", resolved, changed, err)
		}
	}

	t.Run("restart resumes an answer persisted before continuation starts", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "input-crash-recovery-agent", Name: "Input Crash Recovery", ProviderID: testProviderID,
			PermissionMode: PermissionModeAll, WorkMode: WorkModeChat, Status: AgentStatusEnabled,
		})
		response, err := runtime.Chat(t.Context(), ChatRequest{AgentID: agent.ID, Message: "@input.required recover"})
		if err != nil || response.InputRequest == nil {
			t.Fatalf("Chat response=%+v err=%v", response, err)
		}
		resolveWithoutStartingContinuation(t, runtime, response.Run, *response.InputRequest)

		restarted := newRuntimeWithRegistry(t, runtime.Store(), NewToolRegistry())
		completed := waitForRunStatus(t, restarted, response.Run.ID, RunStatusCompleted)
		if completed.ResumeState != "input_resolved" || completed.InputRequest == nil || completed.InputRequest.Status != InputRequestStatusAnswered {
			t.Fatalf("recovered input continuation = %+v", completed)
		}
	})

	t.Run("restart does not steal a fresh foreign continuation lease", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "input-foreign-lease-agent", Name: "Input Foreign Lease", ProviderID: testProviderID,
			PermissionMode: PermissionModeAll, WorkMode: WorkModeChat, Status: AgentStatusEnabled,
		})
		response, err := runtime.Chat(t.Context(), ChatRequest{AgentID: agent.ID, Message: "@input.required fenced"})
		if err != nil || response.InputRequest == nil {
			t.Fatalf("Chat response=%+v err=%v", response, err)
		}
		resolveWithoutStartingContinuation(t, runtime, response.Run, *response.InputRequest)
		lease, err := runtime.Store().ClaimRunLease(t.Context(), response.Run.ID, "executor-other-process", time.Now().UTC(), time.Minute)
		if err != nil {
			t.Fatalf("ClaimRunLease foreign: %v", err)
		}
		defer func() { _ = runtime.Store().ReleaseRunLease(context.Background(), lease) }()

		restarted := newRuntimeWithRegistry(t, runtime.Store(), NewToolRegistry())
		time.Sleep(100 * time.Millisecond)
		stored, ok, err := restarted.Store().Run(t.Context(), response.Run.ID)
		if err != nil || !ok || !runHasRecoverableAnsweredInputContext(stored) {
			t.Fatalf("foreign-owned answered run = %+v, ok=%v err=%v", stored, ok, err)
		}
		restarted.approvalMu.Lock()
		queued := len(restarted.inputRuns)
		restarted.approvalMu.Unlock()
		if queued != 0 {
			t.Fatalf("foreign-owned input continuation queued locally: %d", queued)
		}
	})
}

func newCoverage98AnsweredInputRun(t *testing.T, suffix string) (*Runtime, Agent, Session, Run) {
	t.Helper()
	runtime, agent, session, run := newCoverage98PendingInputRun(t, suffix)
	answeredAt := nowString()
	run.Status = RunStatusRunning
	run.ResumeState = "input_resuming"
	run.InputRequest.Status = InputRequestStatusAnswered
	run.InputRequest.Answers = []InputAnswer{{
		QuestionID: run.InputRequest.Questions[0].ID,
		OptionID:   run.InputRequest.Questions[0].Options[0].ID,
	}}
	run.InputRequest.AnsweredAt = &answeredAt
	run.InputRequest.UpdatedAt = answeredAt
	run.InputRequests = []InputRequest{*run.InputRequest}
	if err := runtime.Store().SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save answered input run: %v", err)
	}
	return runtime, agent, session, run
}

func newCoverage98PendingInputRun(t *testing.T, suffix string) (*Runtime, Agent, Session, Run) {
	t.Helper()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-input-" + strings.ReplaceAll(suffix, " ", "-"), Name: "Input Recovery", Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "input recovery "+suffix)
	request, err := buildInputRequest("coverage98-input-run-"+strings.ReplaceAll(suffix, " ", "-"), agent.ID, "coverage98-input-call-"+strings.ReplaceAll(suffix, " ", "-"), requestUserToolArgs{
		Questions: []requestUserToolQuestion{{
			Question: "Choose the execution mode", Options: []requestUserToolOption{{Label: "Conservative"}, {Label: "Active"}},
		}},
	})
	if err != nil {
		t.Fatalf("build input request: %v", err)
	}
	run := mustSaveRun(t, runtime, Run{
		ID: request.RunID, SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPendingInput,
		InputRequest: request, InputRequests: []InputRequest{*request}, UserMessage: "need a decision",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	return runtime, agent, session, run
}
