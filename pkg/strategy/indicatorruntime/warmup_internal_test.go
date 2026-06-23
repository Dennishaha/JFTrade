package indicatorruntime

import "testing"

func TestCalculateIndicatorWarmupBarsCoversSourceAwareWindowsAndDivergences(t *testing.T) {
	requirements := indicatorRequirements{
		rsiSource:      []sourcePeriodConfig{{source: "hlc3", period: 14}},
		stdevSource:    []sourcePeriodConfig{{source: "hlc3", period: 11}},
		variance:       []sourcePeriodConfig{{source: "close", period: 9}},
		windows:        []windowConfig{{function: "change", source: "close", period: 4}, {function: "range", source: "high", period: 7}},
		cum:            []sourceConfig{{source: "volume"}},
		stoch:          []sourcePeriodConfig{{source: "hlc3", period: 12, timeUnit: "day"}},
		kdj:            []kdjConfig{{period: 9, m1: 3, m2: 3}},
		cciSource:      []sourcePeriodConfig{{source: "close", period: 20}},
		williamsR:      []int{14},
		mfi:            []sourcePeriodConfig{{source: "hlc3", period: 15}},
		dmi:            []dmiConfig{{diLength: 14, adxSmoothing: 14}},
		supertrend:     []supertrendConfig{{factor: 3, atrPeriod: 10}},
		sar:            []sarConfig{{start: 0.02, increment: 0.02, maximum: 0.2}},
		rsiDivergence:  []rsiDivergenceConfig{{period: 14, direction: "top", lookback: 5}},
		macdDivergence: []macdDivergenceConfig{{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 6}},
		kdjDivergence:  []kdjDivergenceConfig{{period: 9, m1: 3, m2: 3, direction: "top", lookback: 4}},
		stopLoss:       []stopLossConfig{{mode: "stopLoss", direction: "auto", timeValue: 3, timeUnit: "bar", percentage: 5, windowPolicy: "continuous"}},
		securitySource: []securitySourceConfig{{source: "close", timeUnit: "week", lookback: 1}},
	}

	warmupBars := calculateIndicatorWarmupBars(requirements, 5, "US.AAPL", false)
	if warmupBars != 1170 {
		t.Fatalf("calculateIndicatorWarmupBars() = %d, want 1170", warmupBars)
	}
}

func TestEstimateTradingPeriodBarsHandlesFallbackAndInvalidInputs(t *testing.T) {
	if got := estimateTradingPeriodBars(0, "day", 5, "US.AAPL", false); got != 0 {
		t.Fatalf("estimateTradingPeriodBars(period=0) = %d, want 0", got)
	}
	if got := estimateTradingPeriodBars(3, "hour", 0, "", false); got != 180 {
		t.Fatalf("estimateTradingPeriodBars(hour, interval=0) = %d, want 180", got)
	}
	if got := estimateTradingPeriodBars(2, "week", 5, "CRYPTO.BTC", false); got != 780 {
		t.Fatalf("estimateTradingPeriodBars(unknown symbol week) = %d, want 780", got)
	}
}
