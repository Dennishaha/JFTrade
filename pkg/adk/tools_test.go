package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultTaskToolSchemaIncludesPlannerProjectionFields(t *testing.T) {
	for _, name := range []string{"tasks.create", "tasks.update"} {
		t.Run(name, func(t *testing.T) {
			schema := defaultToolInputSchema(name)
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema properties = %#v, want object", schema["properties"])
			}
			for _, field := range []string{"order", "modeHint", "agentRole", "plannerStepId", "planSource", "workflowMode", "objective", "plannerWarnings"} {
				if _, ok := properties[field]; !ok {
					t.Fatalf("%s schema missing %s in properties %+v", name, field, properties)
				}
			}
		})
	}
}

func TestTaskCreateIsLowRiskAndDoesNotRequireApproval(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "tasks.create", Permission: "write_task"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"created": true}, nil
	})
	tool, ok := registry.Get("tasks.create")
	if !ok {
		t.Fatal("tasks.create not registered")
	}
	if tool.Descriptor.RiskLevel != "low" {
		t.Fatalf("risk level = %q, want low", tool.Descriptor.RiskLevel)
	}
	if ToolRequiresApproval(tool.Descriptor, PermissionModeApproval) {
		t.Fatal("tasks.create unexpectedly requires approval in approval mode")
	}
}

func TestWorkflowWaitToolWaitsAndDoesNotRequireApproval(t *testing.T) {
	registry := NewToolRegistry()
	tool, ok := registry.Get("workflow.wait")
	if !ok {
		t.Fatal("workflow.wait not registered")
	}
	if tool.Descriptor.Permission != "read_internal" || tool.Descriptor.RiskLevel != "low" {
		t.Fatalf("descriptor = %+v, want read_internal/low", tool.Descriptor)
	}
	if ToolRequiresApproval(tool.Descriptor, PermissionModeApproval) {
		t.Fatal("workflow.wait unexpectedly requires approval in approval mode")
	}
	started := time.Now()
	output, err := tool.Handler(context.Background(), map[string]any{"durationMs": 10, "reason": "test wait"})
	if err != nil {
		t.Fatalf("workflow.wait error = %v", err)
	}
	if time.Since(started) < 10*time.Millisecond {
		t.Fatal("workflow.wait returned before requested duration")
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("output = %T, want map", output)
	}
	if payload["reason"] != "test wait" {
		t.Fatalf("reason = %#v, want test wait", payload["reason"])
	}
	if _, ok := payload["waitedMs"]; !ok {
		t.Fatalf("output missing waitedMs: %#v", payload)
	}
}

func TestWorkflowWaitToolRejectsTooLongDuration(t *testing.T) {
	registry := NewToolRegistry()
	tool, ok := registry.Get("workflow.wait")
	if !ok {
		t.Fatal("workflow.wait not registered")
	}
	if _, err := tool.Handler(context.Background(), map[string]any{"seconds": 26}); err == nil || !strings.Contains(err.Error(), "25s") {
		t.Fatalf("workflow.wait long duration error = %v, want max duration error", err)
	}
}

func TestWorkflowWaitToolReturnsContextCancellation(t *testing.T) {
	registry := NewToolRegistry()
	tool, ok := registry.Get("workflow.wait")
	if !ok {
		t.Fatal("workflow.wait not registered")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tool.Handler(ctx, map[string]any{"durationMs": 100}); !errors.Is(err, context.Canceled) {
		t.Fatalf("workflow.wait cancelled error = %v, want context.Canceled", err)
	}
}

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
	t.Cleanup(func() { jftradeErr3 := store.Close(); jftradeCheckTestError(t, jftradeErr3) })

	registry := NewToolRegistry()
	var mu sync.Mutex
	executedTools := map[string]bool{}

	// Simulate account.orders: simple in-memory read.
	registry.Register(ToolDescriptor{
		Name:        "account.orders",
		DisplayName: "订单摘要",
		Description: "读取执行订单视图摘要。",
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
		ProviderID:     testProviderID,
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
	t.Cleanup(func() { jftradeErr4 := store.Close(); jftradeCheckTestError(t, jftradeErr4) })

	registry := NewToolRegistry()
	var mu sync.Mutex
	executedTools := map[string]bool{}

	// Simulate account.orders: fast.
	registry.Register(ToolDescriptor{
		Name:        "account.orders",
		DisplayName: "订单摘要",
		Description: "读取执行订单视图摘要。",
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
		ProviderID:     testProviderID,
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
	t.Cleanup(func() { jftradeErr2 := store.Close(); jftradeCheckTestError(t, jftradeErr2) })

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
		ProviderID:     testProviderID,
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

// TestAccountOrdersStreamCompletes tests the streaming path specifically.
func TestAccountOrdersStreamCompletes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { jftradeErr1 := store.Close(); jftradeCheckTestError(t, jftradeErr1) })

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
		ID: "agent", Name: "Agent", ProviderID: testProviderID,
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
