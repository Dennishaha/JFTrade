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

var authSessionWriteRehearsalOperations = []string{
	"POST /api/v1/auth/login",
	"POST /api/v1/auth/logout",
}

type authSessionWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t authSessionWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t authSessionWriteRehearsalTarget) BearerToken() string { return t.token }

func (authSessionWriteRehearsalTarget) Profile() string {
	return "auth-session-write-test-cutover.v1"
}

func (authSessionWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), authSessionWriteRehearsalOperations...)
}

func TestAuthSessionWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before auth rehearsal: %v", err)
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

	token := strings.Repeat("a", 64)
	const browserCookie = "jftrade_web_session=browser-rehearsal"
	const browserCSRF = "auth-session-write-csrf"
	const expectedLoginBody = `{"password":"fixture-password"}`
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	expectedOrigin := ""
	rustin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertAuthSessionWritePrivateBoundary(t, request, token, expectedOrigin, browserCookie, browserCSRF)
		if operation := request.Method + " " + request.URL.Path; !containsAuthSessionWriteOperation(operation) {
			t.Errorf("unexpected Rust auth-session operation: %q", operation)
		}
		requestBody, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust auth-session body: %v", err)
		}
		if request.URL.Path == "/api/v1/auth/login" && string(requestBody) != expectedLoginBody {
			t.Errorf("Rust auth-session login body = %q, want %q", requestBody, expectedLoginBody)
		}
		if request.URL.Path == "/api/v1/auth/logout" && string(requestBody) != "not-json" {
			t.Errorf("Rust auth-session logout body = %q, want malformed body", requestBody)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR","message":"fixture failure"}}`))
		case "rate":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"LOGIN_RATE_LIMITED","message":"too many failed login attempts"}}`))
		case "timeout":
			<-request.Context().Done()
		case "cancel":
			cancelReadyOnce.Do(func() { close(cancelReady) })
			<-request.Context().Done()
			cancelDoneOnce.Do(func() { close(cancelDone) })
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			if request.URL.Path == "/api/v1/auth/login" {
				w.Header().Set("Set-Cookie", "jftrade_web_session=rust-session; Path=/; Max-Age=43200; HttpOnly; SameSite=Strict")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true,"data":{"authenticated":true,"csrfToken":"rust-csrf","expiresAt":"fixture-time"},"timestamp":"2026-08-25T00:00:00Z"}`))
				return
			}
			w.Header().Set("Set-Cookie", "jftrade_web_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Strict")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"authenticated":false},"timestamp":"2026-08-25T00:00:00Z"}`))
		}
	}))
	defer rustin.Close()

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       authSessionWriteRehearsalTarget{endpoint: rustin.URL, token: token},
		RehearsalOperations:   authSessionWriteRehearsalOperations,
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

	origin := proxyServer.URL
	expectedOrigin = origin
	body := `{"password":"fixture-password"}`
	for index := 1; index <= 2; index++ {
		response := authSessionWriteRehearsalRequest(
			t,
			proxyServer.URL+"/api/v1/auth/login",
			body,
			"auth-session-write-login-"+string(rune('0'+index)),
			origin,
			browserCookie,
			browserCSRF,
		)
		assertAuthSessionWriteSuccess(
			t,
			response,
			http.StatusOK,
			"auth-session-write-login-"+string(rune('0'+index)),
			"jftrade_web_session=rust-session; Path=/; Max-Age=43200; HttpOnly; SameSite=Strict",
			true,
		)
	}

	response := authSessionWriteRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/auth/logout",
		"not-json",
		"auth-session-write-logout",
		origin,
		browserCookie,
		browserCSRF,
	)
	assertAuthSessionWriteSuccess(
		t,
		response,
		http.StatusOK,
		"auth-session-write-logout",
		"jftrade_web_session=; Path=/; Max-Age=0; HttpOnly; SameSite=Strict",
		false,
	)

	response = authSessionWriteRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/auth/login?rehearsalFailure=error",
		body,
		"auth-session-write-error",
		origin,
		browserCookie,
		browserCSRF,
	)
	assertAuthSessionWriteError(t, response, http.StatusInternalServerError, "auth-session-write-error", "RUST_FIXTURE_ERROR")

	response = authSessionWriteRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/auth/login?rehearsalFailure=rate",
		body,
		"auth-session-write-rate",
		origin,
		browserCookie,
		browserCSRF,
	)
	assertAuthSessionWriteError(t, response, http.StatusTooManyRequests, "auth-session-write-rate", "LOGIN_RATE_LIMITED")
	if got := response.Header.Get("Retry-After"); got != "300" {
		t.Fatalf("Rust rate-limit Retry-After = %q, want 300", got)
	}

	response = authSessionWriteRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/auth/login?rehearsalFailure=timeout",
		body,
		"auth-session-write-timeout",
		origin,
		browserCookie,
		browserCSRF,
	)
	assertAuthSessionWriteError(t, response, http.StatusGatewayTimeout, "auth-session-write-timeout", "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := authSessionWriteRehearsalDo(
			cancelContext,
			proxyServer.URL+"/api/v1/auth/login?rehearsalFailure=cancel",
			body,
			"auth-session-write-cancel",
			origin,
			browserCookie,
			browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust auth-session rehearsal did not receive cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("auth-session cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("auth-session cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust auth-session cancellation was not observed")
	}

	rustin.Close()
	response = authSessionWriteRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/auth/login",
		body,
		"auth-session-write-crash",
		origin,
		browserCookie,
		browserCSRF,
	)
	assertAuthSessionWriteError(t, response, http.StatusBadGateway, "auth-session-write-crash", "RUST_REHEARSAL_UNAVAILABLE")
	if boundaryCalls.Load() != 7 {
		t.Fatalf("authenticated auth-session boundary calls = %d, want 7", boundaryCalls.Load())
	}

	goResponse := authSessionWriteRehearsalRequest(
		t,
		goServer.URL+"/api/v1/auth/login",
		body,
		"auth-session-write-go-rollback",
		goServer.URL,
		browserCookie,
		browserCSRF,
	)
	assertAuthSessionWriteGoFallback(t, goResponse, "auth-session-write-go-rollback")
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after auth rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := authSessionWriteRehearsalRequest(
		t,
		restartedServer.URL+"/api/v1/auth/login",
		body,
		"auth-session-write-go-restart",
		restartedServer.URL,
		browserCookie,
		browserCSRF,
	)
	assertAuthSessionWriteGoFallback(t, restartedResponse, "auth-session-write-go-restart")
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after auth rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("auth-session rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertAuthSessionWritePrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust auth-session private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/auth",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust auth-session boundary %s = %q, want %q", name, got, want)
		}
	}
}

func containsAuthSessionWriteOperation(operation string) bool {
	return operation == authSessionWriteRehearsalOperations[0] || operation == authSessionWriteRehearsalOperations[1]
}

func authSessionWriteRehearsalRequest(t *testing.T, target, body, requestID, origin, cookie, csrf string) *http.Response {
	t.Helper()
	response, err := authSessionWriteRehearsalDo(
		context.Background(), target, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call auth-session rehearsal route: %v", err)
	}
	return response
}

func authSessionWriteRehearsalDo(ctx context.Context, target, body, requestID, origin, cookie, csrf string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-auth-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/auth")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertAuthSessionWriteSuccess(t *testing.T, response *http.Response, wantStatus int, requestID, cookie string, authenticated bool) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read auth-session rehearsal response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("auth-session rehearsal request %s status = %d, want %d; body=%s", requestID, response.StatusCode, wantStatus, body)
	}
	if response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("auth-session rehearsal request %s response ID = %q", requestID, response.Header.Get("X-Request-ID"))
	}
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Set-Cookie") != cookie {
		t.Fatalf("auth-session rehearsal request %s response headers = %#v", requestID, response.Header)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode auth-session rehearsal response %s: %v; body=%s", requestID, err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["authenticated"] != authenticated {
		t.Fatalf("auth-session rehearsal response %s = %#v", requestID, envelope)
	}
}

func assertAuthSessionWriteError(t *testing.T, response *http.Response, wantStatus int, requestID, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read auth-session rehearsal error: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf("auth-session rehearsal error %s = %d %s, want %d containing %q", requestID, response.StatusCode, body, wantStatus, wantCode)
	}
}

func assertAuthSessionWriteGoFallback(t *testing.T, response *http.Response, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go auth-session fallback response: %v", err)
	}
	if strings.Contains(string(body), "RUST_") || strings.Contains(string(body), "rust-session") {
		t.Fatalf("Go auth-session fallback request %s used Rust response: %d %s", requestID, response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go auth-session fallback response %s: %v; body=%s", requestID, err, body)
	}
	errorValue, ok := envelope["error"].(map[string]any)
	if !ok || errorValue["code"] != "WEB_ACCESS_DISABLED" {
		t.Fatalf("Go auth-session fallback envelope %s = %#v", requestID, envelope)
	}
}
