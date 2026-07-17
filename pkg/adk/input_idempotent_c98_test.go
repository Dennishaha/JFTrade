package adk

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestCoverage98ResolveInputAsyncDoesNotRestartAnInFlightContinuation(t *testing.T) {
	ctx := t.Context()
	runtime, agent, session, child := newCoverage98PendingInputRun(t, "input-idempotent-retry")
	parent := mustSaveRun(t, runtime, Run{
		ID:             "coverage98-input-idempotent-parent",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusRunning,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "coverage98-input-idempotent-task", ChildRunID: child.ID, Status: "IN_PROGRESS",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child.ParentRunID = parent.ID
	if err := runtime.Store().SaveRun(ctx, child); err != nil {
		t.Fatalf("link input child to parent: %v", err)
	}

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var continuations atomic.Int32
	execution := newBareGoogleADKExecution(child.ID)
	execution.sessionID = session.ID
	execution.appName = googleADKAppName(agent.ID)
	execution.agent = agent
	execution.runBlocking = func(context.Context, *genai.Content) error {
		continuations.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return errors.New("input continuation released for test")
	}
	runtime.adkRuns[child.ID] = execution

	payload := InputResponseRequest{
		RequestID: child.InputRequest.ID,
		Answers: []InputAnswer{{
			QuestionID: child.InputRequest.Questions[0].ID,
			OptionID:   child.InputRequest.Questions[0].Options[0].ID,
		}},
	}
	first, err := runtime.ResolveInputAsync(ctx, child.ID, payload)
	if err != nil || first.Run == nil || first.ParentRun == nil || first.ParentRun.ID != parent.ID {
		t.Fatalf("first ResolveInputAsync = %+v, err=%v; want linked parent projection", first, err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("input continuation did not start")
	}

	// A browser retry can arrive while the prior answer is already being resumed.
	// It must return the stored answer and workflow projection, rather than
	// starting a second model continuation for the same request.
	second, err := runtime.ResolveInputAsync(ctx, child.ID, payload)
	if err != nil || second.Run == nil || second.Request.Status != InputRequestStatusAnswered || second.ParentRun == nil || second.ParentRun.ID != parent.ID {
		t.Fatalf("idempotent ResolveInputAsync = %+v, err=%v", second, err)
	}
	if got := continuations.Load(); got != 1 {
		t.Fatalf("in-flight answer started %d continuations, want exactly one", got)
	}

	close(release)
	failed := waitForRunStatus(t, runtime, child.ID, RunStatusFailed)
	if failed.ResumeState != "input_resume_failed" || !strings.Contains(failed.FailureReason, "released for test") {
		t.Fatalf("released continuation must settle truthfully: %+v", failed)
	}
}
