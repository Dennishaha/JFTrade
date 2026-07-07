package adk

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adkmodel "google.golang.org/adk/v2/model"
)

func TestOpenAIClientAdditionalTransportAndStreamBranches(t *testing.T) {
	t.Run("message role normalization preserves blank role", func(t *testing.T) {
		if got := normalizeProviderMessageRole("  "); got != "  " {
			t.Fatalf("normalizeProviderMessageRole blank = %q, want original blank role", got)
		}
	})

	t.Run("selectTools forwards nonblank default headers and reports transport failures", func(t *testing.T) {
		descriptors := []ToolDescriptor{{Name: "market.quote", Description: "Read quote"}}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Desk"); got != "alpha" {
				t.Fatalf("X-Desk = %q, want alpha", got)
			}
			_, _ = io.WriteString(w, `{"choices":[]}`)
		}))
		baseURL := server.URL
		if selected, err := (openAIClient{}).selectTools(t.Context(), Provider{
			BaseURL: baseURL, Model: "model", DefaultHeaders: map[string]string{"X-Desk": "alpha", "X-Blank": " "},
		}, "", "", nil, descriptors); err != nil || selected != nil {
			t.Fatalf("selectTools headers selected=%#v err=%v, want nil/nil", selected, err)
		}
		server.Close()
		if _, err := (openAIClient{}).selectTools(t.Context(), Provider{BaseURL: baseURL, Model: "model"}, "", "", nil, descriptors); err == nil {
			t.Fatal("selectTools accepted closed provider server")
		}
	})

	t.Run("chatStream forwards default headers and reports transport failures", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Trace"); got != "trace-1" {
				t.Fatalf("X-Trace = %q, want trace-1", got)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		}))
		baseURL := server.URL
		result, err := (openAIClient{}).chatStream(t.Context(), Provider{
			BaseURL: baseURL, Model: "model", DefaultHeaders: map[string]string{"X-Trace": "trace-1"},
		}, "", "", nil, nil)
		if err != nil || result.Reply != "ok" {
			t.Fatalf("chatStream event stream result=%#v err=%v, want ok/nil", result, err)
		}
		server.Close()
		if _, err := (openAIClient{}).chatStream(t.Context(), Provider{BaseURL: baseURL, Model: "model"}, "", "", nil, nil); err == nil {
			t.Fatal("chatStream accepted closed provider server")
		}
	})

	t.Run("stream accumulator ignores blank payloads and nil delta tail", func(t *testing.T) {
		_, err := (openAIClient{}).readStreamingResponse(strings.NewReader("data:   \n\ndata: [DONE]\n\n"), nil)
		if err == nil || !strings.Contains(err.Error(), "empty reply") {
			t.Fatalf("blank stream err = %v, want empty reply", err)
		}

		var reply, reasoning strings.Builder
		var splitter legacyAssistantContentSplitter
		if err := appendStreamChoice(&splitter, &reply, &reasoning, "visible", "", "", nil); err != nil {
			t.Fatalf("appendStreamChoice nil delta: %v", err)
		}
		if reply.String() != "visible" {
			t.Fatalf("reply = %q, want visible", reply.String())
		}
	})

	t.Run("body read and final aggregation helpers surface direct read failures", func(t *testing.T) {
		wantErr := errors.New("body read failed")
		if err := providerResponseError(&http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Body:       io.NopCloser(streamErrorReader{err: wantErr}),
		}); !errors.Is(err, wantErr) {
			t.Fatalf("providerResponseError err = %v, want %v", err, wantErr)
		}

		if err := scanOpenAIEventStream(bufio.NewScanner(streamErrorReader{err: wantErr}), new([]string), func() error { return nil }); !errors.Is(err, wantErr) {
			t.Fatalf("scanOpenAIEventStream err = %v, want %v", err, wantErr)
		}

		if err := yieldFinalOpenAIStreamResponse(&openAIStreamAggregationState{
			toolCalls: []openAIToolCall{{Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "tool", Arguments: "{"}}},
		}, func(*adkmodel.LLMResponse, error) bool { return true }); err == nil || !strings.Contains(err.Error(), "decode tool arguments") {
			t.Fatalf("yieldFinalOpenAIStreamResponse err = %v, want tool decode failure", err)
		}

		if err := yieldFinalOpenAIStreamResponse(&openAIStreamAggregationState{}, func(*adkmodel.LLMResponse, error) bool { return true }); err == nil || !strings.Contains(err.Error(), "empty reply") {
			t.Fatalf("yieldFinalOpenAIStreamResponse empty err = %v", err)
		}
	})
}
