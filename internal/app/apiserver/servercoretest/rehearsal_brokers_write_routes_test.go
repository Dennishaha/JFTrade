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

var brokersWriteRehearsalOperations = []string{
	"DELETE /api/v1/brokers/{brokerId}/orders",
	"POST /api/v1/brokers/{brokerId}/orders",
	"POST /api/v1/brokers/{brokerId}/unlock",
}

type brokersWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t brokersWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t brokersWriteRehearsalTarget) BearerToken() string { return t.token }

func (brokersWriteRehearsalTarget) Profile() string {
	return "brokers-write-test-cutover.v1"
}

func (brokersWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), brokersWriteRehearsalOperations...)
}

func TestBrokersWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before brokers rehearsal: %v", err)
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
	const browserCookie = "jftrade_web_session=browser-brokers-write"
	const browserCSRF = "brokers-write-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertBrokersWritePrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := request.Method + " " + request.URL.Path
		if !containsBrokersWriteOperation(operation) {
			t.Errorf("unexpected Rust brokers-write operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust brokers-write body: %v", err)
		}
		if err := assertBrokersWriteBody(request.Method, request.URL.Path, body); err != nil {
			t.Error(err)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"BROKER_WRITE_FAILED","message":"fixture broker failed"}}`))
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
		RehearsalTarget:       brokersWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   brokersWriteRehearsalOperations,
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

	placePath := "/api/v1/brokers/futu/orders?tradingEnvironment=REAL&accountId=acct-1&market=US"
	placeBody := `{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}`
	cancelPath := "/api/v1/brokers/futu/orders?tradingEnvironment=REAL&accountId=acct-1&market=US"
	cancelBody := `{"orders":[{"orderId":7,"brokerOrderId":"broker-7","symbol":"US.AAPL"}]}`
	unlockPath := "/api/v1/brokers/futu/unlock?tradingEnvironment=REAL&accountId=acct-1&market=US"
	unlockBody := `{"unlock":true,"passwordMd5":"fixture"}`

	for index, requestCase := range []struct {
		path   string
		method string
		body   string
	}{
		{path: placePath, method: http.MethodPost, body: placeBody},
		{path: placePath, method: http.MethodPost, body: placeBody},
		{path: cancelPath, method: http.MethodDelete, body: cancelBody},
		{path: unlockPath, method: http.MethodPost, body: unlockBody},
	} {
		requestID := "brokers-write-success-" + string(rune('1'+index))
		response := brokersWriteRehearsalRequest(
			t, proxyServer.URL+requestCase.path, requestCase.method, requestCase.body,
			requestID, expectedOrigin, browserCookie, browserCSRF,
		)
		assertBrokersWriteSuccess(t, response, requestID)
	}

	response := brokersWriteRehearsalRequest(
		t, proxyServer.URL+placePath+"&rehearsalFailure=error", http.MethodPost,
		placeBody, "brokers-write-error", expectedOrigin, browserCookie, browserCSRF,
	)
	assertBrokersWriteError(t, response, http.StatusBadGateway, "BROKER_WRITE_FAILED")
	response = brokersWriteRehearsalRequest(
		t, proxyServer.URL+cancelPath+"&rehearsalFailure=timeout", http.MethodDelete,
		cancelBody, "brokers-write-timeout", expectedOrigin, browserCookie, browserCSRF,
	)
	assertBrokersWriteError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := brokersWriteRehearsalDo(
			cancelContext, proxyServer.URL+unlockPath+"&rehearsalFailure=cancel", http.MethodPost,
			unlockBody, "brokers-write-cancel", expectedOrigin, browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust brokers-write rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("brokers-write cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("brokers-write cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust brokers-write cancellation was not observed")
	}

	if boundaryCalls.Load() != 7 {
		t.Fatalf("authenticated brokers-write boundary calls = %d, want 7", boundaryCalls.Load())
	}
	rust.Close()
	crashResponse := brokersWriteRehearsalRequest(
		t, proxyServer.URL+placePath, http.MethodPost, placeBody,
		"brokers-write-crash", expectedOrigin, browserCookie, browserCSRF,
	)
	assertBrokersWriteError(t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")

	goResponse := brokersWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/brokers/missing/orders", http.MethodPost, placeBody,
		"brokers-write-go-rollback", goServer.URL, browserCookie, browserCSRF,
	)
	assertBrokersWriteGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after brokers rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := brokersWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/brokers/missing/unlock", http.MethodPost, unlockBody,
		"brokers-write-go-restart", restartedServer.URL, browserCookie, browserCSRF,
	)
	assertBrokersWriteGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after brokers rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("brokers-write rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertBrokersWritePrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust brokers-write private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/brokers",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust brokers-write boundary %s = %q, want %q", name, got, want)
		}
	}
}

func assertBrokersWriteBody(method, path string, body []byte) error {
	want := map[string]string{
		http.MethodPost + " /api/v1/brokers/futu/orders":   `{"symbol":"US.AAPL","side":"BUY","orderType":"LIMIT","quantity":1}`,
		http.MethodDelete + " /api/v1/brokers/futu/orders": `{"orders":[{"orderId":7,"brokerOrderId":"broker-7","symbol":"US.AAPL"}]}`,
		http.MethodPost + " /api/v1/brokers/futu/unlock":   `{"unlock":true,"passwordMd5":"fixture"}`,
	}[method+" "+path]
	if string(body) != want {
		return errors.New("brokers-write rehearsal body was not forwarded unchanged")
	}
	return nil
}

func containsBrokersWriteOperation(operation string) bool {
	switch operation {
	case "DELETE /api/v1/brokers/futu/orders",
		"POST /api/v1/brokers/futu/orders",
		"POST /api/v1/brokers/futu/unlock":
		return true
	default:
		return false
	}
}

func brokersWriteRehearsalRequest(
	t *testing.T, target, method, body, requestID, origin, cookie, csrf string,
) *http.Response {
	t.Helper()
	response, err := brokersWriteRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call brokers-write rehearsal route: %v", err)
	}
	return response
}

func brokersWriteRehearsalDo(
	ctx context.Context, target, method, body, requestID, origin, cookie, csrf string,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	if csrf != "" {
		request.Header.Set("Authorization", "Bearer public-brokers-write-token")
	}
	request.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
		request.Header.Set("Referer", origin+"/brokers")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertBrokersWriteSuccess(t *testing.T, response *http.Response, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read brokers-write success: %v", err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("brokers-write success %s = %d %#v %s", requestID, response.StatusCode, response.Header, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode brokers-write success %s: %v; body=%s", requestID, err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["source"] != "rust-rehearsal" {
		t.Fatalf("brokers-write success %s = %#v", requestID, envelope)
	}
}

func assertBrokersWriteError(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read brokers-write error: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("brokers-write error status = %d, want %d; body=%s", response.StatusCode, wantStatus, body)
	}
	if wantCode == "" {
		return
	}
	if !strings.Contains(string(body), wantCode) {
		t.Fatalf("brokers-write error body = %s, want code %q", body, wantCode)
	}
}

func assertBrokersWriteGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go brokers fallback: %v", err)
	}
	if response.StatusCode == http.StatusOK || strings.Contains(string(body), "rust-rehearsal") || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go brokers fallback unexpectedly used Rust owner: status=%d body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go brokers fallback: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok {
		t.Fatalf("Go brokers fallback error envelope = %#v", envelope)
	}
}
