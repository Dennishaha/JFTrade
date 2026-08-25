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

const adkMutationRehearsalToken = "adk-mutation-private-bearer"

var adkMutationRehearsalOperations = []string{
	"DELETE /api/v1/adk/agents/{agentId}",
	"DELETE /api/v1/adk/memory/{memoryId}",
	"DELETE /api/v1/adk/providers/{providerId}",
	"DELETE /api/v1/adk/sessions/{sessionId}",
	"DELETE /api/v1/adk/skills/{skillId}",
	"DELETE /api/v1/adk/tasks/{taskId}",
	"DELETE /api/v1/adk/workflows/{workflowId}",
	"DELETE /api/v1/adk/workflows/{workflowId}/triggers/{triggerId}",
	"PATCH /api/v1/adk/runs/{runId}/objective",
	"PATCH /api/v1/adk/sessions/{sessionId}/composer-state",
	"POST /api/v1/adk/agents",
	"POST /api/v1/adk/approvals/{approvalId}/approve",
	"POST /api/v1/adk/approvals/{approvalId}/deny",
	"POST /api/v1/adk/memory",
	"POST /api/v1/adk/optimization-tasks/{taskId}/cancel",
	"POST /api/v1/adk/providers",
	"POST /api/v1/adk/providers/{providerId}/default",
	"POST /api/v1/adk/providers/{providerId}/test",
	"POST /api/v1/adk/runs/{runId}/cancel",
	"POST /api/v1/adk/runs/{runId}/input-response",
	"POST /api/v1/adk/runs/{runId}/pause",
	"POST /api/v1/adk/runs/{runId}/resume",
	"POST /api/v1/adk/sessions",
	"POST /api/v1/adk/sessions/{sessionId}/context/compact",
	"POST /api/v1/adk/skills",
	"POST /api/v1/adk/tasks",
	"POST /api/v1/adk/workflow-triggers/{triggerId}/run",
	"POST /api/v1/adk/workflow-webhooks/{triggerId}",
	"POST /api/v1/adk/workflows",
	"POST /api/v1/adk/workflows/{workflowId}/run",
	"POST /api/v1/adk/workflows/{workflowId}/triggers",
	"PUT /api/v1/adk/agents/{agentId}",
	"PUT /api/v1/adk/providers/{providerId}",
	"PUT /api/v1/adk/sessions/{sessionId}",
	"PUT /api/v1/adk/tasks/{taskId}",
	"PUT /api/v1/adk/workflows/{workflowId}",
	"PUT /api/v1/adk/workflows/{workflowId}/triggers/{triggerId}",
}

type adkMutationRehearsalTarget struct {
	endpoint string
}

func (t adkMutationRehearsalTarget) Endpoint() string     { return t.endpoint }
func (adkMutationRehearsalTarget) BearerToken() string    { return adkMutationRehearsalToken }
func (adkMutationRehearsalTarget) Profile() string        { return "adk-mutations-test-cutover.v1" }
func (adkMutationRehearsalTarget) Capabilities() []string { return adkMutationRehearsalOperations }

type adkMutationRehearsalFixture struct {
	Cases []adkMutationRehearsalCase `json:"cases"`
}

type adkMutationRehearsalCase struct {
	Name        string                          `json:"name"`
	Method      string                          `json:"method"`
	RequestPath string                          `json:"requestPath"`
	Body        *string                         `json:"body"`
	Expected    adkMutationRehearsalExpectation `json:"expected"`
}

type adkMutationRehearsalExpectation struct {
	Status   int                    `json:"status"`
	Headers  map[string]string      `json:"headers"`
	PortCall bool                   `json:"portCall"`
	Envelope map[string]interface{} `json:"envelope"`
}

func TestADKMutationRehearsalPreservesAuthenticatedOwnerFencingAcrossRecovery(t *testing.T) {
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
		t.Fatalf("read settings before ADK mutation rehearsal: %v", err)
	}

	fixture := loadADKMutationRehearsalFixture(t)
	var calls atomic.Int32
	var expectedOrigin string
	var cancelReadyOnce sync.Once
	cancelReady := make(chan struct{})
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		assertADKMutationPrivateBoundary(t, request, expectedOrigin)
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil {
			t.Errorf("read Rust ADK mutation body: %v", readErr)
		}
		if request.URL.Query().Get("rehearsalFailure") == "timeout" {
			<-request.Context().Done()
			return
		}
		if request.URL.Query().Get("rehearsalFailure") == "cancel" {
			cancelReadyOnce.Do(func() { close(cancelReady) })
			<-request.Context().Done()
			return
		}
		fixtureCase := findADKMutationRehearsalCase(fixture.Cases, request, body)
		if fixtureCase == nil {
			t.Errorf("Rust ADK mutation request did not match fixture: %s %s body=%q", request.Method, request.URL.EscapedPath(), body)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		for name, value := range fixtureCase.Expected.Headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(fixtureCase.Expected.Status)
		if err := json.NewEncoder(w).Encode(fixtureCase.Expected.Envelope); err != nil {
			t.Errorf("write Rust ADK mutation fixture response: %v", err)
		}
	}))
	t.Cleanup(rust.Close)

	goOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	goServer := httptest.NewServer(goOwner.WebAccessHandler())
	defer func() {
		goServer.Close()
		jftradeCheckTestError(t, goOwner.Close())
	}()
	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       adkMutationRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   adkMutationRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner.WebAccessHandler())
	t.Cleanup(func() {
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	})
	expectedOrigin = proxyServer.URL

	validCases := 0
	for _, fixtureCase := range fixture.Cases {
		response := adkMutationRehearsalRequest(t, proxyServer.URL+fixtureCase.RequestPath, fixtureCase, expectedOrigin)
		assertADKMutationFixtureResponse(t, response, fixtureCase)
		if fixtureCase.Expected.PortCall {
			validCases++
		}
	}
	if validCases != len(adkMutationRehearsalOperations) || calls.Load() != int32(len(fixture.Cases)) {
		t.Fatalf("authenticated ADK mutation calls = %d, valid fixture cases = %d, want %d forwarded cases", calls.Load(), validCases, len(fixture.Cases))
	}

	duplicate := fixtureCaseByName(t, fixture.Cases, "agent-create")
	response := adkMutationRehearsalRequest(t, proxyServer.URL+duplicate.RequestPath, duplicate, expectedOrigin)
	assertADKMutationFixtureResponse(t, response, duplicate)
	if calls.Load() != int32(len(fixture.Cases)+1) {
		t.Fatalf("duplicate ADK mutation was not forwarded exactly once: calls=%d", calls.Load())
	}

	trailing := *duplicate.Body + ` {"ignored":true}`
	response = adkMutationRehearsalDo(t, context.Background(), proxyServer.URL+duplicate.RequestPath, duplicate.Method, &trailing, expectedOrigin)
	assertADKMutationFixtureResponse(t, response, duplicate)

	timeoutCase := fixtureCaseByName(t, fixture.Cases, "agent-create")
	response = adkMutationRehearsalDo(t, context.Background(), proxyServer.URL+timeoutCase.RequestPath+"?rehearsalFailure=timeout", timeoutCase.Method, timeoutCase.Body, expectedOrigin)
	assertADKMutationError(t, response, http.StatusGatewayTimeout, "RUST_REHEARSAL_TIMEOUT")

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelDone := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(cancelContext, http.MethodPost, proxyServer.URL+duplicate.RequestPath+"?rehearsalFailure=cancel", strings.NewReader(*duplicate.Body))
		if err != nil {
			cancelDone <- err
			return
		}
		setADKMutationBrowserHeaders(request, expectedOrigin)
		_, err = http.DefaultClient.Do(request)
		cancelDone <- err
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust ADK mutation rehearsal did not receive cancellation request")
	}
	select {
	case err := <-cancelDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ADK mutation cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ADK mutation cancellation request did not return")
	}

	rust.Close()
	response = adkMutationRehearsalDo(t, context.Background(), proxyServer.URL+duplicate.RequestPath, duplicate.Method, duplicate.Body, expectedOrigin)
	assertADKMutationError(t, response, http.StatusBadGateway, "RUST_REHEARSAL_UNAVAILABLE")

	goResponse := adkMutationRehearsalDo(t, context.Background(), goServer.URL+"/api/v1/adk/agents", http.MethodPost, ptrString("{"), goServer.URL)
	assertADKMutationGoOwnerResponse(t, goResponse)
	proxyServer.Close()
	goServer.Close()
	jftradeCheckTestError(t, proxyOwner.Close())
	jftradeCheckTestError(t, goOwner.Close())

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after ADK mutation rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner.WebAccessHandler())
	response = adkMutationRehearsalDo(t, context.Background(), restartedServer.URL+"/api/v1/adk/agents", http.MethodPost, ptrString("{"), restartedServer.URL)
	assertADKMutationGoOwnerResponse(t, response)
	restartedServer.Close()
	jftradeCheckTestError(t, restartedOwner.Close())

	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after ADK mutation rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("ADK mutation rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func loadADKMutationRehearsalFixture(t *testing.T) adkMutationRehearsalFixture {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve ADK mutation rehearsal fixture")
	}
	path := filepath.Join(filepath.Dir(source), "../../../../tests/fixtures/rust-migration/stage9/adk-mutations.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ADK mutation rehearsal fixture: %v", err)
	}
	var fixture adkMutationRehearsalFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode ADK mutation rehearsal fixture: %v", err)
	}
	return fixture
}

func findADKMutationRehearsalCase(cases []adkMutationRehearsalCase, request *http.Request, body []byte) *adkMutationRehearsalCase {
	path := request.URL.EscapedPath()
	for index := range cases {
		fixtureCase := &cases[index]
		if fixtureCase.Method != request.Method || fixtureCase.RequestPath != path {
			continue
		}
		if fixtureCase.Body == nil && len(body) == 0 {
			return fixtureCase
		}
		if fixtureCase.Body != nil && string(body) == *fixtureCase.Body {
			return fixtureCase
		}
		if fixtureCase.Name == "agent-create" && string(body) != "{" && len(body) > 0 {
			return fixtureCase
		}
	}
	return nil
}

func fixtureCaseByName(t *testing.T, cases []adkMutationRehearsalCase, name string) adkMutationRehearsalCase {
	t.Helper()
	for _, fixtureCase := range cases {
		if fixtureCase.Name == name {
			return fixtureCase
		}
	}
	t.Fatalf("fixture case %q not found", name)
	return adkMutationRehearsalCase{}
}

func adkMutationRehearsalRequest(t *testing.T, target string, fixtureCase adkMutationRehearsalCase, origin string) *http.Response {
	t.Helper()
	return adkMutationRehearsalDo(t, context.Background(), target, fixtureCase.Method, fixtureCase.Body, origin)
}

func adkMutationRehearsalDo(t *testing.T, ctx context.Context, target, method string, body *string, origin string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(*body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		t.Fatalf("create ADK mutation rehearsal request: %v", err)
	}
	setADKMutationBrowserHeaders(request, origin)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call ADK mutation rehearsal route: %v", err)
	}
	return response
}

func setADKMutationBrowserHeaders(request *http.Request, origin string) {
	request.Header.Set("Authorization", "Bearer public-adk-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "jftrade_web_session=adk-mutation-browser")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/adk")
	request.Header.Set("X-CSRF-Token", "adk-mutation-csrf")
}

func assertADKMutationPrivateBoundary(t *testing.T, request *http.Request, origin string) {
	t.Helper()
	for name, want := range map[string]string{
		"Authorization":                   "Bearer " + adkMutationRehearsalToken,
		rustrehearsal.InternalProxyHeader: rustrehearsal.InternalProxyProtocol,
		rustrehearsal.AccessSurfaceHeader: "web",
		"Cookie":                          "jftrade_web_session=adk-mutation-browser",
		"Origin":                          origin,
		"Referer":                         origin + "/adk",
		"X-CSRF-Token":                    "adk-mutation-csrf",
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust ADK mutation boundary %s = %q, want %q", name, got, want)
		}
	}
	if request.Header.Get("Authorization") == "Bearer public-adk-token" {
		t.Error("public authorization credential crossed the Rust boundary")
	}
}

func assertADKMutationFixtureResponse(t *testing.T, response *http.Response, fixtureCase adkMutationRehearsalCase) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read ADK mutation rehearsal response: %v", err)
	}
	if response.StatusCode != fixtureCase.Expected.Status {
		t.Fatalf("ADK mutation %s status = %d, want %d; body=%s", fixtureCase.Name, response.StatusCode, fixtureCase.Expected.Status, body)
	}
	if got := response.Header.Get("Content-Type"); got != fixtureCase.Expected.Headers["Content-Type"] {
		t.Fatalf("ADK mutation %s Content-Type = %q, want %q", fixtureCase.Name, got, fixtureCase.Expected.Headers["Content-Type"])
	}
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode ADK mutation %s response: %v", fixtureCase.Name, err)
	}
	if !equalJSON(got, fixtureCase.Expected.Envelope) {
		t.Fatalf("ADK mutation %s body = %#v, want %#v", fixtureCase.Name, got, fixtureCase.Expected.Envelope)
	}
}

func assertADKMutationError(t *testing.T, response *http.Response, status int, code string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read ADK mutation failure: %v", err)
	}
	if response.StatusCode != status || !strings.Contains(string(body), code) {
		t.Fatalf("ADK mutation failure = %d %s, want %d containing %q", response.StatusCode, body, status, code)
	}
}

func assertADKMutationGoOwnerResponse(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go ADK mutation rollback response: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go ADK mutation rollback response = %d %s", response.StatusCode, body)
	}
}

func ptrString(value string) *string { return &value }

func equalJSON(left, right map[string]interface{}) bool {
	leftBytes, _ := json.Marshal(left)
	rightBytes, _ := json.Marshal(right)
	return string(leftBytes) == string(rightBytes)
}
