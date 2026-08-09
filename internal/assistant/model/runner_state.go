package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ToolExecutionContext carries the tool activity attached to a run result.
type ToolExecutionContext struct {
	Calls        []ToolCall
	Summaries    []string
	InputRequest *InputRequest
}

// RunStartOptions carries the runtime start options for a workflow or chat run.
type RunStartOptions struct {
	WorkMode           string
	Objective          string
	ClientRequestID    string
	RequestFingerprint string
	ParentRunID        string
	ChildRunIDs        []string
	Iteration          int
	WorkflowStatus     string
	WorkflowEngine     string
}

// HydrateRunExecutionResult projects tool activity and run state onto a run
// after an ADK execution turn.
func HydrateRunExecutionResult(
	run Run,
	toolContext ToolExecutionContext,
	approvals []Approval,
	preToolContent string,
	preToolReasoning string,
) Run {
	run.ToolCalls = toolContext.Calls
	run.ToolSummaries = toolContext.Summaries
	run.PreToolContent = preToolContent
	run.PreToolReasoning = preToolReasoning
	run.OptimizationTaskID = OptimizationTaskID(toolContext.Calls)
	run.PendingApprovals = approvals
	run.InputRequest = NormalizeInputRequest(toolContext.InputRequest)
	if run.Usage != nil {
		run.Usage.ToolCallsTotal = len(toolContext.Calls)
	}
	return run
}

// ResolveChatWorkflowOptions derives the effective work mode, run options, and
// objective from a chat request and agent definition.
func ResolveChatWorkflowOptions(req ChatRequest, agent Agent) (string, RunOptions, string, error) {
	if !ValidWorkMode(req.WorkModeOverride) {
		return "", RunOptions{}, "", fmt.Errorf("invalid work mode %q", req.WorkModeOverride)
	}
	mode := NormalizeWorkMode(agent.WorkMode)
	if strings.TrimSpace(req.WorkModeOverride) != "" {
		mode = NormalizeWorkMode(req.WorkModeOverride)
	}
	options := RunOptions{
		LoopMaxIterations: NormalizeLoopMaxIterations(agent.LoopMaxIterations),
	}
	if req.RunOptions != nil {
		if req.RunOptions.LoopMaxIterations > 0 {
			options.LoopMaxIterations = NormalizeLoopMaxIterations(req.RunOptions.LoopMaxIterations)
		}
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = strings.TrimSpace(req.Message)
	}
	return mode, options, objective, nil
}

// ValidateChatOverrides validates work mode and permission mode overrides.
func ValidateChatOverrides(req ChatRequest) (string, error) {
	if !ValidWorkMode(req.WorkModeOverride) {
		return "", fmt.Errorf("invalid work mode %q", req.WorkModeOverride)
	}
	permissionMode := strings.TrimSpace(req.PermissionModeOverride)
	if permissionMode != "" && !ValidPermissionMode(permissionMode) {
		return "", fmt.Errorf("invalid permission mode %q", permissionMode)
	}
	return permissionMode, nil
}

// ApplyChatModelOverride applies per-request provider/model overrides.
func ApplyChatModelOverride(agent Agent, req ChatRequest) Agent {
	if providerID := strings.TrimSpace(req.ProviderID); providerID != "" {
		agent.ProviderID = providerID
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		agent.Model = model
	}
	return agent
}

// MergeRunActivitySnapshot merges non-empty activity fields from a snapshot
// into the authoritative run state.
func MergeRunActivitySnapshot(run *Run, snapshot Run) {
	if run == nil {
		return
	}
	if strings.TrimSpace(snapshot.SessionID) != "" {
		run.SessionID = snapshot.SessionID
	}
	if strings.TrimSpace(snapshot.AgentID) != "" {
		run.AgentID = snapshot.AgentID
	}
	if strings.TrimSpace(snapshot.ProviderID) != "" {
		run.ProviderID = snapshot.ProviderID
	}
	if strings.TrimSpace(snapshot.Status) != "" {
		run.Status = snapshot.Status
	}
	if strings.TrimSpace(snapshot.Message) != "" {
		run.Message = snapshot.Message
	}
	if strings.TrimSpace(snapshot.FailureReason) != "" {
		run.FailureReason = snapshot.FailureReason
	}
	if strings.TrimSpace(snapshot.ErrorCode) != "" {
		run.ErrorCode = snapshot.ErrorCode
	}
	if snapshot.Degraded {
		run.Degraded = true
	}
	if strings.TrimSpace(snapshot.PreToolContent) != "" {
		run.PreToolContent = snapshot.PreToolContent
	}
	if strings.TrimSpace(snapshot.PreToolReasoning) != "" {
		run.PreToolReasoning = snapshot.PreToolReasoning
	}
	if len(snapshot.ToolSummaries) > 0 {
		run.ToolSummaries = append([]string(nil), snapshot.ToolSummaries...)
	}
	if len(snapshot.ToolCalls) > 0 {
		run.ToolCalls = append([]ToolCall(nil), snapshot.ToolCalls...)
	}
	if len(snapshot.PendingApprovals) > 0 {
		run.PendingApprovals = append([]Approval(nil), snapshot.PendingApprovals...)
	}
	if strings.TrimSpace(snapshot.ResumeState) != "" {
		run.ResumeState = snapshot.ResumeState
	}
	if strings.TrimSpace(snapshot.FinalMessageID) != "" {
		run.FinalMessageID = snapshot.FinalMessageID
	}
	if snapshot.Usage != nil {
		run.Usage = new(*snapshot.Usage)
	}
	if strings.TrimSpace(snapshot.StartedAt) != "" {
		run.StartedAt = snapshot.StartedAt
	}
	if snapshot.CompletedAt != nil {
		run.CompletedAt = new(*snapshot.CompletedAt)
	}
	if snapshot.CancelledAt != nil {
		run.CancelledAt = new(*snapshot.CancelledAt)
	}
	if strings.TrimSpace(snapshot.OptimizationTaskID) != "" {
		run.OptimizationTaskID = snapshot.OptimizationTaskID
	}
}

// ProjectedAssistantMessageForRun returns the projected assistant message for
// a run from a session projection.
func ProjectedAssistantMessageForRun(projection SessionProjection, run Run) *TranscriptEntry {
	finalMessageID := strings.TrimSpace(run.FinalMessageID)
	if finalMessageID != "" {
		if message, ok := projection.MessagesByEventID[finalMessageID]; ok {
			return new(message)
		}
	}
	for index := range projection.Messages {
		message := &projection.Messages[index]
		if finalMessageID != "" && message.ID == finalMessageID {
			return message
		}
		if finalMessageID == "" && strings.TrimSpace(message.RunID) == strings.TrimSpace(run.ID) {
			return message
		}
	}
	return nil
}

// ShouldPreferProjectedToolCalls decides whether the projected tool calls are
// more authoritative than the run's current calls.
func ShouldPreferProjectedToolCalls(run Run, projected []ToolCall) bool {
	current := run.ToolCalls
	if len(projected) == 0 {
		return false
	}
	if len(current) == 0 {
		if strings.TrimSpace(run.ParentRunID) == "" && NormalizeWorkMode(run.WorkMode) != WorkModeChat {
			return false
		}
		return true
	}
	projectedTerminal := TerminalToolCallCount(projected)
	currentTerminal := TerminalToolCallCount(current)
	if projectedTerminal != currentTerminal {
		return projectedTerminal > currentTerminal
	}
	projectedPending := PendingApprovalToolCallCount(projected)
	currentPending := PendingApprovalToolCallCount(current)
	if projectedPending != currentPending {
		return projectedPending > currentPending
	}
	return len(projected) > len(current)
}

// TerminalToolCallCount counts terminal tool call statuses.
func TerminalToolCallCount(calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		switch call.Status {
		case "SUCCEEDED", "FAILED", "DENIED", "COMPLETED", "CANCELLED", "TIMED_OUT":
			count++
		}
	}
	return count
}

// PendingApprovalToolCallCount counts tool calls waiting for approval.
func PendingApprovalToolCallCount(calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		if call.Status == "PENDING_APPROVAL" {
			count++
		}
	}
	return count
}

// ApprovalResolutionSummary renders the user-facing summary after an approval
// is approved or denied.
func ApprovalResolutionSummary(run Run, approval Approval, approved bool) string {
	if !approved {
		return fmt.Sprintf("已拒绝工具调用 `%s`。本次 run 已结束，未执行该操作。", approval.ToolName)
	}
	lines := []string{fmt.Sprintf("已批准并执行工具调用 `%s`。", approval.ToolName)}
	for _, call := range run.ToolCalls {
		if call.ToolName != approval.ToolName {
			continue
		}
		if call.Status == "SUCCEEDED" {
			lines = append(lines, "执行结果：")
			lines = append(lines, SummarizeToolOutput(call.ToolName, call.Output))
		}
		if call.Status == "FAILED" && call.Error != nil {
			lines = append(lines, "执行失败："+*call.Error)
		}
	}
	return strings.Join(lines, "\n")
}

// TerminalAuditMessage returns the audit message for a terminal run status.
func TerminalAuditMessage(status string) string {
	if status == RunStatusCompleted {
		return "Agent run completed."
	}
	return "Agent run finished with a terminal status."
}

// TerminalAuditFields returns the audit fields for a terminal run.
func TerminalAuditFields(run Run) map[string]any {
	fields := map[string]any{
		"runId":   run.ID,
		"agentId": run.AgentID,
		"status":  run.Status,
	}
	if run.ErrorCode != "" {
		fields["errorCode"] = run.ErrorCode
	}
	if run.FailureReason != "" {
		fields["failureReason"] = run.FailureReason
	}
	return fields
}

// IsRetryableADKSessionBusy reports whether an ADK session service error is a
// retryable SQLite busy condition.
func IsRetryableADKSessionBusy(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "append event to sessionservice") &&
		(strings.Contains(lower, "database is locked") || strings.Contains(lower, "sqlite_busy"))
}

// RunHasRecoverableApprovalContext reports whether a run has a pending
// approval that can be resumed.
func RunHasRecoverableApprovalContext(run Run) bool {
	for _, approval := range run.PendingApprovals {
		if approval.Status != ApprovalStatusPending {
			continue
		}
		if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
			return true
		}
	}
	return false
}

// RunHasRecoverableResolvedApprovalContext reports whether a run is resuming
// with resolved approvals that can be continued.
func RunHasRecoverableResolvedApprovalContext(run Run) bool {
	if strings.TrimSpace(run.ResumeState) != "approval_resuming" {
		return false
	}
	for _, approval := range run.PendingApprovals {
		if approval.Status == ApprovalStatusPending {
			continue
		}
		if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
			return true
		}
	}
	return false
}

// RunCanContinueResolvedApproval reports whether a run can continue after a
// resolved approval.
func RunCanContinueResolvedApproval(run Run) bool {
	if IsWorkflowParentRun(run) {
		return false
	}
	if run.Status == RunStatusPending {
		return true
	}
	return run.Status == RunStatusRunning && RunHasRecoverableResolvedApprovalContext(run)
}

// IsWorkflowParentRun reports whether a run is a workflow parent.
func IsWorkflowParentRun(run Run) bool {
	return NormalizeWorkMode(run.WorkMode) != WorkModeChat && strings.TrimSpace(run.WorkflowStatus) != ""
}

// RunHasRecoverableAnsweredInputContext reports whether a run is resuming
// with an answered input request.
func RunHasRecoverableAnsweredInputContext(run Run) bool {
	return run.Status == RunStatusRunning &&
		strings.TrimSpace(run.ResumeState) == "input_resuming" &&
		run.InputRequest != nil &&
		run.InputRequest.Status == InputRequestStatusAnswered &&
		strings.TrimSpace(run.InputRequest.FunctionCallID) != ""
}

// DefaultError returns the provided error or a default message error.
func DefaultError(err error, message string) error {
	if err != nil {
		return err
	}
	return errors.New(message)
}

// RunTimeoutForRun returns the run timeout honoring the run-level override.
func RunTimeoutForRun(run Run) time.Duration {
	if run.MaxDurationMs > 0 {
		return time.Duration(run.MaxDurationMs) * time.Millisecond
	}
	return DefaultRunTimeout
}

// IsRecoverableReconcileStatus reports whether a stale run status is
// recoverable during reconciliation.
func IsRecoverableReconcileStatus(status string) bool {
	return status == RunStatusRunning || status == RunStatusPending || status == RunStatusPendingInput || status == RunStatusPaused
}

// WorkflowChildRunHasNoExecutionActivity reports whether a child run has no
// recorded execution activity yet.
func WorkflowChildRunHasNoExecutionActivity(run Run) bool {
	return strings.TrimSpace(run.ParentRunID) != "" &&
		run.Status == RunStatusRunning &&
		len(run.ToolCalls) == 0 &&
		len(run.PendingApprovals) == 0 &&
		strings.TrimSpace(run.PreToolContent) == "" &&
		strings.TrimSpace(run.PreToolReasoning) == "" &&
		strings.TrimSpace(run.FinalMessageID) == ""
}

// WorkflowParentReferencesChild reports whether a parent run references a
// child run ID in its child list or workflow plan.
func WorkflowParentReferencesChild(parent Run, childRunID string) bool {
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
