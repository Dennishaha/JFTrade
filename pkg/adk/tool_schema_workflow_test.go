package adk

import (
	"context"
	"testing"
)

func TestWorkflowManagementToolSchemasAreStrictAndConvertible(t *testing.T) {
	names := []string{
		"workflows.list", "workflows.get", "workflows.create", "workflows.update", "workflows.delete", "workflows.run",
		"workflow_triggers.list", "workflow_triggers.get", "workflow_triggers.create", "workflow_triggers.update", "workflow_triggers.delete", "workflow_triggers.run",
		"workflow_runs.list", "workflow_runs.get",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			schema := defaultToolInputSchema(name)
			if schema["type"] != "object" || schema["additionalProperties"] != false {
				t.Fatalf("schema = %#v, want strict object", schema)
			}
			if _, err := googleADKJSONSchemaFromMap(sanitizeSchemaForOpenAI(schema)); err != nil {
				t.Fatalf("googleADKJSONSchemaFromMap: %v", err)
			}
		})
	}

	assertWorkflowSchemaRequired(t, "workflows.create", "name", "agentId", "promptTemplate")
	assertWorkflowSchemaRequired(t, "workflows.update", "workflowId")
	assertWorkflowSchemaRequired(t, "workflow_triggers.create", "workflowId", "type")
	assertWorkflowSchemaRequired(t, "workflow_triggers.update", "workflowId", "triggerId")
	assertWorkflowSchemaRequired(t, "workflow_runs.get", "logId")
}

func TestExecuteRegisteredToolPreservesInvocationSessionID(t *testing.T) {
	registered := RegisteredTool{
		Descriptor: ToolDescriptor{Name: "session.read"},
		Handler: func(ctx context.Context, _ map[string]any) (any, error) {
			sessionID, ok := ToolInvocationSessionID(ctx)
			return map[string]any{"sessionId": sessionID, "ok": ok}, nil
		},
	}
	output, err := executeRegisteredTool(toolSessionTestContext{Context: t.Context(), sessionID: "session-tool-1"}, registered, nil)
	if err != nil {
		t.Fatalf("executeRegisteredTool: %v", err)
	}
	mapped := output.(map[string]any)
	if mapped["sessionId"] != "session-tool-1" || mapped["ok"] != true {
		t.Fatalf("output = %#v", output)
	}
}

type toolSessionTestContext struct {
	context.Context
	sessionID string
}

func (c toolSessionTestContext) SessionID() string { return c.sessionID }

func assertWorkflowSchemaRequired(t *testing.T, name string, fields ...string) {
	t.Helper()
	schema := defaultToolInputSchema(name)
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("%s required = %#v", name, schema["required"])
	}
	for _, field := range fields {
		found := false
		for _, value := range required {
			if value == field {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s required = %#v, missing %q", name, required, field)
		}
	}
}
