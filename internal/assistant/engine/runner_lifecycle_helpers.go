package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func runStatusForContext(ctx context.Context, err error) string {
	if err == nil {
		return RunStatusCompleted
	}
	if ctx != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return RunStatusTimedOut
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return RunStatusCancelled
		}
	}
	return RunStatusFailed
}

func runErrorCode(status string, causes ...error) string {
	for _, err := range causes {
		if errors.Is(err, errADKInputUnsupported) {
			return "ADK_INPUT_UNSUPPORTED"
		}
	}
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
