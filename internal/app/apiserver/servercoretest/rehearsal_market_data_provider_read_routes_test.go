package servercoretest

import (
	"context"
	"encoding/json"
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

var marketDataProviderReadRouteOperations = []string{
	"GET /api/v1/market-data/provider",
}

var marketDataProviderReadWireHeaders = []string{
	"Cache-Control",
	"Content-Language",
	"Content-Type",
	"ETag",
	"Expires",
	"Last-Modified",
	"Retry-After",
	"Vary",
	"X-Content-Type-Options",
	"X-Request-ID",
}

type marketDataProviderReadRehearsalTarget struct {
	endpoint string
}

func (t marketDataProviderReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (marketDataProviderReadRehearsalTarget) BearerToken() string {
	return strings.Repeat("m", 64)
}

func (marketDataProviderReadRehearsalTarget) Profile() string {
	return rustrehearsal.ReadOnlyProfile
}

func (marketDataProviderReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), marketDataProviderReadRouteOperations...)
}

type marketDataProviderReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestMarketDataProviderReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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

	var snapshotsMu sync.Mutex
	snapshots := map[string]marketDataProviderReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("m", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("provider read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for provider read wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := marketDataProviderReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: marketDataProviderReadHeaderSnapshot(ownerResponse.Header),
		}
		snapshotsMu.Lock()
		snapshots[request.URL.RequestURI()] = snapshot
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
		RehearsalTarget:       marketDataProviderReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   marketDataProviderReadRouteOperations,
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
		"/api/v1/market-data/provider",
		"/api/v1/market-data/provider?fixture=degraded",
		"/api/v1/market-data/provider?fixture=error",
	}
	for index, path := range paths {
		response := marketDataProviderReadRehearsalRequest(t, proxyServer.URL+path, "provider-read-wire-"+string(rune('a'+index)))
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read provider rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertMarketDataProviderReadWire(t, path, response, body, want)
	}

	for index, operation := range marketDataProviderReadRouteOperations {
		path := marketDataProviderReadOperationPath(operation)
		errorResponse := marketDataProviderReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=error", "provider-read-error-"+string(rune('a'+index)),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := marketDataProviderReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=timeout", "provider-read-timeout-"+string(rune('a'+index)),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	path := marketDataProviderReadOperationPath(marketDataProviderReadRouteOperations[0])
	crashResponse := marketDataProviderReadRehearsalRequest(t, proxyServer.URL+path, "provider-read-crash")
	_ = crashResponse.Body.Close()
	if crashResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf("Rust crash status for %s = %d", path, crashResponse.StatusCode)
	}

	closeProxy()
	closeGoOwner()
	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after rehearsal: %v", err)
	}
	goAfterRestart := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(goAfterRestart)
	t.Cleanup(func() {
		restartedServer.Close()
		jftradeCheckTestError(t, goAfterRestart.Close())
	})
	rollbackResponse := marketDataProviderReadRehearsalRequest(t, restartedServer.URL+path, "provider-read-wire-a")
	rollbackBody, _ := io.ReadAll(rollbackResponse.Body)
	_ = rollbackResponse.Body.Close()
	snapshotsMu.Lock()
	want := snapshots[path]
	snapshotsMu.Unlock()
	if rollbackResponse.StatusCode != want.status ||
		normalizeMarketDataProviderReadEnvelope(rollbackBody) != normalizeMarketDataProviderReadEnvelope([]byte(want.body)) {
		t.Fatalf("Go rollback wire mismatch for %s: status/body = %d %q, want %d %q", path, rollbackResponse.StatusCode, rollbackBody, want.status, want.body)
	}
	assertMarketDataProviderReadHeaders(t, path, rollbackResponse.Header, want.headers)
}

func marketDataProviderReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(marketDataProviderReadWireHeaders))
	for _, name := range marketDataProviderReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertMarketDataProviderReadWire(t *testing.T, path string, response *http.Response, body []byte, want marketDataProviderReadWireSnapshot) {
	t.Helper()
	if response.StatusCode != want.status || string(body) != want.body {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	assertMarketDataProviderReadHeaders(t, path, response.Header, want.headers)
}

func assertMarketDataProviderReadHeaders(t *testing.T, path string, header http.Header, want map[string][]string) {
	t.Helper()
	got := marketDataProviderReadHeaderSnapshot(header)
	for _, name := range marketDataProviderReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want[name])
		}
	}
}

func normalizeMarketDataProviderReadEnvelope(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	normalizeMarketDataProviderReadValue(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func normalizeMarketDataProviderReadValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "checkedAt" || key == "reconciledAt" || key == "resolvedAt" || key == "timestamp" {
				current[key] = "fixture-time"
				continue
			}
			normalizeMarketDataProviderReadValue(child)
		}
	case []any:
		for _, child := range current {
			normalizeMarketDataProviderReadValue(child)
		}
	}
}

func marketDataProviderReadRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create provider read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call provider read route: %v", err)
	}
	return response
}

func marketDataProviderReadOperationPath(operation string) string {
	if operation == "GET /api/v1/market-data/provider" {
		return "/api/v1/market-data/provider"
	}
	panic("unknown market-data provider read operation: " + operation)
}
