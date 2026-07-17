package assistant

import (
	"context"
	"strings"
	"testing"

	jadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestCoverage98WorkflowResourcesRejectCrossWorkflowAndInvalidRequests(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	firstAgent, first := saveWorkflowTestAgentAndDefinition(t, runtime, service, "coverage98-workflow-first", jadk.WorkflowStatusEnabled)
	_, second := saveWorkflowTestAgentAndDefinition(t, runtime, service, "coverage98-workflow-second", jadk.WorkflowStatusEnabled)
	trigger, err := service.SaveWorkflowTrigger(ctx, first.ID, "", jadk.WorkflowTriggerWriteRequest{
		ID: "coverage98-workflow-first-trigger", Type: jadk.WorkflowTriggerTypeManual, Status: jadk.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}

	if _, err := service.SaveWorkflow(ctx, "coverage98-workflow-missing", jadk.WorkflowDefinitionWriteRequest{}); err == nil || !strings.Contains(err.Error(), "workflow not found") {
		t.Fatalf("SaveWorkflow missing update error = %v", err)
	}
	if _, err := service.SaveWorkflow(ctx, "", jadk.WorkflowDefinitionWriteRequest{
		ID: "coverage98-workflow-invalid-mode", Name: "Invalid Mode", Status: jadk.WorkflowStatusDisabled,
		AgentID: firstAgent.ID, WorkMode: "batch", PromptTemplate: "review",
	}); err == nil || !strings.Contains(err.Error(), "invalid workflow work mode") {
		t.Fatalf("SaveWorkflow invalid mode error = %v", err)
	}
	if _, err := service.GetWorkflowTrigger(ctx, second.ID, trigger.Trigger.ID); err == nil || !strings.Contains(err.Error(), "workflow trigger not found") {
		t.Fatalf("GetWorkflowTrigger cross-workflow error = %v", err)
	}
	if _, err := service.SaveWorkflowTrigger(ctx, second.ID, trigger.Trigger.ID, jadk.WorkflowTriggerWriteRequest{Type: jadk.WorkflowTriggerTypeManual}); err == nil || !strings.Contains(err.Error(), "workflow trigger not found") {
		t.Fatalf("SaveWorkflowTrigger cross-workflow error = %v", err)
	}
	if _, err := service.DeleteWorkflowTrigger(ctx, second.ID, trigger.Trigger.ID); err == nil || !strings.Contains(err.Error(), "workflow trigger not found") {
		t.Fatalf("DeleteWorkflowTrigger cross-workflow error = %v", err)
	}
	if _, err := service.GetWorkflowTriggerLog(ctx, "coverage98-workflow-log-missing"); err == nil || !strings.Contains(err.Error(), "workflow run not found") {
		t.Fatalf("GetWorkflowTriggerLog missing error = %v", err)
	}
	if _, err := service.RunWorkflowTrigger(ctx, "coverage98-workflow-trigger-missing", nil); err == nil || !strings.Contains(err.Error(), "workflow trigger not found") {
		t.Fatalf("RunWorkflowTrigger missing error = %v", err)
	}
	if _, err := service.StartWorkflowTrigger(ctx, "coverage98-workflow-trigger-missing", nil); err == nil || !strings.Contains(err.Error(), "workflow trigger not found") {
		t.Fatalf("StartWorkflowTrigger missing error = %v", err)
	}
}

func TestCoverage98WorkflowAsyncTriggerAndBackgroundRecovery(t *testing.T) {
	ctx := t.Context()

	t.Run("accepted trigger runs asynchronously with a detached trigger copy", func(t *testing.T) {
		runtime, service, _ := newAssistantServiceHarness(t)
		assistantServiceProvider(t, runtime)
		_, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "coverage98-workflow-async", jadk.WorkflowStatusEnabled)
		trigger, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jadk.WorkflowTriggerWriteRequest{
			ID: "coverage98-workflow-async-trigger", Type: jadk.WorkflowTriggerTypeManual, Status: jadk.WorkflowTriggerStatusEnabled,
		})
		if err != nil {
			t.Fatalf("SaveWorkflowTrigger: %v", err)
		}
		started, err := service.StartWorkflowTrigger(ctx, trigger.Trigger.ID, map[string]any{"symbol": "US.MSFT"})
		if err != nil || !started.Accepted || started.Trigger == nil || started.Trigger.ID != trigger.Trigger.ID || started.Trigger.SecretHash != "" {
			t.Fatalf("StartWorkflowTrigger = %+v err=%v", started, err)
		}
		finished := waitForWorkflowRunStatus(t, runtime, started.Log.ID, jadk.WorkflowTriggerLogStatusSucceeded)
		if finished.TriggerID != trigger.Trigger.ID || finished.SessionID == "" {
			t.Fatalf("async trigger log = %+v", finished)
		}
	})

	t.Run("template failures happen before runtime access", func(t *testing.T) {
		workflow := jadk.WorkflowDefinition{CanvasGraph: &jadk.WorkflowCanvasGraph{Nodes: []jadk.WorkflowCanvasNode{{
			ID: "agent", Type: "agent", Data: map[string]any{"promptTemplate": "{{"},
		}}}}
		if _, err := (&Service{}).runWorkflowCanvas(ctx, workflow, nil, "session", "message", "objective", nil, nil); err == nil || !strings.Contains(err.Error(), "render canvas node") {
			t.Fatalf("runWorkflowCanvas template error = %v", err)
		}
	})

	t.Run("background panic is converted into a durable failed workflow log", func(t *testing.T) {
		runtime, _, _ := newAssistantServiceHarness(t)
		store := &workflowInvocationFaultStore{base: runtime.Store()}
		workflow := jadk.WorkflowDefinition{
			ID: "coverage98-workflow-background-panic", Name: "Background Panic", Status: jadk.WorkflowStatusEnabled,
			AgentID: "coverage98-agent", PromptTemplate: "render before service access",
		}
		var unavailable *Service
		unavailable.executeQueuedWorkflowBackground(context.Background(), store, workflow, nil, nil, nil, jadk.WorkflowTriggerLog{
			WorkflowID: workflow.ID, TriggerType: jadk.WorkflowTriggerTypeManual, Status: jadk.WorkflowTriggerLogStatusQueued,
		})
		if len(store.savedLogs) < 2 {
			t.Fatalf("background recovery saved logs = %+v, want running and failed records", store.savedLogs)
		}
		failed := store.savedLogs[len(store.savedLogs)-1]
		if failed.Status != jadk.WorkflowTriggerLogStatusFailed || !strings.Contains(failed.Error, "workflow background panic") || failed.FinishedAt == "" {
			t.Fatalf("background panic log = %+v", failed)
		}
	})
}

func TestCoverage98WorkflowBackgroundPersistsFailureAfterRunningTransitionWriteFails(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	workflow := jadk.WorkflowDefinition{
		ID:             "coverage98-workflow-running-write-failure",
		Name:           "Running Write Failure",
		Status:         jadk.WorkflowStatusEnabled,
		AgentID:        "coverage98-agent",
		PromptTemplate: "review {{.symbol}}",
	}
	store := &workflowInvocationFaultStore{base: runtime.Store(), failSaveAt: 2}

	prepared, accepted, err := prepareWorkflowInvocation(
		t.Context(), store, workflow, nil, jadk.WorkflowTriggerTypeManual, map[string]any{"symbol": "US.AAPL"}, nil,
	)
	if err != nil || !accepted || prepared.Log.ID == "" || prepared.Log.Status != jadk.WorkflowTriggerLogStatusQueued {
		t.Fatalf("prepareWorkflowInvocation result=%+v accepted=%v err=%v", prepared, accepted, err)
	}

	service.executeQueuedWorkflowBackground(t.Context(), store, workflow, nil, map[string]any{"symbol": "US.AAPL"}, nil, prepared.Log)

	if store.saveCalls != 3 || len(store.savedLogs) != 2 {
		t.Fatalf("workflow log writes = calls:%d logs:%+v, want queued and recovered failed records", store.saveCalls, store.savedLogs)
	}
	failed := store.savedLogs[len(store.savedLogs)-1]
	if failed.ID != prepared.Log.ID || failed.Status != jadk.WorkflowTriggerLogStatusFailed || failed.FinishedAt == "" || !strings.Contains(failed.Error, errWorkflowLogWriteInjected.Error()) {
		t.Fatalf("recovered failed workflow log = %+v", failed)
	}
}
