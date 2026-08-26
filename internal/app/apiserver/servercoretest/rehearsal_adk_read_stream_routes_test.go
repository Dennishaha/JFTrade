package servercoretest

import (
	"context"
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

var adkReadStreamRehearsalOperations = []string{
	"GET /api/v1/adk/streams/{streamId}",
	"GET /api/v1/adk/runs/{runId}/stream",
}

const adkReadStreamFixtureBody = "retry: 3000\n\nid: stream-fixture:1\ndata: {\"type\":\"final\",\"message\":\"fixture replay\"}\n\n"

type adkReadStreamRehearsalTarget struct{ endpoint string }

func (t adkReadStreamRehearsalTarget) Endpoint() string { return t.endpoint }

func (adkReadStreamRehearsalTarget) BearerToken() string { return strings.Repeat("s", 64) }

func (adkReadStreamRehearsalTarget) Profile() string { return rustrehearsal.ReadOnlyProfile }

func (adkReadStreamRehearsalTarget) Capabilities() []string {
	return append([]string(nil), adkReadStreamRehearsalOperations...)
}

func TestADKReadStreamRehearsalPreservesAuthenticatedSSEAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before ADK read rehearsal: %v", err)
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

	token := strings.Repeat("s", 64)
	const browserCookie = "jftrade_web_session=adk-read-rehearsal"
	const browserCSRF = "adk-read-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertADKReadStreamPrivateBoundary(t, request, token, expectedOrigin, browserCookie, browserCSRF)
		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR","message":"fixture stream failed"}}`)
		case "timeout":
			<-request.Context().Done()
		case "cancel":
			cancelReadyOnce.Do(func() { close(cancelReady) })
			<-request.Context().Done()
			cancelDoneOnce.Do(func() { close(cancelDone) })
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-ADK-Stream-ID", "stream-fixture")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, adkReadStreamFixtureBody)
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       adkReadStreamRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   adkReadStreamRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner.WebAccessHandler())
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
	expectedOrigin = proxyServer.URL

	for index, path := range []string{
		"/api/v1/adk/streams/stream-fixture?after=0",
		"/api/v1/adk/runs/run-fixture/stream?after=0",
	} {
		response := adkReadStreamRehearsalRequest(t, proxyServer.URL+path, "adk-read-stream-wire-"+string(rune('a'+index)), expectedOrigin, browserCookie, browserCSRF)
		assertADKReadStreamSSE(t, response)
	}

	for index, operation := range adkReadStreamRehearsalOperations {
		path := adkReadStreamOperationPath(operation)
		response := adkReadStreamRehearsalRequest(t, proxyServer.URL+adkReadStreamFailurePath(path, "error"), "adk-read-stream-error-"+string(rune('a'+index)), expectedOrigin, browserCookie, browserCSRF)
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust ADK read error for %s was not preserved: %d %s", path, response.StatusCode, body)
		}
		response = adkReadStreamRehearsalRequest(t, proxyServer.URL+adkReadStreamFailurePath(path, "timeout"), "adk-read-stream-timeout-"+string(rune('a'+index)), expectedOrigin, browserCookie, browserCSRF)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust ADK read timeout for %s = %d", path, response.StatusCode)
		}
	}

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := adkReadStreamRehearsalDo(cancelContext, proxyServer.URL+adkReadStreamFailurePath("/api/v1/adk/streams/stream-fixture?after=0", "cancel"), "adk-read-stream-cancel", expectedOrigin, browserCookie, browserCSRF)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust ADK read stream rehearsal did not receive cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("ADK read stream cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("ADK read stream cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust ADK read stream cancellation was not observed")
	}

	rust.Close()
	for index, operation := range adkReadStreamRehearsalOperations {
		response := adkReadStreamRehearsalRequest(t, proxyServer.URL+adkReadStreamOperationPath(operation), "adk-read-stream-crash-"+string(rune('a'+index)), expectedOrigin, browserCookie, browserCSRF)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadGateway {
			t.Fatalf("Rust ADK read crash for %s = %d", operation, response.StatusCode)
		}
	}
	if boundaryCalls.Load() != 7 {
		t.Fatalf("authenticated ADK read stream boundary calls = %d, want 7", boundaryCalls.Load())
	}

	closeProxy()
	closeGoOwner()
	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after ADK read rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	response := adkReadStreamRehearsalRequest(t, restartedServer.URL+"/api/v1/adk/streams/missing", "adk-read-stream-go-restart", restartedServer.URL, browserCookie, browserCSRF)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go ADK read restart response: %v", err)
	}
	if response.StatusCode != http.StatusNotFound || !strings.Contains(string(body), `"code":"NOT_FOUND"`) {
		t.Fatalf("Go ADK read restart response = %d %s", response.StatusCode, body)
	}
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after ADK read rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("ADK read rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertADKReadStreamPrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Method != http.MethodGet || request.ContentLength > 0 {
		t.Errorf("ADK read stream rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
	}
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust ADK read stream private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Accept":       "text/event-stream",
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/adk",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust ADK read stream boundary %s = %q, want %q", name, got, want)
		}
	}
}

func adkReadStreamRehearsalRequest(t *testing.T, target, requestID, origin, cookie, csrf string) *http.Response {
	t.Helper()
	response, err := adkReadStreamRehearsalDo(context.Background(), target, requestID, origin, cookie, csrf)
	if err != nil {
		t.Fatalf("call ADK read stream rehearsal: %v", err)
	}
	return response
}

func adkReadStreamRehearsalDo(ctx context.Context, target, requestID, origin, cookie, csrf string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-adk-read-token")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/adk")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertADKReadStreamSSE(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read ADK read SSE response: %v", err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("ADK read SSE response = %d %#v %s", response.StatusCode, response.Header, body)
	}
	for name, want := range map[string]string{
		"Cache-Control":   "no-cache",
		"X-ADK-Stream-ID": "stream-fixture",
	} {
		if got := response.Header.Get(name); got != want {
			t.Fatalf("ADK read SSE header %s = %q, want %q", name, got, want)
		}
	}
	if string(body) != adkReadStreamFixtureBody {
		t.Fatalf("ADK read SSE body = %q, want %q", body, adkReadStreamFixtureBody)
	}
}

func adkReadStreamOperationPath(operation string) string {
	switch operation {
	case "GET /api/v1/adk/streams/{streamId}":
		return "/api/v1/adk/streams/stream-fixture?after=0"
	case "GET /api/v1/adk/runs/{runId}/stream":
		return "/api/v1/adk/runs/run-fixture/stream?after=0"
	default:
		panic("unknown ADK read stream operation: " + operation)
	}
}

func adkReadStreamFailurePath(path, failure string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "rehearsalFailure=" + failure
}
