package backtest

import (
	"strings"
	"testing"
	"time"

	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestResultViewOrdersLogsAndErrorsUseWindowAndCursor(t *testing.T) {
	runs := newMemoryRunStore()
	run := &RunState{
		ID:     "bt-result-view-boundary",
		Status: "completed",
		Request: StartRequest{
			DefinitionID:   "def-1",
			Market:         "US",
			Code:           "AAPL",
			Symbol:         "US.AAPL",
			Interval:       "1m",
			StartTime:      "2024-01-02T00:00:00Z",
			EndTime:        "2024-01-02T00:10:00Z",
			InitialBalance: 1000,
			RehabType:      "forward",
		},
		Result: &bt.RunResult{
			Symbol: "US.AAPL",
			OrderBook: []bt.OrderBookEntry{
				{OrderID: "submitted-before-filled-inside", SubmittedAt: "2024-01-02T00:00:00Z", FilledAt: "2024-01-02T00:02:00Z", Status: "FILLED"},
				{OrderID: "submitted-inside", SubmittedAt: "2024-01-02T00:02:30Z", Status: "NEW"},
				{OrderID: "outside", SubmittedAt: "2024-01-02T00:05:00Z", FilledAt: "2024-01-02T00:06:00Z", Status: "FILLED"},
				{OrderID: "bad-time", SubmittedAt: "not-time", Status: "NEW"},
			},
			Logs:          []string{"log-1", "log-2", "log-3"},
			RuntimeErrors: []string{"err-1", "err-2"},
		},
	}
	if err := runs.Add(run); err != nil {
		t.Fatalf("runs.Add: %v", err)
	}
	svc := NewService(WithRunStore(runs))

	ordersPayload, err := svc.ResultView(ResultViewRequest{
		RunID:     run.ID,
		View:      "orders",
		StartTime: "2024-01-02T00:01:00Z",
		EndTime:   "2024-01-02T00:03:00Z",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("ResultView orders: %v", err)
	}
	window := jftradeCheckedTypeAssertion[map[string]any](ordersPayload["window"])
	if window["truncated"] != true || window["nextCursor"] != "1" {
		t.Fatalf("orders window = %#v", window)
	}
	orderSeries := jftradeCheckedTypeAssertion[map[string]any](ordersPayload["series"])
	orders := jftradeCheckedTypeAssertion[[]bt.OrderBookEntry](orderSeries["orderBook"])
	if len(orders) != 1 || orders[0].OrderID != "submitted-before-filled-inside" {
		t.Fatalf("orders page 1 = %#v", orders)
	}

	nextPayload, err := svc.ResultView(ResultViewRequest{
		RunID:     run.ID,
		View:      "orders",
		StartTime: "2024-01-02T00:01:00Z",
		EndTime:   "2024-01-02T00:03:00Z",
		Limit:     1,
		Cursor:    "1",
	})
	if err != nil {
		t.Fatalf("ResultView orders page 2: %v", err)
	}
	nextOrders := jftradeCheckedTypeAssertion[[]bt.OrderBookEntry](jftradeCheckedTypeAssertion[map[string]any](nextPayload["series"])["orderBook"])
	if len(nextOrders) != 1 || nextOrders[0].OrderID != "submitted-inside" {
		t.Fatalf("orders page 2 = %#v", nextOrders)
	}

	logPayload, err := svc.ResultView(ResultViewRequest{RunID: run.ID, View: "logs", Limit: 2, Cursor: "1"})
	if err != nil {
		t.Fatalf("ResultView logs: %v", err)
	}
	logs := jftradeCheckedTypeAssertion[[]string](jftradeCheckedTypeAssertion[map[string]any](logPayload["series"])["logs"])
	if strings.Join(logs, ",") != "log-2,log-3" {
		t.Fatalf("logs page = %#v", logs)
	}

	errorPayload, err := svc.ResultView(ResultViewRequest{RunID: run.ID, View: "errors", Limit: 1})
	if err != nil {
		t.Fatalf("ResultView errors: %v", err)
	}
	runtimeErrors := jftradeCheckedTypeAssertion[[]string](jftradeCheckedTypeAssertion[map[string]any](errorPayload["series"])["runtimeErrors"])
	if len(runtimeErrors) != 1 || runtimeErrors[0] != "err-1" {
		t.Fatalf("runtime errors = %#v", runtimeErrors)
	}
}

func TestResultViewParsingAndResolutionBoundaries(t *testing.T) {
	if got := normalizeResultViewLimit(0); got != 500 {
		t.Fatalf("default limit = %d", got)
	}
	if got := normalizeResultViewLimit(5000); got != 2000 {
		t.Fatalf("capped limit = %d", got)
	}
	if got := normalizeResultViewLimit(25); got != 25 {
		t.Fatalf("explicit limit = %d", got)
	}

	if offset, err := parseResultViewCursor(" 2 "); err != nil || offset != 2 {
		t.Fatalf("parse cursor = %d err=%v", offset, err)
	}
	for _, cursor := range []string{"-1", "abc"} {
		if _, err := parseResultViewCursor(cursor); err == nil || !strings.Contains(err.Error(), "cursor must be") {
			t.Fatalf("parse cursor %q err=%v", cursor, err)
		}
	}

	parsed, err := parseOptionalResultViewTime("2024-01-02T08:00:00+08:00", "startTime")
	if err != nil || parsed == nil || parsed.Format(time.RFC3339) != "2024-01-02T00:00:00Z" {
		t.Fatalf("parse optional time = %v err=%v", parsed, err)
	}
	if empty, err := parseOptionalResultViewTime("", "endTime"); err != nil || empty != nil {
		t.Fatalf("empty optional time = %v err=%v", empty, err)
	}
	if _, err := parseOptionalResultViewTime("bad", "endTime"); err == nil || !strings.Contains(err.Error(), "invalid endTime") {
		t.Fatalf("invalid optional time err=%v", err)
	}

	durationCases := map[string]time.Duration{
		"90":  90 * time.Minute,
		"90m": 90 * time.Minute,
		"2h":  2 * time.Hour,
		"3d":  72 * time.Hour,
		"2w":  14 * 24 * time.Hour,
	}
	for raw, want := range durationCases {
		got, err := resultViewIntervalDuration(raw)
		if err != nil || got != want {
			t.Fatalf("resultViewIntervalDuration(%q) = %s err=%v, want %s", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "bad", "3x"} {
		if _, err := resultViewIntervalDuration(raw); err == nil {
			t.Fatalf("resultViewIntervalDuration(%q) succeeded, want error", raw)
		}
	}

	if got := chooseResultViewAutoResolution(time.Minute, 10, 100); got != time.Minute {
		t.Fatalf("auto resolution below limit = %s", got)
	}
	if got := chooseResultViewAutoResolution(time.Minute, 1000, 100); got != 15*time.Minute {
		t.Fatalf("auto resolution for dense minute candles = %s, want 15m", got)
	}
	if got := chooseResultViewAutoResolution(2*time.Hour, 1000, 100); got != 24*time.Hour {
		t.Fatalf("auto resolution for dense 2h candles = %s, want 1d", got)
	}
	if got := chooseResultViewAutoResolution(10*24*time.Hour, 1000, 100); got != 100*24*time.Hour {
		t.Fatalf("auto resolution fallback = %s, want required duration", got)
	}

	labelCases := map[time.Duration]string{
		14 * 24 * time.Hour: "2w",
		48 * time.Hour:      "2d",
		3 * time.Hour:       "3h",
		45 * time.Minute:    "45m",
		30 * time.Second:    "30s",
	}
	for duration, want := range labelCases {
		if got := resultViewResolutionLabel(duration); got != want {
			t.Fatalf("resolution label %s = %q, want %q", duration, got, want)
		}
	}
}

func TestResultViewCandlesFiltersInvalidTimesAndAggregatesVolumeBoundaries(t *testing.T) {
	start := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.January, 2, 0, 5, 0, 0, time.UTC)
	resolution, candles, err := resultViewCandles([]bt.Candle{
		{Time: "bad", Open: "0", High: "0", Low: "0", Close: "0", Volume: "1"},
		{Time: "2024-01-02T00:00:00Z", Open: "9", High: "9", Low: "9", Close: "9", Volume: "1"},
		{Time: "2024-01-02T00:01:00Z", Open: "10", High: "11", Low: "8", Close: "10.5", Volume: "bad"},
		{Time: "2024-01-02T00:02:00Z", Open: "10.5", High: "12", Low: "9", Close: "11.5", Volume: "2"},
		{Time: "2024-01-02T00:03:00Z", Open: "11.5", High: "bad", Low: "7", Close: "12.5", Volume: "3"},
	}, "1m", "2m", &start, &end, 100)
	if err != nil {
		t.Fatalf("resultViewCandles: %v", err)
	}
	if resolution != "2m" || len(candles) != 2 {
		t.Fatalf("resolution/candles = %s %#v", resolution, candles)
	}
	if candles[0].Time != "2024-01-02T00:00:00Z" || candles[0].Open != "9" || candles[0].High != "11" || candles[0].Low != "8" || candles[0].Close != "10.5" || candles[0].Volume != "" {
		t.Fatalf("first aggregate = %#v", candles[0])
	}
	if candles[1].Time != "2024-01-02T00:02:00Z" || candles[1].Close != "12.5" || candles[1].High != "12" || candles[1].Low != "7" || candles[1].Volume != "5" {
		t.Fatalf("second aggregate = %#v", candles[1])
	}
}

func TestResultViewRejectsBadRequestsAndPreservesEmptyRunShape(t *testing.T) {
	svcWithoutStore := NewService()
	if _, err := svcWithoutStore.ResultView(ResultViewRequest{}); err == nil || !strings.Contains(err.Error(), "runId is required") {
		t.Fatalf("missing runId err=%v", err)
	}
	if _, err := svcWithoutStore.ResultView(ResultViewRequest{RunID: "missing"}); err == nil || !strings.Contains(err.Error(), "run store not configured") {
		t.Fatalf("nil store err=%v", err)
	}

	runs := newMemoryRunStore()
	emptyRun := &RunState{
		ID:      "empty-result",
		Status:  "completed",
		Request: StartRequest{Symbol: "US.AAPL", Interval: "1m", InitialBalance: 1000},
	}
	if err := runs.Add(emptyRun); err != nil {
		t.Fatalf("runs.Add empty: %v", err)
	}
	runWithSeries := &RunState{
		ID:      "series-result",
		Status:  "completed",
		Request: StartRequest{Symbol: "US.AAPL", Interval: "1m", InitialBalance: 1000},
		Result: &bt.RunResult{
			Candles: []bt.Candle{
				{Time: "2024-01-02T00:00:00Z", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "100"},
				{Time: "2024-01-02T00:01:00Z", Open: "10.5", High: "12", Low: "10", Close: "11.5", Volume: "120"},
			},
			Trades:        []bt.TradeEvent{{Time: "2024-01-02T00:01:00Z", Side: "BUY", Qty: "1", Price: "11"}},
			PnLCurve:      []bt.PnLPoint{{Time: "2024-01-02T00:00:00Z", Equity: 1000}},
			DrawdownCurve: []bt.DrawdownPoint{{Time: "2024-01-02T00:00:00Z", Drawdown: 0}},
		},
	}
	if err := runs.Add(runWithSeries); err != nil {
		t.Fatalf("runs.Add series: %v", err)
	}
	svc := NewService(WithRunStore(runs))

	if _, err := svc.ResultView(ResultViewRequest{RunID: "missing"}); err == nil || !strings.Contains(err.Error(), "backtest run not found") {
		t.Fatalf("missing run err=%v", err)
	}
	if _, err := svc.ResultView(ResultViewRequest{RunID: emptyRun.ID, View: "positions"}); err == nil || !strings.Contains(err.Error(), "view must be") {
		t.Fatalf("bad view err=%v", err)
	}
	if _, err := svc.ResultView(ResultViewRequest{RunID: emptyRun.ID, StartTime: "bad-time"}); err == nil || !strings.Contains(err.Error(), "invalid startTime") {
		t.Fatalf("bad startTime err=%v", err)
	}
	if _, err := svc.ResultView(ResultViewRequest{RunID: emptyRun.ID, StartTime: "2024-01-03T00:00:00Z", EndTime: "2024-01-02T00:00:00Z"}); err == nil || !strings.Contains(err.Error(), "endTime must be") {
		t.Fatalf("inverted time window err=%v", err)
	}

	emptyPayload, err := svc.ResultView(ResultViewRequest{RunID: emptyRun.ID, View: "chart"})
	if err != nil {
		t.Fatalf("empty run view: %v", err)
	}
	if series := jftradeCheckedTypeAssertion[map[string]any](emptyPayload["series"]); len(series) != 0 {
		t.Fatalf("empty result series = %#v, want empty map", series)
	}

	payload, err := svc.ResultView(ResultViewRequest{
		RunID:      runWithSeries.ID,
		View:       "chart",
		Include:    []string{" trades ", "", "drawdownCurve"},
		Limit:      1,
		Cursor:     "5",
		Resolution: "auto",
	})
	if err != nil {
		t.Fatalf("include/cursor chart view: %v", err)
	}
	series := jftradeCheckedTypeAssertion[map[string]any](payload["series"])
	if _, ok := series["candles"]; ok {
		t.Fatalf("candles included despite explicit include set: %#v", series)
	}
	trades := jftradeCheckedTypeAssertion[[]bt.TradeEvent](series["trades"])
	if len(trades) != 0 {
		t.Fatalf("trades beyond cursor = %#v, want empty", trades)
	}
	drawdowns := jftradeCheckedTypeAssertion[[]bt.DrawdownPoint](series["drawdownCurve"])
	if len(drawdowns) != 0 {
		t.Fatalf("drawdowns beyond cursor = %#v, want empty", drawdowns)
	}
	window := jftradeCheckedTypeAssertion[map[string]any](payload["window"])
	if window["truncated"] != false || window["nextCursor"] != "" {
		t.Fatalf("window beyond cursor = %#v", window)
	}
}

func TestResultViewSummaryPayloadIncludesRunMetadataAndLatestDiagnostics(t *testing.T) {
	runs := newMemoryRunStore()
	run := &RunState{
		ID:     "summary-run",
		Status: "completed",
		Request: StartRequest{
			DefinitionID:      "def-1",
			DefinitionVersion: "v2",
			Market:            "US",
			Code:              "AAPL",
			Symbol:            "US.AAPL",
			InstrumentType:    "stock",
			Interval:          "1m",
			StartDate:         "2024-01-02",
			EndDate:           "2024-01-03",
			StartTime:         "2024-01-02T00:00:00Z",
			EndTime:           "2024-01-03T00:00:00Z",
			MarketTimezone:    "America/New_York",
			InitialBalance:    1000,
			RehabType:         "forward",
			UseExtendedHours:  new(true),
		},
		Result: &bt.RunResult{
			QuoteCurrency:     "USD",
			FinalBalance:      1125,
			PnL:               125,
			TotalBrokerFees:   1.2,
			TotalMarketFees:   0.8,
			TotalFees:         2,
			FeeBreakdown:      []bt.FeeBreakdownEntry{{Category: "broker", Amount: 1.2}},
			MaxDrawdown:       0.03,
			CurrentDrawdown:   0.01,
			TotalTrades:       2,
			WinRate:           0.5,
			Candles:           []bt.Candle{{Time: "2024-01-02T00:00:00Z"}},
			Trades:            []bt.TradeEvent{{Time: "2024-01-02T00:01:00Z"}},
			OrderBook:         []bt.OrderBookEntry{{SubmittedAt: "2024-01-02T00:01:00Z"}},
			PnLCurve:          []bt.PnLPoint{{Time: "2024-01-02T00:01:00Z"}},
			DrawdownCurve:     []bt.DrawdownPoint{{Time: "2024-01-02T00:01:00Z"}},
			Logs:              []string{"started", "finished"},
			Warnings:          []string{"ignored close"},
			WarningTotal:      1,
			IgnoredOrders:     1,
			RuntimeErrors:     []string{"late fill"},
			RuntimeErrorTotal: 1,
		},
		CreatedAt: "2024-01-02T00:00:00Z",
		UpdatedAt: "2024-01-03T00:00:00Z",
	}
	if err := runs.Add(run); err != nil {
		t.Fatalf("runs.Add: %v", err)
	}

	payload, err := NewService(WithRunStore(runs)).ResultView(ResultViewRequest{RunID: run.ID})
	if err != nil {
		t.Fatalf("ResultView summary: %v", err)
	}
	if payload["view"] != "summary" {
		t.Fatalf("default view = %q, want summary", payload["view"])
	}
	runPayload := jftradeCheckedTypeAssertion[map[string]any](payload["run"])
	useExtendedHours, ok := runPayload["useExtendedHours"].(*bool)
	if runPayload["definitionId"] != "def-1" || !ok || useExtendedHours == nil || !*useExtendedHours || runPayload["createdAt"] != run.CreatedAt {
		t.Fatalf("run payload = %#v", runPayload)
	}
	summary := jftradeCheckedTypeAssertion[map[string]any](payload["summary"])
	if summary["totalReturn"] != 0.125 || summary["candlesCount"] != 1 || summary["latestLog"] != "finished" || summary["latestWarning"] != "ignored close" || summary["latestRuntimeError"] != "late fill" {
		t.Fatalf("summary payload = %#v", summary)
	}
	if summary["warningTotal"] != 1 || summary["ignoredOrders"] != 1 {
		t.Fatalf("warning summary payload = %#v", summary)
	}

	if got := resultViewRunPayload(nil); len(got) != 0 {
		t.Fatalf("nil run payload = %#v, want empty", got)
	}
	if got := resultViewSummaryPayload(&RunState{}); len(got) != 0 {
		t.Fatalf("nil result summary = %#v, want empty", got)
	}
}
