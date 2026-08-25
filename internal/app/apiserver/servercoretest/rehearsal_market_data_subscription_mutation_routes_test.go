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

var marketDataSubscriptionMutationRehearsalOperations = []string{
	"DELETE /api/v1/market-data/prediction/contracts/{code}/subscriptions/{leaseId}",
	"DELETE /api/v1/market-data/subscriptions",
	"POST /api/v1/market-data/prediction/contracts/{code}/subscriptions",
	"POST /api/v1/market-data/subscriptions",
	"POST /api/v1/market-data/subscriptions/heartbeat",
	"POST /api/v1/market-data/subscriptions/release",
}

type marketDataSubscriptionMutationRehearsalTarget struct {
	endpoint string
	token    string
}

func (t marketDataSubscriptionMutationRehearsalTarget) Endpoint() string { return t.endpoint }

func (t marketDataSubscriptionMutationRehearsalTarget) BearerToken() string { return t.token }

func (marketDataSubscriptionMutationRehearsalTarget) Profile() string {
	return "market-data-subscription-mutation-test-cutover.v1"
}

func (marketDataSubscriptionMutationRehearsalTarget) Capabilities() []string {
	return append([]string(nil), marketDataSubscriptionMutationRehearsalOperations...)
}

func TestMarketDataSubscriptionMutationRehearsalPreservesBrowserBoundaryAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before subscription rehearsal: %v", err)
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

	token := strings.Repeat("m", 64)
	const browserCookie = "jftrade_web_session=browser-rehearsal"
	const browserCSRF = "market-data-subscription-csrf"
	var expectedOrigin string
	var boundaryCalls atomic.Int32
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertMarketDataSubscriptionMutationPrivateBoundary(
			t, request, token, expectedOrigin, browserCookie, browserCSRF,
		)
		operation := request.Method + " " + request.URL.Path
		if !containsMarketDataSubscriptionMutationOperation(operation) {
			t.Errorf("unexpected Rust market-data subscription operation: %q", operation)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read Rust market-data subscription body: %v", err)
		}
		if err := assertMarketDataSubscriptionMutationBody(request.Method, request.URL.Path, body); err != nil {
			t.Error(err)
		}

		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"BROKER_FEATURE_FAILED","message":"fixture provider failed"}}`))
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
		RehearsalTarget:       marketDataSubscriptionMutationRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   marketDataSubscriptionMutationRehearsalOperations,
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

	requests := []marketDataSubscriptionMutationRehearsalRequestCase{
		{method: http.MethodDelete, path: "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions/lease-1"},
		{method: http.MethodDelete, path: "/api/v1/market-data/subscriptions?consumerId=chart"},
		{method: http.MethodPost, path: "/api/v1/market-data/prediction/contracts/EC-42/subscriptions", body: `{"dataTypes":["ORDER_BOOK"]}`},
		{method: http.MethodPost, path: "/api/v1/market-data/subscriptions", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}`},
		{method: http.MethodPost, path: "/api/v1/market-data/subscriptions/heartbeat", body: `{"consumerId":"chart"}`},
		{method: http.MethodPost, path: "/api/v1/market-data/subscriptions/release", body: `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}`},
	}
	for index, requestCase := range requests {
		response := marketDataSubscriptionMutationRehearsalRequest(
			t, proxyServer.URL+requestCase.path, requestCase.method, requestCase.body,
			"market-data-subscription-success-"+string(rune('1'+index)), expectedOrigin,
			browserCookie, browserCSRF,
		)
		assertMarketDataSubscriptionMutationSuccess(t, response, requestCase.path)
	}

	response := marketDataSubscriptionMutationRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/market-data/subscriptions?rehearsalFailure=error",
		http.MethodPost,
		`{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}`,
		"market-data-subscription-error", expectedOrigin, browserCookie, browserCSRF,
	)
	assertMarketDataSubscriptionMutationError(t, response, http.StatusBadGateway, "BROKER_FEATURE_FAILED")

	response = marketDataSubscriptionMutationRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/market-data/subscriptions/heartbeat?rehearsalFailure=timeout",
		http.MethodPost, `{"consumerId":"chart"}`, "market-data-subscription-timeout",
		expectedOrigin, browserCookie, browserCSRF,
	)
	assertMarketDataSubscriptionMutationError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := marketDataSubscriptionMutationRehearsalDo(
			cancelContext,
			proxyServer.URL+"/api/v1/market-data/subscriptions/release?rehearsalFailure=cancel",
			http.MethodPost, `{"consumerId":"chart","instruments":[]}`,
			"market-data-subscription-cancel", expectedOrigin, browserCookie, browserCSRF,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust market-data subscription rehearsal did not receive cancellation")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("market-data subscription cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("market-data subscription cancellation did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("Rust market-data subscription cancellation was not observed")
	}

	if boundaryCalls.Load() != int32(len(requests)+3) {
		t.Fatalf("authenticated market-data subscription boundary calls = %d, want %d", boundaryCalls.Load(), len(requests)+3)
	}
	rust.Close()
	crashResponse := marketDataSubscriptionMutationRehearsalRequest(
		t,
		proxyServer.URL+"/api/v1/market-data/subscriptions",
		http.MethodPost,
		`{"consumerId":"chart","instruments":[]}`,
		"market-data-subscription-crash", expectedOrigin, browserCookie, browserCSRF,
	)
	assertMarketDataSubscriptionMutationError(t, crashResponse, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")

	goResponse := marketDataSubscriptionMutationRehearsalRequest(
		t, goServer.URL+"/api/v1/market-data/subscriptions", http.MethodPost,
		`{"consumerId":"chart","instruments":[]}`, "market-data-subscription-go-rollback",
		goServer.URL, browserCookie, browserCSRF,
	)
	assertMarketDataSubscriptionMutationGoFallback(t, goResponse)
	closeGoOwner()
	closeProxyOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after subscription rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := marketDataSubscriptionMutationRehearsalRequest(
		t, restartedServer.URL+"/api/v1/market-data/subscriptions", http.MethodPost,
		`{"consumerId":"chart","instruments":[]}`, "market-data-subscription-go-restart",
		restartedServer.URL, browserCookie, browserCSRF,
	)
	assertMarketDataSubscriptionMutationGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after subscription rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("market-data subscription rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

type marketDataSubscriptionMutationRehearsalRequestCase struct {
	method string
	path   string
	body   string
}

func assertMarketDataSubscriptionMutationPrivateBoundary(t *testing.T, request *http.Request, token, origin, cookie, csrf string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "web" {
		t.Errorf("Rust market-data subscription private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       cookie,
		"Origin":       origin,
		"Referer":      origin + "/market-data",
		"X-CSRF-Token": csrf,
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust market-data subscription boundary %s = %q, want %q", name, got, want)
		}
	}
}

func assertMarketDataSubscriptionMutationBody(method, path string, body []byte) error {
	want := ""
	if method == http.MethodDelete {
		if len(body) != 0 {
			return errors.New("DELETE market-data subscription rehearsal body = " + string(body) + ", want empty")
		}
		return nil
	}
	switch {
	case path == "/api/v1/market-data/subscriptions":
		want = `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}`
	case path == "/api/v1/market-data/subscriptions/heartbeat":
		want = `{"consumerId":"chart"}`
	case path == "/api/v1/market-data/subscriptions/release":
		if string(body) == `{"consumerId":"chart","instruments":[]}` {
			return nil
		}
		want = `{"consumerId":"chart","instruments":[{"market":"US","symbol":"AAPL"}]}`
	case strings.HasSuffix(path, "/prediction/contracts/EC-42/subscriptions"):
		want = `{"dataTypes":["ORDER_BOOK"]}`
	default:
		return errors.New("unknown market-data subscription rehearsal path " + path)
	}
	if string(body) != want {
		return errors.New("market-data subscription rehearsal body for " + path + " = " + string(body) + ", want " + want)
	}
	return nil
}

func containsMarketDataSubscriptionMutationOperation(operation string) bool {
	for _, candidate := range marketDataSubscriptionMutationRehearsalOperations {
		method, path, ok := strings.Cut(candidate, " ")
		if ok && method == strings.TrimSpace(strings.SplitN(operation, " ", 2)[0]) {
			if path == "/api/v1/market-data/subscriptions" && strings.HasPrefix(operation, method+" "+path) {
				return true
			}
			if strings.Contains(path, "{code}") && strings.HasPrefix(operation, method+" /api/v1/market-data/prediction/contracts/") {
				return true
			}
			if path == operation[len(method)+1:] {
				return true
			}
		}
	}
	return false
}

func marketDataSubscriptionMutationRehearsalRequest(t *testing.T, target, method, body, requestID, origin, cookie, csrf string) *http.Response {
	t.Helper()
	response, err := marketDataSubscriptionMutationRehearsalDo(
		context.Background(), target, method, body, requestID, origin, cookie, csrf,
	)
	if err != nil {
		t.Fatalf("call market-data subscription rehearsal route: %v", err)
	}
	return response
}

func marketDataSubscriptionMutationRehearsalDo(ctx context.Context, target, method, body, requestID, origin, cookie, csrf string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-market-data-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/market-data")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertMarketDataSubscriptionMutationSuccess(t *testing.T, response *http.Response, path string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read market-data subscription success: %v", err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("market-data subscription success for %s = %d %#v %s", path, response.StatusCode, response.Header, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode market-data subscription success for %s: %v; body=%s", path, err, body)
	}
	if envelope["ok"] != true || envelope["data"].(map[string]any)["source"] != "rust-rehearsal" {
		t.Fatalf("market-data subscription success for %s = %#v", path, envelope)
	}
}

func assertMarketDataSubscriptionMutationError(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read market-data subscription error: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("market-data subscription error status = %d, want %d; body=%s", response.StatusCode, wantStatus, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode market-data subscription error: %v; body=%s", err, body)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != wantCode {
		t.Fatalf("market-data subscription error = %#v, want code %s", envelope, wantCode)
	}
}

func assertMarketDataSubscriptionMutationGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go market-data subscription fallback: %v", err)
	}
	if strings.Contains(string(body), "rust-rehearsal") || strings.Contains(string(body), "RUST_FIXTURE") {
		t.Fatalf("Go market-data subscription fallback unexpectedly used Rust: status=%d body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go market-data subscription fallback: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		t.Fatalf("Go market-data subscription fallback unexpectedly succeeded without an error envelope: status=%d body=%s", response.StatusCode, body)
	}
}
