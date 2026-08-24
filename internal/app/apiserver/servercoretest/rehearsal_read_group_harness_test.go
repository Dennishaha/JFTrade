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

	"github.com/jftrade/jftrade-main/internal/app/apiserver/rustrehearsal"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

var readRouteRehearsalHeaders = []string{
	"Cache-Control",
	"Content-Language",
	"Content-Type",
	"ETag",
	"Expires",
	"Last-Modified",
	"Retry-After",
	"Vary",
	"X-Content-Type-Options",
	"X-Request-ID",
}

type readRouteRehearsalSpec struct {
	prefix          string
	operations      []string
	paths           []string
	operationPaths  map[string]string
	dynamicJSONKeys map[string]struct{}
	prepareStore    func(*testing.T, *servercore.SettingsStore)
}

type readRouteRehearsalTarget struct {
	endpoint    string
	operations  []string
	bearerToken string
}

func (t readRouteRehearsalTarget) Endpoint() string { return t.endpoint }

func (t readRouteRehearsalTarget) BearerToken() string { return t.bearerToken }

func (readRouteRehearsalTarget) Profile() string { return rustrehearsal.ReadOnlyProfile }

func (t readRouteRehearsalTarget) Capabilities() []string {
	return append([]string(nil), t.operations...)
}

type readRouteWireSnapshot struct {
	status  int
	body    string
	headers map[string][]string
}

func runReadRouteRehearsal(t *testing.T, spec readRouteRehearsalSpec) {
	t.Helper()
	validateReadRouteRehearsalSpec(t, spec)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	store, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("open settings store: %v", err)
	}
	spec.prepareStore(t, store)

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

	token := strings.Repeat("h", 64)
	var snapshotsMu sync.Mutex
	snapshots := map[string]readRouteWireSnapshot{}
	ownerCalls := 0
	rust := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token ||
			request.Header.Get(rustrehearsal.InternalProxyHeader) != rustrehearsal.InternalProxyProtocol ||
			request.Header.Get(rustrehearsal.AccessSurfaceHeader) != "desktop" {
			t.Errorf("Rust private boundary headers were not verified: %#v", request.Header)
		}
		if request.Method != http.MethodGet || request.ContentLength > 0 {
			t.Errorf("%s rehearsal forwarded a non-read request: method=%s contentLength=%d", spec.prefix, request.Method, request.ContentLength)
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
			t.Errorf("call Go owner for %s wire comparison: %v", spec.prefix, err)
			return
		}
		defer func() { _ = ownerResponse.Body.Close() }()
		body, err := io.ReadAll(ownerResponse.Body)
		if err != nil {
			t.Errorf("read Go owner response: %v", err)
			return
		}
		snapshot := readRouteWireSnapshot{
			status:  ownerResponse.StatusCode,
			body:    string(body),
			headers: readRouteHeaderSnapshot(ownerResponse.Header),
		}
		snapshotsMu.Lock()
		snapshots[request.URL.RequestURI()] = snapshot
		ownerCalls++
		snapshotsMu.Unlock()
		copyReadRouteHeaders(w.Header(), snapshot.headers)
		w.WriteHeader(snapshot.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rust.Close)

	proxyOwner := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode: true,
		RehearsalTarget: readRouteRehearsalTarget{
			endpoint: rust.URL, operations: spec.operations, bearerToken: token,
		},
		RehearsalOperations: spec.operations, RehearsalProxyTimeout: 500 * time.Millisecond,
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

	for index, path := range spec.paths {
		response := readRouteRehearsalRequest(t, proxyServer.URL+path, fmt.Sprintf("%s-wire-%d", spec.prefix, index+1))
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read %s rehearsal response: %v", spec.prefix, err)
		}
		snapshotsMu.Lock()
		want, ok := snapshots[path]
		snapshotsMu.Unlock()
		if !ok {
			t.Fatalf("missing Go wire snapshot for %s", path)
		}
		assertReadRouteWire(t, path, response, body, want, nil)
	}

	for index, operation := range spec.operations {
		path := spec.operationPaths[operation]
		errorResponse := readRouteRehearsalRequest(
			t, proxyServer.URL+addReadRouteFailure(path, "error"), fmt.Sprintf("%s-error-%d", spec.prefix, index+1),
		)
		errorBody, _ := io.ReadAll(errorResponse.Body)
		_ = errorResponse.Body.Close()
		if errorResponse.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(errorBody), "RUST_FIXTURE_ERROR") {
			t.Fatalf("Rust error response for %s was not preserved: %d %s", path, errorResponse.StatusCode, errorBody)
		}
		timeoutResponse := readRouteRehearsalRequest(
			t, proxyServer.URL+addReadRouteFailure(path, "timeout"), fmt.Sprintf("%s-timeout-%d", spec.prefix, index+1),
		)
		_ = timeoutResponse.Body.Close()
		if timeoutResponse.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("Rust timeout status for %s = %d", path, timeoutResponse.StatusCode)
		}
	}

	rust.Close()
	for index, operation := range spec.operations {
		path := spec.operationPaths[operation]
		response := readRouteRehearsalRequest(t, proxyServer.URL+path, fmt.Sprintf("%s-crash-%d", spec.prefix, index+1))
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadGateway {
			t.Fatalf("Rust crash status for %s = %d", path, response.StatusCode)
		}
	}
	snapshotsMu.Lock()
	if ownerCalls != len(spec.paths) {
		t.Fatalf("failed Rust requests replayed Go: owner calls = %d, want %d", ownerCalls, len(spec.paths))
	}
	snapshotsMu.Unlock()

	closeProxy()
	closeGoOwner()
	restartedStore, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("reopen settings store after rehearsal: %v", err)
	}
	goAfterRestart := servercore.NewSidecarHandlerWithOptions(restartedStore, servercore.SidecarOptions{DesktopMode: true})
	restartedServer := httptest.NewServer(goAfterRestart)
	t.Cleanup(func() {
		restartedServer.Close()
		jftradeCheckTestError(t, goAfterRestart.Close())
	})
	for index, path := range spec.paths {
		response := readRouteRehearsalRequest(t, restartedServer.URL+path, fmt.Sprintf("%s-wire-%d", spec.prefix, index+1))
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		snapshotsMu.Lock()
		want := snapshots[path]
		snapshotsMu.Unlock()
		assertReadRouteWire(t, path, response, body, want, spec.dynamicJSONKeys)
	}
}

func validateReadRouteRehearsalSpec(t *testing.T, spec readRouteRehearsalSpec) {
	t.Helper()
	if spec.prefix == "" || len(spec.operations) == 0 || len(spec.paths) == 0 || spec.prepareStore == nil {
		t.Fatal("incomplete read-route rehearsal specification")
	}
	for _, operation := range spec.operations {
		if spec.operationPaths[operation] == "" {
			t.Fatalf("missing concrete path for operation %s", operation)
		}
	}
}

func readRouteHeaderSnapshot(header http.Header) map[string][]string {
	snapshot := make(map[string][]string, len(readRouteRehearsalHeaders))
	for _, name := range readRouteRehearsalHeaders {
		snapshot[name] = append([]string(nil), header.Values(name)...)
	}
	return snapshot
}

func copyReadRouteHeaders(destination http.Header, source map[string][]string) {
	for name, values := range source {
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func assertReadRouteWire(t *testing.T, path string, response *http.Response, body []byte, want readRouteWireSnapshot, dynamicKeys map[string]struct{}) {
	t.Helper()
	gotBody, wantBody := string(body), want.body
	if dynamicKeys != nil {
		gotBody = normalizeReadRouteEnvelope(body, dynamicKeys)
		wantBody = normalizeReadRouteEnvelope([]byte(want.body), dynamicKeys)
	}
	if response.StatusCode != want.status || gotBody != wantBody {
		t.Fatalf("wire mismatch for %s: status/body = %d %q, want %d %q", path, response.StatusCode, body, want.status, want.body)
	}
	got := readRouteHeaderSnapshot(response.Header)
	for _, name := range readRouteRehearsalHeaders {
		if strings.Join(got[name], "\x00") != strings.Join(want.headers[name], "\x00") {
			t.Fatalf("wire header mismatch for %s header %s: %#v, want %#v", path, name, got[name], want.headers[name])
		}
	}
}

func normalizeReadRouteEnvelope(body []byte, dynamicKeys map[string]struct{}) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return string(body)
	}
	normalizeReadRouteValue(value, dynamicKeys)
	normalized, err := json.Marshal(value)
	if err != nil {
		return string(body)
	}
	return string(normalized)
}

func normalizeReadRouteValue(value any, dynamicKeys map[string]struct{}) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if _, ok := dynamicKeys[key]; ok && child != nil {
				current[key] = "fixture-time"
				continue
			}
			normalizeReadRouteValue(child, dynamicKeys)
		}
	case []any:
		for _, child := range current {
			normalizeReadRouteValue(child, dynamicKeys)
		}
	}
}

func readRouteRehearsalRequest(t *testing.T, target, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("create read-route rehearsal request: %v", err)
	}
	request.Header.Set("X-Request-ID", requestID)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("call read-route rehearsal: %v", err)
	}
	return response
}

func addReadRouteFailure(path, failure string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "rehearsalFailure=" + failure
}

func prepareDisabledFutuReadRehearsal(t *testing.T, store *servercore.SettingsStore) {
	t.Helper()
	isolateTestBacktestDatabase(t, store)
	disableTestExchangeCalendarAutoRefresh(t, store)
	forceTestMarketDataProvider(t, store)
	_, err := store.SaveIntegration(jfsettings.BrokerIntegration{
		BrokerID: "futu",
		Enabled:  false,
		Config: settingsfile.NormalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Type: "futu", Host: "127.0.0.1", APIPort: 1, WebSocketPort: 2,
		}),
	})
	if err != nil {
		t.Fatalf("save disabled Futu integration: %v", err)
	}
}
