package backtest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestRunWithPineWorkerRejectsUnavailableRuntimeAndData(t *testing.T) {
	base := RunConfig{
		DBPath: filepath.Join(t.TempDir(), "missing.db"), Symbol: "US.AAPL", Interval: "1m",
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StrategyScript: `//@version=6
strategy("Boundary")`,
		StartTime: time.Date(2026, time.July, 1, 13, 30, 0, 0, time.UTC),
		EndTime:   time.Date(2026, time.July, 1, 13, 31, 0, 0, time.UTC),
	}
	if result := RunWithPineWorker(context.Background(), base, nil); result == nil || result.Error != "pine worker runner is required" {
		t.Fatalf("nil runner result = %#v", result)
	}
	if result := RunWithPineWorker(context.Background(), base, &fakePineWorkerBacktestRunner{}); result == nil || !strings.Contains(result.Error, "backtest database not found") {
		t.Fatalf("missing database result = %#v", result)
	}

	directoryPath := t.TempDir()
	base.DBPath = directoryPath
	if result := RunWithPineWorker(context.Background(), base, &fakePineWorkerBacktestRunner{}); result == nil || !strings.Contains(result.Error, "open backtest store") {
		t.Fatalf("directory database result = %#v", result)
	}
}

func TestRunWithPineWorkerValidatesSourceAndCoverageBeforeReplay(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backtest.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close empty K-line store: %v", err)
	}
	base := RunConfig{
		DBPath: dbPath, Symbol: "US.AAPL", Interval: "1m",
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StrategyScript: `//@version=6
strategy("Boundary")`,
		StartTime: time.Date(2026, time.July, 1, 13, 30, 0, 0, time.UTC),
		EndTime:   time.Date(2026, time.July, 1, 13, 31, 0, 0, time.UTC),
	}
	runner := &fakePineWorkerBacktestRunner{}

	unsupported := base
	unsupported.SourceFormat = "javascript"
	if result := RunWithPineWorker(context.Background(), unsupported, runner); result == nil || !strings.Contains(result.Error, "unsupported strategy source format") {
		t.Fatalf("unsupported source result = %#v", result)
	}

	invalidScript := base
	invalidScript.StrategyScript = `//@version=6
strategy("broken"
if`
	if result := RunWithPineWorker(context.Background(), invalidScript, runner); result == nil || !strings.Contains(result.Error, "compile pine strategy metadata") {
		t.Fatalf("invalid script result = %#v", result)
	}

	if result := RunWithPineWorker(context.Background(), base, runner); result == nil || !strings.Contains(strings.ToLower(result.Error), "missing k-line coverage") {
		t.Fatalf("missing coverage result = %#v", result)
	}
}

func TestResolveBacktestQuoteCurrencyDefaultsByMarket(t *testing.T) {
	cases := map[string]string{
		"US.AAPL":   "USD",
		"SH.600000": "CNY",
		"SZ.000001": "CNY",
		"CN.000001": "CNY",
		"HK.00700":  "HKD",
	}
	for symbol, want := range cases {
		if got := resolveBacktestQuoteCurrency(symbol, ""); got != want {
			t.Fatalf("resolveBacktestQuoteCurrency(%q) = %q, want %q", symbol, got, want)
		}
	}
	if got := resolveBacktestQuoteCurrency("US.AAPL", "EUR"); got != "EUR" {
		t.Fatalf("requested quote currency = %q", got)
	}
}

func TestRemoveFutuMarketCacheToleratesMissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	removeFutuMarketCache()
	if _, err := os.Stat(filepath.Join(home, ".bbgo", "cache", "futu-markets.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected market cache state: %v", err)
	}
}
