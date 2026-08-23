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

var appearanceReadRouteOperations = []string{
	"GET /api/v1/settings/ui",
}

type appearanceRehearsalTarget struct {
	endpoint string
}

func (t appearanceRehearsalTarget) Endpoint() string  { return t.endpoint }
func (appearanceRehearsalTarget) BearerToken() string { return strings.Repeat("a", 64) }
func (appearanceRehearsalTarget) Profile() string     { return rustrehearsal.ReadOnlyProfile }
func (appearanceRehearsalTarget) Capabilities() []string {
	return append([]string(nil), appearanceReadRouteOperations...)
}

type appearanceWireSnapshot struct {
	status       int
	body         string
	requestID    string
	contentType  string
	cacheControl string
}

func TestAppearanceReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"activeMarketDataProvider":"yfinance","exchangeCalendars":{"autoRefreshEnabled":false},"appearance":{"upColor":"#ABCDEF","downColor":"#a0b0c0"}}`), 0o600); err != nil {
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

	var snapshotMu sync.Mutex
	var snapshot appearanceWireSnapshot
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
			t.Errorf("call Go owner for appearance wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		got := appearanceWireSnapshot{
			status: ownerResponse.StatusCode, body: string(body),
			requestID:    ownerResponse.Header.Get("X-Request-ID"),
			contentType:  ownerResponse.Header.Get("Content-Type"),
			cacheControl: ownerResponse.Header.Get("Cache-Control"),
		}
		snapshotMu.Lock()
		snapshot = got
		snapshotMu.Unlock()
		w.Header().Set("Content-Type", got.contentType)
		w.Header().Set("Cache-Control", got.cacheControl)
		w.Header().Set("X-Request-ID", got.requestID)
		w.WriteHeader(got.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       appearanceRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   appearanceReadRouteOperations,
		RehearsalProxyTimeout: 500 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	t.Cleanup(func() {
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	})

	path := "/api/v1/settings/ui"
	response := appearanceRehearsalRequest(t, proxyServer.URL+path, "appearance-read-wire")
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read appearance rehearsal response: %v", err)
	}
	snapshotMu.Lock()
	want := snapshot
	snapshotMu.Unlock()
	if response.StatusCode != want.status || string(body) != want.body {
		t.Fatalf("wire mismatch: status/body = %d %q, want %d %q", response.StatusCode, body, want.status, want.body)
	}
	if response.Header.Get("X-Request-ID") != want.requestID ||
		response.Header.Get("Content-Type") != want.contentType ||
		response.Header.Get("Cache-Control") != want.cacheControl {
		t.Fatalf("wire header mismatch: %#v, want %#v", response.Header, want)
	}

	errorResponse := appearanceRehearsalRequest(t, proxyServer.URL+path+"?rehearsalFailure=error", "appearance-read-error")
	errorBody, _ := io.ReadAll(errorResponse.Body)
	_ = errorResponse.Body.Close()
	if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
		t.Fatalf("Rust error response was not preserved: %d %s", errorResponse.StatusCode, errorBody)
	}
	timeoutResponse := appearanceRehearsalRequest(t, proxyServer.URL+path+"?rehearsalFailure=timeout", "appearance-read-timeout")
	_ = timeoutResponse.Body.Close()
	if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("Rust timeout status = %d", timeoutResponse.StatusCode)
	}

	rust.Close()
	crashResponse := appearanceRehearsalRequest(t, proxyServer.URL+path, "appearance-read-crash")
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
	rollbackResponse := appearanceRehearsalRequest(t, restartedServer.URL+path, "appearance-read-rollback")
	_ = rollbackResponse.Body.Close()
	if rollbackResponse.StatusCode != http.StatusOK {
		t.Fatalf("Go owner after restart status = %d", rollbackResponse.StatusCode)
	}
}

func appearanceRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create appearance request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call appearance route: %v", err)
	}
	return response
}
