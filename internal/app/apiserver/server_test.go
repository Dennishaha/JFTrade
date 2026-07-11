package apiserver

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestStartForRunArgsInitializesRuntimeLayout(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "var", "jftrade-api")
	settingsPath := filepath.Join(runtimeDir, "settings.json")
	backtestDBPath := filepath.Join(runtimeDir, "backtest.db")
	t.Setenv("JFTRADE_API_DISABLED", "")
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", "127.0.0.1:0")
	t.Setenv("JFTRADE_GUI_BIND", "127.0.0.1:0")
	t.Setenv("FUTU_OPEND_ADDR", "before-startup")

	store, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("settingsfile.New: %v", err)
	}
	if _, err := store.SaveIntegration(jfsettings.BrokerIntegration{
		Config: jfsettings.FutuIntegrationConfig{
			Host:    "127.0.0.4",
			APIPort: 24444,
		},
	}); err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if got := os.Getenv("FUTU_OPEND_ADDR"); got != "before-startup" {
		t.Fatalf("pure store changed FUTU_OPEND_ADDR to %q", got)
	}

	ctx := t.Context()

	shutdown, err := StartForRunArgs(ctx, []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs() error = %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown() error = %v", err)
		}
	}()

	if _, err := os.Stat(runtimeDir); err != nil {
		t.Fatalf("runtime dir not initialized: %v", err)
	}
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file not initialized: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(backtestDBPath)); err != nil {
		t.Fatalf("backtest db dir not initialized: %v", err)
	}
	if got := os.Getenv("FUTU_OPEND_ADDR"); got != "127.0.0.4:24444" {
		t.Fatalf("startup FUTU_OPEND_ADDR = %q", got)
	}
}

func TestStartForRunArgsDisabledReturnsNoop(t *testing.T) {
	t.Setenv("JFTRADE_API_DISABLED", "true")

	shutdown, err := StartForRunArgs(context.Background(), []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs() error = %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
}

func TestStartDesktopDoesNotPersistentlyDisableAdminAuth(t *testing.T) {
	apiBind := freeTCPAddr(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	backtestDBPath := filepath.Join(t.TempDir(), "backtest.db")
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", apiBind)

	store, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("settingsfile.New: %v", err)
	}
	if _, err := store.SaveSecuritySettings(jfsettings.SecuritySettings{AdminAuthRequired: true}); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}

	shutdown, err := StartDesktop(t.Context(), nil)
	if err != nil {
		t.Fatalf("StartDesktop: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	}()

	resp, err := http.Get("http://" + apiBind + "/api/v1/settings/security")
	if err != nil {
		t.Fatalf("GET desktop security settings: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("desktop security settings status = %d, want 200 without login", resp.StatusCode)
	}

	if got := store.SecuritySettings(); !got.AdminAuthRequired {
		t.Fatalf("in-memory desktop security settings mutated to %#v, want persisted admin auth required", got)
	}
	reloaded, err := settingsfile.New(settingsPath)
	if err != nil {
		t.Fatalf("settingsfile.New reload: %v", err)
	}
	if got := reloaded.SecuritySettings(); !got.AdminAuthRequired {
		t.Fatalf("reloaded desktop security settings = %#v, want persisted admin auth required", got)
	}
}

func TestResolveDesktopRuntimeConfigUsesProfileBindInsteadOfPersistedInterfaceBind(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settingsBind := freeTCPAddr(t)
	if err := os.WriteFile(settingsPath, []byte(`{"interfaces":{"apiBind":"`+settingsBind+`"}}`), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_API_BIND", "")
	defaults := jfsettings.LaunchDefaults{
		APIBind:        "127.0.0.1:6698",
		SettingsPath:   settingsPath,
		BacktestDBPath: filepath.Join(t.TempDir(), "backtest.db"),
	}

	config, err := ResolveDesktopRuntimeConfigWithDefaults(defaults, false)
	if err != nil {
		t.Fatalf("ResolveDesktopRuntimeConfigWithDefaults: %v", err)
	}
	if config.APIBind != defaults.APIBind {
		t.Fatalf("APIBind = %q, want profile bind %q instead of persisted %q", config.APIBind, defaults.APIBind, settingsBind)
	}
	if config.APIBaseURL != "http://"+defaults.APIBind {
		t.Fatalf("APIBaseURL = %q, want http://%s", config.APIBaseURL, defaults.APIBind)
	}
}

func TestResolveDesktopRuntimeConfigRejectsEphemeralAPIBind(t *testing.T) {
	t.Setenv("JFTRADE_API_BIND", "127.0.0.1:0")

	if _, err := ResolveDesktopRuntimeConfig(); err == nil || !strings.Contains(err.Error(), "stable local port") {
		t.Fatalf("ResolveDesktopRuntimeConfig error = %v, want stable port error", err)
	}
}

func TestStartDesktopWithConfigKeepsResolvedProfileBind(t *testing.T) {
	profileBind := freeTCPAddr(t)
	persistedBind := freeTCPAddr(t)
	runtimeDir := t.TempDir()
	settingsPath := filepath.Join(runtimeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"interfaces":{"apiBind":"`+persistedBind+`"}}`), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	runtimeConfig := DesktopRuntimeConfig{
		Defaults: jfsettings.LaunchDefaults{
			APIBind:        profileBind,
			SettingsPath:   settingsPath,
			BacktestDBPath: filepath.Join(runtimeDir, "backtest.db"),
		},
		SettingsPath: settingsPath,
		BacktestPath: filepath.Join(runtimeDir, "backtest.db"),
		APIBind:      profileBind,
		APIBaseURL:   "http://" + profileBind,
	}

	shutdown, err := StartDesktopWithConfig(t.Context(), runtimeConfig, nil)
	if err != nil {
		t.Fatalf("StartDesktopWithConfig: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	}()

	response, err := http.Get(runtimeConfig.APIBaseURL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET resolved profile bind: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("resolved profile status = %d", response.StatusCode)
	}
}

func TestResolvePackagedDesktopRuntimeRequiresLoopback(t *testing.T) {
	defaults := jfsettings.LaunchDefaults{
		APIBind:        "127.0.0.1:6699",
		SettingsPath:   filepath.Join(t.TempDir(), "settings.json"),
		BacktestDBPath: filepath.Join(t.TempDir(), "backtest.db"),
	}

	for _, bind := range []string{"0.0.0.0:6699", "192.168.1.20:6699", "[::]:6699"} {
		t.Run(bind, func(t *testing.T) {
			t.Setenv("JFTRADE_API_BIND", bind)
			if _, err := ResolveDesktopRuntimeConfigWithDefaults(defaults, true); err == nil || !strings.Contains(err.Error(), "loopback") {
				t.Fatalf("ResolveDesktopRuntimeConfigWithDefaults(%q) error = %v, want loopback rejection", bind, err)
			}
		})
	}

	for _, bind := range []string{"127.0.0.1:6699", "localhost:6699", "[::1]:6699"} {
		t.Run(bind, func(t *testing.T) {
			t.Setenv("JFTRADE_API_BIND", bind)
			if _, err := ResolveDesktopRuntimeConfigWithDefaults(defaults, true); err != nil {
				t.Fatalf("ResolveDesktopRuntimeConfigWithDefaults(%q): %v", bind, err)
			}
		})
	}
}

func TestStartDesktopStartsAPIButNotLegacyGUIServer(t *testing.T) {
	apiBind := freeTCPAddr(t)
	guiBind := freeTCPAddr(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	backtestDBPath := filepath.Join(t.TempDir(), "backtest.db")
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", apiBind)
	t.Setenv("JFTRADE_GUI_BIND", guiBind)

	shutdown, err := StartDesktop(t.Context(), nil)
	if err != nil {
		t.Fatalf("StartDesktop: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	}()

	waitForHTTPStatus(t, "http://"+apiBind+"/api/v1/system/status", http.StatusOK)

	client := &http.Client{Timeout: 200 * time.Millisecond}
	resp, err := client.Get("http://" + guiBind + "/")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		t.Fatalf("legacy GUI server unexpectedly listening on %s with status %d", guiBind, resp.StatusCode)
	}
}

func TestStartDesktopAllowsWailsDevOrigin(t *testing.T) {
	apiBind := freeTCPAddr(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	backtestDBPath := filepath.Join(t.TempDir(), "backtest.db")
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", apiBind)
	t.Setenv("FRONTEND_DEVSERVER_URL", "http://127.0.0.1:5173")

	shutdown, err := StartDesktop(t.Context(), nil)
	if err != nil {
		t.Fatalf("StartDesktop: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	}()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	assertWailsCORS(t, client, http.MethodGet, "http://"+apiBind+"/api/v1/settings/ui", "wails://localhost:5173")
	assertWailsCORS(t, client, http.MethodOptions, "http://"+apiBind+"/api/v1/settings/ui", "wails://localhost:5173")
}

func TestStartDesktopAllowsPackagedWailsOrigins(t *testing.T) {
	apiBind := freeTCPAddr(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	backtestDBPath := filepath.Join(t.TempDir(), "backtest.db")
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", apiBind)

	shutdown, err := StartDesktop(t.Context(), nil)
	if err != nil {
		t.Fatalf("StartDesktop: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	}()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	for _, origin := range []string{
		"wails://localhost",
		"http://wails.localhost",
	} {
		assertWailsCORS(t, client, http.MethodGet, "http://"+apiBind+"/api/v1/settings/ui", origin)
		assertWailsCORS(t, client, http.MethodOptions, "http://"+apiBind+"/api/v1/settings/ui", origin)
	}
}

func TestStartDesktopReportsAPIBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	previousTimeout := desktopAPIReadyTimeout
	desktopAPIReadyTimeout = 300 * time.Millisecond
	t.Cleanup(func() { desktopAPIReadyTimeout = previousTimeout })

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	backtestDBPath := filepath.Join(t.TempDir(), "backtest.db")
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", listener.Addr().String())

	shutdown, err := StartDesktop(t.Context(), nil)
	if err == nil {
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		t.Fatal("StartDesktop succeeded with occupied API bind")
	}
	if shutdown != nil {
		t.Fatal("StartDesktop returned shutdown on startup failure")
	}
	if !strings.Contains(err.Error(), "API port conflict") || !strings.Contains(err.Error(), listener.Addr().String()) {
		t.Fatalf("StartDesktop error = %q, want explicit occupied port", err)
	}
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}
	return addr
}

func waitForHTTPStatus(t *testing.T, url string, wantStatus int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == wantStatus {
				return
			}
			lastErr = nil
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("GET %s: %v", url, lastErr)
	}
	t.Fatalf("GET %s did not return status %d before deadline", url, wantStatus)
}

func assertWailsCORS(t *testing.T, client *http.Client, method string, url string, origin string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	if err != nil {
		t.Fatalf("NewRequest %s: %v", method, err)
	}
	req.Header.Set("Origin", origin)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("%s origin %q Access-Control-Allow-Origin = %q", method, origin, got)
	}
	if method == http.MethodOptions && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", resp.StatusCode)
	}
}

func TestDependenciesApplyScheduledDatabaseRebuildBeforeStartup(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	backtestPath := filepath.Join(root, "backtest.db")
	for _, path := range []string{backtestPath, backtestPath + "-wal", backtestPath + "-shm"} {
		if err := os.WriteFile(path, []byte("legacy"), 0o600); err != nil {
			t.Fatalf("write legacy database file: %v", err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(root, "database-rebuild.json"),
		[]byte(`{"databaseIds":["backtest"],"createdAt":"2026-06-21T00:00:00Z"}`),
		0o600,
	); err != nil {
		t.Fatalf("write rebuild marker: %v", err)
	}

	deps := dependencies()
	if deps.ApplyDatabaseRebuild == nil || deps.CompleteDatabaseRebuild == nil {
		t.Fatal("database rebuild lifecycle callbacks are not wired")
	}
	if err := deps.ApplyDatabaseRebuild(settingsPath, backtestPath); err != nil {
		t.Fatalf("ApplyDatabaseRebuild: %v", err)
	}
	for _, path := range []string{backtestPath, backtestPath + "-wal", backtestPath + "-shm"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("scheduled database file still exists: %s (%v)", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "database-rebuild.json")); err != nil {
		t.Fatalf("rebuild marker should remain until schema initialization completes: %v", err)
	}
}

func TestRunAPIOnlyReturnsAfterContextCancellation(t *testing.T) {
	t.Setenv("JFTRADE_API_DISABLED", "true")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := RunAPIOnly(ctx); err != nil {
		t.Fatalf("RunAPIOnly() error = %v", err)
	}
}
