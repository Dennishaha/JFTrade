package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
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
	WebAccessHandler() http.Handler
	Close() error
	SetAPIPort(int)
	ConfigureAuthOrigins(...string)
	SetFrontendFS(fs.FS, string)
	ApplySecuritySettings(jfsettings.SecuritySettings)
	SetWebAccessReconfigure(func(jfsettings.SecuritySettings) error)
}

// Dependencies contains the package-specific pieces used by startup lifecycle.
type Dependencies struct {
	SeparateWebListener       bool
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
	securitySettings := store.SecuritySettings()
	configuredAPIBind := deps.EnvOrDefault("JFTRADE_API_BIND", interfaceSettings.APIBind)
	apiBind := webAccessBind(configuredAPIBind, securitySettings)
	if deps.SeparateWebListener {
		apiBind = loopbackBind(configuredAPIBind)
	}
	apiHandler, err := newLifecycleHandler(deps, startup, store)
	if err != nil {
		return nil, err
	}
	servers, webManager, err := startLifecycleServers(deps, startup, store, interfaceSettings, apiBind, apiHandler)
	if err != nil {
		_ = apiHandler.Close()
		return nil, err
	}
	shutdownAll := onceShutdown(servers, webManager, apiHandler)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		jftradeErr1 := shutdownAll(shutdownCtx)
		besteffort.LogError(jftradeErr1)
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

func startLifecycleServers(deps Dependencies, startup lifecycleStartup, store SettingsStore, interfaceSettings jfsettings.InterfaceSettings, apiBind string, apiHandler Handler) ([]*http.Server, *webAccessServerManager, error) {
	if deps.SeparateWebListener {
		webManager := newWebAccessServerManager(deps, apiHandler)
		if err := webManager.Reconfigure(store.SecuritySettings()); err != nil {
			return nil, nil, err
		}
		apiHandler.SetWebAccessReconfigure(webManager.Reconfigure)
		apiServer, err := startLifecycleAPIServer(deps, startup.defaults, apiBind, apiHandler)
		if err != nil {
			_ = webManager.Shutdown(context.Background())
			return nil, nil, err
		}
		return []*http.Server{apiServer}, webManager, nil
	}
	if startup.frontendFS != nil {
		guiBind := webAccessBind(deps.EnvOrDefault("JFTRADE_GUI_BIND", interfaceSettings.GUIBind), store.SecuritySettings())
		if guiBind != "" {
			server, err := startLifecycleIntegratedServer(deps, startup, store, guiBind, apiHandler)
			if err != nil {
				return nil, nil, err
			}
			return []*http.Server{server}, nil, nil
		}
	}

	apiServer, err := startLifecycleAPIServer(deps, startup.defaults, apiBind, apiHandler)
	if err != nil {
		return nil, nil, err
	}
	return []*http.Server{apiServer}, nil, nil
}

func loopbackBind(configured string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(configured))
	if err != nil || strings.TrimSpace(port) == "" {
		return configured
	}
	return net.JoinHostPort("127.0.0.1", port)
}

func webAccessListenerBind(settings jfsettings.SecuritySettings) string {
	if !settings.WebAccessEnabled || !settings.PasswordConfigured {
		return ""
	}
	port := settings.WebPort
	if port < jfsettings.MinWebAccessPort || port > jfsettings.MaxWebAccessPort {
		port = jfsettings.DefaultWebAccessPort
	}
	host := "127.0.0.1"
	if settings.PublicAccessEnabled {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func webAccessBind(configured string, settings jfsettings.SecuritySettings) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(configured))
	if err != nil || strings.TrimSpace(port) == "" {
		return configured
	}
	host := "127.0.0.1"
	if settings.WebAccessEnabled && settings.PasswordConfigured && settings.PublicAccessEnabled {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, port)
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

type webAccessServerManager struct {
	mu       sync.Mutex
	deps     Dependencies
	handler  Handler
	listen   func(network string, address string) (net.Listener, error)
	listener net.Listener
	server   *http.Server
	bind     string
}

func newWebAccessServerManager(deps Dependencies, handler Handler) *webAccessServerManager {
	return &webAccessServerManager{deps: deps, handler: handler, listen: net.Listen}
}

func (m *webAccessServerManager) Reconfigure(settings jfsettings.SecuritySettings) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	desiredBind := webAccessListenerBind(settings)
	if desiredBind == m.bind {
		m.handler.ApplySecuritySettings(settings)
		return nil
	}
	if desiredBind == "" {
		m.handler.ApplySecuritySettings(settings)
		m.closeCurrentLocked()
		return nil
	}

	// Changing only the host while keeping the same port cannot be pre-bound on
	// most platforms. Close briefly, then restore the old bind if the new bind
	// fails. Port changes take the safer path below and bind first.
	if m.server != nil && bindPort(m.bind) == bindPort(desiredBind) {
		oldBind := m.bind
		m.closeCurrentLocked()
		listener, err := m.listen("tcp", desiredBind)
		if err != nil {
			if restoreErr := m.restoreLocked(oldBind); restoreErr != nil {
				return fmt.Errorf("JFTrade Web access port conflict on %s: %w; restoring %s also failed: %v", desiredBind, err, oldBind, restoreErr)
			}
			return fmt.Errorf("JFTrade Web access port conflict on %s: %w", desiredBind, err)
		}
		m.handler.ApplySecuritySettings(settings)
		m.startLocked(desiredBind, listener)
		return nil
	}

	listener, err := m.listen("tcp", desiredBind)
	if err != nil {
		return fmt.Errorf("JFTrade Web access port conflict on %s: %w", desiredBind, err)
	}
	m.handler.ApplySecuritySettings(settings)
	oldServer := m.server
	oldListener := m.listener
	m.startLocked(desiredBind, listener)
	if oldListener != nil {
		_ = oldListener.Close()
	}
	if oldServer != nil {
		_ = oldServer.Close()
	}
	return nil
}

func (m *webAccessServerManager) restoreLocked(bind string) error {
	if bind == "" {
		return nil
	}
	listener, err := m.listen("tcp", bind)
	if err != nil {
		return err
	}
	m.startLocked(bind, listener)
	return nil
}

func (m *webAccessServerManager) startLocked(bind string, listener net.Listener) {
	server := &http.Server{
		Addr:              bind,
		Handler:           m.handler.WebAccessHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	m.bind = bind
	m.listener = listener
	m.server = server
	go func() {
		log.Printf("JFTrade optional Web access listening on http://%s", bind)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("JFTrade Web access server stopped: %v", err)
		}
	}()
}

func (m *webAccessServerManager) closeCurrentLocked() {
	if m.listener != nil {
		_ = m.listener.Close()
	}
	if m.server != nil {
		_ = m.server.Close()
	}
	m.listener = nil
	m.server = nil
	m.bind = ""
}

func (m *webAccessServerManager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == nil {
		return nil
	}
	err := m.server.Shutdown(ctx)
	if m.listener != nil {
		_ = m.listener.Close()
	}
	m.listener = nil
	m.server = nil
	m.bind = ""
	return err
}

func bindPort(bind string) string {
	_, port, err := net.SplitHostPort(bind)
	if err != nil {
		return ""
	}
	return port
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

func onceShutdown(servers []*http.Server, webManager *webAccessServerManager, handler Handler) func(context.Context) error {
	var shutdownOnce sync.Once
	var shutdownErr error
	return func(shutdownCtx context.Context) error {
		shutdownOnce.Do(func() {
			if err := webManager.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && shutdownErr == nil {
				shutdownErr = err
			}
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
