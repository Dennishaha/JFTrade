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

var portfolioReadRouteOperations = []string{
	"GET /api/v1/portfolio/{brokerId}/cash-balances",
	"GET /api/v1/portfolio/{brokerId}/positions",
}

var portfolioReadWireHeaders = []string{
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

type portfolioReadRehearsalTarget struct{ endpoint string }

func (t portfolioReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (portfolioReadRehearsalTarget) BearerToken() string { return strings.Repeat("o", 64) }

func (portfolioReadRehearsalTarget) Profile() string { return rustrehearsal.ReadOnlyProfile }

func (portfolioReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), portfolioReadRouteOperations...)
}

type portfolioReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestPortfolioReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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
	snapshots := map[string]portfolioReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("o", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("portfolio read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for portfolio wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := portfolioReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: portfolioReadHeaderSnapshot(ownerResponse.Header),
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
		RehearsalTarget:       portfolioReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   portfolioReadRouteOperations,
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
		"/api/v1/portfolio/fixture/cash-balances",
		"/api/v1/portfolio/fixture/positions",
	}
	for index, path := range paths {
		response := portfolioReadRehearsalRequest(t, proxyServer.URL+path, "portfolio-read-wire-"+string(rune('a'+index)))
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read portfolio rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertPortfolioReadWire(t, path, response, body, want)
	}

	for index, operation := range portfolioReadRouteOperations {
		path := portfolioReadOperationPath(operation)
		errorResponse := portfolioReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=error", "portfolio-read-error-"+string(rune('a'+index)),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := portfolioReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=timeout", "portfolio-read-timeout-"+string(rune('a'+index)),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range portfolioReadRouteOperations {
		path := portfolioReadOperationPath(operation)
		crashResponse := portfolioReadRehearsalRequest(t, proxyServer.URL+path, "portfolio-read-crash-"+string(rune('a'+index)))
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
		rollbackResponse := portfolioReadRehearsalRequest(t, restartedServer.URL+path, "portfolio-read-wire-"+string(rune('a'+index)))
		rollbackBody, _ := io.ReadAll(rollbackResponse.Body)
		_ = rollbackResponse.Body.Close()
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		if rollbackResponse.StatusCode != want.status ||
			normalizePortfolioReadEnvelope(rollbackBody) != normalizePortfolioReadEnvelope([]byte(want.body)) {
			t.Fatalf("Go rollback wire mismatch for %s: status/body = %d %q, want %d %q", path, rollbackResponse.StatusCode, rollbackBody, want.status, want.body)
		}
		assertPortfolioReadHeaders(t, path, rollbackResponse.Header, want.headers)
	}
}

func portfolioReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(portfolioReadWireHeaders))
	for _, name := range portfolioReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertPortfolioReadWire(t *testing.T, path string, response *http.Response, body []byte, want portfolioReadWireSnapshot) {
	t.Helper()
	if response.StatusCode != want.status || string(body) != want.body {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	assertPortfolioReadHeaders(t, path, response.Header, want.headers)
}

func assertPortfolioReadHeaders(t *testing.T, path string, header http.Header, want map[string][]string) {
	t.Helper()
	got := portfolioReadHeaderSnapshot(header)
	for _, name := range portfolioReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want[name])
		}
	}
}

func normalizePortfolioReadEnvelope(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	normalizePortfolioReadValue(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func normalizePortfolioReadValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "checkedAt" || key == "timestamp" {
				current[key] = "fixture-time"
				continue
			}
			normalizePortfolioReadValue(child)
		}
	case []any:
		for _, child := range current {
			normalizePortfolioReadValue(child)
		}
	}
}

func portfolioReadRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create portfolio read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call portfolio read route: %v", err)
	}
	return response
}

func portfolioReadOperationPath(operation string) string {
	switch operation {
	case "GET /api/v1/portfolio/{brokerId}/cash-balances":
		return "/api/v1/portfolio/fixture/cash-balances"
	case "GET /api/v1/portfolio/{brokerId}/positions":
		return "/api/v1/portfolio/fixture/positions"
	default:
		panic("unknown portfolio read operation: " + operation)
	}
}
