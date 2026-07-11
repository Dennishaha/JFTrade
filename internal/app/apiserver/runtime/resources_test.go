package runtime

import (
	"path/filepath"
	"testing"
)

func TestRuntimeResourcesDeclareOwnersAndDerivedPaths(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "runtime", "settings.json")
	backtestDBPath := filepath.Join(root, "runtime", "backtest.db")

	resources := RuntimeResources(settingsPath, backtestDBPath)
	if len(resources) == 0 {
		t.Fatal("RuntimeResources returned no resources")
	}
	byID := make(map[string]ResourceDescriptor, len(resources))
	for _, resource := range resources {
		if resource.ID == "" || resource.Owner == "" || resource.Kind == "" || resource.Path == "" {
			t.Fatalf("resource missing required metadata: %+v", resource)
		}
		byID[resource.ID] = resource
	}

	if got := byID["settings-file"]; got.Owner != "settings" || got.Path != settingsPath || !got.Critical {
		t.Fatalf("settings-file = %+v", got)
	}
	if got := byID["backtest-kline-db"]; got.Owner != "backtest" || got.Path != backtestDBPath || got.EnvironmentOverride != "JFTRADE_BACKTEST_DB" {
		t.Fatalf("backtest-kline-db = %+v", got)
	}
	if got := byID["execution-orders-db"]; got.Owner != "trading" || got.Path != filepath.Join(filepath.Dir(settingsPath), "execution-orders.db") {
		t.Fatalf("execution-orders-db = %+v", got)
	}
	if got := byID["strategy-catalog"]; got.Owner != "strategy" || got.Kind != "sqlite" || got.Path != filepath.Join(filepath.Dir(settingsPath), "strategy-runtime.db") || got.EnvironmentOverride != "JFTRADE_STRATEGY_RUNTIME_DB" {
		t.Fatalf("strategy-catalog = %+v", got)
	}
	if got := byID["strategy-designs"]; got.Owner != "strategy" || got.Kind != "sqlite" || got.Path != filepath.Join(filepath.Dir(settingsPath), "strategy-runtime.db") || got.EnvironmentOverride != "JFTRADE_STRATEGY_RUNTIME_DB" {
		t.Fatalf("strategy-designs = %+v", got)
	}
	if got := byID["adk-session-db"]; got.Owner != "assistant/runtime" || got.EnvironmentOverride != "JFTRADE_ADK_SESSION_DB" {
		t.Fatalf("adk-session-db = %+v", got)
	}
	if got := byID["watchlist-db"]; got.Owner != "watchlist" || got.Path != filepath.Join(filepath.Dir(settingsPath), "watchlists.db") || got.EnvironmentOverride != "JFTRADE_WATCHLIST_DB" {
		t.Fatalf("watchlist-db = %+v", got)
	}
	if got := byID["real-trade-control"]; got.Owner != "trading" || got.Path != filepath.Join(filepath.Dir(settingsPath), "real-trade-control.json") || got.EnvironmentOverride != "JFTRADE_REAL_TRADE_CONTROL_PATH" {
		t.Fatalf("real-trade-control = %+v", got)
	}
}

func TestRuntimeResourcesHonorRealTradeControlPathOverride(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "runtime", "settings.json")
	backtestDBPath := filepath.Join(root, "runtime", "backtest.db")
	overridePath := filepath.Join(root, "controls", "real-trade-control.override.json")
	t.Setenv("JFTRADE_REAL_TRADE_CONTROL_PATH", overridePath)

	resources := RuntimeResources(settingsPath, backtestDBPath)
	for _, resource := range resources {
		if resource.ID != "real-trade-control" {
			continue
		}
		if resource.Path != overridePath {
			t.Fatalf("real-trade-control override path = %q, want %q", resource.Path, overridePath)
		}
		return
	}
	t.Fatal("real-trade-control resource missing")
}

func TestRuntimeResourceSummaryIncludesCountAndItems(t *testing.T) {
	summary := RuntimeResourceSummary("/tmp/jftrade/settings.json", "/tmp/jftrade/backtest.db")
	if summary["checkedAt"] == "" {
		t.Fatalf("summary checkedAt empty: %#v", summary)
	}
	items, ok := summary["items"].([]ResourceDescriptor)
	if !ok || len(items) == 0 || summary["count"] != len(items) {
		t.Fatalf("summary items/count = %#v", summary)
	}
}
