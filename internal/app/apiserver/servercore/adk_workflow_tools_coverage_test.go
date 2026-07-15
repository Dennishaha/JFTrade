package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

type errorWorkflowToolManager struct{ err error }

func (m errorWorkflowToolManager) ListWorkflows(context.Context, string, int, int) (WorkflowToolPage[jfadk.WorkflowDefinition], error) {
	return WorkflowToolPage[jfadk.WorkflowDefinition]{}, m.err
}
func (m errorWorkflowToolManager) GetWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error) {
	return jfadk.WorkflowDefinition{}, m.err
}
func (m errorWorkflowToolManager) SaveWorkflow(context.Context, string, jfadk.WorkflowDefinitionWriteRequest) (jfadk.WorkflowDefinition, error) {
	return jfadk.WorkflowDefinition{}, m.err
}
func (m errorWorkflowToolManager) DeleteWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error) {
	return jfadk.WorkflowDefinition{}, m.err
}
func (m errorWorkflowToolManager) ListWorkflowTriggers(context.Context, string) ([]jfadk.WorkflowTrigger, error) {
	return nil, m.err
}
func (m errorWorkflowToolManager) GetWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error) {
	return jfadk.WorkflowTrigger{}, m.err
}
func (m errorWorkflowToolManager) SaveWorkflowTrigger(context.Context, string, string, jfadk.WorkflowTriggerWriteRequest) (jfadk.WorkflowTrigger, error) {
	return jfadk.WorkflowTrigger{}, m.err
}
func (m errorWorkflowToolManager) DeleteWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error) {
	return jfadk.WorkflowTrigger{}, m.err
}
func (m errorWorkflowToolManager) ListWorkflowRuns(context.Context, string, string, string, int, int) (WorkflowToolPage[jfadk.WorkflowTriggerLog], error) {
	return WorkflowToolPage[jfadk.WorkflowTriggerLog]{}, m.err
}
func (m errorWorkflowToolManager) GetWorkflowRun(context.Context, string) (jfadk.WorkflowTriggerLog, error) {
	return jfadk.WorkflowTriggerLog{}, m.err
}
func (m errorWorkflowToolManager) StartWorkflow(context.Context, string, map[string]any) (WorkflowToolStartResult, error) {
	return WorkflowToolStartResult{}, m.err
}
func (m errorWorkflowToolManager) StartWorkflowTrigger(context.Context, string, map[string]any) (WorkflowToolStartResult, error) {
	return WorkflowToolStartResult{}, m.err
}

func TestWorkflowToolsRemainingManagerErrorPropagation(t *testing.T) {
	store, err := jfadk.NewStore(filepath.Join(t.TempDir(), "adk.db"), filepath.Join(t.TempDir(), "secrets"), filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(t.Context(), "agent", "Interactive")
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("workflow manager failed")
	registry := jfadk.NewToolRegistry()
	registerJFTradeADKWorkflowManagementTools(store, registry, errorWorkflowToolManager{err: wantErr})
	interactive := workflowToolSessionContext{Context: t.Context(), sessionID: session.ID}

	tests := []struct {
		name  string
		ctx   context.Context
		input map[string]any
	}{
		{name: "workflows.list", ctx: t.Context(), input: map[string]any{}},
		{name: "workflows.get", ctx: t.Context(), input: map[string]any{"workflowId": "workflow"}},
		{name: "workflows.create", ctx: t.Context(), input: map[string]any{"name": "workflow"}},
		{name: "workflows.update", ctx: t.Context(), input: map[string]any{"workflowId": "workflow"}},
		{name: "workflows.delete", ctx: t.Context(), input: map[string]any{"workflowId": "workflow"}},
		{name: "workflow_triggers.list", ctx: t.Context(), input: map[string]any{"workflowId": "workflow"}},
		{name: "workflow_triggers.get", ctx: t.Context(), input: map[string]any{"workflowId": "workflow", "triggerId": "trigger"}},
		{name: "workflow_triggers.create", ctx: t.Context(), input: map[string]any{"workflowId": "workflow", "type": "manual"}},
		{name: "workflow_triggers.update", ctx: t.Context(), input: map[string]any{"workflowId": "workflow", "triggerId": "trigger"}},
		{name: "workflow_triggers.delete", ctx: t.Context(), input: map[string]any{"workflowId": "workflow", "triggerId": "trigger"}},
		{name: "workflow_runs.list", ctx: t.Context(), input: map[string]any{}},
		{name: "workflow_runs.get", ctx: t.Context(), input: map[string]any{"logId": "log"}},
		{name: "workflows.run", ctx: interactive, input: map[string]any{"workflowId": "workflow"}},
		{name: "workflow_triggers.run", ctx: interactive, input: map[string]any{"triggerId": "trigger"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool, ok := registry.Get(test.name)
			if !ok {
				t.Fatalf("tool %q missing", test.name)
			}
			if _, err := tool.Handler(test.ctx, test.input); !errors.Is(err, wantErr) {
				t.Fatalf("tool error = %v, want %v", err, wantErr)
			}
		})
	}
	if _, err := workflowToolManagerRequired(nil).GetWorkflow(t.Context(), "workflow"); err == nil {
		t.Fatal("nil workflow manager did not fail closed")
	}
}

func TestWorkflowToolsRemainingSessionAndPayloadErrors(t *testing.T) {
	if err := requireInteractiveWorkflowToolSession(t.Context(), nil); err == nil {
		t.Fatal("nil store session error = nil")
	}
	store, err := jfadk.NewStore(filepath.Join(t.TempDir(), "adk.db"), filepath.Join(t.TempDir(), "secrets"), filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	missingContext := workflowToolSessionContext{Context: t.Context(), sessionID: "missing"}
	if err := requireInteractiveWorkflowToolSession(missingContext, store); err == nil {
		t.Fatal("missing session error = nil")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := requireInteractiveWorkflowToolSession(missingContext, store); err == nil {
		t.Fatal("closed session store error = nil")
	}

	if _, err := workflowCreateRequest(map[string]any{"canvasGraph": func() {}}); err == nil {
		t.Fatal("unmarshalable create canvas error = nil")
	}
	if _, err := workflowUpdateRequest(jfadk.WorkflowDefinition{}, map[string]any{"canvasGraph": "invalid"}); err == nil {
		t.Fatal("invalid update canvas error = nil")
	}
	payload := jfadk.WorkflowDefinitionWriteRequest{}
	if err := decodeWorkflowCanvasGraph(map[string]any{}, &payload); err != nil {
		t.Fatalf("omitted canvas error = %v", err)
	}
	if err := decodeWorkflowCanvasGraph(map[string]any{"canvasGraph": map[string]any{"version": "v1"}}, &payload); err != nil || payload.CanvasGraph == nil {
		t.Fatalf("valid canvas = %#v, %v", payload.CanvasGraph, err)
	}

	applyWorkflowWriteFields(&payload, map[string]any{
		"name": "name", "description": "description", "status": "status", "agentId": "agent", "workMode": "chat",
		"providerId": "provider", "model": "model", "permissionMode": "approval", "promptTemplate": "prompt",
		"objectiveTemplate": "objective", "defaultInputs": map[string]any{"symbol": "US.AAPL"}, "tags": []any{"one"},
	})
	if payload.DefaultInputs["symbol"] != "US.AAPL" || len(payload.Tags) != 1 {
		t.Fatalf("applied workflow fields = %#v", payload)
	}

	current := jfadk.WorkflowTrigger{Type: jfadk.WorkflowTriggerTypeManual}
	updated, err := workflowTriggerUpdateRequest(current, map[string]any{"type": "schedule", "config": map[string]any{"cron": "* * * * *"}})
	if err != nil || updated.Type != "schedule" || updated.Config["cron"] == nil {
		t.Fatalf("non-webhook trigger update = %#v, %v", updated, err)
	}
}
