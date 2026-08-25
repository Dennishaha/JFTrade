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

var watchlistWriteRehearsalOperations = []string{
	"DELETE /api/v1/watchlist/bindings",
	"DELETE /api/v1/watchlist/groups/{groupId}",
	"PATCH /api/v1/watchlist/groups/{groupId}",
	"POST /api/v1/watchlist/groups",
	"POST /api/v1/watchlist/imports/preview",
	"POST /api/v1/watchlist/imports/{previewId}/commit",
	"POST /api/v1/watchlist/quotes/batch",
	"PUT /api/v1/watchlist/instruments/{market}/{symbol}/memberships",
}

type watchlistWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t watchlistWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t watchlistWriteRehearsalTarget) BearerToken() string { return t.token }

func (watchlistWriteRehearsalTarget) Profile() string {
	return "watchlist-write-test-cutover.v1"
}

func (watchlistWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), watchlistWriteRehearsalOperations...)
}

type watchlistWriteRehearsalRequestSpec struct {
	method string
	path   string
	body   string
}

func TestWatchlistWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before watchlist rehearsal: %v", err)
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

	token := strings.Repeat("w", 64)
	origin := ""
	var boundaryCalls atomic.Int32
	var seenMu sync.Mutex
	seen := make(map[string]int)
	cancelReady := make(chan struct{})
	cancelDone := make(chan struct{})
	var cancelReadyOnce sync.Once
	var cancelDoneOnce sync.Once
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertWatchlistWritePrivateBoundary(t, request, token, origin)
		operation := watchlistWriteOperation(request)
		if !containsWatchlistWriteOperation(operation) {
			t.Errorf("unexpected Rust watchlist write operation: %q", operation)
		}
		if body, err := io.ReadAll(request.Body); err != nil {
			t.Errorf("read Rust watchlist write body: %v", err)
		} else if want := watchlistWriteExpectedBody(operation); string(body) != want {
			t.Errorf("Rust watchlist write body for %s = %q, want %q", operation, body, want)
		}
		seenMu.Lock()
		seen[operation]++
		seenMu.Unlock()

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
			cancelDoneOnce.Do(func() { close(cancelDone) })
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"ok":true,"data":{"accepted":true,"route":%q},"timestamp":"2026-08-26T00:00:00Z"}`, operation)
		}
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       watchlistWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   watchlistWriteRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	origin = proxyServer.URL
	proxyClosed := false
	closeProxy := func() {
		if proxyClosed {
			return
		}
		proxyClosed = true
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	}
	t.Cleanup(closeProxy)

	specs := []watchlistWriteRehearsalRequestSpec{
		{http.MethodDelete, "/api/v1/watchlist/bindings?bindingId=binding-1", ""},
		{http.MethodDelete, "/api/v1/watchlist/groups/group-1", ""},
		{http.MethodPatch, "/api/v1/watchlist/groups/group-1", `{"name":"Growth","expectedRevision":2}`},
		{http.MethodPost, "/api/v1/watchlist/groups", `{"name":"Growth"}`},
		{http.MethodPost, "/api/v1/watchlist/imports/preview", `{"sourceId":"source-1","remoteGroupId":"remote-1","localGroupId":"local-1","newGroupName":"Imported"}`},
		{http.MethodPost, "/api/v1/watchlist/imports/preview-1/commit", `{"deleteInstrumentIds":["US:AAPL"]}`},
		{http.MethodPost, "/api/v1/watchlist/quotes/batch", `{"instrumentIds":["US:AAPL"]}`},
		{http.MethodPut, "/api/v1/watchlist/instruments/US/AAPL/memberships", `{"groupIds":["group-1"],"newGroupNames":[],"expectedRevision":2}`},
	}
	for index, spec := range specs {
		requestID := fmt.Sprintf("watchlist-write-success-%d", index+1)
		response := watchlistWriteRehearsalRequest(t, proxyServer.URL+spec.path, spec.method, spec.body, requestID, origin)
		assertWatchlistWriteStatus(t, response, http.StatusOK, requestID)
	}
	duplicate := specs[3]
	response := watchlistWriteRehearsalRequest(t, proxyServer.URL+duplicate.path, duplicate.method, duplicate.body, "watchlist-write-duplicate", origin)
	assertWatchlistWriteStatus(t, response, http.StatusOK, "watchlist-write-duplicate")

	errorSpec := specs[3]
	response = watchlistWriteRehearsalRequest(t, proxyServer.URL+errorSpec.path+"?rehearsalFailure=error", errorSpec.method, errorSpec.body, "watchlist-write-error", origin)
	assertWatchlistWriteStatus(t, response, http.StatusUnprocessableEntity, "watchlist-write-error")
	response = watchlistWriteRehearsalRequest(t, proxyServer.URL+errorSpec.path+"?rehearsalFailure=timeout", errorSpec.method, errorSpec.body, "watchlist-write-timeout", origin)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("watchlist write timeout status = %d, want 504", response.StatusCode)
	}

	cancelContext, cancel := context.WithCancel(context.Background())
	cancelError := make(chan error, 1)
	go func() {
		_, requestErr := watchlistWriteRehearsalDo(cancelContext, proxyServer.URL+errorSpec.path+"?rehearsalFailure=cancel", errorSpec.method, errorSpec.body, "watchlist-write-cancel", origin)
		cancelError <- requestErr
	}()
	select {
	case <-cancelReady:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("Rust watchlist write rehearsal did not receive cancellation request")
	}
	select {
	case requestErr := <-cancelError:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("watchlist write cancellation error = %v", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("watchlist write cancellation request did not return")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("watchlist write cancellation was not observed by Rust")
	}

	rust.Close()
	crashResponse := watchlistWriteRehearsalRequest(t, proxyServer.URL+specs[0].path, specs[0].method, specs[0].body, "watchlist-write-crash", origin)
	assertWatchlistWriteError(t, crashResponse, "watchlist-write-crash", "RUST_REHEARSAL_UNAVAILABLE")
	if boundaryCalls.Load() != int32(len(specs)+4) {
		t.Fatalf("authenticated watchlist write boundary calls = %d, want %d", boundaryCalls.Load(), len(specs)+4)
	}
	seenMu.Lock()
	for _, operation := range watchlistWriteRehearsalOperations {
		if seen[operation] == 0 {
			t.Errorf("Rust watchlist write operation %s was not exercised", operation)
		}
	}
	seenMu.Unlock()

	goFallback := watchlistWriteRehearsalRequest(t, goServer.URL+"/api/v1/watchlist/groups/missing", http.MethodPatch, `{"name":"Fallback","expectedRevision":1}`, "watchlist-write-go-rollback", goServer.URL)
	assertWatchlistWriteGoFallback(t, goFallback, "watchlist-write-go-rollback")
	closeGoOwner()
	closeProxy()

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after watchlist rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner)
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := watchlistWriteRehearsalRequest(t, restartedServer.URL+"/api/v1/watchlist/groups/missing", http.MethodPatch, `{"name":"Fallback","expectedRevision":1}`, "watchlist-write-go-restart", restartedServer.URL)
	assertWatchlistWriteGoFallback(t, restartedResponse, "watchlist-write-go-restart")
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after watchlist rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("watchlist write rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertWatchlistWritePrivateBoundary(t *testing.T, request *http.Request, token, origin string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
		t.Errorf("Rust private watchlist write boundary headers were not verified: %#v", request.Header)
	}
	for name, want := range map[string]string{
		"Cookie":       "jftrade_web_session=watchlist-write-browser",
		"Origin":       origin,
		"Referer":      origin + "/watchlist",
		"X-CSRF-Token": "watchlist-write-csrf",
	} {
		if got := request.Header.Get(name); got != want {
			t.Errorf("Rust watchlist write boundary %s = %q, want %q", name, got, want)
		}
	}
}

func watchlistWriteOperation(request *http.Request) string {
	path := request.URL.Path
	switch {
	case request.Method == http.MethodDelete && path == "/api/v1/watchlist/bindings":
		return "DELETE /api/v1/watchlist/bindings"
	case request.Method == http.MethodDelete && strings.HasPrefix(path, "/api/v1/watchlist/groups/"):
		return "DELETE /api/v1/watchlist/groups/{groupId}"
	case request.Method == http.MethodPatch && strings.HasPrefix(path, "/api/v1/watchlist/groups/"):
		return "PATCH /api/v1/watchlist/groups/{groupId}"
	case request.Method == http.MethodPost && path == "/api/v1/watchlist/groups":
		return "POST /api/v1/watchlist/groups"
	case request.Method == http.MethodPost && path == "/api/v1/watchlist/imports/preview":
		return "POST /api/v1/watchlist/imports/preview"
	case request.Method == http.MethodPost && strings.HasPrefix(path, "/api/v1/watchlist/imports/") && strings.HasSuffix(path, "/commit"):
		return "POST /api/v1/watchlist/imports/{previewId}/commit"
	case request.Method == http.MethodPost && path == "/api/v1/watchlist/quotes/batch":
		return "POST /api/v1/watchlist/quotes/batch"
	case request.Method == http.MethodPut && strings.HasPrefix(path, "/api/v1/watchlist/instruments/") && strings.HasSuffix(path, "/memberships"):
		return "PUT /api/v1/watchlist/instruments/{market}/{symbol}/memberships"
	default:
		return ""
	}
}

func containsWatchlistWriteOperation(operation string) bool {
	for _, candidate := range watchlistWriteRehearsalOperations {
		if candidate == operation {
			return true
		}
	}
	return false
}

func watchlistWriteExpectedBody(operation string) string {
	switch operation {
	case "DELETE /api/v1/watchlist/bindings", "DELETE /api/v1/watchlist/groups/{groupId}":
		return ""
	case "PATCH /api/v1/watchlist/groups/{groupId}":
		return `{"name":"Growth","expectedRevision":2}`
	case "POST /api/v1/watchlist/groups":
		return `{"name":"Growth"}`
	case "POST /api/v1/watchlist/imports/preview":
		return `{"sourceId":"source-1","remoteGroupId":"remote-1","localGroupId":"local-1","newGroupName":"Imported"}`
	case "POST /api/v1/watchlist/imports/{previewId}/commit":
		return `{"deleteInstrumentIds":["US:AAPL"]}`
	case "POST /api/v1/watchlist/quotes/batch":
		return `{"instrumentIds":["US:AAPL"]}`
	case "PUT /api/v1/watchlist/instruments/{market}/{symbol}/memberships":
		return `{"groupIds":["group-1"],"newGroupNames":[],"expectedRevision":2}`
	default:
		return "<unknown>"
	}
}

func watchlistWriteRehearsalRequest(t *testing.T, target, method, body, requestID, origin string) *http.Response {
	t.Helper()
	response, err := watchlistWriteRehearsalDo(context.Background(), target, method, body, requestID, origin)
	if err != nil {
		t.Fatalf("call watchlist write rehearsal route: %v", err)
	}
	return response
}

func watchlistWriteRehearsalDo(ctx context.Context, target, method, body, requestID, origin string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer public-desktop-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "jftrade_web_session=watchlist-write-browser")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", origin+"/watchlist")
	request.Header.Set("X-CSRF-Token", "watchlist-write-csrf")
	request.Header.Set("X-Request-ID", requestID)
	return http.DefaultClient.Do(request)
}

func assertWatchlistWriteStatus(t *testing.T, response *http.Response, wantStatus int, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read watchlist write rehearsal response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("watchlist write rehearsal request %s status = %d, want %d; body=%s", requestID, response.StatusCode, wantStatus, body)
	}
	if response.Header.Get("X-Request-ID") != requestID && wantStatus != http.StatusGatewayTimeout {
		t.Fatalf("watchlist write rehearsal request %s response ID = %q", requestID, response.Header.Get("X-Request-ID"))
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode watchlist write rehearsal response %s: %v; body=%s", requestID, err, body)
	}
	if envelope["ok"] != (wantStatus == http.StatusOK) {
		t.Fatalf("watchlist write rehearsal response %s envelope = %#v", requestID, envelope)
	}
}

func assertWatchlistWriteError(t *testing.T, response *http.Response, requestID, wantCode string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read watchlist write rehearsal error: %v", err)
	}
	if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(body), wantCode) {
		t.Fatalf("watchlist write rehearsal error %s = %d %s, want 502 containing %q", requestID, response.StatusCode, body, wantCode)
	}
}

func assertWatchlistWriteGoFallback(t *testing.T, response *http.Response, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go watchlist write fallback response: %v", err)
	}
	if response.StatusCode == http.StatusOK || strings.Contains(string(body), "RUST_") {
		t.Fatalf("Go watchlist write fallback response %s = %d %s", requestID, response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go watchlist write fallback response %s: %v; body=%s", requestID, err, body)
	}
	if envelope["ok"] == true {
		t.Fatalf("Go watchlist write fallback response %s unexpectedly succeeded: %#v", requestID, envelope)
	}
}
