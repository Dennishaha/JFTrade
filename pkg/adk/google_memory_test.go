package adk

import (
	"context"
	"testing"

	adkmemory "google.golang.org/adk/v2/memory"
)

func TestGoogleADKMemoryServiceSearchesJFTradeMemory(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{ID: "memory-agent", Name: "Memory Agent", ProviderID: testProviderID, Status: AgentStatusEnabled})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "workspace", Key: "Market", Value: "Prefer HK equities"}); err != nil {
		t.Fatalf("SaveMemory workspace: %v", err)
	}
	if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "agent", AgentID: agent.ID, Key: "Risk", Value: "Keep position size small"}); err != nil {
		t.Fatalf("SaveMemory agent: %v", err)
	}
	other, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{ID: "other-agent", Name: "Other Agent", ProviderID: testProviderID, Status: AgentStatusEnabled})
	if err != nil {
		t.Fatalf("SaveAgent other: %v", err)
	}
	if _, err := runtime.Store().SaveMemory(ctx, MemoryWriteRequest{Scope: "agent", AgentID: other.ID, Key: "Private", Value: "unrelated"}); err != nil {
		t.Fatalf("SaveMemory other: %v", err)
	}

	response, err := runtime.memoryService.SearchMemory(ctx, &adkmemory.SearchRequest{
		AppName: googleADKAppName(agent.ID),
		UserID:  googleADKUserID,
		Query:   "risk",
	})
	if err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
	if response == nil || len(response.Memories) != 1 {
		t.Fatalf("SearchMemory memories = %+v, want one agent memory", response)
		return
	}
	if response.Memories[0].ID == "" || response.Memories[0].Content == nil {
		t.Fatalf("memory entry = %+v, want ID and content", response.Memories[0])
	}
	if response.Memories[0].CustomMetadata["scope"] != "agent" || response.Memories[0].CustomMetadata["agentId"] != agent.ID {
		t.Fatalf("memory metadata = %+v", response.Memories[0].CustomMetadata)
	}

	response, err = runtime.memoryService.SearchMemory(ctx, &adkmemory.SearchRequest{
		AppName: googleADKAppName(agent.ID),
		UserID:  googleADKUserID,
		Query:   "HK",
	})
	if err != nil {
		t.Fatalf("SearchMemory workspace: %v", err)
	}
	if response == nil || len(response.Memories) != 1 || response.Memories[0].CustomMetadata["scope"] != "workspace" {
		t.Fatalf("workspace memories = %+v", response)
	}

	response, err = runtime.memoryService.SearchMemory(ctx, &adkmemory.SearchRequest{
		AppName: "jftrade-default",
		UserID:  googleADKUserID,
		Query:   "",
	})
	if err != nil {
		t.Fatalf("SearchMemory default app: %v", err)
	}
	for _, memory := range response.Memories {
		if memory.CustomMetadata["scope"] != "workspace" {
			t.Fatalf("default app memory leaked non-workspace entry: %+v", memory)
		}
	}
}

func TestGoogleADKAgentIDFromAppName(t *testing.T) {
	if got := googleADKAgentIDFromAppName(googleADKAppName("Memory Agent")); got != "memory-agent" {
		t.Fatalf("roundtrip agent id = %q", got)
	}
	if got := googleADKAgentIDFromAppName("jftrade-default"); got != "" {
		t.Fatalf("default app agent id = %q, want empty", got)
	}
	if got := googleADKAgentIDFromAppName("custom-app"); got != "custom-app" {
		t.Fatalf("custom app agent id = %q", got)
	}
}
