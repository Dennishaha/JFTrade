package model

import (
	"strings"
)

// UserPauseRequestedGoalParent reports whether a loop-mode parent run has a
// pending user pause request.
func UserPauseRequestedGoalParent(run Run) bool {
	return NormalizeWorkMode(run.WorkMode) == WorkModeLoop &&
		strings.TrimSpace(run.ParentRunID) == "" &&
		run.PauseRequestedAt != nil
}

// UserPausedGoalParent reports whether a loop-mode parent run is paused by
// the user.
func UserPausedGoalParent(run Run) bool {
	return NormalizeWorkMode(run.WorkMode) == WorkModeLoop &&
		strings.TrimSpace(run.ParentRunID) == "" &&
		run.Status == RunStatusPaused &&
		run.PausedReason == "user"
}

// MarkUserPausedGoalParent projects a user pause onto a loop-mode parent run.
func MarkUserPausedGoalParent(run Run) Run {
	pausedAt := NowString()
	run.Status = RunStatusPaused
	run.WorkflowStatus = WorkflowStatusPaused
	if run.PausedAt == nil {
		run.PausedAt = &pausedAt
	}
	run.PausedReason = "user"
	run.ResumeState = "user_paused"
	run.Message = "目标已暂停。"
	run.PendingApprovals = PendingApprovalsOnly(run.PendingApprovals)
	return run
}

// ClassifyToolExecutionError maps a tool execution error to a status and a
// user-facing message.
func ClassifyToolExecutionError(err error) (string, string) {
	if err == nil {
		return "SUCCEEDED", ""
	}
	return ClassifyToolErrorText(err.Error())
}

// ClassifyToolErrorText classifies serialized tool error text.
func ClassifyToolErrorText(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "context deadline exceeded"):
		return "TIMED_OUT", PrefixedToolError(trimmed, "tool execution timed out")
	case strings.Contains(lower, "context canceled"):
		return "CANCELLED", PrefixedToolError(trimmed, "tool execution cancelled")
	default:
		return "FAILED", trimmed
	}
}

// PrefixedToolError prefixes a message unless it already contains the prefix.
func PrefixedToolError(text string, prefix string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return prefix
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, prefix) {
		return trimmed
	}
	return prefix + ": " + trimmed
}

// FirstToolCallByStatus returns the first tool call with any allowed status.
func FirstToolCallByStatus(calls []ToolCall, statuses ...string) *ToolCall {
	if len(calls) == 0 || len(statuses) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[strings.ToUpper(strings.TrimSpace(status))] = struct{}{}
	}
	for index := range calls {
		if _, ok := allowed[strings.ToUpper(strings.TrimSpace(calls[index].Status))]; ok {
			return &calls[index]
		}
	}
	return nil
}

// ToolCallFailureMessage returns the user-facing failure for a tool call.
func ToolCallFailureMessage(call *ToolCall) string {
	if call == nil {
		return ""
	}
	if call.Error != nil && strings.TrimSpace(*call.Error) != "" {
		return strings.TrimSpace(*call.Error)
	}
	switch strings.ToUpper(strings.TrimSpace(call.Status)) {
	case "TIMED_OUT":
		return "tool execution timed out"
	case "CANCELLED":
		return "tool execution cancelled"
	default:
		return "tool execution failed"
	}
}

// FirstToolCallFailure returns the first terminal tool failure message.
func FirstToolCallFailure(run *Run) string {
	if run == nil {
		return ""
	}
	call := FirstToolCallByStatus(run.ToolCalls, "TIMED_OUT", "FAILED", "CANCELLED")
	return ToolCallFailureMessage(call)
}

// WorkflowTaskResultSummaries returns non-empty result summaries in task order.
func WorkflowTaskResultSummaries(tasks []Task) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if summary := strings.TrimSpace(task.ResultSummary); summary != "" {
			out = append(out, summary)
		}
	}
	return out
}

// TaskToolTaskSummaries projects tasks into the workflow task tool payload.
func TaskToolTaskSummaries(tasks []Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, TaskToolTaskSummary(task))
	}
	return out
}

// TaskToolTaskSummary projects one task into the workflow task tool payload.
func TaskToolTaskSummary(task Task) map[string]any {
	return map[string]any{
		"id": task.ID, "title": task.Title, "status": task.Status, "order": task.Order,
		"dependsOn": task.DependsOn, "executor": task.Executor, "runId": task.RunID,
		"agentRole": task.AgentRole, "planSource": task.PlanSource, "resultSummary": task.ResultSummary,
		"childAgentId": task.ChildAgentID, "childProviderId": task.ChildProviderID, "childModel": task.ChildModel,
		"childPermissionMode": task.ChildPermissionMode,
	}
}

// TaskMutationProjectionFailure builds the tool response when a task mutation
// committed but the parent workflow-plan projection failed.
func TaskMutationProjectionFailure(task Task, err error) (map[string]any, error) {
	return map[string]any{
		"success":          false,
		"committed":        true,
		"partial":          true,
		"retryable":        false,
		"parentPlanSynced": false,
		"task":             TaskToolTaskSummary(task),
		"message":          "task mutation was committed, but parent workflow-plan projection failed; refresh workflow state before continuing",
		"error":            err.Error(),
	}, nil
}
