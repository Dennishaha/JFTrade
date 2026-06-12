package backtest

import (
	"testing"

	"github.com/c9s/bbgo/pkg/types"
)

func TestSessionFilteredStoreCustomAggregationOnlyAppliesToUS(t *testing.T) {
	store := &sessionFilteredBacktestStore{includeExtendedHours: true}

	nonUSSymbols := []string{"HK.00700", "SH.600519", "SZ.000001"}
	for _, symbol := range nonUSSymbols {
		t.Run(symbol+" daily", func(t *testing.T) {
			if store.shouldUseCustomTradingPeriodAggregation(symbol, types.Interval1d) {
				t.Fatalf("%s must not use US extended-hours trading-period aggregation", symbol)
			}
		})
		t.Run(symbol+" intraday", func(t *testing.T) {
			if store.shouldUseCustomSessionAwareIntradayAggregation(symbol, types.Interval2h) {
				t.Fatalf("%s must not use US extended-hours intraday aggregation", symbol)
			}
		})
	}

	if !store.shouldUseCustomTradingPeriodAggregation("US.AAPL", types.Interval1d) {
		t.Fatal("US.AAPL should keep custom trading-period aggregation when extended hours are included")
	}
	if !store.shouldUseCustomSessionAwareIntradayAggregation("US.AAPL", types.Interval2h) {
		t.Fatal("US.AAPL should keep custom session-aware intraday aggregation when extended hours are included")
	}
}
