package adk

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

func runTimeoutForRun(run Run) time.Duration {
	if run.MaxDurationMs > 0 {
		return time.Duration(run.MaxDurationMs) * time.Millisecond
	}
	return DefaultRunTimeout
}
func (r *Runtime) reconcileStaleRuns(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	runs, err := r.store.ListRuns(ctx)
	if err != nil {
		return
	}
	executor := &WorkflowExecutor{runtime: r}
	for _, run := range runs {
		r.reconcileStaleRun(ctx, executor, run)
	}
}

func (r *Runtime) reconcileStaleRun(ctx context.Context, executor *WorkflowExecutor, run Run) {
	latest, ok := r.loadStaleRunForReconcile(ctx, run.ID)
	if !ok {
		return
	}
	run = latest
	if repaired, repairErr := r.repairWorkflowSelfReference(ctx, &run); repairErr != nil {
		jftradeLogError(repairErr)
		return
	} else if repaired {
		return
	}
	if r.reconcileCompletedWorkflowParent(ctx, executor, run) {
		return
	}
	if r.reconcileTerminalStaleRun(ctx, executor, run) {
		return
	}
	if !isRecoverableReconcileStatus(run.Status) || r.cancelChildOfTerminalParent(ctx, run) {
		return
	}
	if run.Status == RunStatusPaused || isWorkflowParentRun(run) {
		return
	}
	if run.Status == RunStatusPending && runHasRecoverableApprovalContext(run) {
		return
	}
	if run.Status == RunStatusRunning && runHasRecoverableResolvedApprovalContext(run) {
		return
	}
	if r.isDormantWorkflowChildRun(ctx, run) {
		return
	}
	r.failUnrecoverableStaleRun(ctx, run)
}

func (r *Runtime) loadStaleRunForReconcile(ctx context.Context, runID string) (Run, bool) {
	latest, ok, err := r.store.Run(ctx, runID)
	if err != nil || !ok {
		jftradeLogError(err)
		return Run{}, false
	}
	return latest, true
}

func (r *Runtime) reconcileCompletedWorkflowParent(ctx context.Context, executor *WorkflowExecutor, run Run) bool {
	if !isCompletedRunningWorkflowParent(run) {
		return false
	}
	_, _, err := executor.reconcileWorkflowChildren(ctx, run)
	jftradeLogError(err)
	return true
}

func (r *Runtime) reconcileTerminalStaleRun(ctx context.Context, executor *WorkflowExecutor, run Run) bool {
	if !isTerminalLifecycleRunStatus(run.Status) {
		return false
	}
	changed := len(run.PendingApprovals) > 0
	run.PendingApprovals = nil
	if isWorkflowParentRun(run) {
		if tasks, err := executor.workflowTasks(ctx, run, nil); err == nil && len(tasks) > 0 {
			refreshed := workflowPlanFromTasks(tasks, run.WorkflowPlan)
			if !reflect.DeepEqual(refreshed, run.WorkflowPlan) {
				run.WorkflowPlan = refreshed
				changed = true
			}
		}
		r.cancelUnfinishedWorkflowChildren(ctx, run)
	}
	if changed {
		jftradeErr := r.store.SaveRunAndDenyPendingApprovals(ctx, run)
		jftradeLogError(jftradeErr)
	}
	return true
}

func isRecoverableReconcileStatus(status string) bool {
	return status == RunStatusRunning || status == RunStatusPending || status == RunStatusPaused
}

func (r *Runtime) cancelChildOfTerminalParent(ctx context.Context, run Run) bool {
	parentID := strings.TrimSpace(run.ParentRunID)
	if parentID == "" {
		return false
	}
	parent, ok, err := r.store.Run(ctx, parentID)
	if err != nil || !ok || !isTerminalLifecycleRunStatus(parent.Status) || isCompletedRunningWorkflowParent(parent) {
		return false
	}
	_, terminateErr := r.cancelRunTree(ctx, run, "parent workflow "+parent.ID+" is already terminal", "PARENT_RUN_TERMINATED", "cancelled because parent workflow terminated", "run.parent_terminated")
	jftradeLogError(terminateErr)
	return true
}

func (r *Runtime) failUnrecoverableStaleRun(ctx context.Context, run Run) {
	originalStatus := run.Status
	run.Status = RunStatusFailed
	run.ErrorCode = "RUN_ORPHANED"
	run.Message = "run was interrupted by server restart"
	run.FailureReason = "run was interrupted by server restart before completion"
	run.ResumeState = "restart_unrecoverable"
	run.CompletedAt = new(nowString())
	run.Degraded = true
	if originalStatus == RunStatusPending {
		run.FailureReason = "run was waiting for approval, but its ADK confirmation context could not be recovered after server restart"
		run.ResumeState = "approval_context_missing"
	}
	finalizeRunUsage(&run)
	jftradeErr := r.store.SaveRun(ctx, run)
	jftradeLogError(jftradeErr)
	r.audit(ctx, "run.orphaned", run.ID, "Agent run became unrecoverable after server restart.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState,
	})
}

func (r *Runtime) ReconcileExpiredRuns(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	runs, err := r.store.ListRuns(ctx)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, run := range runs {
		if run.Status != RunStatusRunning {
			continue
		}
		startedAt := strings.TrimSpace(run.StartedAt)
		if startedAt == "" {
			startedAt = strings.TrimSpace(run.CreatedAt)
		}
		started, parseErr := time.Parse(time.RFC3339Nano, startedAt)
		timeout := runTimeoutForRun(run)
		if parseErr != nil || now.Sub(started) < timeout {
			continue
		}
		if r.isDormantWorkflowChildRun(ctx, run) {
			continue
		}
		r.activeMu.Lock()
		cancel := r.activeRuns[run.ID]
		delete(r.activeRuns, run.ID)
		r.activeMu.Unlock()
		if cancel != nil {
			cancel()
		}
		for index := range run.ToolCalls {
			call := &run.ToolCalls[index]
			if call.Status != "RUNNING" {
				continue
			}
			call.Status = "FAILED"
			call.Error = new("run timed out while waiting for model or tool completion")
			finishToolCall(call)
		}
		timeoutText := timeout.String()
		run.Status = RunStatusTimedOut
		run.Message = "run timed out"
		run.FailureReason = "run exceeded maximum duration of " + timeoutText
		run.ErrorCode = runErrorCode(RunStatusTimedOut)
		run.Degraded = true
		run.CompletedAt = new(nowString())
		finalizeRunUsage(&run)
		jftradeErr1 := r.store.SaveRun(ctx, run)
		jftradeLogError(jftradeErr1)
		r.audit(ctx, "run.timed_out", run.ID, "Agent run timed out.", map[string]any{
			"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "errorCode": run.ErrorCode, "failureReason": run.FailureReason,
		})
	}
}

func (r *Runtime) isDormantWorkflowChildRun(ctx context.Context, run Run) bool {
	if !workflowChildRunHasNoExecutionActivity(run) {
		return false
	}
	parentID := strings.TrimSpace(run.ParentRunID)
	if parentID == "" || r == nil || r.store == nil {
		return false
	}
	parent, ok, err := r.store.Run(ctx, parentID)
	if err != nil || !ok || !isWorkflowParentRun(parent) || isTerminalLifecycleRunStatus(parent.Status) {
		return false
	}
	return workflowParentReferencesChild(parent, run.ID)
}

func workflowChildRunHasNoExecutionActivity(run Run) bool {
	return strings.TrimSpace(run.ParentRunID) != "" &&
		run.Status == RunStatusRunning &&
		len(run.ToolCalls) == 0 &&
		len(run.PendingApprovals) == 0 &&
		strings.TrimSpace(run.PreToolContent) == "" &&
		strings.TrimSpace(run.PreToolReasoning) == "" &&
		strings.TrimSpace(run.FinalMessageID) == ""
}

func workflowParentReferencesChild(parent Run, childRunID string) bool {
	childRunID = strings.TrimSpace(childRunID)
	if childRunID == "" {
		return false
	}
	for _, id := range parent.ChildRunIDs {
		if strings.TrimSpace(id) == childRunID {
			return true
		}
	}
	for _, step := range parent.WorkflowPlan {
		if strings.TrimSpace(step.ChildRunID) == childRunID {
			return true
		}
	}
	return false
}

type toolExecutionContext struct {
	calls     []ToolCall
	summaries []string
}

type runStartOptions struct {
	WorkMode       string
	Objective      string
	ParentRunID    string
	ChildRunIDs    []string
	Iteration      int
	WorkflowStatus string
	WorkflowEngine string
}

func (r *Runtime) startRun(ctx context.Context, sessionID string, agent Agent, text string) (Run, context.Context, func(), error) {
	return r.startRunWithOptions(ctx, sessionID, agent, text, runStartOptions{WorkMode: agent.WorkMode})
}

func (r *Runtime) startRunWithOptions(ctx context.Context, sessionID string, agent Agent, text string, options runStartOptions) (Run, context.Context, func(), error) {
	resolvedAgent, err := r.resolveAgentProvider(ctx, agent)
	if err != nil {
		return Run{}, nil, nil, err
	}
	agent = resolvedAgent
	now := nowString()
	timeout := r.runtimeLimits().RunTimeout
	workMode := normalizeWorkMode(options.WorkMode)
	providerName, modelName := r.runModelSnapshot(ctx, agent)
	run := Run{
		ID: "run-" + uuid.NewString(), SessionID: sessionID, AgentID: agent.ID, ProviderID: strings.TrimSpace(agent.ProviderID),
		ProviderName: providerName, Model: modelName,
		MaxDurationMs: timeout.Milliseconds(),
		Status:        RunStatusRunning, UserMessage: text, Message: "running",
		WorkMode: workMode, PermissionMode: normalizePermissionMode(agent.PermissionMode), Objective: strings.TrimSpace(options.Objective),
		ParentRunID: strings.TrimSpace(options.ParentRunID), ChildRunIDs: normalizeStringSlice(options.ChildRunIDs),
		Iteration: options.Iteration, WorkflowStatus: strings.TrimSpace(options.WorkflowStatus), WorkflowEngine: strings.TrimSpace(options.WorkflowEngine),
		CreatedAt: now, StartedAt: now, UpdatedAt: now,
		ToolCalls: []ToolCall{}, PendingApprovals: []Approval{},
		Usage: &RunUsage{},
	}
	runContext := adkRunObservabilityContext(ctx, run)
	if err := r.store.SaveRun(runContext, run); err != nil {
		return Run{}, nil, nil, err
	}
	r.audit(runContext, "run.started", run.ID, "Agent run started.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "providerId": run.ProviderID, "status": run.Status, "maxDurationMs": run.MaxDurationMs,
	})
	observability.InfoWithImportance(runContext, observability.ImportanceNormal, "adk run started", "agent_id", run.AgentID, "status", run.Status)
	runCtx, cancel := context.WithTimeout(runContext, timeout)
	r.activeMu.Lock()
	r.activeRuns[run.ID] = cancel
	r.activeMu.Unlock()
	finish := func() {
		cancel()
		r.activeMu.Lock()
		delete(r.activeRuns, run.ID)
		r.activeMu.Unlock()
	}
	return run, runCtx, finish, nil
}

func adkRunObservabilityContext(ctx context.Context, run Run) context.Context {
	return observability.WithFields(ctx, observability.Fields{
		SessionID:  run.SessionID,
		RunID:      run.ID,
		ProviderID: run.ProviderID,
		Source:     "adk",
	})
}

func (r *Runtime) runModelSnapshot(ctx context.Context, agent Agent) (string, string) {
	if r == nil || r.store == nil || strings.TrimSpace(agent.ProviderID) == "" {
		return "", strings.TrimSpace(agent.Model)
	}
	provider, ok, err := r.store.Provider(ctx, agent.ProviderID)
	if err != nil || !ok {
		return "", strings.TrimSpace(agent.Model)
	}
	return provider.DisplayName, defaultString(agent.Model, provider.Model)
}

func (r *Runtime) CancelRun(ctx context.Context, runID string) (Run, error) {
	r.ReconcileExpiredRuns(ctx)
	run, ok, err := r.store.Run(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if !ok {
		return Run{}, fmt.Errorf("run not found")
	}
	if run.Status != RunStatusRunning && run.Status != RunStatusPending && run.Status != RunStatusPaused {
		return run, nil
	}
	return r.cancelRunTree(ctx, run, "run was cancelled by user", "RUN_CANCELLED", "cancelled", "run.cancelled")
}

func (r *Runtime) cancelRunTree(ctx context.Context, run Run, reason string, errorCode string, message string, auditKind string) (Run, error) {
	if run.Status != RunStatusRunning && run.Status != RunStatusPending && run.Status != RunStatusPaused {
		return run, nil
	}
	r.activeMu.Lock()
	cancel := r.activeRuns[run.ID]
	r.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}
	cancelledAt := nowString()
	run.Status = RunStatusCancelled
	run.CancelledAt = &cancelledAt
	run.CompletedAt = &cancelledAt
	run.Message = message
	run.FailureReason = reason
	run.ErrorCode = errorCode
	if isWorkflowParentRun(run) {
		run.WorkflowStatus = workflowStatusFailed
		for index := range run.WorkflowPlan {
			if run.WorkflowPlan[index].Status != "DONE" {
				run.WorkflowPlan[index].Status = "BLOCKED"
			}
		}
	}
	run.PendingApprovals = nil
	for index := range run.ToolCalls {
		call := &run.ToolCalls[index]
		switch call.Status {
		case "RUNNING", "PENDING", "PENDING_APPROVAL":
			call.Status = "CANCELLED"
			call.RequiresUser = false
			finishToolCall(call)
		}
	}
	finalizeRunUsage(&run)
	if err := r.store.SaveRunAndDenyPendingApprovals(ctx, run); err != nil {
		return Run{}, err
	}
	childRunIDs := make(map[string]struct{}, len(run.ChildRunIDs))
	for _, childRunID := range run.ChildRunIDs {
		if childRunID = strings.TrimSpace(childRunID); childRunID != "" {
			childRunIDs[childRunID] = struct{}{}
		}
	}
	if runs, err := r.store.ListRuns(ctx); err == nil {
		for _, candidate := range runs {
			if strings.TrimSpace(candidate.ParentRunID) == run.ID {
				childRunIDs[candidate.ID] = struct{}{}
			}
		}
	} else {
		jftradeLogError(err)
	}
	for childRunID := range childRunIDs {
		if strings.TrimSpace(childRunID) == "" || childRunID == run.ID {
			continue
		}
		child, ok, childErr := r.store.Run(ctx, childRunID)
		if childErr != nil {
			jftradeLogError(childErr)
			continue
		}
		if ok {
			_, childErr = r.cancelRunTree(ctx, child, reason, errorCode, message, auditKind)
			jftradeLogError(childErr)
		}
	}
	r.audit(ctx, auditKind, run.ID, "Agent run cancelled.", map[string]any{
		"runId": run.ID, "sessionId": run.SessionID, "agentId": run.AgentID, "status": run.Status,
	})
	return run, nil
}

func (r *Runtime) cancelUnfinishedWorkflowChildren(ctx context.Context, parent Run) {
	childRunIDs := make(map[string]struct{}, len(parent.ChildRunIDs))
	for _, childRunID := range parent.ChildRunIDs {
		if childRunID = strings.TrimSpace(childRunID); childRunID != "" {
			childRunIDs[childRunID] = struct{}{}
		}
	}
	if runs, err := r.store.ListRuns(ctx); err == nil {
		for _, run := range runs {
			if strings.TrimSpace(run.ParentRunID) == parent.ID {
				childRunIDs[run.ID] = struct{}{}
			}
		}
	} else {
		jftradeLogError(err)
	}
	for childRunID := range childRunIDs {
		childRunID = strings.TrimSpace(childRunID)
		if childRunID == "" || childRunID == parent.ID {
			continue
		}
		child, ok, err := r.store.Run(ctx, childRunID)
		if err != nil || !ok {
			jftradeLogError(err)
			continue
		}
		_, err = r.cancelRunTree(ctx, child, "parent workflow "+parent.ID+" terminated", "PARENT_RUN_TERMINATED", "cancelled because parent workflow terminated", "run.parent_terminated")
		jftradeLogError(err)
	}
}

func (r *Runtime) runExecutionInFlight(runID string) bool {
	if r == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	runID = strings.TrimSpace(runID)
	r.activeMu.Lock()
	_, active := r.activeRuns[runID]
	r.activeMu.Unlock()
	if active {
		return true
	}
	r.approvalMu.Lock()
	_, resuming := r.approvalRuns[runID]
	r.approvalMu.Unlock()
	return resuming
}

func (r *Runtime) repairWorkflowSelfReference(ctx context.Context, parent *Run) (bool, error) {
	if r == nil || r.store == nil || parent == nil || !isWorkflowParentRun(*parent) {
		return false, nil
	}
	if isTerminalLifecycleRunStatus(parent.Status) && !isCompletedRunningWorkflowParent(*parent) {
		return false, nil
	}
	repaired := false
	for index := range parent.WorkflowPlan {
		state := &parent.WorkflowPlan[index]
		selfReference := strings.TrimSpace(state.ChildRunID) == parent.ID
		if !selfReference && strings.TrimSpace(state.ChildRunID) == "" && state.Executor == workflowTaskExecutorChild && strings.TrimSpace(state.TaskID) != "" {
			task, ok, err := r.store.Task(ctx, state.TaskID)
			if err != nil {
				return false, err
			}
			selfReference = ok && task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) == parent.ID
		}
		if !selfReference {
			continue
		}
		repaired = true
		state.ChildRunID = ""
		state.Executor = ""
		state.ResultSummary = ""
		if state.Status != "DONE" && state.Status != "CANCELLED" {
			state.Status = "TODO"
		}
		if strings.TrimSpace(state.TaskID) != "" {
			status := state.Status
			executor := ""
			resultSummary := ""
			if _, err := r.store.UpdateTask(ctx, state.TaskID, TaskPatchRequest{
				Status: &status, Executor: &executor, ResultSummary: &resultSummary,
			}); err != nil {
				return false, err
			}
		}
	}
	if !repaired {
		return false, nil
	}
	children := make([]string, 0, len(parent.ChildRunIDs))
	for _, childRunID := range parent.ChildRunIDs {
		if strings.TrimSpace(childRunID) != parent.ID {
			children = append(children, childRunID)
		}
	}
	parent.ChildRunIDs = children
	pausedAt := nowString()
	parent.Status = RunStatusPaused
	parent.WorkflowStatus = workflowStatusPaused
	parent.Message = "检测到无效的子智能体引用，已修复并暂停目标。"
	parent.ResumeState = "self_reference_recovered"
	parent.PausedReason = "self_reference_recovered"
	parent.PausedAt = &pausedAt
	parent.CompletedAt = nil
	parent.ErrorCode = ""
	parent.FailureReason = ""
	if _, err := r.saveRunPreservingUserGoalPause(ctx, *parent); err != nil {
		return false, err
	}
	r.audit(ctx, "run.workflow.self_reference_recovered", parent.ID, "Invalid workflow child self-reference was repaired.", map[string]any{
		"runId": parent.ID, "sessionId": parent.SessionID,
	})
	return true, nil
}

func (r *Runtime) PauseGoalRun(ctx context.Context, runID string) (Run, error) {
	if r == nil || r.store == nil {
		return Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	run, ok, err := r.store.Run(ctx, strings.TrimSpace(runID))
	if err != nil {
		return Run{}, err
	}
	if !ok {
		return Run{}, fmt.Errorf("run not found")
	}
	if err := validateUserGoalPauseRun(run); err != nil {
		return Run{}, err
	}
	if run.Status == RunStatusPaused && run.PausedReason == "user" {
		return run, nil
	}
	if run.Status != RunStatusRunning {
		return Run{}, fmt.Errorf("only running goal runs can be paused")
	}
	if run.PauseRequestedAt == nil {
		requestedAt := nowString()
		run.PauseRequestedAt = &requestedAt
	}
	run.ResumeState = "user_pause_requested"
	run.Message = "目标将在当前轮结束后暂停。"
	run.UpdatedAt = nowString()
	if err := r.store.SaveRun(ctx, run); err != nil {
		return Run{}, err
	}
	r.audit(ctx, "run.goal.pause_requested", run.ID, "Goal pause requested.", map[string]any{
		"runId": run.ID, "sessionId": run.SessionID, "agentId": run.AgentID,
	})
	return run, nil
}

func (r *Runtime) ResumeGoalRun(ctx context.Context, runID string) (Run, error) {
	if r == nil || r.store == nil {
		return Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	run, ok, err := r.store.Run(ctx, strings.TrimSpace(runID))
	if err != nil {
		return Run{}, err
	}
	if !ok {
		return Run{}, fmt.Errorf("run not found")
	}
	if err := validateUserGoalResumeRun(run); err != nil {
		return Run{}, err
	}
	now := nowString()
	if run.Status == RunStatusTimedOut {
		run.StartedAt = now
		run.CompletedAt = nil
		run.MaxDurationMs = r.runtimeLimits().RunTimeout.Milliseconds()
	}
	run.Status = RunStatusRunning
	run.WorkflowStatus = workflowStatusRunning
	run.ResumeState = "user_resuming"
	run.Message = "goal resumed"
	run.ErrorCode = ""
	run.FailureReason = ""
	run.Degraded = false
	run.PauseRequestedAt = nil
	run.PausedAt = nil
	run.PausedReason = ""
	run.UpdatedAt = now
	if err := r.store.SaveRun(ctx, run); err != nil {
		return Run{}, err
	}
	r.audit(ctx, "run.goal.resumed", run.ID, "Goal resumed by user.", map[string]any{
		"runId": run.ID, "sessionId": run.SessionID, "agentId": run.AgentID,
	})
	r.resumeUserPausedGoalRun(context.WithoutCancel(ctx), run)
	return run, nil
}

func validateUserGoalPauseRun(run Run) error {
	if strings.TrimSpace(run.ParentRunID) != "" {
		return fmt.Errorf("only root goal runs can be paused")
	}
	if normalizeWorkMode(run.WorkMode) != WorkModeLoop || strings.TrimSpace(run.WorkflowStatus) == "" {
		return fmt.Errorf("only loop goal runs can be paused")
	}
	if isTerminalLifecycleRunStatus(run.Status) {
		return fmt.Errorf("terminal runs cannot be paused")
	}
	if run.Status == RunStatusPaused {
		if run.PausedReason == "user" {
			return nil
		}
		return fmt.Errorf("system-paused runs cannot be paused")
	}
	return nil
}

func validateUserGoalResumeRun(run Run) error {
	if strings.TrimSpace(run.ParentRunID) != "" {
		return fmt.Errorf("only root goal runs can be resumed")
	}
	if normalizeWorkMode(run.WorkMode) != WorkModeLoop || strings.TrimSpace(run.WorkflowStatus) == "" {
		return fmt.Errorf("only loop goal runs can be resumed")
	}
	if run.Status == RunStatusTimedOut {
		return nil
	}
	if run.Status != RunStatusPaused || (run.PausedReason != "user" && run.PausedReason != "iteration_limit" && run.PausedReason != "self_reference_recovered") {
		return fmt.Errorf("only resumable paused goal runs can be resumed")
	}
	return nil
}

func isTerminalLifecycleRunStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case RunStatusCompleted, RunStatusFailed, RunStatusDenied, RunStatusCancelled, RunStatusTimedOut:
		return true
	default:
		return false
	}
}

func isCompletedRunningWorkflowParent(run Run) bool {
	return isWorkflowParentRun(run) &&
		strings.EqualFold(strings.TrimSpace(run.Status), RunStatusCompleted) &&
		strings.EqualFold(strings.TrimSpace(run.WorkflowStatus), workflowStatusRunning)
}

func (r *Runtime) resumeUserPausedGoalRun(ctx context.Context, run Run) {
	go func() {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), runTimeoutForRun(run))
		defer cancel()
		r.activeMu.Lock()
		r.activeRuns[run.ID] = cancel
		r.activeMu.Unlock()
		defer func() {
			r.activeMu.Lock()
			delete(r.activeRuns, run.ID)
			r.activeMu.Unlock()
		}()
		session, agent, err := r.workflowResumeContext(timeoutCtx, run)
		executor := &WorkflowExecutor{runtime: r}
		if err != nil {
			_ = executor.failParent(timeoutCtx, run, err)
			return
		}
		updated, err := executor.resumeADKGoalWorkflow(timeoutCtx, session, agent, run)
		if err != nil {
			_ = executor.failParent(timeoutCtx, run, err)
			return
		}
		_ = updated
	}()
}

func (r *Runtime) UpdateRunObjective(ctx context.Context, runID string, objective string) (Run, error) {
	if r == nil || r.store == nil {
		return Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	trimmed := strings.TrimSpace(objective)
	if trimmed == "" {
		return Run{}, fmt.Errorf("objective is required")
	}
	run, ok, err := r.store.Run(ctx, strings.TrimSpace(runID))
	if err != nil {
		return Run{}, err
	}
	if !ok {
		return Run{}, fmt.Errorf("run not found")
	}
	if normalizeWorkMode(run.WorkMode) != WorkModeLoop {
		return Run{}, fmt.Errorf("objective can only be updated for goal runs")
	}
	if strings.TrimSpace(run.ParentRunID) != "" {
		return Run{}, fmt.Errorf("child run objective cannot be updated")
	}
	if run.Status != RunStatusRunning && run.Status != RunStatusPending {
		return Run{}, fmt.Errorf("objective cannot be updated for terminal run")
	}
	run.Objective = trimmed
	if err := r.store.SaveRun(ctx, run); err != nil {
		return Run{}, err
	}
	r.audit(ctx, "run.objective.updated", run.ID, "Goal objective updated.", map[string]any{
		"runId": run.ID, "sessionId": run.SessionID, "agentId": run.AgentID,
	})
	return run, nil
}

func recentOpenAIMessages(messages []Message, maxMessages int, maxChars int) []openAIChatMessage {
	if maxMessages <= 0 || maxChars <= 0 || len(messages) == 0 {
		return nil
	}
	start := 0
	if len(messages) > maxMessages {
		start = len(messages) - maxMessages
	}
	out := make([]openAIChatMessage, 0, len(messages)-start)
	remaining := maxChars
	for _, message := range messages[start:] {
		role := "assistant"
		if message.Role == "user" {
			role = "user"
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if role == "assistant" && isIntermediateApprovalMessage(content) {
			continue
		}
		if len([]rune(content)) > remaining {
			content = string([]rune(content)[:remaining])
		}
		out = append(out, openAIChatMessage{Role: role, Content: content})
		remaining -= len([]rune(content))
		if remaining <= 0 {
			break
		}
	}
	return out
}

func isIntermediateApprovalMessage(content string) bool {
	return strings.Contains(content, "绛夊緟鐢ㄦ埛瀹℃壒") ||
		strings.Contains(content, "璇峰厛鍦?ADK 瀹℃壒闃熷垪")
}
