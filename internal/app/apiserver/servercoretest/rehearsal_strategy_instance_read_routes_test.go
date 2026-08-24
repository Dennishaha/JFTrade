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

var strategyInstanceReadRouteOperations = []string{
	"GET /api/v1/strategies",
	"GET /api/v1/strategies/{instanceId}/logs",
	"GET /api/v1/strategies/{instanceId}/audit",
}

var strategyInstanceReadWireHeaders = []string{
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

type strategyInstanceReadRehearsalTarget struct{ endpoint string }

func (t strategyInstanceReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (strategyInstanceReadRehearsalTarget) BearerToken() string { return strings.Repeat("i", 64) }

func (strategyInstanceReadRehearsalTarget) Profile() string { return rustrehearsal.ReadOnlyProfile }

func (strategyInstanceReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), strategyInstanceReadRouteOperations...)
}

type strategyInstanceReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestStrategyInstanceReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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
	snapshots := map[string]strategyInstanceReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("i", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("strategy instance read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for strategy instance wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := strategyInstanceReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: strategyInstanceReadHeaderSnapshot(ownerResponse.Header),
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
		RehearsalTarget:       strategyInstanceReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   strategyInstanceReadRouteOperations,
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
		"/api/v1/strategies",
		"/api/v1/strategies/missing/logs",
		"/api/v1/strategies/missing/audit",
		"/api/v1/strategies/missing/logs?limit=bad",
		"/api/v1/strategies/missing/audit?toTime=not-a-time",
	}
	for index, path := range paths {
		response := strategyInstanceReadRehearsalRequest(t, proxyServer.URL+path, "strategy-instance-wire-"+string(rune('a'+index)))
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read strategy instance rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertStrategyInstanceReadWire(t, path, response, body, want, false)
	}

	for index, operation := range strategyInstanceReadRouteOperations {
		path := strategyInstanceReadOperationPath(operation)
		errorResponse := strategyInstanceReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=error", "strategy-instance-error-"+string(rune('a'+index)),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := strategyInstanceReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=timeout", "strategy-instance-timeout-"+string(rune('a'+index)),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range strategyInstanceReadRouteOperations {
		path := strategyInstanceReadOperationPath(operation)
		crashResponse := strategyInstanceReadRehearsalRequest(t, proxyServer.URL+path, "strategy-instance-crash-"+string(rune('a'+index)))
		_ = crashResponse.Body.Close()
		if crashResponse.StatusCode != http.StatusBadGateway {
			t.Fatalf("Rust crash status for %s = %d", path, crashResponse.StatusCode)
		}
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
	for index, path := range paths {
		response := strategyInstanceReadRehearsalRequest(t, restartedServer.URL+path, "strategy-instance-wire-"+string(rune('a'+index)))
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		assertStrategyInstanceReadWire(t, path, response, body, want, true)
	}
}

func strategyInstanceReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(strategyInstanceReadWireHeaders))
	for _, name := range strategyInstanceReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertStrategyInstanceReadWire(t *testing.T, path string, response *http.Response, body []byte, want strategyInstanceReadWireSnapshot, normalizeBody bool) {
	t.Helper()
	gotBody, wantBody := string(body), want.body
	if normalizeBody {
		gotBody = normalizeStrategyInstanceReadEnvelope(body)
		wantBody = normalizeStrategyInstanceReadEnvelope([]byte(want.body))
	}
	if response.StatusCode != want.status || gotBody != wantBody {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := strategyInstanceReadHeaderSnapshot(response.Header)
	for _, name := range strategyInstanceReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func normalizeStrategyInstanceReadEnvelope(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	normalizeStrategyInstanceReadValue(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func normalizeStrategyInstanceReadValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "timestamp" || key == "observedAt" || key == "updatedAt" {
				current[key] = "fixture-time"
				continue
			}
			normalizeStrategyInstanceReadValue(child)
		}
	case []any:
		for _, child := range current {
			normalizeStrategyInstanceReadValue(child)
		}
	}
}

func strategyInstanceReadRehearsalRequest(t *testing.T, target, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create strategy instance read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call strategy instance read route: %v", err)
	}
	return response
}

func strategyInstanceReadOperationPath(operation string) string {
	switch operation {
	case "GET /api/v1/strategies":
		return "/api/v1/strategies"
	case "GET /api/v1/strategies/{instanceId}/logs":
		return "/api/v1/strategies/missing/logs"
	case "GET /api/v1/strategies/{instanceId}/audit":
		return "/api/v1/strategies/missing/audit"
	default:
		panic("unknown strategy instance read operation: " + operation)
	}
}
