package model

import "testing"

func TestWorkflowGraphFingerprintNormalizesSetsAndDetectsExecutionDrift(t *testing.T) {
	parent := Run{
		ID: "parent", AgentID: "root", ProviderID: "provider", Model: "model", PermissionMode: PermissionModeApproval,
		WorkMode: WorkModeLoop, WorkflowEngine: WorkflowEngineADK2Canvas, Objective: "objective",
	}
	root := Agent{
		ID: "root", Name: "Root", Instruction: "coordinate", ProviderID: "stale-provider", Model: "stale-model",
		PermissionMode: PermissionModeLessApproval, WorkMode: WorkModeChat, Tools: []string{"trade.submit", " market.read "},
	}
	steps := []WorkflowStep{
		{Order: 1, DependencyID: "a", Title: "A", Message: "first"},
		{Order: 2, DependencyID: "b", Title: "B", Message: "second", DependsOn: []string{" a ", "a"}},
	}
	children := []Agent{{ID: "worker-a", Instruction: "first"}, {ID: "worker-b", Instruction: "second", Skills: []string{"risk", "research"}}}

	want, err := WorkflowGraphFingerprint(parent, root, steps, children)
	if err != nil {
		t.Fatalf("WorkflowGraphFingerprint: %v", err)
	}
	reordered := append([]WorkflowStep(nil), steps...)
	reordered[1].DependsOn = []string{"a"}
	reorderedChildren := append([]Agent(nil), children...)
	reorderedChildren[1].Skills = []string{"research", "risk", "risk"}
	got, err := WorkflowGraphFingerprint(parent, root, reordered, reorderedChildren)
	if err != nil || got != want {
		t.Fatalf("normalized fingerprint = %q err=%v, want %q", got, err, want)
	}

	reordered[1].Message = "changed"
	drifted, err := WorkflowGraphFingerprint(parent, root, reordered, reorderedChildren)
	if err != nil || drifted == want {
		t.Fatalf("drifted fingerprint = %q err=%v, want a different identity", drifted, err)
	}
}
