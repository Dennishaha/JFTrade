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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

var strategyPineRehearsalOperations = []string{
	"POST /api/v1/strategy-pine/analyze",
}

type strategyPineRehearsalTarget struct {
	endpoint string
	token    string
}

func (t strategyPineRehearsalTarget) Endpoint() string { return t.endpoint }

func (t strategyPineRehearsalTarget) BearerToken() string { return t.token }

func (strategyPineRehearsalTarget) Profile() string {
	return "analysis-test-cutover.v1"
}

func (strategyPineRehearsalTarget) Capabilities() []string {
	return append([]string(nil), strategyPineRehearsalOperations...)
}

func TestStrategyPineRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before strategy-pine rehearsal: %v", err)
	}

	goOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	goServer := httptest.NewServer(goOwner)
	t.Cleanup(func() {
		goServer.Close()
		jftradeCheckTestError(t, goOwner.Close())
	})

	token := strings.Repeat("p", 64)
	projection := strategyPineFixtureProjection(t, "success-default-source-format")
	var boundaryCalls atomic.Int32
	var cancelObserved atomic.Int32
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	var proxyOrigin string
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertStrategyPinePrivateBoundary(t, request, token, proxyOrigin)
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/strategy-pine/analyze" {
			t.Errorf("unexpected strategy-pine rehearsal operation: %s %s", request.Method, request.URL.Path)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read strategy-pine rehearsal body: %v", err)
		}
		if !strings.Contains(string(body), `"script"`) {
			t.Errorf("strategy-pine rehearsal body lost script: %s", body)
		}
		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json")
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
			_, _ = w.Write(projection)
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       strategyPineRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   strategyPineRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	proxyOrigin = proxyServer.URL
	t.Cleanup(func() {
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	})

	path := "/api/v1/strategy-pine/analyze"
	body := `{"script":"//@version=6\nstrategy(\"fixture\")\nplot(close)"}`
	for index := 1; index <= 2; index++ {
		requestID := fmt.Sprintf("strategy-pine-success-%d", index)
		response := strategyPineRehearsalRequest(t, proxyServer.URL+path, body, requestID, proxyOrigin)
		assertStrategyPineResponse(t, response, http.StatusOK, requestID, projection)
	}

	response := strategyPineRehearsalRequest(
		t, proxyServer.URL+path+"?rehearsalFailure=error", body, "strategy-pine-error", proxyOrigin,
	)
	assertStrategyPineErrorResponse(t, response, http.StatusUnprocessableEntity, "strategy-pine-error", "RUST_FIXTURE_ERROR")
	response = strategyPineRehearsalRequest(
		t, proxyServer.URL+path+"?rehearsalFailure=timeout", body, "strategy-pine-timeout", proxyOrigin,
	)
	assertStrategyPineErrorResponse(t, response, http.StatusGatewayTimeout, "strategy-pine-timeout", "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := strategyPineRehearsalDo(
			cancelContext, proxyServer.URL+path+"?rehearsalFailure=cancel", body, "strategy-pine-cancel", proxyOrigin,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust strategy-pine rehearsal did not receive the cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("strategy-pine cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("strategy-pine cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatalf("Rust cancellation observations = %d, want 1", cancelObserved.Load())
	}

	rust.Close()
	response = strategyPineRehearsalRequest(t, proxyServer.URL+path, body, "strategy-pine-crash", proxyOrigin)
	assertStrategyPineErrorResponse(t, response, http.StatusBadGateway, "strategy-pine-crash", "RUST_REHEARSAL_UNAVAILABLE")
	if boundaryCalls.Load() != 5 {
		t.Fatalf("authenticated strategy-pine boundary calls = %d, want 5", boundaryCalls.Load())
	}

	goResponse := strategyPineRehearsalRequest(t, goServer.URL+path, body, "strategy-pine-go-rollback", goServer.URL)
	assertStrategyPineGoFallback(t, goResponse, "strategy-pine-go-rollback", projection)
	goServer.Close()
	jftradeCheckTestError(t, goOwner.Close())

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after strategy-pine rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner)
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := strategyPineRehearsalRequest(t, restartedServer.URL+path, body, "strategy-pine-go-restart", restartedServer.URL)
	assertStrategyPineGoFallback(t, restartedResponse, "strategy-pine-go-restart", projection)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after strategy-pine rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("strategy-pine rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertStrategyPinePrivateBoundary(t *testing.T, request *http.Request, token, origin string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
		t.Errorf("Rust strategy-pine private boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       "jftrade_web_session=browser-rehearsal",
		"Origin":       origin,
		"Referer":      origin + "/strategy",
		"X-CSRF-Token": "strategy-pine-csrf",
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust strategy-pine boundary %s = %q, want %q", name, got, want)
		}
	}
}

func strategyPineRehearsalRequest(t *testing.T, target, body, requestID, origin string) *http.Response {
	t.Helper()
	response, err := strategyPineRehearsalDo(context.Background(), target, body, requestID, origin)
	if err != nil {
		t.Fatalf("call strategy-pine rehearsal route: %v", err)
	}
	return response
}

func strategyPineRehearsalDo(ctx context.Context, target, body, requestID, origin string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-desktop-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "jftrade_web_session=browser-rehearsal")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/strategy")
	request.Header.Set("X-CSRF-Token", "strategy-pine-csrf")
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertStrategyPineResponse(t *testing.T, response *http.Response, wantStatus int, requestID string, wantBody []byte) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read strategy-pine response: %v", err)
	}
	if response.StatusCode != wantStatus || string(body) != string(wantBody) {
		t.Fatalf("strategy-pine response %s = %d %s, want %d %s", requestID, response.StatusCode, body, wantStatus, wantBody)
	}
	if response.Header.Get("X-Request-ID") != requestID ||
		response.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("strategy-pine response headers %s = %#v", requestID, response.Header)
	}
}

func assertStrategyPineErrorResponse(t *testing.T, response *http.Response, wantStatus int, requestID, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read strategy-pine error response: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf("strategy-pine error response %s = %d %s, want %d containing %s", requestID, response.StatusCode, body, wantStatus, wantCode)
	}
	if response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("strategy-pine error response ID %s = %q", requestID, response.Header.Get("X-Request-ID"))
	}
}

func assertStrategyPineGoFallback(t *testing.T, response *http.Response, requestID string, rustBody []byte) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go strategy-pine fallback response: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(body) == string(rustBody) {
		t.Fatalf("Go strategy-pine fallback response = %d %s", response.StatusCode, body)
	}
	if response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("Go strategy-pine fallback response ID = %q", response.Header.Get("X-Request-ID"))
	}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || !envelope.OK || len(envelope.Data) == 0 {
		t.Fatalf("decode Go strategy-pine fallback response: %v; body=%s", err, body)
	}
}

func strategyPineFixtureProjection(t *testing.T, name string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve strategy-pine rehearsal source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../../../tests/fixtures/rust-migration/stage9/strategy-pine.json",
	)
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read strategy-pine fixture: %v", err)
	}
	var fixture struct {
		Cases []struct {
			Name string          `json:"name"`
			Data json.RawMessage `json:"data"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode strategy-pine fixture: %v", err)
	}
	for _, testCase := range fixture.Cases {
		if testCase.Name == name {
			body, err := json.Marshal(struct {
				OK   bool            `json:"ok"`
				Data json.RawMessage `json:"data"`
			}{OK: true, Data: testCase.Data})
			if err != nil {
				t.Fatalf("encode strategy-pine fixture projection: %v", err)
			}
			return body
		}
	}
	t.Fatalf("strategy-pine fixture case %q not found", name)
	return nil
}
