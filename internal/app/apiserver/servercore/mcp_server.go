package servercore

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const localMCPMaxRequestBytes int64 = 1 << 20

// mcpServerManager owns the independently bound local MCP listener. Its
// settings are updated synchronously by settings.Service so port conflicts can
// be reported to the caller and persisted settings can be rolled back.
type mcpServerManager struct {
	mu       sync.RWMutex
	runtime  *jfadk.Runtime
	listen   func(network, address string) (net.Listener, error)
	listener net.Listener
	server   *http.Server
	settings jfsettings.MCPServerSettings
	lastErr  string
	closed   bool
}

func newMCPServerManager(runtime *jfadk.Runtime) *mcpServerManager {
	return &mcpServerManager{runtime: runtime, listen: net.Listen}
}

func (m *mcpServerManager) Reconfigure(settings jfsettings.MCPServerSettings) error {
	if m == nil {
		return errors.New("MCP server manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("MCP server manager is closed")
	}

	if !settings.Enabled {
		if err := m.stopLocked(); err != nil {
			m.lastErr = err.Error()
			return err
		}
		m.settings = settings
		m.lastErr = ""
		return nil
	}
	if m.runtime == nil {
		err := errors.New("ADK runtime is unavailable")
		m.lastErr = err.Error()
		return err
	}
	if settings.AuthMode != "none" && strings.TrimSpace(settings.TokenHash) == "" {
		err := errors.New("MCP server token is not configured")
		m.lastErr = err.Error()
		return err
	}
	if m.listener != nil && m.settings.Enabled && m.settings.Port == settings.Port {
		// The authorization wrapper reads the latest settings on each request, so
		// changing token/auth mode does not interrupt existing listener ownership.
		m.settings = settings
		m.lastErr = ""
		return nil
	}

	handler, err := jfadk.NewLocalMCPHandler(m.runtime)
	if err != nil {
		m.lastErr = err.Error()
		return err
	}
	listen := m.listen
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(settings.Port)))
	if err != nil {
		m.lastErr = err.Error()
		return err
	}

	server := &http.Server{
		Handler:           m.authorizedHandler(handler),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       6 * time.Minute,
	}
	oldServer := m.server
	m.server = server
	m.listener = listener
	m.settings = settings
	m.lastErr = ""
	go m.serve(server, listener)
	if oldServer != nil {
		if err := closeMCPHTTPServer(oldServer); err != nil {
			// The new listener is already serving the requested configuration. Keep
			// it alive and surface the cleanup issue through logs/status only.
			m.lastErr = err.Error()
		}
	}
	return nil
}

func (m *mcpServerManager) Status() jfsettings.MCPServerStatus {
	if m == nil {
		return jfsettings.MCPServerStatus{LastError: "MCP server manager is unavailable"}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return jfsettings.MCPServerStatus{
		Running:   m.listener != nil && m.server != nil,
		Endpoint:  mcpEndpoint(m.settings.Port),
		LastError: m.lastErr,
	}
}

func (m *mcpServerManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.stopLocked()
}

func (m *mcpServerManager) serve(server *http.Server, listener net.Listener) {
	err := server.Serve(listener)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == server {
		m.server = nil
		m.listener = nil
		m.lastErr = err.Error()
	}
}

func (m *mcpServerManager) stopLocked() error {
	server := m.server
	m.server = nil
	m.listener = nil
	if server == nil {
		return nil
	}
	return closeMCPHTTPServer(server)
}

func closeMCPHTTPServer(server *http.Server) error {
	if server == nil {
		return nil
	}
	if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (m *mcpServerManager) authorizedHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		if !isLoopbackRemoteAddr(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		m.mu.RLock()
		settings := m.settings
		m.mu.RUnlock()
		if !settings.Enabled {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		if settings.AuthMode != "none" && !m.authorizeBearer(r, settings.TokenHash) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, localMCPMaxRequestBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (m *mcpServerManager) authorizeBearer(r *http.Request, tokenHash string) bool {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" || strings.TrimSpace(tokenHash) == "" {
		return false
	}
	verified, err := passwordhash.Verify(tokenHash, token)
	return err == nil && verified
}

func isLoopbackRemoteAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func mcpEndpoint(port int) string {
	if port <= 0 {
		port = jfsettings.DefaultMCPServerPort
	}
	return fmt.Sprintf("http://127.0.0.1:%d/mcp", port)
}
