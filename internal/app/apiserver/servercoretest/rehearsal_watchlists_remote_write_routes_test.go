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

var watchlistsRemoteWriteRehearsalOperations = []string{
	"POST /api/v1/watchlists/remote",
}

type watchlistsRemoteWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t watchlistsRemoteWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t watchlistsRemoteWriteRehearsalTarget) BearerToken() string { return t.token }

func (watchlistsRemoteWriteRehearsalTarget) Profile() string {
	return "watchlists-remote-write-test-cutover.v1"
}

func (watchlistsRemoteWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), watchlistsRemoteWriteRehearsalOperations...)
}

func TestWatchlistsRemoteWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before remote watchlist rehearsal: %v", err)
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

	token := strings.Repeat("w", 64)
	const body = `{"groupName":"Favorites","op":1,"securityList":[{"market":11,"code":"AAPL"}]}`
	var boundaryCalls atomic.Int32
	var cancelObserved atomic.Int32
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	var proxyOrigin string
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertWatchlistsRemoteWritePrivateBoundary(t, request, token, proxyOrigin)
		if operation := request.Method + " " + request.URL.Path; !containsWatchlistsRemoteWriteOperation(operation) {
			t.Errorf("unexpected Rust remote watchlist operation: %q", operation)
		}
		if got := request.URL.Query().Get("brokerId"); got != "futu" {
			t.Errorf("Rust remote watchlist brokerId = %q, want futu", got)
		}
		if got := request.URL.Query().Get("accountId"); got != "acct-1" {
			t.Errorf("Rust remote watchlist accountId = %q, want acct-1", got)
		}
		requestBody, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust remote watchlist body: %v", err)
		}
		if string(requestBody) != body {
			t.Errorf("Rust remote watchlist body = %q, want forwarded JSON body", requestBody)
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
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"accepted":true,"featureId":"watchlist.remote.modify","operation":"modify","provider":{"brokerId":"futu"}},"timestamp":"2026-08-25T00:00:00Z"}`))
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       watchlistsRemoteWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   watchlistsRemoteWriteRehearsalOperations,
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

	path := "/api/v1/watchlists/remote?brokerId=futu&accountId=acct-1"
	for index := range 2 {
		requestID := "watchlists-remote-write-success-" + string(rune('1'+index))
		response := watchlistsRemoteWriteRehearsalRequest(t, proxyServer.URL+path, body, requestID, proxyOrigin)
		assertWatchlistsRemoteWriteStatus(t, response, http.StatusOK, requestID)
	}

	response := watchlistsRemoteWriteRehearsalRequest(
		t, proxyServer.URL+path+"&rehearsalFailure=error", body, "watchlists-remote-write-error", proxyOrigin,
	)
	assertWatchlistsRemoteWriteStatus(t, response, http.StatusUnprocessableEntity, "watchlists-remote-write-error")
	response = watchlistsRemoteWriteRehearsalRequest(
		t, proxyServer.URL+path+"&rehearsalFailure=timeout", body, "watchlists-remote-write-timeout", proxyOrigin,
	)
	assertWatchlistsRemoteWriteStatus(t, response, http.StatusGatewayTimeout, "watchlists-remote-write-timeout")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := watchlistsRemoteWriteRehearsalDo(
			cancelContext,
			proxyServer.URL+path+"&rehearsalFailure=cancel",
			body,
			"watchlists-remote-write-cancel",
			proxyOrigin,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust remote watchlist rehearsal did not receive cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("remote watchlist cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("remote watchlist cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatalf("remote watchlist cancellation observations = %d, want 1", cancelObserved.Load())
	}

	rust.Close()
	crashResponse := watchlistsRemoteWriteRehearsalRequest(
		t, proxyServer.URL+path, body, "watchlists-remote-write-crash", proxyOrigin,
	)
	assertWatchlistsRemoteWriteError(t, crashResponse, "watchlists-remote-write-crash", "RUST_REHEARSAL_UNAVAILABLE")
	if boundaryCalls.Load() != 5 {
		t.Fatalf("authenticated remote watchlist boundary calls = %d, want 5", boundaryCalls.Load())
	}

	goResponse := watchlistsRemoteWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/watchlists/remote?brokerId=missing", body, "watchlists-remote-write-go-rollback", goServer.URL,
	)
	assertWatchlistsRemoteWriteGoFallback(t, goResponse, "watchlists-remote-write-go-rollback")
	closeGoOwner()
	closeProxy()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after remote watchlist rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner)
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := watchlistsRemoteWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/watchlists/remote?brokerId=missing", body, "watchlists-remote-write-go-restart", restartedServer.URL,
	)
	assertWatchlistsRemoteWriteGoFallback(t, restartedResponse, "watchlists-remote-write-go-restart")
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after remote watchlist rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("remote watchlist rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertWatchlistsRemoteWritePrivateBoundary(t *testing.T, request *http.Request, token, origin string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
		t.Errorf("Rust private remote watchlist boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       "jftrade_web_session=browser-rehearsal",
		"Origin":       origin,
		"Referer":      origin + "/watchlists",
		"X-CSRF-Token": "watchlists-remote-write-csrf",
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust remote watchlist boundary %s = %q, want %q", name, got, want)
		}
	}
}

func containsWatchlistsRemoteWriteOperation(operation string) bool {
	return operation == watchlistsRemoteWriteRehearsalOperations[0]
}

func watchlistsRemoteWriteRehearsalRequest(t *testing.T, target, body, requestID, origin string) *http.Response {
	t.Helper()
	response, err := watchlistsRemoteWriteRehearsalDo(
		context.Background(), target, body, requestID, origin,
	)
	if err != nil {
		t.Fatalf("call remote watchlist rehearsal route: %v", err)
	}
	return response
}

func watchlistsRemoteWriteRehearsalDo(ctx context.Context, target, body, requestID, origin string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-desktop-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "jftrade_web_session=browser-rehearsal")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/watchlists")
	request.Header.Set("X-CSRF-Token", "watchlists-remote-write-csrf")
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertWatchlistsRemoteWriteStatus(t *testing.T, response *http.Response, wantStatus int, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read remote watchlist rehearsal response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("remote watchlist rehearsal request %s status = %d, want %d; body=%s", requestID, response.StatusCode, wantStatus, body)
	}
	if response.Header.Get("X-Request-ID") != requestID && wantStatus != http.StatusGatewayTimeout {
		t.Fatalf("remote watchlist rehearsal request %s response ID = %q", requestID, response.Header.Get("X-Request-ID"))
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode remote watchlist rehearsal response %s: %v; body=%s", requestID, err, body)
	}
	if envelope["ok"] != (wantStatus == http.StatusOK) {
		t.Fatalf("remote watchlist rehearsal response %s envelope = %#v", requestID, envelope)
	}
}

func assertWatchlistsRemoteWriteError(t *testing.T, response *http.Response, requestID, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read remote watchlist rehearsal error: %v", err)
	}
	if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), wantCode) {
		t.Fatalf("remote watchlist rehearsal error %s = %d %s, want 502 containing %q", requestID, response.StatusCode, body, wantCode)
	}
}

func assertWatchlistsRemoteWriteGoFallback(t *testing.T, response *http.Response, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go remote watchlist fallback response: %v", err)
	}
	if response.StatusCode == http.StatusOK || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go remote watchlist fallback response %s = %d %s", requestID, response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go remote watchlist fallback response %s: %v; body=%s", requestID, err, body)
	}
	errorValue, ok := envelope["error"].(map[string]any)
	if !ok || errorValue["code"] != "BROKER_CAPABILITY_UNAVAILABLE" {
		t.Fatalf("Go remote watchlist fallback envelope %s = %#v", requestID, envelope)
	}
}
