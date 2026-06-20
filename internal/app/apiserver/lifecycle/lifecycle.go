package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
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
	NewSettingsStore          func(path string) (SettingsStore, error)
	ResolveIntegrationRuntime func(jfsettings.BrokerIntegration) jfsettings.BrokerIntegration
	ApplyIntegrationRuntime   func(jfsettings.BrokerIntegration)
	NewHandler                func(store SettingsStore) (Handler, error)
	APIBaseURLForBind         func(bind string) string
	PortFromBind              func(bind string, fallback int) int
	ResolveGUIRuntimeAPIBase  func(settings jfsettings.InterfaceSettings, apiBind string) string
}

// StartForRunArgs boots the JFTrade API sidecar as HTTP servers.
func StartForRunArgs(ctx context.Context, args []string, deps Dependencies) (func(context.Context) error, error) {
	if !deps.ShouldStartForArgs(args) {
		return func(context.Context) error { return nil }, nil
	}

	frontendFS := deps.LoadFrontendFS()
	defaults := deps.ResolveLaunchDefaults(frontendFS != nil)
	settingsPath := deps.EnvOrDefault("JFTRADE_SETTINGS_PATH", defaults.SettingsPath)
	backtestDBPath := deps.EnvOrDefault("JFTRADE_BACKTEST_DB", defaults.BacktestDBPath)
	if err := deps.EnsureRuntimeLayout(settingsPath, backtestDBPath); err != nil {
		return nil, err
	}
	store, err := deps.NewSettingsStore(settingsPath)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureBootstrapFile(defaults); err != nil {
		return nil, err
	}
	if deps.ApplyIntegrationRuntime != nil {
		integration := store.Integration()
		if store.SavedIntegration() == nil && deps.ResolveIntegrationRuntime != nil {
			integration = deps.ResolveIntegrationRuntime(integration)
		}
		deps.ApplyIntegrationRuntime(integration)
	}
	interfaceSettings := store.InterfaceSettings(defaults)

	apiBind := deps.EnvOrDefault("JFTRADE_API_BIND", interfaceSettings.APIBind)
	apiHandler, err := deps.NewHandler(store)
	if err != nil {
		return nil, err
	}
	apiHandler.SetAPIPort(deps.PortFromBind(apiBind, deps.PortFromBind(defaults.APIBind, 3000)))
	apiHandler.ConfigureAuthOrigins(deps.APIBaseURLForBind(apiBind))
	apiServer := &http.Server{
		Addr:              apiBind,
		Handler:           apiHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	servers := []*http.Server{apiServer}

	go func() {
		log.Printf("JFTrade API listening on http://%s", apiBind)
		if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("JFTrade API server stopped: %v", err)
		}
	}()

	if frontendFS != nil {
		guiBind := deps.EnvOrDefault("JFTRADE_GUI_BIND", interfaceSettings.GUIBind)
		guiAPIBaseURL := deps.ResolveGUIRuntimeAPIBase(interfaceSettings, apiBind)
		apiHandler.SetFrontendFS(frontendFS, guiAPIBaseURL)
		apiHandler.ApplySecuritySettings(store.SecuritySettings())
		apiHandler.ConfigureAuthOrigins("http://" + guiBind)
		guiServer := &http.Server{
			Addr:              guiBind,
			Handler:           apiHandler,
			ReadHeaderTimeout: 5 * time.Second,
		}
		servers = append(servers, guiServer)

		go func() {
			fmt.Printf("JFTrade 交互界面已启动，请访问 http://%s\n\n", guiBind)
			log.Printf("JFTrade GUI listening on http://%s", guiBind)
			if err := guiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("JFTrade GUI server stopped: %v", err)
			}
		}()
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
	return func(shutdownCtx context.Context) error {
		var shutdownErr error
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
