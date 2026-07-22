package adk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLocalMCPHandlerExposesOnlyReviewedReadTools(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "system.status", DisplayName: "System Status", Description: "status", Permission: "read_internal",
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"healthy": true}, nil
	})
	registry.Register(ToolDescriptor{
		Name: "strategy.save_definition", DisplayName: "Save Strategy", Description: "write", Permission: "write_strategy",
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"saved": true}, nil
	})
	runtime := NewRuntime(nil, registry)
	handler, err := NewLocalMCPHandler(runtime)
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "jftrade-test", Version: "1.0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           server.Client(),
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatalf("MCP initialize: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if !slices.Contains(names, "system.status") {
		t.Fatalf("tools/list missing reviewed tool: %v", names)
	}
	if slices.Contains(names, "strategy.save_definition") || slices.Contains(names, "http.fetch") || slices.Contains(names, "tasks.create") {
		t.Fatalf("tools/list exposed a non-reviewed tool: %v", names)
	}
	var statusTool *mcp.Tool
	for index := range tools.Tools {
		if tools.Tools[index].Name == "system.status" {
			statusTool = tools.Tools[index]
			break
		}
	}
	if statusTool == nil {
		t.Fatal("system.status descriptor is unavailable")
		return
	}
	inputSchema, ok := statusTool.InputSchema.(map[string]any)
	if !ok || inputSchema["type"] != "object" {
		t.Fatalf("system.status input schema = %#v", statusTool.InputSchema)
	}

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "system.status"})
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("tools/call result = %#v", result)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["healthy"] != true {
		t.Fatalf("tools/call structured result = %#v", result.StructuredContent)
	}
}

func TestLocalMCPHandlerRejectsWriteCapableReplacementOfReviewedName(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "system.status", Permission: "write_strategy"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"written": true}, nil
	})
	if _, err := NewLocalMCPHandler(NewRuntime(nil, registry)); err == nil {
		t.Fatal("write-capable replacement was accepted as an MCP tool")
	}
}

func TestLocalMCPHandlerRequiresAtLeastOneReviewedTool(t *testing.T) {
	runtime := NewRuntime(nil, NewToolRegistry())
	if _, err := NewLocalMCPHandler(runtime); err == nil {
		t.Fatal("NewLocalMCPHandler without reviewed tools error = nil")
	}
}

func TestLocalMCPHandlerReturnsToolFailuresAsMCPToolErrors(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return nil, errors.New("status provider unavailable")
	})
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "jftrade-test", Version: "1.0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           server.Client(),
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatalf("MCP initialize: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "system.status"})
	if err != nil {
		t.Fatalf("tools/call transport error: %v", err)
	}
	if !result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "status provider unavailable") {
		t.Fatalf("tools/call failure result = %#v", result)
	}
}

func TestLocalMCPHandlerPreservesMCPHostProtection(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Host = "example.test"
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("POST MCP handler: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("Host protection status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}
