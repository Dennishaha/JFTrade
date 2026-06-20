package servercore

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestBacktestRunStoreGetReturnsDeepCopy(t *testing.T) {
	store := newBacktestRunStore()
	original := &backtestRunState{
		ID:     "bt-1",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID: "def-1",
			Symbol:       "HK.00700",
		},
		Result: &backtest.RunResult{
			Symbol:          "HK.00700",
			Interval:        "1m",
			FinalBalance:    123456,
			MaxDrawdown:     0.12,
			CurrentDrawdown: 0.03,
			Trades:          []backtest.TradeEvent{{Time: "2026-01-02T00:00:00Z", Side: "BUY", Price: "100", Qty: "1"}},
			OrderBook:       []backtest.OrderBookEntry{{OrderID: "1", Side: "BUY", Quantity: "1", Status: "FILLED", FilledPrice: "100"}},
			PnLCurve:        []backtest.PnLPoint{{Time: "2026-01-02T00:00:00Z", Equity: 100000}},
			DrawdownCurve:   []backtest.DrawdownPoint{{Time: "2026-01-02T00:00:00Z", Drawdown: 0.12}},
			Candles:         []backtest.Candle{{Time: "2026-01-02T00:00:00Z", Open: "100", High: "101", Low: "99", Close: "100.5", Volume: "10"}},
			Logs:            []string{"warmup complete"},
			RuntimeErrors:   []string{"risk warning"},
		},
	}
	if err := store.add(original); err != nil {
		t.Fatalf("add: %v", err)
	}

	snapshot, ok := store.get(original.ID)
	if !ok {
		t.Fatal("expected run snapshot")
	}

	snapshot.Status = "failed"
	snapshot.Request.Symbol = "US.TSLA"
	snapshot.Result.FinalBalance = 42
	snapshot.Result.MaxDrawdown = 0.5
	snapshot.Result.CurrentDrawdown = 0.4
	snapshot.Result.Trades[0].Price = "999"
	snapshot.Result.OrderBook[0].FilledPrice = "77"
	snapshot.Result.PnLCurve[0].Equity = 12
	snapshot.Result.DrawdownCurve[0].Drawdown = 0.8
	snapshot.Result.Candles[0].Close = "1"
	snapshot.Result.Logs[0] = "changed"
	snapshot.Result.RuntimeErrors[0] = "changed"

	if original.Status != "completed" {
		t.Fatalf("original status mutated: %s", original.Status)
	}
	if original.Request.Symbol != "HK.00700" {
		t.Fatalf("original request symbol mutated: %s", original.Request.Symbol)
	}
	if original.Result.FinalBalance != 123456 {
		t.Fatalf("original final balance mutated: %f", original.Result.FinalBalance)
	}
	if original.Result.MaxDrawdown != 0.12 {
		t.Fatalf("original max drawdown mutated: %f", original.Result.MaxDrawdown)
	}
	if original.Result.CurrentDrawdown != 0.03 {
		t.Fatalf("original current drawdown mutated: %f", original.Result.CurrentDrawdown)
	}
	if original.Result.Trades[0].Price != "100" {
		t.Fatalf("original trade mutated: %s", original.Result.Trades[0].Price)
	}
	if original.Result.OrderBook[0].FilledPrice != "100" {
		t.Fatalf("original order book mutated: %s", original.Result.OrderBook[0].FilledPrice)
	}
	if original.Result.PnLCurve[0].Equity != 100000 {
		t.Fatalf("original pnl point mutated: %f", original.Result.PnLCurve[0].Equity)
	}
	if original.Result.DrawdownCurve[0].Drawdown != 0.12 {
		t.Fatalf("original drawdown point mutated: %f", original.Result.DrawdownCurve[0].Drawdown)
	}
	if original.Result.Candles[0].Close != "100.5" {
		t.Fatalf("original candle mutated: %s", original.Result.Candles[0].Close)
	}
	if original.Result.Logs[0] != "warmup complete" {
		t.Fatalf("original logs mutated: %s", original.Result.Logs[0])
	}
	if original.Result.RuntimeErrors[0] != "risk warning" {
		t.Fatalf("original runtime errors mutated: %s", original.Result.RuntimeErrors[0])
	}
}

func TestBacktestRunStateFromRowDoesNotInferMissingDateMetadata(t *testing.T) {
	run, err := backtestRunStateFromRow(backtestRunStateRow{
		ID:          "bt-without-date-metadata",
		Status:      "completed",
		RequestJSON: `{"symbol":"US.AAPL","startTime":"2026-05-01T23:30:00-05:00","endTime":"2026-05-02T23:30:00-05:00"}`,
	})
	if err != nil {
		t.Fatalf("backtestRunStateFromRow: %v", err)
	}
	if run.Request.StartDate != "" || run.Request.EndDate != "" || run.Request.MarketTimezone != "" {
		t.Fatalf("missing date metadata was inferred: %+v", run.Request)
	}
	if run.Request.StartTime != "2026-05-01T23:30:00-05:00" || run.Request.EndTime != "2026-05-02T23:30:00-05:00" {
		t.Fatalf("stored timestamps were rewritten: %+v", run.Request)
	}
}

func TestBacktestRunStoreListReturnsIndependentSnapshots(t *testing.T) {
	store := newBacktestRunStore()
	original := &backtestRunState{
		ID:     "bt-2",
		Status: "queued",
		Result: &backtest.RunResult{
			Logs: []string{"queued"},
		},
	}
	if err := store.add(original); err != nil {
		t.Fatalf("add: %v", err)
	}

	runs := store.list()
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	runs[0].Status = "running"
	runs[0].Result.Logs[0] = "mutated"

	fresh, ok := store.get(original.ID)
	if !ok {
		t.Fatal("expected run snapshot")
	}
	if fresh.Status != "queued" {
		t.Fatalf("store status mutated through list snapshot: %s", fresh.Status)
	}
	if fresh.Result.Logs[0] != "queued" {
		t.Fatalf("store logs mutated through list snapshot: %s", fresh.Result.Logs[0])
	}
}

func TestBacktestRunStorePersistsAndRecoversTransientRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backtest-runs.db")
	store, err := newBacktestRunStoreWithDB(dbPath)
	if err != nil {
		t.Fatalf("newBacktestRunStoreWithDB: %v", err)
	}
	t.Cleanup(func() { jftradeErr1 := store.Close(); jftradeCheckTestError(t, jftradeErr1) })

	completedRun := &backtestRunState{
		ID:     "bt-persist-completed",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:   "def-completed",
			Symbol:         "US.AAPL",
			Interval:       "5m",
			StartDate:      "2026-05-01",
			EndDate:        "2026-05-02",
			StartTime:      "2026-05-01T04:00:00Z",
			EndTime:        "2026-05-03T03:59:59.999999999Z",
			MarketTimezone: "America/New_York",
		},
		Result: &backtest.RunResult{
			Symbol:       "US.AAPL",
			Interval:     "5m",
			StartTime:    "2026-05-01T00:00:00Z",
			EndTime:      "2026-05-02T00:00:00Z",
			FinalBalance: 123456,
		},
		CreatedAt: "2026-05-30T00:00:00Z",
		UpdatedAt: "2026-05-30T00:00:01Z",
	}
	runningRun := &backtestRunState{
		ID:     "bt-persist-running",
		Status: "running",
		Request: backtestStartRequest{
			DefinitionID: "def-running",
			Symbol:       "US.TSLA",
			Interval:     "1m",
			StartTime:    "2026-05-03T00:00:00Z",
			EndTime:      "2026-05-04T00:00:00Z",
		},
		CreatedAt: "2026-05-30T00:00:02Z",
		UpdatedAt: "2026-05-30T00:00:03Z",
	}
	if err := store.add(completedRun); err != nil {
		t.Fatalf("add completed run: %v", err)
	}
	if err := store.add(runningRun); err != nil {
		t.Fatalf("add running run: %v", err)
	}

	// Close the first store so the reopened connection can acquire the file on Windows.
	if err := store.Close(); err != nil {
		t.Fatalf("close store before reload: %v", err)
	}

	reloadedStore, err := newBacktestRunStoreWithDB(dbPath)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	t.Cleanup(func() { jftradeErr3 := reloadedStore.Close(); jftradeCheckTestError(t, jftradeErr3) })

	reloadedCompleted, ok := reloadedStore.get(completedRun.ID)
	if !ok {
		t.Fatal("expected completed run after reload")
	}
	if reloadedCompleted.Status != "completed" {
		t.Fatalf("completed run status = %s, want completed", reloadedCompleted.Status)
	}
	if reloadedCompleted.Request.StartDate != completedRun.Request.StartDate || reloadedCompleted.Request.EndDate != completedRun.Request.EndDate {
		t.Fatalf("market date labels changed after reload: %q..%q", reloadedCompleted.Request.StartDate, reloadedCompleted.Request.EndDate)
	}
	if reloadedCompleted.Request.StartTime != completedRun.Request.StartTime || reloadedCompleted.Request.EndTime != completedRun.Request.EndTime {
		t.Fatalf("UTC range changed after reload: %s..%s", reloadedCompleted.Request.StartTime, reloadedCompleted.Request.EndTime)
	}
	if reloadedCompleted.Request.MarketTimezone != completedRun.Request.MarketTimezone {
		t.Fatalf("market timezone changed after reload: %q", reloadedCompleted.Request.MarketTimezone)
	}
	if reloadedCompleted.Result != nil {
		t.Fatalf("lightweight completed run should not load full result: %+v", reloadedCompleted.Result)
	}
	reloadedCompletedFull, ok, err := reloadedStore.getFull(completedRun.ID)
	if err != nil {
		t.Fatalf("getFull completed run: %v", err)
	}
	if reloadedCompletedFull == nil {
		t.Fatal("expected completed full run after reload")
	}
	if !ok || reloadedCompletedFull.Result == nil || reloadedCompletedFull.Result.FinalBalance != 123456 {
		t.Fatalf("completed run full result lost after reload: %+v", reloadedCompletedFull)
	}

	reloadedRunning, ok := reloadedStore.get(runningRun.ID)
	if !ok {
		t.Fatal("expected recovered running run after reload")
	}
	if reloadedRunning.Status != "failed" {
		t.Fatalf("recovered running run status = %s, want failed", reloadedRunning.Status)
	}
	if reloadedRunning.Result == nil || !strings.Contains(reloadedRunning.Result.Error, recoveredBacktestRunErrorText) {
		t.Fatalf("expected recovered running run error, got %+v", reloadedRunning.Result)
	}

	if _, deleted, err := reloadedStore.delete(completedRun.ID); err != nil {
		t.Fatalf("delete completed run: %v", err)
	} else if !deleted {
		t.Fatal("expected completed run delete to succeed")
	}

	// Close before reopening so the next connection can acquire the file on Windows.
	if err := reloadedStore.Close(); err != nil {
		t.Fatalf("close reloadedStore before second reload: %v", err)
	}

	reloadedAgain, err := newBacktestRunStoreWithDB(dbPath)
	if err != nil {
		t.Fatalf("reload store after delete: %v", err)
	}
	t.Cleanup(func() { jftradeErr2 := reloadedAgain.Close(); jftradeCheckTestError(t, jftradeErr2) })
	if _, ok := reloadedAgain.get(completedRun.ID); ok {
		t.Fatal("expected deleted completed run to stay deleted after reload")
	}
}
