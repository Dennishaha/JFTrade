package adk

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowPlanBoundaryFormattingAndGraphSemantics(t *testing.T) {
	longDetail := strings.Repeat("市场验证", 61)
	if got := workflowSelfTaskSummary(Task{Title: "", Description: longDetail}); !strings.HasPrefix(got, "任务 已由父智能体完成：") || !strings.HasSuffix(got, "...") {
		t.Fatalf("long self-task summary = %q", got)
	}
	if got := workflowSelfTaskSummary(Task{Title: "Research", ResultSummary: " saved result "}); got != " saved result " {
		t.Fatalf("explicit self-task summary = %q", got)
	}
	if got := workflowStepDescription(workflowStep{Description: " detail "}); got != "detail" {
		t.Fatalf("description without role = %q", got)
	}
	if got := workflowStepDescription(workflowStep{AgentRole: " analyst "}); got != "Agent role: analyst" {
		t.Fatalf("role-only description = %q", got)
	}
	if got := workflowDescriptionWithoutAgentRole("Agent role: analyst"); got != "" {
		t.Fatalf("role-only stored description = %q", got)
	}

	if !workflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}}) {
		t.Fatal("cyclic runtime task dependencies must be rejected")
	}
	if workflowTasksHaveCycle([]Task{{ID: "a", DependsOn: []string{"missing"}}, {ID: "b", DependsOn: []string{"a"}}}) {
		t.Fatal("an unknown dependency must not be mistaken for a dependency cycle")
	}

	parent := Run{
		ID: "plan-parent", WorkMode: WorkModeChat, Status: RunStatusFailed, Message: "fallback message", FailureReason: "child failed",
		WorkflowPlan: []WorkflowStepState{{Title: "first", ChildRunID: "other"}, {Title: "second", ChildRunID: "matched"}},
	}
	if got := workflowPendingReply(parent); got != "child failed" {
		t.Fatalf("failed workflow pending reply = %q", got)
	}
	parent.FailureReason = ""
	if got := workflowPendingReply(parent); got != "fallback message" {
		t.Fatalf("fallback workflow pending reply = %q", got)
	}
	pendingGoal := Run{Status: RunStatusPending, WorkMode: WorkModeLoop}
	if got := workflowPendingReply(pendingGoal); !strings.Contains(got, "目标模式") {
		t.Fatalf("pending goal reply = %q", got)
	}

	matched := updateWorkflowPlanForChildAt(parent, Run{ID: "matched", Status: RunStatusCompleted, Message: "completed child", AgentID: "child-agent"}, -1)
	if matched.WorkflowPlan[0].Status != "" || matched.WorkflowPlan[1].Status != "DONE" || matched.WorkflowCursor != 1 {
		t.Fatalf("matched workflow child update = %+v", matched)
	}
	unmatched := updateWorkflowPlanForChildAt(parent, Run{ID: "new-child", Status: RunStatusRunning}, -1)
	if unmatched.WorkflowPlan[0].ChildRunID != "other" || unmatched.WorkflowPlan[1].ChildRunID != "matched" {
		t.Fatalf("unmatched workflow child changed plan = %+v", unmatched)
	}

	child := workflowChildAgentForStep(Agent{ID: "parent", ProviderID: "provider-a", Model: "model-a", PermissionMode: PermissionModeApproval}, workflowStep{
		ChildAgentID: "child", ChildProviderID: "provider-b", ChildModel: "model-b", ChildPermissionMode: PermissionModeAll,
	})
	if child.ID != "child" || child.ProviderID != "provider-b" || child.Model != "model-b" || child.PermissionMode != PermissionModeAll || child.WorkMode != WorkModeChat {
		t.Fatalf("child agent overrides = %+v", child)
	}

	summary := workflowSummary(Run{WorkMode: WorkModeChat, Objective: "ship verified workflow", ChildRunIDs: []string{"one", "two"}}, []string{strings.Repeat("结果", 100)})
	if !strings.Contains(summary, "工作流已完成。") || !strings.Contains(summary, "子运行：2 个") || !strings.Contains(summary, "...") {
		t.Fatalf("workflow summary = %q", summary)
	}
	if values := appendUniqueString([]string{"one"}, " "); len(values) != 1 {
		t.Fatalf("blank unique value = %#v", values)
	}
	if values := appendUniqueString([]string{"one"}, "one"); len(values) != 1 {
		t.Fatalf("duplicate unique value = %#v", values)
	}
}

func TestWorkflowPlanAgentResolutionAndApprovalSelectionFailures(t *testing.T) {
	runtime := newTestRuntime(t)
	_, err := runtime.workflowChildAgentForStep(context.Background(), Agent{ID: "parent", PermissionMode: PermissionModeApproval}, workflowStep{ChildAgentID: "missing-child"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing child-agent resolution err = %v", err)
	}
	if approvals := approvalsForRun([]Approval{{RunID: "run", ID: "pending", Status: ApprovalStatusPending}, {RunID: "run", ID: "done", Status: ApprovalStatusApproved}}, " "); approvals != nil {
		t.Fatalf("blank run approval selection = %#v", approvals)
	}
	approvals := approvalsForRun([]Approval{
		{RunID: "run", ID: "pending", Status: ApprovalStatusPending},
		{RunID: "run", ID: "duplicate", Status: ApprovalStatusPending, ConfirmationCallID: "same"},
		{RunID: "other", ID: "other", Status: ApprovalStatusPending},
	}, "run")
	if len(approvals) != 2 {
		t.Fatalf("run-scoped pending approvals = %#v", approvals)
	}
}
