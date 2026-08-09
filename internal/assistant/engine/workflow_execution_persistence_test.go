package adk

import (
	"testing"
)

func TestSanitizeWorkflowPlanStepRewritesEchoedMessages(t *testing.T) {
	step := sanitizeWorkflowPlanStep(workflowStep{
		Title: "same request", Description: "same request", Message: "same request",
	}, "same request", 1)
	if step.Title != "执行计划步骤 2" || step.Description != "" || step.Message != "推进计划中的第 2 步。" {
		t.Fatalf("sanitizeWorkflowPlanStep = %+v", step)
	}

	step = sanitizeWorkflowPlanStep(workflowStep{
		Message: "same request", Description: "custom description",
	}, "same request", 0)
	if step.Message != "custom description" {
		t.Fatalf("sanitizeWorkflowPlanStep message = %+v, want description fallback", step)
	}
}
