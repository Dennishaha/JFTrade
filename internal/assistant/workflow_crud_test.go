package assistant

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestWorkflowResourceCrudPaginationAndLogs(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-resource", jfadk.WorkflowStatusEnabled)

	updated, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name:           " Updated Workflow ",
		Status:         "disabled",
		AgentID:        agent.ID,
		WorkMode:       "loop",
		PermissionMode: jfadk.PermissionModeAll,
		PromptTemplate: " run updated ",
		DefaultInputs:  map[string]any{"symbol": "US.AAPL"},
		Tags:           []string{" daily ", "daily", "", "risk"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflow update: %v", err)
	}
	if updated.Name != "Updated Workflow" || updated.Status != jfadk.WorkflowStatusDisabled || updated.WorkMode != jfadk.WorkModeLoop {
		t.Fatalf("updated workflow = %+v", updated)
	}
	if got := strings.Join(updated.Tags, ","); got != "daily,risk" {
		t.Fatalf("updated tags = %q, want normalized unique tags", got)
	}

	page, err := service.ListWorkflows(ctx, WorkflowQuery{Status: jfadk.WorkflowStatusDisabled, Limit: 200, Offset: -10})
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if page.Limit != 100 || page.Offset != 0 || len(page.Items) == 0 {
		t.Fatalf("workflow page = %+v, want normalized limit/offset with items", page)
	}

	triggerResult, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID:     "workflow-resource-trigger",
		Type:   "",
		Title:  "",
		Status: "error",
		Config: map[string]any{"custom": true},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger manual fallback: %v", err)
	}
	if triggerResult.Trigger.Type != jfadk.WorkflowTriggerTypeManual || triggerResult.Trigger.Title != "手动触发" || triggerResult.Trigger.Status != jfadk.WorkflowTriggerStatusError {
		t.Fatalf("manual trigger result = %+v", triggerResult.Trigger)
	}

	triggers, err := service.ListWorkflowTriggers(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListWorkflowTriggers: %v", err)
	}
	if len(triggers) != 1 || triggers[0].SecretHash != "" {
		t.Fatalf("triggers = %+v, want sanitized trigger", triggers)
	}

	log, err := runtime.Store().SaveWorkflowTriggerLog(ctx, jfadk.WorkflowTriggerLog{
		WorkflowID:  workflow.ID,
		TriggerID:   triggerResult.Trigger.ID,
		TriggerType: triggerResult.Trigger.Type,
		Status:      jfadk.WorkflowTriggerLogStatusFailed,
		Error:       "unit failure",
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTriggerLog: %v", err)
	}
	logs, err := service.ListWorkflowTriggerLogs(ctx, WorkflowTriggerLogQuery{
		WorkflowID: workflow.ID,
		TriggerID:  triggerResult.Trigger.ID,
		Status:     jfadk.WorkflowTriggerLogStatusFailed,
		Limit:      0,
		Offset:     -1,
	})
	if err != nil {
		t.Fatalf("ListWorkflowTriggerLogs: %v", err)
	}
	if logs.Limit != 20 || logs.Offset != 0 || len(logs.Items) != 1 || logs.Items[0].ID != log.ID {
		t.Fatalf("workflow logs page = %+v, want saved failed log", logs)
	}

	deletedTrigger, err := service.DeleteWorkflowTrigger(ctx, workflow.ID, triggerResult.Trigger.ID)
	if err != nil {
		t.Fatalf("DeleteWorkflowTrigger: %v", err)
	}
	if deletedTrigger.DeletedAt == nil || deletedTrigger.Status != jfadk.WorkflowTriggerStatusDisabled {
		t.Fatalf("deleted trigger = %+v, want disabled soft-delete", deletedTrigger)
	}
	if _, err := service.DeleteWorkflowTrigger(ctx, workflow.ID, triggerResult.Trigger.ID); err == nil {
		t.Fatal("DeleteWorkflowTrigger deleted trigger succeeded, want not found")
	}

	deletedWorkflow, err := service.DeleteWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
	if deletedWorkflow.DeletedAt == nil || deletedWorkflow.Status != jfadk.WorkflowStatusDisabled {
		t.Fatalf("deleted workflow = %+v, want disabled soft-delete", deletedWorkflow)
	}
	if _, err := service.GetWorkflow(ctx, workflow.ID); err == nil {
		t.Fatal("GetWorkflow deleted workflow succeeded, want not found")
	}
}

func TestWorkflowTriggerRunWebhookAndValidationFailures(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-run-trigger", jfadk.WorkflowStatusEnabled)
	workflow, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name:           workflow.Name,
		Status:         jfadk.WorkflowStatusEnabled,
		AgentID:        agent.ID,
		WorkMode:       jfadk.WorkModeChat,
		PermissionMode: jfadk.PermissionModeApproval,
		PromptTemplate: workflow.PromptTemplate,
		DefaultInputs:  workflow.DefaultInputs,
		CanvasGraph:    workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow chat mode: %v", err)
	}

	manual, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID:     "workflow-run-trigger-manual",
		Type:   jfadk.WorkflowTriggerTypeManual,
		Status: jfadk.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger manual: %v", err)
	}
	result, err := service.RunWorkflowTrigger(ctx, manual.Trigger.ID, map[string]any{"symbol": "US.MSFT"})
	if err != nil {
		t.Fatalf("RunWorkflowTrigger manual: %v", err)
	}
	if result.Trigger == nil || result.Trigger.ID != manual.Trigger.ID || result.Log.TriggerID != manual.Trigger.ID || result.Log.Status != jfadk.WorkflowTriggerLogStatusSucceeded {
		t.Fatalf("manual trigger result = %+v", result)
	}

	disabledWebhook, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID:     "workflow-disabled-webhook",
		Type:   jfadk.WorkflowTriggerTypeWebhook,
		Status: jfadk.WorkflowTriggerStatusDisabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger disabled webhook: %v", err)
	}
	if _, err := service.RunWorkflowWebhook(ctx, disabledWebhook.Trigger.ID, disabledWebhook.Secret, nil); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("RunWorkflowWebhook disabled err = %v, want disabled", err)
	}

	enabledWebhook, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID:     "workflow-enabled-webhook",
		Type:   jfadk.WorkflowTriggerTypeWebhook,
		Title:  "External webhook",
		Status: jfadk.WorkflowTriggerStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger enabled webhook: %v", err)
	}
	if _, err := service.RunWorkflowWebhook(ctx, enabledWebhook.Trigger.ID, "bad-secret", nil); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("RunWorkflowWebhook invalid secret err = %v, want secret error", err)
	}
	webhookResult, err := service.RunWorkflowWebhook(ctx, enabledWebhook.Trigger.ID, enabledWebhook.Secret, map[string]any{"symbol": "US.GOOG"})
	if err != nil {
		t.Fatalf("RunWorkflowWebhook enabled: %v", err)
	}
	if webhookResult.Trigger == nil || webhookResult.Trigger.ID != enabledWebhook.Trigger.ID || webhookResult.Log.Status != jfadk.WorkflowTriggerLogStatusSucceeded {
		t.Fatalf("webhook invocation result = %+v, want succeeded trigger log", webhookResult)
	}
	if webhookResult.Log.MatchedEvent["type"] != "workflow.webhook" || webhookResult.Log.MatchedEvent["triggerId"] != enabledWebhook.Trigger.ID {
		t.Fatalf("webhook matched event = %+v, want webhook context", webhookResult.Log.MatchedEvent)
	}

	if _, err := service.RunWorkflowWebhook(ctx, "missing-webhook", enabledWebhook.Secret, nil); err == nil || !strings.Contains(err.Error(), "webhook") {
		t.Fatalf("RunWorkflowWebhook missing err = %v, want webhook not found", err)
	}
	missingWorkflowWebhook, err := runtime.Store().SaveWorkflowTrigger(ctx, jfadk.WorkflowTrigger{
		ID:         "workflow-webhook-missing-workflow",
		WorkflowID: "missing-workflow",
		Type:       jfadk.WorkflowTriggerTypeWebhook,
		Title:      "Missing workflow webhook",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
		SecretHash: hashWorkflowSecret("missing-workflow-secret"),
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger missing workflow webhook: %v", err)
	}
	if _, err := service.RunWorkflowWebhook(ctx, missingWorkflowWebhook.ID, "missing-workflow-secret", nil); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("RunWorkflowWebhook missing workflow err = %v, want workflow not found", err)
	}

	if _, err := service.RunWorkflowTrigger(ctx, "missing-trigger", nil); err == nil {
		t.Fatal("RunWorkflowTrigger missing succeeded, want error")
	}
	if _, err := service.RunWorkflowWebhook(ctx, manual.Trigger.ID, "secret", nil); err == nil || !strings.Contains(err.Error(), "webhook") {
		t.Fatalf("RunWorkflowWebhook non-webhook err = %v, want webhook not found", err)
	}
	if _, err := service.SaveWorkflowTrigger(ctx, workflow.ID, manual.Trigger.ID, jfadk.WorkflowTriggerWriteRequest{
		Type:   jfadk.WorkflowTriggerTypeSchedule,
		Status: jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{"cron": "bad"},
	}); err == nil {
		t.Fatal("SaveWorkflowTrigger invalid schedule succeeded, want cron error")
	}
	if _, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		Type:   jfadk.WorkflowTriggerTypeMarketThreshold,
		Status: jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{"instrumentIds": []string{"US.AAPL"}},
	}); err == nil || !strings.Contains(err.Error(), "numeric value") {
		t.Fatalf("SaveWorkflowTrigger invalid market err = %v, want numeric value", err)
	}
}

func TestWorkflowDefinitionValidationAndMissingResourceErrors(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-validation-agent", Name: "Validation Agent", Status: jfadk.AgentStatusEnabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	disabledAgent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-disabled-agent", Name: "Disabled Agent", Status: jfadk.AgentStatusDisabled,
		ProviderID: "test-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent disabled: %v", err)
	}

	for _, tc := range []struct {
		name    string
		payload jfadk.WorkflowDefinitionWriteRequest
		want    string
	}{
		{
			name: "missing name",
			payload: jfadk.WorkflowDefinitionWriteRequest{
				AgentID: agent.ID, PromptTemplate: "run",
			},
			want: "name",
		},
		{
			name: "missing agent id",
			payload: jfadk.WorkflowDefinitionWriteRequest{
				Name: "Missing Agent ID", PromptTemplate: "run",
			},
			want: "agentId",
		},
		{
			name: "missing agent",
			payload: jfadk.WorkflowDefinitionWriteRequest{
				Name: "Missing Agent", AgentID: "missing-agent", PromptTemplate: "run",
			},
			want: "agent not found",
		},
		{
			name: "enabled workflow requires enabled agent",
			payload: jfadk.WorkflowDefinitionWriteRequest{
				Name: "Disabled Agent Workflow", Status: jfadk.WorkflowStatusEnabled,
				AgentID: disabledAgent.ID, PromptTemplate: "run",
			},
			want: "enabled agent",
		},
		{
			name: "missing prompt",
			payload: jfadk.WorkflowDefinitionWriteRequest{
				Name: "Missing Prompt", AgentID: agent.ID,
			},
			want: "promptTemplate",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.SaveWorkflow(ctx, "", tc.payload); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("SaveWorkflow err = %v, want containing %q", err, tc.want)
			}
		})
	}

	if _, err := service.GetWorkflow(ctx, "missing-workflow"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetWorkflow missing err = %v, want not found", err)
	}
	if _, err := service.DeleteWorkflow(ctx, "missing-workflow"); err == nil {
		t.Fatal("DeleteWorkflow missing succeeded, want error")
	}
	if _, err := service.ListWorkflowTriggers(ctx, "missing-workflow"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ListWorkflowTriggers missing err = %v, want not found", err)
	}
	if _, err := service.DeleteWorkflowTrigger(ctx, "missing-workflow", "missing-trigger"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("DeleteWorkflowTrigger missing err = %v, want not found", err)
	}

	_, disabledWorkflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-disabled-run", jfadk.WorkflowStatusDisabled)
	if _, err := service.RunWorkflow(ctx, disabledWorkflow.ID, nil); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("RunWorkflow disabled err = %v, want disabled", err)
	}
}

func TestWorkflowInvocationFailureAndBackgroundPaths(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-invoke-failures", jfadk.WorkflowStatusEnabled)

	invalidPrompt, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: "Invalid Prompt Workflow", Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PermissionMode: jfadk.PermissionModeApproval,
		PromptTemplate: "run {{",
	})
	if err != nil {
		t.Fatalf("SaveWorkflow invalid prompt template: %v", err)
	}
	result, err := service.RunWorkflow(ctx, invalidPrompt.ID, nil)
	if err == nil || result.Log.Status != jfadk.WorkflowTriggerLogStatusFailed || result.Log.Result == nil {
		t.Fatalf("RunWorkflow invalid prompt result=%+v err=%v, want failed log with result", result, err)
	}
	if len(result.Log.NodeRuns) < 2 || result.Log.NodeRuns[1].Status != jfadk.WorkflowTriggerLogStatusFailed {
		t.Fatalf("invalid prompt node runs = %+v, want failed Start node", result.Log.NodeRuns)
	}

	invalidObjective, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: "Invalid Objective Workflow", Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PermissionMode: jfadk.PermissionModeApproval,
		PromptTemplate: "run {{ .symbol }}", ObjectiveTemplate: "objective {{",
		DefaultInputs: map[string]any{"symbol": "US.AAPL"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflow invalid objective template: %v", err)
	}
	result, err = service.RunWorkflow(ctx, invalidObjective.ID, nil)
	if err == nil || result.Log.Status != jfadk.WorkflowTriggerLogStatusFailed || !strings.Contains(result.Log.Error, "template") {
		t.Fatalf("RunWorkflow invalid objective result=%+v err=%v, want template failure", result, err)
	}

	brokenAgent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "workflow-broken-provider-agent", Name: "Broken Provider Agent",
		Status: jfadk.AgentStatusEnabled, ProviderID: "missing-provider", Model: "test-model",
	})
	if err != nil {
		t.Fatalf("SaveAgent broken provider: %v", err)
	}
	brokenWorkflow, err := service.SaveWorkflow(ctx, "", jfadk.WorkflowDefinitionWriteRequest{
		ID: "workflow-broken-provider", Name: "Broken Provider Workflow", Status: jfadk.WorkflowStatusEnabled,
		AgentID: brokenAgent.ID, WorkMode: jfadk.WorkModeChat, PromptTemplate: "run",
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow broken provider: %v", err)
	}
	result, err = service.RunWorkflow(ctx, brokenWorkflow.ID, nil)
	if err == nil || result.Log.SessionID == "" || result.Log.Status != jfadk.WorkflowTriggerLogStatusFailed {
		t.Fatalf("RunWorkflow broken provider result=%+v err=%v, want chat failure after session creation", result, err)
	}

	agent, workflow = saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-background", jfadk.WorkflowStatusEnabled)
	workflow, err = service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: workflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PermissionMode: jfadk.PermissionModeApproval,
		PromptTemplate: "event {{ .event.price }} for {{ .trigger.title }}",
		CanvasGraph:    workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow background: %v", err)
	}
	triggerResult, err := service.SaveWorkflowTrigger(ctx, workflow.ID, "", jfadk.WorkflowTriggerWriteRequest{
		ID: "workflow-background-event", Type: jfadk.WorkflowTriggerTypeEvent,
		Title: "Background Event", Status: jfadk.WorkflowTriggerStatusEnabled,
		Config: map[string]any{"eventType": "unit.event"},
	})
	if err != nil {
		t.Fatalf("SaveWorkflowTrigger background: %v", err)
	}
	service.invokeWorkflowBackground(workflow, triggerResult.Trigger, map[string]any{"type": "unit.event", "price": 101})
	logs := workflowLogsForTrigger(t, runtime, triggerResult.Trigger.ID, jfadk.WorkflowTriggerLogStatusSucceeded)
	if len(logs) != 1 {
		t.Fatalf("background logs = %+v, want one succeeded log with matched event", logs)
	}
	price, _ := anyFloat(logs[0].MatchedEvent["price"])
	if price != 101 {
		t.Fatalf("background log matched event = %+v, want price 101", logs[0].MatchedEvent)
	}
}

func TestWorkflowInvocationPreflightAndStaleResourcePaths(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	agent, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-preflight", jfadk.WorkflowStatusEnabled)

	if _, err := (&Service{}).invokeWorkflow(ctx, workflow, nil, jfadk.WorkflowTriggerTypeManual, nil, nil); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("invokeWorkflow unavailable service err = %v, want unavailable", err)
	}
	disabledTrigger := jfadk.WorkflowTrigger{
		ID:         "workflow-preflight-disabled-trigger",
		WorkflowID: workflow.ID,
		Type:       jfadk.WorkflowTriggerTypeEvent,
		Status:     jfadk.WorkflowTriggerStatusDisabled,
	}
	if _, err := service.invokeWorkflow(ctx, workflow, &disabledTrigger, disabledTrigger.Type, nil, nil); err == nil || !strings.Contains(err.Error(), "trigger is disabled") {
		t.Fatalf("invokeWorkflow disabled trigger err = %v, want disabled", err)
	}

	emptyPrompt := workflow
	emptyPrompt.ID = "workflow-preflight-empty-prompt"
	emptyPrompt.PromptTemplate = "   "
	result, err := service.invokeWorkflow(ctx, emptyPrompt, nil, jfadk.WorkflowTriggerTypeManual, nil, nil)
	if err == nil || result.Log.Status != jfadk.WorkflowTriggerLogStatusFailed || !strings.Contains(err.Error(), "empty message") {
		t.Fatalf("invokeWorkflow empty prompt result=%+v err=%v", result, err)
	}

	staleAgent := workflow
	staleAgent.ID = "workflow-preflight-stale-agent"
	staleAgent.AgentID = "missing-agent"
	staleAgent.PromptTemplate = "run"
	result, err = service.invokeWorkflow(ctx, staleAgent, nil, jfadk.WorkflowTriggerTypeManual, nil, nil)
	if err == nil || result.Log.Status != jfadk.WorkflowTriggerLogStatusFailed || !strings.Contains(err.Error(), "agent") {
		t.Fatalf("invokeWorkflow stale agent result=%+v err=%v", result, err)
	}

	withObjective, err := service.SaveWorkflow(ctx, workflow.ID, jfadk.WorkflowDefinitionWriteRequest{
		Name: workflow.Name, Status: jfadk.WorkflowStatusEnabled, AgentID: agent.ID,
		WorkMode: jfadk.WorkModeChat, PromptTemplate: "run {{ .symbol }}",
		ObjectiveTemplate: "review {{ .symbol }}", DefaultInputs: map[string]any{"symbol": "US.AAPL"},
		CanvasGraph: workflowTestCanvasGraph(),
	})
	if err != nil {
		t.Fatalf("SaveWorkflow objective: %v", err)
	}
	result, err = service.RunWorkflow(ctx, withObjective.ID, nil)
	if err != nil || result.Log.Status != jfadk.WorkflowTriggerLogStatusSucceeded {
		t.Fatalf("RunWorkflow objective result=%+v err=%v", result, err)
	}
	if result.Log.NodeRuns[0].Outputs["objective"] != "review US.AAPL" {
		t.Fatalf("objective node output = %+v", result.Log.NodeRuns[0].Outputs)
	}
}

func TestWorkflowInvocationPersistsOrReturnsEachLogWriteFailure(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	assistantServiceProvider(t, runtime)
	ctx := t.Context()
	_, workflow := saveWorkflowTestAgentAndDefinition(t, runtime, service, "workflow-log-write-failure", jfadk.WorkflowStatusEnabled)
	trigger := jfadk.WorkflowTrigger{
		ID:         "workflow-log-write-failure-trigger",
		WorkflowID: workflow.ID,
		Type:       jfadk.WorkflowTriggerTypeEvent,
		Title:      "Failure injection event",
		Status:     jfadk.WorkflowTriggerStatusEnabled,
	}

	listFailure := &workflowInvocationFaultStore{base: runtime.Store(), listErr: errors.New("list active logs failed")}
	if _, err := service.invokeWorkflowWithStore(ctx, listFailure, workflow, &trigger, trigger.Type, nil, nil); !errors.Is(err, listFailure.listErr) {
		t.Fatalf("active log list err = %v, want %v", err, listFailure.listErr)
	}

	activeSaveFailure := &workflowInvocationFaultStore{
		base:          runtime.Store(),
		activeLogsSet: true,
		activeLogs: []jfadk.WorkflowTriggerLog{{
			ID: "active-log", TriggerID: trigger.ID, Status: jfadk.WorkflowTriggerLogStatusRunning,
		}},
		failSaveAt: 1,
	}
	if _, err := service.invokeWorkflowWithStore(ctx, activeSaveFailure, workflow, &trigger, trigger.Type, nil, nil); !errors.Is(err, errWorkflowLogWriteInjected) {
		t.Fatalf("active skip save err = %v, want injected write failure", err)
	}

	for _, failSaveAt := range []int{1, 2, 3} {
		faultStore := &workflowInvocationFaultStore{base: runtime.Store(), failSaveAt: failSaveAt}
		_, err := service.invokeWorkflowWithStore(ctx, faultStore, workflow, nil, jfadk.WorkflowTriggerTypeManual, nil, nil)
		if !errors.Is(err, errWorkflowLogWriteInjected) {
			t.Fatalf("save call %d err = %v, want injected write failure", failSaveAt, err)
		}
		if faultStore.saveCalls != failSaveAt {
			t.Fatalf("save call count = %d, want %d", faultStore.saveCalls, failSaveAt)
		}
	}
}

func TestWorkflowActiveRunGuardHandlesStoredRunStates(t *testing.T) {
	runtime, _, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	runReadFailure := &workflowInvocationFaultStore{
		base: runtime.Store(), activeLogsSet: true,
		activeLogs: []jfadk.WorkflowTriggerLog{{ID: "run-read-failure", RunID: "run-1"}},
		runErr:     errors.New("run read failed"),
	}
	if active, err := workflowTriggerHasActiveRun(ctx, runReadFailure, "trigger-1"); active || !errors.Is(err, runReadFailure.runErr) {
		t.Fatalf("run read failure active=%v err=%v", active, err)
	}

	missingRun := &workflowInvocationFaultStore{
		base: runtime.Store(), activeLogsSet: true,
		activeLogs: []jfadk.WorkflowTriggerLog{{ID: "missing-run", RunID: "missing"}},
		runsSet:    true,
		runs:       map[string]jfadk.Run{},
	}
	if active, err := workflowTriggerHasActiveRun(ctx, missingRun, "trigger-1"); err != nil || active || missingRun.saveCalls != 1 {
		t.Fatalf("missing run active=%v err=%v saveCalls=%d", active, err, missingRun.saveCalls)
	}

	pendingRun := &workflowInvocationFaultStore{
		base: runtime.Store(), activeLogsSet: true,
		activeLogs: []jfadk.WorkflowTriggerLog{{ID: "pending-run", RunID: "run-pending"}},
		runsSet:    true,
		runs: map[string]jfadk.Run{
			"run-pending": {ID: "run-pending", Status: jfadk.RunStatusPending},
		},
	}
	if active, err := workflowTriggerHasActiveRun(ctx, pendingRun, "trigger-1"); err != nil || !active || pendingRun.saveCalls != 0 {
		t.Fatalf("pending run active=%v err=%v saveCalls=%d", active, err, pendingRun.saveCalls)
	}

	failedRun := &workflowInvocationFaultStore{
		base: runtime.Store(), activeLogsSet: true,
		activeLogs: []jfadk.WorkflowTriggerLog{{ID: "failed-run", RunID: "run-failed"}},
		runsSet:    true,
		runs: map[string]jfadk.Run{
			"run-failed": {ID: "run-failed", Status: jfadk.RunStatusFailed, FailureReason: "provider down"},
		},
	}
	if active, err := workflowTriggerHasActiveRun(ctx, failedRun, "trigger-1"); err != nil || active {
		t.Fatalf("failed run active=%v err=%v", active, err)
	}
	if len(failedRun.savedLogs) != 1 || failedRun.savedLogs[0].Status != jfadk.WorkflowTriggerLogStatusFailed || failedRun.savedLogs[0].Error != "provider down" {
		t.Fatalf("failed run saved logs = %+v", failedRun.savedLogs)
	}

	finishFailure := &workflowInvocationFaultStore{base: runtime.Store(), failSaveAt: 1}
	original := jfadk.WorkflowTriggerLog{ID: "finish-failure", Status: jfadk.WorkflowTriggerLogStatusRunning}
	finished := finishWorkflowLog(ctx, finishFailure, original, jfadk.WorkflowTriggerLogStatusFailed, "write failed")
	if finished.Status != jfadk.WorkflowTriggerLogStatusFailed || finished.Error != "write failed" || finished.FinishedAt == "" {
		t.Fatalf("finishWorkflowLog save failure = %+v", finished)
	}
}

func TestWorkflowEventMatchingCooldownAndUtilityBoundaries(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	event := jfadk.WorkflowEvent{
		ID:       "event-1",
		Type:     "system.notification",
		Source:   "notification",
		EntityID: "account-1",
		At:       now.Format(time.RFC3339Nano),
		Payload:  map[string]any{"category": "broker.connection", "level": "warn", "source": "payload-source"},
	}
	config := map[string]any{
		"source":      "notification",
		"eventType":   "system.notification",
		"entityId":    "account-1",
		"category":    "broker.connection",
		"level":       "warn",
		"cooldownSec": json.Number("600"),
	}
	if !workflowEventMatches(config, event) {
		t.Fatal("workflowEventMatches = false, want true")
	}
	for key, value := range map[string]any{
		"source":    "other",
		"eventType": "other",
		"entityId":  "other",
		"category":  "other",
		"level":     "error",
	} {
		next := cloneMap(config)
		next[key] = value
		if workflowEventMatches(next, event) {
			t.Fatalf("workflowEventMatches with mismatched %s = true, want false", key)
		}
	}

	trigger := jfadk.WorkflowTrigger{Config: cloneMap(config)}
	if !eventTriggerCooldownAllows(&trigger, now) {
		t.Fatal("first cooldownAllows = false, want true")
	}
	if eventTriggerCooldownAllows(&trigger, now.Add(time.Second)) {
		t.Fatal("second cooldownAllows during cooldown = true, want false")
	}
	if !eventTriggerCooldownAllows(&trigger, now.Add(601*time.Second)) {
		t.Fatal("cooldownAllows after cooldown = false, want true")
	}
	if eventTriggerCooldownAllows(nil, now) {
		t.Fatal("eventTriggerCooldownAllows nil = true, want false")
	}

	asMap := eventAsMap(event)
	if asMap["id"] != "event-1" || asMap["source"] != "notification" || asMap["category"] != "broker.connection" {
		t.Fatalf("eventAsMap = %+v", asMap)
	}
	if got := mapString(nil, "level"); got != "" {
		t.Fatalf("mapString nil = %q, want empty", got)
	}
	if got := errorString(nil); got != "" {
		t.Fatalf("errorString nil = %q, want empty", got)
	}
}

func TestWorkflowMarketThresholdAndConfigHelpersCoverEdges(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	trigger := jfadk.WorkflowTrigger{Config: map[string]any{
		"instrumentIds": "US.AAPL, hk.00700, US.AAPL",
		"snapshotPath":  "price",
		"value":         json.Number("100"),
		"edge":          "cross_down",
		"cooldownSec":   "0",
	}}
	matches, changed := evaluateMarketThresholdTrigger(trigger, []map[string]any{
		{"payload": map[string]any{"instrument": map[string]any{"instrumentId": "US.AAPL"}, "price": 101}},
		{"payload": map[string]any{"instrumentId": "US.AAPL", "price": 99}},
		{"entityId": "US.MSFT", "price": 50},
	}, now)
	if !changed || len(matches) != 1 {
		t.Fatalf("cross-down matches=%+v changed=%v, want one match with changed state", matches, changed)
	}
	if threshold, _ := matches[0]["threshold"].(map[string]any); threshold["edge"] != "cross_down" || threshold["current"] != float64(99) {
		t.Fatalf("cross-down threshold payload = %+v", matches[0]["threshold"])
	}

	if thresholdFired("below", "<=", 0, false, 100, 100) != true {
		t.Fatal("below <= threshold did not fire")
	}
	if thresholdFired("above", ">=", 0, false, 100, 100) != true {
		t.Fatal("above >= threshold did not fire")
	}
	if compareThreshold("<", 99, 100) != true || compareThreshold(">", 99, 100) != false {
		t.Fatal("compareThreshold produced unexpected comparison result")
	}
	if _, ok := numericAtPath(map[string]any{"snapshot": "bad"}, "snapshot.price"); ok {
		t.Fatal("numericAtPath through scalar = true, want false")
	}
	if got := eventInstrumentID(map[string]any{"payload": map[string]any{"entityId": "hk.00700"}}); got != "HK.00700" {
		t.Fatalf("eventInstrumentID nested payload = %q", got)
	}
	if got := configStringSlice(map[string]any{"ids": []any{" US.AAPL ", 700, ""}}, "ids"); strings.Join(got, ",") != "US.AAPL,700" {
		t.Fatalf("configStringSlice []any = %+v", got)
	}
	if got := configStringSlice(map[string]any{"ids": []string{"a", "a", " "}}, "ids"); strings.Join(got, ",") != "a" {
		t.Fatalf("configStringSlice []string = %+v", got)
	}
	if got := configString(map[string]any{"n": json.Number("42")}, "n"); got != "42" {
		t.Fatalf("configString Stringer = %q, want 42", got)
	}
	for _, value := range []any{float32(1.5), int64(2), int32(3), "4.5"} {
		if _, ok := anyFloat(value); !ok {
			t.Fatalf("anyFloat(%T) = false, want true", value)
		}
	}
	if _, ok := anyFloat("bad"); ok {
		t.Fatal("anyFloat bad string = true, want false")
	}
}

func TestWorkflowCoreHelpersAndUnavailableService(t *testing.T) {
	service := &Service{}
	if _, err := service.workflowStore(); err == nil {
		t.Fatal("workflowStore without runtime succeeded, want unavailable")
	}
	if _, err := service.ListWorkflows(t.Context(), WorkflowQuery{}); err == nil {
		t.Fatal("ListWorkflows without runtime succeeded, want unavailable")
	}
	if err := service.EnsureBuiltinWorkflowTemplates(t.Context()); err == nil {
		t.Fatal("EnsureBuiltinWorkflowTemplates without runtime succeeded, want unavailable")
	}
	service.StartWorkflowScheduler(t.Context())

	if normalizeWorkflowStatus("", jfadk.WorkflowStatusDisabled) != jfadk.WorkflowStatusDisabled {
		t.Fatal("normalizeWorkflowStatus fallback disabled failed")
	}
	if normalizeTriggerStatus("", jfadk.WorkflowTriggerStatusError) != jfadk.WorkflowTriggerStatusError {
		t.Fatal("normalizeTriggerStatus fallback error failed")
	}
	if normalizeWorkflowWorkMode("", jfadk.WorkModeChat) != jfadk.WorkModeChat {
		t.Fatal("normalizeWorkflowWorkMode fallback chat failed")
	}
	if normalizeWorkflowPermissionMode("bad", "") != jfadk.PermissionModeApproval {
		t.Fatal("normalizeWorkflowPermissionMode bad value did not fall back to approval")
	}
	if normalizeTriggerType("", jfadk.WorkflowTriggerTypeWebhook) != jfadk.WorkflowTriggerTypeWebhook {
		t.Fatal("normalizeTriggerType fallback webhook failed")
	}
	if defaultTriggerTitle(jfadk.WorkflowTriggerTypeEvent) != "事件触发" || defaultTriggerTitle(jfadk.WorkflowTriggerTypeMarketThreshold) != "行情阈值" {
		t.Fatal("defaultTriggerTitle event/market mismatch")
	}
	if workflowSessionTitle("", time.Date(2026, 7, 1, 1, 2, 0, 0, time.UTC)) != "ADK 工作流 - 2026-07-01 01:02" {
		t.Fatal("workflowSessionTitle default mismatch")
	}
	if newSanitizedTriggerPtr(nil) != nil {
		t.Fatal("newSanitizedTriggerPtr nil returned non-nil")
	}
	if triggerID(nil) != "" {
		t.Fatal("triggerID nil returned non-empty")
	}
	if result := workflowResultFromError(nil); result != nil {
		t.Fatalf("workflowResultFromError nil = %+v, want nil", result)
	}
	if prompt := dailyStockReviewPrompt(); !strings.Contains(prompt, "每日股票盘点") || !strings.Contains(prompt, "tasks.create") {
		t.Fatalf("dailyStockReviewPrompt = %q", prompt)
	}
}
