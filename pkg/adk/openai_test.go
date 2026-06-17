package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	adkmodel "google.golang.org/adk/model"
	"google.golang.org/genai"
)

// TestToolNameSanitizeRestoreRoundTrip verifies that tool names survive
// the sanitize → restore round trip correctly, especially names with
// underscores or hyphens.
func TestToolNameSanitizeRestoreRoundTrip(t *testing.T) {
	tests := []struct {
		original  string
		sanitized string
		restored  string
		ok        bool
	}{
		{"account.orders", "account-orders", "account.orders", true},
		{"system.futu_opend", "system-futu_opend", "system.futu_opend", true},
		{"portfolio.summary", "portfolio-summary", "portfolio.summary", true},
		{"market.subscriptions", "market-subscriptions", "market.subscriptions", true},
		{"strategy.save_draft", "strategy-save_draft", "strategy.save_draft", true},
		{"http.fetch", "http-fetch", "http.fetch", true},
		{"backtest.runs", "backtest-runs", "backtest.runs", true},
		// Names with hyphens are NOT safe — the original hyphen is lost after restore.
		// No current JFTrade tools use hyphens, so this is acceptable.
		{"my-tool.name", "my-tool-name", "my.tool.name", false},
	}

	for _, test := range tests {
		t.Run(test.original, func(t *testing.T) {
			sanitized := sanitizeToolNameForOpenAI(test.original)
			restored := restoreToolNameFromOpenAI(sanitized)
			if sanitized != test.sanitized {
				t.Errorf("sanitize(%q) = %q, want %q", test.original, sanitized, test.sanitized)
			}
			if restored != test.restored {
				t.Errorf("restore(sanitize(%q)) = %q, want %q", test.original, restored, test.restored)
			}
			if test.ok && restored != test.original {
				t.Errorf("round trip failed: %q → %q → %q", test.original, sanitized, restored)
			}
		})
	}
}

// TestOpenAIProviderToolCallWithAccountOrders simulates a full flow with a
// mock OpenAI-compatible provider that returns tool calls for account.orders.
func TestOpenAIProviderToolCallWithAccountOrders(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var mu sync.Mutex
	accountOrdersExecuted := false

	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "account.orders", DisplayName: "订单摘要", Description: "读取订单",
		Category: "portfolio", Permission: "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		accountOrdersExecuted = true
		mu.Unlock()
		return map[string]any{"orders": []any{}, "count": 0, "checkedAt": nowString()}, nil
	})
	registry.Register(ToolDescriptor{
		Name: "portfolio.summary", DisplayName: "组合摘要", Description: "读取组合",
		Category: "portfolio", Permission: "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		return map[string]any{"accounts": []any{}, "checkedAt": nowString()}, nil
	})

	// Create a mock OpenAI-compatible provider that returns tool calls.
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// First call: return tool calls for account.orders and portfolio.summary
			resp := map[string]any{
				"id":      "chatcmpl-test",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"choices": []any{
					map[string]any{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []any{
								map[string]any{
									"id":   "call-account-orders-1",
									"type": "function",
									"function": map[string]any{
										"name":      "account-orders", // sanitized name
										"arguments": `{"query": "查看订单"}`,
									},
								},
								map[string]any{
									"id":   "call-portfolio-summary-1",
									"type": "function",
									"function": map[string]any{
										"name":      "portfolio-summary", // sanitized name
										"arguments": `{"query": "查看订单"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Subsequent calls: return text response
		resp := map[string]any{
			"id":      "chatcmpl-test-2",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "已完成分析，订单为空。",
					},
					"finish_reason": "stop",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	// Save provider pointing to mock server.
	_, err = store.SaveProvider(ctx, ProviderWriteRequest{
		ID:          "mock-openai",
		DisplayName: "Mock OpenAI",
		BaseURL:     strings.TrimSuffix(mockServer.URL, "/") + "/v1",
		Model:       "gpt-4o-mini",
		APIKey:      "test-key",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}

	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "投资分析助手",
		ProviderID:     "mock-openai",
		Tools:          []string{"account.orders", "portfolio.summary"},
		PermissionMode: PermissionModeSandboxAuto,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	runtime := newRuntimeWithRegistry(t, store, registry)

	done := make(chan ChatResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, chatErr := runtime.Chat(ctx, ChatRequest{
			AgentID: agent.ID,
			Message: "查看订单",
		})
		if chatErr != nil {
			errCh <- chatErr
			return
		}
		done <- resp
	}()

	select {
	case resp := <-done:
		t.Logf("Chat completed: status=%s toolCalls=%d", resp.Run.Status, len(resp.Run.ToolCalls))
		for _, call := range resp.Run.ToolCalls {
			t.Logf("  tool=%s status=%s error=%v", call.ToolName, call.Status, call.Error)
		}
		mu.Lock()
		if !accountOrdersExecuted {
			t.Error("account.orders was NOT executed through OpenAI provider path")
		}
		mu.Unlock()
		// Check that account.orders was actually called.
		foundAccountOrders := false
		for _, call := range resp.Run.ToolCalls {
			if call.ToolName == "account.orders" && call.Status == "SUCCEEDED" {
				foundAccountOrders = true
			}
		}
		if !foundAccountOrders {
			t.Error("account.orders tool call not found in run results")
		}
	case chatErr := <-errCh:
		t.Fatalf("Chat error: %v", chatErr)
	case <-time.After(30 * time.Second):
		t.Fatal("Chat hung for over 30 seconds — OpenAI provider path stuck on account.orders?")
	}
}

// TestOpenAIProviderWithBrokenToolName tests what happens when the provider
// returns a tool name that doesn't match any registered tool.
func TestOpenAIProviderWithBrokenToolName(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "account.orders", DisplayName: "订单摘要", Description: "读取订单",
		Category: "portfolio", Permission: "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		return map[string]any{"orders": []any{}, "count": 0}, nil
	})

	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// Return a tool call with name that doesn't restore correctly
			resp := map[string]any{
				"id":      "chatcmpl-test",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"choices": []any{
					map[string]any{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []any{
								map[string]any{
									"id":   "call-1",
									"type": "function",
									"function": map[string]any{
										// This name uses dots, which some providers might return
										"name":      "account.orders",
										"arguments": `{}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		resp := map[string]any{
			"id":      "chatcmpl-test-2",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "分析完成。",
					},
					"finish_reason": "stop",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	_, err = store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "mock-openai", DisplayName: "Mock OpenAI",
		BaseURL: strings.TrimSuffix(mockServer.URL, "/") + "/v1",
		Model:   "gpt-4o-mini", APIKey: "test-key", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}

	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: "mock-openai",
		Tools: []string{"account.orders"}, PermissionMode: PermissionModeSandboxAuto,
		Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	runtime := newRuntimeWithRegistry(t, store, registry)

	done := make(chan ChatResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, chatErr := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "查看订单"})
		if chatErr != nil {
			errCh <- chatErr
			return
		}
		done <- resp
	}()

	select {
	case resp := <-done:
		t.Logf("Chat completed: status=%s", resp.Run.Status)
		for _, call := range resp.Run.ToolCalls {
			t.Logf("  tool=%s status=%s", call.ToolName, call.Status)
		}
	case chatErr := <-errCh:
		// An error is acceptable here since the tool name might not resolve.
		t.Logf("Chat returned error (expected): %v", chatErr)
	case <-time.After(30 * time.Second):
		t.Fatal("Chat hung — tool name mismatch caused infinite loop?")
	}
}

// TestOpenAIProviderWithMultipleToolCallRounds tests what happens when the
// provider keeps requesting tool calls in a loop.
func TestOpenAIProviderWithMultipleToolCallRounds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "account.orders", DisplayName: "订单摘要", Description: "读取订单",
		Category: "portfolio", Permission: "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		return map[string]any{"orders": []any{}, "count": 0}, nil
	})

	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if callCount <= 25 {
			// Keep returning tool calls — this simulates a model that loops.
			resp := map[string]any{
				"id":      fmt.Sprintf("chatcmpl-%d", callCount),
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"choices": []any{
					map[string]any{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []any{
								map[string]any{
									"id":   fmt.Sprintf("call-%d", callCount),
									"type": "function",
									"function": map[string]any{
										"name":      "account-orders",
										"arguments": `{}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// Finally return text
		resp := map[string]any{
			"id":      fmt.Sprintf("chatcmpl-%d", callCount),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"choices": []any{
				map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "分析完成。",
					},
					"finish_reason": "stop",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	_, err = store.SaveProvider(ctx, ProviderWriteRequest{
		ID: "mock-openai", DisplayName: "Mock OpenAI",
		BaseURL: strings.TrimSuffix(mockServer.URL, "/") + "/v1",
		Model:   "gpt-4o-mini", APIKey: "test-key", Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}

	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: "mock-openai",
		Tools: []string{"account.orders"}, PermissionMode: PermissionModeSandboxAuto,
		Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	runtime := newRuntimeWithRegistry(t, store, registry)

	done := make(chan ChatResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, chatErr := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "查看订单"})
		if chatErr != nil {
			errCh <- chatErr
			return
		}
		done <- resp
	}()

	select {
	case resp := <-done:
		t.Logf("Chat completed: status=%s toolCalls=%d", resp.Run.Status, len(resp.Run.ToolCalls))
		if resp.Run.Status != RunStatusCompleted {
			t.Fatalf("run status = %s, want completed after repeated tool calls", resp.Run.Status)
		}
		if len(resp.Run.ToolCalls) <= 20 {
			t.Fatalf("tool calls = %d, want more than former limit", len(resp.Run.ToolCalls))
		}
	case chatErr := <-errCh:
		t.Fatalf("Chat error: %v", chatErr)
	case <-time.After(60 * time.Second):
		t.Fatal("Chat hung for over 60 seconds")
	}
}

func TestProviderRequestTimeoutUsesConfiguredValue(t *testing.T) {
	if got := providerRequestTimeout(Provider{}); got != DefaultProviderRequestTimeout {
		t.Fatalf("providerRequestTimeout(default) = %s, want %s", got, DefaultProviderRequestTimeout)
	}
	if got := providerRequestTimeout(Provider{RequestTimeoutMs: 240_000}); got != 240*time.Second {
		t.Fatalf("providerRequestTimeout(configured) = %s, want 240s", got)
	}
	if got := providerRequestTimeout(Provider{RequestTimeoutMs: 1_000}); got != 15*time.Second {
		t.Fatalf("providerRequestTimeout(clamped) = %s, want 15s", got)
	}
}

func TestNormalizeMessagesForProviderRepairsInterruptedToolExchange(t *testing.T) {
	messages := []openAIChatMessage{
		{Role: "user", Content: "approve the tool"},
		{Role: "assistant", ToolCalls: []openAIToolCall{
			testOpenAIToolCall("confirmation-call", "adk_request_confirmation", map[string]any{"tool": "strategy.save_definition"}),
		}},
		{Role: "assistant", Content: "sse write failed: unrelated run failed"},
		{Role: "tool", Name: "adk_request_confirmation", ToolCallID: "confirmation-call", Content: `{"confirmed":true}`},
	}

	normalized := normalizeMessagesForProvider(messages)
	assertValidOpenAIToolMessageSequence(t, normalized)
	toolIndex := indexOfToolMessage(normalized, "confirmation-call")
	if toolIndex <= 0 {
		t.Fatalf("tool message index = %d, want paired tool message", toolIndex)
	}
	previous := normalized[toolIndex-1]
	if previous.Role != "assistant" || !messageHasToolCall(previous, "confirmation-call") {
		t.Fatalf("previous message = %+v, want assistant tool call", previous)
	}
	if !messagesContainContent(normalized, "sse write failed") {
		t.Fatalf("normalized messages dropped unrelated assistant text: %+v", normalized)
	}
}

func TestOpenAICompatibleADKModelBuildChatRequestRepairsInterruptedApprovalHistory(t *testing.T) {
	model := &openAICompatibleADKModel{
		provider: Provider{Model: "test-model"},
		model:    "test-model",
	}
	request := &adkmodel.LLMRequest{
		Model: "test-model",
		Contents: []*genai.Content{
			genai.NewContentFromText("approve the strategy save", genai.RoleUser),
			genai.NewContentFromParts([]*genai.Part{{
				FunctionCall: &genai.FunctionCall{
					ID: "approval-call", Name: "adk_request_confirmation",
					Args: map[string]any{"tool": "strategy.save_definition"},
				},
			}}, genai.RoleModel),
			genai.NewContentFromText("sse write failed: unrelated run failed", genai.RoleModel),
			genai.NewContentFromParts([]*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{
					ID: "approval-call", Name: "adk_request_confirmation",
					Response: map[string]any{"confirmed": true},
				},
			}}, genai.RoleUser),
		},
	}

	payload := model.buildChatRequest(request, false)
	assertValidOpenAIToolMessageSequence(t, payload.Messages)
	toolIndex := indexOfToolMessage(payload.Messages, "approval-call")
	if toolIndex <= 0 {
		t.Fatalf("approval tool response missing from payload: %+v", payload.Messages)
	}
	if !messageHasToolCall(payload.Messages[toolIndex-1], "approval-call") {
		t.Fatalf("approval response previous payload message = %+v, want matching assistant tool call", payload.Messages[toolIndex-1])
	}
	if !messagesContainContent(payload.Messages, "sse write failed") {
		t.Fatalf("payload dropped unrelated assistant message: %+v", payload.Messages)
	}
}

func TestNormalizeMessagesForProviderDropsOrphanToolMessage(t *testing.T) {
	normalized := normalizeMessagesForProvider([]openAIChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "tool", Name: "missing", ToolCallID: "missing-call", Content: `{"ok":true}`},
	})

	assertValidOpenAIToolMessageSequence(t, normalized)
	if indexOfToolMessage(normalized, "missing-call") >= 0 {
		t.Fatalf("orphan tool message was retained: %+v", normalized)
	}
}

func TestNormalizeMessagesForProviderPairsMultipleInterruptedToolResponses(t *testing.T) {
	messages := []openAIChatMessage{
		{Role: "assistant", Content: "calling tools", ToolCalls: []openAIToolCall{
			testOpenAIToolCall("call-a", "market.snapshot", map[string]any{"symbol": "US.TME"}),
			testOpenAIToolCall("call-b", "strategy.definitions", map[string]any{}),
		}},
		{Role: "assistant", Content: "unrelated text between calls and responses"},
		{Role: "tool", Name: "strategy.definitions", ToolCallID: "call-b", Content: `{"definitions":[]}`},
		{Role: "tool", Name: "market.snapshot", ToolCallID: "call-a", Content: `{"price":12.3}`},
	}

	normalized := normalizeMessagesForProvider(messages)
	assertValidOpenAIToolMessageSequence(t, normalized)
	for _, id := range []string{"call-a", "call-b"} {
		toolIndex := indexOfToolMessage(normalized, id)
		if toolIndex <= 0 {
			t.Fatalf("tool %s index = %d, want paired tool message in %+v", id, toolIndex, normalized)
		}
		if !messageHasToolCall(normalized[toolIndex-1], id) {
			t.Fatalf("tool %s previous message = %+v, want matching assistant call", id, normalized[toolIndex-1])
		}
	}
}

func TestTrimMessagesForProviderKeepsToolExchangeGroupsAtomic(t *testing.T) {
	messages := []openAIChatMessage{
		{Role: "system", Content: "system"},
		{Role: "assistant", Content: strings.Repeat("old", 200), ToolCalls: []openAIToolCall{
			testOpenAIToolCall("old-call", "market.candles", map[string]any{"symbol": "US.TME"}),
		}},
		{Role: "tool", Name: "market.candles", ToolCallID: "old-call", Content: strings.Repeat("old-result", 200)},
		{Role: "user", Content: "latest question"},
	}

	trimmed := trimMessagesForProvider(messages, 300)
	assertValidOpenAIToolMessageSequence(t, trimmed)
	if indexOfToolMessage(trimmed, "old-call") >= 0 && !containsAssistantToolCall(trimmed, "old-call") {
		t.Fatalf("trimmed messages split tool exchange: %+v", trimmed)
	}
	if !messagesContainContent(trimmed, "latest question") {
		t.Fatalf("trimmed messages dropped latest fitting user message: %+v", trimmed)
	}
}

func TestTrimMessagesForProviderTruncatesOversizedToolResponseWithoutLosingPair(t *testing.T) {
	messages := []openAIChatMessage{
		{Role: "assistant", ToolCalls: []openAIToolCall{
			testOpenAIToolCall("large-call", "market.candles", map[string]any{"symbol": "US.TME"}),
		}},
		{Role: "tool", Name: "market.candles", ToolCallID: "large-call", Content: strings.Repeat("x", 50000)},
	}

	trimmed := trimMessagesForProvider(messages, 100000)
	assertValidOpenAIToolMessageSequence(t, trimmed)
	toolIndex := indexOfToolMessage(trimmed, "large-call")
	if toolIndex <= 0 {
		t.Fatalf("large tool response missing from trimmed messages: %+v", trimmed)
	}
	if !messageHasToolCall(trimmed[toolIndex-1], "large-call") {
		t.Fatalf("large tool response lost assistant pair: %+v", trimmed)
	}
	if !strings.Contains(trimmed[toolIndex].Content, "...(truncated)") {
		t.Fatalf("large tool response was not truncated")
	}
	if trimmed[toolIndex].ToolCallID != "large-call" || trimmed[toolIndex].Name != "market.candles" {
		t.Fatalf("tool metadata changed after trim: %+v", trimmed[toolIndex])
	}
}

func testOpenAIToolCall(id string, name string, args map[string]any) openAIToolCall {
	rawArgs, _ := json.Marshal(args)
	call := openAIToolCall{ID: id, Type: "function"}
	call.Function.Name = name
	call.Function.Arguments = string(rawArgs)
	return call
}

func assertValidOpenAIToolMessageSequence(t *testing.T, messages []openAIChatMessage) {
	t.Helper()
	active := map[string]struct{}{}
	for index, message := range messages {
		switch message.Role {
		case "assistant":
			active = map[string]struct{}{}
			for _, call := range message.ToolCalls {
				if call.ID != "" {
					active[call.ID] = struct{}{}
				}
			}
		case "tool":
			if _, ok := active[message.ToolCallID]; !ok {
				t.Fatalf("message %d is orphan tool response %q in sequence %+v", index, message.ToolCallID, messages)
			}
			delete(active, message.ToolCallID)
		default:
			active = map[string]struct{}{}
		}
	}
}

func indexOfToolMessage(messages []openAIChatMessage, id string) int {
	for index, message := range messages {
		if message.Role == "tool" && message.ToolCallID == id {
			return index
		}
	}
	return -1
}

func messageHasToolCall(message openAIChatMessage, id string) bool {
	for _, call := range message.ToolCalls {
		if call.ID == id {
			return true
		}
	}
	return false
}

func containsAssistantToolCall(messages []openAIChatMessage, id string) bool {
	for _, message := range messages {
		if message.Role == "assistant" && messageHasToolCall(message, id) {
			return true
		}
	}
	return false
}

func messagesContainContent(messages []openAIChatMessage, text string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, text) {
			return true
		}
	}
	return false
}
