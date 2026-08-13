package providers

import (
	"encoding/json"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
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
		if request.URL.Path != "/v1/responses" {
			t.Fatalf("probe path = %q", request.URL.Path)
		}
		reasoning, _ := body["reasoning"].(map[string]any)
		if effort, ok := reasoning["effort"].(string); ok {
			efforts = append(efforts, effort)
			if effort == "DEEP" {
				http.Error(w, "deep unavailable", http.StatusBadRequest)
				return
			}
		}
		writeResponsesProbeResponse(t, w)
	}))
	defer server.Close()
	provider := probeTestProvider(server.URL, []assistantmodel.ProviderReasoningMapping{
		{Effort: assistantmodel.ReasoningEffortLow, Value: "FAST"},
		{Effort: assistantmodel.ReasoningEffortMedium, Value: "BALANCED"},
		{Effort: assistantmodel.ReasoningEffortHigh, Value: "DEEP"},
	})

	quick, err := ProbeProvider(t.Context(), provider, "secret", assistantmodel.ProviderTestModeQuick)
	if err != nil {
		t.Fatalf("quick probe: %v", err)
	}
	if requestCount != 3 || !slices.Equal(efforts, []string{"BALANCED"}) {
		t.Fatalf("quick requests=%d efforts=%v", requestCount, efforts)
	}
	if quick.Reasoning.Mode != assistantmodel.ProviderTestModeQuick || len(quick.Reasoning.Results) != 1 || !quick.Capabilities["reasoning"] {
		t.Fatalf("quick result = %+v", quick)
	}

	requestCount = 0
	efforts = nil
	full, err := ProbeProvider(t.Context(), provider, "secret", assistantmodel.ProviderTestModeFull)
	if err != nil {
		t.Fatalf("full probe: %v", err)
	}
	if requestCount != 5 || !slices.Equal(efforts, []string{"FAST", "BALANCED", "DEEP"}) {
		t.Fatalf("full requests=%d efforts=%v", requestCount, efforts)
	}
	if full.Reasoning.Mode != assistantmodel.ProviderTestModeFull || len(full.Reasoning.Results) != 3 ||
		full.Reasoning.Results[2].Error == "" || full.Capabilities["reasoning"] {
		t.Fatalf("full result = %+v", full)
	}

	requestCount = 0
	efforts = nil
	withoutMedium := probeTestProvider(server.URL, []assistantmodel.ProviderReasoningMapping{
		{Effort: assistantmodel.ReasoningEffortHigh, Value: "DEEP"},
		{Effort: assistantmodel.ReasoningEffortLow, Value: "FAST"},
	})
	if _, err := ProbeProvider(t.Context(), withoutMedium, "secret", assistantmodel.ProviderTestModeQuick); err != nil {
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
		if _, ok := body["reasoning"]; ok {
			t.Fatalf("unexpected reasoning field: %#v", body)
		}
		writeResponsesProbeResponse(t, w)
	}))
	defer server.Close()
	result, err := ProbeProvider(t.Context(), probeTestProvider(server.URL, nil), "secret", "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if requestCount != 2 || result.Reasoning.Mode != assistantmodel.ProviderTestModeQuick ||
		len(result.Reasoning.Results) != 0 || result.Capabilities["reasoning"] {
		t.Fatalf("requests=%d result=%+v", requestCount, result)
	}
}

func TestProviderProbeTimeoutCapsConfiguredRequestTimeout(t *testing.T) {
	for _, test := range []struct {
		provider assistantmodel.Provider
		want     time.Duration
	}{
		{provider: assistantmodel.Provider{}, want: MaxProviderProbeTimeout},
		{provider: assistantmodel.Provider{RequestTimeoutMs: 15_000}, want: 15 * time.Second},
		{provider: assistantmodel.Provider{RequestTimeoutMs: 600_000}, want: MaxProviderProbeTimeout},
	} {
		if got := ProviderProbeTimeout(test.provider); got != test.want {
			t.Fatalf("ProviderProbeTimeout() = %s, want %s", got, test.want)
		}
	}
}

func probeTestProvider(baseURL string, mappings []assistantmodel.ProviderReasoningMapping) assistantmodel.Provider {
	return assistantmodel.Provider{
		BaseURL: baseURL + "/v1", Model: "test-model",
		ReasoningConfig: assistantmodel.ProviderReasoningConfig{RequestField: "reasoning.effort", Mappings: mappings},
	}
}

func writeResponsesProbeResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id": "resp_probe", "model": "test-model",
		"output": []map[string]any{{
			"type": "message", "role": "assistant",
			"content": []map[string]any{{"type": "output_text", "text": "health check ok", "annotations": []any{}}},
		}},
	}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
