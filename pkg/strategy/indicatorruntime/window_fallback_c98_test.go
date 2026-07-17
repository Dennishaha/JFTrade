package indicatorruntime

import (
	"math"
	"testing"
	"time"
)

func TestCoverage98TradingWindowCompatibilityKeepsUnavailableInputsDistinct(t *testing.T) {
	cache := newSnapshotSeriesCache()
	values := []float64{10, 20, 30}
	volumes := []float64{1, 2, 3}
	endTimes := []time.Time{
		time.Date(2026, time.January, 5, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 6, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 7, 15, 0, 0, 0, time.UTC),
	}

	// Legacy/unknown average labels intentionally normalize to the MA-compatible
	// accumulator. They must still use selected trading periods and must not
	// manufacture an indicator when timestamps are unavailable.
	fallbackConfig := movingAverageConfig{averageType: "custom_average", period: 2, timeUnit: "day"}
	snapshot := buildMovingAverageSnapshotForTradingWindow(values, volumes, endTimes, fallbackConfig, "US.AAPL", false, cache)
	fields := snapshotValueToMap(snapshot, [...]string{"value", "previous"})
	if fields == nil || fields["value"].(float64) != 25 || fields["previous"].(float64) != 15 {
		t.Fatalf("compatible average snapshot = %#v, want current 25 and previous 15", fields)
	}

	if current, ok := calculateTradingWindowMovingAverageCurrentValue(values, volumes, nil, fallbackConfig, "US.AAPL", len(values), false, cache); ok || current != 0 {
		t.Fatalf("compatible average without timestamps = %v/%v, want unavailable", current, ok)
	}
	if snapshot := buildMovingAverageSnapshotForTradingWindow(values, volumes, nil, fallbackConfig, "US.AAPL", false, cache); snapshot != nil {
		t.Fatalf("compatible average snapshot without timestamps = %#v, want nil", snapshot)
	}
}

func TestCoverage98TradingWindowSequenceHandlesInvalidLabelsAndUnsupportedAccumulators(t *testing.T) {
	values := []float64{10, 20, 30}
	labels := []int64{invalidTradingPeriodLabelKey, 10, 11}

	// A two-period HMA must ignore invalid calendar labels and retain exactly the
	// two valid periods; its value is based on chronological selected bars.
	summary := summarizeTradingWindowSelectionFromKeys(labels, 2, len(labels))
	if !summary.valid || summary.count != 2 || summary.startIndex != 1 || summary.endIndex != 2 {
		t.Fatalf("valid-label summary = %#v", summary)
	}
	if current, ok := calculateHMAFromTradingWindowSelection(values, labels, summary); !ok || math.Abs(current-(30+(30.0-20)/3)) > 1e-9 {
		t.Fatalf("HMA across valid labels = %v/%v", current, ok)
	}
	if _, ok := calculateHMAFromTradingWindowSelection(values, labels, tradingWindowSelectionSummary{
		startKey: 10, startIndex: 0, endIndex: 2, count: 3, valid: true,
	}); ok {
		t.Fatal("a summary claiming an invalid calendar bar must not produce HMA")
	}

	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
		values, nil, labels, movingAverageConfig{averageType: "not-supported", period: 2},
	); !handled || !currentOK || !previousOK || current != 25 || previous != 20 {
		t.Fatalf("compatible accumulator normalization = current:%v previous:%v handled:%v currentOK:%v previousOK:%v", current, previous, handled, currentOK, previousOK)
	}
	if _, _, currentOK, _, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
		nil, nil, nil, movingAverageConfig{averageType: "SMA", period: 2},
	); !handled || currentOK {
		t.Fatalf("empty supported window = handled:%v current:%v", handled, currentOK)
	}
}
