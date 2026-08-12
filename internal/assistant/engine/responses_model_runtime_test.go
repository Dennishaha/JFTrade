package adk

import (
	"testing"

	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
)

func TestProviderAlwaysSelectsResponsesModel(t *testing.T) {
	runtime := newTestRuntime(t)
	responsesProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{ID: "responses", BaseURL: "https://example.test/v1", APIKey: "secret", Enabled: true})
	persisted, ok, err := runtime.Store().Provider(t.Context(), responsesProvider.ID)
	if err != nil || !ok {
		t.Fatalf("persisted responses provider = %+v, %v, %v", persisted, ok, err)
	}
	updatedProvider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: responsesProvider.ID, BaseURL: responsesProvider.BaseURL, Model: "updated-model", Enabled: true,
	})
	if updatedProvider.Model != "updated-model" {
		t.Fatalf("updated provider model = %q", updatedProvider.Model)
	}
	llm, err := runtime.GoogleADKModelForAgent(t.Context(), Agent{ProviderID: updatedProvider.ID})
	if err != nil {
		t.Fatalf("googleADKModelForAgent: %v", err)
	}
	if _, ok := llm.(*providers.ResponsesToolNameModel); !ok {
		t.Fatalf("Responses provider model = %T", llm)
	}
}
