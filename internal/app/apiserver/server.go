package apiserver

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/lifecycle"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	"github.com/jftrade/jftrade-main/internal/frontendassets"
	"github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

var desktopAPIReadyTimeout = 5 * time.Second

// RunAPIOnly starts the sidecar in API-only mode and waits for ctx shutdown.
func RunAPIOnly(ctx context.Context) error {
	return lifecycle.RunAPIOnly(ctx, dependencies())
}

// StartForRunArgs starts the API sidecar for supported API command args.
func StartForRunArgs(ctx context.Context, args []string) (func(context.Context) error, error) {
	return lifecycle.StartForRunArgs(ctx, args, dependencies())
}

type DesktopRuntimeConfig struct {
	Defaults     jfsettings.LaunchDefaults
	SettingsPath string
	BacktestPath string
	APIBind      string
	APIBaseURL   string
	APIToken     string
}

// ResolveDesktopRuntimeConfig resolves the desktop API/runtime paths once so
// the Wails asset host and embedded API sidecar agree on the same local API.
func ResolveDesktopRuntimeConfig() (DesktopRuntimeConfig, error) {
	return ResolveDesktopRuntimeConfigWithDefaults(apiruntime.ResolveLaunchDefaults(true), false)
}

// ResolveDesktopRuntimeConfigWithDefaults resolves a desktop runtime from a
// build-profile-specific set of defaults. Packaged builds may require the
// sidecar to remain loopback-only while development builds retain explicit
// bind overrides.
func ResolveDesktopRuntimeConfigWithDefaults(defaults jfsettings.LaunchDefaults, requireLoopback bool) (DesktopRuntimeConfig, error) {
	config := DesktopRuntimeConfig{
		Defaults:     defaults,
		SettingsPath: envOrDefault("JFTRADE_SETTINGS_PATH", defaults.SettingsPath),
		BacktestPath: envOrDefault("JFTRADE_BACKTEST_DB", defaults.BacktestDBPath),
		APIBind:      envOrDefault("JFTRADE_API_BIND", defaults.APIBind),
	}
	config.APIBaseURL = apiruntime.APIBaseURLForBind(config.APIBind)
	if err := validateDesktopAPIBind(config.APIBind, config.APIBaseURL, requireLoopback); err != nil {
		return DesktopRuntimeConfig{}, err
	}
	config.Defaults.SettingsPath = config.SettingsPath
	config.Defaults.BacktestDBPath = config.BacktestPath
	config.Defaults.APIBind = config.APIBind
	return config, nil
}

func validateDesktopAPIBind(apiBind string, apiBaseURL string, requireLoopback bool) error {
	if strings.TrimSpace(apiBaseURL) == "" {
		return fmt.Errorf("desktop API bind %q does not produce a browser-accessible API URL", apiBind)
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(apiBind))
	if err != nil {
		return fmt.Errorf("desktop API bind %q is invalid: %w", apiBind, err)
	}
	if strings.TrimSpace(port) == "0" {
		return fmt.Errorf("desktop API bind %q is not supported; configure a stable local port", apiBind)
	}
	if requireLoopback && !isLoopbackDesktopHost(host) {
		return fmt.Errorf("packaged desktop API bind %q must use a loopback host", apiBind)
	}
	return nil
}

func isLoopbackDesktopHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// StartDesktop starts the API sidecar for an embedded Wails desktop host.
func StartDesktop(ctx context.Context, sink func(live.Event) live.NotificationDelivery) (func(context.Context) error, error) {
	runtimeConfig, err := ResolveDesktopRuntimeConfig()
	if err != nil {
		return nil, err
	}
	return StartDesktopWithConfig(ctx, runtimeConfig, sink)
}

// StartDesktopWithConfig starts the desktop sidecar using a runtime config
// resolved once by the desktop bootstrap.
func StartDesktopWithConfig(ctx context.Context, runtimeConfig DesktopRuntimeConfig, sink func(live.Event) live.NotificationDelivery) (func(context.Context) error, error) {
	deps := dependencies()
	deps.SeparateWebListener = true
	deps.LoadFrontendFS = func() fs.FS { return nil }
	deps.ResolveLaunchDefaults = func(bool) jfsettings.LaunchDefaults {
		return runtimeConfig.Defaults
	}
	defaultEnvOrDefault := deps.EnvOrDefault
	deps.EnvOrDefault = func(key string, fallback string) string {
		switch key {
		case "JFTRADE_SETTINGS_PATH":
			return runtimeConfig.SettingsPath
		case "JFTRADE_BACKTEST_DB":
			return runtimeConfig.BacktestPath
		case "JFTRADE_API_BIND":
			return runtimeConfig.APIBind
		default:
			return defaultEnvOrDefault(key, fallback)
		}
	}
	deps.NewHandler = func(store lifecycle.SettingsStore) (lifecycle.Handler, error) {
		settingsStore, ok := store.(servercore.SidecarSettingsStore)
		if !ok {
			return nil, fmt.Errorf("unexpected settings store type %T", store)
		}
		handler := servercore.NewSidecarHandlerWithOptions(settingsStore, servercore.SidecarOptions{
			FrontendFS:       loadFrontendFS(),
			FrontendDevURL:   desktopFrontendDevURL(),
			NotificationSink: sink,
			DesktopMode:      true,
			DesktopAPIToken:  runtimeConfig.APIToken,
		})
		handler.ConfigureAuthOrigins(desktopTrustedOrigins()...)
		return handler, nil
	}
	shutdown, err := lifecycle.StartForRunArgs(ctx, []string{"api"}, deps)
	if err != nil {
		return nil, err
	}
	if err := waitDesktopAPIReady(ctx, runtimeConfig.APIBaseURL, runtimeConfig.APIToken); err != nil {
		shutdownErr := shutdown(context.Background())
		if shutdownErr != nil {
			return nil, fmt.Errorf("%w; shutdown after desktop API startup failure: %v", err, shutdownErr)
		}
		return nil, err
	}
	return shutdown, nil
}

func desktopFrontendDevURL() string {
	if loadFrontendFS() != nil {
		return ""
	}
	if strings.TrimSpace(os.Getenv("JFTRADE_DESKTOP_MODE")) != "1" {
		return ""
	}
	return envOrDefault("FRONTEND_DEVSERVER_URL", "http://127.0.0.1:5173")
}

func desktopTrustedOrigins() []string {
	origins := []string{
		"wails://localhost",
		"wails://127.0.0.1",
		"wails://localhost:5173",
		"wails://127.0.0.1:5173",
		"http://wails.localhost",
		"https://wails.localhost",
	}
	for _, origin := range wailsOriginsForDevServer(os.Getenv("FRONTEND_DEVSERVER_URL")) {
		if !containsString(origins, origin) {
			origins = append(origins, origin)
		}
	}
	return origins
}

func wailsOriginsForDevServer(rawURL string) []string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return nil
	}
	hosts := []string{parsed.Host}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err == nil && port != "" {
		switch strings.Trim(host, "[]") {
		case "127.0.0.1":
			hosts = append(hosts, net.JoinHostPort("localhost", port))
		case "localhost":
			hosts = append(hosts, net.JoinHostPort("127.0.0.1", port))
		}
	}
	origins := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if host = strings.TrimSpace(host); host != "" {
			origins = append(origins, "wails://"+host)
		}
	}
	return origins
}

func containsString(values []string, target string) bool {
	return slices.Contains(values, target)
}

func waitDesktopAPIReady(ctx context.Context, apiBaseURL string, apiToken string) error {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if apiBaseURL == "" {
		return fmt.Errorf("desktop API base URL is empty")
	}

	waitCtx, cancel := context.WithTimeout(ctx, desktopAPIReadyTimeout)
	defer cancel()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	statusURL := apiBaseURL + "/api/v1/system/status"
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(waitCtx, http.MethodGet, statusURL, nil)
		if err != nil {
			return err
		}
		if token := strings.TrimSpace(apiToken); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return nil
			}
			lastErr = fmt.Errorf("desktop API readiness status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		select {
		case <-waitCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("desktop API did not become ready at %s: %w", apiBaseURL, lastErr)
			}
			return fmt.Errorf("desktop API did not become ready at %s: %w", apiBaseURL, waitCtx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func dependencies() lifecycle.Dependencies {
	return lifecycle.Dependencies{
		ShouldStartForArgs:    shouldStartForArgs,
		LoadFrontendFS:        loadFrontendFS,
		ResolveLaunchDefaults: apiruntime.ResolveLaunchDefaults,
		EnvOrDefault:          envOrDefault,
		EnsureRuntimeLayout:   apiruntime.EnsureRuntimeLayout,
		ApplyDatabaseRebuild: func(settingsPath string, backtestDBPath string) error {
			return datamigration.NewManager(settingsPath, backtestDBPath).ApplyPending()
		},
		CompleteDatabaseRebuild: func(settingsPath string, backtestDBPath string) error {
			return datamigration.NewManager(settingsPath, backtestDBPath).CompletePending(context.Background())
		},
		NewSettingsStore:          newSettingsStore,
		ResolveIntegrationRuntime: apiruntime.IntegrationWithEnvDefaults,
		ApplyIntegrationRuntime:   apiruntime.ApplyIntegrationEnv,
		NewHandler:                newHandler,
		APIBaseURLForBind:         apiruntime.APIBaseURLForBind,
		PortFromBind:              apiruntime.PortFromBind,
	}
}

func newSettingsStore(path string) (lifecycle.SettingsStore, error) {
	return settingsfile.New(path)
}

func newHandler(store lifecycle.SettingsStore) (lifecycle.Handler, error) {
	settingsStore, ok := store.(servercore.SidecarSettingsStore)
	if !ok {
		return nil, fmt.Errorf("unexpected settings store type %T", store)
	}
	return servercore.NewSidecarHandlerWithStore(settingsStore, nil, ""), nil
}

func loadFrontendFS() fs.FS {
	frontendFS, available, err := frontendassets.FileSystem()
	if err != nil {
		log.Printf("JFTrade embedded frontend assets unavailable: %v", err)
		return nil
	}
	if !available {
		return nil
	}
	return frontendFS
}

func shouldStartForArgs(args []string) bool {
	if strings.EqualFold(os.Getenv("JFTRADE_API_DISABLED"), "1") || strings.EqualFold(os.Getenv("JFTRADE_API_DISABLED"), "true") {
		return false
	}
	for _, arg := range args {
		if arg == "api" || arg == "serve-api" {
			return true
		}
		if arg == "help" || arg == "--help" || arg == "-h" {
			return false
		}
	}
	return false
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
