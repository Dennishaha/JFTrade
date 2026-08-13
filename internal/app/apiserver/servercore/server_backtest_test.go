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
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
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
	if _, err := server.stores.Design.SaveDefinition(stratsrv.Definition{
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
	seedServerBacktestCoverage(t, dbPath, "extended", time.Date(2026, time.March, 8, 5, 0, 0, 0, time.UTC), time.Date(2026, time.March, 9, 3, 59, 59, 999999999, time.UTC))

	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body, jftradeErr1 := json.Marshal(map[string]any{
		"definitionId":     "dsl-market-code-route",
		"market":           "US",
		"code":             "AAPL",
		"interval":         "1m",
		"startDate":        "2026-03-08",
		"endDate":          "2026-03-08",
		"initialBalance":   10000,
		"rehabType":        "forward",
		"useExtendedHours": true,
	})
	jftradeCheckTestError(t, jftradeErr1)
	createResp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/backtests", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST backtest: %v", err)
	}
	defer func() { jftradeCheckTestError(t, createResp.Body.Close()) }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST backtest status = %d", createResp.StatusCode)
	}

	runs := server.stores.BacktestRuns.List()
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
	if runs[0].Request.StartDate != "2026-03-08" || runs[0].Request.EndDate != "2026-03-08" {
		t.Fatalf("market date labels were not persisted: %+v", runs[0].Request)
	}
	if runs[0].Request.MarketTimezone != "America/New_York" {
		t.Fatalf("market timezone = %q, want America/New_York", runs[0].Request.MarketTimezone)
	}
	if runs[0].Request.StartTime != "2026-03-08T05:00:00Z" || runs[0].Request.EndTime != "2026-03-09T03:59:59.999999999Z" {
		t.Fatalf("DST-normalized UTC range = %s..%s", runs[0].Request.StartTime, runs[0].Request.EndTime)
	}
}

func TestEnqueueBacktestUsesPineInitialCapitalWhenRequestOmitsBalance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "initial-capital.db")
	t.Setenv("JFTRADE_BACKTEST_DB", dbPath)
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if _, err := server.stores.Design.SaveDefinition(stratsrv.Definition{
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
	startTime, endTime := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC), time.Date(2026, time.May, 26, 9, 31, 0, 0, time.UTC)
	seedServerBacktestCoverage(t, dbPath, "regular", startTime, endTime)
	run, err := server.backtestSvc.Start(t.Context(), btsrv.StartRequest{
		DefinitionID: "pine-initial-capital",
		Symbol:       "US.AAPL",
		Interval:     "1m",
		StartTime:    startTime.Format(time.RFC3339),
		EndTime:      endTime.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("backtestSvc.Start: %v", err)
	}
	server.backtestSvc.Cancel(run.ID)
	if run.Request.InitialBalance != 250000 {
		t.Fatalf("initialBalance = %v, want 250000", run.Request.InitialBalance)
	}
}

func seedServerBacktestCoverage(t *testing.T, dbPath, sessionScope string, startTime, endTime time.Time) {
	store := openServerKLineSeedStore(t, dbPath)
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	store.SetWriteSessionScope(sessionScope)
	row := func(at time.Time) bbgotypes.KLine {
		return bbgotypes.KLine{StartTime: bbgotypes.Time(at), EndTime: bbgotypes.Time(at.Add(time.Minute - time.Millisecond)), Interval: bbgotypes.Interval1m, Symbol: "US.AAPL", Open: fixedpoint.NewFromInt(100), High: fixedpoint.NewFromInt(101), Low: fixedpoint.NewFromInt(99), Close: fixedpoint.NewFromInt(100), Volume: fixedpoint.NewFromInt(1000)}
	}
	if err := store.InsertKLines([]bbgotypes.KLine{row(startTime), row(endTime.Truncate(time.Minute))}, "forward"); err != nil {
		t.Fatalf("InsertKLines: %v", err)
	}
}
