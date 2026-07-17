package backtest

import (
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestCoverage98ResultViewExposesWarningsAndFiltersChartSeries(t *testing.T) {
	runs := newMemoryRunStore()
	run := &RunState{
		ID: "coverage98-result-view", Status: "completed",
		Request: StartRequest{Symbol: "US.AAPL", Interval: "1m", InitialBalance: 1000},
		Result: &bt.RunResult{
			Candles: []bt.Candle{
				{Time: "2024-01-02T00:00:00Z", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "10"},
				{Time: "2024-01-02T00:01:00Z", Open: "10.5", High: "12", Low: "10", Close: "11.5", Volume: "12"},
				{Time: "2024-01-02T00:03:00Z", Open: "11.5", High: "13", Low: "11", Close: "12.5", Volume: "15"},
			},
			Trades: []bt.TradeEvent{
				{Time: "2024-01-02T00:00:00Z", Side: "BUY"},
				{Time: "2024-01-02T00:01:00Z", Side: "SELL"},
				{Time: "2024-01-02T00:03:00Z", Side: "BUY"},
			},
			PnLCurve: []bt.PnLPoint{
				{Time: "2024-01-02T00:00:00Z", Equity: 1000},
				{Time: "2024-01-02T00:01:00Z", Equity: 1010},
				{Time: "2024-01-02T00:03:00Z", Equity: 1020},
			},
			DrawdownCurve: []bt.DrawdownPoint{
				{Time: "2024-01-02T00:00:00Z", Drawdown: 0.01},
				{Time: "2024-01-02T00:01:00Z", Drawdown: 0.005},
				{Time: "2024-01-02T00:03:00Z", Drawdown: 0},
			},
			Warnings: []string{"stale quote", "rounded quantity"},
		},
	}
	if err := runs.Add(run); err != nil {
		t.Fatalf("runs.Add: %v", err)
	}
	service := NewService(WithRunStore(runs))

	warningsPayload, err := service.ResultView(ResultViewRequest{RunID: run.ID, View: "warnings", Limit: 1})
	if err != nil {
		t.Fatalf("ResultView warnings: %v", err)
	}
	warnings := jftradeCheckedTypeAssertion[[]string](jftradeCheckedTypeAssertion[map[string]any](warningsPayload["series"])["warnings"])
	if len(warnings) != 1 || warnings[0] != "stale quote" {
		t.Fatalf("warning page = %#v", warnings)
	}

	chartPayload, err := service.ResultView(ResultViewRequest{
		RunID: run.ID, View: "chart", Resolution: "1m", Limit: 10,
		StartTime: "2024-01-02T00:01:00Z", EndTime: "2024-01-02T00:02:00Z",
		Include: []string{"candles", "trades", "pnlCurve", "drawdownCurve"},
	})
	if err != nil {
		t.Fatalf("ResultView chart range: %v", err)
	}
	series := jftradeCheckedTypeAssertion[map[string]any](chartPayload["series"])
	if candles := jftradeCheckedTypeAssertion[[]bt.Candle](series["candles"]); len(candles) != 1 || candles[0].Time != "2024-01-02T00:01:00Z" {
		t.Fatalf("filtered candles = %#v", candles)
	}
	if trades := jftradeCheckedTypeAssertion[[]bt.TradeEvent](series["trades"]); len(trades) != 1 || trades[0].Side != "SELL" {
		t.Fatalf("filtered trades = %#v", trades)
	}
	if points := jftradeCheckedTypeAssertion[[]bt.PnLPoint](series["pnlCurve"]); len(points) != 1 || points[0].Equity != 1010 {
		t.Fatalf("filtered pnl curve = %#v", points)
	}
	if points := jftradeCheckedTypeAssertion[[]bt.DrawdownPoint](series["drawdownCurve"]); len(points) != 1 || points[0].Drawdown != 0.005 {
		t.Fatalf("filtered drawdown curve = %#v", points)
	}

	ordersPayload, err := service.ResultView(ResultViewRequest{RunID: run.ID, View: "orders", Limit: 10})
	if err != nil {
		t.Fatalf("ResultView empty orders: %v", err)
	}
	if orders := jftradeCheckedTypeAssertion[[]bt.OrderBookEntry](jftradeCheckedTypeAssertion[map[string]any](ordersPayload["series"])["orderBook"]); len(orders) != 0 {
		t.Fatalf("empty orders = %#v", orders)
	}
}

func TestCoverage98ResultViewAggregationDropsDamagedCandlesWithoutInventingVolume(t *testing.T) {
	if got := aggregateResultViewCandles(nil, time.Minute); len(got) != 0 {
		t.Fatalf("aggregate empty candles = %#v", got)
	}
	aggregated := aggregateResultViewCandles([]bt.Candle{
		{Time: "not-a-time", Volume: "99"},
		{Time: "2024-01-02T00:00:00Z", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "not-a-number"},
	}, time.Minute)
	if len(aggregated) != 1 || aggregated[0].Volume != "" {
		t.Fatalf("damaged candle aggregation = %#v", aggregated)
	}
}
