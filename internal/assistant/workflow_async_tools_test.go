package assistant

import (
	"strings"
	"testing"
	"time"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestStartWorkflowQueuesAndCompletesInBackground(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "workflow-async-agent", Name: "Workflow Async Agent", Status: assistantmodel.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workflow, err := service.SaveWorkflow(ctx, "", assistantmodel.WorkflowDefinitionWriteRequest{
		ID: "workflow-async", Name: "Workflow Async", Status: assistantmodel.WorkflowStatusEnabled,
		AgentID: agent.ID, WorkMode: assistantmodel.WorkModeChat, PromptTemplate: "run {{ .symbol }}",
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	started, err := service.StartWorkflow(ctx, workflow.ID, map[string]any{"symbol": "US.AAPL"})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	if !started.Accepted || started.Log.Status != assistantmodel.WorkflowTriggerLogStatusQueued || started.Log.ID == "" {
		t.Fatalf("StartWorkflow result = %+v, want accepted queued log", started)
	}
	completed := waitForWorkflowRunStatus(t, runtime, started.Log.ID, assistantmodel.WorkflowTriggerLogStatusSucceeded)
	if completed.RunID == "" || completed.SessionID == "" || completed.Result == nil {
		t.Fatalf("completed workflow log = %+v, want run/session/result", completed)
	}
	fetched, err := service.GetWorkflowTriggerLog(ctx, completed.ID)
	if err != nil || fetched.Status != assistantmodel.WorkflowTriggerLogStatusSucceeded {
		t.Fatalf("GetWorkflowTriggerLog = %+v err=%v", fetched, err)
	}
}

func TestStartWorkflowTriggerSkipsWhenPreviousRunIsActive(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-async-trigger", assistantmodel.WorkflowStatusEnabled)
	workflow, err := service.SaveWorkflow(ctx, workflow.ID, assistantmodel.WorkflowDefinitionWriteRequest{
		Name: workflow.Name, Status: assistantmodel.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: assistantmodel.WorkModeChat, PromptTemplate: workflow.PromptTemplate, CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	triggerResult, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", assistantmodel.WorkflowTriggerWriteRequest{
		ID: "workflow-async-trigger-manual", Type: assistantmodel.WorkflowTriggerTypeManual, Status: assistantmodel.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}
	if _, err := runtime.Store().SaveWorkflowTriggerLog(ctx, assistantmodel.WorkflowTriggerLog{
		ID: "workflow-async-trigger-active", WorkflowID: workflow.ID, TriggerID: triggerResult.Trigger.ID,
		TriggerType: triggerResult.Trigger.Type, Status: assistantmodel.WorkflowTriggerLogStatusQueued,
	}); err != nil {
		t.Fatalf("SaveWorkflowTriggerLog: %v", err)
	}

	started, err := service.StartWorkflowTrigger(ctx, triggerResult.Trigger.ID, nil)
	if err != nil {
		t.Fatalf("StartWorkflowTrigger: %v", err)
	}
	if started.Accepted || started.Log.Status != assistantmodel.WorkflowTriggerLogStatusSkipped || !strings.Contains(started.Log.Error, "still active") {
		t.Fatalf("StartWorkflowTrigger result = %+v, want rejected skipped log", started)
	}
	trigger, err := service.GetWorkflowTrigger(ctx, workflow.ID, triggerResult.Trigger.ID)
	if err != nil || trigger.ID != triggerResult.Trigger.ID || trigger.SecretHash != "" {
		t.Fatalf("GetWorkflowTrigger = %+v err=%v", trigger, err)
	}
}

func TestStartWorkflowBackgroundFailureTerminatesLog(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-async-failure", assistantmodel.WorkflowStatusEnabled)
	workflow, err := service.SaveWorkflow(ctx, workflow.ID, assistantmodel.WorkflowDefinitionWriteRequest{
		Name: workflow.Name, Status: assistantmodel.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: assistantmodel.WorkModeChat, PromptTemplate: workflow.PromptTemplate,
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	started, err := service.StartWorkflow(ctx, workflow.ID, nil)
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	failed := waitForWorkflowRunStatus(t, runtime, started.Log.ID, assistantmodel.WorkflowTriggerLogStatusFailed)
	if !strings.Contains(failed.Error, "canvas graph is required") || failed.FinishedAt == "" {
		t.Fatalf("failed workflow log = %+v", failed)
	}
}

func waitForWorkflowRunStatus(t *testing.T, runtime *jfadkruntime.Runtime, logID string, status string) assistantmodel.WorkflowTriggerLog {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		log, ok, err := runtime.Store().WorkflowTriggerLog(t.Context(), logID)
		if err != nil {
			t.Fatalf("WorkflowTriggerLog: %v", err)
		}
		if ok && log.Status == status {
			return log
		}
		time.Sleep(10 * time.Millisecond)
	}
	log, _, _ := runtime.Store().WorkflowTriggerLog(t.Context(), logID)
	t.Fatalf("workflow log %q status = %q, want %q", logID, log.Status, status)
	return assistantmodel.WorkflowTriggerLog{}
}
