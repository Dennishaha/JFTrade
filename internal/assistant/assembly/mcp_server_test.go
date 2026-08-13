package assembly

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPServerManagerEnforcesBearerAndSupportsTokenRotation(t *testing.T) {
	registry := jfadkruntime.NewToolRegistry()
	registry.Register(assistantmodel.ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	runtime := jfadkruntime.NewRuntime(nil, registry)
	manager := newMCPServerManager(runtime)
	firstHash, err := passwordhash.Hash("first-token")
	if err != nil {
		t.Fatalf("hash first token: %v", err)
	}

	// Use an httptest server around the authorization wrapper so this unit test
	// exercises auth and rotation without relying on a fixed local port.
	manager.settings = jfsettings.MCPServerSettings{Enabled: true, Port: 6697, AuthMode: "token", TokenHash: firstHash}
	protected := httptest.NewServer(manager.authorizedHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	t.Cleanup(protected.Close)

	request := func(token string) int {
		req, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost, protected.URL+"/mcp", nil)
		if requestErr != nil {
			t.Fatalf("NewRequest: %v", requestErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, requestErr := protected.Client().Do(req)
		if requestErr != nil {
			t.Fatalf("POST mcp: %v", requestErr)
		}
		defer func() { _ = response.Body.Close() }()
		return response.StatusCode
	}
	if got := request("wrong-token"); got != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d", got)
	}
	if got := request("first-token"); got != http.StatusNoContent {
		t.Fatalf("first token status = %d", got)
	}
	secondHash, err := passwordhash.Hash("second-token")
	if err != nil {
		t.Fatalf("hash second token: %v", err)
	}
	manager.mu.Lock()
	manager.settings.TokenHash = secondHash
	manager.mu.Unlock()
	if got := request("first-token"); got != http.StatusUnauthorized {
		t.Fatalf("old token after rotation status = %d", got)
	}
	if got := request("second-token"); got != http.StatusNoContent {
		t.Fatalf("new token after rotation status = %d", got)
	}
}

func TestMCPServerManagerStartsAndStopsOnLoopback(t *testing.T) {
	registry := jfadkruntime.NewToolRegistry()
	registry.Register(assistantmodel.ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	port := reservation.Addr().(*net.TCPAddr).Port
	if err := reservation.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}
	manager := newMCPServerManager(jfadkruntime.NewRuntime(nil, registry))
	settings := jfsettings.MCPServerSettings{Enabled: true, Port: port, AuthMode: "none"}
	if err := manager.Reconfigure(settings); err != nil {
		t.Fatalf("start MCP manager: %v", err)
	}
	status := manager.Status()
	if !status.Running || !strings.Contains(status.Endpoint, ":"+strconv.Itoa(port)+"/mcp") {
		t.Fatalf("started MCP manager status = %#v", status)
	}
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{Port: port, AuthMode: "none"}); err != nil {
		t.Fatalf("stop MCP manager: %v", err)
	}
	if status := manager.Status(); status.Running {
		t.Fatalf("stopped MCP manager status = %#v", status)
	}
}

func TestMCPServerManagerServesAuthenticatedStreamableMCP(t *testing.T) {
	registry := jfadkruntime.NewToolRegistry()
	registry.Register(assistantmodel.ToolDescriptor{
		Name: "system.status", DisplayName: "System Status", Description: "Returns system status", Permission: "read_internal",
	}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"healthy": true}, nil
	})
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	port := reservation.Addr().(*net.TCPAddr).Port
	if err := reservation.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}
	token := "mcp-test-token"
	tokenHash, err := passwordhash.Hash(token)
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	manager := newMCPServerManager(jfadkruntime.NewRuntime(nil, registry))
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{
		Enabled: true, Port: port, AuthMode: "token", TokenHash: tokenHash,
	}); err != nil {
		t.Fatalf("start MCP manager: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "jftrade-test", Version: "1.0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             manager.Status().Endpoint,
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{token: token}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatalf("initialize MCP session: %v", err)
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
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("tools/call result = %#v", result)
	}

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		request, requestErr := http.NewRequestWithContext(t.Context(), method, manager.Status().Endpoint, nil)
		if requestErr != nil {
			t.Fatalf("NewRequest(%s): %v", method, requestErr)
		}
		request.Header.Set("Accept", "application/json, text/event-stream")
		response, requestErr := (&http.Client{Transport: bearerRoundTripper{token: token}}).Do(request)
		if requestErr != nil {
			t.Fatalf("%s MCP handler: %v", method, requestErr)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want %d", method, response.StatusCode, http.StatusMethodNotAllowed)
		}
		if allow := response.Header.Get("Allow"); allow != http.MethodPost {
			t.Fatalf("%s Allow = %q, want %q", method, allow, http.MethodPost)
		}
	}
}

func TestMCPServerManagerListenerFailurePreservesRunningState(t *testing.T) {
	registry := jfadkruntime.NewToolRegistry()
	registry.Register(assistantmodel.ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	manager := newMCPServerManager(jfadkruntime.NewRuntime(nil, registry))
	handler := newTrackingMCPHandler()
	manager.newHandler = func(*jfadkruntime.Runtime) (mcpLifecycleHandler, error) { return handler, nil }
	manager.listen = func(string, string) (net.Listener, error) { return nil, errors.New("address already in use") }
	settings := jfsettings.MCPServerSettings{Enabled: true, Port: 6697, AuthMode: "none"}
	if err := manager.Reconfigure(settings); err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("Reconfigure error = %v", err)
	}
	status := manager.Status()
	if status.Running || !strings.Contains(status.LastError, "address already in use") {
		t.Fatalf("status after listener failure = %#v", status)
	}
	if got := handler.closeCalls.Load(); got != 1 || manager.handler != nil {
		t.Fatalf("handler cleanup after listener failure calls=%d current=%T", got, manager.handler)
	}
}

func TestMCPServerManagerReleasesHandlersOnReplacementDisableAndClose(t *testing.T) {
	ports := reserveMCPTestPorts(t, 3)
	manager := newMCPServerManager(jfadkruntime.NewRuntime(nil, jfadkruntime.NewToolRegistry()))
	handlers := make([]*trackingMCPHandler, 0, 2)
	manager.newHandler = func(*jfadkruntime.Runtime) (mcpLifecycleHandler, error) {
		handler := newTrackingMCPHandler()
		handlers = append(handlers, handler)
		return handler, nil
	}
	first := jfsettings.MCPServerSettings{Enabled: true, Port: ports[0], AuthMode: "token", TokenHash: "first-hash"}
	if err := manager.Reconfigure(first); err != nil {
		t.Fatalf("start first MCP listener: %v", err)
	}
	rotated := first
	rotated.TokenHash = "second-hash"
	if err := manager.Reconfigure(rotated); err != nil {
		t.Fatalf("rotate MCP token: %v", err)
	}
	if len(handlers) != 1 || handlers[0].closeCalls.Load() != 0 {
		t.Fatalf("same-port handler lifecycle count=%d closes=%d", len(handlers), handlers[0].closeCalls.Load())
	}

	replaced := rotated
	replaced.Port = ports[1]
	if err := manager.Reconfigure(replaced); err != nil {
		t.Fatalf("replace MCP listener: %v", err)
	}
	if len(handlers) != 2 || handlers[0].closeCalls.Load() != 1 || handlers[1].closeCalls.Load() != 0 {
		t.Fatalf("replacement handler lifecycle count=%d closes=%d/%d", len(handlers), handlers[0].closeCalls.Load(), handlers[1].closeCalls.Load())
	}
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{Port: ports[1], AuthMode: "none"}); err != nil {
		t.Fatalf("disable MCP listener: %v", err)
	}
	if handlers[1].closeCalls.Load() != 1 || manager.handler != nil {
		t.Fatalf("disabled handler closes=%d current=%T", handlers[1].closeCalls.Load(), manager.handler)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close disabled MCP manager: %v", err)
	}

	closeManager := newMCPServerManager(jfadkruntime.NewRuntime(nil, jfadkruntime.NewToolRegistry()))
	closeHandler := newTrackingMCPHandler()
	closeManager.newHandler = func(*jfadkruntime.Runtime) (mcpLifecycleHandler, error) { return closeHandler, nil }
	if err := closeManager.Reconfigure(jfsettings.MCPServerSettings{Enabled: true, Port: ports[2], AuthMode: "none"}); err != nil {
		t.Fatalf("start close-path MCP listener: %v", err)
	}
	if err := closeManager.Close(); err != nil {
		t.Fatalf("close MCP manager: %v", err)
	}
	if got := closeHandler.closeCalls.Load(); got != 1 || closeManager.handler != nil {
		t.Fatalf("manager-close handler cleanup calls=%d current=%T", got, closeManager.handler)
	}
}

func TestMCPServerManagerReleasesHandlerOnUnexpectedServeExit(t *testing.T) {
	manager := newMCPServerManager(jfadkruntime.NewRuntime(nil, jfadkruntime.NewToolRegistry()))
	handler := newTrackingMCPHandler()
	manager.newHandler = func(*jfadkruntime.Runtime) (mcpLifecycleHandler, error) { return handler, nil }
	manager.listen = func(string, string) (net.Listener, error) {
		return failingMCPListener{err: errors.New("accept failed")}, nil
	}
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{Enabled: true, Port: 6697, AuthMode: "none"}); err != nil {
		t.Fatalf("start failing MCP listener: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for handler.closeCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	status := manager.Status()
	if got := handler.closeCalls.Load(); got != 1 || status.Running || !strings.Contains(status.LastError, "accept failed") {
		t.Fatalf("unexpected-exit cleanup calls=%d status=%#v", got, status)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close failed MCP manager: %v", err)
	}
}

func TestMCPServerManagerUsesLoopbackOnly(t *testing.T) {
	if isLoopbackRemoteAddr("192.0.2.4:9000") {
		t.Fatal("public remote address accepted as loopback")
	}
	for _, remoteAddr := range []string{"127.0.0.1:9000", "[::1]:9000"} {
		if !isLoopbackRemoteAddr(remoteAddr) {
			t.Fatalf("loopback remote address rejected: %q", remoteAddr)
		}
	}
}

type bearerRoundTripper struct {
	token string
}

type trackingMCPHandler struct {
	http.Handler
	closeCalls atomic.Int32
}

func newTrackingMCPHandler() *trackingMCPHandler {
	return &trackingMCPHandler{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
}

func (h *trackingMCPHandler) Close() {
	h.closeCalls.Add(1)
}

func reserveMCPTestPorts(t *testing.T, count int) []int {
	t.Helper()
	listeners := make([]net.Listener, 0, count)
	ports := make([]int, 0, count)
	for range count {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve MCP test port: %v", err)
		}
		listeners = append(listeners, listener)
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
	}
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatalf("release MCP test port: %v", err)
		}
	}
	return ports
}

func (t bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(clone)
}
