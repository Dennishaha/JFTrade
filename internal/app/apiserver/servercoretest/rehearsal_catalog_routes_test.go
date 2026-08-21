package servercoretest

import (
	"context"
	"fmt"
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
)

var immutableCatalogRouteOperations = []string{
	"GET /api/v1/adk/agent-templates",
	"GET /api/v1/research/screens/catalog",
}

type catalogRehearsalTarget struct {
	endpoint string
}

func (t catalogRehearsalTarget) Endpoint() string  { return t.endpoint }
func (catalogRehearsalTarget) BearerToken() string { return strings.Repeat("r", 64) }
func (catalogRehearsalTarget) Profile() string     { return rustrehearsal.ReadOnlyProfile }
func (catalogRehearsalTarget) Capabilities() []string {
	return append([]string(nil), immutableCatalogRouteOperations...)
}

type wireSnapshot struct {
	status       int
	body         string
	requestID    string
	contentType  string
	cacheControl string
}

func TestImmutableCatalogRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("open settings store: %v", err)
	}
	isolateTestBacktestDatabase(t, store)
	disableTestExchangeCalendarAutoRefresh(t, store)
	forceTestMarketDataProvider(t, store)

	goOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	goServer := httptest.NewServer(goOwner)
	t.Cleanup(func() {
		goServer.Close()
		jftradeCheckTestError(t, goOwner.Close())
	})

	var snapshotsMu sync.Mutex
	snapshots := map[string]wireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("r", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.URL.Query().Get("rehearsalFailure") == "error" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR"}}`))
			return
		}
		if request.URL.Query().Get("rehearsalFailure") == "timeout" {
			<-request.Context().Done()
			return
		}
		ownerRequest, err := http.NewRequestWithContext(request.Context(), request.Method, goServer.URL+request.URL.RequestURI(), nil)
		if err != nil {
			t.Errorf("create Go owner comparison request: %v", err)
			return
		}
		ownerRequest.Header.Set("X-Request-ID", request.Header.Get("X-Request-ID"))
		ownerResponse, err := http.DefaultClient.Do(ownerRequest)
		if err != nil {
			t.Errorf("call Go owner for wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := wireSnapshot{
			status: ownerResponse.StatusCode, body: string(body),
			requestID:    ownerResponse.Header.Get("X-Request-ID"),
			contentType:  ownerResponse.Header.Get("Content-Type"),
			cacheControl: ownerResponse.Header.Get("Cache-Control"),
		}
		snapshotsMu.Lock()
		snapshots[request.URL.RequestURI()] = snapshot
		snapshotsMu.Unlock()
		w.Header().Set("Content-Type", snapshot.contentType)
		w.Header().Set("Cache-Control", snapshot.cacheControl)
		w.Header().Set("X-Request-ID", snapshot.requestID)
		w.WriteHeader(snapshot.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       catalogRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   immutableCatalogRouteOperations,
		RehearsalProxyTimeout: 500 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	t.Cleanup(func() {
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	})

	paths := []string{
		"/api/v1/adk/agent-templates",
		"/api/v1/research/screens/catalog?brokerId=futu&market=US",
	}
	for index, path := range paths {
		requestID := fmt.Sprintf("catalog-rehearsal-%d", index+1)
		response := catalogRehearsalRequest(t, proxyServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read rehearsal response: %v", err)
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

	errorResponse := catalogRehearsalRequest(t, proxyServer.URL+paths[0]+"?rehearsalFailure=error", "catalog-error")
	errorBody, _ := io.ReadAll(errorResponse.Body)
	_ = errorResponse.Body.Close()
	if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
		t.Fatalf("Rust error response was not preserved: %d %s", errorResponse.StatusCode, errorBody)
	}
	timeoutResponse := catalogRehearsalRequest(t, proxyServer.URL+paths[0]+"?rehearsalFailure=timeout", "catalog-timeout")
	_ = timeoutResponse.Body.Close()
	if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("Rust timeout status = %d", timeoutResponse.StatusCode)
	}

	rust.Close()
	crashResponse := catalogRehearsalRequest(t, proxyServer.URL+paths[0], "catalog-crash")
	_ = crashResponse.Body.Close()
	if crashResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf("Rust crash status = %d", crashResponse.StatusCode)
	}

	// Disabling the profile is a restart-time composition decision. It creates
	// a new Go-only router; no failed proxied request is replayed in place.
	goAfterRestart := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(goAfterRestart)
	defer restartedServer.Close()
	defer jftradeCheckTestError(t, goAfterRestart.Close())
	rollbackResponse := catalogRehearsalRequest(t, restartedServer.URL+paths[0], "catalog-rollback")
	_ = rollbackResponse.Body.Close()
	if rollbackResponse.StatusCode != http.StatusOK {
		t.Fatalf("Go owner after restart status = %d", rollbackResponse.StatusCode)
	}
}

func catalogRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create catalog request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call catalog route: %v", err)
	}
	return response
}
