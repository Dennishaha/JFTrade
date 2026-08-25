package servercoretest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

var backtestsWriteRehearsalOperations = []string{
	"POST /api/v1/backtests",
	"POST /api/v1/backtests/sync",
	"DELETE /api/v1/backtests/sync/{taskId}",
	"DELETE /api/v1/backtests/{runId}",
}

type backtestsWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t backtestsWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t backtestsWriteRehearsalTarget) BearerToken() string { return t.token }

func (backtestsWriteRehearsalTarget) Profile() string {
	return "backtests-write-test-cutover.v1"
}

func (backtestsWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), backtestsWriteRehearsalOperations...)
}

func TestBacktestsWriteRehearsalPreservesAuthenticatedBoundaryAndRecoversAcrossRestart(t *testing.T) {
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
	if err := store.SaveBacktestMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveBacktestMarketDataProvider: %v", err)
	}
	settingsBefore, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings before backtests-write rehearsal: %v", err)
	}

	goOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	goServer := httptest.NewServer(goOwner.WebAccessHandler())
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

	token := strings.Repeat("b", 64)
	const browserCookie = "jftrade_web_session=backtests-write"
	const browserCSRF = "backtests-write-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertBacktestsWritePrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := request.Method + " " + request.URL.Path
		if !containsBacktestsWriteOperation(operation) {
			t.Errorf("unexpected Rust backtests-write operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust backtests-write body: %v", err)
		}
		if err := assertBacktestsWriteRequest(request.Method, request.URL.Path, body); err != nil {
			t.Error(err)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"BACKTEST_WRITE_FIXTURE_ERROR","message":"fixture conflict"}}`))
		case "timeout":
			<-request.Context().Done()
		case "cancel":
			cancelReadyOnce.Do(func() { close(cancelReady) })
			<-request.Context().Done()
			cancelDoneOnce.Do(func() { close(cancelDone) })
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"accepted":true,"source":"rust-rehearsal"},"timestamp":"fixture-time"}`))
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       backtestsWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   backtestsWriteRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner.WebAccessHandler())
	proxyClosed := false
	closeProxyOwner := func() {
		if proxyClosed {
			return
		}
		proxyClosed = true
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	}
	t.Cleanup(closeProxyOwner)
	expectedOrigin = proxyServer.URL

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/backtests", `{"definitionId":"def-1","market":"US","code":"AAPL"}`},
		{http.MethodPost, "/api/v1/backtests", `{"definitionId":"def-1","market":"US","code":"AAPL"}`},
		{http.MethodPost, "/api/v1/backtests/sync", `{"market":"US","symbol":"AAPL"}`},
		{http.MethodDelete, "/api/v1/backtests/sync/fixture-task?rehearsalFailure=error", ""},
		{http.MethodDelete, "/api/v1/backtests/fixture-run", ""},
	}
	for index, requestCase := range requests {
		response := backtestsWriteRehearsalRequest(
			t, proxyServer.URL+requestCase.path, requestCase.method, requestCase.body,
			"backtests-write-success-"+string(rune('1'+index)), proxyServer.URL, browserCookie, browserCSRF,
		)
		if index == 3 {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusConflict {
				t.Fatalf("backtests-write fixture error status = %d", response.StatusCode)
			}
			continue
		}
		assertBacktestsWriteSuccess(t, response, requestCase.path)
	}

	timeoutResponse := backtestsWriteRehearsalRequest(
		t, proxyServer.URL+"/api/v1/backtests/sync?rehearsalFailure=timeout", http.MethodPost,
		`{"market":"US"}`, "backtests-write-timeout", proxyServer.URL, browserCookie, browserCSRF,
	)
	assertBacktestsWriteError(t, timeoutResponse, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := backtestsWriteRehearsalDo(
			cancelContext,
			proxyServer.URL+"/api/v1/backtests/sync/fixture-task?rehearsalFailure=cancel",
			http.MethodDelete, "", "backtests-write-cancel", proxyServer.URL, browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust backtests-write rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("backtests-write cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("backtests-write cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust backtests-write cancellation was not observed")
	}

	if boundaryCalls.Load() != int32(len(requests)+2) {
		t.Fatalf("authenticated backtests-write boundary calls = %d, want %d", boundaryCalls.Load(), len(requests)+2)
	}
	rust.Close()
	crashResponse := backtestsWriteRehearsalRequest(
		t, proxyServer.URL+"/api/v1/backtests/sync/missing-task", http.MethodDelete,
		"", "backtests-write-crash", proxyServer.URL, browserCookie, browserCSRF,
	)
	assertBacktestsWriteError(t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")

	goResponse := backtestsWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/backtests/sync/missing-task", http.MethodDelete,
		"", "backtests-write-go-rollback", goServer.URL, browserCookie, browserCSRF,
	)
	assertBacktestsWriteGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after backtests-write rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := backtestsWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/backtests/sync/missing-task", http.MethodDelete,
		"", "backtests-write-go-restart", restartedServer.URL, browserCookie, browserCSRF,
	)
	assertBacktestsWriteGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after backtests-write rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("backtests-write rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertBacktestsWritePrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust backtests-write private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/backtests",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust backtests-write boundary %s = %q, want %q", name, got, want)
		}
	}
}

func assertBacktestsWriteRequest(method, path string, body []byte) error {
	if strings.HasPrefix(path, "/api/v1/backtests") && len(body) > 0 && !json.Valid(body) {
		return errors.New("backtests-write rehearsal body is not valid JSON")
	}
	if method == http.MethodDelete && strings.HasPrefix(path, "/api/v1/backtests/") && len(body) != 0 {
		return errors.New("backtests-write delete forwarded an unexpected body: " + string(body))
	}
	return nil
}

func containsBacktestsWriteOperation(operation string) bool {
	method, path, ok := strings.Cut(operation, " ")
	if !ok {
		return false
	}
	for _, candidate := range backtestsWriteRehearsalOperations {
		candidateMethod, candidatePath, candidateOK := strings.Cut(candidate, " ")
		if !candidateOK || method != candidateMethod {
			continue
		}
		if candidatePath == path ||
			(candidatePath == "/api/v1/backtests/sync/{taskId}" && strings.HasPrefix(path, "/api/v1/backtests/sync/")) ||
			(candidatePath == "/api/v1/backtests/{runId}" && strings.HasPrefix(path, "/api/v1/backtests/") && !strings.HasPrefix(path, "/api/v1/backtests/sync/")) {
			return true
		}
	}
	return false
}

func backtestsWriteRehearsalRequest(t *testing.T, target, method, body, requestID, origin, cookie, csrf string) *http.Response {
	t.Helper()
	response, err := backtestsWriteRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call backtests-write rehearsal route: %v", err)
	}
	return response
}

func backtestsWriteRehearsalDo(ctx context.Context, target, method, body, requestID, origin, cookie, csrf string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-backtests-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/backtests")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertBacktestsWriteSuccess(t *testing.T, response *http.Response, path string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read backtests-write success: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("backtests-write success for %s = %d %s", path, response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode backtests-write success for %s: %v; body=%s", path, err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["source"] != "rust-rehearsal" {
		t.Fatalf("backtests-write success for %s = %#v", path, envelope)
	}
}

func assertBacktestsWriteError(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read backtests-write error: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("backtests-write error status = %d, want %d; body=%s", response.StatusCode, wantStatus, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode backtests-write error: %v; body=%s", err, body)
	}
	if wantCode == "" {
		return
	}
	errorData, ok := envelope["error"].(map[string]any)
	if !ok || errorData["code"] != wantCode {
		t.Fatalf("backtests-write error envelope = %#v", envelope)
	}
}

func assertBacktestsWriteGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go backtests-write fallback: %v", err)
	}
	if strings.Contains(string(body), "rust-rehearsal") {
		t.Fatalf("Go backtests-write fallback unexpectedly used Rust owner: status=%d body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go backtests-write fallback: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok {
		t.Fatalf("Go backtests-write fallback error envelope = %#v", envelope)
	}
}
