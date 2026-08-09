package workflowexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkworkflow "google.golang.org/adk/v2/workflow"
)

func TestGoalWorkflowStateBoundariesFailClosedAndRemainResumable(t *testing.T) {
	ctx := context.Background()

	t.Run("model bootstrap failures become a durable failed goal response", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "coverage98-goal-missing-provider", "goal bootstrap failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-bootstrap-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})

		response, err := (&WorkflowExecutor{runtime: runtime}).ContinueADKGoalWorkflow(ctx, workflowRequest{
			Agent:   Agent{ID: session.AgentID, Name: "Unavailable Model", ProviderID: "coverage98-missing-provider", Status: AgentStatusEnabled},
			Session: session, Mode: WorkModeLoop,
		}, parent, nil, "continue", 1, 1)
		if err != nil {
			t.Fatalf("ContinueADKGoalWorkflow returned transport error: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed || strings.TrimSpace(response.Reply) == "" {
			t.Fatalf("model bootstrap failure response = %+v", response)
		}
	})

	t.Run("invalid resume iteration is normalized into a durable iteration-limit pause", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "coverage98-goal-iteration-agent", Name: "Goal Iteration Boundary", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal iteration boundary")
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-iteration-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})

		response, err := (&WorkflowExecutor{runtime: runtime}).ContinueADKGoalWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Mode: WorkModeLoop,
		}, parent, nil, "continue", 0, 0)
		if err != nil {
			t.Fatalf("ContinueADKGoalWorkflow iteration boundary: %v", err)
		}
		if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "iteration_limit" || response.Run.PausedReason != "iteration_limit" || response.Run.WorkflowEngine != WorkflowEngineADK2Loop {
			t.Fatalf("iteration-limit response = %+v", response.Run)
		}
	})

	t.Run("missing final reply asks for a reply while a user pause wins immediately", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "coverage98-goal-decision-agent", Name: "Goal Decision Boundary", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal decision boundary")
		executor := (&WorkflowExecutor{runtime: runtime})
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-no-final-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})

		executionWithoutPostToolReply := &fakeWorkflowExecutionHandle{calls: []ToolCall{{ID: "coverage98-finished-tool", RunID: parent.ID, ToolName: workflowTasksListTool, Status: "SUCCEEDED"}}}
		updated, _, snapshot, done, response, prompt, err := executor.ResolveGoalWorkflowDecision(ctx, workflowRequest{Session: session}, parent, nil,
			executionWithoutPostToolReply, &workflowGoalDecision{}, jfadk.AssistantExecutionResult{}, "visible progress", "", 1, false)
		if err != nil {
			t.Fatalf("resolve missing final reply: %v", err)
		}
		if done || snapshot.Status != "" || response.Run.ID != "" || !strings.Contains(prompt, "最终可见答复") || updated.ID != parent.ID {
			t.Fatalf("missing final reply resolution = parent:%+v decision:%+v done:%v response:%+v prompt:%q", updated, snapshot, done, response, prompt)
		}

		pauseRequestedAt := jfadkmodel.NowString()
		pausedParent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-pause-decision-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		updated, _, _, done, response, prompt, err = executor.ResolveGoalWorkflowDecision(ctx, workflowRequest{Session: session}, pausedParent, nil,
			&fakeWorkflowExecutionHandle{}, &workflowGoalDecision{}, jfadk.AssistantExecutionResult{}, "progress before pause", "", 1, false)
		if err != nil {
			t.Fatalf("resolve pause-first decision: %v", err)
		}
		if !done || prompt != "" || updated.Status != RunStatusPaused || response.Run.Status != RunStatusPaused || response.Reply != "progress before pause" {
			t.Fatalf("pause-first resolution = parent:%+v done:%v response:%+v prompt:%q", updated, done, response, prompt)
		}

		unchanged, replyResult, pauseDone, pauseResponse, err := executor.PauseAfterMissingGoalDecision(ctx, workflowRequest{Session: session}, parent,
			jfadk.AssistantExecutionResult{}, "visible fallback", workflowGoalDecisionSnapshot{}, 1)
		if err != nil {
			t.Fatalf("pause after missing decision: %v", err)
		}
		if pauseDone || pauseResponse.Run.ID != "" || replyResult.Reply != "" || unchanged.ID != parent.ID {
			t.Fatalf("missing-decision fallback = parent:%+v reply:%+v done:%v response:%+v", unchanged, replyResult, pauseDone, pauseResponse)
		}
	})

	t.Run("continuation honors a concurrent pause and interrupted call filtering only removes internal calls", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "coverage98-goal-continue-agent", Name: "Goal Continue Boundary", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal continue boundary")
		pauseRequestedAt := jfadkmodel.NowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage98-goal-continue-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		continued, response, paused, prompt, err := (&WorkflowExecutor{runtime: runtime}).FinishContinueGoalWorkflow(ctx, workflowRequest{Session: session}, parent,
			jfadk.AssistantExecutionResult{}, workflowGoalDecisionSnapshot{Reason: "await review"}, "", 2)
		if err != nil {
			t.Fatalf("finish continued goal: %v", err)
		}
		if !paused || prompt != "" || continued.Status != RunStatusPaused || response.Run.Status != RunStatusPaused {
			t.Fatalf("continue while pausing = parent:%+v response:%+v paused:%v prompt:%q", continued, response, paused, prompt)
		}

		pauseErr := jfadkmodel.ErrUserGoalPauseRequested.Error()
		if jfadkmodel.InterruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: workflowTaskClaimTool, Status: "FAILED"}) {
			t.Fatal("failed workflow call without an interruption error must remain visible")
		}
		if jfadkmodel.InterruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: workflowTaskClaimTool, Status: "FAILED", Error: new("ordinary failure")}) {
			t.Fatal("ordinary workflow failure must remain visible")
		}
		if jfadkmodel.InterruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: "market.snapshot", Status: "FAILED", Error: &pauseErr}) {
			t.Fatal("non-workflow interruption must remain visible")
		}
		if !jfadkmodel.InterruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: workflowTaskClaimTool, Status: "FAILED", Error: &pauseErr}) {
			t.Fatal("goal pause sentinel must hide the interrupted workflow call")
		}
		interrupted := fmt.Sprintf("workflow child: %s", adkworkflow.ErrNodeInterrupted)
		if !jfadkmodel.InterruptedGoalWorkflowToolCall(parent, ToolCall{ToolName: workflowTaskClaimTool, Status: "FAILED", Error: &interrupted}) {
			t.Fatal("GO-ADK interruption sentinel must hide the interrupted workflow call")
		}
		if !errors.Is(jfadkmodel.ErrorFromSerializedADKText(interrupted), adkworkflow.ErrNodeInterrupted) {
			t.Fatal("persisted GO-ADK interruption must restore sentinel identity")
		}
	})
}

func TestWorkflowResumeReconcilerPausesOnRequestAndExposesStoreFailures(t *testing.T) {
	ctx := context.Background()
	runtime, agent, session := newWorkflowApprovalFixture(t, "resume-and-store")
	pauseRequestedAt := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "coverage98-resume-requested-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &pauseRequestedAt,
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	resumed, err := (&WorkflowExecutor{runtime: runtime}).ResumeLoopWorkflow(ctx, session, parent)
	if err != nil || resumed.Status != RunStatusPaused || resumed.PausedReason != "user" {
		t.Fatalf("resume requested pause = %+v, %v", resumed, err)
	}

	brokenParent := mustSaveRun(t, runtime, Run{
		ID: "coverage98-reconcile-store-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{ChildRunID: "coverage98-reconcile-store-child"}},
		CreatedAt:    jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableRuns); err != nil {
		t.Fatalf("drop run table: %v", err)
	}
	if _, _, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(ctx, brokenParent); err == nil {
		t.Fatal("child run storage failure was swallowed")
	}
}
