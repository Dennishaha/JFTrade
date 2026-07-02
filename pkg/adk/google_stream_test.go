package adk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adkmodel "google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestOpenAICompatibleADKModelGenerateContentBuildsProviderRequest(t *testing.T) {
	var captured openAIChatRequest
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/chat/completions" {
			t.Fatalf("request path = %q, want /openai/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q, want bearer test-key", got)
		}
		if got := r.Header.Get("X-JFTrade-Provider"); got != "mock" {
			t.Fatalf("default provider header = %q, want mock", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		jftradeCheckTestError(t, json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": "收到订单上下文",
				},
			}},
		}))
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/") + "/openai",
		Model:   "provider-model",
		DefaultHeaders: map[string]string{
			"X-JFTrade-Provider": "mock",
			"X-Blank":            " ",
		},
	}, " test-key ", "fallback-model")

	req := &adkmodel.LLMRequest{
		Model: "runtime-model",
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText("只回答可执行计划。", genai.RoleUser),
			Tools: []*genai.Tool{{
				FunctionDeclarations: []*genai.FunctionDeclaration{{
					Name:        "account.orders",
					Description: "读取订单列表",
					ParametersJsonSchema: map[string]any{
						"type":                 "object",
						"additionalProperties": true,
						"properties": map[string]any{
							"symbol": map[string]any{
								"type":                 "string",
								"additionalProperties": true,
							},
						},
					},
				}},
			}},
		},
		Contents: []*genai.Content{
			genai.NewContentFromText("查询 HK.00700 的订单", genai.RoleUser),
			genai.NewContentFromParts([]*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID:   "call-orders",
					Name: "account.orders",
					Args: map[string]any{"symbol": "HK.00700"},
				},
			}}, genai.RoleModel),
			genai.NewContentFromParts([]*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{
					ID:       "call-orders",
					Name:     "account.orders",
					Response: map[string]any{"count": 2},
				},
			}}, genai.RoleUser),
		},
	}

	var responses []*adkmodel.LLMResponse
	for response, err := range model.GenerateContent(context.Background(), req, false) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=false) error: %v", err)
		}
		responses = append(responses, response)
	}

	if len(responses) != 1 {
		t.Fatalf("response count = %d, want 1", len(responses))
	}
	if got := responses[0].Content.Parts[0].Text; got != "收到订单上下文" {
		t.Fatalf("response text = %q, want 收到订单上下文", got)
	}
	if captured.Model != "runtime-model" || captured.Stream {
		t.Fatalf("captured model/stream = %q/%v, want runtime-model/false", captured.Model, captured.Stream)
	}
	if len(captured.Messages) != 4 {
		t.Fatalf("message count = %d, want system/user/assistant/tool", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" || captured.Messages[0].Content != "只回答可执行计划。" {
		t.Fatalf("system message = %#v", captured.Messages[0])
	}
	if captured.Messages[1].Role != "user" || !strings.Contains(captured.Messages[1].Content, "HK.00700") {
		t.Fatalf("user message = %#v", captured.Messages[1])
	}
	if captured.Messages[2].Role != "assistant" || len(captured.Messages[2].ToolCalls) != 1 {
		t.Fatalf("assistant tool call message = %#v", captured.Messages[2])
	}
	if got := captured.Messages[2].ToolCalls[0].Function.Name; got != "account.orders" {
		t.Fatalf("assistant tool call name = %q, want account.orders before provider normalization", got)
	}
	if captured.Messages[3].Role != "tool" || captured.Messages[3].ToolCallID != "call-orders" {
		t.Fatalf("tool response message = %#v", captured.Messages[3])
	}
	if len(captured.Tools) != 1 || captured.ToolChoice != "auto" {
		t.Fatalf("tools/tool_choice = %#v/%#v", captured.Tools, captured.ToolChoice)
	}
	tool := captured.Tools[0].Function
	if tool.Name != "account-orders" || tool.Description != "读取订单列表" {
		t.Fatalf("tool declaration = %#v", tool)
	}
	if _, ok := tool.Parameters["additionalProperties"]; ok {
		t.Fatalf("top-level additionalProperties should be stripped: %#v", tool.Parameters)
	}
	properties, ok := tool.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool properties = %#v, want object", tool.Parameters["properties"])
	}
	symbol, ok := properties["symbol"].(map[string]any)
	if !ok {
		t.Fatalf("symbol property = %#v, want object", properties["symbol"])
	}
	if _, ok := symbol["additionalProperties"]; ok {
		t.Fatalf("nested additionalProperties should be stripped: %#v", symbol)
	}
}

func TestOpenAICompatibleADKModelGenerateContentStreamYieldsPartialAndFinal(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, jftradeErr1 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr1)
		_, jftradeErr2 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr2)
		_, jftradeErr3 := w.Write([]byte("data: [DONE]\n\n"))
		jftradeCheckTestError(t, jftradeErr3)
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/"),
		Model:   "gpt-test",
	}, "test-key", "gpt-test")

	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("你好", genai.RoleUser)},
	}

	var responses []*adkmodel.LLMResponse
	for response, err := range model.GenerateContent(context.Background(), req, true) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=true) error: %v", err)
		}
		responses = append(responses, response)
	}

	if len(responses) != 3 {
		t.Fatalf("stream response count = %d, want 3", len(responses))
	}
	if !responses[0].Partial || responses[0].Content == nil || responses[0].Content.Parts[0].Text != "你" {
		t.Fatalf("partial[0] = %#v, want partial text 你", responses[0])
	}
	if !responses[1].Partial || responses[1].Content == nil || responses[1].Content.Parts[0].Text != "好" {
		t.Fatalf("partial[1] = %#v, want partial text 好", responses[1])
	}
	if responses[2].Partial {
		t.Fatalf("final response unexpectedly marked partial")
	}
	if got := responses[2].Content.Parts[0].Text; got != "你好" {
		t.Fatalf("final response text = %q, want 你好", got)
	}
}

func TestOpenAICompatibleADKModelGenerateContentStreamMergesToolCallChunks(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeOpenAIStreamEvent(t, w, map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"content": "正在",
					"tool_calls": []any{map[string]any{
						"index": 0,
						"id":    "call-orders",
						"type":  "function",
						"function": map[string]any{
							"name":      "account-orders",
							"arguments": `{"symbol":`,
						},
					}},
				},
			}},
		})
		writeOpenAIStreamEvent(t, w, map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"content": "查询",
					"tool_calls": []any{map[string]any{
						"index": 0,
						"function": map[string]any{
							"arguments": `"HK.00700","limit":5}`,
						},
					}},
				},
			}},
		})
		_, jftradeErr1 := w.Write([]byte("data: [DONE]\n\n"))
		jftradeCheckTestError(t, jftradeErr1)
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/"),
		Model:   "gpt-test",
	}, "test-key", "gpt-test")

	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("查询订单", genai.RoleUser)},
	}

	var final *adkmodel.LLMResponse
	var partialText strings.Builder
	for response, err := range model.GenerateContent(context.Background(), req, true) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=true) error: %v", err)
		}
		if response.Partial {
			partialText.WriteString(response.Content.Parts[0].Text)
			continue
		}
		final = response
	}
	if got := partialText.String(); got != "正在查询" {
		t.Fatalf("partial text = %q, want 正在查询", got)
	}
	if final == nil || final.Content == nil {
		t.Fatalf("final response = %#v, want content", final)
	}
	if got := final.Content.Parts[0].Text; got != "正在查询" {
		t.Fatalf("final text = %q, want 正在查询", got)
	}
	if len(final.Content.Parts) != 2 || final.Content.Parts[1].FunctionCall == nil {
		t.Fatalf("final parts = %#v, want text plus function call", final.Content.Parts)
	}
	call := final.Content.Parts[1].FunctionCall
	if call.ID != "call-orders" || call.Name != "account.orders" {
		t.Fatalf("function call identity = id:%q name:%q, want call-orders/account.orders", call.ID, call.Name)
	}
	if call.Args["symbol"] != "HK.00700" || call.Args["limit"] != float64(5) {
		t.Fatalf("function call args = %#v, want merged JSON args", call.Args)
	}
}

func TestOpenAICompatibleADKModelGenerateContentStreamPreservesChunkSpacing(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, jftradeErr4 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Let\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr4)
		_, jftradeErr5 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\" me\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr5)
		_, jftradeErr6 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\" analyze\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr6)
		_, jftradeErr7 := w.Write([]byte("data: [DONE]\n\n"))
		jftradeCheckTestError(t, jftradeErr7)
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/"),
		Model:   "gpt-test",
	}, "test-key", "gpt-test")

	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("analyze", genai.RoleUser)},
	}

	var final *adkmodel.LLMResponse
	for response, err := range model.GenerateContent(context.Background(), req, true) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=true) error: %v", err)
		}
		if !response.Partial {
			final = response
		}
	}
	if final == nil || final.Content == nil || len(final.Content.Parts) == 0 {
		t.Fatalf("final response = %#v, want reasoning response", final)
	}
	if got := final.Content.Parts[0].Text; got != "Let me analyze" {
		t.Fatalf("final reasoning text = %q, want preserved spaces", got)
	}
}

func TestOpenAICompatibleADKModelGenerateContentStopsAfterYieldFalse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, jftradeErr8 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr8)
		_, jftradeErr9 := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n"))
		jftradeCheckTestError(t, jftradeErr9)
		_, jftradeErr10 := w.Write([]byte("data: [DONE]\n\n"))
		jftradeCheckTestError(t, jftradeErr10)
	}))
	defer mockServer.Close()

	model := newOpenAICompatibleADKModel(Provider{
		BaseURL: strings.TrimSuffix(mockServer.URL, "/"),
		Model:   "gpt-test",
	}, "test-key", "gpt-test")

	req := &adkmodel.LLMRequest{
		Model:    "gpt-test",
		Contents: []*genai.Content{genai.NewContentFromText("你好", genai.RoleUser)},
	}

	count := 0
	for response, err := range model.GenerateContent(context.Background(), req, true) {
		if err != nil {
			t.Fatalf("GenerateContent(stream=true) unexpected error: %v", err)
		}
		if response == nil {
			t.Fatal("expected non-nil response before stopping iteration")
		}
		count++
		break
	}

	if count != 1 {
		t.Fatalf("response count = %d, want 1", count)
	}
}

func TestOpenAIMessageToADKResponseRejectsMalformedToolArguments(t *testing.T) {
	var call openAIToolCall
	call.ID = "call-orders"
	call.Type = "function"
	call.Function.Name = "account-orders"
	call.Function.Arguments = `{"symbol":`

	_, err := openAIMessageToADKResponse(openAIChatMessage{
		ToolCalls: []openAIToolCall{call},
	}, false)
	if err == nil {
		t.Fatal("openAIMessageToADKResponse() error = nil, want malformed tool arguments error")
	}
	if !strings.Contains(err.Error(), "decode tool arguments for account-orders") {
		t.Fatalf("error = %v, want tool argument decode context", err)
	}
}

func writeOpenAIStreamEvent(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	jftradeCheckTestError(t, err)
	_, writeErr := w.Write([]byte("data: " + string(raw) + "\n\n"))
	jftradeCheckTestError(t, writeErr)
}

func TestGoogleADKExecutionConsumeEventSkipsDuplicateFinalTextAfterPartial(t *testing.T) {
	execution := &googleADKExecution{}

	partialA := adksession.NewEvent("partial-a")
	partialA.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("你", genai.RoleModel),
		Partial: true,
	}
	partialB := adksession.NewEvent("partial-b")
	partialB.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("好", genai.RoleModel),
		Partial: true,
	}
	final := adksession.NewEvent("final")
	final.LLMResponse = adkmodel.LLMResponse{
		Content: genai.NewContentFromText("你好", genai.RoleModel),
	}

	if err := execution.consumeEvent(partialA); err != nil {
		t.Fatalf("consumeEvent(partialA): %v", err)
	}
	if err := execution.consumeEvent(partialB); err != nil {
		t.Fatalf("consumeEvent(partialB): %v", err)
	}
	if err := execution.consumeEvent(final); err != nil {
		t.Fatalf("consumeEvent(final): %v", err)
	}

	if got := execution.result().Reply; got != "你好" {
		t.Fatalf("reply = %q, want 你好", got)
	}
}
