package apiserver

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/lifecycle"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	"github.com/jftrade/jftrade-main/internal/frontendassets"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// RunAPIOnly starts the sidecar in API-only mode and waits for ctx shutdown.
func RunAPIOnly(ctx context.Context) error {
	return lifecycle.RunAPIOnly(ctx, dependencies())
}

// StartForRunArgs starts the API sidecar for bbgo-compatible command args.
func StartForRunArgs(ctx context.Context, args []string) (func(context.Context) error, error) {
	return lifecycle.StartForRunArgs(ctx, args, dependencies())
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
		ResolveGUIRuntimeAPIBase:  resolveGUIRuntimeAPIBaseURL,
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

func resolveGUIAPIBaseURL(interfaceSettings jfsettings.InterfaceSettings, apiBind string) string {
	envValue := strings.TrimSpace(os.Getenv("JFTRADE_GUI_API_BASE_URL"))
	if envValue != "" {
		return envValue
	}

	configuredValue := strings.TrimSpace(interfaceSettings.GUIAPIBaseURL)
	defaultConfiguredValue := apiruntime.APIBaseURLForBind(interfaceSettings.APIBind)
	if configuredValue == "" || configuredValue == defaultConfiguredValue {
		return apiruntime.APIBaseURLForBind(apiBind)
	}
	return configuredValue
}

func resolveGUIRuntimeAPIBaseURL(interfaceSettings jfsettings.InterfaceSettings, apiBind string) string {
	envValue := strings.TrimSpace(os.Getenv("JFTRADE_GUI_API_BASE_URL"))
	if envValue != "" {
		return envValue
	}

	guiAPIBaseURL := resolveGUIAPIBaseURL(interfaceSettings, apiBind)
	if guiAPIBaseURL == apiruntime.APIBaseURLForBind(apiBind) {
		return ""
	}
	return guiAPIBaseURL
}
