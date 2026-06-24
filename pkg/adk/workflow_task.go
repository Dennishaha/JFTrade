package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	adktool "google.golang.org/adk/tool"
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

type workflowTaskToolset struct {
	mu            sync.Mutex
	executor      *WorkflowExecutor
	req           workflowRequest
	parentID      string
	currentTaskID string
}

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
		Model:               llm,
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{execution.beforeToolCallback},
		AfterToolCallbacks:  []llmagent.AfterToolCallback{execution.afterToolCallback},
		Toolsets:            []adktool.Toolset{taskToolset},
		IncludeContents:     llmagent.IncludeContentsDefault,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK task orchestrator agent: %w", err)
	}
	return r.attachGoogleADKRunner(ctx, execution, productSession, root)
}

func (t *workflowTaskToolset) Name() string { return "jftrade-workflow-task-tools" }

func (t *workflowTaskToolset) Tools(adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if normalizeWorkMode(t.req.Mode) == WorkModeLoop && t.req.GoalDecision != nil && t.req.GoalDecision.decisionPhase() {
		return []adktool.Tool{
			&workflowPlannerTool{name: workflowGoalCompleteTool, description: "Declare that the current objective is complete and finish the goal loop.", schema: workflowGoalCompleteSchema(), run: t.goalComplete},
			&workflowPlannerTool{name: workflowGoalContinueTool, description: "Declare that the current objective is not complete yet and continue orchestration.", schema: workflowGoalContinueSchema(), run: t.goalContinue},
		}, nil
	}
	tools := []adktool.Tool{
		&workflowPlannerTool{name: workflowTasksListTool, description: "List current workflow TODO DAG, ready tasks, completed results and blocked state.", schema: emptyObjectSchema(), run: t.list},
		&workflowPlannerTool{name: workflowTaskAddTool, description: "Add a runtime TODO to the current ADK task workflow.", schema: workflowTaskAddSchema(), run: t.add},
		&workflowPlannerTool{name: workflowTaskClaimTool, description: "Claim a ready TODO for the orchestrator itself or a child agent.", schema: workflowTaskClaimSchema(), run: t.claim},
		&workflowPlannerTool{name: workflowTaskCompleteTool, description: "Mark a claimed or ready TODO as DONE with a result summary.", schema: workflowTaskCompleteSchema(), run: t.complete},
		&workflowPlannerTool{name: workflowTaskBlockTool, description: "Mark a TODO as BLOCKED with a blocking reason.", schema: workflowTaskBlockSchema(), run: t.block},
		&workflowPlannerTool{name: workflowTaskDelegateTool, description: "Delegate a ready TODO to an ADK child agent. This creates a JFTrade child run only when called.", schema: workflowTaskDelegateSchema(), run: t.delegate},
		&workflowPlannerTool{name: workflowModelsListTool, description: "List callable ADK models that can be selected for delegated child agents.", schema: workflowModelsListSchema(), run: t.modelsList},
	}
	return tools, nil
}

func (t *workflowTaskToolset) modelsList(args map[string]any) (map[string]any, error) {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	output, err := t.executor.runtime.modelsListTool(context.Background(), args)
	if err != nil {
		return nil, err
	}
	mapped, ok := output.(map[string]any)
	if !ok {
		return map[string]any{"result": output}, nil
	}
	return mapped, nil
}

func (t *workflowTaskToolset) list(map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	jftradeErr15 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr15)
	return map[string]any{"success": true, "tasks": taskToolTaskSummaries(tasks), "readyTasks": taskToolTaskSummaries(executableWorkflowTasks(tasks, parent.WorkMode))}, nil
}

func (t *workflowTaskToolset) add(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, _, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	current, _ := t.taskByID(context.Background(), t.currentTaskID)
	task, err := t.executor.addRuntimeWorkflowTask(context.Background(), parent, current, workflowRuntimeTaskRequest{
		Title: plannerStringArg(args, "title"), Message: plannerStringArg(args, "message"), Description: plannerStringArg(args, "description"),
		DependsOn: plannerStringSliceArg(args, "dependsOn"), AgentRole: plannerStringArg(args, "agentRole"), ModeHint: plannerStringArg(args, "modeHint"),
		ChildProviderID: plannerStringArg(args, "childProviderId"), ChildModel: plannerStringArg(args, "childModel"),
	})
	if err != nil {
		return nil, err
	}
	parent, tasks, jftradeErr22 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr22)
	jftradeErr13 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr13)
	return map[string]any{"success": true, "task": taskToolTaskSummary(task)}, nil
}

func (t *workflowTaskToolset) claim(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), true)
	if err != nil {
		return nil, err
	}
	executor := plannerStringArg(args, "executor")
	if executor != workflowTaskExecutorChild {
		executor = workflowTaskExecutorSelf
	}
	updated, err := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{Status: new("IN_PROGRESS"), Executor: new(executor)})
	if err != nil {
		return nil, err
	}
	t.currentTaskID = updated.ID
	parent, tasks, jftradeErr16 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr16)
	jftradeErr11 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr11)
	return map[string]any{"success": true, "task": taskToolTaskSummary(updated)}, nil
}

func (t *workflowTaskToolset) complete(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), false)
	if err != nil {
		return nil, err
	}
	switch strings.ToUpper(strings.TrimSpace(task.Status)) {
	case "DONE", "CANCELLED", "BLOCKED":
		return map[string]any{
			"success": false, "message": "task is not completable in its current status",
			"taskId": task.ID, "status": task.Status,
		}, nil
	}
	if task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) != "" {
		child, ok, childErr := t.executor.runtime.store.Run(context.Background(), task.RunID)
		if childErr != nil {
			return nil, childErr
		}
		if !ok || !isDirectWorkflowChild(parent, child) {
			return map[string]any{"success": false, "message": "delegated child run is unavailable", "taskId": task.ID}, nil
		}
		if child.Status != RunStatusCompleted {
			return map[string]any{
				"success": false, "message": "delegated task cannot be completed before its child run succeeds",
				"taskId": task.ID, "childRunId": child.ID, "childStatus": child.Status,
			}, nil
		}
	}
	summary := plannerStringArg(args, "resultSummary")
	if summary == "" {
		summary = plannerStringArg(args, "summary")
	}
	if summary == "" {
		summary = workflowSelfTaskSummary(task)
	}
	updated, err := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Status: new("DONE"), Executor: new(defaultString(task.Executor, workflowTaskExecutorSelf)), ResultSummary: new(summary),
	})
	if err != nil {
		return nil, err
	}
	t.currentTaskID = ""
	parent, tasks, jftradeErr12 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr12)
	jftradeErr14 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr14)
	return map[string]any{"success": true, "task": taskToolTaskSummary(updated)}, nil
}

func (t *workflowTaskToolset) block(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), false)
	if err != nil {
		return nil, err
	}
	reason := plannerStringArg(args, "reason")
	if reason == "" {
		reason = "任务被阻塞。"
	}
	updated, err := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Status: new("BLOCKED"), ResultSummary: new(reason),
	})
	if err != nil {
		return nil, err
	}
	parent, tasks, jftradeErr20 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr20)
	jftradeErr18 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr18)
	return map[string]any{"success": true, "task": taskToolTaskSummary(updated)}, nil
}

func (t *workflowTaskToolset) delegate(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), true)
	if err != nil {
		return nil, err
	}
	if task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) != "" {
		child, ok, childErr := t.executor.runtime.store.Run(context.Background(), task.RunID)
		if childErr != nil {
			return nil, childErr
		}
		if ok && isDirectWorkflowChild(parent, child) && (child.Status == RunStatusPending || child.Status == RunStatusRunning) {
			return map[string]any{
				"success": true, "taskId": task.ID, "childRunId": child.ID, "status": child.Status,
				"pendingApproval": child.Status == RunStatusPending, "result": strings.TrimSpace(child.Message),
				"reused": true,
			}, nil
		}
	}
	step := workflowStepFromTask(task)
	if prompt := plannerStringArg(args, "prompt"); prompt != "" {
		step.Message = prompt
	}
	if role := plannerStringArg(args, "agentRole"); role != "" {
		step.AgentRole = role
	}
	if providerID := plannerStringArg(args, "childProviderId"); providerID != "" {
		step.ChildProviderID = providerID
	}
	if modelName := plannerStringArg(args, "childModel"); modelName != "" {
		step.ChildModel = modelName
	}
	_, jftradeErr31 := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Executor: new(workflowTaskExecutorChild), ChildProviderID: &step.ChildProviderID, ChildModel: &step.ChildModel,
	})
	jftradeLogError(jftradeErr31)
	result := t.executor.runChild(context.Background(), t.req, parent, step, task, workflowTaskIteration(task))
	if result.Err != nil {
		return map[string]any{"success": false, "message": result.Err.Error()}, nil
	}
	parent, ok, err := t.executor.runtime.store.Run(context.Background(), parent.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("parent run not found")
	}
	parent = t.executor.mergeTaskChildProjectionAt(context.Background(), parent, result.Response.Run, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
	if result.Response.Run.Status == RunStatusPending {
		parent = pauseParentForChild(parent, result.Response.Run, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
		_, jftradeErr30 := t.executor.runtime.saveRunPreservingUserGoalPause(context.Background(), parent)
		jftradeLogError(jftradeErr30)
	}
	t.currentTaskID = ""
	return map[string]any{
		"success": true, "taskId": task.ID, "childRunId": result.Response.Run.ID, "status": result.Response.Run.Status,
		"pendingApproval": result.Response.Run.Status == RunStatusPending, "result": strings.TrimSpace(result.Response.Reply),
	}, nil
}

func (t *workflowTaskToolset) goalComplete(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	if blockers := t.workflowCompletionBlockers(context.Background(), parent, tasks); len(blockers) > 0 {
		return map[string]any{
			"success":  false,
			"status":   "blocked",
			"message":  "goal cannot complete while workflow tasks or child runs are unfinished",
			"blockers": blockers,
		}, nil
	}
	summary := plannerStringArg(args, "summary")
	if summary == "" {
		summary = plannerStringArg(args, "resultSummary")
	}
	t.req.GoalDecision.setComplete(summary)
	jftradeErr17 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr17)
	return map[string]any{"success": true, "status": "complete", "summary": summary}, nil
}

func (t *workflowTaskToolset) workflowCompletionBlockers(ctx context.Context, parent Run, tasks []Task) []map[string]any {
	blockers := make([]map[string]any, 0)
	pendingApprovalRuns := map[string]struct{}{}
	if approvals, err := t.executor.runtime.store.ListApprovals(ctx); err == nil {
		for _, approval := range approvals {
			if approval.Status == ApprovalStatusPending {
				pendingApprovalRuns[strings.TrimSpace(approval.RunID)] = struct{}{}
			}
		}
	}
	for _, task := range tasks {
		status := strings.ToUpper(strings.TrimSpace(task.Status))
		if status != "DONE" && status != "CANCELLED" {
			blockers = append(blockers, map[string]any{"type": "task", "id": task.ID, "status": status})
		}
		if task.Executor != workflowTaskExecutorChild || strings.TrimSpace(task.RunID) == "" {
			continue
		}
		child, ok, err := t.executor.runtime.store.Run(ctx, task.RunID)
		if err != nil || !ok || !isDirectWorkflowChild(parent, child) {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": task.RunID, "status": "MISSING"})
			continue
		}
		if child.Status != RunStatusCompleted {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": child.ID, "status": child.Status})
			continue
		}
		if _, pending := pendingApprovalRuns[child.ID]; pending || t.executor.runtime.runExecutionInFlight(child.ID) {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": child.ID, "status": "STILL_ACTIVE"})
		}
	}
	known := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if strings.TrimSpace(task.RunID) != "" {
			known[task.RunID] = struct{}{}
		}
	}
	for _, childRunID := range parent.ChildRunIDs {
		childRunID = strings.TrimSpace(childRunID)
		if childRunID == "" || childRunID == parent.ID {
			continue
		}
		if _, ok := known[childRunID]; ok {
			continue
		}
		child, ok, err := t.executor.runtime.store.Run(ctx, childRunID)
		_, pending := pendingApprovalRuns[childRunID]
		if err != nil || !ok || !isDirectWorkflowChild(parent, child) || child.Status != RunStatusCompleted || pending || t.executor.runtime.runExecutionInFlight(childRunID) {
			status := "MISSING"
			if ok {
				status = child.Status
			}
			blockers = append(blockers, map[string]any{"type": "child_run", "id": childRunID, "status": status})
		}
	}
	return blockers
}

func (t *workflowTaskToolset) goalContinue(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	reason := plannerStringArg(args, "reason")
	if reason == "" {
		reason = "目标尚未完成。"
	}
	t.req.GoalDecision.setContinue(reason)
	parent, tasks, jftradeErr21 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr21)
	jftradeErr19 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr19)
	return map[string]any{"success": true, "status": "continue", "reason": reason}, nil
}

func (t *workflowTaskToolset) parentAndTasks(ctx context.Context) (Run, []Task, error) {
	parent, ok, err := t.executor.runtime.store.Run(ctx, t.parentID)
	if err != nil {
		return Run{}, nil, err
	}
	if !ok {
		return Run{}, nil, fmt.Errorf("parent run not found")
	}
	tasks, err := t.executor.workflowTasks(ctx, parent, nil)
	if err != nil {
		return Run{}, nil, err
	}
	return parent, tasks, nil
}

func (t *workflowTaskToolset) saveParentPlan(ctx context.Context, parent Run, tasks []Task) error {
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	_, err := t.executor.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	return err
}

func (t *workflowTaskToolset) taskByID(ctx context.Context, id string) (Task, bool) {
	if strings.TrimSpace(id) == "" {
		return Task{}, false
	}
	task, ok, err := t.executor.runtime.store.Task(ctx, id)
	if err != nil {
		return Task{}, false
	}
	return task, ok
}

func (t *workflowTaskToolset) resolveTask(ctx context.Context, parent Run, tasks []Task, id string, allowReady bool) (Task, error) {
	if strings.TrimSpace(id) != "" {
		task, ok := t.taskByID(ctx, id)
		if !ok {
			return Task{}, fmt.Errorf("task not found: %s", id)
		}
		return task, nil
	}
	if task, ok := t.taskByID(ctx, t.currentTaskID); ok && task.Status != "DONE" && task.Status != "CANCELLED" {
		return task, nil
	}
	for _, task := range tasks {
		if task.Status == "IN_PROGRESS" {
			return task, nil
		}
	}
	if allowReady {
		ready := executableWorkflowTasks(tasks, parent.WorkMode)
		if len(ready) > 0 {
			return ready[0], nil
		}
	}
	return Task{}, fmt.Errorf("no executable workflow task")
}

func (e *WorkflowExecutor) mergeTaskChildProjectionAt(ctx context.Context, parent Run, child Run, index int) Run {
	if strings.TrimSpace(child.ID) != "" {
		parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
	}
	parent = updateWorkflowPlanForChildAt(parent, child, index)
	if child.Status == RunStatusPending {
		parent.Status = RunStatusPending
		parent.PendingApprovals = append([]Approval(nil), child.PendingApprovals...)
	}
	parent, jftradeErr28 := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	jftradeLogError(jftradeErr28)
	return parent
}

func workflowTaskToolDescriptors() []ToolDescriptor {
	return []ToolDescriptor{
		{Name: workflowTasksListTool, DisplayName: "列出工作流任务", Description: "列出当前任务 DAG 和可执行 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskAddTool, DisplayName: "新增工作流任务", Description: "运行中新增一个 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskClaimTool, DisplayName: "领取工作流任务", Description: "领取一个可执行 TODO。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskCompleteTool, DisplayName: "完成工作流任务", Description: "完成一个 TODO 并写入结果摘要。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskBlockTool, DisplayName: "阻塞工作流任务", Description: "标记一个 TODO 被阻塞。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowTaskDelegateTool, DisplayName: "委派子智能体", Description: "将一个 TODO 委派给 ADK 子智能体。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowModelsListTool, DisplayName: "查询子智能体模型", Description: "列出可供委派子智能体使用的 ADK 模型。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowGoalCompleteTool, DisplayName: "完成目标", Description: "声明目标已经完成并退出目标循环。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
		{Name: workflowGoalContinueTool, DisplayName: "继续目标", Description: "声明目标尚未完成并继续目标循环。", Category: "workflow", Permission: "workflow_internal", RiskLevel: "low", AllowedModes: allPermissionModes()},
	}
}

func allPermissionModes() []string {
	return []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll}
}

func taskOrchestratorInstruction(base string) string {
	var builder strings.Builder
	builder.WriteString("JFTRADE_TASK_ORCHESTRATOR\n你是 ADK 任务模式的主控调度智能体。你必须通过 workflow.task.* 工具推进 TODO DAG，可以亲自完成任务，也可以在确有必要时调用 workflow.task.delegate 委派给子智能体。不要直接调用业务工具；业务工具只能由被委派的子智能体使用。需要为子智能体选择不同模型时，先调用 workflow.models.list 查询可调用模型，再把 childProviderId 和可选 childModel 传给委派工具。所有新增任务必须通过 workflow.task.add。完成前必须确认没有未完成任务、等待审批或运行中的子智能体。")
	if strings.TrimSpace(base) != "" {
		builder.WriteString("\n\n基础 Agent 指令：")
		builder.WriteString(strings.TrimSpace(base))
	}
	return builder.String()
}

func goalOrchestratorInstruction(base string) string {
	var builder strings.Builder
	builder.WriteString("JFTRADE_GOAL_ORCHESTRATOR\n你是目标模式主控调度智能体。目标模式是任务模式的扩展：你必须通过 workflow.task.* 工具推进 TODO DAG，可以亲自完成任务、增加后续 TODO、阻塞无法完成的任务，或在确有必要时委派子智能体。不要直接调用业务工具；业务工具只能由被委派的子智能体使用。需要为子智能体选择不同模型时，先调用 workflow.models.list 查询可调用模型，再把 childProviderId 和可选 childModel 传给委派工具。收到“是否完成目标”追问时，必须调用 workflow.goal.complete 或 workflow.goal.continue 二选一；不要只输出文字。")
	if strings.TrimSpace(base) != "" {
		builder.WriteString("\n\n基础 Agent 指令：")
		builder.WriteString(strings.TrimSpace(base))
	}
	return builder.String()
}

func taskOrchestratorUserMessage(parent Run) string {
	return fmt.Sprintf("请推进这个任务编排。\n总体目标：%s\n用户请求：%s", strings.TrimSpace(parent.Objective), strings.TrimSpace(parent.UserMessage))
}

func taskOrchestratorNudge(parent Run) string {
	return fmt.Sprintf("仍有未完成 TODO。请调用 workflow.tasks.list 检查状态，然后继续完成、委派、阻塞或新增任务。若所有任务已完成，请输出最终答复。\n总体目标：%s", strings.TrimSpace(parent.Objective))
}

func goalOrchestratorUserMessage(parent Run) string {
	return fmt.Sprintf("请推进这个目标。你可以使用 workflow.task.* 工具维护 TODO DAG，并在本轮完成可见回复后等待系统追问再裁决目标是否完成。\n总体目标：%s\n用户请求：%s", strings.TrimSpace(parent.Objective), strings.TrimSpace(parent.UserMessage))
}

func goalDecisionPrompt(parent Run, lastReply string, retry bool) string {
	prefix := "请判断是否完成目标"
	if retry {
		prefix = "上一次没有调用目标裁决工具。现在必须调用 workflow.goal.complete 或 workflow.goal.continue"
	}
	return fmt.Sprintf("%s：“%s”。\n上一轮可见回复：%s\n如果目标已完成，调用 workflow.goal.complete 并给出 summary；如果尚未完成，调用 workflow.goal.continue 并给出 reason。不要只输出文字。", prefix, strings.TrimSpace(parent.Objective), strings.TrimSpace(lastReply))
}

func goalFinalReplyPrompt(parent Run) string {
	return fmt.Sprintf("所有当前工作步骤已经返回，但还没有形成最终可见答复。请总结本轮结果并直接回复用户；本轮不要再调用工具。\n当前目标：%s", strings.TrimSpace(parent.Objective))
}

func goalTurnHasFinalReply(execution *googleADKExecution, runID string, visibleReply string) bool {
	if execution == nil || strings.TrimSpace(visibleReply) == "" || execution.activeToolCallCountForRun(runID) > 0 {
		return false
	}
	execution.mu.Lock()
	defer execution.mu.Unlock()
	if len(execution.callsForRunLocked(runID)) == 0 {
		return true
	}
	return execution.runHasPostToolTextLocked(runID)
}

func goalOrchestratorContinueNudge(parent Run, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "目标尚未完成。"
	}
	return fmt.Sprintf("目标尚未完成，原因：%s\n请调用 workflow.tasks.list 检查状态，然后继续完成、委派、阻塞或新增 TODO。完成本轮可见回复后等待系统再次询问目标是否完成。\n当前目标：%s", reason, strings.TrimSpace(parent.Objective))
}

func workflowTaskResultSummaries(tasks []Task) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if summary := strings.TrimSpace(task.ResultSummary); summary != "" {
			out = append(out, summary)
		}
	}
	return out
}

func taskToolTaskSummaries(tasks []Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, taskToolTaskSummary(task))
	}
	return out
}

func taskToolTaskSummary(task Task) map[string]any {
	return map[string]any{
		"id": task.ID, "title": task.Title, "status": task.Status, "order": task.Order,
		"dependsOn": task.DependsOn, "executor": task.Executor, "runId": task.RunID,
		"agentRole": task.AgentRole, "planSource": task.PlanSource, "resultSummary": task.ResultSummary,
		"childProviderId": task.ChildProviderID, "childModel": task.ChildModel,
	}
}

func plannerStringSliceArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	values, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func emptyObjectSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}

func workflowTaskAddSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
		"dependsOn": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "agentRole": map[string]any{"type": "string"}, "modeHint": map[string]any{"type": "string"},
		"childProviderId": map[string]any{"type": "string"}, "childModel": map[string]any{"type": "string"},
	}, "required": []string{"title"}, "additionalProperties": false}
}

func workflowTaskClaimSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "executor": map[string]any{"type": "string", "enum": []string{workflowTaskExecutorSelf, workflowTaskExecutorChild}}}, "additionalProperties": false}
}

func workflowTaskCompleteSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "resultSummary": map[string]any{"type": "string"}, "summary": map[string]any{"type": "string"}}, "additionalProperties": false}
}

func workflowTaskBlockSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}}, "additionalProperties": false}
}

func workflowTaskDelegateSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"taskId": map[string]any{"type": "string"}, "prompt": map[string]any{"type": "string"}, "agentRole": map[string]any{"type": "string"},
		"childProviderId": map[string]any{"type": "string"}, "childModel": map[string]any{"type": "string"},
	}, "additionalProperties": false}
}

func workflowModelsListSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"query": map[string]any{"type": "string"}, "providerId": map[string]any{"type": "string"},
		"callableOnly": map[string]any{"type": "boolean"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
	}, "additionalProperties": false}
}

func workflowGoalCompleteSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string"}, "resultSummary": map[string]any{"type": "string"}}, "additionalProperties": false}
}

func workflowGoalContinueSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"reason": map[string]any{"type": "string"}}, "additionalProperties": false}
}

var _ adktool.Toolset = (*workflowTaskToolset)(nil)
