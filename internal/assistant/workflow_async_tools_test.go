package assistant

import (
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestStartWorkflowQueuesAndCompletesInBackground(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-async-agent", Name: "Workflow Async Agent", Status: jfadk.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	workflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID: "workflow-async", Name: "Workflow Async", Status: jfadk.WorkflowStatusEnabled,
		AgentID: agent.ID, WorkMode: jfadk.WorkModeChat, PromptTemplate: "run {{ .symbol }}",
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	started, err := service.StartWorkflow(ctx, workflow.ID, map[string]any{"symbol": "US.AAPL"})
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	if !started.Accepted || started.Log.Status != jfadk.WorkflowTriggerLogStatusQueued || started.Log.ID == "" {
		t.Fatalf("StartWorkflow result = %+v, want accepted queued log", started)
	}
	completed := waitForWorkflowRunStatus(t, runtime, started.Log.ID, jfadk.WorkflowTriggerLogStatusSucceeded)
	if completed.RunID == "" || completed.SessionID == "" || completed.Result == nil {
		t.Fatalf("completed workflow log = %+v, want run/session/result", completed)
	}
	fetched, err := service.GetWorkflowTriggerLog(ctx, completed.ID)
	if err != nil || fetched.Status != jfadk.WorkflowTriggerLogStatusSucceeded {
		t.Fatalf("GetWorkflowTriggerLog = %+v err=%v", fetched, err)
	}
}

func TestStartWorkflowTriggerSkipsWhenPreviousRunIsActive(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-async-trigger", jfadk.WorkflowStatusEnabled)
	workflow, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: workflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PromptTemplate: workflow.PromptTemplate, CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	triggerResult, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID: "workflow-async-trigger-manual", Type: jfadk.WorkflowTriggerTypeManual, Status: jfadk.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger: %v", err)
	}
	if _, err := runtime.Store().SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
		ID: "workflow-async-trigger-active", WorkflowID: workflow.ID, TriggerID: triggerResult.Trigger.ID,
		TriggerType: triggerResult.Trigger.Type, Status: jfadk.WorkflowTriggerLogStatusQueued,
	}); err != nil {
		t.Fatalf("SaveWorkflowTriggerLog: %v", err)
	}

	started, err := service.StartWorkflowTrigger(ctx, triggerResult.Trigger.ID, nil)
	if err != nil {
		t.Fatalf("StartWorkflowTrigger: %v", err)
	}
	if started.Accepted || started.Log.Status != jfadk.WorkflowTriggerLogStatusSkipped || !strings.Contains(started.Log.Error, "still active") {
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
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-async-failure", jfadk.WorkflowStatusEnabled)
	workflow, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: workflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PromptTemplate: workflow.PromptTemplate,
	})
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	started, err := service.StartWorkflow(ctx, workflow.ID, nil)
	if err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	failed := waitForWorkflowRunStatus(t, runtime, started.Log.ID, jfadk.WorkflowTriggerLogStatusFailed)
	if !strings.Contains(failed.Error, "canvas graph is required") || failed.FinishedAt == "" {
		t.Fatalf("failed workflow log = %+v", failed)
	}
}

func waitForWorkflowRunStatus(t *testing.T, runtime *jfadk.Runtime, logID string, status string) jfadk.WorkflowTriggerLog {
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
	return jfadk.WorkflowTriggerLog{}
}
