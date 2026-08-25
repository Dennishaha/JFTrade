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

var executionWriteRehearsalOperations = []string{
	"POST /api/v1/execution/buying-power",
	"POST /api/v1/execution/combos/previews",
	"POST /api/v1/execution/combos",
	"POST /api/v1/execution/combos/{internalOrderId}/cancel",
	"POST /api/v1/execution/orders",
	"POST /api/v1/execution/orders/{internalOrderId}/cancel",
	"POST /api/v1/execution/previews",
}

type executionWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t executionWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t executionWriteRehearsalTarget) BearerToken() string { return t.token }

func (executionWriteRehearsalTarget) Profile() string {
	return "execution-write-test-cutover.v1"
}

func (executionWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), executionWriteRehearsalOperations...)
}

func TestExecutionWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before execution rehearsal: %v", err)
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

	token := strings.Repeat("e", 64)
	const browserCookie = "jftrade_web_session=browser-execution-write"
	const browserCSRF = "execution-write-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertExecutionWritePrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := request.Method + " " + request.URL.Path
		if !containsExecutionWriteOperation(operation) {
			t.Errorf("unexpected Rust execution-write operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust execution-write body: %v", err)
		}
		if err := assertExecutionWriteBody(operation, body); err != nil {
			t.Error(err)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"EXECUTION_WRITE_FAILED","message":"fixture execution gateway failed"}}`))
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
		RehearsalTarget:       executionWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   executionWriteRehearsalOperations,
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
		path   string
		body   string
		method string
	}{
		{path: "/api/v1/execution/buying-power", body: `{"brokerId":"FUTU","accountId":"acct-1","market":"US","quantity":1}`, method: http.MethodPost},
		{path: "/api/v1/execution/combos/previews", body: `{"brokerId":"FUTU","accountId":"acct-1","market":"US","legs":[]}`, method: http.MethodPost},
		{path: "/api/v1/execution/combos", body: `{"brokerId":"FUTU","accountId":"acct-1","market":"US","legs":[]}`, method: http.MethodPost},
		{path: "/api/v1/execution/combos/combo-1/cancel", method: http.MethodPost},
		{path: "/api/v1/execution/orders", body: `{"brokerId":"FUTU","accountId":"acct-1","market":"US","symbol":"US.AAPL","side":"BUY","quantity":1}`, method: http.MethodPost},
		{path: "/api/v1/execution/orders/order-1/cancel", method: http.MethodPost},
		{path: "/api/v1/execution/previews", body: `{"brokerId":"FUTU","accountId":"acct-1","market":"US","symbol":"US.AAPL","side":"BUY","quantity":1}`, method: http.MethodPost},
	}
	for index, requestCase := range requests {
		response := executionWriteRehearsalRequest(
			t, proxyServer.URL+requestCase.path, requestCase.method, requestCase.body,
			"execution-write-success-"+string(rune('1'+index)), expectedOrigin,
			browserCookie, browserCSRF,
		)
		assertExecutionWriteSuccess(t, response)
	}
	response := executionWriteRehearsalRequest(
		t, proxyServer.URL+requests[4].path, http.MethodPost, requests[4].body,
		"execution-write-repeat", expectedOrigin, browserCookie, browserCSRF,
	)
	assertExecutionWriteSuccess(t, response)

	response = executionWriteRehearsalRequest(
		t, proxyServer.URL+requests[2].path+"?rehearsalFailure=error", http.MethodPost,
		requests[2].body, "execution-write-error", expectedOrigin, browserCookie, browserCSRF,
	)
	assertExecutionWriteError(t, response, http.StatusBadGateway, "EXECUTION_WRITE_FAILED")
	response = executionWriteRehearsalRequest(
		t, proxyServer.URL+requests[5].path+"?rehearsalFailure=timeout", http.MethodPost,
		"", "execution-write-timeout", expectedOrigin, browserCookie, browserCSRF,
	)
	assertExecutionWriteError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := executionWriteRehearsalDo(
			cancelContext, proxyServer.URL+requests[0].path+"?rehearsalFailure=cancel", http.MethodPost,
			requests[0].body, "execution-write-cancel", expectedOrigin, browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust execution-write rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("execution-write cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("execution-write cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust execution-write cancellation was not observed")
	}

	if boundaryCalls.Load() != 11 {
		t.Fatalf("authenticated execution-write boundary calls = %d, want 11", boundaryCalls.Load())
	}
	rust.Close()
	crashResponse := executionWriteRehearsalRequest(
		t, proxyServer.URL+requests[4].path, http.MethodPost, requests[4].body,
		"execution-write-crash", expectedOrigin, browserCookie, browserCSRF,
	)
	assertExecutionWriteError(t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")

	goResponse := executionWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/execution/orders", http.MethodPost, requests[4].body,
		"execution-write-go-rollback", goServer.URL, browserCookie, browserCSRF,
	)
	assertExecutionWriteGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after execution rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := executionWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/execution/previews", http.MethodPost,
		requests[6].body, "execution-write-go-restart", restartedServer.URL,
		browserCookie, browserCSRF,
	)
	assertExecutionWriteGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after execution rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("execution-write rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertExecutionWritePrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust execution-write private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/execution",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust execution-write boundary %s = %q, want %q", name, got, want)
		}
	}
}

func assertExecutionWriteBody(operation string, body []byte) error {
	want := map[string]string{
		"POST /api/v1/execution/buying-power":          `{"brokerId":"FUTU","accountId":"acct-1","market":"US","quantity":1}`,
		"POST /api/v1/execution/combos/previews":       `{"brokerId":"FUTU","accountId":"acct-1","market":"US","legs":[]}`,
		"POST /api/v1/execution/combos":                `{"brokerId":"FUTU","accountId":"acct-1","market":"US","legs":[]}`,
		"POST /api/v1/execution/combos/combo-1/cancel": "",
		"POST /api/v1/execution/orders":                `{"brokerId":"FUTU","accountId":"acct-1","market":"US","symbol":"US.AAPL","side":"BUY","quantity":1}`,
		"POST /api/v1/execution/orders/order-1/cancel": "",
		"POST /api/v1/execution/previews":              `{"brokerId":"FUTU","accountId":"acct-1","market":"US","symbol":"US.AAPL","side":"BUY","quantity":1}`,
	}
	if string(body) != want[operation] {
		return errors.New("execution-write rehearsal body was not forwarded unchanged")
	}
	return nil
}

func containsExecutionWriteOperation(operation string) bool {
	return operation == "POST /api/v1/execution/buying-power" ||
		operation == "POST /api/v1/execution/combos/previews" ||
		operation == "POST /api/v1/execution/combos" ||
		operation == "POST /api/v1/execution/orders" ||
		operation == "POST /api/v1/execution/previews" ||
		(strings.HasPrefix(operation, "POST /api/v1/execution/combos/") && strings.HasSuffix(operation, "/cancel")) ||
		(strings.HasPrefix(operation, "POST /api/v1/execution/orders/") && strings.HasSuffix(operation, "/cancel"))
}

func executionWriteRehearsalRequest(t *testing.T, target, method, body, requestID, origin, cookie, csrf string) *http.Response {
	t.Helper()
	response, err := executionWriteRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call execution-write rehearsal route: %v", err)
	}
	return response
}

func executionWriteRehearsalDo(ctx context.Context, target, method, body, requestID, origin, cookie, csrf string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/execution")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertExecutionWriteSuccess(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read execution-write success: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("execution-write success status = %d; body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode execution-write success: %v; body=%s", err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["source"] != "rust-rehearsal" {
		t.Fatalf("execution-write success = %#v", envelope)
	}
}

func assertExecutionWriteError(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read execution-write error: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf("execution-write error = %d %s, want %d/%s", response.StatusCode, body, wantStatus, wantCode)
	}
}

func assertExecutionWriteGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go execution fallback: %v", err)
	}
	if response.StatusCode == http.StatusOK || strings.Contains(string(body), "rust-rehearsal") || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go execution fallback unexpectedly used Rust owner: status=%d body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go execution fallback: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok {
		t.Fatalf("Go execution fallback envelope = %#v", envelope)
	}
}
