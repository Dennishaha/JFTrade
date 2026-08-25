package servercoretest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

var researchPresetWriteRehearsalOperations = []string{
	"POST /api/v1/research/screens/presets",
	"PATCH /api/v1/research/screens/presets/{presetId}",
	"DELETE /api/v1/research/screens/presets/{presetId}",
}

type researchPresetWriteRehearsalTarget struct {
	endpoint string
	token    string
}

func (t researchPresetWriteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t researchPresetWriteRehearsalTarget) BearerToken() string { return t.token }

func (researchPresetWriteRehearsalTarget) Profile() string { return "mutation-test-cutover.v1" }

func (researchPresetWriteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), researchPresetWriteRehearsalOperations...)
}

type researchPresetWriteEnvelope struct {
	Data struct {
		ID       string `json:"presetId"`
		Name     string `json:"name"`
		Revision int64  `json:"revision"`
	} `json:"data"`
}

func TestResearchPresetWriteRehearsalFencesOwnersAndRecoversAcrossRestart(t *testing.T) {
	root := t.TempDir()
	fallbackSettings := filepath.Join(root, "go-fallback", "settings.json")
	rehearsalSettings := filepath.Join(root, "rust-rehearsal", "settings.json")
	fallbackStore := openResearchPresetWriteSettings(t, fallbackSettings)
	rehearsalStore := openResearchPresetWriteSettings(t, rehearsalSettings)

	rehearsalOwner := servercore.NewSidecarHandlerWithOptions(rehearsalStore, servercore.SidecarOptions{DesktopMode: true})
	rehearsalOwnerServer := httptest.NewServer(rehearsalOwner)
	rehearsalOwnerClosed := false
	closeRehearsalOwner := func() {
		if rehearsalOwnerClosed {
			return
		}
		rehearsalOwnerClosed = true
		rehearsalOwnerServer.Close()
		jftradeCheckTestError(t, rehearsalOwner.Close())
	}
	t.Cleanup(closeRehearsalOwner)

	token := strings.Repeat("m", 64)
	var boundaryCalls atomic.Int32
	var referenceMutationCalls atomic.Int32
	rustBoundary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		boundaryCalls.Add(1)
		assertResearchPresetWritePrivateBoundary(t, request, token)
		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR"}}`))
			return
		case "timeout":
			select {
			case <-request.Context().Done():
			case <-time.After(250 * time.Millisecond):
			}
			return
		}
		referenceMutationCalls.Add(1)
		forwardResearchPresetWriteReference(t, w, request, rehearsalOwnerServer.URL)
	}))
	rustBoundaryClosed := false
	closeRustBoundary := func() {
		if rustBoundaryClosed {
			return
		}
		rustBoundaryClosed = true
		rustBoundary.Close()
	}
	t.Cleanup(closeRustBoundary)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(fallbackStore, servercore.SidecarOptions{
		DesktopMode: true,
		RehearsalTarget: researchPresetWriteRehearsalTarget{
			endpoint: rustBoundary.URL,
			token:    token,
		},
		RehearsalOperations:   researchPresetWriteRehearsalOperations,
		RehearsalProxyTimeout: 100 * time.Millisecond,
	})
	proxyServer := httptest.NewServer(proxyOwner)
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

	created := researchPresetWriteMutation(t, proxyServer.URL, http.MethodPost,
		"/api/v1/research/screens/presets", researchPresetWriteCreateBody("Rehearsal owner"), http.StatusOK)
	if created.Data.ID == "" || created.Data.Revision != 1 {
		t.Fatalf("created preset = %#v", created.Data)
	}
	_ = researchPresetWriteMutation(t, proxyServer.URL, http.MethodPost,
		"/api/v1/research/screens/presets", researchPresetWriteCreateBody("Rehearsal owner"), http.StatusConflict)
	updated := researchPresetWriteMutation(t, proxyServer.URL, http.MethodPatch,
		"/api/v1/research/screens/presets/"+created.Data.ID,
		fmt.Sprintf(`{"name":"Recovered owner","expectedRevision":%d}`, created.Data.Revision), http.StatusOK)
	if updated.Data.ID != created.Data.ID || updated.Data.Name != "Recovered owner" || updated.Data.Revision != 2 {
		t.Fatalf("updated preset = %#v", updated.Data)
	}
	deleted := researchPresetWriteMutation(t, proxyServer.URL, http.MethodPost,
		"/api/v1/research/screens/presets", researchPresetWriteCreateBody("Delete fence"), http.StatusOK)
	_ = researchPresetWriteMutation(t, proxyServer.URL, http.MethodDelete,
		"/api/v1/research/screens/presets/"+deleted.Data.ID, "ignored invalid body", http.StatusOK)

	assertResearchPresetWriteFailuresDoNotReplay(t, proxyServer.URL, created.Data.ID)
	if referenceMutationCalls.Load() != 5 {
		t.Fatalf("reference mutation calls = %d, want 5", referenceMutationCalls.Load())
	}
	assertResearchPresetWriteList(t, proxyServer.URL, nil)

	closeRustBoundary()
	for index, operation := range researchPresetWriteRehearsalOperations {
		method, path, body := researchPresetWriteOperationRequest(operation, created.Data.ID)
		response := researchPresetWriteRequest(t, proxyServer.URL+path, method, body,
			fmt.Sprintf("research-preset-write-crash-%d", index+1))
		responseBody, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadGateway || !strings.Contains(string(responseBody), "RUST_REHEARSAL_UNAVAILABLE") {
			t.Fatalf("crashed rehearsal response for %s = %d %s", operation, response.StatusCode, responseBody)
		}
	}
	if referenceMutationCalls.Load() != 5 {
		t.Fatalf("crashed Rust requests reached reference owner: calls=%d", referenceMutationCalls.Load())
	}
	assertResearchPresetWriteList(t, proxyServer.URL, nil)

	closeProxy()
	closeRehearsalOwner()
	assertResearchPresetWriteRestartState(t, rehearsalSettings, []string{"Recovered owner"})
	assertResearchPresetWriteRestartState(t, fallbackSettings, nil)
	if boundaryCalls.Load() != 11 {
		t.Fatalf("authenticated boundary calls = %d, want 11", boundaryCalls.Load())
	}
}

func openResearchPresetWriteSettings(t *testing.T, settingsPath string) *servercore.SettingsStore {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatalf("create settings directory: %v", err)
	}
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
	return store
}

func assertResearchPresetWritePrivateBoundary(t *testing.T, request *http.Request, token string) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+token ||
		request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
		request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
		t.Errorf("Rust mutation boundary headers were not verified: %#v", request.Header)
	}
	if got := request.Header.Get("Cookie"); got != "jftrade_web_session=browser-rehearsal" {
		t.Errorf("browser session cookie at Rust mutation boundary = %q", got)
	}
}

func forwardResearchPresetWriteReference(t *testing.T, w http.ResponseWriter, request *http.Request, endpoint string) {
	t.Helper()
	forwarded, err := http.NewRequestWithContext(
		request.Context(), request.Method, endpoint+request.URL.RequestURI(), request.Body,
	)
	if err != nil {
		t.Errorf("create research preset reference request: %v", err)
		return
	}
	forwarded.ContentLength = request.ContentLength
	forwarded.Header.Set("Content-Type", request.Header.Get("Content-Type"))
	forwarded.Header.Set("X-Request-ID", request.Header.Get("X-Request-ID"))
	response, err := http.DefaultClient.Do(forwarded)
	if err != nil {
		t.Errorf("call research preset reference owner: %v", err)
		return
	}
	defer func() { _ = response.Body.Close() }()
	for _, name := range readRouteRehearsalHeaders {
		for _, value := range response.Header.Values(name) {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func researchPresetWriteCreateBody(name string) string {
	return fmt.Sprintf(`{"name":%q,"definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}}]}}`, name)
}

func researchPresetWriteMutation(
	t *testing.T,
	endpoint string,
	method string,
	path string,
	body string,
	wantStatus int,
) researchPresetWriteEnvelope {
	t.Helper()
	response := researchPresetWriteRequest(t, endpoint+path, method, body, "research-preset-write-wire")
	responseBody, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read research preset write response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, response.StatusCode, wantStatus, responseBody)
	}
	if response.Header.Get("X-Request-ID") != "research-preset-write-wire" {
		t.Fatalf("%s %s request ID = %q", method, path, response.Header.Get("X-Request-ID"))
	}
	var envelope researchPresetWriteEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		t.Fatalf("decode research preset write response: %v; body=%s", err, responseBody)
	}
	return envelope
}

func assertResearchPresetWriteFailuresDoNotReplay(t *testing.T, endpoint, presetID string) {
	t.Helper()
	for index, operation := range researchPresetWriteRehearsalOperations {
		method, path, body := researchPresetWriteOperationRequest(operation, presetID)
		for _, failure := range []struct {
			name       string
			wantStatus int
		}{
			{name: "error", wantStatus: http.StatusUnprocessableEntity},
			{name: "timeout", wantStatus: http.StatusGatewayTimeout},
		} {
			separator := "?"
			if strings.Contains(path, "?") {
				separator = "&"
			}
			response := researchPresetWriteRequest(t,
				endpoint+path+separator+"rehearsalFailure="+failure.name, method, body,
				fmt.Sprintf("research-preset-write-%s-%d", failure.name, index+1),
			)
			responseBody, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if response.StatusCode != failure.wantStatus {
				t.Fatalf("%s response for %s = %d %s", failure.name, operation, response.StatusCode, responseBody)
			}
		}
	}
}

func researchPresetWriteOperationRequest(operation, presetID string) (string, string, string) {
	switch operation {
	case "POST /api/v1/research/screens/presets":
		return http.MethodPost, "/api/v1/research/screens/presets", researchPresetWriteCreateBody("must not persist")
	case "PATCH /api/v1/research/screens/presets/{presetId}":
		return http.MethodPatch, "/api/v1/research/screens/presets/" + presetID, `{"name":"must not persist","expectedRevision":2}`
	case "DELETE /api/v1/research/screens/presets/{presetId}":
		return http.MethodDelete, "/api/v1/research/screens/presets/" + presetID, ""
	default:
		panic("unknown research preset write operation: " + operation)
	}
}

func researchPresetWriteRequest(t *testing.T, target, method, body, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, target, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("create research preset write request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("Cookie", "jftrade_web_session=browser-rehearsal")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call research preset write route: %v", err)
	}
	return response
}

func assertResearchPresetWriteList(t *testing.T, endpoint string, wantNames []string) {
	t.Helper()
	response := researchPresetWriteRequest(t, endpoint+"/api/v1/research/screens/presets", http.MethodGet, "", "research-preset-write-state")
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("read research preset state: status=%d err=%v body=%s", response.StatusCode, err, body)
	}
	var envelope struct {
		Data struct {
			Presets []struct {
				Name string `json:"name"`
			} `json:"presets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode research preset state: %v; body=%s", err, body)
	}
	got := make([]string, 0, len(envelope.Data.Presets))
	for _, preset := range envelope.Data.Presets {
		got = append(got, preset.Name)
	}
	if strings.Join(got, "\x00") != strings.Join(wantNames, "\x00") {
		t.Fatalf("research preset names = %#v, want %#v", got, wantNames)
	}
}

func assertResearchPresetWriteRestartState(t *testing.T, settingsPath string, wantNames []string) {
	t.Helper()
	store, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after mutation rehearsal: %v", err)
	}
	owner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{DesktopMode: true})
	server := httptest.NewServer(owner)
	defer server.Close()
	defer func() { jftradeCheckTestError(t, owner.Close()) }()
	assertResearchPresetWriteList(t, server.URL, wantNames)
	if _, err := os.Stat(apiruntime.DeriveResearchDBPath(settingsPath)); err != nil {
		t.Fatalf("research preset database after restart: %v", err)
	}
}
