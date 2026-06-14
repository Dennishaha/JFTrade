package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestBacktestRouteAcceptsExplicitMarketAndCode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-route-market-code.db")
	t.Setenv("JFTRADE_BACKTEST_DB", dbPath)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "dsl-market-code-route",
		Name:         "Pine Market Code Route",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "1m",
		Script: `//@version=6
strategy("Pine Market Code Route", overlay=true, initial_capital=25000)
strategy.entry("Long", strategy.long, qty=1)`,
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{
		"definitionId":     "dsl-market-code-route",
		"market":           "US",
		"code":             "AAPL",
		"interval":         "1m",
		"startTime":        time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC).Format(time.RFC3339),
		"endTime":          time.Date(2026, time.May, 26, 9, 31, 0, 0, time.UTC).Format(time.RFC3339),
		"initialBalance":   10000,
		"rehabType":        "forward",
		"useExtendedHours": true,
	})
	createResp, err := http.Post(srv.URL+"/api/v1/backtests", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST backtest: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST backtest status = %d", createResp.StatusCode)
	}

	runs := server.backtestRuns.list()
	if len(runs) != 1 {
		t.Fatalf("expected 1 backtest run, got %+v", runs)
	}
	if runs[0].Request.Market != "US" || runs[0].Request.Code != "AAPL" || runs[0].Request.Symbol != "US.AAPL" {
		t.Fatalf("unexpected normalized backtest request: %+v", runs[0].Request)
	}
	if runs[0].Request.UseExtendedHours == nil || !*runs[0].Request.UseExtendedHours {
		t.Fatalf("expected useExtendedHours to be preserved: %+v", runs[0].Request)
	}
	if runs[0].Request.DefinitionVersion != "0.1.0" {
		t.Fatalf("expected definitionVersion to be snapshotted: %+v", runs[0].Request)
	}
	if runs[0].Request.InitialBalance != 10000 {
		t.Fatalf("explicit initialBalance = %v, want 10000", runs[0].Request.InitialBalance)
	}
}

func TestEnqueueBacktestUsesPineInitialCapitalWhenRequestOmitsBalance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(t.TempDir(), "missing.db"))
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.designStore.saveDefinition(strategyDesignDefinition{
		ID:           "pine-initial-capital",
		Name:         "Pine Initial Capital",
		Version:      "0.1.0",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "1m",
		Script: `//@version=6
strategy("Pine Initial Capital", initial_capital=250000)
log.info("ready")`,
	}); err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	run, err := server.backtestSvc.Start(t.Context(), btsrv.StartRequest{
		DefinitionID: "pine-initial-capital",
		Symbol:       "US.AAPL",
		Interval:     "1m",
		StartTime:    time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC).Format(time.RFC3339),
		EndTime:      time.Date(2026, time.May, 26, 9, 31, 0, 0, time.UTC).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("backtestSvc.Start: %v", err)
	}
	server.backtestSvc.Cancel(run.ID)
	if run.Request.InitialBalance != 250000 {
		t.Fatalf("initialBalance = %v, want 250000", run.Request.InitialBalance)
	}
}
