package servercoretest

import (
	"context"
	"encoding/json"
	"fmt"
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

var watchlistReadRouteOperations = []string{
	"GET /api/v1/watchlist/groups",
	"GET /api/v1/watchlist/items",
	"GET /api/v1/watchlist/sources",
	"GET /api/v1/watchlist/sources/{sourceId}/groups",
	"GET /api/v1/watchlist/bindings",
	"GET /api/v1/watchlist/import-runs",
}

var watchlistReadWireHeaders = []string{
	"Cache-Control",
	"Content-Language",
	"Content-Type",
	"ETag",
	"Expires",
	"Last-Modified",
	"Vary",
	"X-Content-Type-Options",
	"X-Request-ID",
}

type watchlistReadRehearsalTarget struct {
	endpoint string
}

func (t watchlistReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (watchlistReadRehearsalTarget) BearerToken() string {
	return strings.Repeat("w", 64)
}

func (watchlistReadRehearsalTarget) Profile() string {
	return rustrehearsal.ReadOnlyProfile
}

func (watchlistReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), watchlistReadRouteOperations...)
}

type watchlistReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestWatchlistReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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
	beforeRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings before rehearsal: %v", err)
	}

	var snapshotsMu sync.Mutex
	snapshots := map[string]watchlistReadWireSnapshot{}
	ownerCalls := 0
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("w", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("watchlist read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for watchlist wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := watchlistReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: watchlistReadHeaderSnapshot(ownerResponse.Header),
		}
		snapshotsMu.Lock()
		snapshots[request.URL.RequestURI()] = snapshot
		ownerCalls++
		snapshotsMu.Unlock()
		for name, values := range snapshot.headers {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(snapshot.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       watchlistReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   watchlistReadRouteOperations,
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
		"/api/v1/watchlist/groups",
		"/api/v1/watchlist/items?query=missing",
		"/api/v1/watchlist/items?limit=0",
		"/api/v1/watchlist/sources",
		"/api/v1/watchlist/sources/missing/groups",
		"/api/v1/watchlist/bindings?sourceId=missing",
		"/api/v1/watchlist/import-runs?limit=1",
	}
	for index, path := range paths {
		requestID := fmt.Sprintf("watchlist-read-wire-%d", index+1)
		response := watchlistReadRehearsalRequest(t, proxyServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read watchlist rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertWatchlistReadWire(t, path, response, body, want, false)
	}

	for index, operation := range watchlistReadRouteOperations {
		path := watchlistReadOperationPath(operation)
		errorResponse := watchlistReadRehearsalRequest(
			t, proxyServer.URL+path+watchlistReadFailureQuery(path, "error"),
			fmt.Sprintf("watchlist-read-error-%d", index+1),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := watchlistReadRehearsalRequest(
			t, proxyServer.URL+path+watchlistReadFailureQuery(path, "timeout"),
			fmt.Sprintf("watchlist-read-timeout-%d", index+1),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range watchlistReadRouteOperations {
		path := watchlistReadOperationPath(operation)
		crashResponse := watchlistReadRehearsalRequest(
			t, proxyServer.URL+path, fmt.Sprintf("watchlist-read-crash-%d", index+1),
		)
		_ = crashResponse.Body.Close()
		if crashResponse.StatusCode != http.StatusBadGateway {
			t.Fatalf("Rust crash status for %s = %d", path, crashResponse.StatusCode)
		}
	}
	snapshotsMu.Lock()
	if ownerCalls != len(paths) {
		t.Fatalf("failed Rust requests replayed Go: owner calls = %d, want %d", ownerCalls, len(paths))
	}
	snapshotsMu.Unlock()

	closeProxy()
	closeGoOwner()
	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after rehearsal: %v", err)
	}
	goAfterRestart := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(goAfterRestart)
	defer restartedServer.Close()
	t.Cleanup(func() { jftradeCheckTestError(t, goAfterRestart.Close()) })
	for index, path := range paths {
		requestID := fmt.Sprintf("watchlist-read-wire-%d", index+1)
		response := watchlistReadRehearsalRequest(t, restartedServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read Go rollback response: %v", err)
		}
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		assertWatchlistReadWire(t, path, response, body, want, true)
	}

	afterRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after rehearsal: %v", err)
	}
	if string(afterRequest) != string(beforeRequest) {
		t.Fatalf("watchlist read rehearsal modified settings: before=%q after=%q", beforeRequest, afterRequest)
	}
}

func watchlistReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(watchlistReadWireHeaders))
	for _, name := range watchlistReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertWatchlistReadWire(
	t *testing.T,
	path string,
	response *http.Response,
	body []byte,
	want watchlistReadWireSnapshot,
	normalizeBody bool,
) {
	t.Helper()
	gotBody, wantBody := string(body), want.body
	if normalizeBody {
		gotBody = normalizeWatchlistReadEnvelope(body)
		wantBody = normalizeWatchlistReadEnvelope([]byte(want.body))
	}
	if response.StatusCode != want.status || gotBody != wantBody {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := watchlistReadHeaderSnapshot(response.Header)
	for _, name := range watchlistReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func normalizeWatchlistReadEnvelope(body []byte) string {
	var envelope any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return string(body)
	}
	normalizeWatchlistReadValue(envelope)
	normalized, err := json.Marshal(envelope)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func normalizeWatchlistReadValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "timestamp")
		delete(typed, "updatedAt")
		delete(typed, "observedAt")
		for _, child := range typed {
			normalizeWatchlistReadValue(child)
		}
	case []any:
		for _, child := range typed {
			normalizeWatchlistReadValue(child)
		}
	}
}

func watchlistReadRehearsalRequest(t *testing.T, target, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create watchlist read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call watchlist read route: %v", err)
	}
	return response
}

func watchlistReadOperationPath(operation string) string {
	switch operation {
	case "GET /api/v1/watchlist/groups":
		return "/api/v1/watchlist/groups"
	case "GET /api/v1/watchlist/items":
		return "/api/v1/watchlist/items"
	case "GET /api/v1/watchlist/sources":
		return "/api/v1/watchlist/sources"
	case "GET /api/v1/watchlist/sources/{sourceId}/groups":
		return "/api/v1/watchlist/sources/missing/groups"
	case "GET /api/v1/watchlist/bindings":
		return "/api/v1/watchlist/bindings"
	case "GET /api/v1/watchlist/import-runs":
		return "/api/v1/watchlist/import-runs"
	default:
		panic("unknown watchlist read operation: " + operation)
	}
}

func watchlistReadFailureQuery(path, failure string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return separator + "rehearsalFailure=" + failure
}
