package jftradeapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// StartForRunArgs boots the JFTrade API sidecar as HTTP servers (API + optional GUI).
// It returns a shutdown function that the caller should invoke on process exit.
func StartForRunArgs(ctx context.Context, args []string) (func(context.Context) error, error) {
	if !shouldStartForArgs(args) {
		return func(context.Context) error { return nil }, nil
	}

	frontendFS := loadFrontendFS()
	defaults := resolveLaunchDefaults(frontendFS != nil)
	settingsPath := envOrDefault("JFTRADE_SETTINGS_PATH", defaults.settingsPath)
	backtestDBPath := envOrDefault("JFTRADE_BACKTEST_DB", defaults.backtestDBPath)
	if err := ensureRuntimeLayout(settingsPath, backtestDBPath); err != nil {
		return nil, err
	}
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		return nil, err
	}
	if err := store.ensureBootstrapFile(defaults); err != nil {
		return nil, err
	}
	store.applyRuntimeEnv()
	interfaceSettings := store.interfaceSettings(defaults)

	apiBind := envOrDefault("JFTRADE_API_BIND", interfaceSettings.APIBind)
	apiHandler := newServerWithFrontend(store, nil)
	apiHandler.apiPort = portFromBind(apiBind, portFromBind(defaults.apiBind, 3000))
	apiHandler.auth.configureOrigins(apiBaseURLForBind(apiBind))
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
		guiBind := envOrDefault("JFTRADE_GUI_BIND", interfaceSettings.GUIBind)
		guiAPIBaseURL := resolveGUIRuntimeAPIBaseURL(interfaceSettings, apiBind)
		apiHandler.frontend = newFrontendServerWithRuntimeConfig(frontendFS, guiAPIBaseURL)
		apiHandler.applySecuritySettings(store.securitySettings())
		apiHandler.auth.configureOrigins("http://" + guiBind)
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

	var shutdownOnce sync.Once
	shutdownAll := func(shutdownCtx context.Context) error {
		var shutdownErr error
		shutdownOnce.Do(func() {
			for _, server := range servers {
				if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && shutdownErr == nil {
					shutdownErr = err
				}
			}
			if err := apiHandler.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		})
		return shutdownErr
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownAll(shutdownCtx)
	}()

	return shutdownAll, nil
}

func resolveGUIAPIBaseURL(interfaceSettings InterfaceSettings, apiBind string) string {
	envValue := strings.TrimSpace(os.Getenv("JFTRADE_GUI_API_BASE_URL"))
	if envValue != "" {
		return envValue
	}

	configuredValue := strings.TrimSpace(interfaceSettings.GUIAPIBaseURL)
	defaultConfiguredValue := apiBaseURLForBind(interfaceSettings.APIBind)
	if configuredValue == "" || configuredValue == defaultConfiguredValue {
		return apiBaseURLForBind(apiBind)
	}
	return configuredValue
}

func resolveGUIRuntimeAPIBaseURL(interfaceSettings InterfaceSettings, apiBind string) string {
	envValue := strings.TrimSpace(os.Getenv("JFTRADE_GUI_API_BASE_URL"))
	if envValue != "" {
		return envValue
	}

	guiAPIBaseURL := resolveGUIAPIBaseURL(interfaceSettings, apiBind)
	if guiAPIBaseURL == apiBaseURLForBind(apiBind) {
		return ""
	}
	return guiAPIBaseURL
}

func shouldStartForArgs(args []string) bool {
	if strings.EqualFold(os.Getenv("JFTRADE_API_DISABLED"), "1") || strings.EqualFold(os.Getenv("JFTRADE_API_DISABLED"), "true") {
		return false
	}
	for _, arg := range args {
		if arg == "run" || arg == "api" || arg == "serve-api" {
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
