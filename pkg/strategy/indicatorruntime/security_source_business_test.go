package indicatorruntime

import (
	"math"
	"testing"
	"time"
)

func TestSecuritySourceSnapshotValuesCoversFixedAndTradingPeriodWindows(t *testing.T) {
	runtime := &indicatorRuntime{
		intervalMinutes: 1,
		opens:           []float64{1, 2, 3, 4},
		highs:           []float64{2, 3, 4, 5},
		lows:            []float64{0, 1, 2, 3},
		closes:          []float64{1.5, 2.5, 3.5, 4.5},
		volumes:         []float64{10, 20, 30, 40},
	}

	current, previous, currentOK, previousOK := runtime.securitySourceSnapshotValues(securitySourceConfig{source: "close", timeUnit: "2m"}, nil)
	if !currentOK || !previousOK || current != 4.5 || previous != 2.5 {
		t.Fatalf("fixed 2m close snapshot = %v/%v/%v/%v, want 4.5/2.5/true/true", current, previous, currentOK, previousOK)
	}
	current, previous, currentOK, previousOK = runtime.securitySourceSnapshotValues(securitySourceConfig{source: "high", timeUnit: "2m", lookback: 1}, nil)
	if !currentOK || previousOK || current != 3 || previous != 0 {
		t.Fatalf("fixed 2m lookback snapshot = %v/%v/%v/%v, want 3/0/true/false", current, previous, currentOK, previousOK)
	}

	cache := newSnapshotSeriesCache()
	tradingRuntime := &indicatorRuntime{
		symbol:          "US.AAPL",
		intervalMinutes: 1,
		opens:           []float64{100, 80, 90, 85},
		highs:           []float64{101, 82, 91, 86},
		lows:            []float64{99, 79, 88, 84},
		closes:          []float64{100, 80, 90, 85},
		volumes:         []float64{10, 20, 30, 40},
		endTimes: []time.Time{
			time.Date(2026, time.May, 28, 19, 59, 59, 0, time.UTC),
			time.Date(2026, time.May, 28, 21, 0, 0, 0, time.UTC),
			time.Date(2026, time.May, 29, 14, 0, 0, 0, time.UTC),
			time.Date(2026, time.May, 29, 19, 30, 0, 0, time.UTC),
		},
		tradingPeriodLabels: map[string][]int64{},
	}
	current, previous, currentOK, previousOK = tradingRuntime.securitySourceSnapshotValues(securitySourceConfig{source: "volume", timeUnit: "day"}, cache)
	if !currentOK || !previousOK || current != 70 || previous != 10 {
		t.Fatalf("trading-day volume snapshot = %v/%v/%v/%v, want 70/10/true/true", current, previous, currentOK, previousOK)
	}
	if len(cache.tradingPeriodLabels) == 0 {
		t.Fatal("security_source trading-period path did not populate label cache")
	}

	current, previous, currentOK, previousOK = tradingRuntime.securitySourceSnapshotValues(securitySourceConfig{source: "close", timeUnit: "day", lookback: 1}, cache)
	if !currentOK || previousOK || math.Abs(current-100) > 1e-9 || previous != 0 {
		t.Fatalf("trading-day lookback snapshot = %v/%v/%v/%v, want 100/0/true/false", current, previous, currentOK, previousOK)
	}

	empty := &indicatorRuntime{}
	if current, previous, currentOK, previousOK := empty.securitySourceSnapshotValues(securitySourceConfig{source: "close", timeUnit: "day"}, cache); currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("empty security source snapshot = %v/%v/%v/%v, want zeros false/false", current, previous, currentOK, previousOK)
	}
}
