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

var marketDataProviderActionsRehearsalOperations = []string{
	"POST /api/v1/market-data/instruments/normalize",
	"POST /api/v1/market-data/options/analysis/{instrumentId}",
	"POST /api/v1/market-data/options/events/zero-dte-contracts",
	"POST /api/v1/market-data/prediction/combos/quotes",
	"POST /api/v1/market-data/snapshots",
}

type marketDataProviderActionsRehearsalTarget struct {
	endpoint string
	token    string
}

func (t marketDataProviderActionsRehearsalTarget) Endpoint() string { return t.endpoint }

func (t marketDataProviderActionsRehearsalTarget) BearerToken() string { return t.token }

func (marketDataProviderActionsRehearsalTarget) Profile() string {
	return "market-data-provider-actions-test-cutover.v1"
}

func (marketDataProviderActionsRehearsalTarget) Capabilities() []string {
	return append([]string(nil), marketDataProviderActionsRehearsalOperations...)
}

func TestMarketDataProviderActionsRehearsalPreservesBrowserBoundaryAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before provider-actions rehearsal: %v", err)
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

	token := strings.Repeat("p", 64)
	const browserCookie = "jftrade_web_session=browser-provider-actions"
	const browserCSRF = "market-data-provider-actions-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertMarketDataProviderActionsPrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := request.Method + " " + request.URL.Path
		if !containsMarketDataProviderActionsOperation(operation) {
			t.Errorf("unexpected Rust provider-actions operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust provider-actions body: %v", err)
		}
		if err := assertMarketDataProviderActionsBody(request.URL.Path, body); err != nil {
			t.Error(err)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Retry-After", "7")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"PROVIDER_RATE_LIMITED","message":"retry later"}}`))
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
		RehearsalTarget:       marketDataProviderActionsRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   marketDataProviderActionsRehearsalOperations,
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
		path string
		body string
	}{
		{"/api/v1/market-data/instruments/normalize", `{"symbol":"AAPL"}`},
		{"/api/v1/market-data/instruments/normalize", `{"symbol":"AAPL"}`},
		{"/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test&accountId=eligible&rehearsalFailure=error", `{"operation":"chain"}`},
		{"/api/v1/market-data/options/events/zero-dte-contracts", `{"market":"US","underlying":"AAPL","expiry":"2026-08-25"}`},
		{"/api/v1/market-data/prediction/combos/quotes", `{"legs":[{"side":"BUY","quantity":1}]}`},
		{"/api/v1/market-data/snapshots", `{"symbols":["AAPL"]}`},
	}
	for index, requestCase := range requests {
		response := marketDataProviderActionsRehearsalRequest(
			t, proxyServer.URL+requestCase.path, "POST", requestCase.body,
			"provider-actions-success-"+string(rune('1'+index)), proxyServer.URL, browserCookie, browserCSRF,
		)
		if index == 2 {
			assertMarketDataProviderActionsError(t, response, http.StatusTooManyRequests, "PROVIDER_RATE_LIMITED")
			if response.Header.Get("Retry-After") != "7" {
				t.Fatalf("provider-actions rate-limit Retry-After = %q, want 7", response.Header.Get("Retry-After"))
			}
			continue
		}
		assertMarketDataProviderActionsSuccess(t, response, requestCase.path)
	}

	response := marketDataProviderActionsRehearsalRequest(
		t, proxyServer.URL+"/api/v1/market-data/snapshots?rehearsalFailure=timeout", "POST",
		`{"symbols":["AAPL"]}`, "provider-actions-timeout", proxyServer.URL, browserCookie, browserCSRF,
	)
	assertMarketDataProviderActionsError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := marketDataProviderActionsRehearsalDo(
			cancelContext,
			proxyServer.URL+"/api/v1/market-data/options/events/zero-dte-contracts?rehearsalFailure=cancel",
			"POST", `{"market":"US"}`, "provider-actions-cancel", proxyServer.URL, browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust provider-actions rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("provider-actions cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("provider-actions cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust provider-actions cancellation was not observed")
	}

	if boundaryCalls.Load() != int32(len(requests)+2) {
		t.Fatalf("authenticated provider-actions boundary calls = %d, want %d", boundaryCalls.Load(), len(requests)+2)
	}
	rust.Close()
	crashResponse := marketDataProviderActionsRehearsalRequest(
		t, proxyServer.URL+"/api/v1/market-data/instruments/normalize", "POST",
		`{"symbol":"AAPL"}`, "provider-actions-crash", proxyServer.URL, browserCookie, browserCSRF,
	)
	assertMarketDataProviderActionsError(t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")

	goResponse := marketDataProviderActionsRehearsalRequest(
		t, goServer.URL+"/api/v1/market-data/instruments/normalize", "POST",
		`{"symbol":"AAPL"}`, "provider-actions-go-rollback", goServer.URL, browserCookie, browserCSRF,
	)
	assertMarketDataProviderActionsGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after provider-actions rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := marketDataProviderActionsRehearsalRequest(
		t, restartedServer.URL+"/api/v1/market-data/instruments/normalize", "POST",
		`{"symbol":"AAPL"}`, "provider-actions-go-restart", restartedServer.URL, browserCookie, browserCSRF,
	)
	assertMarketDataProviderActionsGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after provider-actions rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("provider-actions rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertMarketDataProviderActionsPrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust provider-actions private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/market-data",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust provider-actions boundary %s = %q, want %q", name, got, want)
		}
	}
}

func assertMarketDataProviderActionsBody(path string, body []byte) error {
	if !json.Valid(body) {
		return errors.New("provider-actions rehearsal body is not valid JSON")
	}
	if strings.Contains(path, "/options/analysis/") && string(body) != `{"operation":"chain"}` {
		return errors.New("provider-actions option-analysis body did not preserve the fixture")
	}
	return nil
}

func containsMarketDataProviderActionsOperation(operation string) bool {
	for _, candidate := range marketDataProviderActionsRehearsalOperations {
		method, path, ok := strings.Cut(candidate, " ")
		if !ok || method != http.MethodPost {
			continue
		}
		if path == operation[len(method)+1:] {
			return true
		}
		if path == "/api/v1/market-data/options/analysis/{instrumentId}" &&
			strings.HasPrefix(operation, method+" /api/v1/market-data/options/analysis/") {
			return true
		}
	}
	return false
}

func marketDataProviderActionsRehearsalRequest(t *testing.T, target, method, body, requestID, origin, cookie, csrf string) *http.Response {
	t.Helper()
	response, err := marketDataProviderActionsRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call provider-actions rehearsal route: %v", err)
	}
	return response
}

func marketDataProviderActionsRehearsalDo(ctx context.Context, target, method, body, requestID, origin, cookie, csrf string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-market-data-token")
	request.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
		request.Header.Set("Referer", origin+"/market-data")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertMarketDataProviderActionsSuccess(t *testing.T, response *http.Response, path string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read provider-actions success: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("provider-actions success for %s = %d %s", path, response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode provider-actions success for %s: %v; body=%s", path, err, body)
	}
	data, ok := envelope["data"].(map[string]any)
	if envelope["ok"] != true || !ok || data["source"] != "rust-rehearsal" {
		t.Fatalf("provider-actions success for %s = %#v", path, envelope)
	}
}

func assertMarketDataProviderActionsError(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read provider-actions error: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("provider-actions error status = %d, want %d; body=%s", response.StatusCode, wantStatus, body)
	}
	if wantCode == "" {
		return
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode provider-actions error: %v; body=%s", err, body)
	}
	errorData, ok := envelope["error"].(map[string]any)
	if !ok || errorData["code"] != wantCode {
		t.Fatalf("provider-actions error envelope = %#v", envelope)
	}
}

func assertMarketDataProviderActionsGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go provider-actions fallback: %v", err)
	}
	if response.StatusCode == http.StatusOK || strings.Contains(string(body), "rust-rehearsal") {
		t.Fatalf("Go provider-actions fallback unexpectedly used Rust owner: status=%d body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go provider-actions fallback: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok {
		t.Fatalf("Go provider-actions fallback error envelope = %#v", envelope)
	}
}
