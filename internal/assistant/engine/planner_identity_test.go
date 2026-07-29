package adk

import "testing"

func TestWorkflowPlannerToolsetNameIsStableForADKRegistration(t *testing.T) {
	toolset := newWorkflowPlannerToolset(&workflowPlanDraft{})
	if got := toolset.Name(); got != "jftrade-workflow-planner-tools" {
		t.Fatalf("workflow planner toolset name = %q", got)
	}
}
