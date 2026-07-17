package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	jadk "github.com/jftrade/jftrade-main/pkg/adk"
)

const coverage98ValidPineScript = `//@version=6
strategy("Coverage strategy", overlay=true)
fast = ta.sma(close, 2)
if close > fast
    strategy.entry("long", strategy.long)`

func TestCoverage98ADKToolFailureContracts(t *testing.T) {
	ctx := t.Context()

	t.Run("market and watchlist handlers reject malformed requests before any remote call", func(t *testing.T) {
		registry := jadk.NewToolRegistry()
		RegisterJFTradeADKTools(nil, registry, ToolDeps{
			MarketCandles: func(context.Context, string, string, string, int) (any, error) {
				t.Fatal("market candle dependency must not be called for invalid inputs")
				return nil, nil
			},
		})
		candles, _ := registry.Get("market.candles")
		if _, err := candles.Handler(ctx, map[string]any{}); err == nil || !strings.Contains(err.Error(), "market and symbol") {
			t.Fatalf("missing candle instrument error = %v", err)
		}
		if _, err := candles.Handler(ctx, map[string]any{"query": "US.AAPL", "period": "not-a-period"}); err == nil {
			t.Fatal("invalid candle period was accepted")
		}

		watchlist, _ := registry.Get("watchlist.list")
		if _, err := watchlist.Handler(ctx, map[string]any{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("unavailable watchlist error = %v", err)
		}
		registry = jadk.NewToolRegistry()
		RegisterJFTradeADKTools(nil, registry, ToolDeps{
			WatchlistList: func(context.Context, WatchlistListInput) (any, error) {
				t.Fatal("watchlist dependency must not be called for an invalid page limit")
				return nil, nil
			},
		})
		watchlist, _ = registry.Get("watchlist.list")
		if _, err := watchlist.Handler(ctx, map[string]any{"limit": 0}); err == nil || !strings.Contains(err.Error(), "between 1 and 200") {
			t.Fatalf("watchlist limit error = %v", err)
		}
	})

	t.Run("research and optimization stop on data-readiness or queue errors", func(t *testing.T) {
		registry := jadk.NewToolRegistry()
		registerADKStrategyResearchTools(registry, ToolDeps{
			EnsureResearchBacktestData: func(ResearchBacktestInput) (BacktestDataReadiness, error) {
				return BacktestDataReadiness{}, errors.New("research data source unavailable")
			},
			StartResearchBacktest: func(ResearchBacktestInput) (BacktestRunSummary, error) {
				t.Fatal("research queue must not be called when readiness fails")
				return BacktestRunSummary{}, nil
			},
		})
		research, _ := registry.Get("strategy.research_backtest")
		if _, err := research.Handler(ctx, map[string]any{"script": coverage98ValidPineScript}); err == nil || !strings.Contains(err.Error(), "research data source unavailable") {
			t.Fatalf("research readiness error = %v", err)
		}

		registry = jadk.NewToolRegistry()
		registerADKStrategyResearchTools(registry, ToolDeps{
			StartResearchBacktest: func(ResearchBacktestInput) (BacktestRunSummary, error) {
				return BacktestRunSummary{}, errors.New("research queue unavailable")
			},
		})
		research, _ = registry.Get("strategy.research_backtest")
		if _, err := research.Handler(ctx, map[string]any{"script": coverage98ValidPineScript}); err == nil || !strings.Contains(err.Error(), "research queue unavailable") {
			t.Fatalf("research queue error = %v", err)
		}

		store, err := jadk.NewStore(filepath.Join(t.TempDir(), "adk.db"), filepath.Join(t.TempDir(), "secrets"), filepath.Join(t.TempDir(), "skills"))
		if err != nil {
			t.Fatalf("NewStore: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		registry = jadk.NewToolRegistry()
		registerADKStrategyOptimizationTools(store, registry, ToolDeps{
			EnsureBacktestData: func([]string, BacktestStartInput) (BacktestDataReadiness, error) {
				return BacktestDataReadiness{}, errors.New("optimization data source unavailable")
			},
		})
		optimize, _ := registry.Get("strategy.optimize")
		if _, err := optimize.Handler(ctx, map[string]any{"definitionId": "strategy-1"}); err == nil || !strings.Contains(err.Error(), "optimization data source unavailable") {
			t.Fatalf("optimization readiness error = %v", err)
		}
	})

	t.Run("closed ADK storage is surfaced by task and memory tools", func(t *testing.T) {
		store, err := jadk.NewStore(filepath.Join(t.TempDir(), "adk.db"), filepath.Join(t.TempDir(), "secrets"), filepath.Join(t.TempDir(), "skills"))
		if err != nil {
			t.Fatalf("NewStore: %v", err)
		}
		registry := jadk.NewToolRegistry()
		RegisterJFTradeADKTools(store, registry, ToolDeps{})
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close: %v", err)
		}

		for _, request := range []struct {
			name  string
			input map[string]any
		}{
			{name: "tasks.list", input: map[string]any{}},
			{name: "tasks.delete", input: map[string]any{"id": "missing-task"}},
			{name: "memory.list", input: map[string]any{}},
			{name: "memory.forget", input: map[string]any{"id": "missing-memory"}},
		} {
			t.Run(request.name, func(t *testing.T) {
				tool, ok := registry.Get(request.name)
				if !ok {
					t.Fatalf("tool %q missing", request.name)
				}
				if _, err := tool.Handler(ctx, request.input); err == nil {
					t.Fatalf("%s hid a closed-store failure", request.name)
				}
			})
		}
	})
}
