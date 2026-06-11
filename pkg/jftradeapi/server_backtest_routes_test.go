package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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
strategy("Pine Market Code Route", overlay=true)
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
}
