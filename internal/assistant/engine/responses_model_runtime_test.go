package adk

import (
	"testing"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
)

func TestProviderAPIProtocolDefaultsPersistsAndSelectsModel(t *testing.T) {
	runtime := newTestRuntime(t)
	defaultProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{ID: "chat", BaseURL: "https://example.test/v1", APIKey: "secret", Enabled: true})
	if defaultProvider.APIProtocol != ProviderAPIProtocolChatCompletions {
		t.Fatalf("default API protocol = %q", defaultProvider.APIProtocol)
	}
	responsesProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{ID: "responses", BaseURL: "https://example.test/v1", APIKey: "secret", APIProtocol: ProviderAPIProtocolResponses, Enabled: true})
	persisted, ok, err := runtime.Store().Provider(t.Context(), responsesProvider.ID)
	if err != nil || !ok || persisted.APIProtocol != ProviderAPIProtocolResponses {
		t.Fatalf("persisted responses provider = %+v, %v, %v", persisted, ok, err)
	}
	updatedProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: responsesProvider.ID, BaseURL: responsesProvider.BaseURL, Model: "updated-model", Enabled: true,
	})
	if updatedProvider.APIProtocol != ProviderAPIProtocolResponses {
		t.Fatalf("updated provider API protocol = %q, want responses", updatedProvider.APIProtocol)
	}
	llm, err := runtime.GoogleADKModelForAgent(t.Context(), Agent{ProviderID: responsesProvider.ID})
	if err != nil {
		t.Fatalf("googleADKModelForAgent: %v", err)
	}
	if _, ok := llm.(*providers.ResponsesToolNameModel); !ok {
		t.Fatalf("Responses provider model = %T", llm)
	}
	if _, err := runtime.Store().SaveProvider(t.Context(), ProviderWriteRequest{ID: "invalid", APIProtocol: "legacy", Enabled: true}); err == nil {
		t.Fatal("SaveProvider invalid API protocol succeeded")
	}
}
