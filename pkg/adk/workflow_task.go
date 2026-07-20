package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	adktool "google.golang.org/adk/v2/tool"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

const (
	workflowTasksListTool     = "workflow.tasks.list"
	workflowTaskAddTool       = "workflow.task.add"
	workflowTaskClaimTool     = "workflow.task.claim"
	workflowTaskCompleteTool  = "workflow.task.complete"
	workflowTaskBlockTool     = "workflow.task.block"
	workflowTaskDelegateTool  = "workflow.task.delegate"
	workflowModelsListTool    = "workflow.models.list"
	workflowTaskIncompleteErr = "WORKFLOW_TASK_INCOMPLETE"

	workflowGoalCompleteTool = "workflow.goal.complete"
	workflowGoalContinueTool = "workflow.goal.continue"
)

type workflowGoalDecision struct {
	mu      sync.Mutex
	phase   string
	status  string
	summary string
	reason  string
}

type workflowGoalDecisionSnapshot struct {
	status  string
	summary string
	reason  string
}

func (d *workflowGoalDecision) reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.phase = "work"
	d.status = ""
	d.summary = ""
	d.reason = ""
}

func (d *workflowGoalDecision) beginDecision() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.phase = "decision"
	d.status = ""
	d.summary = ""
	d.reason = ""
}

func (d *workflowGoalDecision) decisionPhase() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.phase == "decision"
}

func (d *workflowGoalDecision) setComplete(summary string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = "complete"
	d.summary = strings.TrimSpace(summary)
	d.reason = ""
}

func (d *workflowGoalDecision) setContinue(reason string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = "continue"
	d.summary = ""
	d.reason = strings.TrimSpace(reason)
}

func (d *workflowGoalDecision) snapshot() workflowGoalDecisionSnapshot {
	if d == nil {
		return workflowGoalDecisionSnapshot{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return workflowGoalDecisionSnapshot{status: d.status, summary: d.summary, reason: d.reason}
}

func (e *WorkflowExecutor) runADKGoalWorkflow(ctx context.Context, req workflowRequest, parent Run, tasks []Task) (ChatResponse, error) {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = workflowEngineForMode(WorkModeLoop)
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if _, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
		parent, persistErr := e.failParent(ctx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	limit := normalizeLoopMaxIterations(req.RunOptions.LoopMaxIterations)
	return e.continueADKGoalWorkflow(ctx, req, parent, tasks, goalOrchestratorUserMessage(parent), 1, limit)
}

func (e *WorkflowExecutor) continueADKGoalWorkflow(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	tasks []Task,
	nextPrompt string,
	startIteration int,
	limit int,
) (ChatResponse, error) {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = workflowEngineForMode(WorkModeLoop)
	}
	decision := &workflowGoalDecision{}
	req.GoalDecision = decision
	execution, err := e.runtime.newGoogleADKTaskExecution(ctx, req.Agent, req.Session, parent, req, req.OnDelta)
	if err != nil {
		parent, persistErr := e.failParent(ctx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	if startIteration < 1 {
		startIteration = 1
	}
	for iteration := startIteration; iteration <= limit; iteration++ {
		var response ChatResponse
		var paused bool
		var pauseErr error
		parent, response, paused, pauseErr = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration-1, "")
		if pauseErr != nil {
			return ChatResponse{}, pauseErr
		}
		if paused {
			return response, nil
		}
		decision.reset()
		adkErr := execution.run(ctx, genai.NewContentFromText(nextPrompt, genai.RoleUser))
		parent, response, done, prompt, turnErr := e.finishADKGoalWorkflowTurn(ctx, req, parent, tasks, execution, decision, adkErr, iteration, false)
		if turnErr != nil {
			return ChatResponse{}, turnErr
		}
		if done {
			return response, nil
		}
		if prompt == "" {
			prompt = goalOrchestratorContinueNudge(parent, "")
		}
		nextPrompt = prompt
	}
	pausedAt := nowString()
	parent.Status = RunStatusPaused
	parent.WorkflowStatus = workflowStatusPaused
	parent.Message = "目标达到本轮运行上限，已暂停。"
	parent.ResumeState = "iteration_limit"
	parent.PausedReason = "iteration_limit"
	parent.PausedAt = &pausedAt
	parent.Iteration = limit
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	parent, saveErr := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	if saveErr != nil {
		return ChatResponse{}, fmt.Errorf("persist goal iteration-limit pause: %w", saveErr)
	}
	return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.Message}), nil
}

func (e *WorkflowExecutor) pauseADKGoalWorkflowIfRequested(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	completedIteration int,
	reply string,
) (Run, ChatResponse, bool, error) {
	latest := parent
	if refreshed, ok, err := e.runtime.store.Run(ctx, parent.ID); err == nil && ok {
		latest = refreshed
	}
	if latest.PauseRequestedAt == nil {
		return parent, ChatResponse{}, false, nil
	}
	parent = latest
	if parent.Status == RunStatusPaused && parent.PausedReason == "user" {
		if cleaned, changed := pruneInterruptedGoalWorkflowToolCalls(parent); changed {
			parent = cleaned
			updatedParent, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
			if err != nil {
				return Run{}, ChatResponse{}, false, fmt.Errorf("persist cleaned paused goal state: %w", err)
			}
			parent = updatedParent
		}
		return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: defaultString(reply, parent.Message)}), true, nil
	}
	pausedAt := nowString()
	parent.Status = RunStatusPaused
	parent.WorkflowStatus = workflowStatusPaused
	if parent.PausedAt == nil {
		parent.PausedAt = &pausedAt
	}
	parent.PausedReason = "user"
	parent.ResumeState = "user_paused"
	parent.Message = "目标已暂停。"
	if completedIteration > parent.Iteration {
		parent.Iteration = completedIteration
	}
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	parent, _ = pruneInterruptedGoalWorkflowToolCalls(parent)
	parent, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	if err != nil {
		return Run{}, ChatResponse{}, false, fmt.Errorf("persist user-paused goal state: %w", err)
	}
	return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: defaultString(reply, parent.Message)}), true, nil
}

func (e *WorkflowExecutor) resumeADKGoalWorkflow(ctx context.Context, session Session, agent Agent, parent Run) (Run, error) {
	parent, blocked, err := e.reconcileWorkflowChildren(ctx, parent)
	if err != nil {
		return Run{}, err
	}
	if blocked {
		return parent, nil
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = "goal resumed"
	parent.ResumeState = "user_resuming"
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	if _, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
		return Run{}, err
	}
	tasks, err := e.workflowTasks(ctx, parent, nil)
	if err != nil {
		return Run{}, err
	}
	limit := parent.Iteration + normalizeLoopMaxIterations(agent.LoopMaxIterations)
	response, err := e.continueADKGoalWorkflow(ctx, workflowRequest{
		Agent: agent, Session: session, Message: parent.UserMessage, Mode: parent.WorkMode, Objective: parent.Objective,
		RunOptions: RunOptions{
			LoopMaxIterations: limit,
		},
	}, parent, tasks, goalOrchestratorContinueNudge(parent, "用户继续运行目标。"), parent.Iteration+1, limit)
	if err != nil {
		return Run{}, err
	}
	return response.Run, nil
}

func (e *WorkflowExecutor) finishADKGoalWorkflowTurn(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution *googleADKExecution,
	decision *workflowGoalDecision,
	adkErr error,
	iteration int,
	decisionRetry bool,
) (Run, ChatResponse, bool, string, error) {
	parent, replyResult, done, prompt, err := e.prepareGoalWorkflowTurn(ctx, req, parent, known, execution, adkErr, iteration)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if done {
		return parent, e.workflowResponse(ctx, req.Session, parent, replyResult), true, "", nil
	}
	visibleReply := strings.TrimSpace(replyResult.Reply)
	parent, replyResult, snapshot, done, response, prompt, err := e.resolveGoalWorkflowDecision(
		ctx, req, parent, known, execution, decision, replyResult, visibleReply, prompt, iteration, decisionRetry,
	)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if done {
		return parent, response, true, prompt, nil
	}
	switch snapshot.status {
	case "complete":
		return e.finishCompleteGoalWorkflow(ctx, req, parent, known, replyResult, snapshot, visibleReply, iteration)
	case "continue":
		return e.finishContinueGoalWorkflow(ctx, req, parent, replyResult, snapshot, visibleReply, iteration)
	default:
		return parent, ChatResponse{}, false, prompt, nil
	}
}

func (e *WorkflowExecutor) resolveGoalWorkflowDecision(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution *googleADKExecution,
	decision *workflowGoalDecision,
	replyResult openAIChatResult,
	visibleReply string,
	prompt string,
	iteration int,
	decisionRetry bool,
) (Run, openAIChatResult, workflowGoalDecisionSnapshot, bool, ChatResponse, string, error) {
	snapshot := decision.snapshot()
	if snapshot.status != "" {
		return parent, replyResult, snapshot, false, ChatResponse{}, prompt, nil
	}
	parent, response, paused, err := e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, visibleReply)
	if err != nil {
		return Run{}, openAIChatResult{}, workflowGoalDecisionSnapshot{}, false, ChatResponse{}, "", err
	}
	if paused {
		replyResult.Reply = defaultString(visibleReply, replyResult.Reply)
		return parent, replyResult, snapshot, true, response, "", nil
	}
	latest := parent
	if refreshed, ok, err := e.runtime.store.Run(ctx, parent.ID); err == nil && ok {
		latest = refreshed
		parent = refreshed
	}
	if !goalTurnHasFinalReply(execution, parent.ID, visibleReply) {
		return parent, replyResult, snapshot, false, ChatResponse{}, goalFinalReplyPrompt(parent), nil
	}
	return e.runGoalWorkflowDecision(ctx, req, parent, known, execution, decision, latest, visibleReply, iteration, decisionRetry)
}

func (e *WorkflowExecutor) runGoalWorkflowDecision(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution *googleADKExecution,
	decision *workflowGoalDecision,
	latest Run,
	visibleReply string,
	iteration int,
	decisionRetry bool,
) (Run, openAIChatResult, workflowGoalDecisionSnapshot, bool, ChatResponse, string, error) {
	decision.beginDecision()
	decisionErr := execution.run(ctx, genai.NewContentFromText(goalDecisionPrompt(latest, visibleReply, decisionRetry), genai.RoleUser))
	parent, replyResult, done, prompt, err := e.prepareGoalWorkflowTurn(ctx, req, parent, known, execution, decisionErr, iteration)
	if err != nil {
		return Run{}, openAIChatResult{}, workflowGoalDecisionSnapshot{}, false, ChatResponse{}, "", err
	}
	if done {
		return parent, replyResult, decision.snapshot(), true, e.workflowResponse(ctx, req.Session, parent, replyResult), "", nil
	}
	snapshot := decision.snapshot()
	parent, replyResult, done, response, err := e.pauseAfterMissingGoalDecision(ctx, req, parent, replyResult, visibleReply, snapshot, iteration)
	if err != nil {
		return Run{}, openAIChatResult{}, workflowGoalDecisionSnapshot{}, false, ChatResponse{}, "", err
	}
	if done {
		return parent, replyResult, snapshot, true, response, "", nil
	}
	if snapshot.status == "" && !decisionRetry {
		parent, response, done, prompt, err = e.finishADKGoalWorkflowTurn(ctx, req, parent, known, execution, decision, nil, iteration, true)
		return parent, replyResult, snapshot, done, response, prompt, err
	}
	if snapshot.status == "" {
		decision.setContinue("目标裁决未按要求调用工具，安全地继续目标。")
		snapshot = decision.snapshot()
	}
	return parent, replyResult, snapshot, false, ChatResponse{}, prompt, nil
}

func (e *WorkflowExecutor) pauseAfterMissingGoalDecision(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	replyResult openAIChatResult,
	visibleReply string,
	snapshot workflowGoalDecisionSnapshot,
	iteration int,
) (Run, openAIChatResult, bool, ChatResponse, error) {
	if snapshot.status != "" {
		return parent, replyResult, false, ChatResponse{}, nil
	}
	reply := strings.TrimSpace(replyResult.Reply)
	if reply == "" {
		reply = visibleReply
	}
	parent, response, paused, err := e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
	if err != nil {
		return Run{}, openAIChatResult{}, false, ChatResponse{}, err
	}
	if paused {
		replyResult.Reply = defaultString(reply, replyResult.Reply)
		return parent, replyResult, true, response, nil
	}
	return parent, replyResult, false, ChatResponse{}, nil
}

func (e *WorkflowExecutor) finishCompleteGoalWorkflow(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	replyResult openAIChatResult,
	snapshot workflowGoalDecisionSnapshot,
	visibleReply string,
	iteration int,
) (Run, ChatResponse, bool, string, error) {
	reply := e.completeGoalReply(ctx, parent, known, snapshot, visibleReply)
	replyResult.Reply = reply
	parent, response, paused, err := e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if paused {
		return parent, response, true, "", nil
	}
	parent.Status = RunStatusCompleted
	parent.WorkflowStatus = workflowStatusComplete
	parent.Message = "goal completed"
	parent.Iteration = iteration
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	parent.CompletedAt = new(nowString())
	finalizeRunUsage(&parent)
	if saved, err := e.runtime.attachFinalAssistantMessage(ctx, req.Session, parent, replyResult); err == nil {
		parent = saved
	} else {
		if saveErr := e.runtime.store.SaveRun(ctx, parent); saveErr != nil {
			return Run{}, ChatResponse{}, false, "", fmt.Errorf("persist completed goal state: %w", saveErr)
		}
	}
	return parent, e.workflowResponse(ctx, req.Session, parent, replyResult), true, "", nil
}

func (e *WorkflowExecutor) completeGoalReply(
	ctx context.Context,
	parent Run,
	known []Task,
	snapshot workflowGoalDecisionSnapshot,
	visibleReply string,
) string {
	reply := strings.TrimSpace(snapshot.summary)
	if reply != "" {
		return reply
	}
	if visibleReply != "" {
		return visibleReply
	}
	tasks, jftradeErr10 := e.workflowTasks(ctx, parent, known)
	besteffort.LogError(jftradeErr10)
	return workflowSummary(parent, workflowTaskResultSummaries(tasks))
}

func (e *WorkflowExecutor) finishContinueGoalWorkflow(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	replyResult openAIChatResult,
	snapshot workflowGoalDecisionSnapshot,
	visibleReply string,
	iteration int,
) (Run, ChatResponse, bool, string, error) {
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = defaultString(snapshot.reason, "goal continues")
	parent.Iteration = iteration
	reply := defaultString(visibleReply, defaultString(snapshot.reason, "目标已暂停。"))
	parent, response, paused, err := e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if paused {
		replyResult.Reply = reply
		return parent, response, true, "", nil
	}
	parent, err = e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", fmt.Errorf("persist continued goal state: %w", err)
	}
	return parent, ChatResponse{}, false, goalOrchestratorContinueNudge(parent, snapshot.reason), nil
}

func (e *WorkflowExecutor) prepareGoalWorkflowTurn(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution *googleADKExecution,
	adkErr error,
	iteration int,
) (Run, openAIChatResult, bool, string, error) {
	if latest, ok, err := e.runtime.store.Run(ctx, parent.ID); err == nil && ok {
		parent = latest
	}
	toolContext := execution.toolContextForRun(parent.ID)
	replyResult := execution.resultForRun(parent.ID)
	parent = hydrateRunExecutionResult(parent, toolContext, nil, "", "")
	parent.Iteration = iteration
	tasks, err := e.workflowTasks(ctx, parent, known)
	if err != nil {
		parent, persistErr := e.failParent(ctx, parent, err)
		if persistErr != nil {
			return Run{}, openAIChatResult{}, false, "", persistErr
		}
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, "", nil
	}
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if errors.Is(adkErr, errUserGoalPauseRequested) {
		reply := strings.TrimSpace(replyResult.Reply)
		var paused bool
		parent, _, paused, err = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
		if err != nil {
			return Run{}, openAIChatResult{}, false, "", err
		}
		if paused {
			return parent, openAIChatResult{Reply: defaultString(reply, parent.Message)}, true, "", nil
		}
	}
	if adkErr != nil {
		parent, persistErr := e.failParent(ctx, parent, adkErr)
		if persistErr != nil {
			return Run{}, openAIChatResult{}, false, "", persistErr
		}
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, "", nil
	}
	if child, index, ok := e.firstBlockingTaskChild(ctx, parent); ok {
		if child.Status == RunStatusPending || child.Status == RunStatusPendingInput || child.Status == RunStatusRunning {
			parent = pauseParentForChild(parent, child, index)
			parent.Iteration = iteration
			parent, err = e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
			if err != nil {
				return Run{}, openAIChatResult{}, false, "", fmt.Errorf("persist goal blocked by child: %w", err)
			}
			return parent, openAIChatResult{Reply: workflowPendingReply(parent)}, true, "", nil
		}
		parent, err = e.runtime.terminateParentWorkflowFromChild(ctx, parent, child)
		if err != nil {
			return Run{}, openAIChatResult{}, false, "", err
		}
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, "", nil
	}
	if blockedTask, ok := firstTerminalWorkflowTask(tasks); ok {
		parent.Status = RunStatusFailed
		parent.WorkflowStatus = workflowStatusFailed
		parent.Message = defaultString(blockedTask.ResultSummary, blockedTask.Description)
		parent.FailureReason = parent.Message
		parent.ErrorCode = "WORKFLOW_TASK_BLOCKED"
		parent.Degraded = true
		parent.CompletedAt = new(nowString())
		finalizeRunUsage(&parent)
		if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
			return Run{}, openAIChatResult{}, false, "", fmt.Errorf("persist blocked goal state: %w", err)
		}
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, "", nil
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = "goal running"
	parent, err = e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	if err != nil {
		return Run{}, openAIChatResult{}, false, "", fmt.Errorf("persist running goal state: %w", err)
	}
	return parent, replyResult, false, "", nil
}

func pruneInterruptedGoalWorkflowToolCalls(run Run) (Run, bool) {
	if len(run.ToolCalls) == 0 {
		return run, false
	}
	filtered := make([]ToolCall, 0, len(run.ToolCalls))
	changed := false
	for _, call := range run.ToolCalls {
		if interruptedGoalWorkflowToolCall(run, call) {
			changed = true
			continue
		}
		filtered = append(filtered, call)
	}
	if !changed {
		return run, false
	}
	run.ToolCalls = filtered
	run.ToolSummaries = toolSummariesForRun(Run{ToolCalls: filtered})
	return run, true
}

func interruptedGoalWorkflowToolCall(parent Run, call ToolCall) bool {
	switch strings.ToUpper(strings.TrimSpace(call.Status)) {
	case "RUNNING", "PENDING":
		runID := strings.TrimSpace(call.RunID)
		if runID != "" && runID != strings.TrimSpace(parent.ID) {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(call.ToolName), "workflow.")
	case "FAILED":
		if strings.HasPrefix(strings.TrimSpace(call.ToolName), "workflow.goal.") {
			return true
		}
		if call.Error == nil {
			return false
		}
		errorText := strings.TrimSpace(*call.Error)
		if !strings.Contains(errorText, errUserGoalPauseRequested.Error()) && !strings.Contains(errorText, adkworkflow.ErrNodeInterrupted.Error()) {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(call.ToolName), "workflow.")
	}
	return false
}

func (e *WorkflowExecutor) firstBlockingTaskChild(ctx context.Context, parent Run) (Run, int, bool) {
	for index, state := range parent.WorkflowPlan {
		childRunID := strings.TrimSpace(state.ChildRunID)
		if childRunID == "" {
			continue
		}
		child, ok, err := e.runtime.store.Run(ctx, childRunID)
		if err != nil || !ok {
			continue
		}
		if !isDirectWorkflowChild(parent, child) {
			continue
		}
		switch child.Status {
		case RunStatusPending, RunStatusPendingInput, RunStatusRunning, RunStatusFailed, RunStatusDenied, RunStatusCancelled, RunStatusTimedOut:
			return child, index, true
		}
	}
	return Run{}, -1, false
}

func (r *Runtime) newGoogleADKTaskExecution(
	ctx context.Context,
	definition Agent,
	productSession Session,
	parent Run,
	req workflowRequest,
	onDelta func(ChatDelta) error,
) (*googleADKExecution, error) {
	llm, err := r.googleADKModelForAgent(ctx, definition)
	if err != nil {
		return nil, err
	}
	rootName := googleADKWorkflowRootName(parent.ID)
	engine := workflowEngineForMode(req.Mode)
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = engine
	}
	descriptors := toolDescriptorIndex(workflowTaskToolDescriptors())
	execution := &googleADKExecution{
		sessionID: productSession.ID,
		appName:   googleADKAppName(definition.ID),
		agent:     definition,
		runID:     parent.ID,
		runIDByAgentName: map[string]string{
			rootName: parent.ID,
		},
		runSnapshotBaseByID: map[string]Run{
			parent.ID: parent,
		},
		descriptors:              descriptors,
		calls:                    []ToolCall{},
		summaries:                []string{},
		replyByRunID:             map[string]*strings.Builder{},
		reasoningByRunID:         map[string]*strings.Builder{},
		bufferedReplyByRunID:     map[string]*strings.Builder{},
		bufferedReasoningByRunID: map[string]*strings.Builder{},
		toolResponseSeenByRunID:  map[string]bool{},
		postToolTextByRunID:      map[string]bool{},
		toolResponseSeqByRunID:   map[string]int{},
		postToolTextSeqByRunID:   map[string]int{},
		onDelta:                  onDelta,
		loadRun: func(ctx context.Context, runID string) (Run, bool, error) {
			if r.store == nil {
				return Run{}, false, nil
			}
			return r.store.Run(ctx, runID)
		},
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			return r.persistRunActivitySnapshot(context.Background(), snapshot)
		},
	}
	taskToolset := &workflowTaskToolset{executor: &WorkflowExecutor{runtime: r}, req: req, parentID: parent.ID}
	orchestratorName := rootName + "_iteration"
	execution.runIDByAgentName[orchestratorName] = parent.ID
	orchestrator, err := llmagent.New(llmagent.Config{
		Name:        orchestratorName,
		Description: definition.Name + " goal orchestrator",
		InstructionProvider: func(ctx adkagent.ReadonlyContext) (string, error) {
			instruction := goalOrchestratorInstruction(definition.Instruction)
			if r.contextManager == nil || ctx == nil {
				return instruction, nil
			}
			suffix, suffixErr := r.contextManager.InstructionSuffix(ctx, ctx.SessionID())
			if suffixErr != nil || strings.TrimSpace(suffix) == "" {
				return instruction, nil
			}
			return instruction + "\n\n" + suffix, nil
		},
		Model:           llm,
		Toolsets:        []adktool.Toolset{taskToolset},
		IncludeContents: llmagent.IncludeContentsDefault,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK goal orchestrator agent: %w", err)
	}
	root, err := newGoogleADKLoopWorkflowAgent(rootName, definition.Name+" goal loop", []adkagent.Agent{orchestrator}, 1)
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK goal loop agent: %w", err)
	}
	return r.attachGoogleADKRunner(ctx, execution, productSession, root)
}
