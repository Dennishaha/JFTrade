package indicatorruntime

import (
	"math"
	"testing"
)

func TestRollingSnapshotStatesExposeMatureAndImmatureBoundaries(t *testing.T) {
	t.Run("moving average", func(t *testing.T) {
		state := &rollingMovingAverageSnapshotState{kind: "SMA", period: 3}
		if state.snapshotValue() != nil {
			t.Fatal("immature moving average snapshot should be nil")
		}
		state.push(10, 1)
		state.push(20, 1)
		if value, ok := state.PreferredScalarValue(); ok || value != 0 {
			t.Fatalf("immature moving average scalar = %v/%v", value, ok)
		}
		state.push(30, 1)
		if value, ok := state.PreferredScalarValue(); !ok || value != 20 {
			t.Fatalf("mature moving average scalar = %v/%v, want 20/true", value, ok)
		}
		state.push(60, 1)
		current, previous, currentOK, previousOK, fieldOK := state.SeriesField("value")
		if !fieldOK || !currentOK || !previousOK || current != 110.0/3.0 || previous != 20 {
			t.Fatalf("moving average series = %v %v %v %v %v", current, previous, currentOK, previousOK, fieldOK)
		}
		if value, ok := state.FieldValue("previous"); !ok || value.(float64) != 20 {
			t.Fatalf("moving average previous field = %#v/%v", value, ok)
		}
		if _, ok := state.FieldValue("unknown"); ok {
			t.Fatal("moving average accepted unknown field")
		}
	})

	t.Run("volume weighted moving average", func(t *testing.T) {
		state := &rollingMovingAverageSnapshotState{kind: "VWMA", period: 2}
		state.push(10, 0)
		state.push(20, 0)
		if value, ok := state.PreferredScalarValue(); ok || value != 0 {
			t.Fatalf("zero-volume VWMA scalar = %v/%v", value, ok)
		}
		state.push(30, 2)
		if value, ok := state.PreferredScalarValue(); !ok || value != 30 {
			t.Fatalf("VWMA after zero-volume eviction = %v/%v, want 30/true", value, ok)
		}
	})

	t.Run("bollinger", func(t *testing.T) {
		state := &rollingBollingerState{period: 3, multiplier: 2}
		state.push(5)
		state.push(5)
		if state.snapshot() != nil {
			t.Fatal("immature bollinger snapshot should be nil")
		}
		state.push(5)
		if value, ok := state.PreferredScalarValue(); !ok || value != 5 {
			t.Fatalf("flat bollinger middle = %v/%v, want 5/true", value, ok)
		}
		upper, ok := state.FieldValue("upper")
		if !ok || upper.(float64) != 5 {
			t.Fatalf("flat bollinger upper = %#v/%v, want 5/true", upper, ok)
		}
		if _, ok := state.FieldValue("unknown"); ok {
			t.Fatal("bollinger accepted unknown field")
		}
	})

	t.Run("atr and standard deviation", func(t *testing.T) {
		atr := &rollingATRState{period: 2}
		atr.push(12, 10, 11, 0, false)
		if value := atr.value(); value != nil {
			t.Fatalf("immature ATR value = %#v, want nil", value)
		}
		atr.push(15, 11, 12, 10, true)
		if value, ok := atr.currentValue(); !ok || value != 3.5 {
			t.Fatalf("ATR current = %v/%v, want 3.5/true", value, ok)
		}

		stddev := &rollingStdDevState{period: 2}
		stddev.push(2)
		if value, ok := stddev.currentValue(); ok || value != 0 {
			t.Fatalf("immature stddev = %v/%v, want 0/false", value, ok)
		}
		stddev.push(4)
		if value, ok := stddev.currentValue(); !ok || math.Abs(value-1) > 1e-9 {
			t.Fatalf("stddev current = %v/%v, want 1/true", value, ok)
		}
	})
}

func TestMACDStateBoundaryAccessors(t *testing.T) {
	if value := (*rollingMACDState)(nil).currentSignal(); value != 0 {
		t.Fatalf("nil MACD currentSignal = %v, want 0", value)
	}
	if _, ok := (*rollingMACDState)(nil).previousSignal(); ok {
		t.Fatal("nil MACD previousSignal should be unavailable")
	}

	onePeriod := newRollingMACDState(macdConfig{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1}, 4, nil)
	for _, close := range []float64{10, 11, 12} {
		onePeriod.push(close, false, 0, 0, false, false)
	}
	diff, ok := onePeriod.currentDiff()
	if !ok {
		t.Fatal("expected current MACD diff")
	}
	signal := onePeriod.currentSignal()
	if signal != diff {
		t.Fatalf("signal-period-one MACD signal = %v, want diff %v", signal, diff)
	}
	if previous, ok := onePeriod.previousSignal(); !ok || previous == signal {
		t.Fatalf("previous signal = %v/%v, want distinct previous diff", previous, ok)
	}

	immature := newRollingMACDState(macdConfig{fastPeriod: 2, slowPeriod: 3, signalPeriod: 2}, 4, nil)
	immature.push(10, false, 0, 0, false, false)
	if currentOK := func() bool {
		_, _, _, _, ok, _ := immature.snapshotValues()
		return ok
	}(); currentOK {
		t.Fatal("immature MACD snapshot should not be current-ready")
	}
}
