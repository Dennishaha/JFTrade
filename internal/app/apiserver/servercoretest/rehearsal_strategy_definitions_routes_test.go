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
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

var strategyDefinitionsReadRouteOperations = []string{
	"GET /api/v1/strategy-definitions",
	"GET /api/v1/strategy-definitions/{definitionId}",
	"GET /api/v1/strategy-definitions/{definitionId}/versions",
	"GET /api/v1/strategy-definitions/{definitionId}/versions/{version}",
}

var strategyDefinitionsReadWireHeaders = []string{
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

type strategyDefinitionsReadRehearsalTarget struct {
	endpoint string
}

func (t strategyDefinitionsReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (strategyDefinitionsReadRehearsalTarget) BearerToken() string {
	return strings.Repeat("s", 64)
}

func (strategyDefinitionsReadRehearsalTarget) Profile() string {
	return rustrehearsal.ReadOnlyProfile
}

func (strategyDefinitionsReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), strategyDefinitionsReadRouteOperations...)
}

type strategyDefinitionsReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestStrategyDefinitionsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", "")
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
	seedStrategyDefinitionsReadOwner(t, settingsPath)

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
	snapshots := map[string]strategyDefinitionsReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("strategy definitions rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for strategy definition wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := strategyDefinitionsReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: strategyDefinitionsReadHeaderSnapshot(ownerResponse.Header),
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
		RehearsalTarget:       strategyDefinitionsReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   strategyDefinitionsReadRouteOperations,
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
		"/api/v1/strategy-definitions",
		"/api/v1/strategy-definitions/fixture-current",
		"/api/v1/strategy-definitions/fixture-current?interval=1m&symbol=US.MSFT&useExtendedHours=true",
		"/api/v1/strategy-definitions/fixture-current?useExtendedHours=maybe",
		"/api/v1/strategy-definitions/missing-definition",
		"/api/v1/strategy-definitions/fixture-current/versions",
		"/api/v1/strategy-definitions/fixture-deleted/versions",
		"/api/v1/strategy-definitions/missing-definition/versions",
		"/api/v1/strategy-definitions/fixture-current/versions/0.1.0",
		"/api/v1/strategy-definitions/fixture-current/versions/9.9.9",
	}
	for index, path := range paths {
		requestID := "strategy-definitions-read-wire-" + string(rune('a'+index))
		response := strategyDefinitionsReadRehearsalRequest(t, proxyServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read strategy definitions rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertStrategyDefinitionsReadWire(t, path, response, body, want)
	}

	for index, operation := range strategyDefinitionsReadRouteOperations {
		path := strategyDefinitionsReadOperationPath(operation)
		errorResponse := strategyDefinitionsReadRehearsalRequest(
			t,
			proxyServer.URL+path+strategyDefinitionsReadFailureQuery(path, "error"),
			"strategy-definitions-read-error-"+string(rune('a'+index)),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := strategyDefinitionsReadRehearsalRequest(
			t,
			proxyServer.URL+path+strategyDefinitionsReadFailureQuery(path, "timeout"),
			"strategy-definitions-read-timeout-"+string(rune('a'+index)),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range strategyDefinitionsReadRouteOperations {
		path := strategyDefinitionsReadOperationPath(operation)
		crashResponse := strategyDefinitionsReadRehearsalRequest(
			t,
			proxyServer.URL+path,
			"strategy-definitions-read-crash-"+string(rune('a'+index)),
		)
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
	defer restartedServer.Close()
	t.Cleanup(func() { jftradeCheckTestError(t, goAfterRestart.Close()) })
	for index, path := range paths {
		requestID := "strategy-definitions-read-wire-" + string(rune('a'+index))
		response := strategyDefinitionsReadRehearsalRequest(t, restartedServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read Go rollback response: %v", err)
		}
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		assertStrategyDefinitionsReadRollbackWire(t, path, response, body, want)
	}

	afterRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after rehearsal: %v", err)
	}
	if string(afterRequest) != string(beforeRequest) {
		t.Fatalf("strategy definitions read rehearsal modified settings: before=%q after=%q", beforeRequest, afterRequest)
	}
}

func seedStrategyDefinitionsReadOwner(t *testing.T, settingsPath string) {
	t.Helper()
	resource, err := strategystore.New(strategystore.DerivePath(settingsPath))
	if err != nil {
		t.Fatalf("open strategy definition store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resource.Close()) }()
	current := stratsrv.Definition{
		ID:           "fixture-current",
		Name:         "Fixture Current",
		Description:  "first saved description",
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "5m",
		Script:       "//@version=6\nstrategy(\"Fixture Current\", overlay=true)\nslow = ta.sma(close, 20)",
	}
	created, err := resource.SaveDefinition(current)
	if err != nil {
		t.Fatalf("save current strategy definition: %v", err)
	}
	created.Description = "second saved description"
	if _, err := resource.SaveDefinition(created); err != nil {
		t.Fatalf("save current strategy definition version: %v", err)
	}
	archived := current
	archived.ID = "fixture-deleted"
	archived.Name = "Fixture Deleted"
	archived.Script = "//@version=6\nstrategy(\"Fixture Deleted\")"
	deleted, err := resource.SaveDefinition(archived)
	if err != nil {
		t.Fatalf("save deleted strategy definition: %v", err)
	}
	if _, err := resource.DeleteDefinition(deleted.ID); err != nil {
		t.Fatalf("soft-delete strategy definition: %v", err)
	}
}

func strategyDefinitionsReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(strategyDefinitionsReadWireHeaders))
	for _, name := range strategyDefinitionsReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertStrategyDefinitionsReadWire(
	t *testing.T,
	path string,
	response *http.Response,
	body []byte,
	want strategyDefinitionsReadWireSnapshot,
) {
	t.Helper()
	if response.StatusCode != want.status || string(body) != want.body {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := strategyDefinitionsReadHeaderSnapshot(response.Header)
	for _, name := range strategyDefinitionsReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func assertStrategyDefinitionsReadRollbackWire(
	t *testing.T,
	path string,
	response *http.Response,
	body []byte,
	want strategyDefinitionsReadWireSnapshot,
) {
	t.Helper()
	if response.StatusCode != want.status ||
		normalizeStrategyDefinitionsReadEnvelope(body) != normalizeStrategyDefinitionsReadEnvelope([]byte(want.body)) {
		t.Fatalf("rollback wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := strategyDefinitionsReadHeaderSnapshot(response.Header)
	for _, name := range strategyDefinitionsReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("rollback wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func normalizeStrategyDefinitionsReadEnvelope(body []byte) string {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return string(body)
	}
	delete(envelope, "timestamp")
	normalized, err := json.Marshal(envelope)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func strategyDefinitionsReadRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create strategy definitions read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call strategy definitions read route: %v", err)
	}
	return response
}

func strategyDefinitionsReadOperationPath(operation string) string {
	switch operation {
	case "GET /api/v1/strategy-definitions":
		return "/api/v1/strategy-definitions"
	case "GET /api/v1/strategy-definitions/{definitionId}":
		return "/api/v1/strategy-definitions/fixture-current"
	case "GET /api/v1/strategy-definitions/{definitionId}/versions":
		return "/api/v1/strategy-definitions/fixture-current/versions"
	case "GET /api/v1/strategy-definitions/{definitionId}/versions/{version}":
		return "/api/v1/strategy-definitions/fixture-current/versions/0.1.0"
	default:
		panic("unknown strategy definitions read operation: " + operation)
	}
}

func strategyDefinitionsReadFailureQuery(path string, failure string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return separator + "rehearsalFailure=" + failure
}
