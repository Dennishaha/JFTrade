package runmodel

import (
	"fmt"
	"testing"
)

func TestRunResultSnapshotHandlesNilAndReturnsIndependentCopy(t *testing.T) {
	var nilResult *RunResult
	if snapshot := nilResult.Snapshot(); snapshot != nil {
		t.Fatalf("nil snapshot = %#v, want nil", snapshot)
	}

	original := &RunResult{
		Symbol:                 "US.AAPL",
		Interval:               "1m",
		StartTime:              "2026-06-23T13:30:00Z",
		EndTime:                "2026-06-23T20:00:00Z",
		QuoteCurrency:          "USD",
		FinalBalance:           125000,
		PnL:                    5000,
		MaxDrawdown:            0.08,
		CurrentDrawdown:        0.01,
		TradeStatsVersion:      2,
		TotalFills:             24,
		TotalTrades:            12,
		WinRate:                0.58,
		Trades:                 []TradeEvent{{Time: "2026-06-23T13:31:00Z", Side: "BUY", Price: "100", Qty: "10"}},
		OrderBook:              []OrderBookEntry{{OrderID: "1", Status: "FILLED", FilledPrice: "100"}},
		PnLCurve:               []PnLPoint{{Time: "2026-06-23T13:31:00Z", Equity: 101000}},
		DrawdownCurve:          []DrawdownPoint{{Time: "2026-06-23T13:32:00Z", Drawdown: 0.02}},
		Candles:                []Candle{{Time: "2026-06-23T13:31:00Z", Close: "100.5"}},
		Logs:                   []string{"warmup complete"},
		Warnings:               []string{"ignored close"},
		WarningTotal:           1,
		IgnoredOrders:          1,
		Error:                  "transient warning",
		RuntimeErrors:          []string{"partial fill warning"},
		RuntimeErrorCounts:     map[string]int{"partial fill warning": 2},
		RuntimeErrorTotal:      2,
		RuntimeErrorsTruncated: true,
		TotalBrokerFees:        18,
		TotalMarketFees:        11.27,
		TotalFees:              29.27,
		FeeBreakdown:           []FeeBreakdownEntry{{RuleID: "hk_stamp_duty", Group: "market", Amount: 10, Count: 1}},
		TradingCosts: TradingCosts{
			BrokerFees: FeeSchedule{
				Mode:     "market_preset",
				PresetID: "futu_hk_hk_stock_2026_06_30",
				Rules: []FeeRule{{
					ID:        "futu_hk_hk_commission",
					Category:  "broker",
					AppliesTo: []string{"stock", "etf"},
				}},
			},
		},
	}

	snapshot := original.Snapshot()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	snapshot.Trades[0].Price = "999"
	snapshot.OrderBook[0].FilledPrice = "77"
	snapshot.PnLCurve[0].Equity = 5
	snapshot.DrawdownCurve[0].Drawdown = 0.9
	snapshot.Candles[0].Close = "1"
	snapshot.Logs[0] = "changed"
	snapshot.Warnings[0] = "changed"
	snapshot.RuntimeErrors[0] = "changed"
	snapshot.RuntimeErrorCounts["partial fill warning"] = 9
	snapshot.FeeBreakdown[0].Amount = 0
	snapshot.TradingCosts.BrokerFees.Rules[0].AppliesTo[0] = "mutated"

	if original.Trades[0].Price != "100" {
		t.Fatalf("original trade mutated: %#v", original.Trades)
	}
	if original.OrderBook[0].FilledPrice != "100" {
		t.Fatalf("original order book mutated: %#v", original.OrderBook)
	}
	if original.PnLCurve[0].Equity != 101000 {
		t.Fatalf("original pnl curve mutated: %#v", original.PnLCurve)
	}
	if original.DrawdownCurve[0].Drawdown != 0.02 {
		t.Fatalf("original drawdown curve mutated: %#v", original.DrawdownCurve)
	}
	if original.Candles[0].Close != "100.5" {
		t.Fatalf("original candles mutated: %#v", original.Candles)
	}
	if original.Logs[0] != "warmup complete" {
		t.Fatalf("original logs mutated: %#v", original.Logs)
	}
	if original.Warnings[0] != "ignored close" {
		t.Fatalf("original warnings mutated: %#v", original.Warnings)
	}
	if original.RuntimeErrors[0] != "partial fill warning" {
		t.Fatalf("original runtime errors mutated: %#v", original.RuntimeErrors)
	}
	if original.RuntimeErrorCounts["partial fill warning"] != 2 {
		t.Fatalf("original runtime error counts mutated: %#v", original.RuntimeErrorCounts)
	}
	if original.FeeBreakdown[0].Amount != 10 {
		t.Fatalf("original fee breakdown mutated: %#v", original.FeeBreakdown)
	}
	if original.TradingCosts.BrokerFees.Rules[0].AppliesTo[0] != "stock" {
		t.Fatalf("original fee rule appliesTo mutated: %#v", original.TradingCosts.BrokerFees.Rules[0].AppliesTo)
	}
	if snapshot.TotalBrokerFees != 18 || snapshot.TotalMarketFees != 11.27 || snapshot.TotalFees != 29.27 {
		t.Fatalf("snapshot fee totals lost: broker=%f market=%f total=%f", snapshot.TotalBrokerFees, snapshot.TotalMarketFees, snapshot.TotalFees)
	}
	if snapshot.WarningTotal != 1 || snapshot.IgnoredOrders != 1 {
		t.Fatalf("snapshot warning counters lost: warningTotal=%d ignoredOrders=%d", snapshot.WarningTotal, snapshot.IgnoredOrders)
	}
	if snapshot.TradeStatsVersion != 2 || snapshot.TotalFills != 24 {
		t.Fatalf("snapshot trade-stat metadata lost: version=%d fills=%d", snapshot.TradeStatsVersion, snapshot.TotalFills)
	}
}

func TestRunResultSnapshotOmitsEmptyRuntimeErrorCounts(t *testing.T) {
	snapshot := (&RunResult{Symbol: "US.AAPL"}).Snapshot()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if snapshot.RuntimeErrorCounts != nil {
		t.Fatalf("RuntimeErrorCounts = %#v, want nil for empty map", snapshot.RuntimeErrorCounts)
	}
}

func TestAddRuntimeErrorReusesExistingSamplesAndCapsUniqueList(t *testing.T) {
	result := &RunResult{
		RuntimeErrors:      []string{"preloaded warning"},
		RuntimeErrorCounts: map[string]int{"preloaded warning": 1},
	}

	result.AddRuntimeError("preloaded warning")
	result.AddRuntimeError("new warning")
	for index := range 101 {
		result.AddRuntimeError(fmt.Sprintf("unique-%03d", index))
	}

	if result.RuntimeErrorTotal != 103 {
		t.Fatalf("RuntimeErrorTotal = %d, want 103", result.RuntimeErrorTotal)
	}
	if result.RuntimeErrorCounts["preloaded warning"] != 2 {
		t.Fatalf("preloaded warning count = %d, want 2", result.RuntimeErrorCounts["preloaded warning"])
	}
	if result.RuntimeErrorCounts["new warning"] != 1 {
		t.Fatalf("new warning count = %d, want 1", result.RuntimeErrorCounts["new warning"])
	}
	if len(result.RuntimeErrors) != 100 {
		t.Fatalf("RuntimeErrors len = %d, want 100", len(result.RuntimeErrors))
	}
	if result.RuntimeErrors[0] != "preloaded warning" || result.RuntimeErrors[1] != "new warning" {
		t.Fatalf("RuntimeErrors prefix = %#v", result.RuntimeErrors[:2])
	}
	if result.RuntimeErrors[len(result.RuntimeErrors)-1] != "unique-097" {
		t.Fatalf("last sampled runtime error = %q, want unique-097", result.RuntimeErrors[len(result.RuntimeErrors)-1])
	}
	if !result.RuntimeErrorsTruncated {
		t.Fatal("expected RuntimeErrorsTruncated after unique sample cap")
	}
}

func TestRunResultWarningsTrackIgnoredOrdersAndCapSamples(t *testing.T) {
	result := &RunResult{}

	result.AddWarning("general warning")
	result.AddIgnoredOrderWarning("ignored order")
	for index := range 100 {
		result.AddWarning(fmt.Sprintf("warning-%03d", index))
	}

	if result.WarningTotal != 102 {
		t.Fatalf("WarningTotal = %d, want 102", result.WarningTotal)
	}
	if result.IgnoredOrders != 1 {
		t.Fatalf("IgnoredOrders = %d, want 1", result.IgnoredOrders)
	}
	if len(result.Warnings) != 100 {
		t.Fatalf("Warnings len = %d, want 100", len(result.Warnings))
	}
	if result.Warnings[0] != "general warning" || result.Warnings[1] != "ignored order" {
		t.Fatalf("Warnings prefix = %#v", result.Warnings[:2])
	}
	if !result.WarningsTruncated {
		t.Fatal("expected WarningsTruncated after sample cap")
	}
}

func TestRunResultGroupsRepeatedIgnoredOrderWarnings(t *testing.T) {
	result := &RunResult{}

	result.AddWarning("general warning")
	result.AddIgnoredOrderWarningGroup("HK.00700|entry|long|market rules unavailable", "bar 1: ignored entry command")
	result.AddIgnoredOrderWarningGroup("HK.00700|entry|long|market rules unavailable", "bar 2: ignored entry command")
	result.AddIgnoredOrderWarningGroup("HK.00700|entry|long|market rules unavailable", "bar 3: ignored entry command")
	result.AddIgnoredOrderWarningGroup("HK.00700|entry|short|market rules unavailable", "bar 4: ignored entry command")

	if result.IgnoredOrders != 4 {
		t.Fatalf("IgnoredOrders = %d, want 4", result.IgnoredOrders)
	}
	if result.WarningTotal != 3 {
		t.Fatalf("WarningTotal = %d, want 3", result.WarningTotal)
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("Warnings len = %d, want 3: %#v", len(result.Warnings), result.Warnings)
	}
	if result.Warnings[1] != "bar 1: ignored entry command (occurred 3 times; first occurrence shown)" {
		t.Fatalf("grouped warning = %q", result.Warnings[1])
	}
	if result.Warnings[2] != "bar 4: ignored entry command" {
		t.Fatalf("second group warning = %q", result.Warnings[2])
	}
}

func TestRunResultWarningAndRuntimeErrorBoundaryAccounting(t *testing.T) {
	runtimeErrors := &RunResult{RuntimeErrors: []string{"already visible"}}
	runtimeErrors.AddRuntimeError("already visible")
	if runtimeErrors.RuntimeErrorCounts["already visible"] != 1 || len(runtimeErrors.RuntimeErrors) != 1 {
		t.Fatalf("preloaded runtime error accounting = %+v", runtimeErrors)
	}

	grouped := &RunResult{}
	grouped.AddIgnoredOrderWarningGroup(" ", "ungrouped ignored order")
	if grouped.IgnoredOrders != 1 || grouped.WarningTotal != 1 || grouped.Warnings[0] != "ungrouped ignored order" {
		t.Fatalf("blank warning group = %+v", grouped)
	}
	for index := range 100 {
		grouped.AddWarning(fmt.Sprintf("sample-%03d", index))
	}
	grouped.AddIgnoredOrderWarningGroup("capped-group", "this group cannot be sampled")
	grouped.AddIgnoredOrderWarningGroup("capped-group", "later occurrence")
	if !grouped.WarningsTruncated || grouped.WarningTotal != 102 || grouped.IgnoredOrders != 3 || len(grouped.Warnings) != 100 {
		t.Fatalf("capped warning group = %+v", grouped)
	}
	if got := groupedWarningMessage("first", 1); got != "first" {
		t.Fatalf("single grouped warning = %q", got)
	}
}
