package adk

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowPlanAgentResolutionFailure(t *testing.T) {
	runtime := newTestRuntime(t)
	_, err := runtime.WorkflowChildAgentForStep(context.Background(), Agent{ID: "parent", PermissionMode: PermissionModeApproval}, workflowStep{ChildAgentID: "missing-child"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing child-agent resolution err = %v", err)
	}
}
