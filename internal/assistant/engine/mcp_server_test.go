package adk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

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
		Name: "strategy.definition_versions.list", DisplayName: "Strategy Versions", Description: "versions", Permission: "read_internal",
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"definitionId": "def-1", "versionCount": 2}, nil
	})
	registry.Register(ToolDescriptor{
		Name: "strategy.definition_versions.get", DisplayName: "Strategy Version", Description: "version", Permission: "read_internal",
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"definitionId": "def-1", "version": "0.1.0", "script": "strategy(\"v1\")"}, nil
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
	for _, name := range []string{"strategy.definition_versions.list", "strategy.definition_versions.get"} {
		if !slices.Contains(names, name) {
			t.Fatalf("tools/list missing reviewed strategy version tool %q: %v", name, names)
		}
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

	versionResult, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "strategy.definition_versions.get",
		Arguments: map[string]any{"definitionId": "def-1", "version": "0.1.0"},
	})
	if err != nil {
		t.Fatalf("strategy version tools/call: %v", err)
	}
	versionStructured, ok := versionResult.StructuredContent.(map[string]any)
	if versionResult.IsError || !ok || versionStructured["version"] != "0.1.0" || versionStructured["script"] == "" {
		t.Fatalf("strategy version tools/call result = %#v", versionResult)
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

func TestLocalMCPHandlerCloseUnsubscribesRegistryListener(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	baseline := toolRegistryChangeHandlerCount(registry)
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	if got := toolRegistryChangeHandlerCount(registry); got != baseline+1 {
		t.Fatalf("registry listeners after handler creation = %d, want %d", got, baseline+1)
	}
	handler.Close()
	handler.Close()
	if got := toolRegistryChangeHandlerCount(registry); got != baseline {
		t.Fatalf("registry listeners after handler close = %d, want %d", got, baseline)
	}
}

func toolRegistryChangeHandlerCount(registry *ToolRegistry) int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return len(registry.changeHandlers)
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

func TestLocalMCPHandlerServesStatelessPostOnlyRequests(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"healthy": true}, nil
	})
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	roundTripper := &noSessionRoundTripper{base: server.Client().Transport}
	client := mcp.NewClient(&mcp.Implementation{Name: "jftrade-test", Version: "1.0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           &http.Client{Transport: roundTripper},
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
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "system.status" {
		t.Fatalf("tools/list = %#v", tools.Tools)
	}
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "system.status"})
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if result.IsError || !ok || structured["healthy"] != true {
		t.Fatalf("tools/call result = %#v", result)
	}

	for _, sessionID := range roundTripper.requestSessionIDs {
		if sessionID != "" {
			t.Fatalf("stateless client sent Mcp-Session-Id %q", sessionID)
		}
	}
	for _, sessionID := range roundTripper.responseSessionIDs {
		if sessionID != "" {
			t.Fatalf("stateless server returned Mcp-Session-Id %q", sessionID)
		}
	}

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		request, requestErr := http.NewRequestWithContext(t.Context(), method, server.URL, nil)
		if requestErr != nil {
			t.Fatalf("NewRequest(%s): %v", method, requestErr)
		}
		request.Header.Set("Accept", "application/json, text/event-stream")
		response, requestErr := server.Client().Do(request)
		if requestErr != nil {
			t.Fatalf("%s MCP handler: %v", method, requestErr)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want %d", method, response.StatusCode, http.StatusMethodNotAllowed)
		}
		if allow := response.Header.Get("Allow"); allow != http.MethodPost {
			t.Fatalf("%s Allow = %q, want %q", method, allow, http.MethodPost)
		}
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

func TestLocalMCPHandlerReadsSanitizedRuntimeStatusResource(t *testing.T) {
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
	session := connectLocalMCPClient(t, server, nil)
	t.Cleanup(func() { _ = session.Close() })

	resources, err := session.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	if len(resources.Resources) != 1 || resources.Resources[0].URI != localMCPRuntimeStatusURI {
		t.Fatalf("resources/list = %#v", resources.Resources)
	}
	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: localMCPRuntimeStatusURI})
	if err != nil {
		t.Fatalf("resources/read: %v", err)
	}
	if len(result.Contents) != 1 || result.Contents[0].MIMEType != "application/json" {
		t.Fatalf("resources/read = %#v", result)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &status); err != nil {
		t.Fatalf("decode runtime status: %v", err)
	}
	if status["storeConfigured"] != false || len(status["tools"].([]any)) == 0 {
		t.Fatalf("sanitized runtime status = %#v", status)
	}
	if _, found := status["snapshotError"]; found {
		t.Fatalf("runtime status exposed an internal snapshot error: %#v", status)
	}
}

func TestSanitizedMCPRuntimeStatusIncludesConfiguredDataAndErrors(t *testing.T) {
	runtime := newTestRuntime(t)
	if _, err := runtime.Store().SaveAgent(t.Context(), AgentWriteRequest{ID: "status-agent", Name: "Status Agent", ProviderID: testProviderID, Status: AgentStatusEnabled}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	status := sanitizedMCPRuntimeStatus(t.Context(), runtime)
	if status["storeConfigured"] != true {
		t.Fatalf("storeConfigured = %#v", status["storeConfigured"])
	}
	if len(status["providers"].([]map[string]any)) == 0 || len(status["agents"].([]map[string]any)) == 0 {
		t.Fatalf("configured status omitted providers or agents: %#v", status)
	}
	status = sanitizedMCPRuntimeStatus(t.Context(), runtime)
	if _, found := status["snapshotError"]; found {
		t.Fatalf("healthy runtime reported snapshot error: %#v", status)
	}
	if err := runtime.Store().Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	status = sanitizedMCPRuntimeStatus(t.Context(), runtime)
	if status["snapshotError"] != "runtime snapshot unavailable" {
		t.Fatalf("closed runtime status = %#v", status)
	}
}

func TestSanitizedMCPRuntimeStatusSerializesDescriptors(t *testing.T) {
	providers := sanitizedMCPProviders([]Provider{{ID: "p", DisplayName: "Provider", Model: "model", Enabled: true, Default: true, HasAPIKey: true, Capabilities: map[string]bool{"tools": true}}})
	agents := sanitizedMCPAgents([]Agent{{ID: "a", Name: "Agent", ProviderID: "p", Model: "model", Tools: []string{"system.status"}, Skills: []string{"skill"}, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled, Builtin: true}})
	skills := sanitizedMCPSkills([]Skill{{ID: "skill", DisplayName: "Skill", Description: "desc", Source: "builtin", Enabled: true, Builtin: true, Tools: []string{"system.status"}, Version: "1", ValidationStatus: "valid"}})
	if providers[0]["id"] != "p" || agents[0]["providerId"] != "p" || skills[0]["validationStatus"] != "valid" {
		t.Fatalf("sanitized descriptors = %#v %#v %#v", providers, agents, skills)
	}
}

func TestLocalMCPRuntimeStatusSubscriptionValidation(t *testing.T) {
	if err := subscribeLocalMCPRuntimeStatus(t.Context(), nil); err == nil {
		t.Fatal("nil subscribe request accepted")
	}
	if err := subscribeLocalMCPRuntimeStatus(t.Context(), &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: "jftrade://invalid"}}); err == nil {
		t.Fatal("invalid subscribe URI accepted")
	}
	if err := unsubscribeLocalMCPRuntimeStatus(t.Context(), nil); err == nil {
		t.Fatal("nil unsubscribe request accepted")
	}
	if err := unsubscribeLocalMCPRuntimeStatus(t.Context(), &mcp.UnsubscribeRequest{Params: &mcp.UnsubscribeParams{URI: "jftrade://invalid"}}); err == nil {
		t.Fatal("invalid unsubscribe URI accepted")
	}
}

func TestLocalMCPHandlerSynchronizesReviewedToolsAndRuntimeSubscriptions(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	server := httptest.NewServer(handler)
	toolChanged := make(chan struct{}, 2)
	resourceUpdated := make(chan string, 2)
	client := mcp.NewClient(&mcp.Implementation{Name: "jftrade-test", Version: "1.0"}, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) { toolChanged <- struct{}{} },
		ResourceUpdatedHandler: func(_ context.Context, request *mcp.ResourceUpdatedNotificationRequest) {
			resourceUpdated <- request.Params.URI
		},
	})
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: server.Client(), DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		server.Close()
		t.Fatalf("MCP initialize: %v", err)
	}
	defer func() {
		_ = session.Close()
		server.CloseClientConnections()
		registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
			return map[string]any{"ok": true}, nil
		})
		server.Close()
	}()
	if err := session.Subscribe(t.Context(), &mcp.SubscribeParams{URI: localMCPRuntimeStatusURI}); err != nil {
		t.Fatalf("resources/subscribe: %v", err)
	}

	registry.Register(ToolDescriptor{Name: "market.search", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"items": []any{}}, nil
	})
	awaitLocalMCPNotification(t, "tools/list_changed", toolChanged)
	select {
	case uri := <-resourceUpdated:
		if uri != localMCPRuntimeStatusURI {
			t.Fatalf("resources/updated URI = %q", uri)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for resources/updated")
	}
	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("tools/list after registry change: %v", err)
	}
	if !slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "market.search" }) {
		t.Fatalf("tools/list did not receive the reviewed registry update: %#v", tools.Tools)
	}
}

func TestLocalMCPHandlerRefreshesReplacedToolHandler(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"version": "old"}, nil
	})
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	toolChanged := make(chan struct{}, 2)
	session := connectLocalMCPClient(t, server, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) { toolChanged <- struct{}{} },
	})
	t.Cleanup(func() { _ = session.Close() })

	initial, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "system.status"})
	if err != nil {
		t.Fatalf("initial tools/call: %v", err)
	}
	initialOutput, ok := initial.StructuredContent.(map[string]any)
	if initial.IsError || !ok || initialOutput["version"] != "old" {
		t.Fatalf("initial tools/call result = %#v", initial.StructuredContent)
	}

	registry.Register(ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"version": "new"}, nil
	})
	awaitLocalMCPNotification(t, "tools/list_changed", toolChanged)

	updated, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "system.status"})
	if err != nil {
		t.Fatalf("updated tools/call: %v", err)
	}
	updatedOutput, ok := updated.StructuredContent.(map[string]any)
	if updated.IsError || !ok || updatedOutput["version"] != "new" {
		t.Fatalf("updated tools/call result = %#v", updated.StructuredContent)
	}
}

func connectLocalMCPClient(t *testing.T, server *httptest.Server, options *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "jftrade-test", Version: "1.0"}, options)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: server.Client(), DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("MCP initialize: %v", err)
	}
	return session
}

func awaitLocalMCPNotification(t *testing.T, name string, events <-chan struct{}) {
	t.Helper()
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

type noSessionRoundTripper struct {
	base               http.RoundTripper
	requestSessionIDs  []string
	responseSessionIDs []string
}

func (t *noSessionRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	t.requestSessionIDs = append(t.requestSessionIDs, request.Header.Get("Mcp-Session-Id"))
	response, err := t.base.RoundTrip(request)
	if err == nil {
		t.responseSessionIDs = append(t.responseSessionIDs, response.Header.Get("Mcp-Session-Id"))
	}
	return response, err
}
