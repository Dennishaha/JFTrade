package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type lifecycleTestStore struct {
	integration       jfsettings.BrokerIntegration
	savedIntegration  *jfsettings.BrokerIntegration
	interfaceSettings jfsettings.InterfaceSettings
	securitySettings  jfsettings.SecuritySettings
	bootstrapCalls    int
	bootstrapErr      error
}

func (s *lifecycleTestStore) EnsureBootstrapFile(jfsettings.LaunchDefaults) error {
	s.bootstrapCalls++
	return s.bootstrapErr
}
func (s *lifecycleTestStore) Integration() jfsettings.BrokerIntegration { return s.integration }
func (s *lifecycleTestStore) SavedIntegration() *jfsettings.BrokerIntegration {
	return s.savedIntegration
}
func (s *lifecycleTestStore) InterfaceSettings(jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	return s.interfaceSettings
}
func (s *lifecycleTestStore) SecuritySettings() jfsettings.SecuritySettings {
	return s.securitySettings
}

type lifecycleTestHandler struct {
	mu              sync.Mutex
	apiPort         int
	authOrigins     []string
	frontendBaseURL string
	frontendSet     bool
	securityApplied jfsettings.SecuritySettings
	webReconfigure  func(jfsettings.SecuritySettings) error
	closeCalls      int
	closeErr        error
}

func (h *lifecycleTestHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (h *lifecycleTestHandler) WebAccessHandler() http.Handler               { return h }
func (h *lifecycleTestHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closeCalls++
	return h.closeErr
}
func (h *lifecycleTestHandler) SetAPIPort(port int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apiPort = port
}
func (h *lifecycleTestHandler) ConfigureAuthOrigins(origins ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.authOrigins = append(h.authOrigins, origins...)
}
func (h *lifecycleTestHandler) SetFrontendFS(_ fs.FS, baseURL string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frontendSet = true
	h.frontendBaseURL = baseURL
}
func (h *lifecycleTestHandler) ApplySecuritySettings(settings jfsettings.SecuritySettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.securityApplied = settings
}
func (h *lifecycleTestHandler) SetWebAccessReconfigure(reconfigure func(jfsettings.SecuritySettings) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.webReconfigure = reconfigure
}

func TestStartForRunArgsReturnsNoopWhenDisabled(t *testing.T) {
	called := false
	shutdown, err := StartForRunArgs(t.Context(), []string{"gui"}, Dependencies{
		ShouldStartForArgs: func([]string) bool { return false },
		LoadFrontendFS: func() fs.FS {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}
	if called {
		t.Fatal("dependencies should not be evaluated when startup is skipped")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown noop: %v", err)
	}
}

func TestStartForRunArgsConfiguresRuntimeAndFrontend(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	store := &lifecycleTestStore{
		integration: jfsettings.BrokerIntegration{BrokerID: "futu", Enabled: true},
		interfaceSettings: jfsettings.InterfaceSettings{
			APIBind: "127.0.0.1:0",
			GUIBind: "127.0.0.1:0",
		},
		securitySettings: jfsettings.SecuritySettings{
			WebAccessEnabled:   true,
			PasswordConfigured: true,
			PasswordHash:       "test-verifier",
		},
	}
	handler := &lifecycleTestHandler{}
	var appliedIntegration jfsettings.BrokerIntegration

	shutdown, err := StartForRunArgs(ctx, []string{"api"}, Dependencies{
		ShouldStartForArgs: func([]string) bool { return true },
		LoadFrontendFS:     func() fs.FS { return fstest.MapFS{"index.html": {Data: []byte("ok")}} },
		ResolveLaunchDefaults: func(bool) jfsettings.LaunchDefaults {
			return jfsettings.LaunchDefaults{APIBind: "127.0.0.1:3000", GUIBind: "127.0.0.1:3003", SettingsPath: "settings.json", BacktestDBPath: "backtest.db"}
		},
		EnvOrDefault:        func(_ string, value string) string { return value },
		EnsureRuntimeLayout: func(string, string) error { return nil },
		NewSettingsStore:    func(string) (SettingsStore, error) { return store, nil },
		ResolveIntegrationRuntime: func(input jfsettings.BrokerIntegration) jfsettings.BrokerIntegration {
			input.Config.Host = "runtime-host"
			return input
		},
		ApplyIntegrationRuntime: func(input jfsettings.BrokerIntegration) { appliedIntegration = input },
		NewHandler:              func(SettingsStore) (Handler, error) { return handler, nil },
		APIBaseURLForBind:       func(bind string) string { return "http://" + bind },
		PortFromBind: func(bind string, fallback int) int {
			if bind == "127.0.0.1:0" {
				return fallback
			}
			if bind == "127.0.0.1:3000" {
				return 3000
			}
			return fallback
		},
	})
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}

	handler.mu.Lock()
	gotPort := handler.apiPort
	gotFrontend := handler.frontendSet
	gotFrontendBase := handler.frontendBaseURL
	gotSecurity := handler.securityApplied
	gotOrigins := append([]string(nil), handler.authOrigins...)
	handler.mu.Unlock()

	if gotPort != 6688 {
		t.Fatalf("SetAPIPort = %d, want integrated HTTP port fallback 6688", gotPort)
	}
	if !gotFrontend || gotFrontendBase != "" {
		t.Fatalf("frontend config = set:%v base:%q", gotFrontend, gotFrontendBase)
	}
	if !gotSecurity.WebAccessEnabled || !gotSecurity.PasswordConfigured {
		t.Fatalf("security settings not applied: %#v", gotSecurity)
	}
	if len(gotOrigins) != 1 || gotOrigins[0] != "http://127.0.0.1:0" {
		t.Fatalf("auth origins = %#v", gotOrigins)
	}
	if appliedIntegration.Config.Host != "runtime-host" {
		t.Fatalf("integration runtime = %#v", appliedIntegration)
	}
	if store.bootstrapCalls != 1 {
		t.Fatalf("bootstrap calls = %d, want 1", store.bootstrapCalls)
	}

	cancel()
	waitForClose(t, handler, 1)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	waitForClose(t, handler, 1)
}

func TestWebAccessBindDefaultsToLoopbackAndRequiresCompletePublicConfiguration(t *testing.T) {
	configured := "192.0.2.10:6688"
	for _, settings := range []jfsettings.SecuritySettings{
		{},
		{WebAccessEnabled: true, PublicAccessEnabled: true},
		{WebAccessEnabled: true, PasswordConfigured: true},
	} {
		if got := webAccessBind(configured, settings); got != "127.0.0.1:6688" {
			t.Fatalf("webAccessBind(%#v) = %q, want loopback", settings, got)
		}
	}
	public := jfsettings.SecuritySettings{
		WebAccessEnabled:    true,
		PublicAccessEnabled: true,
		PasswordConfigured:  true,
	}
	if got := webAccessBind(configured, public); got != "0.0.0.0:6688" {
		t.Fatalf("public webAccessBind = %q", got)
	}
	for _, invalid := range []string{"", "not-a-bind", "127.0.0.1:"} {
		if got := webAccessBind(invalid, public); got != invalid {
			t.Fatalf("webAccessBind(%q) = %q, want unchanged", invalid, got)
		}
	}
}

func TestWebAccessListenerBindUsesIndependentConfiguredPort(t *testing.T) {
	if got := webAccessListenerBind(jfsettings.SecuritySettings{}); got != "" {
		t.Fatalf("disabled Web listener bind = %q, want empty", got)
	}
	local := jfsettings.SecuritySettings{
		WebAccessEnabled:   true,
		PasswordConfigured: true,
		WebPort:            7443,
	}
	if got := webAccessListenerBind(local); got != "127.0.0.1:7443" {
		t.Fatalf("local Web listener bind = %q", got)
	}
	local.PublicAccessEnabled = true
	if got := webAccessListenerBind(local); got != "0.0.0.0:7443" {
		t.Fatalf("public Web listener bind = %q", got)
	}
	if got := loopbackBind("0.0.0.0:6699"); got != "127.0.0.1:6699" {
		t.Fatalf("desktop sidecar bind = %q", got)
	}
}

func TestSeparateWebListenerStartsAlongsideLoopbackDesktopSidecar(t *testing.T) {
	webPort := availableTCPPort(t)
	store := &lifecycleTestStore{
		interfaceSettings: jfsettings.InterfaceSettings{APIBind: "0.0.0.0:0"},
		securitySettings: jfsettings.SecuritySettings{
			WebAccessEnabled:   true,
			PasswordConfigured: true,
			PasswordHash:       "test-verifier",
			WebPort:            webPort,
		},
	}
	handler := &lifecycleTestHandler{}
	deps := lifecycleDependencies(store, handler, nil)
	deps.SeparateWebListener = true

	shutdown, err := StartForRunArgs(t.Context(), []string{"api"}, deps)
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", webPort))
	if err != nil {
		t.Fatalf("GET separate Web listener: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Web listener status = %d", response.StatusCode)
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestSeparateWebListenerRebindsImmediatelyAndKeepsOldPortOnConflict(t *testing.T) {
	firstPort := availableTCPPort(t)
	store := &lifecycleTestStore{
		interfaceSettings: jfsettings.InterfaceSettings{APIBind: "127.0.0.1:0"},
		securitySettings: jfsettings.SecuritySettings{
			WebAccessEnabled:   true,
			PasswordConfigured: true,
			PasswordHash:       "test-verifier",
			WebPort:            firstPort,
		},
	}
	handler := &lifecycleTestHandler{}
	deps := lifecycleDependencies(store, handler, nil)
	deps.SeparateWebListener = true
	shutdown, err := StartForRunArgs(t.Context(), []string{"api"}, deps)
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}
	defer func() { _ = shutdown(t.Context()) }()

	handler.mu.Lock()
	reconfigure := handler.webReconfigure
	handler.mu.Unlock()
	if reconfigure == nil {
		t.Fatal("Web access reconfigure callback was not installed")
	}

	secondPort := availableTCPPort(t)
	updated := store.securitySettings
	updated.WebPort = secondPort
	if err := reconfigure(updated); err != nil {
		t.Fatalf("reconfigure Web port: %v", err)
	}
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", secondPort))
	if err != nil {
		t.Fatalf("GET rebound Web listener: %v", err)
	}
	_ = response.Body.Close()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy conflict port: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	conflict := updated
	conflict.WebPort = occupied.Addr().(*net.TCPAddr).Port
	if err := reconfigure(conflict); err == nil || !strings.Contains(err.Error(), "Web access port conflict") {
		t.Fatalf("conflicting reconfigure error = %v", err)
	}
	response, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/", secondPort))
	if err != nil {
		t.Fatalf("previous Web listener did not survive conflict: %v", err)
	}
	_ = response.Body.Close()
}

func TestWebAccessServerManagerCoversLiveReconfigurationLifecycle(t *testing.T) {
	port := availableTCPPort(t)
	handler := &lifecycleTestHandler{}
	manager := newWebAccessServerManager(lifecycleDependencies(&lifecycleTestStore{}, handler, nil), handler)
	settings := jfsettings.SecuritySettings{
		WebAccessEnabled:   true,
		PasswordConfigured: true,
		PasswordHash:       "test-verifier",
		WebPort:            port,
	}
	if err := manager.Reconfigure(settings); err != nil {
		t.Fatalf("initial Reconfigure: %v", err)
	}
	if err := manager.Reconfigure(settings); err != nil {
		t.Fatalf("same-bind Reconfigure: %v", err)
	}
	settings.PublicAccessEnabled = true
	if err := manager.Reconfigure(settings); err != nil {
		t.Fatalf("same-port host Reconfigure: %v", err)
	}
	settings.WebAccessEnabled = false
	settings.PublicAccessEnabled = false
	if err := manager.Reconfigure(settings); err != nil {
		t.Fatalf("disable Reconfigure: %v", err)
	}
	if manager.server != nil || manager.bind != "" {
		t.Fatalf("disabled manager = server:%v bind:%q", manager.server, manager.bind)
	}
	if err := manager.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown disabled manager: %v", err)
	}

	var nilManager *webAccessServerManager
	if err := nilManager.Reconfigure(settings); err != nil {
		t.Fatalf("nil Reconfigure: %v", err)
	}
	if err := nilManager.Shutdown(t.Context()); err != nil {
		t.Fatalf("nil Shutdown: %v", err)
	}
	if got := bindPort("invalid"); got != "" {
		t.Fatalf("bindPort invalid = %q", got)
	}
	if got := loopbackBind("invalid"); got != "invalid" {
		t.Fatalf("loopbackBind invalid = %q", got)
	}
}

func TestWebAccessServerManagerRestoresOldBindAfterHostSwitchFailure(t *testing.T) {
	port := availableTCPPort(t)
	handler := &lifecycleTestHandler{}
	manager := newWebAccessServerManager(lifecycleDependencies(&lifecycleTestStore{}, handler, nil), handler)
	settings := jfsettings.SecuritySettings{
		WebAccessEnabled:   true,
		PasswordConfigured: true,
		PasswordHash:       "test-verifier",
		WebPort:            port,
	}
	if err := manager.Reconfigure(settings); err != nil {
		t.Fatalf("initial Reconfigure: %v", err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(t.Context()) })

	wantErr := errors.New("injected public bind failure")
	listenCalls := 0
	manager.listen = func(network string, address string) (net.Listener, error) {
		listenCalls++
		if listenCalls == 1 {
			return nil, wantErr
		}
		return net.Listen(network, address)
	}
	public := settings
	public.PublicAccessEnabled = true
	if err := manager.Reconfigure(public); !errors.Is(err, wantErr) {
		t.Fatalf("host switch error = %v, want %v", err, wantErr)
	}
	if manager.bind != fmt.Sprintf("127.0.0.1:%d", port) || manager.server == nil {
		t.Fatalf("old bind was not restored: bind=%q server=%v", manager.bind, manager.server)
	}

	restoreErr := errors.New("injected restore failure")
	manager.listen = func(string, string) (net.Listener, error) { return nil, restoreErr }
	if err := manager.Reconfigure(public); err == nil || !strings.Contains(err.Error(), "restoring") {
		t.Fatalf("combined restore error = %v", err)
	}
	if manager.server != nil || manager.bind != "" {
		t.Fatalf("failed restore left manager active: bind=%q server=%v", manager.bind, manager.server)
	}
	if err := manager.restoreLocked(""); err != nil {
		t.Fatalf("restore empty bind: %v", err)
	}
}

func TestStartForRunArgsStartsAPIOnlyAndShutsDown(t *testing.T) {
	store := &lifecycleTestStore{
		interfaceSettings: jfsettings.InterfaceSettings{APIBind: "127.0.0.1:0"},
	}
	handler := &lifecycleTestHandler{}
	deps := lifecycleDependencies(store, handler, nil)
	completeCalls := 0
	deps.CompleteDatabaseRebuild = func(string, string) error {
		completeCalls++
		return nil
	}

	shutdown, err := StartForRunArgs(t.Context(), []string{"api"}, deps)
	if err != nil {
		t.Fatalf("StartForRunArgs API-only: %v", err)
	}
	if completeCalls != 1 {
		t.Fatalf("CompleteDatabaseRebuild calls = %d, want 1", completeCalls)
	}
	handler.mu.Lock()
	gotPort := handler.apiPort
	gotOrigins := append([]string(nil), handler.authOrigins...)
	handler.mu.Unlock()
	if gotPort != 3000 {
		t.Fatalf("API port = %d, want fallback 3000", gotPort)
	}
	if len(gotOrigins) != 1 || gotOrigins[0] != "http://127.0.0.1:0" {
		t.Fatalf("auth origins = %#v", gotOrigins)
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown API-only: %v", err)
	}
	waitForClose(t, handler, 1)
}

func TestStartForRunArgsClosesHandlerWhenDatabaseRebuildFinalizeFails(t *testing.T) {
	wantErr := errors.New("rebuild failed")
	store := &lifecycleTestStore{
		interfaceSettings: jfsettings.InterfaceSettings{APIBind: "127.0.0.1:0"},
	}
	handler := &lifecycleTestHandler{}

	_, err := StartForRunArgs(t.Context(), []string{"api"}, Dependencies{
		ShouldStartForArgs: func([]string) bool { return true },
		LoadFrontendFS:     func() fs.FS { return nil },
		ResolveLaunchDefaults: func(bool) jfsettings.LaunchDefaults {
			return jfsettings.LaunchDefaults{APIBind: "127.0.0.1:3000", SettingsPath: "settings.json", BacktestDBPath: "backtest.db"}
		},
		EnvOrDefault:            func(_ string, value string) string { return value },
		EnsureRuntimeLayout:     func(string, string) error { return nil },
		NewSettingsStore:        func(string) (SettingsStore, error) { return store, nil },
		NewHandler:              func(SettingsStore) (Handler, error) { return handler, nil },
		CompleteDatabaseRebuild: func(string, string) error { return wantErr },
		APIBaseURLForBind:       func(bind string) string { return "http://" + bind },
		PortFromBind:            func(string, int) int { return 3000 },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("StartForRunArgs error = %v, want %v", err, wantErr)
	}
	waitForClose(t, handler, 1)
}

func TestStartForRunArgsReportsAPIPortConflictAndClosesHandler(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve API port: %v", err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Errorf("close reserved API listener: %v", closeErr)
		}
	}()

	store := &lifecycleTestStore{
		interfaceSettings: jfsettings.InterfaceSettings{APIBind: listener.Addr().String()},
	}
	handler := &lifecycleTestHandler{}
	_, err = StartForRunArgs(t.Context(), []string{"api"}, lifecycleDependencies(store, handler, nil))
	if err == nil || !strings.Contains(err.Error(), "JFTrade API port conflict") {
		t.Fatalf("StartForRunArgs error = %v, want API port conflict", err)
	}
	waitForClose(t, handler, 1)
}

func TestStartForRunArgsWithEmbeddedFrontendDoesNotBindAPIPort(t *testing.T) {
	apiListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve API port: %v", err)
	}
	defer func() {
		if closeErr := apiListener.Close(); closeErr != nil {
			t.Errorf("close reserved API listener: %v", closeErr)
		}
	}()

	store := &lifecycleTestStore{
		interfaceSettings: jfsettings.InterfaceSettings{
			APIBind: apiListener.Addr().String(),
			GUIBind: "127.0.0.1:0",
		},
	}
	handler := &lifecycleTestHandler{}
	shutdown, err := StartForRunArgs(
		t.Context(),
		[]string{"api"},
		lifecycleDependencies(store, handler, fstest.MapFS{"index.html": {Data: []byte("ok")}}),
	)
	if err != nil {
		t.Fatalf("StartForRunArgs with occupied API port: %v", err)
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown integrated HTTP server: %v", err)
	}
	waitForClose(t, handler, 1)
}

func TestStartForRunArgsReportsIntegratedHTTPPortConflictAndClosesHandler(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve GUI port: %v", err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Errorf("close reserved GUI listener: %v", closeErr)
		}
	}()

	store := &lifecycleTestStore{
		interfaceSettings: jfsettings.InterfaceSettings{
			APIBind: "127.0.0.1:0",
			GUIBind: listener.Addr().String(),
		},
	}
	handler := &lifecycleTestHandler{}
	_, err = StartForRunArgs(
		t.Context(),
		[]string{"api"},
		lifecycleDependencies(store, handler, fstest.MapFS{"index.html": {Data: []byte("ok")}}),
	)
	if err == nil || !strings.Contains(err.Error(), "JFTrade integrated HTTP port conflict") {
		t.Fatalf("StartForRunArgs error = %v, want integrated HTTP port conflict", err)
	}
	waitForClose(t, handler, 1)
}

func TestStartForRunArgsStopsAtFailingStartupStage(t *testing.T) {
	wantErr := errors.New("startup failed")
	defaults := jfsettings.LaunchDefaults{
		APIBind:        "127.0.0.1:0",
		SettingsPath:   "settings.json",
		BacktestDBPath: "backtest.db",
	}

	tests := []struct {
		name  string
		build func(*lifecycleTestStore) Dependencies
	}{
		{
			name: "runtime layout",
			build: func(*lifecycleTestStore) Dependencies {
				return Dependencies{EnsureRuntimeLayout: func(string, string) error { return wantErr }}
			},
		},
		{
			name: "database rebuild",
			build: func(*lifecycleTestStore) Dependencies {
				return Dependencies{
					EnsureRuntimeLayout:  func(string, string) error { return nil },
					ApplyDatabaseRebuild: func(string, string) error { return wantErr },
				}
			},
		},
		{
			name: "settings store",
			build: func(*lifecycleTestStore) Dependencies {
				return Dependencies{
					EnsureRuntimeLayout: func(string, string) error { return nil },
					NewSettingsStore:    func(string) (SettingsStore, error) { return nil, wantErr },
				}
			},
		},
		{
			name: "bootstrap file",
			build: func(store *lifecycleTestStore) Dependencies {
				store.bootstrapErr = wantErr
				return Dependencies{
					EnsureRuntimeLayout: func(string, string) error { return nil },
					NewSettingsStore:    func(string) (SettingsStore, error) { return store, nil },
				}
			},
		},
		{
			name: "handler construction",
			build: func(store *lifecycleTestStore) Dependencies {
				return Dependencies{
					EnsureRuntimeLayout: func(string, string) error { return nil },
					NewSettingsStore:    func(string) (SettingsStore, error) { return store, nil },
					NewHandler:          func(SettingsStore) (Handler, error) { return nil, wantErr },
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &lifecycleTestStore{interfaceSettings: jfsettings.InterfaceSettings{APIBind: "127.0.0.1:0"}}
			deps := test.build(store)
			deps.ShouldStartForArgs = func([]string) bool { return true }
			deps.LoadFrontendFS = func() fs.FS { return nil }
			deps.ResolveLaunchDefaults = func(bool) jfsettings.LaunchDefaults { return defaults }
			deps.EnvOrDefault = func(_ string, value string) string { return value }

			shutdown, err := StartForRunArgs(t.Context(), []string{"api"}, deps)
			if shutdown != nil || !errors.Is(err, wantErr) {
				t.Fatalf("shutdown nil=%t err=%v, want nil and %v", shutdown == nil, err, wantErr)
			}
		})
	}
}

func TestRunAPIOnlyReturnsStartupErrorAndWaitsForCancellation(t *testing.T) {
	wantErr := errors.New("layout unavailable")
	errorDeps := Dependencies{
		ShouldStartForArgs:    func([]string) bool { return true },
		LoadFrontendFS:        func() fs.FS { return nil },
		ResolveLaunchDefaults: func(bool) jfsettings.LaunchDefaults { return jfsettings.LaunchDefaults{} },
		EnvOrDefault:          func(_ string, value string) string { return value },
		EnsureRuntimeLayout:   func(string, string) error { return wantErr },
	}
	if err := RunAPIOnly(t.Context(), errorDeps); !errors.Is(err, wantErr) {
		t.Fatalf("RunAPIOnly error = %v, want %v", err, wantErr)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := RunAPIOnly(ctx, Dependencies{ShouldStartForArgs: func([]string) bool { return false }}); err != nil {
		t.Fatalf("RunAPIOnly canceled noop: %v", err)
	}
}

func TestOnceShutdownReturnsStableHandlerError(t *testing.T) {
	wantErr := errors.New("handler close failed")
	handler := &lifecycleTestHandler{closeErr: wantErr}
	shutdown := onceShutdown(nil, nil, handler)

	if err := shutdown(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("first shutdown error = %v, want %v", err, wantErr)
	}
	if err := shutdown(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("second shutdown error = %v, want stable %v", err, wantErr)
	}
	waitForClose(t, handler, 1)
}

func TestBestEffortLoggingIgnoresNonErrors(t *testing.T) {
	besteffort.LogError(errors.New("expected close error"))
}

func lifecycleDependencies(store SettingsStore, handler Handler, frontendFS fs.FS) Dependencies {
	return Dependencies{
		ShouldStartForArgs: func([]string) bool { return true },
		LoadFrontendFS:     func() fs.FS { return frontendFS },
		ResolveLaunchDefaults: func(bool) jfsettings.LaunchDefaults {
			return jfsettings.LaunchDefaults{
				APIBind:        "127.0.0.1:3000",
				GUIBind:        "127.0.0.1:3003",
				SettingsPath:   "settings.json",
				BacktestDBPath: "backtest.db",
			}
		},
		EnvOrDefault:        func(_ string, value string) string { return value },
		EnsureRuntimeLayout: func(string, string) error { return nil },
		NewSettingsStore:    func(string) (SettingsStore, error) { return store, nil },
		NewHandler:          func(SettingsStore) (Handler, error) { return handler, nil },
		APIBaseURLForBind:   func(bind string) string { return "http://" + bind },
		PortFromBind:        func(string, int) int { return 3000 },
	}
}

func waitForClose(t *testing.T, handler *lifecycleTestHandler, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		handler.mu.Lock()
		got := handler.closeCalls
		handler.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	handler.mu.Lock()
	got := handler.closeCalls
	handler.mu.Unlock()
	t.Fatalf("handler closeCalls = %d, want %d", got, want)
}

func availableTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate TCP port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release TCP port: %v", err)
	}
	return port
}
