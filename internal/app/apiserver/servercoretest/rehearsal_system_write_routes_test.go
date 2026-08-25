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

var systemWriteRehearsalOperations = []string{
	"DELETE /api/v1/system/real-trade-risk-limits",
	"POST /api/v1/system/futu-opend/manual-retry",
	"POST /api/v1/system/real-trade-hard-stops",
	"POST /api/v1/system/real-trade-hard-stops/{hardStopId}/release",
	"POST /api/v1/system/real-trade-kill-switch/activate",
	"POST /api/v1/system/real-trade-kill-switch/release",
	"PUT /api/v1/system/real-trade-risk-limits",
}

type systemWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t systemWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t systemWriteRehearsalTarget) BearerToken() string { return t.token }

func (systemWriteRehearsalTarget) Profile() string { return "system-write-test-cutover.v1" }

func (systemWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), systemWriteRehearsalOperations...)
}

type systemWriteRehearsalRequestSpec struct {
	method string
	path   string
	body   string
}

func TestSystemWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before system-write rehearsal: %v", err)
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

	token := strings.Repeat("y", 64)
	const browserCookie = "jftrade_web_session=browser-system-write"
	const browserCSRF = "system-write-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertSystemWritePrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := systemWriteRehearsalOperation(request)
		if !containsSystemWriteOperation(operation) {
			t.Errorf("unexpected Rust system-write operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust system-write body: %v", err)
		} else if want := systemWriteExpectedBody(operation); string(body) != want {
			t.Errorf("Rust system-write body for %s = %q, want %q", operation, body, want)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"REAL_TRADE_CONTROL_FAILED","message":"fixture control failure"}}`))
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
		RehearsalTarget:       systemWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   systemWriteRehearsalOperations,
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

	specs := []systemWriteRehearsalRequestSpec{
		{http.MethodDelete, "/api/v1/system/real-trade-risk-limits", ""},
		{http.MethodPost, "/api/v1/system/futu-opend/manual-retry", "not-json"},
		{http.MethodPost, "/api/v1/system/real-trade-hard-stops", `{"accountId":"ACC-1"}`},
		{http.MethodPost, "/api/v1/system/real-trade-hard-stops/hs-1/release", `{}`},
		{http.MethodPost, "/api/v1/system/real-trade-kill-switch/activate", `{"operatorId":"fixture"}`},
		{http.MethodPost, "/api/v1/system/real-trade-kill-switch/release", ""},
		{http.MethodPut, "/api/v1/system/real-trade-risk-limits", `{"realTradingEnabled":true,"maxOrderQuantity":1}`},
	}
	for index, spec := range specs {
		response := systemWriteRehearsalRequest(
			t, proxyServer.URL+spec.path, spec.method, spec.body,
			"system-write-success-"+string(rune('1'+index)), expectedOrigin,
			browserCookie, browserCSRF,
		)
		assertSystemWriteSuccess(t, response)
	}
	duplicate := specs[4]
	response := systemWriteRehearsalRequest(
		t, proxyServer.URL+duplicate.path, duplicate.method, duplicate.body,
		"system-write-duplicate", expectedOrigin, browserCookie, browserCSRF,
	)
	assertSystemWriteSuccess(t, response)

	errorSpec := specs[2]
	response = systemWriteRehearsalRequest(
		t, proxyServer.URL+errorSpec.path+"?rehearsalFailure=error", errorSpec.method,
		errorSpec.body, "system-write-error", expectedOrigin, browserCookie, browserCSRF,
	)
	assertSystemWriteError(t, response, http.StatusConflict, "REAL_TRADE_CONTROL_FAILED")
	timeoutSpec := specs[3]
	response = systemWriteRehearsalRequest(
		t, proxyServer.URL+timeoutSpec.path+"?rehearsalFailure=timeout", timeoutSpec.method,
		timeoutSpec.body, "system-write-timeout", expectedOrigin, browserCookie, browserCSRF,
	)
	assertSystemWriteError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := systemWriteRehearsalDo(
			cancelContext, proxyServer.URL+specs[6].path+"?rehearsalFailure=cancel",
			specs[6].method, specs[6].body, "system-write-cancel", expectedOrigin,
			browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust system-write rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("system-write cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("system-write cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust system-write cancellation was not observed")
	}

	if boundaryCalls.Load() != 11 {
		t.Fatalf("authenticated system-write boundary calls = %d, want 11", boundaryCalls.Load())
	}
	rust.Close()
	crashResponse := systemWriteRehearsalRequest(
		t, proxyServer.URL+specs[1].path, specs[1].method, specs[1].body,
		"system-write-crash", expectedOrigin, browserCookie, browserCSRF,
	)
	assertSystemWriteError(t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")

	goFallbackBody := `{"realTradingEnabled":true}`
	goResponse := systemWriteRehearsalRequest(
		t, goServer.URL+"/api/v1/system/real-trade-risk-limits", http.MethodPut,
		goFallbackBody, "system-write-go-rollback", goServer.URL, browserCookie, browserCSRF,
	)
	assertSystemWriteGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after system-write rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := systemWriteRehearsalRequest(
		t, restartedServer.URL+"/api/v1/system/real-trade-risk-limits", http.MethodPut,
		goFallbackBody, "system-write-go-restart", restartedServer.URL, browserCookie, browserCSRF,
	)
	assertSystemWriteGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after system-write rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("system-write rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertSystemWritePrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust private system-write boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/system",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust system-write boundary %s = %q, want %q", name, got, want)
		}
	}
}

func systemWriteRehearsalOperation(request *http.Request) string {
	path := request.URL.Path
	switch {
	case request.Method == http.MethodDelete && path == "/api/v1/system/real-trade-risk-limits":
		return "DELETE /api/v1/system/real-trade-risk-limits"
	case request.Method == http.MethodPost && path == "/api/v1/system/futu-opend/manual-retry":
		return "POST /api/v1/system/futu-opend/manual-retry"
	case request.Method == http.MethodPost && path == "/api/v1/system/real-trade-hard-stops":
		return "POST /api/v1/system/real-trade-hard-stops"
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/api/v1/system/real-trade-hard-stops/") && strings.HasSuffix(path, "/release"):
		return "POST /api/v1/system/real-trade-hard-stops/{hardStopId}/release"
	case request.Method == http.MethodPost && path == "/api/v1/system/real-trade-kill-switch/activate":
		return "POST /api/v1/system/real-trade-kill-switch/activate"
	case request.Method == http.MethodPost && path == "/api/v1/system/real-trade-kill-switch/release":
		return "POST /api/v1/system/real-trade-kill-switch/release"
	case request.Method == http.MethodPut && path == "/api/v1/system/real-trade-risk-limits":
		return "PUT /api/v1/system/real-trade-risk-limits"
	default:
		return ""
	}
}

func containsSystemWriteOperation(operation string) bool {
	for _, candidate := range systemWriteRehearsalOperations {
		if candidate == operation {
			return true
		}
	}
	return false
}

func systemWriteExpectedBody(operation string) string {
	switch operation {
	case "DELETE /api/v1/system/real-trade-risk-limits", "POST /api/v1/system/real-trade-kill-switch/release":
		return ""
	case "POST /api/v1/system/futu-opend/manual-retry":
		return "not-json"
	case "POST /api/v1/system/real-trade-hard-stops":
		return `{"accountId":"ACC-1"}`
	case "POST /api/v1/system/real-trade-hard-stops/{hardStopId}/release":
		return `{}`
	case "POST /api/v1/system/real-trade-kill-switch/activate":
		return `{"operatorId":"fixture"}`
	case "PUT /api/v1/system/real-trade-risk-limits":
		return `{"realTradingEnabled":true,"maxOrderQuantity":1}`
	default:
		return "<unknown>"
	}
}

func systemWriteRehearsalRequest(t *testing.T, target, method, body, requestID, origin, cookie, csrf string) *http.Response {
	t.Helper()
	response, err := systemWriteRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call system-write rehearsal route: %v", err)
	}
	return response
}

func systemWriteRehearsalDo(ctx context.Context, target, method, body, requestID, origin, cookie, csrf string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/system")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertSystemWriteSuccess(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read system-write success: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("system-write success status = %d; body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode system-write success: %v; body=%s", err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["source"] != "rust-rehearsal" {
		t.Fatalf("system-write success = %#v", envelope)
	}
}

func assertSystemWriteError(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read system-write error: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf("system-write error = %d %s, want %d/%s", response.StatusCode, body, wantStatus, wantCode)
	}
}

func assertSystemWriteGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go system-write fallback: %v", err)
	}
	if response.StatusCode == http.StatusOK || strings.Contains(string(body), "rust-rehearsal") || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go system-write fallback unexpectedly used Rust owner: status=%d body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go system-write fallback: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok {
		t.Fatalf("Go system-write fallback envelope = %#v", envelope)
	}
}
