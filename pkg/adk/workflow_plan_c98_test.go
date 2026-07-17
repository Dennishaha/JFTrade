package adk

import (
	"strings"
	"testing"
)

func TestCoverage98WorkflowPlanPresentationKeepsDeterministicHumanState(t *testing.T) {
	tasks := []Task{
		{ID: "z-last", Order: 1, CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "a-first", Order: 1, CreatedAt: "2026-01-01T00:00:00Z"},
	}
	sortWorkflowTasks(tasks)
	if got := []string{tasks[0].ID, tasks[1].ID}; strings.Join(got, ",") != "a-first,z-last" {
		t.Fatalf("equal workflow task ordering = %v, want deterministic ID order", got)
	}

	if got := workflowStepDescription(workflowStep{Description: "验证市场状态", AgentRole: "Research Agent"}); got != "验证市场状态\n\nAgent role: Research Agent" {
		t.Fatalf("workflow step description = %q", got)
	}
	if got := workflowPendingReply(Run{Status: RunStatusPending, WorkMode: WorkModeChat}); got != "工作流正在等待审批。" {
		t.Fatalf("ordinary pending workflow reply = %q", got)
	}

	summary := workflowSummary(Run{WorkMode: WorkModeChat}, []string{"", "  ", "已完成数据核验"})
	if strings.Contains(summary, "\n- \n") || !strings.Contains(summary, "\n- 已完成数据核验") {
		t.Fatalf("workflow summary must omit blank child replies: %q", summary)
	}
}
