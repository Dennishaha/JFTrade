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

var researchScreensWriteRehearsalOperations = []string{
	"POST /api/v1/research/screens",
}

type researchScreensWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t researchScreensWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t researchScreensWriteRehearsalTarget) BearerToken() string { return t.token }

func (researchScreensWriteRehearsalTarget) Profile() string {
	return "research-screen-test-cutover.v1"
}

func (researchScreensWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), researchScreensWriteRehearsalOperations...)
}

func TestResearchScreensWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before research-screens rehearsal: %v", err)
	}

	goOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	goServer := httptest.NewServer(goOwner)
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

	token := strings.Repeat("r", 64)
	projection := researchScreensFixtureProjection(t, "valid-page-result")
	var boundaryCalls atomic.Int32
	var cancelObserved atomic.Int32
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	var proxyOrigin string
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertResearchScreensWritePrivateBoundary(t, request, token, proxyOrigin)
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/research/screens" {
			t.Errorf("unexpected research-screens rehearsal operation: %s %s", request.Method, request.URL.Path)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read research-screens rehearsal body: %v", err)
		}
		if !strings.Contains(string(body), `"querySchemaVersion":2`) {
			t.Errorf("research-screens rehearsal body lost the typed query: %s", body)
		}
		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
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
		RehearsalTarget:       researchScreensWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   researchScreensWriteRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	proxyOrigin = proxyServer.URL
	t.Cleanup(func() {
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	})

	path := "/api/v1/research/screens"
	body := `{"brokerId":" API-TEST ","market":"us","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"page":{"offset":50,"limit":25}}`
	for index := 1; index <= 2; index++ {
		requestID := fmt.Sprintf("research-screens-success-%d", index)
		response := researchScreensWriteRehearsalRequest(
			t, proxyServer.URL+path, body, requestID, proxyOrigin,
		)
		assertResearchScreensWriteResponse(t, response, http.StatusOK, requestID, projection)
	}

	response := researchScreensWriteRehearsalRequest(
		t, proxyServer.URL+path+"?rehearsalFailure=error", body, "research-screens-error", proxyOrigin,
	)
	assertResearchScreensWriteError(t, response, http.StatusUnprocessableEntity, "research-screens-error", "RUST_FIXTURE_ERROR")
	response = researchScreensWriteRehearsalRequest(
		t, proxyServer.URL+path+"?rehearsalFailure=timeout", body, "research-screens-timeout", proxyOrigin,
	)
	assertResearchScreensWriteError(t, response, http.StatusGatewayTimeout, "research-screens-timeout", "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := researchScreensWriteRehearsalDo(
			cancelContext,
			proxyServer.URL+path+"?rehearsalFailure=cancel",
			body,
			"research-screens-cancel",
			proxyOrigin,
		)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust research-screens rehearsal did not receive the cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("research-screens cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("research-screens cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatalf("Rust cancellation observations = %d, want 1", cancelObserved.Load())
	}

	rust.Close()
	response = researchScreensWriteRehearsalRequest(
		t, proxyServer.URL+path, body, "research-screens-crash", proxyOrigin,
	)
	assertResearchScreensWriteError(t, response, http.StatusBadGateway, "research-screens-crash", "RUST_REHEARSAL_UNAVAILABLE")
	if boundaryCalls.Load() != 5 {
		t.Fatalf("authenticated research-screens boundary calls = %d, want 5", boundaryCalls.Load())
	}

	fallbackPath := path + "?brokerId=missing&market=US"
	goResponse := researchScreensWriteRehearsalRequest(
		t, goServer.URL+fallbackPath, body, "research-screens-go-rollback", goServer.URL,
	)
	assertResearchScreensWriteGoFallback(t, goResponse, "research-screens-go-rollback", projection)
	closeGoOwner()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after research-screens rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner)
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := researchScreensWriteRehearsalRequest(
		t,
		restartedServer.URL+fallbackPath,
		body,
		"research-screens-go-restart",
		restartedServer.URL,
	)
	assertResearchScreensWriteGoFallback(t, restartedResponse, "research-screens-go-restart", projection)

	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after research-screens rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("research-screens rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertResearchScreensWritePrivateBoundary(t *testing.T, request *http.Request, token, origin string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
		t.Errorf("Rust research-screens mutation boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       "jftrade_web_session=browser-rehearsal",
		"Origin":       origin,
		"Referer":      origin + "/research",
		"X-CSRF-Token": "research-screens-csrf",
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust research-screens boundary %s = %q, want %q", name, got, want)
		}
	}
}

func researchScreensWriteRehearsalRequest(t *testing.T, target, body, requestID, origin string) *http.Response {
	t.Helper()
	response, err := researchScreensWriteRehearsalDo(
		context.Background(), target, body, requestID, origin,
	)
	if err != nil {
		t.Fatalf("call research-screens rehearsal route: %v", err)
	}
	return response
}

func researchScreensWriteRehearsalDo(ctx context.Context, target, body, requestID, origin string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-desktop-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "jftrade_web_session=browser-rehearsal")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/research")
	request.Header.Set("X-CSRF-Token", "research-screens-csrf")
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertResearchScreensWriteResponse(t *testing.T, response *http.Response, wantStatus int, requestID string, wantBody []byte) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read research-screens response: %v", err)
	}
	if response.StatusCode != wantStatus || string(body) != string(wantBody) {
		t.Fatalf("research-screens response %s = %d %s, want %d %s", requestID, response.StatusCode, body, wantStatus, wantBody)
	}
	if response.Header.Get("X-Request-ID") != requestID ||
		response.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("research-screens response headers %s = %#v", requestID, response.Header)
	}
}

func assertResearchScreensWriteError(t *testing.T, response *http.Response, wantStatus int, requestID, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read research-screens error response: %v", err)
	}
	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantCode) {
		t.Fatalf("research-screens error response %s = %d %s, want %d containing %s", requestID, response.StatusCode, body, wantStatus, wantCode)
	}
	if response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("research-screens error response ID %s = %q", requestID, response.Header.Get("X-Request-ID"))
	}
}

func assertResearchScreensWriteGoFallback(t *testing.T, response *http.Response, requestID string, rustBody []byte) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go research-screens fallback response: %v", err)
	}
	if response.StatusCode == http.StatusOK || string(body) == string(rustBody) {
		t.Fatalf("Go research-screens fallback response = %d %s", response.StatusCode, body)
	}
	if response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("Go research-screens fallback response ID = %q", response.Header.Get("X-Request-ID"))
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go research-screens fallback response: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok {
		t.Fatalf("Go research-screens fallback error envelope = %#v", envelope)
	}
}

func researchScreensFixtureProjection(t *testing.T, name string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve research-screens rehearsal source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source), "../../../../tests/fixtures/rust-migration/stage9/research-screens.json",
	)
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read research-screens fixture: %v", err)
	}
	var fixture struct {
		Cases []struct {
			Name     string `json:"name"`
			Expected []struct {
				Envelope json.RawMessage `json:"envelope"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode research-screens fixture: %v", err)
	}
	for _, testCase := range fixture.Cases {
		if testCase.Name == name && len(testCase.Expected) > 0 {
			return append([]byte(nil), testCase.Expected[0].Envelope...)
		}
	}
	t.Fatalf("research-screens fixture case %q is missing an expected envelope", name)
	return nil
}
