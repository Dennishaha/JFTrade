package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultRealTradeControlFilename = "real-trade-control.json"

type ResourceDescriptor struct {
	ID                  string `json:"id"`
	Owner               string `json:"owner"`
	Kind                string `json:"kind"`
	Path                string `json:"path"`
	InitializedBy       string `json:"initializedBy"`
	SchemaOwner         string `json:"schemaOwner"`
	CloseOwner          string `json:"closeOwner"`
	HealthProvider      string `json:"healthProvider"`
	EnvironmentOverride string `json:"environmentOverride,omitempty"`
	Critical            bool   `json:"critical"`
}

func RuntimeResources(settingsPath string, backtestDBPath string) []ResourceDescriptor {
	settingsPath = strings.TrimSpace(settingsPath)
	backtestDBPath = strings.TrimSpace(backtestDBPath)
	return []ResourceDescriptor{
		{
			ID: "settings-file", Owner: "settings", Kind: "json-file", Path: settingsPath,
			InitializedBy: "settings module", SchemaOwner: "settings store", CloseOwner: "n/a",
			HealthProvider: "system.status.persistence", Critical: true,
		},
		{
			ID: "backtest-kline-db", Owner: "backtest", Kind: "sqlite", Path: backtestDBPath,
			InitializedBy: "backtest module", SchemaOwner: "pkg/backtest storage", CloseOwner: "backtest module",
			HealthProvider: "data-migration/backtest", EnvironmentOverride: "JFTRADE_BACKTEST_DB", Critical: true,
		},
		{
			ID: "backtest-run-db", Owner: "backtest", Kind: "sqlite", Path: DeriveBacktestRunDBPath(settingsPath),
			InitializedBy: "backtest module", SchemaOwner: "backtest run store", CloseOwner: "backtest module",
			HealthProvider: "data-migration/backtest-runs", EnvironmentOverride: "JFTRADE_BACKTEST_RUN_DB", Critical: true,
		},
		{
			ID: "strategy-catalog", Owner: "strategy", Kind: "sqlite", Path: DeriveStrategyRuntimeDBPath(settingsPath),
			InitializedBy: "strategy module", SchemaOwner: "strategy catalog tables", CloseOwner: "strategy module",
			HealthProvider: "data-migration/strategy", EnvironmentOverride: "JFTRADE_STRATEGY_RUNTIME_DB", Critical: true,
		},
		{
			ID: "strategy-designs", Owner: "strategy", Kind: "sqlite", Path: DeriveStrategyRuntimeDBPath(settingsPath),
			InitializedBy: "strategy module", SchemaOwner: "strategy design tables", CloseOwner: "strategy module",
			HealthProvider: "data-migration/strategy", EnvironmentOverride: "JFTRADE_STRATEGY_RUNTIME_DB", Critical: true,
		},
		{
			ID: "strategy-runtime-db", Owner: "strategy", Kind: "sqlite", Path: DeriveStrategyRuntimeDBPath(settingsPath),
			InitializedBy: "strategy module", SchemaOwner: "strategy runtime store", CloseOwner: "strategy module",
			HealthProvider: "data-migration/strategy", EnvironmentOverride: "JFTRADE_STRATEGY_RUNTIME_DB", Critical: true,
		},
		{
			ID: "execution-orders-db", Owner: "trading", Kind: "sqlite", Path: DeriveExecutionOrderDBPath(settingsPath),
			InitializedBy: "trading module", SchemaOwner: "execution order store", CloseOwner: "trading module",
			HealthProvider: "data-migration/execution", EnvironmentOverride: "JFTRADE_EXECUTION_ORDER_DB", Critical: true,
		},
		{
			ID: "real-trade-control", Owner: "trading", Kind: "json-file", Path: deriveRealTradeControlPath(settingsPath),
			InitializedBy: "trading risk module", SchemaOwner: "real-trade control plane", CloseOwner: "n/a",
			HealthProvider: "system.real-trade-risk", EnvironmentOverride: "JFTRADE_REAL_TRADE_CONTROL_PATH", Critical: true,
		},
		{
			ID: "adk-db", Owner: "assistant/runtime", Kind: "sqlite", Path: DeriveADKDBPath(settingsPath),
			InitializedBy: "assistant runtime", SchemaOwner: "adk store", CloseOwner: "assistant runtime",
			HealthProvider: "system.runtime-dependencies/adk", EnvironmentOverride: "JFTRADE_ADK_DB", Critical: false,
		},
		{
			ID: "adk-session-db", Owner: "assistant/runtime", Kind: "sqlite", Path: DeriveADKSessionDBPath(settingsPath),
			InitializedBy: "assistant runtime", SchemaOwner: "adk session store", CloseOwner: "assistant runtime",
			HealthProvider: "system.runtime-dependencies/adk", EnvironmentOverride: "JFTRADE_ADK_SESSION_DB", Critical: false,
		},
		{
			ID: "adk-secrets", Owner: "assistant/admin", Kind: "json-file", Path: DeriveADKSecretsPath(settingsPath),
			InitializedBy: "assistant admin", SchemaOwner: "adk secrets store", CloseOwner: "n/a",
			HealthProvider: "system.runtime-dependencies/adk", EnvironmentOverride: "JFTRADE_ADK_SECRETS", Critical: false,
		},
		{
			ID: "adk-skills-dir", Owner: "assistant/admin", Kind: "directory", Path: DeriveADKSkillsDir(settingsPath),
			InitializedBy: "assistant admin", SchemaOwner: "filesystem", CloseOwner: "n/a",
			HealthProvider: "system.runtime-dependencies/adk", EnvironmentOverride: "JFTRADE_ADK_SKILLS_DIR", Critical: false,
		},
		{
			ID: "exchange-calendar-dir", Owner: "system/exchange-calendar", Kind: "directory", Path: DeriveExchangeCalendarDir(settingsPath),
			InitializedBy: "exchange calendar module", SchemaOwner: "exchange calendar store", CloseOwner: "n/a",
			HealthProvider: "system.exchange-calendars", EnvironmentOverride: "JFTRADE_EXCHANGE_CALENDAR_DIR", Critical: false,
		},
		{
			ID: "strategy-plugin-dir", Owner: "strategy", Kind: "directory", Path: DeriveStrategyPluginTargetDir(settingsPath),
			InitializedBy: "strategy module", SchemaOwner: "filesystem", CloseOwner: "n/a",
			HealthProvider: "strategy plugin catalog", Critical: false,
		},
	}
}

func RuntimeResourceSummary(settingsPath string, backtestDBPath string) map[string]any {
	resources := RuntimeResources(settingsPath, backtestDBPath)
	return map[string]any{
		"checkedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"count":     len(resources),
		"items":     resources,
	}
}

func deriveRealTradeControlPath(settingsPath string) string {
	if envPath := strings.TrimSpace(os.Getenv("JFTRADE_REAL_TRADE_CONTROL_PATH")); envPath != "" {
		return envPath
	}
	directory := filepath.Dir(strings.TrimSpace(settingsPath))
	if directory == "" || directory == "." {
		return defaultRealTradeControlFilename
	}
	return filepath.Join(directory, defaultRealTradeControlFilename)
}
