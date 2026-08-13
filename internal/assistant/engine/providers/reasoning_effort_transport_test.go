package providers

import (
	"encoding/json"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"net/http"
	"net/http/httptest"
	"testing"

	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func TestResponsesReasoningEffortRequestField(t *testing.T) {
	for _, test := range []struct {
		name       string
		effort     assistantmodel.ReasoningEffort
		wantEffort string
	}{
		{name: "model default"},
		{name: "low", effort: assistantmodel.ReasoningEffortLow, wantEffort: "low"},
		{name: "medium", effort: assistantmodel.ReasoningEffortMedium, wantEffort: "medium"},
		{name: "high", effort: assistantmodel.ReasoningEffortHigh, wantEffort: "high"},
		{name: "xhigh", effort: assistantmodel.ReasoningEffortXHigh, wantEffort: "xhigh"},
		{name: "max", effort: assistantmodel.ReasoningEffortMax, wantEffort: "max"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/v1/responses" {
					t.Errorf("request path = %q", request.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode Responses request: %v", err)
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				reasoning, hasReasoning := body["reasoning"].(map[string]any)
				if hasReasoning != (test.wantEffort != "") {
					t.Errorf("reasoning field present = %v, want %v: %#v", hasReasoning, test.wantEffort != "", body)
				}
				if got, _ := reasoning["effort"].(string); got != test.wantEffort {
					t.Errorf("reasoning.effort = %q, want %q", got, test.wantEffort)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"resp_reasoning","model":"test-model","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}]}`))
			}))
			defer server.Close()

			llm, err := NewOpenAIResponsesADKModel(t.Context(), assistantmodel.Provider{
				BaseURL: server.URL + "/v1", Model: "test-model",
				ReasoningConfig: identityReasoningConfig("reasoning.effort"),
			}, "secret", "", test.effort)
			if err != nil {
				t.Fatalf("NewOpenAIResponsesADKModel: %v", err)
			}
			if response := singleResponse(t, llm, textRequest()); response == nil {
				t.Fatal("Responses request returned nil response")
			}
		})
	}
}

func TestResponsesCustomReasoningMappingInjectsNestedField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode Responses custom request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		providerObject, ok := body["provider"].(map[string]any)
		if !ok {
			t.Errorf("provider object missing: %#v", body)
		}
		reasoningObject, ok := providerObject["reasoning"].(map[string]any)
		if !ok || reasoningObject["level"] != "BALANCED" {
			t.Errorf("custom Responses mapping = %#v, want BALANCED", providerObject)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_custom_reasoning","model":"test-model","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}]}`))
	}))
	defer server.Close()

	llm, err := NewOpenAIResponsesADKModel(t.Context(), assistantmodel.Provider{
		BaseURL: server.URL + "/v1", Model: "test-model",
		ReasoningConfig: assistantmodel.ProviderReasoningConfig{
			RequestField: "provider.reasoning.level",
			Mappings:     []assistantmodel.ProviderReasoningMapping{{Effort: assistantmodel.ReasoningEffortHigh, Value: "BALANCED"}},
		},
	}, "secret", "", assistantmodel.ReasoningEffortHigh)
	if err != nil {
		t.Fatalf("NewOpenAIResponsesADKModel: %v", err)
	}
	if response := singleResponse(t, llm, textRequest()); response == nil {
		t.Fatal("custom Responses request returned nil response")
	}
}

func textRequest() *adkmodel.LLMRequest {
	return &adkmodel.LLMRequest{Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)}}
}

func identityReasoningConfig(field string) assistantmodel.ProviderReasoningConfig {
	efforts := []assistantmodel.ReasoningEffort{
		assistantmodel.ReasoningEffortLow, assistantmodel.ReasoningEffortMedium, assistantmodel.ReasoningEffortHigh,
		assistantmodel.ReasoningEffortXHigh, assistantmodel.ReasoningEffortMax,
	}
	mappings := make([]assistantmodel.ProviderReasoningMapping, 0, len(efforts))
	for _, effort := range efforts {
		mappings = append(mappings, assistantmodel.ProviderReasoningMapping{Effort: effort, Value: string(effort)})
	}
	return assistantmodel.ProviderReasoningConfig{RequestField: field, Mappings: mappings}
}
