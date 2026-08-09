package adk

import "testing"
import jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"

func TestInputRequestProjectsThroughWorkflowBlockingState(t *testing.T) {
	request := &InputRequest{ID: "input-workflow", RunID: "child", Status: InputRequestStatusPending}
	parent := Run{
		ID: "parent", Status: RunStatusRunning,
		WorkflowPlan: []WorkflowStepState{{ChildRunID: "child", Status: "IN_PROGRESS"}},
	}
	child := Run{ID: "child", Status: RunStatusPendingInput, Message: "waiting", InputRequest: request}

	paused := jfadkmodel.PauseParentForChild(parent, child, 0)
	if paused.Status != RunStatusPendingInput || paused.InputRequest == nil || paused.InputRequest.ID != request.ID {
		t.Fatalf("paused parent = %+v", paused)
	}
	if paused.WorkflowPlan[0].Status != "BLOCKED" || jfadkmodel.WorkflowPendingReply(paused) != "工作流正在等待用户回答。" {
		t.Fatalf("paused workflow plan = %+v reply=%q", paused.WorkflowPlan, jfadkmodel.WorkflowPendingReply(paused))
	}
	if !jfadkmodel.IsWorkflowBlockingStatus(RunStatusPendingInput) || jfadkmodel.IsWorkflowBlockingStatus(RunStatusCompleted) {
		t.Fatal("pending input must be blocking while completed is not")
	}
}
