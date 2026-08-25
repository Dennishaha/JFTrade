package servercoretest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

var strategiesWriteRehearsalOperations = []string{
	"DELETE /api/v1/strategies/{instanceId}",
	"POST /api/v1/strategies/{instanceId}/pause",
	"POST /api/v1/strategies/{instanceId}/refresh-definition",
	"POST /api/v1/strategies/{instanceId}/start",
	"POST /api/v1/strategies/{instanceId}/stop",
	"PUT /api/v1/strategies/{instanceId}",
	"PUT /api/v1/strategies/{instanceId}/runtime-risk",
}

type strategiesWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t strategiesWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t strategiesWriteRehearsalTarget) BearerToken() string { return t.token }

func (strategiesWriteRehearsalTarget) Profile() string {
	return "strategies-write-test-cutover.v1"
}

func (strategiesWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), strategiesWriteRehearsalOperations...)
}

type strategiesWriteRehearsalRequestSpec struct {
	method string
	path   string
	body   string
}

func TestStrategiesWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before strategies-write rehearsal: %v", err)
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
	const browserCookie = "jftrade_web_session=browser-strategies-write"
	const browserCSRF = "strategies-write-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertStrategiesWritePrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := strategiesWriteRehearsalOperation(request)
		if !containsStrategiesWriteOperation(operation) {
			t.Errorf("unexpected Rust strategies-write operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust strategies-write body: %v", err)
		} else if want := strategiesWriteExpectedBody(operation); string(body) != want {
			t.Errorf("Rust strategies-write body for %s = %q, want %q", operation, body, want)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"STRATEGY_RUNTIME_START_FAILED","message":"fixture runtime failure"}}`))
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
			_, _ = fmt.Fprintf(
				w,
				`{"ok":true,"data":{"accepted":true,"operation":%q,"source":"rust-rehearsal"},"timestamp":"fixture-time"}`,
				operation,
			)
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       strategiesWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   strategiesWriteRehearsalOperations,
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

	specs := []strategiesWriteRehearsalRequestSpec{
		{http.MethodDelete, "/api/v1/strategies/instance-1", ""},
		{http.MethodPost, "/api/v1/strategies/instance-1/pause", "ignored-pause-body"},
		{http.MethodPost, "/api/v1/strategies/instance-1/refresh-definition", "not-json"},
		{http.MethodPost, "/api/v1/strategies/instance-1/start", `{"ignored":true}`},
		{http.MethodPost, "/api/v1/strategies/instance-1/stop", ""},
		{http.MethodPut, "/api/v1/strategies/instance-1", `{"symbols":["AAPL"],"interval":"1m"}`},
		{http.MethodPut, "/api/v1/strategies/instance-1/runtime-risk", `{"mode":"paper","closeOnly":true}`},
	}
	for index, spec := range specs {
		response := strategiesWriteRehearsalRequest(
			t, proxyServer.URL+spec.path, spec.method, spec.body,
			fmt.Sprintf("strategies-write-success-%d", index+1), expectedOrigin,
			browserCookie, browserCSRF,
		)
		assertStrategiesWriteSuccess(t, response)
	}
	duplicate := specs[1]
	response := strategiesWriteRehearsalRequest(
		t, proxyServer.URL+duplicate.path, duplicate.method, duplicate.body,
		"strategies-write-duplicate", expectedOrigin, browserCookie, browserCSRF,
	)
	assertStrategiesWriteSuccess(t, response)

	errorSpec := specs[3]
	response = strategiesWriteRehearsalRequest(
		t, proxyServer.URL+errorSpec.path+"?rehearsalFailure=error", errorSpec.method,
		errorSpec.body, "strategies-write-error", expectedOrigin, browserCookie, browserCSRF,
	)
	assertStrategiesWriteError(
		t, response, http.StatusBadGateway, "STRATEGY_RUNTIME_START_FAILED",
	)
	timeoutSpec := specs[4]
	response = strategiesWriteRehearsalRequest(
		t, proxyServer.URL+timeoutSpec.path+"?rehearsalFailure=timeout", timeoutSpec.method,
		timeoutSpec.body, "strategies-write-timeout", expectedOrigin, browserCookie, browserCSRF,
	)
	assertStrategiesWriteError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := strategiesWriteRehearsalDo(
			cancelContext, proxyServer.URL+specs[6].path+"?rehearsalFailure=cancel",
			specs[6].method, specs[6].body, "strategies-write-cancel", expectedOrigin,
			browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust strategies-write rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("strategies-write cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("strategies-write cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust strategies-write cancellation was not observed")
	}

	if boundaryCalls.Load() != 11 {
		t.Fatalf("authenticated strategies-write boundary calls = %d, want 11", boundaryCalls.Load())
	}
	rust.Close()
	crashResponse := strategiesWriteRehearsalRequest(
		t, proxyServer.URL+specs[2].path, specs[2].method, specs[2].body,
		"strategies-write-crash", expectedOrigin, browserCookie, browserCSRF,
	)
	assertStrategiesWriteError(
		t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE",
	)

	goResponse := strategiesWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/strategies/missing", http.MethodPut, "{",
		"strategies-write-go-rollback", goServer.URL, browserCookie, browserCSRF,
	)
	assertStrategiesWriteGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after strategies-write rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(
		restartedStore, servercore.SidecarOptions{DesktopMode: true},
	)
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := strategiesWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/strategies/missing", http.MethodPut, "{",
		"strategies-write-go-restart", restartedServer.URL, browserCookie, browserCSRF,
	)
	assertStrategiesWriteGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after strategies-write rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("strategies-write rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertStrategiesWritePrivateBoundary(
	t *testing.T,
	request *http.Request,
	token string,
	origin string,
	cookie string,
	csrf string,
) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust private strategies-write boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/strategies",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust strategies-write boundary %s = %q, want %q", name, got, want)
		}
	}
}

func strategiesWriteRehearsalOperation(request *http.Request) string {
	path := request.URL.Path
	suffix, ok := strings.CutPrefix(path, "/api/v1/strategies/")
	if !ok {
		return ""
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 {
		switch request.Method {
		case http.MethodDelete:
			return "DELETE /api/v1/strategies/{instanceId}"
		case http.MethodPut:
			return "PUT /api/v1/strategies/{instanceId}"
		}
	}
	if len(parts) != 2 {
		return ""
	}
	switch {
	case request.Method == http.MethodPut && parts[1] == "runtime-risk":
		return "PUT /api/v1/strategies/{instanceId}/runtime-risk"
	case request.Method == http.MethodPost && parts[1] == "pause":
		return "POST /api/v1/strategies/{instanceId}/pause"
	case request.Method == http.MethodPost && parts[1] == "refresh-definition":
		return "POST /api/v1/strategies/{instanceId}/refresh-definition"
	case request.Method == http.MethodPost && parts[1] == "start":
		return "POST /api/v1/strategies/{instanceId}/start"
	case request.Method == http.MethodPost && parts[1] == "stop":
		return "POST /api/v1/strategies/{instanceId}/stop"
	default:
		return ""
	}
}

func containsStrategiesWriteOperation(operation string) bool {
	for _, candidate := range strategiesWriteRehearsalOperations {
		if candidate == operation {
			return true
		}
	}
	return false
}

func strategiesWriteExpectedBody(operation string) string {
	switch operation {
	case "DELETE /api/v1/strategies/{instanceId}", "POST /api/v1/strategies/{instanceId}/stop":
		return ""
	case "POST /api/v1/strategies/{instanceId}/pause":
		return "ignored-pause-body"
	case "POST /api/v1/strategies/{instanceId}/refresh-definition":
		return "not-json"
	case "POST /api/v1/strategies/{instanceId}/start":
		return `{"ignored":true}`
	case "PUT /api/v1/strategies/{instanceId}":
		return `{"symbols":["AAPL"],"interval":"1m"}`
	case "PUT /api/v1/strategies/{instanceId}/runtime-risk":
		return `{"mode":"paper","closeOnly":true}`
	default:
		return "<unknown>"
	}
}

func strategiesWriteRehearsalRequest(
	t *testing.T,
	target string,
	method string,
	body string,
	requestID string,
	origin string,
	cookie string,
	csrf string,
) *http.Response {
	t.Helper()
	response, err := strategiesWriteRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call strategies-write rehearsal route: %v", err)
	}
	return response
}

func strategiesWriteRehearsalDo(
	ctx context.Context,
	target string,
	method string,
	body string,
	requestID string,
	origin string,
	cookie string,
	csrf string,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/strategies")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertStrategiesWriteSuccess(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read strategies-write success: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("strategies-write success status = %d; body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode strategies-write success: %v; body=%s", err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["source"] != "rust-rehearsal" {
		t.Fatalf("strategies-write success = %#v", envelope)
	}
}

func assertStrategiesWriteError(
	t *testing.T,
	response *http.Response,
	wantStatus int,
	wantCode string,
) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read strategies-write error: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf(
			"strategies-write error = %d %s, want %d/%s",
			response.StatusCode, body, wantStatus, wantCode,
		)
	}
}

func assertStrategiesWriteGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go strategies-write fallback: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest ||
		strings.Contains(string(body), "rust-rehearsal") ||
		strings.Contains(string(body), "RUST_") {
		t.Fatalf(
			"Go strategies-write fallback unexpectedly used Rust owner: status=%d body=%s",
			response.StatusCode, body,
		)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go strategies-write fallback: %v; body=%s", err, body)
	}
	errorEnvelope, ok := envelope["error"].(map[string]any)
	if !ok || errorEnvelope["code"] != "BAD_REQUEST" {
		t.Fatalf("Go strategies-write fallback envelope = %#v", envelope)
	}
}
