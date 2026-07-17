package indicatorruntime

import (
	"testing"
	"time"
)

func TestCoverage98TradingWindowSequenceSnapshotsRetainCurrentAndPreviousSessions(t *testing.T) {
	// A daily strategy may receive more than one intraday bar for each trading
	// session. Sequence-based averages must keep both the current and the
	// previous daily result available to crossover rules.
	values := []float64{10, 12, 20}
	endTimes := []time.Time{
		time.Date(2026, time.May, 27, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
	}

	for _, averageType := range []string{"EMA", "EXPMA", "SMMA", "TMA", "HMA"} {
		t.Run(averageType, func(t *testing.T) {
			current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotOnlineWithCache(
				values,
				nil,
				endTimes,
				movingAverageConfig{averageType: averageType, period: 1, timeUnit: "day"},
				"US.AAPL",
				false,
				newSnapshotSeriesCache(),
			)
			if !handled || !currentOK || !previousOK || !isFiniteBusinessNumber(current) || !isFiniteBusinessNumber(previous) {
				t.Fatalf("%s daily snapshot = current:%v previous:%v currentOK:%v previousOK:%v handled:%v", averageType, current, previous, currentOK, previousOK, handled)
			}
		})
	}
}

func TestCoverage98TradingWindowHMAIgnoresNonTradingGapsInsideSelectedRange(t *testing.T) {
	// A weekend/holiday gap can sit between the two selected sessions. It must
	// not be treated as a price sample when computing the HMA snapshot.
	labels := []int64{101, invalidTradingPeriodLabelKey, 102}
	summary := summarizeTradingWindowSelectionFromKeys(labels, 2, len(labels))
	value, ok := calculateHMAFromTradingWindowSelection([]float64{10, 999, 20}, labels, summary)
	if !ok || value != 20+(20.0-10)/3 {
		t.Fatalf("HMA across a non-trading gap = %v/%v, want %v/true", value, ok, 20+(20.0-10)/3)
	}
}
