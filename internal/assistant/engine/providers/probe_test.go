package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestProbeProviderQuickAndFullRequestCounts(t *testing.T) {
	var efforts []string
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if effort, ok := body["reasoning_effort"].(string); ok {
			efforts = append(efforts, effort)
			if effort == "DEEP" {
				http.Error(w, "deep unavailable", http.StatusBadRequest)
				return
			}
		}
		writeChatProbeResponse(t, w)
	}))
	defer server.Close()
	provider := probeTestProvider(server.URL, []jfadkmodel.ProviderReasoningMapping{
		{Effort: jfadkmodel.ReasoningEffortLow, Value: "FAST"},
		{Effort: jfadkmodel.ReasoningEffortMedium, Value: "BALANCED"},
		{Effort: jfadkmodel.ReasoningEffortHigh, Value: "DEEP"},
	})

	quick, err := ProbeProvider(t.Context(), provider, "secret", jfadkmodel.ProviderTestModeQuick)
	if err != nil {
		t.Fatalf("quick probe: %v", err)
	}
	if requestCount != 3 || !slices.Equal(efforts, []string{"BALANCED"}) {
		t.Fatalf("quick requests=%d efforts=%v", requestCount, efforts)
	}
	if quick.Reasoning.Mode != jfadkmodel.ProviderTestModeQuick || len(quick.Reasoning.Results) != 1 || !quick.Capabilities["reasoning"] {
		t.Fatalf("quick result = %+v", quick)
	}

	requestCount = 0
	efforts = nil
	full, err := ProbeProvider(t.Context(), provider, "secret", jfadkmodel.ProviderTestModeFull)
	if err != nil {
		t.Fatalf("full probe: %v", err)
	}
	if requestCount != 5 || !slices.Equal(efforts, []string{"FAST", "BALANCED", "DEEP"}) {
		t.Fatalf("full requests=%d efforts=%v", requestCount, efforts)
	}
	if full.Reasoning.Mode != jfadkmodel.ProviderTestModeFull || len(full.Reasoning.Results) != 3 ||
		full.Reasoning.Results[2].Error == "" || full.Capabilities["reasoning"] {
		t.Fatalf("full result = %+v", full)
	}

	requestCount = 0
	efforts = nil
	withoutMedium := probeTestProvider(server.URL, []jfadkmodel.ProviderReasoningMapping{
		{Effort: jfadkmodel.ReasoningEffortHigh, Value: "DEEP"},
		{Effort: jfadkmodel.ReasoningEffortLow, Value: "FAST"},
	})
	if _, err := ProbeProvider(t.Context(), withoutMedium, "secret", jfadkmodel.ProviderTestModeQuick); err != nil {
		t.Fatalf("quick canonical probe: %v", err)
	}
	if requestCount != 3 || !slices.Equal(efforts, []string{"FAST"}) {
		t.Fatalf("quick canonical requests=%d efforts=%v", requestCount, efforts)
	}
}

func TestProbeProviderWithoutMappingsSendsNoReasoningField(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Fatalf("unexpected reasoning field: %#v", body)
		}
		writeChatProbeResponse(t, w)
	}))
	defer server.Close()
	result, err := ProbeProvider(t.Context(), probeTestProvider(server.URL, nil), "secret", "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if requestCount != 2 || result.Reasoning.Mode != jfadkmodel.ProviderTestModeQuick ||
		len(result.Reasoning.Results) != 0 || result.Capabilities["reasoning"] {
		t.Fatalf("requests=%d result=%+v", requestCount, result)
	}
}

func TestProviderProbeTimeoutCapsConfiguredRequestTimeout(t *testing.T) {
	for _, test := range []struct {
		provider jfadkmodel.Provider
		want     time.Duration
	}{
		{provider: jfadkmodel.Provider{}, want: MaxProviderProbeTimeout},
		{provider: jfadkmodel.Provider{RequestTimeoutMs: 15_000}, want: 15 * time.Second},
		{provider: jfadkmodel.Provider{RequestTimeoutMs: 600_000}, want: MaxProviderProbeTimeout},
	} {
		if got := ProviderProbeTimeout(test.provider); got != test.want {
			t.Fatalf("ProviderProbeTimeout() = %s, want %s", got, test.want)
		}
	}
}

func probeTestProvider(baseURL string, mappings []jfadkmodel.ProviderReasoningMapping) jfadkmodel.Provider {
	return jfadkmodel.Provider{
		BaseURL: baseURL, Model: "test-model", APIProtocol: jfadkmodel.ProviderAPIProtocolChatCompletions,
		ReasoningConfig: jfadkmodel.ProviderReasoningConfig{RequestField: "reasoning_effort", Mappings: mappings},
	}
}

func writeChatProbeResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "health check ok"}}},
	}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
