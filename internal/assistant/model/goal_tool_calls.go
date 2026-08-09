package model

import (
	"errors"
	"strings"

	adkworkflow "google.golang.org/adk/v2/workflow"
)

// PruneInterruptedGoalWorkflowToolCalls removes tool calls that were
// interrupted when a goal workflow paused, returning the pruned run and
// whether anything changed.
func PruneInterruptedGoalWorkflowToolCalls(run Run) (Run, bool) {
	if len(run.ToolCalls) == 0 {
		return run, false
	}
	filtered := make([]ToolCall, 0, len(run.ToolCalls))
	changed := false
	for _, call := range run.ToolCalls {
		if InterruptedGoalWorkflowToolCall(run, call) {
			changed = true
			continue
		}
		filtered = append(filtered, call)
	}
	if !changed {
		return run, false
	}
	run.ToolCalls = filtered
	run.ToolSummaries = ToolSummariesForRun(Run{ToolCalls: filtered})
	return run, true
}

// InterruptedGoalWorkflowToolCall reports whether a tool call belongs to the
// goal workflow and was left in a paused/interrupted state.
func InterruptedGoalWorkflowToolCall(parent Run, call ToolCall) bool {
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
		callErr := ErrorFromSerializedADKText(*call.Error)
		if !errors.Is(callErr, ErrUserGoalPauseRequested) && !errors.Is(callErr, adkworkflow.ErrNodeInterrupted) {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(call.ToolName), "workflow.")
	}
	return false
}
