package adk

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopWorkflowCanBeSelectedPerRun(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "loop-agent", Name: "Loop", Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "完成一次目标推进",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 3},
	})
	if err != nil {
		t.Fatalf("Chat loop workflow: %v", err)
	}
	if response.Run.WorkMode != WorkModeLoop || response.Run.Iteration != 1 || response.Run.Status != RunStatusCompleted {
		t.Fatalf("parent run = %+v, want completed loop workflow at iteration 1", response.Run)
	}
	if response.Run.Objective != "完成一次目标推进" {
		t.Fatalf("objective = %q", response.Run.Objective)
	}
	if response.Run.WorkflowEngine != WorkflowEngineADK2Loop {
		t.Fatalf("workflow engine = %q, want %q", response.Run.WorkflowEngine, WorkflowEngineADK2Loop)
	}
	if len(response.Run.WorkflowPlan) != 1 || response.Run.WorkflowPlan[0].PlanSource != workflowPlanSourceRuntime {
		t.Fatalf("workflow plan = %+v, want runtime initial goal task", response.Run.WorkflowPlan)
	}
	if step := response.Run.WorkflowPlan[0]; step.Title == response.Run.UserMessage || step.Message == response.Run.UserMessage || step.Objective != "" {
		t.Fatalf("runtime goal plan copied root user request: %+v", step)
	}
	for _, call := range response.Run.ToolCalls {
		if strings.HasPrefix(call.ToolName, "workflow.plan.") {
			t.Fatalf("tool calls = %+v, loop goal mode must not call planner tools", response.Run.ToolCalls)
		}
	}
	if !runHasToolCall(response.Run, workflowGoalCompleteTool) {
		t.Fatalf("tool calls = %+v, want goal completion decision", response.Run.ToolCalls)
	}
}

func TestGoalWorkflowMissingDecisionSafelyContinuesUntilPaused(t *testing.T) {
	runtime := newTestRuntime(t)
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-no-decision-provider", func(req openAIChatRequest) openAIChatMessage {
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "目标推进了一步，但没有裁决。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-no-decision-agent", Name: "Goal No Decision", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "推进一个必须裁决的目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 2},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow missing decision: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "iteration_limit" {
		t.Fatalf("run = %+v, want resumable iteration-limit pause", response.Run)
	}
	if runHasToolCall(response.Run, workflowGoalCompleteTool) {
		t.Fatalf("run = %+v, missing decision must not complete the goal", response.Run)
	}
}

func TestGoalWorkflowContinueRespectsMaxIterations(t *testing.T) {
	runtime := newTestRuntime(t)
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-continue-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "是否完成目标") && containsTool(testProviderToolNames(req), workflowGoalContinueTool) {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if !seen[workflowGoalContinueTool] {
				return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
					testProviderToolCall("call-goal-continue", workflowGoalContinueTool, map[string]any{
						"reason": "还需要继续推进。",
					}),
				}}
			}
			return openAIChatMessage{Role: "assistant", Content: "继续推进中。"}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "本轮推进完成，等待裁决。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-continue-agent", Name: "Goal Continue", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "持续推进直到超过上限",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 2},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow continue: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "iteration_limit" || response.Run.ErrorCode != "" {
		t.Fatalf("run = %+v, want resumable non-error pause", response.Run)
	}
}

func TestGoalWorkflowPauseAfterContinueAndResume(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var mu sync.Mutex
	pauseRequested := false
	resumeNudgeSeen := false
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-pause-resume-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "用户继续运行目标") {
			mu.Lock()
			resumeNudgeSeen = true
			mu.Unlock()
		}
		if strings.Contains(lastUser, "是否完成目标") && containsTool(testProviderToolNames(req), workflowGoalContinueTool) {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if seen[workflowGoalContinueTool] || seen[workflowGoalCompleteTool] {
				return openAIChatMessage{Role: "assistant", Content: "目标裁决已记录。"}
			}
			mu.Lock()
			shouldPause := !pauseRequested
			if shouldPause {
				pauseRequested = true
			}
			shouldComplete := resumeNudgeSeen
			mu.Unlock()
			if shouldPause {
				runs, jftradeErr1 := runtime.Store().ListRuns(ctx)
				jftradeCheckTestError(t, jftradeErr1)
				for _, run := range runs {
					if normalizeWorkMode(run.WorkMode) == WorkModeLoop && run.ParentRunID == "" && run.Status == RunStatusRunning {
						_, jftradeErr2 := runtime.PauseGoalRun(ctx, run.ID)
						jftradeCheckTestError(t, jftradeErr2)
						break
					}
				}
				return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
					testProviderToolCall("call-goal-continue", workflowGoalContinueTool, map[string]any{
						"reason": "用户请求本轮后暂停。",
					}),
				}}
			}
			if shouldComplete {
				return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
					testProviderToolCall("call-goal-complete", workflowGoalCompleteTool, map[string]any{
						"summary": "恢复后目标完成。",
					}),
				}}
			}
			return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
				testProviderToolCall("call-goal-continue-after-resume", workflowGoalContinueTool, map[string]any{
					"reason": "继续推进。",
				}),
			}}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "本轮推进完成。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-resume-agent", Name: "Goal Pause Resume", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat, LoopMaxIterations: 3,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "推进一个可暂停目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 3},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow pause: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.WorkflowStatus != workflowStatusPaused || response.Run.ResumeState != "user_paused" {
		t.Fatalf("paused run = %+v, want user-paused goal", response.Run)
	}
	if response.Run.PauseRequestedAt == nil || response.Run.PausedAt == nil || response.Run.PausedReason != "user" {
		t.Fatalf("paused fields = pauseRequestedAt:%v pausedAt:%v pausedReason:%q", response.Run.PauseRequestedAt, response.Run.PausedAt, response.Run.PausedReason)
	}
	resumed, err := runtime.ResumeGoalRun(ctx, response.Run.ID)
	if err != nil {
		t.Fatalf("ResumeGoalRun: %v", err)
	}
	if resumed.Status != RunStatusRunning || resumed.PauseRequestedAt != nil || resumed.PausedAt != nil || resumed.PausedReason != "" {
		t.Fatalf("resumed run = %+v, want running with pause fields cleared", resumed)
	}
	completed := waitForRunStatus(t, runtime, response.Run.ID, RunStatusCompleted)
	if completed.Message != "goal completed" {
		t.Fatalf("completed run = %+v, want goal completed", completed)
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.runExecutionInFlight(response.Run.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if runtime.runExecutionInFlight(response.Run.ID) {
		t.Fatal("resumed goal execution did not leave the runtime before test cleanup")
	}
	mu.Lock()
	defer mu.Unlock()
	if !resumeNudgeSeen {
		t.Fatal("resume nudge was not sent to the existing goal run")
	}
}

func TestGoalWorkflowPauseRequestedBeforeCompleteDecisionPausesInsteadOfCompleting(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var pauseRequested atomic.Bool
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-pause-before-complete-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "是否完成目标") && containsTool(testProviderToolNames(req), workflowGoalCompleteTool) {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if seen[workflowGoalCompleteTool] {
				return openAIChatMessage{Role: "assistant", Content: "goal.complete 已被中断。"}
			}
			if !pauseRequested.Swap(true) {
				runs, jftradeErr3 := runtime.Store().ListRuns(ctx)
				jftradeCheckTestError(t, jftradeErr3)
				for _, run := range runs {
					if normalizeWorkMode(run.WorkMode) == WorkModeLoop && run.ParentRunID == "" && run.Status == RunStatusRunning {
						_, jftradeErr4 := runtime.PauseGoalRun(ctx, run.ID)
						jftradeCheckTestError(t, jftradeErr4)
						break
					}
				}
			}
			return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
				testProviderToolCall("call-goal-complete-after-pause", workflowGoalCompleteTool, map[string]any{
					"summary": "这一轮已经完成。",
				}),
			}}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "本轮推进完成。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-before-complete-agent", Name: "Goal Pause Before Complete", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat, LoopMaxIterations: 3,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "推进一个在完成前暂停的目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 3},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow pause before complete: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.WorkflowStatus != workflowStatusPaused || response.Run.ResumeState != "user_paused" {
		t.Fatalf("paused run = %+v, want user-paused goal instead of completion", response.Run)
	}
	if response.Run.CompletedAt != nil {
		t.Fatalf("completedAt = %v, want nil for paused goal", response.Run.CompletedAt)
	}
	if response.Run.PauseRequestedAt == nil || response.Run.PausedAt == nil || response.Run.PausedReason != "user" {
		t.Fatalf("paused fields = pauseRequestedAt:%v pausedAt:%v pausedReason:%q", response.Run.PauseRequestedAt, response.Run.PausedAt, response.Run.PausedReason)
	}
	if runHasToolCall(response.Run, workflowGoalCompleteTool) {
		t.Fatalf("tool calls = %+v, want interrupted goal.complete pruned from paused snapshot", response.Run.ToolCalls)
	}
}

func TestGoalWorkflowPauseRequestBeforeNextTurnDoesNotCallModel(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-pause-before-next-turn-provider", func(req openAIChatRequest) openAIChatMessage {
		t.Fatalf("provider called after pause request: last user=%q", testGoalWorkflowLastUserMessage(req))
		return openAIChatMessage{}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-before-next-turn-agent", Name: "Goal Pause Before Next Turn", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop, LoopMaxIterations: 3,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause before next turn")
	now := nowString()
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
	run.WorkflowPlan = workflowPlanFromTasks([]Task{task}, run.WorkflowPlan)
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun with plan: %v", err)
	}
	response, err := (&WorkflowExecutor{runtime: runtime}).continueADKGoalWorkflow(ctx, workflowRequest{
		Agent: agent, Session: session, Message: run.UserMessage, Mode: WorkModeLoop, Objective: run.Objective,
		RunOptions: RunOptions{LoopMaxIterations: 3},
	}, run, []Task{task}, goalOrchestratorContinueNudge(run, "继续推进。"), 2, 3)
	if err != nil {
		t.Fatalf("continueADKGoalWorkflow: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "user_paused" || response.Run.PausedReason != "user" {
		t.Fatalf("run = %+v, want user-paused without another model call", response.Run)
	}
}

func TestGoalWorkflowPauseRequestBlocksChildCompletionContinuation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-child-agent", Name: "Goal Pause Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause child continuation")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-goal-pause-child-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, Message: "等待子运行", UserMessage: "推进目标", WorkMode: WorkModeLoop,
		Objective: "推进目标", Iteration: 1, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &now, ChildRunIDs: []string{"run-goal-pause-child"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "子步骤", Message: "执行子步骤", Status: "TODO", ChildRunID: "run-goal-pause-child",
		}},
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "run-goal-pause-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusRunning, Message: "子运行仍在执行", UserMessage: "执行子步骤",
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	runningParent, err := runtime.continueParentWorkflowAfterChild(ctx, child)
	if err != nil {
		t.Fatalf("continueParentWorkflowAfterChild running child: %v", err)
	}
	if runningParent == nil || runningParent.Status != RunStatusRunning || runningParent.PauseRequestedAt == nil {
		t.Fatalf("running child parent = %+v, want still running with pause requested", runningParent)
	}

	completedAt := nowString()
	child.Status = RunStatusCompleted
	child.Message = "子运行完成"
	child.CompletedAt = &completedAt
	child.UpdatedAt = completedAt
	if err := runtime.Store().SaveRun(ctx, child); err != nil {
		t.Fatalf("Save child completed: %v", err)
	}
	pausedParent, err := runtime.continueParentWorkflowAfterChild(ctx, child)
	if err != nil {
		t.Fatalf("continueParentWorkflowAfterChild completed child: %v", err)
	}
	if pausedParent == nil || pausedParent.Status != RunStatusPaused || pausedParent.WorkflowStatus != workflowStatusPaused || pausedParent.ResumeState != "user_paused" {
		t.Fatalf("completed child parent = %+v, want user-paused parent", pausedParent)
	}
	if pausedParent.CompletedAt != nil {
		t.Fatalf("parent completedAt = %v, want nil while user-paused", *pausedParent.CompletedAt)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPaused || stored.PausedReason != "user" {
		t.Fatalf("stored parent = %+v, want user-paused", stored)
	}
}

func TestGoalWorkflowActivitySnapshotDoesNotDowngradeUserPausedParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-paused-snapshot-agent", Name: "Goal Paused Snapshot", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "paused snapshot")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-goal-paused-snapshot-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusPaused, Message: "目标已暂停。", UserMessage: "推进目标", WorkMode: WorkModeLoop,
		Objective: "推进目标", Iteration: 1, WorkflowStatus: workflowStatusPaused,
		PauseRequestedAt: &now, PausedAt: &now, PausedReason: "user", ResumeState: "user_paused",
		WorkflowPlan: []WorkflowStepState{{
			Title: "已暂停步骤", Message: "等待继续", Status: "TODO",
		}},
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	snapshot := parent
	snapshot.Status = RunStatusRunning
	snapshot.WorkflowStatus = workflowStatusRunning
	snapshot.Message = "goal running"
	snapshot.PauseRequestedAt = nil
	snapshot.PausedAt = nil
	snapshot.PausedReason = ""
	snapshot.ResumeState = ""
	snapshot.ToolCalls = []ToolCall{{
		ID: "tool-paused-stale", RunID: parent.ID, ToolName: workflowGoalContinueTool, Status: "RUNNING",
		CreatedAt: now, UpdatedAt: now,
	}}

	if _, err := runtime.persistRunActivitySnapshot(ctx, snapshot); err != nil {
		t.Fatalf("persistRunActivitySnapshot: %v", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPaused || stored.WorkflowStatus != workflowStatusPaused || stored.PausedReason != "user" || stored.ResumeState != "user_paused" {
		t.Fatalf("stored parent = %+v, want lifecycle to remain user-paused", stored)
	}
	if len(stored.ToolCalls) != 1 || stored.ToolCalls[0].ID != "tool-paused-stale" {
		t.Fatalf("stored tool calls = %+v, want activity snapshot merged", stored.ToolCalls)
	}
}

func TestGoalWorkflowDecisionPromptUsesUpdatedObjective(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var mu sync.Mutex
	var decisionPrompts []string
	updated := false
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-updated-objective-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "是否完成目标") {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if seen[workflowGoalCompleteTool] {
				return openAIChatMessage{Role: "assistant", Content: "目标已按新描述完成。"}
			}
			mu.Lock()
			decisionPrompts = append(decisionPrompts, lastUser)
			mu.Unlock()
			return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
				testProviderToolCall("call-goal-complete", workflowGoalCompleteTool, map[string]any{
					"summary": "目标已按新描述完成。",
				}),
			}}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		if !updated && testProviderToolResponseNames(req.Messages)[workflowTaskCompleteTool] {
			updated = true
			runs, err := runtime.Store().ListRuns(ctx)
			if err != nil {
				return openAIChatMessage{Role: "assistant", Content: "读取 run 失败。"}
			}
			for _, run := range runs {
				if normalizeWorkMode(run.WorkMode) == WorkModeLoop && run.ParentRunID == "" && run.Status == RunStatusRunning {
					_, jftradeErr5 := runtime.UpdateRunObjective(ctx, run.ID, "更新后的目标")
					jftradeCheckTestError(t, jftradeErr5)
					break
				}
			}
		}
		return openAIChatMessage{Role: "assistant", Content: "已根据当前目标推进。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-updated-objective-agent", Name: "Goal Updated Objective", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "旧目标",
		Objective:        "旧目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 2},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow updated objective: %v", err)
	}
	if response.Run.Status != RunStatusCompleted {
		t.Fatalf("run = %+v, want completed", response.Run)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(decisionPrompts) == 0 {
		t.Fatal("decision prompts empty")
	}
	if !strings.Contains(decisionPrompts[0], "更新后的目标") {
		t.Fatalf("decision prompt = %q, want updated objective", decisionPrompts[0])
	}
}

func TestWorkflowResponseUsesAuthoritativePauseRequestedParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-response-pause-agent", Name: "Goal Response Pause", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause response")
	now := nowString()
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

	response := (&WorkflowExecutor{runtime: runtime}).workflowResponse(
		ctx,
		session,
		stale,
		openAIChatResult{Reply: "still running"},
	)

	if response.Run.PauseRequestedAt == nil || response.Run.ResumeState != "user_pause_requested" {
		t.Fatalf("response run = %+v, want authoritative pause request fields", response.Run)
	}
}

func TestUpdateRunObjectiveOnlyAllowsActiveGoalParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-objective-agent", Name: "Goal Objective", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "goal objective")
	active := mustSaveRun(t, runtime, Run{
		ID: "run-goal-active", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, Objective: "旧目标",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	updated, err := runtime.UpdateRunObjective(ctx, active.ID, " 新目标 ")
	if err != nil {
		t.Fatalf("UpdateRunObjective active goal: %v", err)
	}
	if updated.Objective != "新目标" {
		t.Fatalf("objective = %q, want updated", updated.Objective)
	}
	if _, err := runtime.UpdateRunObjective(ctx, active.ID, " "); err == nil {
		t.Fatal("UpdateRunObjective empty err = nil, want error")
	}
	chat := mustSaveRun(t, runtime, Run{
		ID: "run-chat-active", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, Objective: "chat",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.UpdateRunObjective(ctx, chat.ID, "新目标"); err == nil {
		t.Fatal("UpdateRunObjective chat run err = nil, want error")
	}
	child := mustSaveRun(t, runtime, Run{
		ID: "run-goal-child", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, Objective: "child", ParentRunID: active.ID,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.UpdateRunObjective(ctx, child.ID, "新目标"); err == nil {
		t.Fatal("UpdateRunObjective child run err = nil, want error")
	}
	completed := mustSaveRun(t, runtime, Run{
		ID: "run-goal-complete", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusCompleted, WorkMode: WorkModeLoop, Objective: "done",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.UpdateRunObjective(ctx, completed.ID, "新目标"); err == nil {
		t.Fatal("UpdateRunObjective terminal run err = nil, want error")
	}
}
