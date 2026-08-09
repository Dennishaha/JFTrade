package adk

import (
	"context"

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

func finalizeRunUsage(run *Run) {
	jfadkmodel.FinalizeRunUsage(run)
}

func toolSummariesForRun(run Run) []string {
	return jfadkmodel.ToolSummariesForRun(run)
}

func optimizationTaskID(calls []ToolCall) string {
	return jfadkmodel.OptimizationTaskID(calls)
}
