package skillsruntime

import (
	"slices"
	"testing"
)

func TestWorkflowSchemasStayStrict(t *testing.T) {
	names := []string{
		"workflows.list", "workflows.get", "workflows.create", "workflows.update",
		"workflow_triggers.list", "workflow_triggers.create", "workflow_triggers.update",
		"workflow_runs.list", "workflow_runs.get",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			schema := DefaultToolInputSchema(name)
			if schema["type"] != "object" || schema["additionalProperties"] != false {
				t.Fatalf("schema = %#v, want strict object", schema)
			}
		})
	}
	assertRequired(t, "workflows.create", "name", "agentId", "promptTemplate")
	assertRequired(t, "workflows.update", "workflowId")
	assertRequired(t, "workflow_triggers.update", "workflowId", "triggerId")
	assertRequired(t, "workflow_runs.get", "logId")
}

func TestToolMetadataNormalization(t *testing.T) {
	if got := NormalizeToolAlias(" @JFTrade  market//snapshot::latest "); got != "market.snapshot.latest" {
		t.Fatalf("NormalizeToolAlias = %q", got)
	}
	if got := DefaultToolRiskLevelForTool("tasks.create", "write_task"); got != "low" {
		t.Fatalf("task risk = %q", got)
	}
	if got := DefaultToolRiskLevel("live_trading"); got != "critical" {
		t.Fatalf("live trading risk = %q", got)
	}
	input := map[string]any{"string": "42", "float": 12.8}
	if got := StringValue(input, "string"); got != "42" {
		t.Fatalf("StringValue = %q", got)
	}
	if got := IntValue(input, "float", 0); got != 12 {
		t.Fatalf("IntValue = %d", got)
	}
}

func assertRequired(t *testing.T, name string, fields ...string) {
	t.Helper()
	required, ok := DefaultToolInputSchema(name)["required"].([]string)
	if !ok {
		t.Fatalf("%s required is not []string", name)
	}
	for _, field := range fields {
		if !slices.Contains(required, field) {
			t.Fatalf("%s required = %#v, missing %q", name, required, field)
		}
	}
}
