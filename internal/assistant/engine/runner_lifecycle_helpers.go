package adk

import (
	"context"
	"fmt"
	"strings"
	"time"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func runStatusForContext(ctx context.Context, err error) string {
	return jfadkmodel.RunStatusForContext(ctx, err)
}

func runErrorCode(status string, causes ...error) string {
	return jfadkmodel.RunErrorCode(status, causes...)
}

func runLifecycleAuditKind(status string) string {
	return jfadkmodel.RunLifecycleAuditKind(status)
}

func finalizeRunUsage(run *Run) { jfadkmodel.FinalizeRunUsage(run) }

func toolSummariesForRun(run Run) []string {
	return jfadkmodel.ToolSummariesForRun(run)
}

func optimizationTaskID(calls []ToolCall) string {
	return jfadkmodel.OptimizationTaskID(calls)
}

func validateUserGoalPauseRun(run Run) error {
	if strings.TrimSpace(run.ParentRunID) != "" {
		return fmt.Errorf("only root goal runs can be paused")
	}
	if jfadkmodel.NormalizeWorkMode(run.WorkMode) != WorkModeLoop || strings.TrimSpace(run.WorkflowStatus) == "" {
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
	if jfadkmodel.NormalizeWorkMode(run.WorkMode) != WorkModeLoop || strings.TrimSpace(run.WorkflowStatus) == "" {
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

func recentOpenAIMessages(messages []TranscriptEntry, maxMessages int, maxChars int) []openAIChatMessage {
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
	return strings.Contains(content, "等待用户审批") ||
		strings.Contains(content, "请先在 ADK 审批队列")
}

func runTimeoutForRun(run Run) time.Duration { return jfadkmodel.RunTimeoutForRun(run) }

func isRecoverableReconcileStatus(status string) bool {
	return jfadkmodel.IsRecoverableReconcileStatus(status)
}

func workflowChildRunHasNoExecutionActivity(run Run) bool {
	return jfadkmodel.WorkflowChildRunHasNoExecutionActivity(run)
}

func workflowParentReferencesChild(parent Run, childRunID string) bool {
	return jfadkmodel.WorkflowParentReferencesChild(parent, childRunID)
}
