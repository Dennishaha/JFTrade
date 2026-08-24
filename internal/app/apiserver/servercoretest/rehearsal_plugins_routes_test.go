package servercoretest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategycatalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
)

var pluginsReadRouteOperations = []string{
	"GET /api/v1/plugins",
	"GET /api/v1/plugins/operations/{operationId}",
	"GET /api/v1/plugins/{pluginId}/uninstall-guidance",
}

type pluginsReadRehearsalTarget struct {
	endpoint string
}

func (t pluginsReadRehearsalTarget) Endpoint() string  { return t.endpoint }
func (pluginsReadRehearsalTarget) BearerToken() string { return strings.Repeat("p", 64) }
func (pluginsReadRehearsalTarget) Profile() string     { return rustrehearsal.ReadOnlyProfile }
func (pluginsReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), pluginsReadRouteOperations...)
}

type pluginsReadWireSnapshot struct {
	status       int
	body         string
	requestID    string
	contentType  string
	cacheControl string
}

func TestPluginsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("open settings store: %v", err)
	}
	isolateTestBacktestDatabase(t, store)
	disableTestExchangeCalendarAutoRefresh(t, store)
	forceTestMarketDataProvider(t, store)
	operationID := seedPluginsReadCatalog(t, store)

	goOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	goServer := httptest.NewServer(goOwner)
	goClosed := false
	closeGoOwner := func() {
		if goClosed {
			return
		}
		goClosed = true
		goServer.Close()
		jftradeCheckTestError(t, goOwner.Close())
	}
	t.Cleanup(closeGoOwner)

	var snapshotsMu sync.Mutex
	snapshots := map[string]pluginsReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("p", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.Body != http.NoBody && request.ContentLength != 0 {
			t.Errorf("plugins read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
		}
		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR"}}`))
			return
		case "timeout":
			<-request.Context().Done()
			return
		}

		ownerRequest, err := http.NewRequestWithContext(
			request.Context(), request.Method, goServer.URL+request.URL.RequestURI(), nil,
		)
		if err != nil {
			t.Errorf("create Go owner comparison request: %v", err)
			return
		}
		ownerRequest.Header.Set("X-Request-ID", request.Header.Get("X-Request-ID"))
		ownerResponse, err := http.DefaultClient.Do(ownerRequest)
		if err != nil {
			t.Errorf("call Go owner for plugins wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := pluginsReadWireSnapshot{
			status: ownerResponse.StatusCode, body: string(body),
			requestID:    ownerResponse.Header.Get("X-Request-ID"),
			contentType:  ownerResponse.Header.Get("Content-Type"),
			cacheControl: ownerResponse.Header.Get("Cache-Control"),
		}
		snapshotsMu.Lock()
		snapshots[request.URL.Path] = snapshot
		snapshotsMu.Unlock()
		w.Header().Set("Content-Type", snapshot.contentType)
		w.Header().Set("Cache-Control", snapshot.cacheControl)
		w.Header().Set("X-Request-ID", snapshot.requestID)
		w.WriteHeader(snapshot.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rust.Close)
	proxyEndpoint := rust.URL
	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       pluginsReadRehearsalTarget{endpoint: proxyEndpoint},
		RehearsalOperations:   pluginsReadRouteOperations,
		RehearsalProxyTimeout: 500 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	proxyClosed := false
	closeProxy := func() {
		if proxyClosed {
			return
		}
		proxyClosed = true
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	}
	t.Cleanup(closeProxy)

	paths := []string{
		"/api/v1/plugins",
		"/api/v1/plugins/operations/" + operationID,
		"/api/v1/plugins/alpha/uninstall-guidance",
	}
	for index, path := range paths {
		requestID := "plugins-read-wire-" + string(rune('1'+index))
		response := pluginsReadRehearsalRequest(t, proxyServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read plugins rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		if response.StatusCode != want.status || string(body) != want.body {
			t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
		}
		if response.Header.Get("X-Request-ID") != want.requestID ||
			response.Header.Get("Content-Type") != want.contentType ||
			response.Header.Get("Cache-Control") != want.cacheControl {
			t.Fatalf("wire header mismatch for %s: %#v, want %#v", path, response.Header, want)
		}
	}

	for index, path := range paths {
		errorResponse := pluginsReadRehearsalRequest(t, proxyServer.URL+path+"?rehearsalFailure=error", "plugins-read-error-"+string(rune('1'+index)))
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := pluginsReadRehearsalRequest(t, proxyServer.URL+path+"?rehearsalFailure=timeout", "plugins-read-timeout-"+string(rune('1'+index)))
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for _, path := range paths {
		crashResponse := pluginsReadRehearsalRequest(t, proxyServer.URL+path, "plugins-read-crash")
		_ = crashResponse.Body.Close()
		if crashResponse.StatusCode != http.StatusBadGateway {
			t.Fatalf("Rust crash status for %s = %d", path, crashResponse.StatusCode)
		}
	}

	closeProxy()
	closeGoOwner()
	goAfterRestart := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(goAfterRestart)
	defer restartedServer.Close()
	defer jftradeCheckTestError(t, goAfterRestart.Close())
	for _, path := range paths {
		rollbackResponse := pluginsReadRehearsalRequest(t, restartedServer.URL+path, "plugins-read-rollback")
		rollbackBody, _ := io.ReadAll(rollbackResponse.Body)
		_ = rollbackResponse.Body.Close()
		if rollbackResponse.StatusCode != http.StatusOK {
			t.Fatalf("Go owner after restart status for %s = %d body=%s", path, rollbackResponse.StatusCode, rollbackBody)
		}
	}
}

func seedPluginsReadCatalog(t *testing.T, store *servercore.SettingsStore) string {
	t.Helper()
	catalog, err := openSeededStrategyCatalog(t, store)
	if err != nil {
		t.Fatalf("open plugin catalog: %v", err)
	}
	if err := catalog.RegisterPlugin(strategycatalog.ManagedPlugin{
		Descriptor: stratsrv.PluginDescriptor{
			ID:          "alpha",
			Type:        "strategy-go-plugin",
			DisplayName: "Alpha Strategy",
			Version:     "1.2.3",
			Description: "fixture strategy plugin",
			Keywords:    []string{"alpha", "trend"},
		},
	}); err != nil {
		t.Fatalf("register plugin fixture: %v", err)
	}
	operation, err := catalog.InstallPlugin("alpha")
	if err != nil {
		t.Fatalf("install plugin fixture metadata: %v", err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatalf("close plugin catalog fixture: %v", err)
	}
	return operation.OperationID
}

func pluginsReadRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create plugins read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call plugins read route: %v", err)
	}
	return response
}
