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

		if callCount <= 25 { // More than MaxToolCallsPerRun=20
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
		// The run should fail or complete with limited tool calls due to iteration guard
		if resp.Run.Status == RunStatusFailed {
			t.Logf("Run failed as expected due to infinite loop guard: %s", resp.Run.Message)
		}
	case chatErr := <-errCh:
		// Error is expected — the iteration guard or tool limit should kick in
		t.Logf("Chat error (expected for loop): %v", chatErr)
	case <-time.After(60 * time.Second):
		t.Fatal("Chat hung for over 60 seconds — infinite tool call loop not prevented!")
	}
}
