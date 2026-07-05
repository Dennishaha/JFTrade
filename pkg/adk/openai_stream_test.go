package adk

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIClientChatStreamStructuredResponseContract(t *testing.T) {
	var received openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Method != http.MethodPost {
			t.Fatalf("provider request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Tenant"); got != "research" {
			t.Fatalf("X-Tenant = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"final answer","reasoning_content":"checked risk"}}]}`)
	}))
	t.Cleanup(server.Close)

	var deltas []ChatDelta
	result, err := (openAIClient{}).chatStream(t.Context(), Provider{
		BaseURL: server.URL, Model: "provider-default", DefaultHeaders: map[string]string{"X-Tenant": "research", " ": "ignored"},
	}, " secret-key ", "", []openAIChatMessage{{Role: "user", Content: "analyze"}}, func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("chatStream: %v", err)
	}
	if received.Model != "provider-default" || !received.Stream || len(received.Messages) != 1 {
		t.Fatalf("provider payload = %#v", received)
	}
	if result.Reply != "final answer" || result.ReasoningContent != "checked risk" {
		t.Fatalf("structured result = %#v", result)
	}
	if len(deltas) != 1 || deltas[0].Reply != result.Reply || deltas[0].ReasoningContent != result.ReasoningContent {
		t.Fatalf("structured deltas = %#v", deltas)
	}
}

func TestOpenAIClientChatStreamProviderFailureContracts(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        string
	}{
		{name: "json error on non-success", status: http.StatusTooManyRequests, body: `{"error":{"message":"quota exhausted"}}`, want: "provider returned 429: quota exhausted"},
		{name: "opaque non-success", status: http.StatusBadGateway, body: `upstream unavailable`, want: "provider returned 502"},
		{name: "malformed json success", status: http.StatusOK, body: `{`, want: "decode OpenAI-compatible response"},
		{name: "json error on success", status: http.StatusOK, body: `{"error":{"message":"model unavailable"}}`, want: "provider returned: model unavailable"},
		{name: "missing choices", status: http.StatusOK, body: `{"choices":[]}`, want: "provider returned no choices"},
		{name: "empty structured reply", status: http.StatusOK, body: `{"choices":[{"message":{"content":" "}}]}`, want: "provider returned an empty reply"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			_, err := (openAIClient{}).chatStream(t.Context(), Provider{BaseURL: server.URL, Model: "model"}, "", "", nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("chatStream err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestOpenAIClientReadStreamingResponseCombinesDeltaAndMessageFrames(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		": keepalive",
		`data: {"choices":[{"delta":{"content":"hello ","reasoning_content":"first "}}]}`,
		"",
		`data: {"choices":[{"message":{"content":"world","reasoning":"second"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
	var deltas []ChatDelta
	result, err := (openAIClient{}).readStreamingResponse(body, func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("readStreamingResponse: %v", err)
	}
	if result.Reply != "hello world" || result.ReasoningContent != "first second" {
		t.Fatalf("stream result = %#v", result)
	}
	if len(deltas) != 2 || deltas[0].Reply != "hello " || deltas[1].Reply != "world" {
		t.Fatalf("stream deltas = %#v", deltas)
	}
}

func TestOpenAIClientReadStreamingResponseFailureContracts(t *testing.T) {
	callbackErr := errors.New("consumer stopped")
	cases := []struct {
		name     string
		body     string
		callback func(ChatDelta) error
		want     string
	}{
		{name: "malformed event", body: "data: {\n\n", want: "decode OpenAI-compatible stream chunk"},
		{name: "provider event error", body: "data: {\"error\":{\"message\":\"stream rejected\"}}\n\n", want: "provider returned: stream rejected"},
		{name: "empty completed stream", body: "data: [DONE]\n\n", want: "provider returned an empty reply"},
		{name: "delta callback error", body: "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", callback: func(ChatDelta) error { return callbackErr }, want: callbackErr.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (openAIClient{}).readStreamingResponse(strings.NewReader(tc.body), tc.callback)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("readStreamingResponse err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestOpenAIClientReadStreamingResponseSurfacesTransportInterruption(t *testing.T) {
	wantErr := errors.New("connection reset")
	body := io.MultiReader(
		strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"),
		streamErrorReader{err: wantErr},
	)
	_, err := (openAIClient{}).readStreamingResponse(body, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("readStreamingResponse err = %v, want %v", err, wantErr)
	}
}

func TestOpenAIClientEmitStructuredMessagePropagatesConsumerError(t *testing.T) {
	wantErr := errors.New("downstream closed")
	_, err := (openAIClient{}).emitStructuredMessage(openAIChatMessage{Content: "answer"}, func(ChatDelta) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("emitStructuredMessage err = %v, want %v", err, wantErr)
	}
}

func TestOpenAIClientSelectToolsProviderContracts(t *testing.T) {
	client := openAIClient{}
	if selected, err := client.selectTools(t.Context(), Provider{}, "", "", nil, nil); err != nil || selected != nil {
		t.Fatalf("selectTools without descriptors = %#v, %v", selected, err)
	}

	descriptors := []ToolDescriptor{{
		Name: "account.orders", DisplayName: "Orders", Description: "Read current broker orders",
		Category: "portfolio", Permission: "read_internal",
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tool-key" {
			t.Fatalf("tool selection Authorization = %q", got)
		}
		var request openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode tool selection request: %v", err)
		}
		if request.Model != "override-model" || request.Stream || len(request.Tools) != 1 || request.ToolChoice != "auto" {
			t.Fatalf("tool selection request = %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"account-orders","arguments":"{\"scope\":\"current\"}"}}]}}]}`)
	}))
	defer server.Close()

	selected, err := client.selectTools(t.Context(), Provider{BaseURL: server.URL, Model: "default-model"}, " tool-key ", "override-model", []openAIChatMessage{{Role: "user", Content: "show orders"}}, descriptors)
	if err != nil {
		t.Fatalf("selectTools: %v", err)
	}
	if len(selected) != 1 || selected[0].Name != "account.orders" || selected[0].Input["scope"] != "current" {
		t.Fatalf("selected tools = %#v", selected)
	}
}

func TestOpenAIClientSelectToolsFailureContracts(t *testing.T) {
	descriptors := []ToolDescriptor{{Name: "market.quote", Description: "Read quote"}}
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "status with detail", status: http.StatusTooManyRequests, body: "rate limited", want: "provider returned 429 during tool selection: rate limited"},
		{name: "status without detail", status: http.StatusBadGateway, body: " ", want: "provider returned 502 during tool selection: 502 Bad Gateway"},
		{name: "malformed response", status: http.StatusOK, body: `{`, want: "decode OpenAI-compatible tool selection"},
		{name: "provider error", status: http.StatusOK, body: `{"error":{"message":"tools disabled"}}`, want: "provider returned: tools disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			_, err := (openAIClient{}).selectTools(t.Context(), Provider{BaseURL: server.URL, Model: "model"}, "", "", nil, descriptors)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("selectTools err = %v, want containing %q", err, tc.want)
			}
		})
	}

	t.Run("no choices means no tool call", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"choices":[]}`)
		}))
		defer server.Close()
		selected, err := (openAIClient{}).selectTools(t.Context(), Provider{BaseURL: server.URL, Model: "model"}, "", "", nil, descriptors)
		if err != nil || selected != nil {
			t.Fatalf("selectTools no choices = %#v, %v", selected, err)
		}
	})
}

type streamErrorReader struct {
	err error
}

func (r streamErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}
