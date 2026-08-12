package adk

import "testing"

const (
	ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh = ReasoningEffort("low"), ReasoningEffort("medium"), ReasoningEffort("high")
	ReasoningEffortXHigh, ReasoningEffortMax                       = ReasoningEffort("xhigh"), ReasoningEffort("max")
)

func TestReasoningEffortOverridePriority(t *testing.T) {
	agent := Agent{ReasoningEffort: ReasoningEffortMedium}
	if got := applyChatModelOverride(agent, ChatRequest{}).ReasoningEffort; got != ReasoningEffortMedium {
		t.Fatalf("inherited reasoning effort = %q", got)
	}
	if got := applyChatModelOverride(agent, ChatRequest{ReasoningEffortOverride: ReasoningEffortMax}).ReasoningEffort; got != ReasoningEffortMax {
		t.Fatalf("session reasoning effort = %q", got)
	}
	if _, err := validateChatOverrides(ChatRequest{ReasoningEffortOverride: ReasoningEffort("extreme")}); err == nil {
		t.Fatal("invalid chat reasoning effort was accepted")
	}
}

func TestReasoningEffortResumeUsesRunSnapshot(t *testing.T) {
	runtime := newTestRuntime(t)
	provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: testProviderID, Enabled: true,
		ReasoningConfig: &ProviderReasoningConfig{RequestField: "vendor.current", Mappings: []ProviderReasoningMapping{
			{Effort: ReasoningEffortLow, Value: "LOW_V2"},
		}},
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "reasoning-resume", Name: "Reasoning Resume", ProviderID: provider.ID,
		ReasoningEffort: ReasoningEffortLow, Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "reasoning snapshot")
	run := Run{
		ID: "reasoning-snapshot-run", SessionID: session.ID, AgentID: agent.ID,
		ProviderID: provider.ID, Model: "snapshot-model", ReasoningEffort: ReasoningEffortMax,
		ReasoningEffortField: "reasoning_effort", ReasoningEffortValue: "MAX_V1",
		Status: RunStatusPending, WorkMode: WorkModeChat,
	}
	execution, err := runtime.newResumedGoogleADKExecution(t.Context(), run)
	if err != nil {
		t.Fatalf("newResumedGoogleADKExecution: %v", err)
	}
	if execution.agent.ReasoningEffort != ReasoningEffortMax || execution.agent.ReasoningEffortField != "reasoning_effort" || execution.agent.ReasoningEffortValue != "MAX_V1" {
		t.Fatalf("resumed snapshot = %+v", execution.agent)
	}
	_, resumedAgent, err := runtime.workflowResumeContext(t.Context(), run)
	if err != nil || resumedAgent.ReasoningEffortValue != "MAX_V1" {
		t.Fatalf("workflow resumed snapshot = %+v err=%v", resumedAgent, err)
	}
}
