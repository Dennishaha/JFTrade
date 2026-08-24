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
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

const watchlistsReadOperation = "GET /api/v1/watchlists/remote"

var watchlistsReadWireHeaders = []string{
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

type watchlistsReadRehearsalTarget struct{ endpoint string }

func (t watchlistsReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (watchlistsReadRehearsalTarget) BearerToken() string { return strings.Repeat("r", 64) }

func (watchlistsReadRehearsalTarget) Profile() string { return rustrehearsal.ReadOnlyProfile }

func (watchlistsReadRehearsalTarget) Capabilities() []string {
	return []string{watchlistsReadOperation}
}

type watchlistsReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestWatchlistsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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

	path := "/api/v1/watchlists/remote"
	var snapshot watchlistsReadWireSnapshot
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("r", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("watchlists read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for watchlists wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot = watchlistsReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: watchlistsReadHeaderSnapshot(ownerResponse.Header),
		}
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
		RehearsalTarget:       watchlistsReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   []string{watchlistsReadOperation},
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

	response := watchlistsReadRehearsalRequest(t, proxyServer.URL+path, "watchlists-read-wire")
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read watchlists rehearsal response: %v", err)
	}
	assertWatchlistsReadWire(t, path, response, body, snapshot, false)

	errorResponse := watchlistsReadRehearsalRequest(t, proxyServer.URL+path+"?rehearsalFailure=error", "watchlists-read-error")
	errorBody, _ := io.ReadAll(errorResponse.Body)
	_ = errorResponse.Body.Close()
	if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
		t.Fatalf("Rust error response was not preserved: %d %s", errorResponse.StatusCode, errorBody)
	}
	timeoutResponse := watchlistsReadRehearsalRequest(t, proxyServer.URL+path+"?rehearsalFailure=timeout", "watchlists-read-timeout")
	_ = timeoutResponse.Body.Close()
	if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("Rust timeout status = %d", timeoutResponse.StatusCode)
	}

	rust.Close()
	crashResponse := watchlistsReadRehearsalRequest(t, proxyServer.URL+path, "watchlists-read-crash")
	_ = crashResponse.Body.Close()
	if crashResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf("Rust crash status = %d", crashResponse.StatusCode)
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
	rollbackResponse := watchlistsReadRehearsalRequest(t, restartedServer.URL+path, "watchlists-read-wire")
	rollbackBody, _ := io.ReadAll(rollbackResponse.Body)
	_ = rollbackResponse.Body.Close()
	assertWatchlistsReadWire(t, path, rollbackResponse, rollbackBody, snapshot, true)
}

func watchlistsReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(watchlistsReadWireHeaders))
	for _, name := range watchlistsReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertWatchlistsReadWire(t *testing.T, path string, response *http.Response, body []byte, want watchlistsReadWireSnapshot, normalizeBody bool) {
	t.Helper()
	gotBody, wantBody := string(body), want.body
	if normalizeBody {
		gotBody = normalizeWatchlistsReadEnvelope(body)
		wantBody = normalizeWatchlistsReadEnvelope([]byte(want.body))
	}
	if response.StatusCode != want.status || gotBody != wantBody {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := watchlistsReadHeaderSnapshot(response.Header)
	for _, name := range watchlistsReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func normalizeWatchlistsReadEnvelope(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	normalizeWatchlistsReadValue(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func normalizeWatchlistsReadValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "timestamp" {
				current[key] = "fixture-time"
				continue
			}
			normalizeWatchlistsReadValue(child)
		}
	case []any:
		for _, child := range current {
			normalizeWatchlistsReadValue(child)
		}
	}
}

func watchlistsReadRehearsalRequest(t *testing.T, target, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create watchlists read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call watchlists read route: %v", err)
	}
	return response
}
