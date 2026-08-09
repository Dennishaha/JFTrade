package workflowexec

import (
	"context"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestGoalWorkflowPauseRequestBeforeNextTurnDoesNotCallModel(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-pause-before-next-turn-provider", func(req providers.OpenAIChatRequest) providers.OpenAIChatMessage {
		t.Fatalf("provider called after pause request: last user=%q", lastUserMessage(req))
		return providers.OpenAIChatMessage{}
	})
	agent := mustSaveAgent(t, runtime, jfadk.AgentWriteRequest{
		ID: "goal-pause-before-next-turn-agent", Name: "Goal Pause Before Next Turn", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop, LoopMaxIterations: 3,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause before next turn")
	now := jfadkmodel.NowString()
	run := mustSaveRun(t, runtime, Run{
		ID: "run-goal-pause-before-next-turn", SessionID: session.ID, AgentID: agent.ID, ProviderID: providerID,
		Status: RunStatusRunning, Message: "goal continues", UserMessage: "继续目标", WorkMode: WorkModeLoop,
		Objective: "继续目标", Iteration: 1, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &now, CreatedAt: now, StartedAt: now, UpdatedAt: now,
		ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		Title: "继续目标", Status: "DONE", AgentID: agent.ID, RunID: run.ID,
		Order: 1, ModeHint: WorkModeLoop, PlanSource: workflowPlanSourceRuntime, WorkflowMode: WorkModeLoop,
		Objective: run.Objective, Message: run.UserMessage,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	run.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks([]Task{task}, run.WorkflowPlan)
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun with plan: %v", err)
	}
	response, err := (&WorkflowExecutor{runtime: runtime}).ContinueADKGoalWorkflow(ctx, workflowRequest{
		Agent: agent, Session: session, Message: run.UserMessage, Mode: WorkModeLoop, Objective: run.Objective,
		RunOptions: RunOptions{LoopMaxIterations: 3},
	}, run, []Task{task}, jfadkmodel.GoalOrchestratorContinueNudge(run, "继续推进。"), 2, 3)
	if err != nil {
		t.Fatalf("ContinueADKGoalWorkflow: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "user_paused" || response.Run.PausedReason != "user" {
		t.Fatalf("run = %+v, want user-paused without another model call", response.Run)
	}
}

func TestWorkflowResponseUsesAuthoritativePauseRequestedParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, jfadk.AgentWriteRequest{
		ID: "goal-response-pause-agent", Name: "Goal Response Pause", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause response")
	now := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-goal-response-pause-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, Message: "目标将在当前轮结束后暂停。", UserMessage: "推进目标", WorkMode: WorkModeLoop,
		Objective: "推进目标", WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &now, ResumeState: "user_pause_requested",
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	stale := parent
	stale.Message = "goal running"
	stale.PauseRequestedAt = nil
	stale.ResumeState = ""

	response := (&WorkflowExecutor{runtime: runtime}).WorkflowResponse(
		ctx,
		session,
		stale,
		jfadk.AssistantExecutionResult{Reply: "still running"},
	)

	if response.Run.PauseRequestedAt == nil || response.Run.ResumeState != "user_pause_requested" {
		t.Fatalf("response run = %+v, want authoritative pause request fields", response.Run)
	}
}

func lastUserMessage(req providers.OpenAIChatRequest) string {
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if req.Messages[index].Role == "user" {
			return req.Messages[index].Content
		}
	}
	return ""
}
