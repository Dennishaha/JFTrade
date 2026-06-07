package jftradeapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ensureRuntimeLayout(settingsPath string, backtestDBPath string) error {
	directories := []string{
		filepath.Dir(strings.TrimSpace(settingsPath)),
		filepath.Dir(deriveStrategyCatalogPath(settingsPath)),
		filepath.Dir(deriveStrategyRuntimeDBPath(settingsPath)),
		filepath.Dir(deriveStrategyDesignPath(settingsPath)),
		filepath.Dir(deriveBacktestRunDBPath(settingsPath)),
		filepath.Dir(deriveADKDBPath(settingsPath)),
		filepath.Dir(deriveADKSessionDBPath(settingsPath)),
		filepath.Dir(deriveADKSecretsPath(settingsPath)),
		deriveStrategyPluginTargetDir(settingsPath),
		deriveADKSkillsDir(settingsPath),
		filepath.Dir(strings.TrimSpace(backtestDBPath)),
	}

	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		directory = strings.TrimSpace(directory)
		if directory == "" || directory == "." {
			continue
		}
		if _, ok := seen[directory]; ok {
			continue
		}
		seen[directory] = struct{}{}
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", directory, err)
		}
	}

	return nil
}

func deriveBacktestDBPath() string {
	path := strings.TrimSpace(os.Getenv("JFTRADE_BACKTEST_DB"))
	if path == "" {
		return resolveLaunchDefaults(loadFrontendFS() != nil).backtestDBPath
	}
	return path
}
