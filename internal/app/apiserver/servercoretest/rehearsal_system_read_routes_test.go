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

var systemReadRouteOperations = []string{
	"GET /api/v1/system/futu-opend",
	"GET /api/v1/system/worker/broker-order-updates",
}

var systemReadWireHeaders = []string{
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

type systemReadRehearsalTarget struct{ endpoint string }

func (t systemReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (systemReadRehearsalTarget) BearerToken() string { return strings.Repeat("s", 64) }

func (systemReadRehearsalTarget) Profile() string { return rustrehearsal.ReadOnlyProfile }

func (systemReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), systemReadRouteOperations...)
}

type systemReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestSystemReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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
	snapshots := map[string]systemReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("system read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for system wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := systemReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: systemReadHeaderSnapshot(ownerResponse.Header),
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
		RehearsalTarget:       systemReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   systemReadRouteOperations,
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
		"/api/v1/system/futu-opend",
		"/api/v1/system/worker/broker-order-updates",
	}
	for index, path := range paths {
		response := systemReadRehearsalRequest(t, proxyServer.URL+path, "system-read-wire-"+string(rune('a'+index)))
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read system rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertSystemReadWire(t, path, response, body, want, false)
	}

	for index, operation := range systemReadRouteOperations {
		path := systemReadOperationPath(operation)
		errorResponse := systemReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=error", "system-read-error-"+string(rune('a'+index)),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := systemReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=timeout", "system-read-timeout-"+string(rune('a'+index)),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range systemReadRouteOperations {
		path := systemReadOperationPath(operation)
		crashResponse := systemReadRehearsalRequest(t, proxyServer.URL+path, "system-read-crash-"+string(rune('a'+index)))
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
		response := systemReadRehearsalRequest(t, restartedServer.URL+path, "system-read-wire-"+string(rune('a'+index)))
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		assertSystemReadWire(t, path, response, body, want, true)
	}
}

func systemReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(systemReadWireHeaders))
	for _, name := range systemReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertSystemReadWire(t *testing.T, path string, response *http.Response, body []byte, want systemReadWireSnapshot, normalizeBody bool) {
	t.Helper()
	gotBody, wantBody := string(body), want.body
	if normalizeBody {
		gotBody = normalizeSystemReadEnvelope(body)
		wantBody = normalizeSystemReadEnvelope([]byte(want.body))
	}
	if response.StatusCode != want.status || gotBody != wantBody {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := systemReadHeaderSnapshot(response.Header)
	for _, name := range systemReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func normalizeSystemReadEnvelope(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	normalizeSystemReadValue(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func normalizeSystemReadValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lowerKey := strings.ToLower(key)
			if child != nil && (lowerKey == "timestamp" || strings.HasSuffix(lowerKey, "at") || strings.HasSuffix(lowerKey, "retryafter")) {
				current[key] = "fixture-time"
				continue
			}
			normalizeSystemReadValue(child)
		}
	case []any:
		for _, child := range current {
			normalizeSystemReadValue(child)
		}
	}
}

func systemReadRehearsalRequest(t *testing.T, target, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create system read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call system read route: %v", err)
	}
	return response
}

func systemReadOperationPath(operation string) string {
	switch operation {
	case "GET /api/v1/system/futu-opend":
		return "/api/v1/system/futu-opend"
	case "GET /api/v1/system/worker/broker-order-updates":
		return "/api/v1/system/worker/broker-order-updates"
	default:
		panic("unknown system read operation: " + operation)
	}
}
