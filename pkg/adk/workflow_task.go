package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	adktool "google.golang.org/adk/v2/tool"
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

func (d *workflowGoalDecision) snapshot() workflowGoalDecision {
	if d == nil {
		return workflowGoalDecision{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return workflowGoalDecision{status: d.status, summary: d.summary, reason: d.reason}
}

func (e *WorkflowExecutor) runADKTaskWorkflow(ctx context.Context, req workflowRequest, parent Run, tasks []Task) (ChatResponse, error) {
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if _, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	execution, err := e.runtime.newGoogleADKTaskExecution(ctx, req.Agent, req.Session, parent, req, req.OnDelta)
	if err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	adkErr := execution.run(ctx, genai.NewContentFromText(taskOrchestratorUserMessage(parent), genai.RoleUser))
	parent, response, done := e.finishADKTaskWorkflowAttempt(ctx, req, parent, tasks, execution, adkErr, false)
	if done {
		return response, nil
	}
	adkErr = execution.run(ctx, genai.NewContentFromText(taskOrchestratorNudge(parent), genai.RoleUser))
	_, response, _ = e.finishADKTaskWorkflowAttempt(ctx, req, parent, tasks, execution, adkErr, true)
	return response, nil
}

func (e *WorkflowExecutor) runADKGoalWorkflow(ctx context.Context, req workflowRequest, parent Run, tasks []Task) (ChatResponse, error) {
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if _, err := e.runtime.saveRunPreservingUserGoalPause(ctx, parent); err != nil {
		parent = e.failParent(ctx, parent, err)
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
	decision := &workflowGoalDecision{}
	req.GoalDecision = decision
	execution, err := e.runtime.newGoogleADKTaskExecution(ctx, req.Agent, req.Session, parent, req, req.OnDelta)
	if err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	if startIteration < 1 {
		startIteration = 1
	}
	for iteration := startIteration; iteration <= limit; iteration++ {
		var response ChatResponse
		var paused bool
		parent, response, paused = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration-1, "")
		if paused {
			return response, nil
		}
		decision.reset()
		adkErr := execution.run(ctx, genai.NewContentFromText(nextPrompt, genai.RoleUser))
		parent, response, done, prompt := e.finishADKGoalWorkflowTurn(ctx, req, parent, tasks, execution, decision, adkErr, iteration, false)
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
	jftradeLogError(saveErr)
	return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.Message}), nil
}

func (e *WorkflowExecutor) pauseADKGoalWorkflowIfRequested(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	completedIteration int,
	reply string,
) (Run, ChatResponse, bool) {
	latest := parent
	if refreshed, ok, err := e.runtime.store.Run(ctx, parent.ID); err == nil && ok {
		latest = refreshed
	}
	if latest.PauseRequestedAt == nil {
		return parent, ChatResponse{}, false
	}
	parent = latest
	if parent.Status == RunStatusPaused && parent.PausedReason == "user" {
		if cleaned, changed := pruneInterruptedGoalWorkflowToolCalls(parent); changed {
			parent = cleaned
			updatedParent, jftradeErr29 := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
			jftradeLogError(jftradeErr29)
			parent = updatedParent
		}
		return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: defaultString(reply, parent.Message)}), true
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
	parent, jftradeErr25 := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	jftradeLogError(jftradeErr25)
	return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: defaultString(reply, parent.Message)}), true
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
) (Run, ChatResponse, bool, string) {
	parent, replyResult, done, prompt := e.prepareGoalWorkflowTurn(ctx, req, parent, known, execution, adkErr, iteration)
	if done {
		return parent, e.workflowResponse(ctx, req.Session, parent, replyResult), true, ""
	}
	visibleReply := strings.TrimSpace(replyResult.Reply)
	snapshot := decision.snapshot()
	if snapshot.status == "" {
		var response ChatResponse
		var paused bool
		parent, response, paused = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, visibleReply)
		if paused {
			replyResult.Reply = defaultString(visibleReply, replyResult.Reply)
			return parent, response, true, ""
		}
		latest := parent
		if refreshed, ok, err := e.runtime.store.Run(ctx, parent.ID); err == nil && ok {
			latest = refreshed
			parent = refreshed
		}
		if !goalTurnHasFinalReply(execution, parent.ID, visibleReply) {
			return parent, ChatResponse{}, false, goalFinalReplyPrompt(parent)
		}
		decision.beginDecision()
		decisionErr := execution.run(ctx, genai.NewContentFromText(goalDecisionPrompt(latest, visibleReply, decisionRetry), genai.RoleUser))
		parent, replyResult, done, prompt = e.prepareGoalWorkflowTurn(ctx, req, parent, known, execution, decisionErr, iteration)
		if done {
			return parent, e.workflowResponse(ctx, req.Session, parent, replyResult), true, ""
		}
		snapshot = decision.snapshot()
		if snapshot.status == "" {
			reply := strings.TrimSpace(replyResult.Reply)
			if reply == "" {
				reply = visibleReply
			}
			parent, response, paused = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
			if paused {
				replyResult.Reply = defaultString(reply, replyResult.Reply)
				return parent, response, true, ""
			}
		}
		if snapshot.status == "" && !decisionRetry {
			return e.finishADKGoalWorkflowTurn(ctx, req, parent, known, execution, decision, nil, iteration, true)
		}
		if snapshot.status == "" {
			decision.setContinue("目标裁决未按要求调用工具，安全地继续目标。")
			snapshot = decision.snapshot()
		}
	}
	switch snapshot.status {
	case "complete":
		reply := strings.TrimSpace(snapshot.summary)
		if reply == "" {
			reply = visibleReply
		}
		if reply == "" {
			tasks, jftradeErr10 := e.workflowTasks(ctx, parent, known)
			jftradeLogError(jftradeErr10)
			reply = workflowSummary(parent, workflowTaskResultSummaries(tasks))
		}
		replyResult.Reply = reply
		var response ChatResponse
		var paused bool
		parent, response, paused = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
		if paused {
			return parent, response, true, ""
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
			jftradeErr6 := e.runtime.store.SaveRun(ctx, parent)
			jftradeLogError(jftradeErr6)
		}
		return parent, e.workflowResponse(ctx, req.Session, parent, replyResult), true, ""
	case "continue":
		parent.Status = RunStatusRunning
		parent.WorkflowStatus = workflowStatusRunning
		parent.Message = defaultString(snapshot.reason, "goal continues")
		parent.Iteration = iteration
		reply := visibleReply
		if reply == "" {
			reply = defaultString(snapshot.reason, "目标已暂停。")
		}
		var response ChatResponse
		var paused bool
		parent, response, paused = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
		if paused {
			replyResult.Reply = reply
			return parent, response, true, ""
		}
		parent, jftradeErr24 := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
		jftradeLogError(jftradeErr24)
		return parent, ChatResponse{}, false, goalOrchestratorContinueNudge(parent, snapshot.reason)
	default:
		return parent, ChatResponse{}, false, prompt
	}
}

func (e *WorkflowExecutor) prepareGoalWorkflowTurn(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution *googleADKExecution,
	adkErr error,
	iteration int,
) (Run, openAIChatResult, bool, string) {
	if latest, ok, err := e.runtime.store.Run(ctx, parent.ID); err == nil && ok {
		parent = latest
	}
	toolContext := execution.toolContextForRun(parent.ID)
	replyResult := execution.resultForRun(parent.ID)
	parent = hydrateRunExecutionResult(parent, toolContext, nil, "", "")
	parent.Iteration = iteration
	tasks, err := e.workflowTasks(ctx, parent, known)
	if err != nil {
		parent = e.failParent(ctx, parent, err)
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, ""
	}
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if errors.Is(adkErr, errUserGoalPauseRequested) {
		reply := strings.TrimSpace(replyResult.Reply)
		var paused bool
		parent, _, paused = e.pauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
		if paused {
			return parent, openAIChatResult{Reply: defaultString(reply, parent.Message)}, true, ""
		}
	}
	if adkErr != nil {
		parent = e.failParent(ctx, parent, adkErr)
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, ""
	}
	if child, index, ok := e.firstBlockingTaskChild(ctx, parent); ok {
		if child.Status == RunStatusPending || child.Status == RunStatusRunning {
			parent = pauseParentForChild(parent, child, index)
			parent.Iteration = iteration
			parent, jftradeErr26 := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
			jftradeLogError(jftradeErr26)
			return parent, openAIChatResult{Reply: workflowPendingReply(parent)}, true, ""
		}
		parent = e.runtime.terminateParentWorkflowFromChild(ctx, parent, child)
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, ""
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
		jftradeErr1 := e.runtime.store.SaveRun(ctx, parent)
		jftradeLogError(jftradeErr1)
		return parent, openAIChatResult{Reply: parent.FailureReason}, true, ""
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = "goal running"
	parent, jftradeErr27 := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	jftradeLogError(jftradeErr27)
	return parent, replyResult, false, ""
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
	runID := strings.TrimSpace(call.RunID)
	if runID != "" && runID != strings.TrimSpace(parent.ID) {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(call.Status)) {
	case "RUNNING", "PENDING":
		return strings.HasPrefix(strings.TrimSpace(call.ToolName), "workflow.")
	case "FAILED":
		if call.Error == nil || !strings.Contains(strings.TrimSpace(*call.Error), errUserGoalPauseRequested.Error()) {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(call.ToolName), "workflow.")
	}
	return false
}

func (e *WorkflowExecutor) resumeADKTaskWorkflow(ctx context.Context, session Session, agent Agent, parent Run) (Run, error) {
	parent, blocked, err := e.reconcileWorkflowChildren(ctx, parent)
	if err != nil {
		return Run{}, err
	}
	if blocked {
		return parent, nil
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = "workflow resumed"
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
		return Run{}, err
	}
	tasks, err := e.workflowTasks(ctx, parent, nil)
	if err != nil {
		return Run{}, err
	}
	response, err := e.runADKTaskWorkflow(ctx, workflowRequest{
		Agent: agent, Session: session, Message: parent.UserMessage, Mode: parent.WorkMode, Objective: parent.Objective,
		RunOptions: RunOptions{
			LoopMaxIterations: normalizeLoopMaxIterations(agent.LoopMaxIterations),
		},
	}, parent, tasks)
	if err != nil {
		return Run{}, err
	}
	return response.Run, nil
}

func (e *WorkflowExecutor) finishADKTaskWorkflowAttempt(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution *googleADKExecution,
	adkErr error,
	finalAttempt bool,
) (Run, ChatResponse, bool) {
	if latest, ok, err := e.runtime.store.Run(ctx, parent.ID); err == nil && ok {
		parent = latest
	}
	toolContext := execution.toolContextForRun(parent.ID)
	replyResult := execution.resultForRun(parent.ID)
	parent = hydrateRunExecutionResult(parent, toolContext, nil, "", "")
	tasks, err := e.workflowTasks(ctx, parent, known)
	if err != nil {
		parent = e.failParent(ctx, parent, err)
		return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), true
	}
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if adkErr != nil {
		parent = e.failParent(ctx, parent, adkErr)
		return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), true
	}
	if child, index, ok := e.firstBlockingTaskChild(ctx, parent); ok {
		if child.Status == RunStatusPending || child.Status == RunStatusRunning {
			parent = pauseParentForChild(parent, child, index)
			jftradeErr4 := e.runtime.store.SaveRun(ctx, parent)
			jftradeLogError(jftradeErr4)
			return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowPendingReply(parent)}), true
		}
		parent = e.runtime.terminateParentWorkflowFromChild(ctx, parent, child)
		return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), true
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
		jftradeErr5 := e.runtime.store.SaveRun(ctx, parent)
		jftradeLogError(jftradeErr5)
		return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), true
	}
	if !workflowTasksComplete(tasks) {
		parent.Status = RunStatusRunning
		parent.WorkflowStatus = workflowStatusRunning
		parent.Message = "workflow running"
		jftradeErr9 := e.runtime.store.SaveRun(ctx, parent)
		jftradeLogError(jftradeErr9)
		if !finalAttempt {
			return parent, ChatResponse{}, false
		}
		parent = e.failParent(ctx, parent, fmt.Errorf("workflow task scheduler incomplete"))
		parent.ErrorCode = workflowTaskIncompleteErr
		jftradeErr2 := e.runtime.store.SaveRun(ctx, parent)
		jftradeLogError(jftradeErr2)
		return parent, e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), true
	}
	reply := strings.TrimSpace(replyResult.Reply)
	if reply == "" {
		reply = workflowSummary(parent, workflowTaskResultSummaries(tasks))
		replyResult.Reply = reply
	}
	parent.Status = RunStatusCompleted
	parent.WorkflowStatus = workflowStatusComplete
	parent.Message = "workflow completed"
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	parent.CompletedAt = new(nowString())
	finalizeRunUsage(&parent)
	if saved, err := e.runtime.attachFinalAssistantMessage(ctx, req.Session, parent, replyResult); err == nil {
		parent = saved
	} else {
		jftradeErr3 := e.runtime.store.SaveRun(ctx, parent)
		jftradeLogError(jftradeErr3)
	}
	return parent, e.workflowResponse(ctx, req.Session, parent, replyResult), true
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
		case RunStatusPending, RunStatusRunning, RunStatusFailed, RunStatusDenied, RunStatusCancelled, RunStatusTimedOut:
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
	root, err := llmagent.New(llmagent.Config{
		Name:        rootName,
		Description: definition.Name + " task orchestrator",
		InstructionProvider: func(ctx adkagent.ReadonlyContext) (string, error) {
			instruction := taskOrchestratorInstruction(definition.Instruction)
			if normalizeWorkMode(req.Mode) == WorkModeLoop {
				instruction = goalOrchestratorInstruction(definition.Instruction)
			}
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
		return nil, fmt.Errorf("create GO-ADK task orchestrator agent: %w", err)
	}
	return r.attachGoogleADKRunner(ctx, execution, productSession, root)
}
