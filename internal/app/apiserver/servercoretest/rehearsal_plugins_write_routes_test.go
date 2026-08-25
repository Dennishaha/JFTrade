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

var pluginsWriteRehearsalOperations = []string{
	"POST /api/v1/plugins/{pluginId}/install",
	"POST /api/v1/plugins/{pluginId}/uninstall",
}

type pluginsWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t pluginsWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t pluginsWriteRehearsalTarget) BearerToken() string { return t.token }

func (pluginsWriteRehearsalTarget) Profile() string {
	return "plugins-write-test-cutover.v1"
}

func (pluginsWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), pluginsWriteRehearsalOperations...)
}

func TestPluginsWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before plugins rehearsal: %v", err)
	}

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

	token := strings.Repeat("p", 64)
	var boundaryCalls atomic.Int32
	var cancelObserved atomic.Int32
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	var proxyOrigin string
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertPluginsWritePrivateBoundary(t, request, token, proxyOrigin)
		operation := request.Method + " " + request.URL.Path
		if !containsPluginsWriteOperation(operation) {
			t.Errorf("unexpected Rust plugins mutation operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read plugins mutation body: %v", err)
		}
		if string(body) != "not-json" {
			t.Errorf("plugins mutation body = %q, want forwarded arbitrary body", body)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR","message":"fixture failure"}}`))
		case "timeout":
			select {
			case <-request.Context().Done():
			case <-time.After(250 * time.Millisecond):
			}
		case "cancel":
			cancelReadyOnce.Do(func() { close(cancelReady) })
			<-request.Context().Done()
			cancelObserved.Add(1)
			cancelDoneOnce.Do(func() { close(cancelDone) })
		default:
			phase := "installed"
			if strings.HasSuffix(request.URL.Path, "/uninstall") {
				phase = "uninstalled"
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"operation":{"operationId":"plugins-rehearsal","pluginId":"alpha","status":"SUCCEEDED","phase":"` + phase + `","progress":100,"message":"plugin metadata ` + phase + `","targetDir":"plugins","installPath":"plugins/alpha.so","startedAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:00Z","completedAt":"2026-08-25T00:00:00Z","error":null}},"timestamp":"2026-08-25T00:00:00Z"}`))
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       pluginsWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   pluginsWriteRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
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
	proxyOrigin = proxyServer.URL

	for index, path := range []string{
		"/api/v1/plugins/alpha/install",
		"/api/v1/plugins/alpha/uninstall",
		"/api/v1/plugins/alpha/install",
	} {
		response := pluginsWriteRehearsalRequest(
			t, proxyServer.URL+path, "not-json", "plugins-write-success-"+string(rune('1'+index)), proxyOrigin,
		)
		assertPluginsWriteResponse(t, response, http.StatusOK, "plugins-write-success-"+string(rune('1'+index)))
	}

	response := pluginsWriteRehearsalRequest(
		t, proxyServer.URL+"/api/v1/plugins/alpha/install?rehearsalFailure=error", "not-json", "plugins-write-error", proxyOrigin,
	)
	assertPluginsWriteError(t, response, http.StatusUnprocessableEntity, "plugins-write-error", "RUST_FIXTURE_ERROR")
	response = pluginsWriteRehearsalRequest(
		t, proxyServer.URL+"/api/v1/plugins/alpha/install?rehearsalFailure=timeout", "not-json", "plugins-write-timeout", proxyOrigin,
	)
	assertPluginsWriteError(t, response, http.StatusGatewayTimeout, "plugins-write-timeout", "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := pluginsWriteRehearsalDo(
			cancelContext,
			proxyServer.URL+"/api/v1/plugins/alpha/install?rehearsalFailure=cancel",
			"not-json",
			"plugins-write-cancel",
			proxyOrigin,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust plugins mutation rehearsal did not receive the cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("plugins mutation cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("plugins mutation cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatalf("Rust plugins cancellation observations = %d, want 1", cancelObserved.Load())
	}

	rust.Close()
	for index, path := range []string{
		"/api/v1/plugins/alpha/install",
		"/api/v1/plugins/alpha/uninstall",
	} {
		crashResponse := pluginsWriteRehearsalRequest(
			t, proxyServer.URL+path, "not-json", "plugins-write-crash-"+string(rune('1'+index)), proxyOrigin,
		)
		assertPluginsWriteError(t, crashResponse, http.StatusBadGateway, "plugins-write-crash-"+string(rune('1'+index)), "RUST_REHEARSAL_UNAVAILABLE")
	}
	if boundaryCalls.Load() != 6 {
		t.Fatalf("authenticated plugins mutation boundary calls = %d, want 6", boundaryCalls.Load())
	}

	goResponse := pluginsWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/plugins/missing/install", "not-json", "plugins-write-go-rollback", goServer.URL,
	)
	assertPluginsWriteGoFallback(t, goResponse, "plugins-write-go-rollback")
	closeGoOwner()
	closeProxy()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after plugins rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner)
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := pluginsWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/plugins/missing/uninstall", "not-json", "plugins-write-go-restart", restartedServer.URL,
	)
	assertPluginsWriteGoFallback(t, restartedResponse, "plugins-write-go-restart")
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after plugins rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("plugins rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertPluginsWritePrivateBoundary(t *testing.T, request *http.Request, token, origin string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
		t.Errorf("Rust private plugins mutation boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       "jftrade_web_session=browser-rehearsal",
		"Origin":       origin,
		"Referer":      origin + "/plugins",
		"X-CSRF-Token": "plugins-write-csrf",
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust plugins mutation boundary %s = %q, want %q", name, got, want)
		}
	}
}

func containsPluginsWriteOperation(operation string) bool {
	method, path, ok := strings.Cut(operation, " ")
	if !ok || method != http.MethodPost || !strings.HasPrefix(path, "/api/v1/plugins/alpha/") {
		return false
	}
	return strings.HasSuffix(path, "/install") || strings.HasSuffix(path, "/uninstall")
}

func pluginsWriteRehearsalRequest(t *testing.T, target, body, requestID, origin string) *http.Response {
	t.Helper()
	response, err := pluginsWriteRehearsalDo(
		context.Background(), target, body, requestID, origin,
	)
	if err != nil {
		t.Fatalf("call plugins mutation rehearsal route: %v", err)
	}
	return response
}

func pluginsWriteRehearsalDo(ctx context.Context, target, body, requestID, origin string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-desktop-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "jftrade_web_session=browser-rehearsal")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/plugins")
	request.Header.Set("X-CSRF-Token", "plugins-write-csrf")
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertPluginsWriteResponse(t *testing.T, response *http.Response, wantStatus int, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read plugins mutation rehearsal response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("plugins mutation rehearsal request %s status = %d, want %d; body=%s", requestID, response.StatusCode, wantStatus, body)
	}
	if response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("plugins mutation rehearsal request %s response ID = %q", requestID, response.Header.Get("X-Request-ID"))
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode plugins mutation rehearsal response %s: %v; body=%s", requestID, err, body)
	}
	if envelope["ok"] != true {
		t.Fatalf("plugins mutation rehearsal response %s envelope = %#v", requestID, envelope)
	}
}

func assertPluginsWriteError(t *testing.T, response *http.Response, wantStatus int, requestID, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read plugins mutation rehearsal error: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf("plugins mutation rehearsal error %s = %d %s, want %d containing %q", requestID, response.StatusCode, body, wantStatus, wantCode)
	}
	if response.Header.Get("X-Request-ID") != requestID && wantStatus != http.StatusGatewayTimeout {
		t.Fatalf("plugins mutation rehearsal error %s response ID = %q", requestID, response.Header.Get("X-Request-ID"))
	}
}

func assertPluginsWriteGoFallback(t *testing.T, response *http.Response, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go plugins fallback response: %v", err)
	}
	if response.StatusCode != http.StatusInternalServerError || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go plugins fallback response %s = %d %s", requestID, response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go plugins fallback response %s: %v; body=%s", requestID, err, body)
	}
	errorValue, ok := envelope["error"].(map[string]any)
	if !ok || errorValue["code"] != "INTERNAL_ERROR" {
		t.Fatalf("Go plugins fallback envelope %s = %#v", requestID, envelope)
	}
}
