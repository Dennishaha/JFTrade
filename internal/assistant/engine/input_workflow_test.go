package adk

import "testing"

func TestInputRequestProjectsThroughWorkflowBlockingState(t *testing.T) {
	request := &InputRequest{ID: "input-workflow", RunID: "child", Status: InputRequestStatusPending}
	parent := Run{
		ID: "parent", Status: RunStatusRunning,
		WorkflowPlan: []WorkflowStepState{{ChildRunID: "child", Status: "IN_PROGRESS"}},
	}
	child := Run{ID: "child", Status: RunStatusPendingInput, Message: "waiting", InputRequest: request}

	paused := pauseParentForChild(parent, child, 0)
	if paused.Status != RunStatusPendingInput || paused.InputRequest == nil || paused.InputRequest.ID != request.ID {
		t.Fatalf("paused parent = %+v", paused)
	}
	if paused.WorkflowPlan[0].Status != "BLOCKED" || workflowPendingReply(paused) != "工作流正在等待用户回答。" {
		t.Fatalf("paused workflow plan = %+v reply=%q", paused.WorkflowPlan, workflowPendingReply(paused))
	}
	if !isWorkflowBlockingStatus(RunStatusPendingInput) || isWorkflowBlockingStatus(RunStatusCompleted) {
		t.Fatal("pending input must be blocking while completed is not")
	}
}
