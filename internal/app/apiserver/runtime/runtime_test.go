package runtime

import (
	"os"
	"path/filepath"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestLaunchDefaultsForDevelopmentMode(t *testing.T) {
	defaults := LaunchDefaultsForExecutableDir(false, filepath.Join(t.TempDir(), "ignored"))
	if defaults.APIBind != DefaultDevelopmentAPIBind {
		t.Fatalf("APIBind = %q", defaults.APIBind)
	}
	if defaults.GUIBind != "" {
		t.Fatalf("GUIBind = %q, want empty", defaults.GUIBind)
	}
	if defaults.SettingsPath != filepath.Join(DefaultRuntimeDir, DefaultSettingsFilename) {
		t.Fatalf("SettingsPath = %q", defaults.SettingsPath)
	}
	if defaults.BacktestDBPath != filepath.Join(DefaultRuntimeDir, DefaultBacktestDBFilename) {
		t.Fatalf("BacktestDBPath = %q", defaults.BacktestDBPath)
	}
}

func TestLaunchDefaultsForEmbeddedFrontendMode(t *testing.T) {
	executableDir := filepath.Join(t.TempDir(), "bin")
	defaults := LaunchDefaultsForExecutableDir(true, executableDir)

	if defaults.APIBind != DefaultReleaseAPIBind {
		t.Fatalf("APIBind = %q", defaults.APIBind)
	}
	if defaults.GUIBind != DefaultReleaseGUIBind {
		t.Fatalf("GUIBind = %q", defaults.GUIBind)
	}
	wantSettingsPath := filepath.Join(executableDir, DefaultRuntimeDir, DefaultSettingsFilename)
	if defaults.SettingsPath != wantSettingsPath {
		t.Fatalf("SettingsPath = %q, want %q", defaults.SettingsPath, wantSettingsPath)
	}
}

func TestBindHelpers(t *testing.T) {
	if got := APIBaseURLForBind("0.0.0.0:6699"); got != "http://127.0.0.1:6699" {
		t.Fatalf("APIBaseURLForBind() = %q", got)
	}
	if got := APIBaseURLForBind("127.0.0.1:6699"); got != "http://127.0.0.1:6699" {
		t.Fatalf("APIBaseURLForBind() = %q", got)
	}
	if got := PortFromBind("127.0.0.1:6699", 3000); got != 6699 {
		t.Fatalf("PortFromBind() = %d", got)
	}
	if got := PortFromBind("bad", 3000); got != 3000 {
		t.Fatalf("PortFromBind() fallback = %d", got)
	}
}

func TestEnsureRuntimeLayout(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "var", "jftrade-api")
	settingsPath := filepath.Join(runtimeDir, "settings.json")
	backtestDBPath := filepath.Join(runtimeDir, "backtest.db")

	if err := EnsureRuntimeLayout(settingsPath, backtestDBPath); err != nil {
		t.Fatalf("EnsureRuntimeLayout: %v", err)
	}

	for _, path := range []string{
		runtimeDir,
		DeriveStrategyPluginTargetDir(settingsPath),
		DeriveADKSkillsDir(settingsPath),
		filepath.Dir(DeriveADKSecretsPath(settingsPath)),
		filepath.Dir(backtestDBPath),
	} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("runtime directory %s not initialized: info=%v err=%v", path, info, err)
		}
	}
}

func TestRuntimePathEnvOverrides(t *testing.T) {
	backtestOverride := filepath.Join(t.TempDir(), "backtest.db")
	t.Setenv("JFTRADE_BACKTEST_DB", backtestOverride)
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", filepath.Join(t.TempDir(), "strategy.db"))
	t.Setenv("JFTRADE_ADK_DB", filepath.Join(t.TempDir(), "adk.db"))
	t.Setenv("JFTRADE_BACKTEST_RUN_DB", filepath.Join(t.TempDir(), "runs.db"))
	t.Setenv("JFTRADE_EXECUTION_ORDER_DB", filepath.Join(t.TempDir(), "orders.db"))
	t.Setenv("JFTRADE_ADK_SECRETS", filepath.Join(t.TempDir(), "adk-secrets.json"))

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if got := DeriveBacktestDBPath(false); got != backtestOverride {
		t.Fatalf("DeriveBacktestDBPath() = %q", got)
	}
	if got := DeriveStrategyRuntimeDBPath(settingsPath); got != os.Getenv("JFTRADE_STRATEGY_RUNTIME_DB") {
		t.Fatalf("DeriveStrategyRuntimeDBPath() = %q", got)
	}
	if got := DeriveADKDBPath(settingsPath); got != os.Getenv("JFTRADE_ADK_DB") {
		t.Fatalf("DeriveADKDBPath() = %q", got)
	}
	if got := DeriveBacktestRunDBPath(settingsPath); got != os.Getenv("JFTRADE_BACKTEST_RUN_DB") {
		t.Fatalf("DeriveBacktestRunDBPath() = %q", got)
	}
	if got := DeriveExecutionOrderDBPath(settingsPath); got != os.Getenv("JFTRADE_EXECUTION_ORDER_DB") {
		t.Fatalf("DeriveExecutionOrderDBPath() = %q", got)
	}
	if got := DeriveADKSecretsPath(settingsPath); got != os.Getenv("JFTRADE_ADK_SECRETS") {
		t.Fatalf("DeriveADKSecretsPath() = %q", got)
	}
}

func TestRuntimePathDerivationFallsBackForRelativeSettings(t *testing.T) {
	t.Setenv("JFTRADE_BACKTEST_DB", "")
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", "")
	t.Setenv("JFTRADE_BACKTEST_RUN_DB", "")
	t.Setenv("JFTRADE_EXECUTION_ORDER_DB", "")
	t.Setenv("JFTRADE_ADK_DB", "")
	t.Setenv("JFTRADE_ADK_SECRETS", "")

	if got := ResolveLaunchDefaults(false); got.APIBind != DefaultDevelopmentAPIBind || got.BacktestDBPath != filepath.Join(DefaultRuntimeDir, DefaultBacktestDBFilename) {
		t.Fatalf("ResolveLaunchDefaults(false) = %#v", got)
	}
	if got := DeriveBacktestDBPath(false); got != filepath.Join(DefaultRuntimeDir, DefaultBacktestDBFilename) {
		t.Fatalf("DeriveBacktestDBPath fallback = %q", got)
	}
	for name, item := range map[string]struct {
		got  string
		want string
	}{
		"strategy catalog": {DeriveStrategyCatalogPath("settings.json"), defaultStrategyCatalogFilename},
		"strategy plugins": {DeriveStrategyPluginTargetDir("settings.json"), defaultStrategyPluginDirName},
		"strategy runtime": {DeriveStrategyRuntimeDBPath("settings.json"), defaultStrategyRuntimeDBFilename},
		"strategy design":  {DeriveStrategyDesignPath("settings.json"), defaultStrategyDesignFilename},
		"backtest runs":    {DeriveBacktestRunDBPath("settings.json"), defaultBacktestRunDBFilename},
		"execution orders": {DeriveExecutionOrderDBPath("settings.json"), defaultExecutionOrderDBFilename},
		"adk db":           {DeriveADKDBPath("settings.json"), "adk.db"},
		"adk secrets":      {DeriveADKSecretsPath("settings.json"), filepath.Join("secrets", "adk-secrets.json")},
		"adk skills":       {DeriveADKSkillsDir("settings.json"), filepath.Join("adk", "skills")},
		"adk session":      {DeriveADKSessionDBPath("settings.json"), "adk-session.db"},
		"calendar":         {DeriveExchangeCalendarDir("settings.json"), "exchange-calendars"},
	} {
		if item.got != item.want {
			t.Fatalf("%s = %q, want %q", name, item.got, item.want)
		}
	}
}

func TestBindHelpersRejectMalformedAndIPv6Binds(t *testing.T) {
	if got := APIBaseURLForBind("127.0.0.1"); got != "" {
		t.Fatalf("APIBaseURLForBind malformed = %q", got)
	}
	if got := APIBaseURLForBind("[::]:6699"); got != "http://127.0.0.1:6699" {
		t.Fatalf("APIBaseURLForBind IPv6 wildcard = %q", got)
	}
	if got := PortFromBind("127.0.0.1:not-a-port", 3000); got != 3000 {
		t.Fatalf("PortFromBind invalid port = %d", got)
	}
	if got := PortFromBind("127.0.0.1:0", 3000); got != 3000 {
		t.Fatalf("PortFromBind zero port = %d", got)
	}
}

func TestApplyIntegrationEnv(t *testing.T) {
	for _, key := range []string{
		futuOpenDAddrEnv,
		futuOpenDWebSocketKeyEnv,
		jftradeFutuWebSocketKeyEnv,
		jftradeFutuAPIPortEnv,
		jftradeFutuWebSocketPortEnv,
	} {
		t.Setenv(key, "")
	}

	ApplyIntegrationEnv(jfsettings.BrokerIntegration{
		Config: jfsettings.FutuIntegrationConfig{
			Host:          "127.0.0.2",
			APIPort:       22222,
			WebSocketPort: 22223,
			WebSocketKey:  "secret",
		},
	})

	want := map[string]string{
		futuOpenDAddrEnv:            "127.0.0.2:22222",
		futuOpenDWebSocketKeyEnv:    "secret",
		jftradeFutuWebSocketKeyEnv:  "secret",
		jftradeFutuAPIPortEnv:       "22222",
		jftradeFutuWebSocketPortEnv: "22223",
	}
	for key, expected := range want {
		if got := os.Getenv(key); got != expected {
			t.Fatalf("%s = %q, want %q", key, got, expected)
		}
	}
}

func TestIntegrationWithEnvDefaults(t *testing.T) {
	t.Setenv(futuOpenDAddrEnv, "127.0.0.6:26666")
	t.Setenv(jftradeFutuWebSocketPortEnv, "26667")
	t.Setenv(jftradeFutuWebSocketKeyEnv, "runtime-key")

	got := IntegrationWithEnvDefaults(jfsettings.BrokerIntegration{
		Config: jfsettings.FutuIntegrationConfig{
			Host:                    "127.0.0.1",
			APIPort:                 11110,
			WebSocketPort:           11111,
			MaxWebSocketConnections: 20,
		},
	})
	if got.Config.Host != "127.0.0.6" || got.Config.APIPort != 26666 {
		t.Fatalf("OpenD config = %#v", got.Config)
	}
	if got.Config.WebSocketPort != 26667 || got.Config.WebSocketKey != "runtime-key" {
		t.Fatalf("websocket config = %#v", got.Config)
	}
}
