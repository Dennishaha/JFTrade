package adk

import (
	"context"
	"testing"

	adktool "google.golang.org/adk/v2/tool"
)

// Tool metadata is consumed by the ADK model planner before Run is called, so
// the skill gate must preserve it exactly when it wraps a product tool.
func TestCoverage98SkillGatedToolsPreservePlannerMetadata(t *testing.T) {
	registered := RegisteredTool{
		Descriptor: ToolDescriptor{
			Name:          "workflow.publish",
			DisplayName:   "Publish workflow",
			Description:   "Persist a reviewed workflow",
			RequiredSkill: WorkflowManagementSkillName,
		},
		Handler: func(context.Context, map[string]any) (any, error) { return map[string]any{"published": true}, nil },
	}
	productTool, err := newGoogleADKTool(registered.Descriptor, registered)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	base := &googleADKProductToolset{name: "workflow-tools", tools: []adktool.Tool{productTool}}
	toolset := newGoogleADKSkillGatedToolset(base, []ToolDescriptor{registered.Descriptor})
	if got := toolset.Name(); got != "workflow-tools" {
		t.Fatalf("gated toolset name = %q, want workflow-tools", got)
	}
	tools, err := toolset.Tools(newSkillGateTestContext(newSkillActivationTestState(), "planner-agent"))
	if err != nil || len(tools) != 1 {
		t.Fatalf("gated tools = %#v, %v", tools, err)
	}
	gated, ok := tools[0].(*googleADKSkillGatedTool)
	if !ok {
		t.Fatalf("gated tool type = %T, want googleADKSkillGatedTool", tools[0])
	}
	if got := gated.Name(); got != registered.Descriptor.Name {
		t.Fatalf("gated name = %q, want %q", got, registered.Descriptor.Name)
	}
	if got := gated.Description(); got != registered.Descriptor.Description {
		t.Fatalf("gated description = %q, want %q", got, registered.Descriptor.Description)
	}
	if gated.IsLongRunning() {
		t.Fatal("ordinary product tool unexpectedly became long-running after gating")
	}
	if declaration := gated.Declaration(); declaration == nil || declaration.Name != registered.Descriptor.Name || declaration.Description != registered.Descriptor.Description {
		t.Fatalf("gated declaration = %#v", declaration)
	}

	var unavailable *googleADKSkillGatedToolset
	if unavailable.Name() != "" {
		t.Fatal("nil gated toolset exposed a planner name")
	}
	if listed, err := unavailable.Tools(newSkillGateTestContext(newSkillActivationTestState(), "planner-agent")); err != nil || listed != nil {
		t.Fatalf("nil gated toolset tools = %#v, %v", listed, err)
	}
}
