package model

import (
	"strings"
	"testing"
)

func TestWorkflowGoalDecisionAndUtilityContracts(t *testing.T) {
	var nilDecision *WorkflowGoalDecision
	nilDecision.Reset()
	nilDecision.BeginDecision()
	nilDecision.SetComplete("ignored")
	nilDecision.SetContinue("ignored")
	if nilDecision.DecisionPhase() || nilDecision.Snapshot().Status != "" {
		t.Fatal("nil goal decision must remain inert")
	}

	decision := &WorkflowGoalDecision{}
	decision.Reset()
	if decision.DecisionPhase() || decision.Snapshot().Status != "" {
		t.Fatalf("reset decision = %+v", decision.Snapshot())
	}
	decision.BeginDecision()
	if !decision.DecisionPhase() {
		t.Fatal("decision phase was not entered")
	}
	decision.SetComplete(" complete summary ")
	if snapshot := decision.Snapshot(); snapshot.Status != "complete" || snapshot.Summary != "complete summary" || snapshot.Reason != "" {
		t.Fatalf("complete snapshot = %+v", snapshot)
	}
	decision.SetContinue(" keep researching ")
	if snapshot := decision.Snapshot(); snapshot.Status != "continue" || snapshot.Reason != "keep researching" || snapshot.Summary != "" {
		t.Fatalf("continue snapshot = %+v", snapshot)
	}

	parent := Run{ID: "goal", Objective: "完成研究", UserMessage: "分析市场", WorkMode: WorkModeLoop}
	if !strings.Contains(GoalOrchestratorUserMessage(parent), "总体目标：完成研究") || !strings.Contains(GoalDecisionPrompt(parent, "已有回复", true), WorkflowGoalCompleteTool) {
		t.Fatal("goal prompts lost their user objective or decision contract")
	}
	if !strings.Contains(GoalFinalReplyPrompt(parent), "完成研究") || !strings.Contains(GoalOrchestratorContinueNudge(parent, ""), "目标尚未完成") {
		t.Fatal("goal follow-up prompts lost their state")
	}
	if got := WorkflowTaskResultSummaries([]Task{{ResultSummary: " first "}, {ResultSummary: " "}, {ResultSummary: "second"}}); strings.Join(got, ",") != "first,second" {
		t.Fatalf("task result summaries = %v", got)
	}
	if got := PlannerStringListArg(map[string]any{"items": []any{" first ", 2, ""}}, "items"); strings.Join(got, ",") != "first,2" {
		t.Fatalf("planner string slice = %v", got)
	}
	if PlannerStringListArg(map[string]any{"items": []string{"not-any"}}, "items") != nil {
		t.Fatal("typed non-any slice should not be accepted as tool arguments")
	}
	if PlannerStringListArg(nil, "dependsOn") != nil || PlannerStringListArg(map[string]any{"dependsOn": "invalid"}, "dependsOn") != nil {
		t.Fatal("planner dependency list accepted an absent or malformed value")
	}
}
