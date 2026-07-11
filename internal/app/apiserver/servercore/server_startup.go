package servercore

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/lifecycle"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
)

// StartForRunArgs boots the JFTrade API sidecar as HTTP servers (API + optional GUI).
// It returns a shutdown function that the caller should invoke on process exit.
func StartForRunArgs(ctx context.Context, args []string) (func(context.Context) error, error) {
	return lifecycle.StartForRunArgs(ctx, args, startupDependencies())
}

func startupDependencies() lifecycle.Dependencies {
	return lifecycle.Dependencies{
		ShouldStartForArgs:    shouldStartForArgs,
		LoadFrontendFS:        loadFrontendFS,
		ResolveLaunchDefaults: resolveLaunchDefaults,
		EnvOrDefault:          envOrDefault,
		EnsureRuntimeLayout:   ensureRuntimeLayout,
		ApplyDatabaseRebuild: func(settingsPath string, backtestDBPath string) error {
			return datamigration.NewManager(settingsPath, backtestDBPath).ApplyPending()
		},
		CompleteDatabaseRebuild: func(settingsPath string, backtestDBPath string) error {
			return datamigration.NewManager(settingsPath, backtestDBPath).CompletePending(context.Background())
		},
		NewSettingsStore:          newLifecycleSettingsStore,
		ResolveIntegrationRuntime: apiruntime.IntegrationWithEnvDefaults,
		ApplyIntegrationRuntime:   apiruntime.ApplyIntegrationEnv,
		NewHandler:                newLifecycleHandler,
		APIBaseURLForBind:         apiBaseURLForBind,
		PortFromBind:              portFromBind,
	}
}

func newLifecycleSettingsStore(path string) (lifecycle.SettingsStore, error) {
	return NewSettingsStore(path)
}

func newLifecycleHandler(store lifecycle.SettingsStore) (lifecycle.Handler, error) {
	settingsStore, ok := store.(*SettingsStore)
	if !ok {
		return nil, fmt.Errorf("unexpected settings store type %T", store)
	}
	return NewSidecarHandler(settingsStore, nil, ""), nil
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

func envOrDefault(key string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}
