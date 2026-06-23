package indicatorruntime

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestSeriesTrimHelpersKeepNewestValues(t *testing.T) {
	times := []time.Time{
		time.Unix(1, 0),
		time.Unix(2, 0),
		time.Unix(3, 0),
	}
	trimmedTimes := trimTimeSeriesInPlace(append([]time.Time(nil), times...), 2)
	if !reflect.DeepEqual(trimmedTimes, []time.Time{times[1], times[2]}) {
		t.Fatalf("trimTimeSeriesInPlace() = %#v", trimmedTimes)
	}

	sessions := []market.Session{market.SessionPre, market.SessionRegular, market.SessionAfter}
	trimmedSessions := trimSessionSeriesInPlace(append([]market.Session(nil), sessions...), 2)
	if !reflect.DeepEqual(trimmedSessions, []market.Session{market.SessionRegular, market.SessionAfter}) {
		t.Fatalf("trimSessionSeriesInPlace() = %#v", trimmedSessions)
	}

	ints := []int64{1, 2, 3, 4}
	trimmedInts := trimInt64SeriesInPlace(append([]int64(nil), ints...), 3)
	if !reflect.DeepEqual(trimmedInts, []int64{2, 3, 4}) {
		t.Fatalf("trimInt64SeriesInPlace() = %#v", trimmedInts)
	}

	if got := trimTimeSeriesInPlace(append([]time.Time(nil), times...), 0); len(got) != len(times) {
		t.Fatalf("trimTimeSeriesInPlace(limit=0) len = %d, want %d", len(got), len(times))
	}
}

func TestRangeAndRollingExtremaHelpersFollowMarketDataSemantics(t *testing.T) {
	if got := trueRange(12, 10, 7); got != 5 {
		t.Fatalf("trueRange() = %v, want 5", got)
	}

	highs := []float64{10, 12, 11, 15, 14}
	lows := []float64{8, 9, 7, 10, 11}
	rollingHighs, rollingLows := calculateRollingHighLow(highs, lows, 3)
	if !reflect.DeepEqual(rollingHighs, []float64{10, 12, 12, 15, 15}) {
		t.Fatalf("rolling highs = %#v", rollingHighs)
	}
	if !reflect.DeepEqual(rollingLows, []float64{8, 8, 7, 7, 7}) {
		t.Fatalf("rolling lows = %#v", rollingLows)
	}
	if highs, lows := calculateRollingHighLow(highs, lows[:2], 3); highs != nil || lows != nil {
		t.Fatalf("calculateRollingHighLow(mismatched) = %#v %#v, want nil nil", highs, lows)
	}
}

func TestMovingAverageAndMoneyFlowHelpersValidateAndCompute(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5}
	if got := calculateRMASequence(values, 3); len(got) != 3 ||
		math.Abs(got[0]-2) > 1e-9 ||
		math.Abs(got[1]-8.0/3.0) > 1e-9 ||
		math.Abs(got[2]-31.0/9.0) > 1e-9 {
		t.Fatalf("calculateRMASequence() = %#v", got)
	}
	if got := calculateRMASequence(values, 0); got != nil {
		t.Fatalf("calculateRMASequence(period=0) = %#v, want nil", got)
	}

	mfiValues := []float64{10, 11, 13, 12, 14}
	volumes := []float64{1, 2, 3, 4, 5}
	gotMFI, ok := calculateMFIValue(mfiValues, volumes, 4)
	if !ok {
		t.Fatal("calculateMFIValue() ok = false, want true")
	}
	positiveFlow := 11.0*2 + 13.0*3 + 14.0*5
	negativeFlow := 12.0 * 4
	wantMFI := 100 - 100/(1+positiveFlow/negativeFlow)
	if math.Abs(gotMFI-wantMFI) > 1e-9 {
		t.Fatalf("calculateMFIValue() = %v, want %v", gotMFI, wantMFI)
	}

	if got, ok := calculateMFIValue([]float64{1, 2, 3}, []float64{1, 1}, 2); ok || got != 0 {
		t.Fatalf("calculateMFIValue(mismatched) = (%v, %v), want (0, false)", got, ok)
	}
	if got, ok := calculateMFIValue([]float64{1, 2, 3, 4}, []float64{1, 1, 1, 1}, 3); !ok || got != 100 {
		t.Fatalf("calculateMFIValue(all positive) = (%v, %v), want (100, true)", got, ok)
	}
}

func TestDMIHelpersCaptureDirectionalTrendAndInvalidInput(t *testing.T) {
	highs := []float64{10, 11, 12, 13, 14, 15}
	lows := []float64{9, 9.5, 10.5, 11.5, 12.5, 13.5}
	closes := []float64{9.5, 10.5, 11.5, 12.5, 13.5, 14.5}
	config := dmiConfig{diLength: 2, adxSmoothing: 2}

	plusDI, minusDI, adx, ok := calculateDMIValues(highs, lows, closes, config)
	if !ok {
		t.Fatal("calculateDMIValues() ok = false, want true")
	}
	if plusDI <= minusDI {
		t.Fatalf("calculateDMIValues() plus=%v minus=%v, want plus > minus for uptrend", plusDI, minusDI)
	}
	if adx <= 0 {
		t.Fatalf("calculateDMIValues() adx=%v, want positive trend strength", adx)
	}

	snapshot := calculateDMISnapshot(highs, lows, closes, config)
	if snapshot == nil {
		t.Fatal("calculateDMISnapshot() = nil, want snapshot")
	}
	if snapshot["plus"] != plusDI || snapshot["minus"] != minusDI || snapshot["adx"] != adx {
		t.Fatalf("calculateDMISnapshot() = %#v", snapshot)
	}

	if snapshot := calculateDMISnapshot(highs[:2], lows[:2], closes[:2], config); snapshot != nil {
		t.Fatalf("calculateDMISnapshot(short input) = %#v, want nil", snapshot)
	}
}
