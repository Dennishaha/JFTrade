package servercore

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPServerManagerEnforcesBearerAndSupportsTokenRotation(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	registry.Register(jfadk.ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	runtime := jfadk.NewRuntime(nil, registry)
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
	registry := jfadk.NewToolRegistry()
	registry.Register(jfadk.ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
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
	manager := newMCPServerManager(jfadk.NewRuntime(nil, registry))
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
	registry := jfadk.NewToolRegistry()
	registry.Register(jfadk.ToolDescriptor{
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
	manager := newMCPServerManager(jfadk.NewRuntime(nil, registry))
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
}

func TestMCPServerManagerListenerFailurePreservesRunningState(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	registry.Register(jfadk.ToolDescriptor{Name: "system.status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	manager := newMCPServerManager(jfadk.NewRuntime(nil, registry))
	manager.listen = func(string, string) (net.Listener, error) { return nil, errors.New("address already in use") }
	settings := jfsettings.MCPServerSettings{Enabled: true, Port: 6697, AuthMode: "none"}
	if err := manager.Reconfigure(settings); err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("Reconfigure error = %v", err)
	}
	status := manager.Status()
	if status.Running || !strings.Contains(status.LastError, "address already in use") {
		t.Fatalf("status after listener failure = %#v", status)
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

func (t bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(clone)
}
