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
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/chart"
)

var backtestsReadRouteOperations = []string{
	"GET /api/v1/backtests",
	"GET /api/v1/backtests/{runId}/status",
	"GET /api/v1/backtests/{runId}",
	"GET /api/v1/backtests/sync/{taskId}",
}

var backtestsReadWireHeaders = []string{
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

type backtestsReadRehearsalTarget struct {
	endpoint string
}

func (t backtestsReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (backtestsReadRehearsalTarget) BearerToken() string {
	return strings.Repeat("b", 64)
}

func (backtestsReadRehearsalTarget) Profile() string {
	return rustrehearsal.ReadOnlyProfile
}

func (backtestsReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), backtestsReadRouteOperations...)
}

type backtestsReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestBacktestsReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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
	seedBacktestsReadOwner(t, store)

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
	snapshots := map[string]backtestsReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("b", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("backtests read rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for backtests wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := backtestsReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: backtestsReadHeaderSnapshot(ownerResponse.Header),
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
		RehearsalTarget:       backtestsReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   backtestsReadRouteOperations,
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
		"/api/v1/backtests",
		"/api/v1/backtests/fixture-run/status",
		"/api/v1/backtests/fixture-run",
		"/api/v1/backtests/%20/status",
		"/api/v1/backtests/%20",
		"/api/v1/backtests/sync/missing-task",
		"/api/v1/backtests/sync/%20",
	}
	for index, path := range paths {
		requestID := "backtests-read-wire-" + string(rune('a'+index))
		response := backtestsReadRehearsalRequest(t, proxyServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read backtests rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertBacktestsReadWire(t, path, response, body, want)
	}

	for index, operation := range backtestsReadRouteOperations {
		path := backtestsReadOperationPath(operation)
		errorResponse := backtestsReadRehearsalRequest(
			t,
			proxyServer.URL+path+backtestsReadFailureQuery(path, "error"),
			"backtests-read-error-"+string(rune('a'+index)),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := backtestsReadRehearsalRequest(
			t,
			proxyServer.URL+path+backtestsReadFailureQuery(path, "timeout"),
			"backtests-read-timeout-"+string(rune('a'+index)),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range backtestsReadRouteOperations {
		path := backtestsReadOperationPath(operation)
		crashResponse := backtestsReadRehearsalRequest(
			t,
			proxyServer.URL+path,
			"backtests-read-crash-"+string(rune('a'+index)),
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
	t.Cleanup(func() {
		restartedServer.Close()
		jftradeCheckTestError(t, goAfterRestart.Close())
	})
	for index, path := range paths {
		requestID := "backtests-read-wire-" + string(rune('a'+index))
		response := backtestsReadRehearsalRequest(t, restartedServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read Go rollback response: %v", err)
		}
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		assertBacktestsReadRollbackWire(t, path, response, body, want)
	}

	afterRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after rehearsal: %v", err)
	}
	if string(afterRequest) != string(beforeRequest) {
		t.Fatalf("backtests read rehearsal modified settings: before=%q after=%q", beforeRequest, afterRequest)
	}
}

func seedBacktestsReadOwner(t *testing.T, store *servercore.SettingsStore) {
	t.Helper()
	runStore, err := backteststore.New(backteststore.DerivePath(store.Path()))
	if err != nil {
		t.Fatalf("open backtest run store: %v", err)
	}
	t.Cleanup(func() {
		jftradeCheckTestError(t, runStore.Close())
	})
	useExtendedHours := false
	run := &btsrv.RunState{
		ID: "fixture-run", Status: "completed",
		Request: btsrv.StartRequest{
			DefinitionID: "fixture-strategy", DefinitionVersion: "v1", Market: "US", Code: "AAPL", Symbol: "US.AAPL",
			InstrumentType: "stock", Interval: "1d", StartDate: "2026-08-01", EndDate: "2026-08-15",
			MarketTimezone: "America/New_York", InitialBalance: 10000, RehabType: "none", UseExtendedHours: &useExtendedHours,
			ExecutionModel: "next_open", ChartType: chart.ChartTypeStandard,
		},
		Result: &bt.RunResult{
			Symbol: "US.AAPL", MarketDataProvider: "futu", Interval: "1d", StartTime: "2026-08-01T13:30:00Z", EndTime: "2026-08-15T20:00:00Z",
			QuoteCurrency: "USD", FinalBalance: 10500, PnL: 500, TotalTrades: 1, WinRate: 1, Logs: []string{"fixture complete"}, ChartType: chart.ChartTypeStandard,
		},
		CreatedAt: "2026-08-15T20:00:00Z", UpdatedAt: "2026-08-15T20:01:00Z", MarketDataProvider: "futu",
	}
	if err := runStore.Add(run); err != nil {
		t.Fatalf("seed backtest run: %v", err)
	}
}

func backtestsReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(backtestsReadWireHeaders))
	for _, name := range backtestsReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertBacktestsReadWire(
	t *testing.T,
	path string,
	response *http.Response,
	body []byte,
	want backtestsReadWireSnapshot,
) {
	t.Helper()
	if response.StatusCode != want.status || string(body) != want.body {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	assertBacktestsReadHeaders(t, path, response.Header, want.headers)
}

func assertBacktestsReadRollbackWire(
	t *testing.T,
	path string,
	response *http.Response,
	body []byte,
	want backtestsReadWireSnapshot,
) {
	t.Helper()
	if response.StatusCode != want.status ||
		normalizeBacktestsReadEnvelope(body) != normalizeBacktestsReadEnvelope([]byte(want.body)) {
		t.Fatalf("rollback wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	assertBacktestsReadHeaders(t, path, response.Header, want.headers)
}

func assertBacktestsReadHeaders(t *testing.T, path string, header http.Header, want map[string][]string) {
	t.Helper()
	got := backtestsReadHeaderSnapshot(header)
	for _, name := range backtestsReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want[name])
		}
	}
}

func normalizeBacktestsReadEnvelope(body []byte) string {
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

func backtestsReadRehearsalRequest(t *testing.T, target string, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create backtests read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call backtests read route: %v", err)
	}
	return response
}

func backtestsReadOperationPath(operation string) string {
	switch operation {
	case "GET /api/v1/backtests":
		return "/api/v1/backtests"
	case "GET /api/v1/backtests/{runId}/status":
		return "/api/v1/backtests/fixture-run/status"
	case "GET /api/v1/backtests/{runId}":
		return "/api/v1/backtests/fixture-run"
	case "GET /api/v1/backtests/sync/{taskId}":
		return "/api/v1/backtests/sync/missing-task"
	default:
		panic("unknown backtests read operation: " + operation)
	}
}

func backtestsReadFailureQuery(path string, failure string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return separator + "rehearsalFailure=" + failure
}
