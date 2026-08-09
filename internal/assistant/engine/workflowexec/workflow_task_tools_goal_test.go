package workflowexec

import (
	"testing"
)

func TestWorkflowTaskToolsetSwitchesToGoalDecisionTools(t *testing.T) {
	decision := &workflowGoalDecision{}
	decision.BeginDecision()
	toolset := NewWorkflowTaskToolset(nil, "", "")
	toolset.Req = workflowRequest{Mode: WorkModeLoop, GoalDecision: decision}
	tools, err := toolset.Tools(nil)
	if err != nil {
		t.Fatalf("WorkflowTaskToolset.Tools: %v", err)
	}
	if len(tools) != 2 || tools[0].Name() != workflowGoalCompleteTool || tools[1].Name() != workflowGoalContinueTool {
		t.Fatalf("workflow goal decision tools = %#v, want complete/continue only", tools)
	}
}
