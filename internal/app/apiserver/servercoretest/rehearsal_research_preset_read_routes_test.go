package servercoretest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	researchstore "github.com/jftrade/jftrade-main/internal/store/research"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

var researchPresetReadRouteOperations = []string{
	"GET /api/v1/research/screens/presets",
	"GET /api/v1/research/screens/presets/{presetId}",
}

var researchPresetReadWireHeaders = []string{
	"Cache-Control",
	"Content-Language",
	"Content-Type",
	"ETag",
	"Expires",
	"Last-Modified",
	"Vary",
	"X-Content-Type-Options",
	"X-Request-ID",
}

type researchPresetReadRehearsalTarget struct {
	endpoint string
}

func (t researchPresetReadRehearsalTarget) Endpoint() string { return t.endpoint }

func (researchPresetReadRehearsalTarget) BearerToken() string {
	return strings.Repeat("p", 64)
}

func (researchPresetReadRehearsalTarget) Profile() string {
	return rustrehearsal.ReadOnlyProfile
}

func (researchPresetReadRehearsalTarget) Capabilities() []string {
	return append([]string(nil), researchPresetReadRouteOperations...)
}

type researchPresetReadWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func TestResearchPresetReadRehearsalPreservesWireAndRequiresRestartForGoRollback(t *testing.T) {
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
	presetID := seedResearchPresetReadOwner(t, settingsPath)

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
	beforeRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after owner startup: %v", err)
	}

	var snapshotsMu sync.Mutex
	snapshots := map[string]researchPresetReadWireSnapshot{}
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("p", 64) ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("research preset rehearsal forwarded a non-read request: method=%s contentLength=%d", request.Method, request.ContentLength)
		}
		switch request.URL.Query().Get("rehearsalFailure") {
		case "error":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"RUST_FIXTURE_ERROR"}}`))
			return
		case "timeout":
			<-request.Context().Done()
			return
		}

		ownerRequest, err := http.NewRequestWithContext(
			request.Context(), request.Method, goServer.URL+request.URL.RequestURI(), nil,
		)
		if err != nil {
			t.Errorf("create Go owner comparison request: %v", err)
			return
		}
		ownerRequest.Header.Set("X-Request-ID", request.Header.Get("X-Request-ID"))
		ownerResponse, err := http.DefaultClient.Do(ownerRequest)
		if err != nil {
			t.Errorf("call Go owner for research preset wire comparison: %v", err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := researchPresetReadWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: researchPresetReadHeaderSnapshot(ownerResponse.Header),
		}
		snapshotsMu.Lock()
		snapshots[request.URL.RequestURI()] = snapshot
		snapshotsMu.Unlock()
		for name, values := range snapshot.headers {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(snapshot.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:           true,
		RehearsalTarget:       researchPresetReadRehearsalTarget{endpoint: rust.URL},
		RehearsalOperations:   researchPresetReadRouteOperations,
		RehearsalProxyTimeout: 500 * time.Millisecond,
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

	paths := []string{
		"/api/v1/research/screens/presets",
		"/api/v1/research/screens/presets/" + presetID,
		"/api/v1/research/screens/presets/missing",
	}
	for index, path := range paths {
		requestID := fmt.Sprintf("research-preset-read-wire-%d", index+1)
		response := researchPresetReadRehearsalRequest(t, proxyServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read research preset rehearsal response: %v", err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertResearchPresetReadWire(t, path, response, body, want)
	}

	for index, operation := range researchPresetReadRouteOperations {
		path := researchPresetReadOperationPath(operation, presetID)
		errorResponse := researchPresetReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=error",
			fmt.Sprintf("research-preset-read-error-%d", index+1),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := researchPresetReadRehearsalRequest(
			t, proxyServer.URL+path+"?rehearsalFailure=timeout",
			fmt.Sprintf("research-preset-read-timeout-%d", index+1),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range researchPresetReadRouteOperations {
		path := researchPresetReadOperationPath(operation, presetID)
		crashResponse := researchPresetReadRehearsalRequest(
			t, proxyServer.URL+path, fmt.Sprintf("research-preset-read-crash-%d", index+1),
		)
		_ = crashResponse.Body.Close()
		if crashResponse.StatusCode != http.StatusBadGateway {
			t.Fatalf("Rust crash status for %s = %d", path, crashResponse.StatusCode)
		}
	}

	closeProxy()
	closeGoOwner()
	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after rehearsal: %v", err)
	}
	goAfterRestart := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(goAfterRestart)
	defer restartedServer.Close()
	t.Cleanup(func() { jftradeCheckTestError(t, goAfterRestart.Close()) })
	for index, path := range paths {
		requestID := fmt.Sprintf("research-preset-read-wire-%d", index+1)
		response := researchPresetReadRehearsalRequest(t, restartedServer.URL+path, requestID)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read Go rollback response: %v", err)
		}
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		assertResearchPresetReadRollbackWire(t, path, response, body, want)
	}

	afterRequest, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after rehearsal: %v", err)
	}
	if string(afterRequest) != string(beforeRequest) {
		t.Fatalf("research preset read rehearsal modified settings: before=%q after=%q", beforeRequest, afterRequest)
	}
}

func seedResearchPresetReadOwner(t *testing.T, settingsPath string) string {
	t.Helper()
	resource, err := researchstore.Open(context.Background(), apiruntime.DeriveResearchDBPath(settingsPath))
	if err != nil {
		t.Fatalf("open research preset store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resource.Close()) }()
	definition := broker.ScreenDefinitionV2{
		BrokerID: "futu",
		Market:   "US",
		Pool:     broker.ResearchScreenPool{},
		Columns: []broker.ScreenColumn{{
			ID:     "price",
			Factor: broker.FactorRef{InstanceID: "price", FactorKey: "simple.price"},
		}},
		CatalogVersion:     researchscreen.CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
	}
	preset, err := resource.CreateScreenPreset(
		context.Background(), "Stage 9 research preset", definition, broker.ScreenQuerySchemaVersionV2,
	)
	if err != nil {
		t.Fatalf("create research preset fixture: %v", err)
	}
	return preset.ID
}

func researchPresetReadHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(researchPresetReadWireHeaders))
	for _, name := range researchPresetReadWireHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func assertResearchPresetReadWire(
	t *testing.T,
	path string,
	response *http.Response,
	body []byte,
	want researchPresetReadWireSnapshot,
) {
	t.Helper()
	if response.StatusCode != want.status || string(body) != want.body {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := researchPresetReadHeaderSnapshot(response.Header)
	for _, name := range researchPresetReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func assertResearchPresetReadRollbackWire(
	t *testing.T,
	path string,
	response *http.Response,
	body []byte,
	want researchPresetReadWireSnapshot,
) {
	t.Helper()
	if response.StatusCode != want.status ||
		normalizeResearchPresetReadEnvelope(body) != normalizeResearchPresetReadEnvelope([]byte(want.body)) {
		t.Fatalf("rollback wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := researchPresetReadHeaderSnapshot(response.Header)
	for _, name := range researchPresetReadWireHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("rollback wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func normalizeResearchPresetReadEnvelope(body []byte) string {
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return string(body)
	}
	delete(envelope, "timestamp")
	normalized, err := json.Marshal(envelope)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func researchPresetReadRehearsalRequest(t *testing.T, target, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create research preset read request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call research preset read route: %v", err)
	}
	return response
}

func researchPresetReadOperationPath(operation, presetID string) string {
	switch operation {
	case "GET /api/v1/research/screens/presets":
		return "/api/v1/research/screens/presets"
	case "GET /api/v1/research/screens/presets/{presetId}":
		return "/api/v1/research/screens/presets/" + presetID
	default:
		panic("unknown research preset read operation: " + operation)
	}
}
