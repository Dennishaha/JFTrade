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

var strategyDefinitionsWriteRehearsalOperations = []string{
	"DELETE /api/v1/strategy-definitions/{definitionId}",
	"POST /api/v1/strategy-definitions",
	"POST /api/v1/strategy-definitions/{definitionId}/apply-linked-instances",
	"POST /api/v1/strategy-definitions/{definitionId}/instantiate",
	"PUT /api/v1/strategy-definitions/{definitionId}",
}

type strategyDefinitionsWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t strategyDefinitionsWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t strategyDefinitionsWriteRehearsalTarget) BearerToken() string { return t.token }

func (strategyDefinitionsWriteRehearsalTarget) Profile() string {
	return "strategy-definitions-write-test-cutover.v1"
}

func (strategyDefinitionsWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), strategyDefinitionsWriteRehearsalOperations...)
}

type strategyDefinitionsWriteRehearsalRequestSpec struct {
	method string
	path   string
	body   string
}

func TestStrategyDefinitionsWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
	if err := store.SaveBacktestMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveBacktestMarketDataProvider: %v", err)
	}
	settingsBefore, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings before strategy-definitions-write rehearsal: %v", err)
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

	token := strings.Repeat("d", 64)
	const browserCookie = "jftrade_web_session=browser-strategy-definitions-write"
	const browserCSRF = "strategy-definitions-write-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertStrategyDefinitionsWritePrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := strategyDefinitionsWriteRehearsalOperation(request)
		if !containsStrategyDefinitionsWriteOperation(operation) {
			t.Errorf("unexpected Rust strategy-definitions-write operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust strategy-definitions-write body: %v", err)
		} else if want := strategyDefinitionsWriteExpectedBody(operation); string(body) != want {
			t.Errorf(
				"Rust strategy-definitions-write body for %s = %q, want %q",
				operation, body, want,
			)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"STRATEGY_FAILED","message":"fixture definition failure"}}`))
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
		RehearsalTarget:       strategyDefinitionsWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   strategyDefinitionsWriteRehearsalOperations,
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

	specs := []strategyDefinitionsWriteRehearsalRequestSpec{
		{http.MethodDelete, "/api/v1/strategy-definitions/definition-1", ""},
		{http.MethodPost, "/api/v1/strategy-definitions", `{"name":"Draft"}`},
		{http.MethodPost, "/api/v1/strategy-definitions/definition-1/apply-linked-instances", "not-json"},
		{http.MethodPost, "/api/v1/strategy-definitions/definition-1/instantiate", `{"symbols":["AAPL"]}`},
		{http.MethodPut, "/api/v1/strategy-definitions/definition-1", `{"id":"body-id","name":"Updated"}`},
	}
	for index, spec := range specs {
		response := strategyDefinitionsWriteRehearsalRequest(
			t, proxyServer.URL+spec.path, spec.method, spec.body,
			fmt.Sprintf("strategy-definitions-write-success-%d", index+1), expectedOrigin,
			browserCookie, browserCSRF,
		)
		assertStrategyDefinitionsWriteSuccess(t, response)
	}
	duplicate := specs[4]
	response := strategyDefinitionsWriteRehearsalRequest(
		t, proxyServer.URL+duplicate.path, duplicate.method, duplicate.body,
		"strategy-definitions-write-duplicate", expectedOrigin, browserCookie, browserCSRF,
	)
	assertStrategyDefinitionsWriteSuccess(t, response)

	errorSpec := specs[2]
	response = strategyDefinitionsWriteRehearsalRequest(
		t, proxyServer.URL+errorSpec.path+"?rehearsalFailure=error", errorSpec.method,
		errorSpec.body, "strategy-definitions-write-error", expectedOrigin,
		browserCookie, browserCSRF,
	)
	assertStrategyDefinitionsWriteError(t, response, http.StatusInternalServerError, "STRATEGY_FAILED")
	timeoutSpec := specs[0]
	response = strategyDefinitionsWriteRehearsalRequest(
		t, proxyServer.URL+timeoutSpec.path+"?rehearsalFailure=timeout", timeoutSpec.method,
		timeoutSpec.body, "strategy-definitions-write-timeout", expectedOrigin,
		browserCookie, browserCSRF,
	)
	assertStrategyDefinitionsWriteError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := strategyDefinitionsWriteRehearsalDo(
			cancelContext, proxyServer.URL+specs[3].path+"?rehearsalFailure=cancel",
			specs[3].method, specs[3].body, "strategy-definitions-write-cancel",
			expectedOrigin, browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust strategy-definitions-write rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("strategy-definitions-write cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("strategy-definitions-write cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust strategy-definitions-write cancellation was not observed")
	}

	if boundaryCalls.Load() != 9 {
		t.Fatalf(
			"authenticated strategy-definitions-write boundary calls = %d, want 9",
			boundaryCalls.Load(),
		)
	}
	rust.Close()
	crashResponse := strategyDefinitionsWriteRehearsalRequest(
		t, proxyServer.URL+specs[1].path, specs[1].method, specs[1].body,
		"strategy-definitions-write-crash", expectedOrigin, browserCookie, browserCSRF,
	)
	assertStrategyDefinitionsWriteError(
		t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE",
	)

	goResponse := strategyDefinitionsWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/strategy-definitions", http.MethodPost, "{",
		"strategy-definitions-write-go-rollback", goServer.URL, browserCookie, browserCSRF,
	)
	assertStrategyDefinitionsWriteGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after strategy-definitions-write rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(
		restartedStore, servercore.SidecarOptions{DesktopMode: true},
	)
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := strategyDefinitionsWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/strategy-definitions", http.MethodPost, "{",
		"strategy-definitions-write-go-restart", restartedServer.URL, browserCookie, browserCSRF,
	)
	assertStrategyDefinitionsWriteGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after strategy-definitions-write rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf(
			"strategy-definitions-write rehearsal modified settings: before=%q after=%q",
			settingsBefore, settingsAfter,
		)
	}
}

func assertStrategyDefinitionsWritePrivateBoundary(
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
		t.Errorf("Rust private strategy-definitions-write boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/strategy-definitions",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust strategy-definitions-write boundary %s = %q, want %q", name, got, want)
		}
	}
}

func strategyDefinitionsWriteRehearsalOperation(request *http.Request) string {
	path := request.URL.Path
	if request.Method == http.MethodPost && path == "/api/v1/strategy-definitions" {
		return "POST /api/v1/strategy-definitions"
	}
	suffix, ok := strings.CutPrefix(path, "/api/v1/strategy-definitions/")
	if !ok {
		return ""
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 {
		switch request.Method {
		case http.MethodDelete:
			return "DELETE /api/v1/strategy-definitions/{definitionId}"
		case http.MethodPut:
			return "PUT /api/v1/strategy-definitions/{definitionId}"
		}
	}
	if len(parts) != 2 || request.Method != http.MethodPost {
		return ""
	}
	switch parts[1] {
	case "apply-linked-instances":
		return "POST /api/v1/strategy-definitions/{definitionId}/apply-linked-instances"
	case "instantiate":
		return "POST /api/v1/strategy-definitions/{definitionId}/instantiate"
	default:
		return ""
	}
}

func containsStrategyDefinitionsWriteOperation(operation string) bool {
	for _, candidate := range strategyDefinitionsWriteRehearsalOperations {
		if candidate == operation {
			return true
		}
	}
	return false
}

func strategyDefinitionsWriteExpectedBody(operation string) string {
	switch operation {
	case "DELETE /api/v1/strategy-definitions/{definitionId}":
		return ""
	case "POST /api/v1/strategy-definitions":
		return `{"name":"Draft"}`
	case "POST /api/v1/strategy-definitions/{definitionId}/apply-linked-instances":
		return "not-json"
	case "POST /api/v1/strategy-definitions/{definitionId}/instantiate":
		return `{"symbols":["AAPL"]}`
	case "PUT /api/v1/strategy-definitions/{definitionId}":
		return `{"id":"body-id","name":"Updated"}`
	default:
		return "<unknown>"
	}
}

func strategyDefinitionsWriteRehearsalRequest(
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
	response, err := strategyDefinitionsWriteRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call strategy-definitions-write rehearsal route: %v", err)
	}
	return response
}

func strategyDefinitionsWriteRehearsalDo(
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
	request.Header.Set("Referer", origin+"/strategy-definitions")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertStrategyDefinitionsWriteSuccess(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read strategy-definitions-write success: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("strategy-definitions-write success status = %d; body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode strategy-definitions-write success: %v; body=%s", err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["source"] != "rust-rehearsal" {
		t.Fatalf("strategy-definitions-write success = %#v", envelope)
	}
}

func assertStrategyDefinitionsWriteError(
	t *testing.T,
	response *http.Response,
	wantStatus int,
	wantCode string,
) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read strategy-definitions-write error: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf(
			"strategy-definitions-write error = %d %s, want %d/%s",
			response.StatusCode, body, wantStatus, wantCode,
		)
	}
}

func assertStrategyDefinitionsWriteGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go strategy-definitions-write fallback: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest ||
		strings.Contains(string(body), "rust-rehearsal") ||
		strings.Contains(string(body), "RUST_") {
		t.Fatalf(
			"Go strategy-definitions-write fallback unexpectedly used Rust owner: status=%d body=%s",
			response.StatusCode, body,
		)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go strategy-definitions-write fallback: %v; body=%s", err, body)
	}
	errorEnvelope, ok := envelope["error"].(map[string]any)
	if !ok || errorEnvelope["code"] != "BAD_REQUEST" {
		t.Fatalf("Go strategy-definitions-write fallback envelope = %#v", envelope)
	}
}
