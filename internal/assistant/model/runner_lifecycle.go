package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrADKInputUnsupported marks a GO-ADK input request that the runtime cannot
// satisfy.
var ErrADKInputUnsupported = errors.New("GO-ADK requested input is unsupported; configure the agent/workflow to collect required input before running")

// RunStatusForContext derives a run status from the surrounding context.
func RunStatusForContext(ctx context.Context, err error) string {
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

// RunErrorCode maps a run status and underlying causes to an error code.
func RunErrorCode(status string, causes ...error) string {
	for _, err := range causes {
		if errors.Is(err, ErrADKInputUnsupported) {
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

// RunLifecycleAuditKind maps a terminal status to its audit event kind.
func RunLifecycleAuditKind(status string) string {
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

// FinalizeRunUsage fills the run usage duration from start/completed times.
func FinalizeRunUsage(run *Run) {
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

// SummarizeToolOutput renders a tool output as a compact summary line.
func SummarizeToolOutput(toolName string, output any) string {
	raw, err := json.Marshal(output)
	if err != nil {
		return fmt.Sprintf("%s: %v", toolName, output)
	}
	text := string(raw)
	if len(text) > 1800 {
		text = text[:1800] + "...(truncated)"
	}
	return fmt.Sprintf("%s => %s", toolName, text)
}

// ToolSummariesForRun projects tool calls into run tool summaries.
func ToolSummariesForRun(run Run) []string {
	summaries := make([]string, 0, len(run.ToolCalls))
	for _, call := range run.ToolCalls {
		if call.Status == "SUCCEEDED" {
			summaries = append(summaries, SummarizeToolOutput(call.ToolName, call.Output))
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

// OptimizationTaskID extracts a strategy optimization task ID from calls.
func OptimizationTaskID(calls []ToolCall) string {
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

// MarkFailedChatRun projects a terminal failure onto a chat run.
func MarkFailedChatRun(ctx context.Context, run Run, adkErr error) Run {
	run.Status = RunStatusForContext(ctx, adkErr)
	run.Message = adkErr.Error()
	run.FailureReason = adkErr.Error()
	run.ErrorCode = RunErrorCode(run.Status, adkErr)
	run.Degraded = true
	completedAt := NowString()
	run.CompletedAt = &completedAt
	if run.Status == RunStatusCancelled {
		run.CancelledAt = &completedAt
	}
	FinalizeRunUsage(&run)
	return run
}

// MarkCompletedChatRun projects a successful terminal state onto a chat run.
func MarkCompletedChatRun(run Run) (Run, string) {
	run.Status = RunStatusCompleted
	run.CompletedAt = new(NowString())
	run.Message = "completed"
	run.FailureReason = ""
	run.ErrorCode = ""
	toolFailure := FirstToolCallFailure(&run)
	run.Degraded = toolFailure != ""
	FinalizeRunUsage(&run)
	return run, toolFailure
}
