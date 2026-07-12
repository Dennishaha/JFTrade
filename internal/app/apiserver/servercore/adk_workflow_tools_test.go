package servercore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestWorkflowManagementToolCatalogAndApprovalMatrix(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	registerJFTradeADKWorkflowManagementTools(nil, registry, nil)
	names := []string{
		"workflows.list", "workflows.get", "workflows.create", "workflows.update", "workflows.delete", "workflows.run",
		"workflow_triggers.list", "workflow_triggers.get", "workflow_triggers.create", "workflow_triggers.update", "workflow_triggers.delete", "workflow_triggers.run",
		"workflow_runs.list", "workflow_runs.get",
	}
	for _, name := range names {
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("tool %q is not registered", name)
		}
		if tool.Descriptor.Category != "workflow" || tool.Descriptor.InputSchema == nil {
			t.Fatalf("descriptor %q = %+v", name, tool.Descriptor)
		}
		if tool.Descriptor.RequiredSkill != jfadk.WorkflowManagementSkillName {
			t.Fatalf("descriptor %q required skill = %q", name, tool.Descriptor.RequiredSkill)
		}
		isMutation := strings.Contains(name, ".create") || strings.Contains(name, ".update") || strings.Contains(name, ".delete") || strings.HasSuffix(name, ".run")
		for _, mode := range []string{jfadk.PermissionModeApproval, jfadk.PermissionModeLessApproval, jfadk.PermissionModeAll} {
			wantApproval := isMutation && mode == jfadk.PermissionModeApproval
			if got := jfadk.ToolRequiresApproval(tool.Descriptor, mode); got != wantApproval {
				t.Fatalf("ToolRequiresApproval(%s, %s) = %v, want %v", name, mode, got, wantApproval)
			}
		}
	}
	defaultTools := jfadk.ToolDescriptorsForAgent(jfadk.Agent{}, registry)
	if !workflowToolDescriptorsContain(defaultTools, "workflows.run") || !workflowToolDescriptorsContain(defaultTools, "workflow_runs.get") {
		t.Fatalf("default agent tools = %+v, want workflow management tools", defaultTools)
	}
	explicitTools := jfadk.ToolDescriptorsForAgent(jfadk.Agent{Tools: []string{"workflows.get"}}, registry)
	if len(explicitTools) != 1 || explicitTools[0].Name != "workflows.get" {
		t.Fatalf("explicit agent tools = %+v, want only workflows.get", explicitTools)
	}
	searchTool, _ := registry.Get("tools.search")
	searchOutput, err := searchTool.Handler(t.Context(), map[string]any{"query": "异步", "category": "workflow"})
	if err != nil {
		t.Fatalf("tools.search: %v", err)
	}
	searchItems := searchOutput.(map[string]any)["tools"].([]map[string]any)
	foundRunTool := false
	for _, item := range searchItems {
		if item["name"] == "workflow_triggers.run" || item["name"] == "workflows.run" {
			foundRunTool = true
			break
		}
	}
	if foundRunTool {
		t.Fatalf("tools.search exposed workflow tools before skill activation: %#v", searchOutput)
	}
}

func TestWorkflowManagementToolUpdatesUsePatchSemantics(t *testing.T) {
	currentWorkflow := jfadk.WorkflowDefinition{
		ID: "workflow-patch", Name: "Keep Name", Description: "old", Status: jfadk.WorkflowStatusEnabled,
		AgentID: "agent-1", WorkMode: jfadk.WorkModeLoop, ProviderID: "provider-1", Model: "model-1",
		PermissionMode: jfadk.PermissionModeApproval, PromptTemplate: "keep prompt", ObjectiveTemplate: "keep objective",
		DefaultInputs: map[string]any{"symbol": "US.AAPL"}, CanvasGraph: &jfadk.WorkflowCanvasGraph{Version: "v1"}, Tags: []string{"old"},
	}
	currentTrigger := jfadk.WorkflowTrigger{
		ID: "trigger-patch", WorkflowID: currentWorkflow.ID, Type: jfadk.WorkflowTriggerTypeWebhook,
		Title: "Old Webhook", Status: jfadk.WorkflowTriggerStatusEnabled, Config: map[string]any{"source": "old"},
	}
	spy := &workflowToolManagerSpy{workflow: currentWorkflow, trigger: currentTrigger}
	registry := jfadk.NewToolRegistry()
	registerJFTradeADKWorkflowManagementTools(nil, registry, spy)

	updateWorkflow, _ := registry.Get("workflows.update")
	output, err := updateWorkflow.Handler(t.Context(), map[string]any{
		"workflowId": currentWorkflow.ID, "description": "", "tags": []any{}, "clearCanvasGraph": true,
	})
	if err != nil {
		t.Fatalf("workflows.update: %v", err)
	}
	updated := output.(jfadk.WorkflowDefinition)
	if updated.Name != currentWorkflow.Name || updated.PromptTemplate != currentWorkflow.PromptTemplate || updated.Description != "" || len(updated.Tags) != 0 || updated.CanvasGraph != nil {
		t.Fatalf("updated workflow = %+v, want omitted values preserved and explicit clears applied", updated)
	}

	updateTrigger, _ := registry.Get("workflow_triggers.update")
	output, err = updateTrigger.Handler(t.Context(), map[string]any{
		"workflowId": currentWorkflow.ID, "triggerId": currentTrigger.ID, "title": "Renamed Webhook", "config": map[string]any{},
	})
	if err != nil {
		t.Fatalf("workflow_triggers.update: %v", err)
	}
	updatedTrigger := output.(jfadk.WorkflowTrigger)
	if updatedTrigger.Type != jfadk.WorkflowTriggerTypeWebhook || updatedTrigger.Title != "Renamed Webhook" || len(updatedTrigger.Config) != 0 || spy.savedTrigger.ResetSecret {
		t.Fatalf("updated trigger = %+v saved=%+v", updatedTrigger, spy.savedTrigger)
	}
	if _, err := updateTrigger.Handler(t.Context(), map[string]any{
		"workflowId": currentWorkflow.ID, "triggerId": currentTrigger.ID, "type": jfadk.WorkflowTriggerTypeManual,
	}); err == nil || !strings.Contains(err.Error(), "UI/API") {
		t.Fatalf("webhook type change err = %v", err)
	}
	createTrigger, _ := registry.Get("workflow_triggers.create")
	if _, err := createTrigger.Handler(t.Context(), map[string]any{
		"workflowId": currentWorkflow.ID, "type": jfadk.WorkflowTriggerTypeWebhook,
	}); err == nil || !strings.Contains(err.Error(), "UI/API") {
		t.Fatalf("webhook create err = %v", err)
	}
}

func TestWorkflowManagementToolListsCreatesAndDeletes(t *testing.T) {
	workflow := jfadk.WorkflowDefinition{ID: "workflow-crud", Name: "CRUD Workflow", AgentID: "agent-1"}
	trigger := jfadk.WorkflowTrigger{ID: "trigger-crud", WorkflowID: workflow.ID, Type: jfadk.WorkflowTriggerTypeManual}
	run := jfadk.WorkflowTriggerLog{ID: "run-crud", WorkflowID: workflow.ID, TriggerID: trigger.ID}
	spy := &workflowToolManagerSpy{workflow: workflow, trigger: trigger, run: run}
	registry := jfadk.NewToolRegistry()
	registerJFTradeADKWorkflowManagementTools(nil, registry, spy)

	call := func(name string, input map[string]any) any {
		t.Helper()
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("tool %q not found", name)
		}
		output, err := tool.Handler(t.Context(), input)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return output
	}

	workflowList := call("workflows.list", map[string]any{"limit": 10}).(map[string]any)
	if len(workflowList["workflows"].([]map[string]any)) != 1 {
		t.Fatalf("workflow list = %#v", workflowList)
	}
	created := call("workflows.create", map[string]any{
		"name": "Created", "agentId": "agent-1", "promptTemplate": "Run {{symbol}}",
	}).(jfadk.WorkflowDefinition)
	if created.Name != "Created" || spy.savedWorkflow.PromptTemplate != "Run {{symbol}}" {
		t.Fatalf("created workflow = %+v saved=%+v", created, spy.savedWorkflow)
	}
	deletedWorkflow := call("workflows.delete", map[string]any{"workflowId": workflow.ID}).(map[string]any)
	if deletedWorkflow["deleted"] != true {
		t.Fatalf("deleted workflow = %#v", deletedWorkflow)
	}

	triggerList := call("workflow_triggers.list", map[string]any{"workflowId": workflow.ID}).(map[string]any)
	if len(triggerList["triggers"].([]jfadk.WorkflowTrigger)) != 1 {
		t.Fatalf("trigger list = %#v", triggerList)
	}
	deletedTrigger := call("workflow_triggers.delete", map[string]any{
		"workflowId": workflow.ID, "triggerId": trigger.ID,
	}).(map[string]any)
	if deletedTrigger["deleted"] != true {
		t.Fatalf("deleted trigger = %#v", deletedTrigger)
	}

	runList := call("workflow_runs.list", map[string]any{"workflowId": workflow.ID}).(map[string]any)
	if len(runList["runs"].([]map[string]any)) != 1 {
		t.Fatalf("run list = %#v", runList)
	}
	gotRun := call("workflow_runs.get", map[string]any{"logId": run.ID}).(jfadk.WorkflowTriggerLog)
	if gotRun.ID != run.ID {
		t.Fatalf("run = %+v", gotRun)
	}
}

func TestWorkflowRunToolsRequireInteractiveSession(t *testing.T) {
	store, err := jfadk.NewStore(filepath.Join(t.TempDir(), "adk.db"), filepath.Join(t.TempDir(), "secrets"), filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	interactive, err := store.CreateSession(t.Context(), "agent", "Interactive")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	workflowSession, err := store.CreateSessionWithSource(t.Context(), "agent", "Workflow", "workflow-parent", "Parent")
	if err != nil {
		t.Fatalf("CreateSessionWithSource: %v", err)
	}
	spy := &workflowToolManagerSpy{}
	registry := jfadk.NewToolRegistry()
	registerJFTradeADKWorkflowManagementTools(store, registry, spy)
	runTool, _ := registry.Get("workflows.run")

	if _, err := runTool.Handler(t.Context(), map[string]any{"workflowId": "workflow-1"}); err == nil || !strings.Contains(err.Error(), "resolvable") {
		t.Fatalf("missing session err = %v", err)
	}
	if _, err := runTool.Handler(workflowToolSessionContext{Context: t.Context(), sessionID: workflowSession.ID}, map[string]any{"workflowId": "workflow-1"}); err == nil || !strings.Contains(err.Error(), "cannot start") {
		t.Fatalf("workflow source session err = %v", err)
	}
	output, err := runTool.Handler(workflowToolSessionContext{Context: t.Context(), sessionID: interactive.ID}, map[string]any{"workflowId": "workflow-1", "inputs": map[string]any{"symbol": "US.AAPL"}})
	if err != nil {
		t.Fatalf("interactive workflows.run: %v", err)
	}
	result := output.(WorkflowToolStartResult)
	if !result.Accepted || spy.startedWorkflowID != "workflow-1" || spy.startedInputs["symbol"] != "US.AAPL" {
		t.Fatalf("result=%+v spy=%+v", result, spy)
	}
	triggerRunTool, _ := registry.Get("workflow_triggers.run")
	output, err = triggerRunTool.Handler(workflowToolSessionContext{Context: t.Context(), sessionID: interactive.ID}, map[string]any{
		"triggerId": "trigger-1", "inputs": map[string]any{"symbol": "US.MSFT"},
	})
	if err != nil {
		t.Fatalf("interactive workflow_triggers.run: %v", err)
	}
	if !output.(WorkflowToolStartResult).Accepted || spy.startedTriggerID != "trigger-1" || spy.startedInputs["symbol"] != "US.MSFT" {
		t.Fatalf("trigger result=%+v spy=%+v", output, spy)
	}
}

func TestUnavailableWorkflowToolManagerFailsClosed(t *testing.T) {
	manager := unavailableWorkflowToolManager{}
	ctx := t.Context()
	errors := make([]error, 0, 12)
	_, err := manager.ListWorkflows(ctx, "", 20, 0)
	errors = append(errors, err)
	_, err = manager.GetWorkflow(ctx, "workflow")
	errors = append(errors, err)
	_, err = manager.SaveWorkflow(ctx, "workflow", jfadk.WorkflowDefinitionWriteRequest{})
	errors = append(errors, err)
	_, err = manager.DeleteWorkflow(ctx, "workflow")
	errors = append(errors, err)
	_, err = manager.ListWorkflowTriggers(ctx, "workflow")
	errors = append(errors, err)
	_, err = manager.GetWorkflowTrigger(ctx, "workflow", "trigger")
	errors = append(errors, err)
	_, err = manager.SaveWorkflowTrigger(ctx, "workflow", "trigger", jfadk.WorkflowTriggerWriteRequest{})
	errors = append(errors, err)
	_, err = manager.DeleteWorkflowTrigger(ctx, "workflow", "trigger")
	errors = append(errors, err)
	_, err = manager.ListWorkflowRuns(ctx, "workflow", "trigger", "", 20, 0)
	errors = append(errors, err)
	_, err = manager.GetWorkflowRun(ctx, "log")
	errors = append(errors, err)
	_, err = manager.StartWorkflow(ctx, "workflow", nil)
	errors = append(errors, err)
	_, err = manager.StartWorkflowTrigger(ctx, "trigger", nil)
	errors = append(errors, err)

	for index, callErr := range errors {
		if callErr == nil || !strings.Contains(callErr.Error(), "workflow management is unavailable") {
			t.Fatalf("unavailable manager call %d error = %v", index, callErr)
		}
	}

	limit, offset := workflowToolPageBounds(map[string]any{"limit": 500, "offset": 7})
	if limit != 100 || offset != 7 {
		t.Fatalf("workflow page bounds = %d/%d", limit, offset)
	}
	workflowSummary := workflowToolSummary(jfadk.WorkflowDefinition{ID: "workflow", Name: "Workflow"})
	if workflowSummary["id"] != "workflow" || workflowSummary["name"] != "Workflow" {
		t.Fatalf("workflow summary = %#v", workflowSummary)
	}
	runSummary := workflowRunToolSummary(jfadk.WorkflowTriggerLog{ID: "log", WorkflowID: "workflow"})
	if runSummary["id"] != "log" || runSummary["workflowId"] != "workflow" {
		t.Fatalf("workflow run summary = %#v", runSummary)
	}
}

type workflowToolManagerSpy struct {
	workflow          jfadk.WorkflowDefinition
	trigger           jfadk.WorkflowTrigger
	run               jfadk.WorkflowTriggerLog
	savedWorkflow     jfadk.WorkflowDefinitionWriteRequest
	savedTrigger      jfadk.WorkflowTriggerWriteRequest
	startedWorkflowID string
	startedTriggerID  string
	startedInputs     map[string]any
}

func (s *workflowToolManagerSpy) ListWorkflows(_ context.Context, _ string, limit int, offset int) (WorkflowToolPage[jfadk.WorkflowDefinition], error) {
	return WorkflowToolPage[jfadk.WorkflowDefinition]{Items: []jfadk.WorkflowDefinition{s.workflow}, Total: 1, Limit: limit, Offset: offset}, nil
}

func (s *workflowToolManagerSpy) GetWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error) {
	return s.workflow, nil
}

func (s *workflowToolManagerSpy) SaveWorkflow(_ context.Context, _ string, payload jfadk.WorkflowDefinitionWriteRequest) (jfadk.WorkflowDefinition, error) {
	s.savedWorkflow = payload
	return jfadk.WorkflowDefinition{ID: s.workflow.ID, Name: payload.Name, Description: payload.Description, Status: payload.Status, AgentID: payload.AgentID, WorkMode: payload.WorkMode, ProviderID: payload.ProviderID, Model: payload.Model, PermissionMode: payload.PermissionMode, PromptTemplate: payload.PromptTemplate, ObjectiveTemplate: payload.ObjectiveTemplate, DefaultInputs: payload.DefaultInputs, CanvasGraph: payload.CanvasGraph, Tags: payload.Tags}, nil
}

func (s *workflowToolManagerSpy) DeleteWorkflow(context.Context, string) (jfadk.WorkflowDefinition, error) {
	return s.workflow, nil
}

func (s *workflowToolManagerSpy) ListWorkflowTriggers(context.Context, string) ([]jfadk.WorkflowTrigger, error) {
	return []jfadk.WorkflowTrigger{s.trigger}, nil
}

func (s *workflowToolManagerSpy) GetWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error) {
	return s.trigger, nil
}

func (s *workflowToolManagerSpy) SaveWorkflowTrigger(_ context.Context, workflowID string, triggerID string, payload jfadk.WorkflowTriggerWriteRequest) (jfadk.WorkflowTrigger, error) {
	s.savedTrigger = payload
	return jfadk.WorkflowTrigger{ID: triggerID, WorkflowID: workflowID, Type: payload.Type, Title: payload.Title, Status: payload.Status, Config: payload.Config}, nil
}

func (s *workflowToolManagerSpy) DeleteWorkflowTrigger(context.Context, string, string) (jfadk.WorkflowTrigger, error) {
	return s.trigger, nil
}

func (s *workflowToolManagerSpy) ListWorkflowRuns(_ context.Context, _ string, _ string, _ string, limit int, offset int) (WorkflowToolPage[jfadk.WorkflowTriggerLog], error) {
	return WorkflowToolPage[jfadk.WorkflowTriggerLog]{Items: []jfadk.WorkflowTriggerLog{s.run}, Total: 1, Limit: limit, Offset: offset}, nil
}

func (s *workflowToolManagerSpy) GetWorkflowRun(context.Context, string) (jfadk.WorkflowTriggerLog, error) {
	return s.run, nil
}

func (s *workflowToolManagerSpy) StartWorkflow(_ context.Context, workflowID string, inputs map[string]any) (WorkflowToolStartResult, error) {
	s.startedWorkflowID = workflowID
	s.startedInputs = inputs
	return WorkflowToolStartResult{Accepted: true, Workflow: jfadk.WorkflowDefinition{ID: workflowID}, Log: jfadk.WorkflowTriggerLog{Status: jfadk.WorkflowTriggerLogStatusQueued}}, nil
}

func (s *workflowToolManagerSpy) StartWorkflowTrigger(_ context.Context, triggerID string, inputs map[string]any) (WorkflowToolStartResult, error) {
	s.startedTriggerID = triggerID
	s.startedInputs = inputs
	return WorkflowToolStartResult{Accepted: true, Trigger: &jfadk.WorkflowTrigger{ID: triggerID}, Log: jfadk.WorkflowTriggerLog{Status: jfadk.WorkflowTriggerLogStatusQueued}}, nil
}

type workflowToolSessionContext struct {
	context.Context
	sessionID string
}

func (c workflowToolSessionContext) SessionID() string { return c.sessionID }

func workflowToolDescriptorsContain(items []jfadk.ToolDescriptor, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
