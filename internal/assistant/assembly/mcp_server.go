package assembly

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
)

const localMCPMaxRequestBytes int64 = 1 << 20

type mcpLifecycleHandler interface {
	http.Handler
	Close()
}

// mcpServerManager owns the independently bound local MCP listener. Its
// settings are updated synchronously by settings.Service so port conflicts can
// be reported to the caller and persisted settings can be rolled back.
type mcpServerManager struct {
	mu         sync.RWMutex
	runtime    *jfadk.Runtime
	listen     func(network, address string) (net.Listener, error)
	newHandler func(*jfadk.Runtime) (mcpLifecycleHandler, error)
	listener   net.Listener
	server     *http.Server
	handler    mcpLifecycleHandler
	serveDone  chan struct{}
	settings   jfsettings.MCPServerSettings
	lastErr    string
	closed     bool
	serveWG    sync.WaitGroup
}

func newMCPServerManager(runtime *jfadk.Runtime) *mcpServerManager {
	return &mcpServerManager{
		runtime: runtime,
		listen:  net.Listen,
		newHandler: func(runtime *jfadk.Runtime) (mcpLifecycleHandler, error) {
			return jfadk.NewLocalMCPHandler(runtime)
		},
	}
}

func (m *mcpServerManager) Reconfigure(settings jfsettings.MCPServerSettings) error {
	if m == nil {
		return errors.New("MCP server manager is unavailable")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("MCP server manager is closed")
	}

	if !settings.Enabled {
		server, handler, done := m.detachLocked()
		m.settings = settings
		m.lastErr = ""
		m.mu.Unlock()
		if err := stopMCPServer(server, handler, done); err != nil {
			m.mu.Lock()
			m.lastErr = err.Error()
			m.mu.Unlock()
			return err
		}
		return nil
	}
	if m.runtime == nil {
		err := errors.New("ADK runtime is unavailable")
		m.lastErr = err.Error()
		m.mu.Unlock()
		return err
	}
	if settings.AuthMode != "none" && strings.TrimSpace(settings.TokenHash) == "" {
		err := errors.New("MCP server token is not configured")
		m.lastErr = err.Error()
		m.mu.Unlock()
		return err
	}
	if m.listener != nil && m.settings.Enabled && m.settings.Port == settings.Port {
		// The authorization wrapper reads the latest settings on each request, so
		// changing token/auth mode does not interrupt existing listener ownership.
		m.settings = settings
		m.lastErr = ""
		m.mu.Unlock()
		return nil
	}

	handler, err := m.createHandler()
	if err != nil {
		m.lastErr = err.Error()
		m.mu.Unlock()
		return err
	}
	listen := m.listen
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(settings.Port)))
	if err != nil {
		handler.Close()
		m.lastErr = err.Error()
		m.mu.Unlock()
		return err
	}

	server := &http.Server{
		Handler:           m.authorizedHandler(handler),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       6 * time.Minute,
	}
	oldServer := m.server
	oldHandler := m.handler
	oldDone := m.serveDone
	serveDone := make(chan struct{})
	m.server = server
	m.listener = listener
	m.handler = handler
	m.serveDone = serveDone
	m.settings = settings
	m.lastErr = ""
	m.serveWG.Go(func() {
		m.serve(server, listener, serveDone)
	})
	m.mu.Unlock()
	if oldServer != nil {
		if err := closeMCPHTTPServer(oldServer); err != nil {
			// The new listener is already serving the requested configuration. Keep
			// it alive and surface the cleanup issue through logs/status only.
			m.mu.Lock()
			m.lastErr = err.Error()
			m.mu.Unlock()
		}
	}
	if oldHandler != nil {
		oldHandler.Close()
	}
	waitMCPServe(oldDone)
	return nil
}

func (m *mcpServerManager) createHandler() (mcpLifecycleHandler, error) {
	if m.newHandler != nil {
		return m.newHandler(m.runtime)
	}
	return jfadk.NewLocalMCPHandler(m.runtime)
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
	m.closed = true
	server, handler, done := m.detachLocked()
	m.mu.Unlock()
	closeErr := stopMCPServer(server, handler, done)
	m.serveWG.Wait()
	return closeErr
}

func (m *mcpServerManager) serve(server *http.Server, listener net.Listener, done ...chan struct{}) {
	if len(done) > 0 && done[0] != nil {
		defer close(done[0])
	}
	err := server.Serve(listener)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return
	}
	m.mu.Lock()
	var handler mcpLifecycleHandler
	if m.server == server {
		handler = m.handler
		m.server = nil
		m.listener = nil
		m.handler = nil
		m.serveDone = nil
		m.lastErr = err.Error()
	}
	m.mu.Unlock()
	if handler != nil {
		handler.Close()
	}
}

func (m *mcpServerManager) detachLocked() (*http.Server, mcpLifecycleHandler, chan struct{}) {
	server := m.server
	handler := m.handler
	done := m.serveDone
	m.server = nil
	m.listener = nil
	m.handler = nil
	m.serveDone = nil
	return server, handler, done
}

func stopMCPServer(server *http.Server, handler mcpLifecycleHandler, done chan struct{}) error {
	err := closeMCPHTTPServer(server)
	if handler != nil {
		handler.Close()
	}
	waitMCPServe(done)
	return err
}

func waitMCPServe(done chan struct{}) {
	if done != nil {
		<-done
	}
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
		// Keep Origin handling identical to the Rust listener.  MCP clients
		// commonly omit Origin; when supplied, only a valid same-host origin is
		// accepted.  Browser "null", malformed and cross-origin values fail
		// closed before the SDK parses the JSON-RPC body.
		if !mcpOriginAllowed(r) {
			http.Error(w, "Forbidden: invalid Origin header", http.StatusForbidden)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, localMCPMaxRequestBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func mcpOriginAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if strings.EqualFold(origin, "null") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, strings.TrimSpace(r.Host))
}

func (m *mcpServerManager) authorizeBearer(r *http.Request, tokenHash string) bool {
	token := mcpBearerToken(r.Header.Get("Authorization"))
	if token == "" || strings.TrimSpace(tokenHash) == "" {
		return false
	}
	verified, err := passwordhash.Verify(tokenHash, token)
	return err == nil && verified
}

func mcpBearerToken(header string) string {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
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
