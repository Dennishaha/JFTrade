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

var adkChatStreamRehearsalOperations = []string{
	"POST /api/v1/adk/chat",
	"POST /api/v1/adk/chat/stream",
}

type adkChatStreamRehearsalTarget struct {
	endpoint string
	token    string
}

func (t adkChatStreamRehearsalTarget) Endpoint() string { return t.endpoint }

func (t adkChatStreamRehearsalTarget) BearerToken() string { return t.token }

func (adkChatStreamRehearsalTarget) Profile() string {
	return "adk-chat-stream-test-cutover.v1"
}

func (adkChatStreamRehearsalTarget) Capabilities() []string {
	return append([]string(nil), adkChatStreamRehearsalOperations...)
}

func TestADKChatStreamRehearsalPreservesAuthenticatedSSEAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before ADK rehearsal: %v", err)
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
	const browserCSRF = "adk-chat-stream-csrf"
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	var expectedOrigin string
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertADKChatStreamPrivateBoundary(t, request, token, expectedOrigin, browserCookie, browserCSRF)
		operation := request.Method + " " + request.URL.Path
		if !containsADKChatStreamOperation(operation) {
			t.Errorf("unexpected Rust ADK chat-stream operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust ADK chat-stream body: %v", err)
		}
		if string(body) != `{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"hello"}` {
			t.Errorf("Rust ADK chat-stream body = %q", body)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if request.URL.Path == "/api/v1/adk/chat/stream" {
				w.Header().Set("X-ADK-Stream-Idle-Timeout-Ms", "420000")
			}
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"MODEL_CALL_FAILED","message":"fixture provider failed"}}`))
		case "timeout":
			<-request.Context().Done()
		case "cancel":
			cancelReadyOnce.Do(func() { close(cancelReady) })
			<-request.Context().Done()
			cancelDoneOnce.Do(func() { close(cancelDone) })
		default:
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			if request.URL.Path == "/api/v1/adk/chat/stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				w.Header().Set("X-ADK-Stream-ID", "stream-fixture")
				w.Header().Set("X-ADK-Stream-Idle-Timeout-Ms", "420000")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, "retry: 3000\n\nid: 1\ndata: {\"type\":\"final\",\"message\":\"fixture response\"}\n\n")
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"run":{"id":"run-fixture"},"message":"fixture response"},"timestamp":"fixture-time"}`))
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       adkChatStreamRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   adkChatStreamRehearsalOperations,
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
	body := `{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"hello"}`

	for index := 1; index <= 2; index++ {
		response := adkChatStreamRehearsalRequest(
			t,
			proxyServer.URL+"/api/v1/adk/chat",
			body,
			"adk-chat-success-"+string(rune('0'+index)),
			expectedOrigin,
			browserCookie,
			browserCSRF,
			"application/json",
		)
		assertADKChatStreamJSONSuccess(t, response, "adk-chat-success-"+string(rune('0'+index)))
	}

	streamResponse := adkChatStreamRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/adk/chat/stream",
		body,
		"adk-stream-success",
		expectedOrigin,
		browserCookie,
		browserCSRF,
		"text/event-stream",
	)
	assertADKChatStreamSSESuccess(t, streamResponse)

	for _, path := range []string{"/api/v1/adk/chat", "/api/v1/adk/chat/stream"} {
		response := adkChatStreamRehearsalRequest(
			t,
			proxyServer.URL+path+"?rehearsalFailure=error",
			body,
			"adk-error-"+strings.TrimPrefix(strings.TrimPrefix(path, "/api/v1/adk/"), "chat/"),
			expectedOrigin,
			browserCookie,
			browserCSRF,
			map[bool]string{true: "text/event-stream", false: "application/json"}[path == "/api/v1/adk/chat/stream"],
		)
		assertADKChatStreamError(t, response, http.StatusBadGateway, "MODEL_CALL_FAILED")
	}

	response := adkChatStreamRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/adk/chat/stream?rehearsalFailure=timeout",
		body,
		"adk-stream-timeout",
		expectedOrigin,
		browserCookie,
		browserCSRF,
		"text/event-stream",
	)
	assertADKChatStreamError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := adkChatStreamRehearsalDo(
			cancelContext,
			proxyServer.URL+"/api/v1/adk/chat/stream?rehearsalFailure=cancel",
			body,
			"adk-stream-cancel",
			expectedOrigin,
			browserCookie,
			browserCSRF,
			"text/event-stream",
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust ADK stream rehearsal did not receive cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("ADK stream cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("ADK stream cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust ADK stream cancellation was not observed")
	}

	rust.Close()
	response = adkChatStreamRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/adk/chat/stream",
		body,
		"adk-stream-crash",
		expectedOrigin,
		browserCookie,
		browserCSRF,
		"text/event-stream",
	)
	assertADKChatStreamError(t, response, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")
	if boundaryCalls.Load() != 7 {
		t.Fatalf("authenticated ADK chat-stream boundary calls = %d, want 7", boundaryCalls.Load())
	}

	goResponse := adkChatStreamRehearsalRequest(
		t,
		goServer.URL+"/api/v1/adk/chat/stream",
		body,
		"adk-stream-go-rollback",
		goServer.URL,
		browserCookie,
		browserCSRF,
		"text/event-stream",
	)
	assertADKChatStreamGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after ADK rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := adkChatStreamRehearsalRequest(
		t,
		restartedServer.URL+"/api/v1/adk/chat/stream",
		body,
		"adk-stream-go-restart",
		restartedServer.URL,
		browserCookie,
		browserCSRF,
		"text/event-stream",
	)
	assertADKChatStreamGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after ADK rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("ADK chat-stream rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertADKChatStreamPrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust ADK chat-stream private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/adk",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust ADK chat-stream boundary %s = %q, want %q", name, got, want)
		}
	}
}

func containsADKChatStreamOperation(operation string) bool {
	return operation == adkChatStreamRehearsalOperations[0] || operation == adkChatStreamRehearsalOperations[1]
}

func adkChatStreamRehearsalRequest(
	t *testing.T,
	target, body, requestID, origin, cookie, csrf, accept string,
) *http.Response {
	t.Helper()
	response, err := adkChatStreamRehearsalDo(
		context.Background(), target, body, requestID, origin, cookie, csrf, accept,
	)
	if err != nil {
		t.Fatalf("call ADK chat-stream rehearsal route: %v", err)
	}
	return response
}

func adkChatStreamRehearsalDo(
	ctx context.Context,
	target, body, requestID, origin, cookie, csrf, accept string,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-adk-token")
	request.Header.Set("Accept", accept)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/adk")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertADKChatStreamJSONSuccess(t *testing.T, response *http.Response, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read ADK chat response: %v", err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("ADK chat response %s = %d %#v %s", requestID, response.StatusCode, response.Header, body)
	}
	if !strings.Contains(string(body), `"run":{"id":"run-fixture"}`) {
		t.Fatalf("ADK chat response %s = %s", requestID, body)
	}
}

func assertADKChatStreamSSESuccess(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read ADK SSE response: %v", err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("ADK SSE response = %d %#v %s", response.StatusCode, response.Header, body)
	}
	// Go's client transport removes hop-by-hop Connection from Response.Header;
	// the rehearsal proxy recorder test checks the public wire header directly.
	for name, want := range map[string]string{
		"Cache-Control":                "no-cache",
		"X-ADK-Stream-ID":              "stream-fixture",
		"X-ADK-Stream-Idle-Timeout-Ms": "420000",
	} {
		if got := response.Header.Get(name); got != want {
			t.Fatalf("ADK SSE header %s = %q, want %q", name, got, want)
		}
	}
	wantBody := "retry: 3000\n\nid: 1\ndata: {\"type\":\"final\",\"message\":\"fixture response\"}\n\n"
	if string(body) != wantBody {
		t.Fatalf("ADK SSE body = %q, want %q", body, wantBody)
	}
}

func assertADKChatStreamError(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read ADK chat-stream error: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf("ADK chat-stream error = %d %s, want %d containing %q", response.StatusCode, body, wantStatus, wantCode)
	}
}

func assertADKChatStreamGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go ADK fallback: %v", err)
	}
	if response.StatusCode == http.StatusBadGateway || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go ADK fallback used Rust response: %d %s", response.StatusCode, body)
	}
}
