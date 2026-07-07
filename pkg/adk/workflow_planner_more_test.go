package adk

import (
	"context"
	"errors"
	"strings"
	"testing"

	adksession "google.golang.org/adk/v2/session"
)

func TestWorkflowPlannerAdditionalRuntimeBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("planner uses in-memory session service fallback when runtime session service is nil", func(t *testing.T) {
		runtime := newTestRuntime(t)
		providerID := saveGoalWorkflowProvider(t, runtime, "workflow-planner-fallback-provider", testProviderMessage)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-planner-fallback-agent", Name: "Workflow Planner Fallback", ProviderID: providerID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow planner fallback")
		runtime.sessionService = nil

		steps, warnings, err := runtime.planWorkflowWithADK(ctx, agent, session, WorkModeLoop, "整理一个执行清单", "整理一个执行清单", RunOptions{})
		if err != nil {
			t.Fatalf("planWorkflowWithADK fallback: %v", err)
		}
		if len(steps) == 0 {
			t.Fatalf("planner fallback steps = %+v warnings=%+v, want at least one step", steps, warnings)
		}
	})

	t.Run("planner surfaces session lookup and creation errors before execution", func(t *testing.T) {
		getRuntime := newTestRuntime(t)
		providerID := saveGoalWorkflowProvider(t, getRuntime, "workflow-planner-get-error-provider", testProviderMessage)
		agent := mustSaveAgent(t, getRuntime, AgentWriteRequest{
			ID: "workflow-planner-get-error-agent", Name: "Workflow Planner Get Error", ProviderID: providerID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, getRuntime, agent.ID, "workflow planner get error")
		getRuntime.sessionService = getErrorADKSessionService{Service: adksession.InMemoryService(), err: errors.New("planner session get failed")}
		if _, _, err := getRuntime.planWorkflowWithADK(ctx, agent, session, WorkModeLoop, "整理一个执行清单", "整理一个执行清单", RunOptions{}); err == nil || !strings.Contains(err.Error(), "planner session get failed") {
			t.Fatalf("planWorkflowWithADK get err = %v, want planner session get failed", err)
		}

		createRuntime := newTestRuntime(t)
		providerID = saveGoalWorkflowProvider(t, createRuntime, "workflow-planner-create-error-provider", testProviderMessage)
		agent = mustSaveAgent(t, createRuntime, AgentWriteRequest{
			ID: "workflow-planner-create-error-agent", Name: "Workflow Planner Create Error", ProviderID: providerID,
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session = mustCreateSession(t, createRuntime, agent.ID, "workflow planner create error")
		createRuntime.sessionService = notFoundCreateErrorSessionService{Service: adksession.InMemoryService(), err: errors.New("planner session create failed")}
		if _, _, err := createRuntime.planWorkflowWithADK(ctx, agent, session, WorkModeLoop, "整理一个执行清单", "整理一个执行清单", RunOptions{}); err == nil || !strings.Contains(err.Error(), "planner session create failed") {
			t.Fatalf("planWorkflowWithADK create err = %v, want planner session create failed", err)
		}
	})

	t.Run("planner surfaces model run errors from the provider", func(t *testing.T) {
		runtime := newTestRuntime(t)
		mustSaveProvider(t, runtime, ProviderWriteRequest{
			ID: "workflow-planner-run-error-provider", DisplayName: "Workflow Planner Run Error",
			BaseURL: "http://127.0.0.1:1/v1", Model: "test-model", APIKey: "sk-test", Enabled: true,
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-planner-run-error-agent", Name: "Workflow Planner Run Error", ProviderID: "workflow-planner-run-error-provider",
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow planner run error")
		if _, _, err := runtime.planWorkflowWithADK(ctx, agent, session, WorkModeLoop, "整理一个执行清单", "整理一个执行清单", RunOptions{}); err == nil {
			t.Fatal("planWorkflowWithADK accepted provider execution failure")
		}
	})
}
