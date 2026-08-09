package model

import (
	"strings"
	"testing"
)

func TestWorkflowPlanPresentationKeepsDeterministicHumanState(t *testing.T) {
	tasks := []Task{
		{ID: "z-last", Order: 1, CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "a-first", Order: 1, CreatedAt: "2026-01-01T00:00:00Z"},
	}
	SortWorkflowTasks(tasks)
	if got := []string{tasks[0].ID, tasks[1].ID}; strings.Join(got, ",") != "a-first,z-last" {
		t.Fatalf("equal workflow task ordering = %v, want deterministic ID order", got)
	}

	if got := WorkflowStepDescription(WorkflowStep{Description: "验证市场状态", AgentRole: "Research Agent"}); got != "验证市场状态\n\nAgent role: Research Agent" {
		t.Fatalf("workflow step description = %q", got)
	}
	if got := WorkflowPendingReply(Run{Status: RunStatusPending, WorkMode: WorkModeChat}); got != "工作流正在等待审批。" {
		t.Fatalf("ordinary pending workflow reply = %q", got)
	}

	summary := WorkflowSummary(Run{WorkMode: WorkModeChat}, []string{"", "  ", "已完成数据核验"})
	if strings.Contains(summary, "\n- \n") || !strings.Contains(summary, "\n- 已完成数据核验") {
		t.Fatalf("workflow summary must omit blank child replies: %q", summary)
	}
}

func TestWorkflowPlanBoundaryFormattingAndGraphSemantics(t *testing.T) {
	longDetail := strings.Repeat("市场验证", 61)
	if got := WorkflowSelfTaskSummary(Task{Title: "", Description: longDetail}); !strings.HasPrefix(got, "任务 已由父智能体完成：") || !strings.HasSuffix(got, "...") {
		t.Fatalf("long self-task summary = %q", got)
	}
	if got := WorkflowSelfTaskSummary(Task{Title: "Research", ResultSummary: " saved result "}); got != " saved result " {
		t.Fatalf("explicit self-task summary = %q", got)
	}
	if got := WorkflowStepDescription(WorkflowStep{Description: " detail "}); got != "detail" {
		t.Fatalf("description without role = %q", got)
	}
	if got := WorkflowStepDescription(WorkflowStep{AgentRole: " analyst "}); got != "Agent role: analyst" {
		t.Fatalf("role-only description = %q", got)
	}
	if got := WorkflowDescriptionWithoutAgentRole("Agent role: analyst"); got != "" {
		t.Fatalf("role-only stored description = %q", got)
	}

	if !WorkflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}}) {
		t.Fatal("cyclic runtime task dependencies must be rejected")
	}
	if WorkflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"missing"}}, {ID: "b", DependsOn: []string{"a"}}}) {
		t.Fatal("an unknown dependency must not be mistaken for a dependency cycle")
	}

	parent := Run{
		ID: "plan-parent", WorkMode: WorkModeChat, Status: RunStatusFailed, Message: "fallback message", FailureReason: "child failed",
		WorkflowPlan: []WorkflowStepState{{Title: "first", ChildRunID: "other"}, {Title: "second", ChildRunID: "matched"}},
	}
	if got := WorkflowPendingReply(parent); got != "child failed" {
		t.Fatalf("failed workflow pending reply = %q", got)
	}
	parent.FailureReason = ""
	if got := WorkflowPendingReply(parent); got != "fallback message" {
		t.Fatalf("fallback workflow pending reply = %q", got)
	}
	pendingGoal := Run{Status: RunStatusPending, WorkMode: WorkModeLoop}
	if got := WorkflowPendingReply(pendingGoal); !strings.Contains(got, "目标模式") {
		t.Fatalf("pending goal reply = %q", got)
	}

	matched := UpdateWorkflowPlanForChildAt(parent, Run{ID: "matched", Status: RunStatusCompleted, Message: "completed child", AgentID: "child-agent"}, -1)
	if matched.WorkflowPlan[0].Status != "" || matched.WorkflowPlan[1].Status != "DONE" || matched.WorkflowCursor != 1 {
		t.Fatalf("matched workflow child update = %+v", matched)
	}
	unmatched := UpdateWorkflowPlanForChildAt(parent, Run{ID: "new-child", Status: RunStatusRunning}, -1)
	if unmatched.WorkflowPlan[0].ChildRunID != "other" || unmatched.WorkflowPlan[1].ChildRunID != "matched" {
		t.Fatalf("unmatched workflow child changed plan = %+v", unmatched)
	}

	child := WorkflowChildAgentForStep(Agent{ID: "parent", ProviderID: "provider-a", Model: "model-a", PermissionMode: PermissionModeApproval}, WorkflowStep{
		ChildAgentID: "child", ChildProviderID: "provider-b", ChildModel: "model-b", ChildPermissionMode: PermissionModeAll,
	})
	if child.ID != "child" || child.ProviderID != "provider-b" || child.Model != "model-b" || child.PermissionMode != PermissionModeAll || child.WorkMode != WorkModeChat {
		t.Fatalf("child agent overrides = %+v", child)
	}

	summary := WorkflowSummary(Run{WorkMode: WorkModeChat, Objective: "ship verified workflow", ChildRunIDs: []string{"one", "two"}}, []string{strings.Repeat("结果", 100)})
	if !strings.Contains(summary, "工作流已完成。") || !strings.Contains(summary, "子运行：2 个") || !strings.Contains(summary, "...") {
		t.Fatalf("workflow summary = %q", summary)
	}
	if values := AppendUniqueString([]string{"one"}, " "); len(values) != 1 {
		t.Fatalf("blank unique value = %#v", values)
	}
	if values := AppendUniqueString([]string{"one"}, "one"); len(values) != 1 {
		t.Fatalf("duplicate unique value = %#v", values)
	}
}

func TestApprovalsForRunScoping(t *testing.T) {
	if got := ApprovalsForRun([]Approval{
		{RunID: "run", ID: "pending", Status: ApprovalStatusPending},
		{RunID: "run", ID: "done", Status: ApprovalStatusApproved},
	}, " "); got != nil {
		t.Fatalf("blank run approval selection = %#v", got)
	}
	got := ApprovalsForRun([]Approval{
		{RunID: "run", ID: "pending", Status: ApprovalStatusPending},
		{RunID: "run", ID: "duplicate", Status: ApprovalStatusPending, ConfirmationCallID: "same"},
		{RunID: "other", ID: "other", Status: ApprovalStatusPending},
	}, "run")
	if len(got) != 2 {
		t.Fatalf("run-scoped pending approvals = %#v", got)
	}
}
