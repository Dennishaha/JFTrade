package providers

import (
	"context"
	"encoding/json"
	"errors"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

func TestResponsesModelSendsSanitizedToolsAndRestoresCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assertResponsesProviderRequest(t, request)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_1","model":"test-model","output":[{"type":"function_call","call_id":"call_1","name":"market-data","arguments":{"symbol":"AAPL"}}],"usage":{"input_tokens":12,"input_tokens_details":{"cached_tokens":2},"output_tokens":4,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":16}}`))
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	llm, err := NewOpenAIResponsesADKModel(t.Context(), assistantmodel.Provider{
		BaseURL: server.URL + "/v1", Model: "test-model", DefaultHeaders: map[string]string{"X-Desk": "research"},
	}, "secret", "")
	if err != nil {
		t.Fatalf("NewOpenAIResponsesADKModel: %v", err)
	}
	response := singleResponse(t, llm, responsesToolRequest())
	call := response.Content.Parts[0].FunctionCall
	if call == nil || call.Name != "market.data" || call.ID != "call_1" {
		t.Fatalf("restored function call = %+v, want market.data/call_1", call)
	}
	if call.Args["symbol"] != "AAPL" {
		t.Fatalf("restored function arguments = %#v, want symbol AAPL", call.Args)
	}
	if response.UsageMetadata == nil || response.UsageMetadata.PromptTokenCount != 12 || response.UsageMetadata.CandidatesTokenCount != 4 || response.UsageMetadata.ThoughtsTokenCount != 1 {
		t.Fatalf("Responses usage = %+v, want input=12 output=4 thoughts=1", response.UsageMetadata)
	}
}

func TestResponsesModelRetainsStreamingUsageMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"response.created","response":{"id":"resp_stream","model":"test-model"}}`,
			`{"type":"response.output_text.delta","delta":"done"}`,
			`{"type":"response.completed","response":{"id":"resp_stream","model":"test-model","usage":{"input_tokens":9,"input_tokens_details":{"cached_tokens":0},"output_tokens":3,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":12}}}`,
			`[DONE]`,
		}
		for _, event := range events {
			if _, err := w.Write([]byte("data: " + event + "\n\n")); err != nil {
				t.Errorf("write stream event: %v", err)
				return
			}
		}
	}))
	defer server.Close()

	llm, err := NewOpenAIResponsesADKModel(t.Context(), assistantmodel.Provider{
		BaseURL: server.URL + "/v1", Model: "test-model",
	}, "secret", "")
	if err != nil {
		t.Fatalf("NewOpenAIResponsesADKModel: %v", err)
	}
	var final *model.LLMResponse
	for response, responseErr := range llm.GenerateContent(t.Context(), responsesToolRequest(), true) {
		if responseErr != nil {
			t.Fatalf("GenerateContent: %v", responseErr)
		}
		final = response
	}
	if final == nil || final.Partial || final.UsageMetadata == nil {
		t.Fatalf("final streaming response = %+v, want final usage", final)
	}
	if final.UsageMetadata.PromptTokenCount != 9 || final.UsageMetadata.CandidatesTokenCount != 3 || final.UsageMetadata.TotalTokenCount != 12 {
		t.Fatalf("final streaming usage = %+v, want input=9 output=3 total=12", final.UsageMetadata)
	}
}

func TestResponsesToolNamesRejectSanitizationCollision(t *testing.T) {
	req := responsesToolRequest()
	req.Config.Tools[0].FunctionDeclarations = append(req.Config.Tools[0].FunctionDeclarations, &genai.FunctionDeclaration{Name: "market-data"})
	_, _, err := prepareResponsesRequest(req)
	if err == nil || !strings.Contains(err.Error(), "tool name collision") {
		t.Fatalf("prepareResponsesRequest collision error = %v", err)
	}
}

func TestResponsesToolNameModelRestoresStreamFunctionCalls(t *testing.T) {
	wrapped := &ResponsesToolNameModel{inner: responsesModelStub{responses: []*model.LLMResponse{{
		Content: genai.NewContentFromFunctionCall("market-data", map[string]any{"symbol": "AAPL"}, genai.RoleModel), Partial: true,
	}, {
		Content: genai.NewContentFromFunctionCall("market-data", map[string]any{"symbol": "AAPL"}, genai.RoleModel), TurnComplete: true,
	}}}}
	for response, err := range wrapped.GenerateContent(t.Context(), responsesToolRequest(), true) {
		if err != nil {
			t.Fatalf("GenerateContent: %v", err)
		}
		if got := response.Content.Parts[0].FunctionCall.Name; got != "market.data" {
			t.Fatalf("stream function name = %q, want market.data", got)
		}
	}
}

func TestResponsesModelBoundaryErrorsAndNameFallbacks(t *testing.T) {
	if _, _, err := prepareResponsesRequest(nil); err == nil {
		t.Fatal("nil Responses request accepted")
	}
	for response, err := range responseModelError(errors.New("response error")) {
		if response != nil || err == nil {
			t.Fatalf("responseModelError yielded response=%#v err=%v", response, err)
		}
	}
	mapper := responsesToolNames{toWire: map[string]string{"market.data": "market-data"}, fromWire: map[string]string{"market-data": "market.data"}}
	if got := mapper.toResponseName("unknown.name"); got != "unknown-name" {
		t.Fatalf("unknown response tool name = %q", got)
	}
	if got := mapper.restoreResponseName("unknown-name"); got != "unknown-name" {
		t.Fatalf("unknown restored tool name = %q", got)
	}
	if got := mapper.cloneTools([]*genai.Tool{nil, {FunctionDeclarations: []*genai.FunctionDeclaration{nil, {Name: "market.data"}}}}); len(got) != 2 || got[0] != nil || got[1].FunctionDeclarations[1].Name != "market-data" {
		t.Fatalf("cloned tool declarations = %#v", got)
	}
	if got := mapper.cloneContent(nil, mapper.toResponseName); got != nil {
		t.Fatalf("nil content clone = %#v", got)
	}
	if got := cloneResponsePart(nil, mapper.toResponseName); got != nil {
		t.Fatalf("nil response part clone = %#v", got)
	}
	for response, err := range (&ResponsesToolNameModel{inner: responsesModelStub{responses: []*model.LLMResponse{nil}}}).GenerateContent(t.Context(), responsesToolRequest(), false) {
		if response != nil || err != nil {
			t.Fatalf("nil inner response yielded response=%#v err=%v", response, err)
		}
	}
}

func TestProbeResponsesProviderReportsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()
	_, err := ProbeOpenAIResponsesProvider(t.Context(), assistantmodel.Provider{BaseURL: server.URL + "/v1", Model: "model"}, "key", false)
	if err == nil {
		t.Fatal("malformed Responses response accepted")
	}
}

type responsesModelStub struct {
	responses []*model.LLMResponse
}

func (responsesModelStub) Name() string { return "stub" }

func (stub responsesModelStub) GenerateContent(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		for _, response := range stub.responses {
			if !yield(response, nil) {
				return
			}
		}
	}
}

func responsesToolRequest() *model.LLMRequest {
	return &model.LLMRequest{
		Contents: []*genai.Content{
			genai.NewContentFromText("look up AAPL", genai.RoleUser),
			genai.NewContentFromFunctionCall("market.data", map[string]any{"symbol": "AAPL"}, genai.RoleModel),
			genai.NewContentFromFunctionResponse("market.data", map[string]any{"output": "ready"}, genai.RoleUser),
		},
		Config: &genai.GenerateContentConfig{
			Tools:      []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: "market.data"}}}},
			ToolConfig: &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{AllowedFunctionNames: []string{"market.data"}}},
		},
	}
}

func singleResponse(t *testing.T, llm model.LLM, req *model.LLMRequest) *model.LLMResponse {
	t.Helper()
	for response, err := range llm.GenerateContent(t.Context(), req, false) {
		if err != nil {
			t.Fatalf("GenerateContent: %v", err)
		}
		return response
	}
	t.Fatal("GenerateContent returned no response")
	return nil
}

func assertResponsesProviderRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.URL.Path != "/v1/responses" || request.Header.Get("X-Desk") != "research" {
		t.Fatalf("Responses request = %s, X-Desk=%q", request.URL.Path, request.Header.Get("X-Desk"))
	}
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatalf("decode Responses request: %v", err)
	}
	raw, err := json.Marshal(body)
	if err != nil || strings.Contains(string(raw), "market.data") || !strings.Contains(string(raw), "market-data") {
		t.Fatalf("Responses request tools = %s", raw)
	}
}
