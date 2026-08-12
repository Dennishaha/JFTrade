package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func TestOpenAICompatibleReasoningEffortRequestField(t *testing.T) {
	request := &adkmodel.LLMRequest{
		Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
	}
	for _, test := range []struct {
		name   string
		effort jfadkmodel.ReasoningEffort
		want   string
	}{
		{name: "model default", effort: ""},
		{name: "low", effort: jfadkmodel.ReasoningEffortLow, want: "low"},
		{name: "medium", effort: jfadkmodel.ReasoningEffortMedium, want: "medium"},
		{name: "high", effort: jfadkmodel.ReasoningEffortHigh, want: "high"},
		{name: "xhigh", effort: jfadkmodel.ReasoningEffortXHigh, want: "xhigh"},
		{name: "max", effort: jfadkmodel.ReasoningEffortMax, want: "max"},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := jfadkmodel.Provider{
				Model: "test-model", ReasoningConfig: identityReasoningConfig("reasoning_effort"),
			}
			llm := NewOpenAICompatibleADKModel(
				provider, "secret", "", test.effort,
			).(*OpenAICompatibleADKModel)
			payload := llm.BuildChatRequest(request, true)
			if payload.ReasoningValue != test.want {
				t.Fatalf("reasoning value = %q, want %q", payload.ReasoningValue, test.want)
			}
			httpRequest, err := llm.NewChatRequest(t.Context(), payload)
			if err != nil {
				t.Fatalf("NewChatRequest: %v", err)
			}
			var wire map[string]any
			if err := json.NewDecoder(httpRequest.Body).Decode(&wire); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			_, hasField := wire["reasoning_effort"]
			if hasField != (test.want != "") {
				t.Fatalf("reasoning_effort field present = %v, want %v: %#v", hasField, test.want != "", wire)
			}
		})
	}
}

func TestOpenAICompatibleCustomReasoningMappingInjectsNestedField(t *testing.T) {
	provider := jfadkmodel.Provider{
		BaseURL:     "https://example.test/v1",
		Model:       "test-model",
		APIProtocol: jfadkmodel.ProviderAPIProtocolChatCompletions,
		ReasoningConfig: jfadkmodel.ProviderReasoningConfig{
			RequestField: "provider.reasoning.level",
			Mappings: []jfadkmodel.ProviderReasoningMapping{
				{Effort: jfadkmodel.ReasoningEffortLow, Value: "LOW"},
				{Effort: jfadkmodel.ReasoningEffortHigh, Value: "balanced"},
				{Effort: jfadkmodel.ReasoningEffortMax, Value: "balanced"},
			},
		},
	}
	request := &adkmodel.LLMRequest{
		Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
	}
	llm := NewOpenAICompatibleADKModel(provider, "secret", "", jfadkmodel.ReasoningEffortMax).(*OpenAICompatibleADKModel)
	httpRequest, err := llm.NewChatRequest(t.Context(), llm.BuildChatRequest(request, false))
	if err != nil {
		t.Fatalf("NewChatRequest: %v", err)
	}
	var wire map[string]any
	if err := json.NewDecoder(httpRequest.Body).Decode(&wire); err != nil {
		t.Fatalf("decode custom chat request: %v", err)
	}
	if _, ok := wire["reasoning_effort"]; ok {
		t.Fatalf("custom mapping leaked legacy reasoning_effort: %#v", wire)
	}
	providerObject, ok := wire["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider object missing: %#v", wire)
	}
	reasoningObject, ok := providerObject["reasoning"].(map[string]any)
	if !ok || reasoningObject["level"] != "balanced" {
		t.Fatalf("custom nested reasoning mapping = %#v, want balanced", providerObject)
	}
}

func TestResponsesReasoningEffortRequestField(t *testing.T) {
	for _, test := range []struct {
		name       string
		effort     jfadkmodel.ReasoningEffort
		wantEffort string
	}{
		{name: "model default", effort: ""},
		{name: "high", effort: jfadkmodel.ReasoningEffortHigh, wantEffort: "high"},
		{name: "xhigh", effort: jfadkmodel.ReasoningEffortXHigh, wantEffort: "xhigh"},
		{name: "max", effort: jfadkmodel.ReasoningEffortMax, wantEffort: "max"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
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

			llm, err := NewOpenAIResponsesADKModel(t.Context(), jfadkmodel.Provider{
				BaseURL: server.URL + "/v1", Model: "test-model", APIProtocol: jfadkmodel.ProviderAPIProtocolResponses,
				ReasoningConfig: identityReasoningConfig("reasoning.effort"),
			}, "secret", "", test.effort)
			if err != nil {
				t.Fatalf("NewOpenAIResponsesADKModel: %v", err)
			}
			response := singleResponse(t, llm, &adkmodel.LLMRequest{
				Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
			})
			if response == nil {
				t.Fatal("Responses request returned nil response")
			}
		})
	}
}

func identityReasoningConfig(field string) jfadkmodel.ProviderReasoningConfig {
	efforts := []jfadkmodel.ReasoningEffort{
		jfadkmodel.ReasoningEffortLow, jfadkmodel.ReasoningEffortMedium, jfadkmodel.ReasoningEffortHigh,
		jfadkmodel.ReasoningEffortXHigh, jfadkmodel.ReasoningEffortMax,
	}
	mappings := make([]jfadkmodel.ProviderReasoningMapping, 0, len(efforts))
	for _, effort := range efforts {
		mappings = append(mappings, jfadkmodel.ProviderReasoningMapping{Effort: effort, Value: string(effort)})
	}
	return jfadkmodel.ProviderReasoningConfig{RequestField: field, Mappings: mappings}
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

	llm, err := NewOpenAIResponsesADKModel(t.Context(), jfadkmodel.Provider{
		BaseURL:     server.URL + "/v1",
		Model:       "test-model",
		APIProtocol: jfadkmodel.ProviderAPIProtocolResponses,
		ReasoningConfig: jfadkmodel.ProviderReasoningConfig{
			RequestField: "provider.reasoning.level",
			Mappings: []jfadkmodel.ProviderReasoningMapping{
				{Effort: jfadkmodel.ReasoningEffortHigh, Value: "BALANCED"},
			},
		},
	}, "secret", "", jfadkmodel.ReasoningEffortHigh)
	if err != nil {
		t.Fatalf("NewOpenAIResponsesADKModel: %v", err)
	}
	response := singleResponse(t, llm, &adkmodel.LLMRequest{
		Contents: []*genai.Content{genai.NewContentFromText("hello", genai.RoleUser)},
	})
	if response == nil {
		t.Fatal("custom Responses request returned nil response")
	}
}
