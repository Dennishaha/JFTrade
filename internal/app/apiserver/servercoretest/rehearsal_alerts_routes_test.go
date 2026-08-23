package servercoretest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

var alertsReadRouteOperations = []string{
	"GET /api/v1/alerts/option-events",
	"GET /api/v1/alerts/price",
}

type alertsRehearsalTarget struct {
	endpoint string
}

func (t alertsRehearsalTarget) Endpoint() string  { return t.endpoint }
func (alertsRehearsalTarget) BearerToken() string { return strings.Repeat("a", 64) }
func (alertsRehearsalTarget) Profile() string     { return rustrehearsal.ReadOnlyProfile }
func (alertsRehearsalTarget) Capabilities() []string {
	return append([]string(nil), alertsReadRouteOperations...)
}

type alertsWireSnapshot struct {
	status       int
	body         string
	requestID    string
	contentType  string
	cacheControl string
	retryAfter   string
}

func TestAlertsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	store, err := servercore.NewSettingsStore(settingsPath)
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
	beforeRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings before rehearsal: %v", err)
	}

	var snapshotsMu sync.Mutex
	snapshots := map[string]alertsWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
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
			t.Errorf("call Go owner for alerts wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := alertsWireSnapshot{
			status: ownerResponse.StatusCode, body: string(body),
			requestID:    ownerResponse.Header.Get("X-Request-ID"),
			contentType:  ownerResponse.Header.Get("Content-Type"),
			cacheControl: ownerResponse.Header.Get("Cache-Control"),
			retryAfter:   ownerResponse.Header.Get("Retry-After"),
		}
		snapshotsMu.Lock()
		snapshots[request.URL.RequestURI()] = snapshot
		snapshotsMu.Unlock()
		w.Header().Set("Content-Type", snapshot.contentType)
		w.Header().Set("Cache-Control", snapshot.cacheControl)
		w.Header().Set("X-Request-ID", snapshot.requestID)
		if snapshot.retryAfter != "" {
			w.Header().Set("Retry-After", snapshot.retryAfter)
		}
		w.WriteHeader(snapshot.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       alertsRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   alertsReadRouteOperations,
		RehearsalProxyTimeout: 500 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	t.Cleanup(func() {
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	})

	paths := []string{
		"/api/v1/alerts/price?brokerId=missing&market=US",
		"/api/v1/alerts/option-events?brokerId=missing&market=US",
	}
	for index, path := range paths {
		requestID := "alerts-read-wire-" + string(rune('1'+index))
		response := alertsRehearsalRequest(t, proxyServer.URL+path, requestID)
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
			response.Header.Get("Cache-Control") != want.cacheControl ||
			response.Header.Get("Retry-After") != want.retryAfter {
			t.Fatalf("wire header mismatch for %s: %#v, want %#v", path, response.Header, want)
		}
	}

	errorResponse := alertsRehearsalRequest(t, proxyServer.URL+paths[0]+"&rehearsalFailure=error", "alerts-read-error")
	errorBody, _ := io.ReadAll(errorResponse.Body)
	_ = errorResponse.Body.Close()
	if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
		t.Fatalf("Rust error response was not preserved: %d %s", errorResponse.StatusCode, errorBody)
	}
	timeoutResponse := alertsRehearsalRequest(t, proxyServer.URL+paths[0]+"&rehearsalFailure=timeout", "alerts-read-timeout")
	_ = timeoutResponse.Body.Close()
	if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("Rust timeout status = %d", timeoutResponse.StatusCode)
	}

	rust.Close()
	crashResponse := alertsRehearsalRequest(t, proxyServer.URL+paths[0], "alerts-read-crash")
	_ = crashResponse.Body.Close()
	if crashResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf("Rust crash status = %d", crashResponse.StatusCode)
	}

	// A failed proxy request does not replay to Go. Rollback is a restart-time
	// composition decision that builds a fresh Go-only router.
	goAfterRestart := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(goAfterRestart)
	defer restartedServer.Close()
	defer jftradeCheckTestError(t, goAfterRestart.Close())
	rollbackResponse := alertsRehearsalRequest(t, restartedServer.URL+paths[0], "alerts-read-rollback")
	rollbackBody, _ := io.ReadAll(rollbackResponse.Body)
	_ = rollbackResponse.Body.Close()
	if rollbackResponse.StatusCode != http.StatusConflict || !strings.Contains(string(rollbackBody), "BROKER_CAPABILITY_UNAVAILABLE") {
		t.Fatalf("Go owner after restart status/body = %d %s", rollbackResponse.StatusCode, rollbackBody)
	}

	afterRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after rehearsal: %v", err)
	}
	if string(afterRequest) != string(beforeRequest) {
		t.Fatalf("alerts read rehearsal modified settings: before=%q after=%q", beforeRequest, afterRequest)
	}
}

func alertsRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create alerts request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call alerts route: %v", err)
	}
	return response
}
