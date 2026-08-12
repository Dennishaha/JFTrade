package assistant

import (
	"errors"
	"testing"

	adk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
)

func TestPrimaryBuiltinAgentAllowsOnlyProviderReasoningSettings(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	provider, err := runtime.Store().SaveProvider(t.Context(), adk.ProviderWriteRequest{
		ID: "builtin-reasoning-provider", DisplayName: "Reasoning Provider", APIKey: "sk-test", Enabled: true,
		ReasoningConfig: &adk.ProviderReasoningConfig{RequestField: "reasoning_effort", Mappings: []adk.ProviderReasoningMapping{
			{Effort: "max", Value: "max"},
		}},
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	current, ok, err := runtime.Store().Agent(t.Context(), adk.DefaultBuiltinAgentID)
	if err != nil || !ok {
		t.Fatalf("Agent default ok=%v err=%v", ok, err)
	}

	request := primaryBuiltinAgentWriteRequest(current)
	request.ProviderID = provider.ID
	request.Model = "reasoning-model"
	request.ReasoningEffort = "max"
	updated, err := service.SaveAgent(t.Context(), request)
	if err != nil {
		t.Fatalf("SaveAgent reasoning settings: %v", err)
	}
	if updated.ProviderID != provider.ID || updated.Model != "reasoning-model" || updated.ReasoningEffort != "max" {
		t.Fatalf("updated reasoning settings = %+v", updated)
	}

	request.Instruction = "replace protected instruction"
	if _, err := service.SaveAgent(t.Context(), request); !errors.Is(err, adk.ErrBuiltinAgentProtected) {
		t.Fatalf("protected instruction update error = %v", err)
	}
	request = primaryBuiltinAgentWriteRequest(updated)
	request.Status = adk.AgentStatusDisabled
	if _, err := service.SaveAgent(t.Context(), request); !errors.Is(err, adk.ErrBuiltinAgentProtected) {
		t.Fatalf("protected status update error = %v", err)
	}
}

func primaryBuiltinAgentWriteRequest(agent adk.Agent) adk.AgentWriteRequest {
	return adk.AgentWriteRequest{
		ID: agent.ID, Name: agent.Name, Instruction: agent.Instruction,
		ProviderID: agent.ProviderID, Model: agent.Model, ReasoningEffort: agent.ReasoningEffort,
		Tools: append([]string(nil), agent.Tools...), Skills: append([]string(nil), agent.Skills...),
		PermissionMode: agent.PermissionMode, MemoryEnabled: agent.MemoryEnabled,
		RecentUserWindow: agent.RecentUserWindow, WorkMode: agent.WorkMode,
		LoopMaxIterations: agent.LoopMaxIterations, Status: agent.Status,
	}
}
