package runtimecontrol

import "testing"

func TestLiveExecutionLimitations(t *testing.T) {
	script := `//@version=6
strategy("Live", default_qty_type=strategy.percent_of_equity)
strategy.entry("Long", strategy.long, qty_percent=10)
strategy.cancel_all()`
	limitations := LiveExecutionLimitations(script)
	if len(limitations) != 2 || limitations[0].Code != "LIVE_PERCENT_QUANTITY_UNSUPPORTED" || limitations[1].Code != "LIVE_CANCEL_UNSUPPORTED" {
		t.Fatalf("limitations = %#v", limitations)
	}
}

func TestLiveExecutionLimitationsIgnoreCommentsAndStrings(t *testing.T) {
	script := `//@version=6
strategy("strategy.cancel_all and qty_percent")
// strategy.cancel_all()
// strategy.entry("Long", strategy.long, qty_percent=10)
strategy.entry("Long", strategy.long, qty=1)`
	if limitations := LiveExecutionLimitations(script); len(limitations) != 0 {
		t.Fatalf("limitations = %#v", limitations)
	}
}
