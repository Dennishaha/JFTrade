package servercore

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type failingMCPListener struct{ err error }

func (l failingMCPListener) Accept() (net.Conn, error) { return nil, l.err }
func (failingMCPListener) Close() error                { return nil }
func (failingMCPListener) Addr() net.Addr              { return testMCPAddr("127.0.0.1:0") }

type testMCPAddr string

func (testMCPAddr) Network() string  { return "tcp" }
func (a testMCPAddr) String() string { return string(a) }

func TestMCPServerManagerRemainingLifecycleBoundaries(t *testing.T) {
	var nilManager *mcpServerManager
	if err := nilManager.Reconfigure(jfsettings.MCPServerSettings{}); err == nil {
		t.Fatal("nil manager reconfigure error = nil")
	}
	if status := nilManager.Status(); status.LastError == "" {
		t.Fatalf("nil manager status = %#v", status)
	}
	if err := nilManager.Close(); err != nil {
		t.Fatal(err)
	}

	manager := newMCPServerManager(nil)
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{Enabled: true, AuthMode: "none"}); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("nil runtime reconfigure error = %v", err)
	}
	manager.runtime = jfadk.NewRuntime(nil, jfadk.NewToolRegistry())
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{Enabled: true, AuthMode: "token"}); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("missing token reconfigure error = %v", err)
	}

	listener := failingMCPListener{err: errors.New("unused")}
	manager.listener = listener
	manager.server = &http.Server{}
	manager.settings = jfsettings.MCPServerSettings{Enabled: true, Port: 6697, AuthMode: "none"}
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{Enabled: true, Port: 6697, AuthMode: "none"}); err != nil {
		t.Fatalf("same-listener reconfigure: %v", err)
	}
	if manager.lastErr != "" {
		t.Fatalf("same-listener error = %q", manager.lastErr)
	}
	manager.server = nil
	manager.listener = nil
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{}); err != nil {
		t.Fatalf("disabled empty stop: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := manager.Reconfigure(jfsettings.MCPServerSettings{}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("closed reconfigure error = %v", err)
	}
	if err := closeMCPHTTPServer(nil); err != nil {
		t.Fatal(err)
	}
}

func TestMCPServerManagerRemainingServeFailureStates(t *testing.T) {
	serveErr := errors.New("accept failed")
	manager := newMCPServerManager(jfadk.NewRuntime(nil, jfadk.NewToolRegistry()))
	server := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	manager.server = server
	manager.listener = failingMCPListener{err: serveErr}
	manager.serve(server, manager.listener)
	if manager.server != nil || manager.listener != nil || !strings.Contains(manager.lastErr, "accept failed") {
		t.Fatalf("matched serve failure state = %#v", manager.Status())
	}

	current := &http.Server{}
	manager.server = current
	manager.listener = failingMCPListener{err: errors.New("current")}
	manager.lastErr = ""
	manager.serve(server, failingMCPListener{err: serveErr})
	if manager.server != current || manager.lastErr != "" {
		t.Fatalf("stale serve failure changed current server")
	}
}

func TestMCPAuthorizedHandlerRemainingRequestBoundaries(t *testing.T) {
	manager := newMCPServerManager(jfadk.NewRuntime(nil, jfadk.NewToolRegistry()))
	nextCalls := 0
	handler := manager.authorizedHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusNoContent)
	}))

	request := func(path string, remote string, body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.RemoteAddr = remote
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	if recorder := request("/other", "127.0.0.1:1", ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("non-MCP status = %d", recorder.Code)
	}
	if recorder := request("/mcp", "192.0.2.1:1", ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("remote MCP status = %d", recorder.Code)
	}
	if recorder := request("/mcp", "127.0.0.1:1", ""); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled MCP status = %d", recorder.Code)
	}

	manager.settings = jfsettings.MCPServerSettings{Enabled: true, AuthMode: "token", TokenHash: "invalid"}
	if recorder := request("/mcp", "127.0.0.1:1", "payload"); recorder.Code != http.StatusUnauthorized || recorder.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("unauthorized MCP response = %d %#v", recorder.Code, recorder.Header())
	}
	if manager.authorizeBearer(httptest.NewRequest(http.MethodPost, "/mcp", nil), "") {
		t.Fatal("blank bearer authorization succeeded")
	}

	manager.settings = jfsettings.MCPServerSettings{Enabled: true, AuthMode: "none"}
	if recorder := request("/mcp", "127.0.0.1", "payload"); recorder.Code != http.StatusNoContent || nextCalls != 1 {
		t.Fatalf("unqualified loopback MCP response = %d calls=%d", recorder.Code, nextCalls)
	}
	if isLoopbackRemoteAddr("not-an-ip") {
		t.Fatal("invalid remote address accepted")
	}
	if endpoint := mcpEndpoint(0); !strings.Contains(endpoint, ":6697/mcp") {
		t.Fatalf("default MCP endpoint = %q", endpoint)
	}
}
