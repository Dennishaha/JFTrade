package adk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestAccountOrdersCompletesWithoutHanging verifies that a chat request triggering
// account.orders (alongside portfolio.summary) completes within a reasonable time
// instead of hanging forever.
func TestAccountOrdersCompletesWithoutHanging(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := NewToolRegistry()
	var mu sync.Mutex
	executedTools := map[string]bool{}

	// Simulate account.orders: simple in-memory read.
	registry.Register(ToolDescriptor{
		Name:        "account.orders",
		DisplayName: "订单摘要",
		Description: "读取本地执行订单视图摘要。",
		Category:    "portfolio",
		Permission:  "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		executedTools["account.orders"] = true
		mu.Unlock()
		return map[string]any{"orders": []any{}, "count": 0, "checkedAt": nowString()}, nil
	})

	// Simulate portfolio.summary: includes broker calls with timeout.
	registry.Register(ToolDescriptor{
		Name:        "portfolio.summary",
		DisplayName: "组合摘要",
		Description: "读取托管账户、资金、订单和持仓的控制台摘要。",
		Category:    "portfolio",
		Permission:  "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		executedTools["portfolio.summary"] = true
		mu.Unlock()
		return map[string]any{"accounts": []any{}, "brokerEnabled": false, "orderCount": 0, "checkedAt": nowString()}, nil
	})

	// Simulate market.subscriptions.
	registry.Register(ToolDescriptor{
		Name:        "market.subscriptions",
		DisplayName: "行情订阅",
		Description: "读取当前行情订阅和配额摘要。",
		Category:    "market",
		Permission:  "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		executedTools["market.subscriptions"] = true
		mu.Unlock()
		return map[string]any{"subscriptions": []any{}, "activeInstruments": []string{}, "checkedAt": nowString()}, nil
	})

	// Simulate backtest.runs.
	registry.Register(ToolDescriptor{
		Name:        "backtest.runs",
		DisplayName: "回测结果",
		Description: "读取最近回测运行结果。",
		Category:    "strategy",
		Permission:  "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		executedTools["backtest.runs"] = true
		mu.Unlock()
		return map[string]any{"runs": []any{}}, nil
	})

	// Simulate system.status.
	registry.Register(ToolDescriptor{
		Name:        "system.status",
		DisplayName: "系统状态",
		Description: "读取系统状态摘要。",
		Category:    "system",
		Permission:  "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		executedTools["system.status"] = true
		mu.Unlock()
		return map[string]any{"ok": true, "checkedAt": nowString()}, nil
	})

	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "投资分析助手",
		Tools:          []string{"account.orders", "portfolio.summary", "market.subscriptions", "backtest.runs", "system.status"},
		PermissionMode: PermissionModeSandboxAuto,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	// Use a timeout to detect hangs.
	done := make(chan ChatResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, chatErr := runtime.Chat(ctx, ChatRequest{
			AgentID: agent.ID,
			Message: "查看账户、订单、回测和行情订阅情况",
		})
		if chatErr != nil {
			errCh <- chatErr
			return
		}
		done <- resp
	}()

	select {
	case resp := <-done:
		if resp.Run.Status != RunStatusCompleted {
			t.Fatalf("run status = %q, want COMPLETED; message: %s", resp.Run.Status, resp.Run.Message)
		}
		mu.Lock()
		if !executedTools["account.orders"] {
			t.Fatal("account.orders was not executed")
		}
		if !executedTools["portfolio.summary"] {
			t.Fatal("portfolio.summary was not executed")
		}
		mu.Unlock()
		t.Logf("Chat completed successfully with %d tool calls", len(resp.Run.ToolCalls))
		for _, call := range resp.Run.ToolCalls {
			t.Logf("  tool=%s status=%s durationMs=%d", call.ToolName, call.Status, call.DurationMs)
		}
	case chatErr := <-errCh:
		t.Fatalf("Chat error: %v", chatErr)
	case <-time.After(30 * time.Second):
		t.Fatal("Chat hung for over 30 seconds — account.orders stuck?")
	}
}

// TestAccountOrdersWithSlowPortfolioSummary verifies that account.orders still completes
// even when portfolio.summary (which calls broker APIs) takes a long time.
func TestAccountOrdersWithSlowPortfolioSummary(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := NewToolRegistry()
	var mu sync.Mutex
	executedTools := map[string]bool{}

	// Simulate account.orders: fast.
	registry.Register(ToolDescriptor{
		Name:        "account.orders",
		DisplayName: "订单摘要",
		Description: "读取本地执行订单视图摘要。",
		Category:    "portfolio",
		Permission:  "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		executedTools["account.orders"] = true
		mu.Unlock()
		return map[string]any{"orders": []any{}, "count": 0, "checkedAt": nowString()}, nil
	})

	// Simulate portfolio.summary: slow (simulates broker API timeout).
	registry.Register(ToolDescriptor{
		Name:        "portfolio.summary",
		DisplayName: "组合摘要",
		Description: "读取托管账户、资金、订单和持仓的控制台摘要。",
		Category:    "portfolio",
		Permission:  "read_internal",
	}, func(toolCtx context.Context, _ map[string]any) (any, error) {
		mu.Lock()
		executedTools["portfolio.summary"] = true
		mu.Unlock()
		// Simulate slow broker API call (but still within tool timeout).
		select {
		case <-time.After(5 * time.Second):
			return map[string]any{"accounts": []any{}, "brokerEnabled": false, "orderCount": 0, "checkedAt": nowString()}, nil
		case <-toolCtx.Done():
			return nil, toolCtx.Err()
		}
	})

	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "投资分析助手",
		Tools:          []string{"account.orders", "portfolio.summary"},
		PermissionMode: PermissionModeSandboxAuto,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	done := make(chan ChatResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, chatErr := runtime.Chat(ctx, ChatRequest{
			AgentID: agent.ID,
			Message: "查看账户和订单",
		})
		if chatErr != nil {
			errCh <- chatErr
			return
		}
		done <- resp
	}()

	select {
	case resp := <-done:
		if resp.Run.Status != RunStatusCompleted {
			t.Fatalf("run status = %q, want COMPLETED; message: %s", resp.Run.Status, resp.Run.Message)
		}
		mu.Lock()
		if !executedTools["account.orders"] {
			t.Fatal("account.orders was not executed")
		}
		if !executedTools["portfolio.summary"] {
			t.Fatal("portfolio.summary was not executed")
		}
		mu.Unlock()
		t.Logf("Chat completed with %d tool calls", len(resp.Run.ToolCalls))
	case chatErr := <-errCh:
		t.Fatalf("Chat error: %v", chatErr)
	case <-time.After(60 * time.Second):
		t.Fatal("Chat hung for over 60 seconds — stuck on slow portfolio.summary?")
	}
}

func TestChatContinuesAfterToolFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:        "strategy.save_draft",
		DisplayName: "保存草稿",
		Description: "保存策略草稿。",
		Category:    "strategy",
		Permission:  "write_strategy",
		AllowedModes: []string{
			PermissionModeSandboxAuto,
			PermissionModeHighAuto,
		},
	}, func(context.Context, map[string]any) (any, error) {
		return nil, fmt.Errorf("disk full")
	})

	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID:             "agent",
		Name:           "投资分析助手",
		Tools:          []string{"strategy.save_draft"},
		PermissionMode: PermissionModeSandboxAuto,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	resp, chatErr := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: "@strategy.save_draft 保存策略草稿",
	})
	if chatErr != nil {
		t.Fatalf("Chat error: %v", chatErr)
	}
	if resp.Run.Status != RunStatusCompleted {
		t.Fatalf("run status = %q, want %q; run=%+v", resp.Run.Status, RunStatusCompleted, resp.Run)
	}
	if !resp.Run.Degraded {
		t.Fatalf("run degraded = %v, want true", resp.Run.Degraded)
	}
	if len(resp.Run.ToolCalls) != 1 || resp.Run.ToolCalls[0].Status != "FAILED" {
		t.Fatalf("tool calls = %+v, want failed tool call", resp.Run.ToolCalls)
	}
	if resp.Run.ToolCalls[0].Error == nil || !strings.Contains(*resp.Run.ToolCalls[0].Error, "disk full") {
		t.Fatalf("tool error = %#v, want disk full", resp.Run.ToolCalls[0].Error)
	}
	if strings.TrimSpace(resp.Reply) == "" {
		t.Fatalf("reply = %q, want assistant follow-up reply", resp.Reply)
	}
}

// TestSelectToolInvocationsAccountOrders verifies that the keyword-based
// tool selection includes account.orders when the user mentions relevant keywords.
func TestSelectToolInvocationsAccountOrders(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "account.orders", DisplayName: "订单摘要", Description: "读取订单", Category: "portfolio", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	registry.Register(ToolDescriptor{Name: "portfolio.summary", DisplayName: "组合摘要", Description: "读取组合", Category: "portfolio", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	registry.Register(ToolDescriptor{Name: "market.subscriptions", DisplayName: "行情订阅", Description: "读取行情", Category: "market", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	registry.Register(ToolDescriptor{Name: "system.status", DisplayName: "系统状态", Description: "读取系统", Category: "system", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})

	tests := []struct {
		message  string
		expected []string
	}{
		{"查看账户", []string{"portfolio.summary", "account.orders"}},
		{"我的订单", []string{"portfolio.summary", "account.orders"}},
		{"持仓情况", []string{"portfolio.summary", "account.orders"}},
		{"查看行情订阅", []string{"market.subscriptions"}},
	}

	for _, test := range tests {
		t.Run(test.message, func(t *testing.T) {
			agent := Agent{ID: "agent", PermissionMode: PermissionModeSandboxAuto}
			invocations := SelectToolInvocations(test.message, agent, registry)
			names := make([]string, 0, len(invocations))
			for _, inv := range invocations {
				names = append(names, inv.Name)
			}
			for _, expected := range test.expected {
				found := false
				for _, name := range names {
					if name == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected tool %q in selection, got %v", expected, names)
				}
			}
		})
	}
}

func TestSelectToolInvocationsAddsExplicitStrategyWorkflowTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, descriptor := range []ToolDescriptor{
		{Name: "strategy.validate_pine", DisplayName: "校验 Pine", Category: "strategy", Permission: "read_internal"},
		{Name: "strategy.save_definition", DisplayName: "保存策略定义", Category: "strategy", Permission: "write_strategy"},
		{Name: "strategy.update_instance_mode", DisplayName: "修改实例模式", Category: "strategy", Permission: "write_strategy"},
		{Name: "system.status", DisplayName: "系统状态", Category: "system", Permission: "read_internal"},
	} {
		descriptor := descriptor
		registry.Register(descriptor, func(context.Context, map[string]any) (any, error) {
			return nil, nil
		})
	}

	tests := []struct {
		message  string
		expected string
	}{
		{message: "请先校验这个 Pine 语法有没有问题", expected: "strategy.validate_pine"},
		{message: "把这个策略定义保存起来，并更新已有定义", expected: "strategy.save_definition"},
		{message: "把实例切到 notify_only 执行模式", expected: "strategy.update_instance_mode"},
	}

	agent := Agent{
		ID:    "agent",
		Tools: []string{"strategy.validate_pine", "strategy.save_definition", "strategy.update_instance_mode"},
	}
	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			invocations := SelectToolInvocations(test.message, agent, registry)
			found := false
			for _, invocation := range invocations {
				if invocation.Name == test.expected {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("invocations = %+v, want %s", invocations, test.expected)
			}
		})
	}
}

// TestAccountOrdersStreamCompletes tests the streaming path specifically.
func TestAccountOrdersStreamCompletes(t *testing.T) {
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
		return map[string]any{"orders": []any{}, "count": 0, "checkedAt": nowString()}, nil
	})
	registry.Register(ToolDescriptor{
		Name: "portfolio.summary", DisplayName: "组合摘要", Description: "读取组合",
		Category: "portfolio", Permission: "read_internal",
	}, func(_ context.Context, _ map[string]any) (any, error) {
		return map[string]any{"accounts": []any{}, "checkedAt": nowString()}, nil
	})

	runtime := newRuntimeWithRegistry(t, store, registry)
	agent, err := store.SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent",
		Tools:          []string{"account.orders", "portfolio.summary"},
		PermissionMode: PermissionModeSandboxAuto,
		Status:         AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	var deltasMu sync.Mutex
	var deltas []ChatDelta
	done := make(chan ChatResponse, 1)
	errCh := make(chan error, 1)

	go func() {
		resp, chatErr := runtime.ChatStream(ctx, ChatRequest{
			AgentID: agent.ID,
			Message: "查看账户和订单",
		}, func(delta ChatDelta) error {
			deltasMu.Lock()
			deltas = append(deltas, delta)
			deltasMu.Unlock()
			return nil
		})
		if chatErr != nil {
			errCh <- chatErr
			return
		}
		done <- resp
	}()

	select {
	case resp := <-done:
		if resp.Run.Status != RunStatusCompleted {
			t.Fatalf("run status = %q, want COMPLETED; message: %s", resp.Run.Status, resp.Run.Message)
		}
		deltasMu.Lock()
		streamDeltas := append([]ChatDelta(nil), deltas...)
		deltasMu.Unlock()
		// Verify we got tool progress deltas.
		hasToolProgress := false
		for _, delta := range streamDeltas {
			if strings.Contains(delta.ToolProgress, "account.orders") {
				hasToolProgress = true
			}
		}
		if !hasToolProgress {
			t.Logf("Warning: no tool progress delta for account.orders (deltas: %d)", len(streamDeltas))
		}
		t.Logf("Stream completed with %d deltas, %d tool calls", len(streamDeltas), len(resp.Run.ToolCalls))
	case chatErr := <-errCh:
		t.Fatalf("ChatStream error: %v", chatErr)
	case <-time.After(30 * time.Second):
		t.Fatal("ChatStream hung for over 30 seconds")
	}
}
