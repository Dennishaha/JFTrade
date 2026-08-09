package adk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestRuntimeTestProviderUsesResponsesProtocol(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.URL.Path != "/v1/responses" {
			t.Fatalf("provider test request path = %q, want /v1/responses", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret" || request.Header.Get("X-Desk") != "research" {
			t.Fatalf("provider test headers Authorization=%q X-Desk=%q", request.Header.Get("Authorization"), request.Header.Get("X-Desk"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode provider test request: %v", err)
		}
		_, hasTools := body["tools"]
		if hasTools != (requestCount == 2) {
			t.Fatalf("request %d tools present = %v", requestCount, hasTools)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_probe","object":"response","status":"completed","model":"test-model","output":[{"id":"msg_probe","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"health check ok","annotations":[]}]}]}`))
	}))
	defer server.Close()

	runtime := newTestRuntime(t)
	provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "responses-probe", BaseURL: server.URL + "/v1", Model: "test-model", APIKey: "secret",
		APIProtocol: ProviderAPIProtocolResponses, DefaultHeaders: map[string]string{"X-Desk": "research"}, Enabled: true,
	})
	probe, err := runtime.TestProvider(t.Context(), provider.ID)
	if err != nil {
		t.Fatalf("TestProvider: %v", err)
	}
	if requestCount != 2 || probe["reply"] != "health check ok" {
		t.Fatalf("provider probe requests=%d payload=%#v", requestCount, probe)
	}
	capabilities, ok := probe["capabilities"].(map[string]bool)
	if !ok || !capabilities["tools"] || !capabilities["streaming"] {
		t.Fatalf("provider capabilities = %#v", probe["capabilities"])
	}
}
