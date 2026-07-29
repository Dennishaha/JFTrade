package assembly

import (
	"path/filepath"
	"strings"
	"testing"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	adksession "google.golang.org/adk/v2/session"
)

func TestWorkflowManagerProjectsServiceCRUDAndRuns(t *testing.T) {
	runtime, service := newWorkflowBridgeService(t)
	manager := NewWorkflowToolManager(func() *assistant.Service { return service })
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-bridge-agent", Name: "Workflow Bridge Agent", Status: jfadk.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workflow, err := manager.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID: "workflow-bridge", Name: "Workflow Bridge", Status: jfadk.WorkflowStatusDisabled,
		AgentID: agent.ID, WorkMode: jfadk.WorkModeLoop, PromptTemplate: "Review {{symbol}}",
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	page, err := manager.ListWorkflows(ctx, jfadk.WorkflowStatusDisabled, 5, 0)
	if err != nil || page.Total != 1 || page.Limit != 5 || page.Offset != 0 || len(page.Items) != 1 {
		t.Fatalf("ListWorkflows = %+v, err=%v", page, err)
	}
	gotWorkflow, err := manager.GetWorkflow(ctx, workflow.ID)
	if err != nil || gotWorkflow.ID != workflow.ID {
		t.Fatalf("GetWorkflow = %+v, err=%v", gotWorkflow, err)
	}
	workflow.Description = "updated through bridge"
	updated, err := manager.SaveWorkflow(ctx, workflow.ID, workflowWriteRequest(workflow))
	if err != nil || updated.Description != workflow.Description {
		t.Fatalf("update workflow = %+v, err=%v", updated, err)
	}

	trigger, err := manager.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID: "workflow-bridge-trigger", Type: jfadk.WorkflowTriggerTypeManual,
		Title: "Manual bridge", Status: jfadk.WorkflowTriggerStatusDisabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}
	triggers, err := manager.ListWorkflowTriggers(ctx, workflow.ID)
	if err != nil || len(triggers) != 1 || triggers[0].ID != trigger.ID {
		t.Fatalf("ListWorkflowTriggers = %+v, err=%v", triggers, err)
	}
	gotTrigger, err := manager.GetWorkflowTrigger(ctx, workflow.ID, trigger.ID)
	if err != nil || gotTrigger.ID != trigger.ID {
		t.Fatalf("GetWorkflowTrigger = %+v, err=%v", gotTrigger, err)
	}
	trigger.Title = "Updated bridge trigger"
	trigger, err = manager.SaveWorkflowTrigger(ctx, workflow.ID, trigger.ID, jfadk.WorkflowTriggerWriteRequest{
		Type: trigger.Type, Title: trigger.Title, Status: trigger.Status, Config: trigger.Config,
	})
	if err != nil || trigger.Title != "Updated bridge trigger" {
		t.Fatalf("update workflow trigger = %+v, err=%v", trigger, err)
	}

	logEntry, err := runtime.Store().SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
		ID: "workflow-bridge-run", WorkflowID: workflow.ID, TriggerID: trigger.ID,
		TriggerType: trigger.Type, Status: jfadk.WorkflowTriggerLogStatusSucceeded,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTriggerLog: %v", err)
	}
	runs, err := manager.ListWorkflowRuns(ctx, workflow.ID, trigger.ID, logEntry.Status, 10, 0)
	if err != nil || runs.Total != 1 || runs.Limit != 10 || len(runs.Items) != 1 {
		t.Fatalf("ListWorkflowRuns = %+v, err=%v", runs, err)
	}
	gotRun, err := manager.GetWorkflowRun(ctx, logEntry.ID)
	if err != nil || gotRun.ID != logEntry.ID {
		t.Fatalf("GetWorkflowRun = %+v, err=%v", gotRun, err)
	}

	if result, startErr := manager.StartWorkflow(ctx, "missing-workflow", map[string]any{"symbol": "US.AAPL"}); startErr == nil || result.Accepted {
		t.Fatalf("StartWorkflow missing = %+v, err=%v", result, startErr)
	}
	if result, startErr := manager.StartWorkflowTrigger(ctx, "missing-trigger", nil); startErr == nil || result.Accepted {
		t.Fatalf("StartWorkflowTrigger missing = %+v, err=%v", result, startErr)
	}
	deletedTrigger, err := manager.DeleteWorkflowTrigger(ctx, workflow.ID, trigger.ID)
	if err != nil || deletedTrigger.DeletedAt == nil {
		t.Fatalf("DeleteWorkflowTrigger = %+v, err=%v", deletedTrigger, err)
	}
	deletedWorkflow, err := manager.DeleteWorkflow(ctx, workflow.ID)
	if err != nil || deletedWorkflow.DeletedAt == nil {
		t.Fatalf("DeleteWorkflow = %+v, err=%v", deletedWorkflow, err)
	}
}

func TestWorkflowManagerRejectsUnavailableServicesAcrossOperations(t *testing.T) {
	ctx := t.Context()
	unavailable := NewWorkflowToolManager(nil)
	operations := map[string]func() error{
		"list workflows": func() error { _, err := unavailable.ListWorkflows(ctx, "", 10, 0); return err },
		"get workflow":   func() error { _, err := unavailable.GetWorkflow(ctx, "workflow"); return err },
		"save workflow": func() error {
			_, err := unavailable.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{})
			return err
		},
		"delete workflow": func() error { _, err := unavailable.DeleteWorkflow(ctx, "workflow"); return err },
		"list triggers":   func() error { _, err := unavailable.ListWorkflowTriggers(ctx, "workflow"); return err },
		"get trigger":     func() error { _, err := unavailable.GetWorkflowTrigger(ctx, "workflow", "trigger"); return err },
		"save trigger": func() error {
			_, err := unavailable.SaveWorkflowTrigger(ctx, "workflow", "", jfadk.WorkflowTriggerWriteRequest{})
			return err
		},
		"delete trigger": func() error { _, err := unavailable.DeleteWorkflowTrigger(ctx, "workflow", "trigger"); return err },
		"list runs":      func() error { _, err := unavailable.ListWorkflowRuns(ctx, "", "", "", 10, 0); return err },
		"get run":        func() error { _, err := unavailable.GetWorkflowRun(ctx, "run"); return err },
		"start workflow": func() error { _, err := unavailable.StartWorkflow(ctx, "workflow", nil); return err },
		"start trigger":  func() error { _, err := unavailable.StartWorkflowTrigger(ctx, "trigger", nil); return err },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(); err == nil || !strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("error = %v, want unavailable", err)
			}
		})
	}

	for name, provider := range map[string]WorkflowServiceProvider{
		"nil service":   func() *assistant.Service { return nil },
		"closed facade": func() *assistant.Service { return assistant.NewService(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewWorkflowToolManager(provider).GetWorkflow(ctx, "workflow"); err == nil {
				t.Fatal("GetWorkflow error = nil, want unavailable")
			}
		})
	}
}

func newWorkflowBridgeService(t *testing.T) (*assistanttestkit.Runtime, *assistant.Service) {
	t.Helper()
	root := t.TempDir()
	store, err := assistanttestkit.NewStore(
		filepath.Join(root, "adk.db"),
		filepath.Join(root, "secrets.json"),
		filepath.Join(root, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	runtime := assistanttestkit.NewRuntimeWithSessionService(
		store,
		assistanttestkit.NewToolRegistry(),
		adksession.InMemoryService(),
	)
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("runtime.Close: %v", err)
		}
	})
	return runtime, assistant.NewService(runtime)
}

func workflowWriteRequest(workflow jfadk.WorkflowDefinition) jfadk.WorkflowDefinitionWriteRequest {
	return jfadk.WorkflowDefinitionWriteRequest{
		ID: workflow.ID, Name: workflow.Name, Description: workflow.Description, Status: workflow.Status,
		AgentID: workflow.AgentID, WorkMode: workflow.WorkMode, ProviderID: workflow.ProviderID,
		Model: workflow.Model, PermissionMode: workflow.PermissionMode, PromptTemplate: workflow.PromptTemplate,
		ObjectiveTemplate: workflow.ObjectiveTemplate, DefaultInputs: workflow.DefaultInputs,
		CanvasGraph: workflow.CanvasGraph, Tags: workflow.Tags,
	}
}
