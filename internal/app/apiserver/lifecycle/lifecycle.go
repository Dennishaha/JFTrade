package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// SettingsStore is the settings surface needed by API server startup.
type SettingsStore interface {
	EnsureBootstrapFile(jfsettings.LaunchDefaults) error
	Integration() jfsettings.BrokerIntegration
	SavedIntegration() *jfsettings.BrokerIntegration
	InterfaceSettings(jfsettings.LaunchDefaults) jfsettings.InterfaceSettings
	SecuritySettings() jfsettings.SecuritySettings
}

// Handler is the sidecar HTTP handler surface needed by API server startup.
type Handler interface {
	http.Handler
	Close() error
	SetAPIPort(int)
	ConfigureAuthOrigins(...string)
	SetFrontendFS(fs.FS, string)
	ApplySecuritySettings(jfsettings.SecuritySettings)
}

// Dependencies contains the package-specific pieces used by startup lifecycle.
type Dependencies struct {
	ShouldStartForArgs        func([]string) bool
	LoadFrontendFS            func() fs.FS
	ResolveLaunchDefaults     func(bool) jfsettings.LaunchDefaults
	EnvOrDefault              func(string, string) string
	EnsureRuntimeLayout       func(settingsPath string, backtestDBPath string) error
	ApplyDatabaseRebuild      func(settingsPath string, backtestDBPath string) error
	CompleteDatabaseRebuild   func(settingsPath string, backtestDBPath string) error
	NewSettingsStore          func(path string) (SettingsStore, error)
	ResolveIntegrationRuntime func(jfsettings.BrokerIntegration) jfsettings.BrokerIntegration
	ApplyIntegrationRuntime   func(jfsettings.BrokerIntegration)
	NewHandler                func(store SettingsStore) (Handler, error)
	APIBaseURLForBind         func(bind string) string
	PortFromBind              func(bind string, fallback int) int
}

type lifecycleStartup struct {
	frontendFS     fs.FS
	defaults       jfsettings.LaunchDefaults
	settingsPath   string
	backtestDBPath string
}

// StartForRunArgs boots the JFTrade API sidecar as HTTP servers.
func StartForRunArgs(ctx context.Context, args []string, deps Dependencies) (func(context.Context) error, error) {
	if !deps.ShouldStartForArgs(args) {
		return func(context.Context) error { return nil }, nil
	}
	startup, err := prepareLifecycleStartup(deps)
	if err != nil {
		return nil, err
	}
	store, err := openLifecycleSettingsStore(deps, startup)
	if err != nil {
		return nil, err
	}
	applyLifecycleIntegrationRuntime(deps, store)
	interfaceSettings := store.InterfaceSettings(startup.defaults)
	apiBind := deps.EnvOrDefault("JFTRADE_API_BIND", interfaceSettings.APIBind)
	apiHandler, err := newLifecycleHandler(deps, startup, store)
	if err != nil {
		return nil, err
	}
	servers, err := startLifecycleServers(deps, startup, store, interfaceSettings, apiBind, apiHandler)
	if err != nil {
		_ = apiHandler.Close()
		return nil, err
	}
	shutdownAll := onceShutdown(servers, apiHandler)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		jftradeErr1 := shutdownAll(shutdownCtx)
		jftradeLogError(jftradeErr1)
	}()

	return shutdownAll, nil
}

func prepareLifecycleStartup(deps Dependencies) (lifecycleStartup, error) {
	startup := lifecycleStartup{}
	startup.frontendFS = deps.LoadFrontendFS()
	startup.defaults = deps.ResolveLaunchDefaults(startup.frontendFS != nil)
	startup.settingsPath = deps.EnvOrDefault("JFTRADE_SETTINGS_PATH", startup.defaults.SettingsPath)
	startup.backtestDBPath = deps.EnvOrDefault("JFTRADE_BACKTEST_DB", startup.defaults.BacktestDBPath)
	if err := deps.EnsureRuntimeLayout(startup.settingsPath, startup.backtestDBPath); err != nil {
		return lifecycleStartup{}, err
	}
	if deps.ApplyDatabaseRebuild != nil {
		if err := deps.ApplyDatabaseRebuild(startup.settingsPath, startup.backtestDBPath); err != nil {
			return lifecycleStartup{}, err
		}
	}
	return startup, nil
}

func openLifecycleSettingsStore(deps Dependencies, startup lifecycleStartup) (SettingsStore, error) {
	store, err := deps.NewSettingsStore(startup.settingsPath)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureBootstrapFile(startup.defaults); err != nil {
		return nil, err
	}
	return store, nil
}

func applyLifecycleIntegrationRuntime(deps Dependencies, store SettingsStore) {
	if deps.ApplyIntegrationRuntime == nil {
		return
	}
	integration := store.Integration()
	if store.SavedIntegration() == nil && deps.ResolveIntegrationRuntime != nil {
		integration = deps.ResolveIntegrationRuntime(integration)
	}
	deps.ApplyIntegrationRuntime(integration)
}

func newLifecycleHandler(deps Dependencies, startup lifecycleStartup, store SettingsStore) (Handler, error) {
	apiHandler, err := deps.NewHandler(store)
	if err != nil {
		return nil, err
	}
	if deps.CompleteDatabaseRebuild == nil {
		return apiHandler, nil
	}
	if err := deps.CompleteDatabaseRebuild(startup.settingsPath, startup.backtestDBPath); err != nil {
		_ = apiHandler.Close()
		return nil, err
	}
	return apiHandler, nil
}

func startLifecycleServers(deps Dependencies, startup lifecycleStartup, store SettingsStore, interfaceSettings jfsettings.InterfaceSettings, apiBind string, apiHandler Handler) ([]*http.Server, error) {
	if startup.frontendFS != nil {
		guiBind := deps.EnvOrDefault("JFTRADE_GUI_BIND", interfaceSettings.GUIBind)
		if guiBind != "" {
			server, err := startLifecycleIntegratedServer(deps, startup, store, guiBind, apiHandler)
			if err != nil {
				return nil, err
			}
			return []*http.Server{server}, nil
		}
	}

	apiServer, err := startLifecycleAPIServer(deps, startup.defaults, apiBind, apiHandler)
	if err != nil {
		return nil, err
	}
	return []*http.Server{apiServer}, nil
}

func startLifecycleAPIServer(deps Dependencies, defaults jfsettings.LaunchDefaults, apiBind string, apiHandler Handler) (*http.Server, error) {
	apiHandler.SetAPIPort(deps.PortFromBind(apiBind, deps.PortFromBind(defaults.APIBind, 3000)))
	apiHandler.ConfigureAuthOrigins(deps.APIBaseURLForBind(apiBind))
	listener, err := net.Listen("tcp", apiBind)
	if err != nil {
		return nil, fmt.Errorf("JFTrade API port conflict on %s: %w", apiBind, err)
	}
	apiServer := &http.Server{
		Addr:              apiBind,
		Handler:           apiHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("JFTrade API listening on http://%s", apiBind)
		if err := apiServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("JFTrade API server stopped: %v", err)
		}
	}()
	return apiServer, nil
}

func startLifecycleIntegratedServer(deps Dependencies, startup lifecycleStartup, store SettingsStore, guiBind string, apiHandler Handler) (*http.Server, error) {
	listener, err := net.Listen("tcp", guiBind)
	if err != nil {
		return nil, fmt.Errorf("JFTrade integrated HTTP port conflict on %s: %w", guiBind, err)
	}
	apiHandler.SetAPIPort(deps.PortFromBind(guiBind, deps.PortFromBind(startup.defaults.GUIBind, 6688)))
	apiHandler.SetFrontendFS(startup.frontendFS, "")
	apiHandler.ApplySecuritySettings(store.SecuritySettings())
	apiHandler.ConfigureAuthOrigins(deps.APIBaseURLForBind(guiBind))
	integratedServer := &http.Server{
		Addr:              guiBind,
		Handler:           apiHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		fmt.Printf("JFTrade 交互界面已启动，请访问 http://%s\n\n", guiBind)
		log.Printf("JFTrade integrated frontend and API listening on http://%s", guiBind)
		if err := integratedServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("JFTrade integrated HTTP server stopped: %v", err)
		}
	}()
	return integratedServer, nil
}

// RunAPIOnly starts the sidecar in API-only mode and waits for ctx shutdown.
func RunAPIOnly(ctx context.Context, deps Dependencies) error {
	shutdown, err := StartForRunArgs(ctx, []string{"api"}, deps)
	if err != nil {
		return err
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return shutdown(shutdownCtx)
}

func onceShutdown(servers []*http.Server, handler Handler) func(context.Context) error {
	var shutdownOnce sync.Once
	var shutdownErr error
	return func(shutdownCtx context.Context) error {
		shutdownOnce.Do(func() {
			for _, server := range servers {
				if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && shutdownErr == nil {
					shutdownErr = err
				}
			}
			if err := handler.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		})
		return shutdownErr
	}
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
