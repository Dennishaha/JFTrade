package runtimecontrol

import "testing"

func TestLiveExecutionLimitationsAllowsBrokerExecutedPineSemantics(t *testing.T) {
	script := `//@version=6
strategy("Live", default_qty_type=strategy.percent_of_equity)
strategy.entry("Long", strategy.long, qty_percent=10)
strategy.cancel_all()`
	if limitations := LiveExecutionLimitations(script); len(limitations) != 0 {
		t.Fatalf("limitations = %#v", limitations)
	}
}
