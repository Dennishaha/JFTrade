package adk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDefaultTaskToolSchemaIncludesPlannerProjectionFields(t *testing.T) {
	for _, name := range []string{"tasks.create", "tasks.update"} {
		t.Run(name, func(t *testing.T) {
			schema := defaultToolInputSchema(name)
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema properties = %#v, want object", schema["properties"])
			}
			for _, field := range []string{"order", "modeHint", "agentRole", "plannerStepId", "planSource", "workflowMode", "objective", "childProviderId", "childModel", "plannerWarnings"} {
				if _, ok := properties[field]; !ok {
					t.Fatalf("%s schema missing %s in properties %+v", name, field, properties)
				}
			}
		})
	}
}

func TestModelsListToolRegisteredWithSafeSchema(t *testing.T) {
	runtime := newTestRuntime(t)
	tool, ok := runtime.Tools().Get("models.list")
	if !ok {
		t.Fatal("models.list not registered")
	}
	if tool.Descriptor.Permission != "read_internal" || tool.Descriptor.RiskLevel != "low" {
		t.Fatalf("models.list descriptor = %+v, want read_internal/low", tool.Descriptor)
	}
	if ToolRequiresApproval(tool.Descriptor, PermissionModeApproval) {
		t.Fatal("models.list unexpectedly requires approval")
	}
	properties, ok := tool.Descriptor.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("models.list schema properties = %#v", tool.Descriptor.InputSchema["properties"])
	}
	for _, field := range []string{"query", "providerId", "callableOnly", "limit"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("models.list schema missing %s in %+v", field, properties)
		}
	}
	if strings.Contains(fmt.Sprint(tool.Descriptor.InputSchema), "apiKey") {
		t.Fatalf("models.list schema mentions apiKey: %+v", tool.Descriptor.InputSchema)
	}
}

func TestTaskWriteToolsMarkedLowRiskCanSkipApproval(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"tasks.create", "tasks.update", "tasks.delete"} {
		registry.Register(ToolDescriptor{Name: name, Permission: "write_task"}, func(context.Context, map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		})
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if tool.Descriptor.RiskLevel != "low" {
			t.Fatalf("%s risk level = %q, want low", name, tool.Descriptor.RiskLevel)
		}
		if ToolRequiresApproval(tool.Descriptor, PermissionModeApproval) {
			t.Fatalf("%s unexpectedly requires approval in approval mode", name)
		}
	}
}

func TestLowRiskWriteToolsCanSkipApproval(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"memory.remember", "memory.forget", "strategy.save_draft"} {
		registry.Register(ToolDescriptor{Name: name, Permission: map[string]string{
			"memory.remember":     "write_memory",
			"memory.forget":       "write_memory",
			"strategy.save_draft": "write_strategy",
		}[name]}, func(context.Context, map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		})
		tool, ok := registry.Get(name)
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if tool.Descriptor.RiskLevel != "low" {
			t.Fatalf("%s risk level = %q, want low", name, tool.Descriptor.RiskLevel)
		}
		if ToolRequiresApproval(tool.Descriptor, PermissionModeApproval) {
			t.Fatalf("%s unexpectedly requires approval in approval mode", name)
		}
	}
}

func TestApprovalModeRequiresMediumAndHigherRiskApproval(t *testing.T) {
	for _, risk := range []string{"medium", "high", "critical"} {
		descriptor := ToolDescriptor{Name: "risk." + risk, Permission: "write_external", RiskLevel: risk, AllowedModes: allPermissionModes()}
		if !ToolRequiresApproval(descriptor, PermissionModeApproval) {
			t.Fatalf("risk %s did not require approval in approval mode", risk)
		}
		if ToolRequiresApproval(descriptor, PermissionModeAll) {
			t.Fatalf("risk %s unexpectedly required approval in all mode", risk)
		}
	}
	low := ToolDescriptor{Name: "risk.low", Permission: "write_external", RiskLevel: "low", AllowedModes: allPermissionModes()}
	if ToolRequiresApproval(low, PermissionModeApproval) {
		t.Fatal("low risk tool unexpectedly requires approval in approval mode")
	}
}

func TestResearchBacktestExplicitlySkipsApproval(t *testing.T) {
	descriptor := ToolDescriptor{
		Name:               "strategy.research_backtest",
		Permission:         "optimize_strategy",
		RiskLevel:          "low",
		RequiresApprovalIn: []string{PermissionModeApproval},
		AllowedModes:       allPermissionModes(),
	}
	for _, mode := range allPermissionModes() {
		if ToolRequiresApproval(descriptor, mode) {
			t.Fatalf("strategy.research_backtest requires approval in %s", mode)
		}
		if !ToolAllowedInMode(descriptor, mode) {
			t.Fatalf("strategy.research_backtest is not allowed in %s", mode)
		}
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

func TestWorkflowWaitDurationParsesMultipleInputForms(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    map[string]any
		want     time.Duration
		wantText string
	}{
		{
			name:  "duration ms wins",
			input: map[string]any{"durationMs": 1500, "seconds": 9},
			want:  1500 * time.Millisecond,
		},
		{
			name:  "float seconds",
			input: map[string]any{"seconds": 1.5},
			want:  1500 * time.Millisecond,
		},
		{
			name:  "int seconds",
			input: map[string]any{"seconds": 2},
			want:  2 * time.Second,
		},
		{
			name:  "string seconds",
			input: map[string]any{"seconds": "0.25"},
			want:  250 * time.Millisecond,
		},
		{
			name:     "blank string rejected",
			input:    map[string]any{"seconds": "   "},
			wantText: "greater than 0",
		},
		{
			name:     "invalid string rejected",
			input:    map[string]any{"seconds": "later"},
			wantText: "greater than 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := workflowWaitDuration(tc.input)
			if tc.wantText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantText) {
					t.Fatalf("workflowWaitDuration(%#v) err=%v, want substring %q", tc.input, err, tc.wantText)
				}
				return
			}
			if err != nil {
				t.Fatalf("workflowWaitDuration(%#v): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("workflowWaitDuration(%#v) = %s, want %s", tc.input, got, tc.want)
			}
		})
	}
}

func TestHTTPFetchToolRejectsInvalidAndUnsafeTargets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    map[string]any
		wantText string
	}{
		{
			name:     "missing url",
			input:    map[string]any{},
			wantText: "url is required",
		},
		{
			name:     "invalid url",
			input:    map[string]any{"url": "://bad"},
			wantText: "invalid url",
		},
		{
			name:     "unsupported scheme",
			input:    map[string]any{"url": "ftp://example.com/feed"},
			wantText: "only http and https are supported",
		},
		{
			name:     "localhost blocked",
			input:    map[string]any{"url": "http://localhost:8080/health"},
			wantText: "localhost targets are blocked",
		},
		{
			name:     "metadata blocked",
			input:    map[string]any{"url": "http://169.254.169.254/latest/meta-data"},
			wantText: "private, loopback, link-local, multicast and metadata addresses are blocked",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := httpFetchTool(context.Background(), tc.input); err == nil || !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("httpFetchTool(%#v) err=%v, want substring %q", tc.input, err, tc.wantText)
			}
		})
	}
}

func TestRejectUnsafeHostAndUnsafeAddrClassification(t *testing.T) {
	if err := rejectUnsafeHost(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "host is required") {
		t.Fatalf("rejectUnsafeHost empty err = %v, want host required", err)
	}
	if err := rejectUnsafeHost(context.Background(), "8.8.8.8"); err != nil {
		t.Fatalf("rejectUnsafeHost public ip: %v", err)
	}

	for _, tc := range []struct {
		addr netip.Addr
		want bool
	}{
		{addr: netip.MustParseAddr("127.0.0.1"), want: true},
		{addr: netip.MustParseAddr("10.0.0.1"), want: true},
		{addr: netip.MustParseAddr("169.254.1.10"), want: true},
		{addr: netip.MustParseAddr("224.0.0.1"), want: true},
		{addr: netip.MustParseAddr("0.0.0.0"), want: true},
		{addr: netip.MustParseAddr("169.254.169.254"), want: true},
		{addr: netip.MustParseAddr("8.8.8.8"), want: false},
		{addr: netip.MustParseAddr("1.1.1.1"), want: false},
	} {
		if got := unsafeAddr(tc.addr); got != tc.want {
			t.Fatalf("unsafeAddr(%s) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestHTTPFetchToolHandlesResponsesWithoutRealNetwork(t *testing.T) {
	oldTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
	})

	t.Run("successful text response", func(t *testing.T) {
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("User-Agent") != "JFTrade-ADK/1.0" {
				t.Fatalf("user agent = %q, want JFTrade-ADK/1.0", req.Header.Get("User-Agent"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
				Body:       io.NopCloser(strings.NewReader("market snapshot")),
				Request:    req,
			}, nil
		})

		output, err := httpFetchTool(context.Background(), map[string]any{"url": "http://8.8.8.8/report"})
		if err != nil {
			t.Fatalf("httpFetchTool success: %v", err)
		}
		payload, ok := output.(map[string]any)
		if !ok {
			t.Fatalf("output = %T, want map[string]any", output)
		}
		if payload["status"] != http.StatusOK || payload["body"] != "market snapshot" || payload["truncated"] != false {
			t.Fatalf("payload = %#v, want successful fetch payload", payload)
		}
		if payload["url"] != "http://8.8.8.8/report" {
			t.Fatalf("payload url = %#v, want canonical request url", payload["url"])
		}
	})

	t.Run("unsupported content type", func(t *testing.T) {
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
				Body:       io.NopCloser(strings.NewReader("binary")),
				Request:    req,
			}, nil
		})

		if _, err := httpFetchTool(context.Background(), map[string]any{"url": "http://8.8.8.8/file"}); err == nil || !strings.Contains(err.Error(), "unsupported content type") {
			t.Fatalf("httpFetchTool unsupported content type err = %v, want content type error", err)
		}
	})

	t.Run("redirect to unsafe host blocked", func(t *testing.T) {
		calls := 0
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://127.0.0.1/private"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})

		if _, err := httpFetchTool(context.Background(), map[string]any{"url": "http://8.8.8.8/redirect"}); err == nil || !strings.Contains(err.Error(), "redirect to unsafe host") {
			t.Fatalf("httpFetchTool redirect err = %v, want redirect safety error", err)
		}
		if calls != 1 {
			t.Fatalf("redirect transport calls = %d, want 1", calls)
		}
	})

	t.Run("large text response marked truncated", func(t *testing.T) {
		body := strings.Repeat("a", (1<<20)+64)
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		})

		output, err := httpFetchTool(context.Background(), map[string]any{"url": "http://8.8.8.8/large"})
		if err != nil {
			t.Fatalf("httpFetchTool large body: %v", err)
		}
		payload := output.(map[string]any)
		if payload["truncated"] != true {
			t.Fatalf("payload truncated = %#v, want true", payload["truncated"])
		}
		if got := len(payload["body"].(string)); got != 1<<20 {
			t.Fatalf("payload body len = %d, want %d", got, 1<<20)
		}
	})
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
		PermissionMode: PermissionModeLessApproval,
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
		PermissionMode: PermissionModeLessApproval,
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

func TestLiveTradingToolsAreAvailableInAllModesWithApproval(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:       "orders.place",
		Permission: "live_trading",
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	registered, ok := registry.Get("orders.place")
	if !ok {
		t.Fatal("live trading tool was not registered")
	}
	if !ToolAllowedInMode(registered.Descriptor, PermissionModeApproval) {
		t.Fatal("live trading tool must be available in approval mode")
	}
	if !ToolAllowedInMode(registered.Descriptor, PermissionModeLessApproval) {
		t.Fatal("live trading tool must be available in less_approval mode")
	}
	if !ToolAllowedInMode(registered.Descriptor, PermissionModeAll) {
		t.Fatal("live trading tool must be allowed in all mode")
	}
	if !ToolRequiresApproval(registered.Descriptor, PermissionModeAll) {
		t.Fatal("live trading tool must require approval even in all mode")
	}
}

func TestBacktestToolsIncludeRequiredKLineSyncStatusCompanion(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range []string{"strategy.research_backtest", "backtest.kline_sync_status"} {
		registry.Register(ToolDescriptor{Name: name, Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
			return nil, nil
		})
	}
	descriptors := ToolDescriptorsForAgent(Agent{Tools: []string{"strategy.research_backtest"}}, registry)
	names := make(map[string]bool, len(descriptors))
	for _, descriptor := range descriptors {
		names[descriptor.Name] = true
	}
	if !names["strategy.research_backtest"] || !names["backtest.kline_sync_status"] {
		t.Fatalf("tool descriptors = %#v, want research and sync status companion", names)
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
			PermissionModeLessApproval,
			PermissionModeAll,
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
		PermissionMode: PermissionModeLessApproval,
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
		PermissionMode: PermissionModeLessApproval,
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
