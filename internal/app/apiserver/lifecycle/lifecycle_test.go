package lifecycle

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

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
	closeCalls      int
	closeErr        error
}

func (h *lifecycleTestHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
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
		securitySettings: jfsettings.SecuritySettings{AdminAuthRequired: true},
	}
	handler := &lifecycleTestHandler{}
	var appliedIntegration jfsettings.BrokerIntegration

	shutdown, err := StartForRunArgs(ctx, []string{"api"}, Dependencies{
		ShouldStartForArgs: func([]string) bool { return true },
		LoadFrontendFS:     func() fs.FS { return fstest.MapFS{"index.html": {Data: []byte("ok")}} },
		ResolveLaunchDefaults: func(bool) jfsettings.LaunchDefaults {
			return jfsettings.LaunchDefaults{APIBind: "127.0.0.1:3000", GUIBind: "127.0.0.1:5173", SettingsPath: "settings.json", BacktestDBPath: "backtest.db"}
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
		ResolveGUIRuntimeAPIBase: func(jfsettings.InterfaceSettings, string) string { return "http://runtime-api" },
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

	if gotPort != 3000 {
		t.Fatalf("SetAPIPort = %d, want 3000", gotPort)
	}
	if !gotFrontend || gotFrontendBase != "http://runtime-api" {
		t.Fatalf("frontend config = set:%v base:%q", gotFrontend, gotFrontendBase)
	}
	if !gotSecurity.AdminAuthRequired {
		t.Fatalf("security settings not applied: %#v", gotSecurity)
	}
	if len(gotOrigins) != 2 || gotOrigins[0] != "http://127.0.0.1:0" || gotOrigins[1] != "http://127.0.0.1:0" {
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
	defer listener.Close()

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

func TestStartForRunArgsReportsGUIPortConflictAndClosesHandler(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve GUI port: %v", err)
	}
	defer listener.Close()

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
	if err == nil || !strings.Contains(err.Error(), "JFTrade GUI port conflict") {
		t.Fatalf("StartForRunArgs error = %v, want GUI port conflict", err)
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
	shutdown := onceShutdown(nil, handler)

	if err := shutdown(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("first shutdown error = %v, want %v", err, wantErr)
	}
	if err := shutdown(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("second shutdown error = %v, want stable %v", err, wantErr)
	}
	waitForClose(t, handler, 1)
}

func TestBestEffortLoggingIgnoresNonErrors(t *testing.T) {
	jftradeLogError("ignored", nil, errors.New("expected close error"))
}

func lifecycleDependencies(store SettingsStore, handler Handler, frontendFS fs.FS) Dependencies {
	return Dependencies{
		ShouldStartForArgs: func([]string) bool { return true },
		LoadFrontendFS:     func() fs.FS { return frontendFS },
		ResolveLaunchDefaults: func(bool) jfsettings.LaunchDefaults {
			return jfsettings.LaunchDefaults{
				APIBind:        "127.0.0.1:3000",
				GUIBind:        "127.0.0.1:5173",
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
		ResolveGUIRuntimeAPIBase: func(jfsettings.InterfaceSettings, string) string {
			return "http://runtime-api"
		},
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
