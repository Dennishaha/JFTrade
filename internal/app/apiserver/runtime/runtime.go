package runtime

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const (
	DefaultDevelopmentAPIBind = "127.0.0.1:3000"
	DefaultReleaseAPIBind     = "127.0.0.1:6699"
	DefaultReleaseGUIBind     = "127.0.0.1:6688"
	DefaultRuntimeDir         = "var/jftrade-api"
	DefaultSettingsFilename   = "settings.json"
	DefaultBacktestDBFilename = "backtest.db"

	defaultStrategyCatalogFilename   = "strategies.json"
	defaultStrategyPluginDirName     = "plugins"
	defaultStrategyDesignFilename    = "strategy-designs.json"
	defaultStrategyRuntimeDBFilename = "strategy-runtime.db"
	defaultBacktestRunDBFilename     = "backtest-runs.db"
	defaultExecutionOrderDBFilename  = "execution-orders.db"
)

func ResolveLaunchDefaults(embeddedFrontend bool) jfsettings.LaunchDefaults {
	return LaunchDefaultsForExecutableDir(embeddedFrontend, resolveExecutableDir())
}

func LaunchDefaultsForExecutableDir(embeddedFrontend bool, executableDir string) jfsettings.LaunchDefaults {
	runtimeRoot := defaultRuntimeRoot(embeddedFrontend, executableDir)
	defaults := jfsettings.LaunchDefaults{
		APIBind:        DefaultDevelopmentAPIBind,
		SettingsPath:   filepath.Join(DefaultRuntimeDir, DefaultSettingsFilename),
		BacktestDBPath: filepath.Join(DefaultRuntimeDir, DefaultBacktestDBFilename),
	}
	if !embeddedFrontend {
		return defaults
	}
	return jfsettings.LaunchDefaults{
		APIBind:        DefaultReleaseAPIBind,
		GUIBind:        DefaultReleaseGUIBind,
		SettingsPath:   filepath.Join(runtimeRoot, DefaultSettingsFilename),
		BacktestDBPath: filepath.Join(runtimeRoot, DefaultBacktestDBFilename),
	}
}

func defaultRuntimeRoot(embeddedFrontend bool, executableDir string) string {
	if !embeddedFrontend {
		return DefaultRuntimeDir
	}
	trimmedDir := strings.TrimSpace(executableDir)
	if trimmedDir == "" {
		return DefaultRuntimeDir
	}
	return filepath.Join(trimmedDir, DefaultRuntimeDir)
}

func resolveExecutableDir() string {
	executablePath, err := os.Executable()
	if err != nil {
		return ""
	}
	trimmedPath := strings.TrimSpace(executablePath)
	if trimmedPath == "" {
		return ""
	}
	if resolvedPath, resolveErr := filepath.EvalSymlinks(trimmedPath); resolveErr == nil && strings.TrimSpace(resolvedPath) != "" {
		trimmedPath = resolvedPath
	}
	return filepath.Dir(trimmedPath)
}

func APIBaseURLForBind(bind string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return ""
	}
	host = normalizeBrowserHost(host)
	if host == "" || port == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(host, port)
}

func normalizeBrowserHost(host string) string {
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.TrimSpace(host)
	}
}

func PortFromBind(bind string, fallback int) int {
	_, port, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return fallback
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort <= 0 {
		return fallback
	}
	return parsedPort
}

func EnsureRuntimeLayout(settingsPath string, backtestDBPath string) error {
	directories := []string{
		filepath.Dir(strings.TrimSpace(settingsPath)),
		filepath.Dir(DeriveStrategyCatalogPath(settingsPath)),
		filepath.Dir(DeriveStrategyRuntimeDBPath(settingsPath)),
		filepath.Dir(DeriveStrategyDesignPath(settingsPath)),
		filepath.Dir(DeriveBacktestRunDBPath(settingsPath)),
		filepath.Dir(DeriveADKDBPath(settingsPath)),
		filepath.Dir(DeriveADKSessionDBPath(settingsPath)),
		filepath.Dir(DeriveADKSecretsPath(settingsPath)),
		DeriveExchangeCalendarDir(settingsPath),
		DeriveStrategyPluginTargetDir(settingsPath),
		DeriveADKSkillsDir(settingsPath),
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

func DeriveBacktestDBPath(embeddedFrontend bool) string {
	path := strings.TrimSpace(os.Getenv("JFTRADE_BACKTEST_DB"))
	if path == "" {
		return ResolveLaunchDefaults(embeddedFrontend).BacktestDBPath
	}
	return path
}

func DeriveStrategyCatalogPath(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyCatalogFilename
	}
	return filepath.Join(directory, defaultStrategyCatalogFilename)
}

func DeriveStrategyPluginTargetDir(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyPluginDirName
	}
	return filepath.Join(directory, defaultStrategyPluginDirName)
}

func DeriveStrategyRuntimeDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_STRATEGY_RUNTIME_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyRuntimeDBFilename
	}
	return filepath.Join(directory, defaultStrategyRuntimeDBFilename)
}

func DeriveStrategyDesignPath(settingsPath string) string {
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultStrategyDesignFilename
	}
	return filepath.Join(directory, defaultStrategyDesignFilename)
}

func DeriveBacktestRunDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_BACKTEST_RUN_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultBacktestRunDBFilename
	}
	return filepath.Join(directory, defaultBacktestRunDBFilename)
}

func DeriveExecutionOrderDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_EXECUTION_ORDER_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultExecutionOrderDBFilename
	}
	return filepath.Join(directory, defaultExecutionOrderDBFilename)
}

func DeriveADKDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return "adk.db"
	}
	return filepath.Join(directory, "adk.db")
}

func DeriveADKSecretsPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_SECRETS")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return filepath.Join("secrets", "adk-secrets.json")
	}
	return filepath.Join(directory, "secrets", "adk-secrets.json")
}

func DeriveADKSkillsDir(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_SKILLS_DIR")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return filepath.Join("adk", "skills")
	}
	return filepath.Join(directory, "adk", "skills")
}

func DeriveADKSessionDBPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_ADK_SESSION_DB")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return "adk-session.db"
	}
	return filepath.Join(directory, "adk-session.db")
}

func DeriveExchangeCalendarDir(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_EXCHANGE_CALENDAR_DIR")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return filepath.Join("exchange-calendars")
	}
	return filepath.Join(directory, "exchange-calendars")
}
