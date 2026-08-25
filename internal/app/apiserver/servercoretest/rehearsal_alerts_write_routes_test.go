package servercoretest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

var alertsWriteRehearsalOperations = []string{
	"POST /api/v1/alerts/price",
	"POST /api/v1/alerts/option-events",
}

type alertsWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t alertsWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t alertsWriteRehearsalTarget) BearerToken() string { return t.token }

func (alertsWriteRehearsalTarget) Profile() string { return "mutation-test-cutover.v1" }

func (alertsWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), alertsWriteRehearsalOperations...)
}

func TestAlertsWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
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
		t.Fatalf("read settings before alerts rehearsal: %v", err)
	}

	goOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	goServer := httptest.NewServer(goOwner)
	t.Cleanup(func() {
		goServer.Close()
		jftradeCheckTestError(t, goOwner.Close())
	})

	token := strings.Repeat("a", 64)
	var boundaryCalls atomic.Int32
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertAlertsWritePrivateBoundary(t, request, token)
		operation := request.Method + " " + request.URL.Path
		if !containsAlertsWriteOperation(operation) {
			t.Errorf("unexpected Rust rehearsal operation %q", operation)
		}
		if request.ContentLength == 0 {
			t.Errorf("Rust rehearsal request %s lost its JSON body", operation)
		}
		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR","message":"fixture failure"}}`))
			return
		case "timeout":
			select {
			case <-request.Context().Done():
			case <-time.After(250 * time.Millisecond):
			}
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Request-ID", request.Header.Get("X-Request-ID"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"data":{"entries":[{"accepted":true}],"provider":{"brokerId":"futu"}},"timestamp":"2026-08-25T00:00:00Z"}`))
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       alertsWriteRehearsalTarget{endpoint: rust.URL, token: token},
		RehearsalOperations:   alertsWriteRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
	t.Cleanup(func() {
		proxyServer.Close()
		jftradeCheckTestError(t, proxyOwner.Close())
	})

	path := "/api/v1/alerts/price?brokerId=futu&accountId=acct-1"
	body := `{"symbol":"US.AAPL","price":100}`
	response := alertsWriteRehearsalRequest(t, proxyServer.URL+path, body, "alerts-write-success-1")
	assertAlertsWriteStatus(t, response, http.StatusOK, "alerts-write-success-1")
	response = alertsWriteRehearsalRequest(t, proxyServer.URL+path, body, "alerts-write-success-2")
	assertAlertsWriteStatus(t, response, http.StatusOK, "alerts-write-success-2")

	response = alertsWriteRehearsalRequest(
		t, proxyServer.URL+path+"&rehearsalFailure=error", body, "alerts-write-error",
	)
	assertAlertsWriteStatus(t, response, http.StatusUnprocessableEntity, "alerts-write-error")
	response = alertsWriteRehearsalRequest(
		t, proxyServer.URL+path+"&rehearsalFailure=timeout", body, "alerts-write-timeout",
	)
	assertAlertsWriteStatus(t, response, http.StatusGatewayTimeout, "alerts-write-timeout")

	rust.Close()
	for index, operation := range alertsWriteRehearsalOperations {
		method, route, ok := strings.Cut(operation, " ")
		if !ok {
			t.Fatalf("invalid alerts operation %q", operation)
		}
		fallbackPath := route + "?brokerId=futu"
		if method != http.MethodPost {
			t.Fatalf("alerts operation %q is not POST", operation)
		}
		response := alertsWriteRehearsalRequest(
			t, proxyServer.URL+fallbackPath, body, "alerts-write-crash-"+string(rune('a'+index)),
		)
		bodyBytes, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read crashed rehearsal response: %v", err)
		}
		if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(bodyBytes), "RUST_REHEARSAL_UNAVAILABLE") {
			t.Fatalf("crashed alerts rehearsal response for %s = %d %s", operation, response.StatusCode, bodyBytes)
		}
	}
	if boundaryCalls.Load() != 4 {
		t.Fatalf("authenticated alerts boundary calls = %d, want 4", boundaryCalls.Load())
	}

	goResponse := alertsWriteRehearsalRequest(t, goServer.URL+path, body, "alerts-write-go-rollback")
	assertAlertsWriteGoFallback(t, goResponse)
	goServer.Close()
	jftradeCheckTestError(t, goOwner.Close())

	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after alerts rehearsal: %v", err)
	}
	restartedOwner := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(restartedOwner)
	defer restartedServer.Close()
	defer func() { jftradeCheckTestError(t, restartedOwner.Close()) }()
	restartedResponse := alertsWriteRehearsalRequest(t, restartedServer.URL+path, body, "alerts-write-go-restart")
	assertAlertsWriteGoFallback(t, restartedResponse)
	settingsAfter, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after alerts rehearsal: %v", err)
	}
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatalf("alerts rehearsal modified settings: before=%q after=%q", settingsBefore, settingsAfter)
	}
}

func assertAlertsWritePrivateBoundary(t *testing.T, request *http.Request, token string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
		t.Errorf("Rust alerts mutation boundary headers were not verified: %#v", request.Header)
	}
	if got := request.Header.Get("Cookie"); got != "jftrade_web_session=browser-rehearsal" {
		t.Errorf("browser session cookie at Rust alerts boundary = %q", got)
	}
}

func containsAlertsWriteOperation(operation string) bool {
	for _, candidate := range alertsWriteRehearsalOperations {
		if candidate == operation {
			return true
		}
	}
	return false
}

func alertsWriteRehearsalRequest(t *testing.T, target, body, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create alerts rehearsal request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("Cookie", "jftrade_web_session=browser-rehearsal")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call alerts rehearsal route: %v", err)
	}
	return response
}

func assertAlertsWriteStatus(t *testing.T, response *http.Response, wantStatus int, requestID string) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read alerts rehearsal response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("alerts rehearsal request %s status = %d, want %d; body=%s", requestID, response.StatusCode, wantStatus, body)
	}
	if response.Header.Get("X-Request-ID") != requestID {
		t.Fatalf("alerts rehearsal request %s response ID = %q", requestID, response.Header.Get("X-Request-ID"))
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode alerts rehearsal response %s: %v; body=%s", requestID, err, body)
	}
	if envelope["ok"] != (wantStatus == http.StatusOK) {
		t.Fatalf("alerts rehearsal response %s envelope = %#v", requestID, envelope)
	}
}

func assertAlertsWriteGoFallback(t *testing.T, response *http.Response) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read Go alerts fallback response: %v", err)
	}
	if response.StatusCode == http.StatusOK || strings.Contains(string(body), "RUST_FIXTURE") {
		t.Fatalf("Go fallback unexpectedly used Rust alerts owner: status=%d body=%s", response.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Go alerts fallback response: %v; body=%s", err, body)
	}
	if _, ok := envelope["error"].(map[string]any); !ok {
		t.Fatalf("Go alerts fallback error envelope = %#v", envelope)
	}
}
