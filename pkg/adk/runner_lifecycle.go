package adk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func runTimeoutForRun(run Run) time.Duration {
	if run.MaxDurationMs > 0 {
		return time.Duration(run.MaxDurationMs) * time.Millisecond
	}
	return DefaultRunTimeout
}

// reconcileStaleRuns reclassifies unfinished runs from a previous process

func (r *Runtime) reconcileStaleRuns(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	runs, err := r.store.ListRuns(ctx)
	if err != nil {
		return
	}
	for _, run := range runs {
		if run.Status != RunStatusRunning && run.Status != RunStatusPending {
			continue
		}
		if isWorkflowParentRun(run) {
			continue
		}
		originalStatus := run.Status
		if originalStatus == RunStatusPending && runHasRecoverableApprovalContext(run) {
			continue
		}
		if originalStatus == RunStatusRunning && runHasRecoverableResolvedApprovalContext(run) {
			continue
		}
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
		_ = r.store.SaveRun(ctx, run)
		r.audit(ctx, "run.orphaned", run.ID, "Agent run became unrecoverable after server restart.", map[string]any{
			"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState,
		})
	}
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
		_ = r.store.SaveRun(ctx, run)
		r.audit(ctx, "run.timed_out", run.ID, "Agent run timed out.", map[string]any{
			"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "errorCode": run.ErrorCode, "failureReason": run.FailureReason,
		})
	}
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
}

func (r *Runtime) startRun(ctx context.Context, sessionID string, agent Agent, text string) (Run, context.Context, func(), error) {
	return r.startRunWithOptions(ctx, sessionID, agent, text, runStartOptions{WorkMode: agent.WorkMode})
}

func (r *Runtime) startRunWithOptions(ctx context.Context, sessionID string, agent Agent, text string, options runStartOptions) (Run, context.Context, func(), error) {
	now := nowString()
	timeout := r.runtimeLimits().RunTimeout
	workMode := normalizeWorkMode(options.WorkMode)
	providerName, modelName := r.runModelSnapshot(ctx, agent)
	run := Run{
		ID: "run-" + uuid.NewString(), SessionID: sessionID, AgentID: agent.ID, ProviderID: strings.TrimSpace(agent.ProviderID),
		ProviderName: providerName, Model: modelName,
		MaxDurationMs: timeout.Milliseconds(),
		Status:        RunStatusRunning, UserMessage: text, Message: "running",
		WorkMode: workMode, Objective: strings.TrimSpace(options.Objective),
		ParentRunID: strings.TrimSpace(options.ParentRunID), ChildRunIDs: normalizeStringSlice(options.ChildRunIDs),
		Iteration: options.Iteration, WorkflowStatus: strings.TrimSpace(options.WorkflowStatus),
		CreatedAt: now, StartedAt: now, UpdatedAt: now,
		ToolCalls: []ToolCall{}, PendingApprovals: []Approval{},
		Usage: &RunUsage{},
	}
	if err := r.store.SaveRun(ctx, run); err != nil {
		return Run{}, nil, nil, err
	}
	r.audit(ctx, "run.started", run.ID, "Agent run started.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "providerId": run.ProviderID, "status": run.Status, "maxDurationMs": run.MaxDurationMs,
	})
	runCtx, cancel := context.WithTimeout(ctx, timeout)
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
	if run.Status != RunStatusRunning && run.Status != RunStatusPending {
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
	run.Message = "cancelled"
	run.FailureReason = "run was cancelled by user"
	run.ErrorCode = "RUN_CANCELLED"
	if isWorkflowParentRun(run) {
		run.WorkflowStatus = workflowStatusFailed
		for index := range run.WorkflowPlan {
			if run.WorkflowPlan[index].Status != "DONE" {
				run.WorkflowPlan[index].Status = "BLOCKED"
			}
		}
	}
	for index := range run.PendingApprovals {
		if run.PendingApprovals[index].Status == ApprovalStatusPending {
			resolved, changed, resolveErr := r.store.ResolvePendingApproval(ctx, run.PendingApprovals[index].ID, ApprovalStatusDenied)
			if resolveErr == nil && changed {
				run.PendingApprovals[index] = resolved
			}
		}
	}
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
	if err := r.store.SaveRun(ctx, run); err != nil {
		return Run{}, err
	}
	for _, childRunID := range run.ChildRunIDs {
		if strings.TrimSpace(childRunID) == "" || childRunID == run.ID {
			continue
		}
		_, _ = r.CancelRun(ctx, childRunID)
	}
	r.audit(ctx, "run.cancelled", run.ID, "Agent run cancelled.", map[string]any{
		"runId": run.ID, "sessionId": run.SessionID, "agentId": run.AgentID, "status": run.Status,
	})
	return run, nil
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

func runStatusForContext(ctx context.Context, err error) string {
	if err == nil {
		return RunStatusCompleted
	}
	if ctx != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return RunStatusTimedOut
		}
		if ctx.Err() == context.Canceled {
			return RunStatusCancelled
		}
	}
	return RunStatusFailed
}

func runErrorCode(status string) string {
	switch status {
	case RunStatusTimedOut:
		return "RUN_TIMED_OUT"
	case RunStatusCancelled:
		return "RUN_CANCELLED"
	default:
		return "MODEL_CALL_FAILED"
	}
}

func runLifecycleAuditKind(status string) string {
	switch status {
	case RunStatusTimedOut:
		return "run.timed_out"
	case RunStatusCancelled:
		return "run.cancelled"
	case RunStatusDenied:
		return "run.denied"
	case RunStatusFailed:
		return "run.failed"
	default:
		return "run.completed"
	}
}

func finalizeRunUsage(run *Run) {
	if run.Usage == nil {
		return
	}
	if run.StartedAt != "" && run.CompletedAt != nil {
		if started, err := time.Parse(time.RFC3339Nano, run.StartedAt); err == nil {
			if completed, err := time.Parse(time.RFC3339Nano, *run.CompletedAt); err == nil {
				run.Usage.DurationMs = completed.Sub(started).Milliseconds()
			}
		}
	}
}

func toolSummariesForRun(run Run) []string {
	summaries := make([]string, 0, len(run.ToolCalls))
	for _, call := range run.ToolCalls {
		if call.Status == "SUCCEEDED" {
			summaries = append(summaries, summarizeToolOutput(call.ToolName, call.Output))
		}
		if call.Status == "FAILED" && call.Error != nil {
			summaries = append(summaries, fmt.Sprintf("%s failed: %s", call.ToolName, *call.Error))
		}
		if call.Status == "DENIED" {
			summaries = append(summaries, fmt.Sprintf("%s denied by user", call.ToolName))
		}
	}
	return summaries
}

func optimizationTaskID(calls []ToolCall) string {
	for _, call := range calls {
		if call.ToolName != "strategy.optimize" || call.Status != "SUCCEEDED" {
			continue
		}
		if output, ok := call.Output.(map[string]any); ok {
			if taskID, ok := output["taskId"].(string); ok {
				return strings.TrimSpace(taskID)
			}
		}
	}
	return ""
}
