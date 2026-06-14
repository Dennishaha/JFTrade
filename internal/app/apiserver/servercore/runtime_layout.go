package servercore

import apruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"

func ensureRuntimeLayout(settingsPath string, backtestDBPath string) error {
	return apruntime.EnsureRuntimeLayout(settingsPath, backtestDBPath)
}

// EnsureRuntimeLayout creates directories required by the API sidecar runtime.
func EnsureRuntimeLayout(settingsPath string, backtestDBPath string) error {
	return ensureRuntimeLayout(settingsPath, backtestDBPath)
}

func deriveBacktestDBPath() string {
	return apruntime.DeriveBacktestDBPath(loadFrontendFS() != nil)
}
