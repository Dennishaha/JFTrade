package indicatorruntime

import (
	"math"
	"testing"
)

func TestMonotonicWindowDequeKeepsExtremaAfterExpiredBarsAreCompacted(t *testing.T) {
	var highs monotonicWindowValueDeque
	var lows monotonicWindowValueDeque
	for index, bar := range []struct {
		high float64
		low  float64
	}{
		{high: 12, low: 8},
		{high: 11, low: 7},
		{high: 14, low: 9},
		{high: 13, low: 6},
		{high: 10, low: 5},
	} {
		highs.pushMax(index, bar.high)
		lows.pushMin(index, bar.low)
	}

	highs.popExpired(3)
	lows.popExpired(3)
	if value, ok := highs.frontValue(); !ok || value != 13 {
		t.Fatalf("window high after expiry = (%v, %v), want (13, true)", value, ok)
	}
	if value, ok := lows.frontValue(); !ok || value != 5 {
		t.Fatalf("window low after expiry = (%v, %v), want (5, true)", value, ok)
	}

	highs.popExpired(5)
	lows.popExpired(5)
	if value, ok := highs.frontValue(); ok || value != 0 {
		t.Fatalf("expired high deque = (%v, %v), want (0, false)", value, ok)
	}
	if value, ok := lows.frontValue(); ok || value != 0 {
		t.Fatalf("expired low deque = (%v, %v), want (0, false)", value, ok)
	}
}

func TestRollingFloatWindowPreservesNewestBarsAndResetsWhenCapacityChanges(t *testing.T) {
	var window rollingFloatWindow
	for _, value := range []float64{100, 101, 102} {
		if evicted, ok := window.push(value, 3); ok || evicted != 0 {
			t.Fatalf("warmup push evicted (%v, %v), want (0, false)", evicted, ok)
		}
	}
	if evicted, ok := window.push(103, 3); !ok || evicted != 100 {
		t.Fatalf("rolling push evicted (%v, %v), want (100, true)", evicted, ok)
	}
	if value, ok := window.at(0); !ok || value != 101 {
		t.Fatalf("oldest retained value = (%v, %v), want (101, true)", value, ok)
	}
	if value, ok := window.last(); !ok || value != 103 {
		t.Fatalf("latest value = (%v, %v), want (103, true)", value, ok)
	}

	if evicted, ok := window.push(200, 2); ok || evicted != 0 {
		t.Fatalf("capacity-change push evicted (%v, %v), want reset warmup", evicted, ok)
	}
	if got := window.len(); got != 1 {
		t.Fatalf("window len after capacity change = %d, want 1", got)
	}
	if value, ok := window.at(1); ok || value != 0 {
		t.Fatalf("out-of-range value = (%v, %v), want (0, false)", value, ok)
	}
}

func TestIndexDequeMaintainsWindowExtremaIndexes(t *testing.T) {
	values := []float64{10, 12, 11, 9}
	var highs indexDeque
	highs.reset(2)
	highs.pushMax(values, 0)
	highs.pushMax(values, 1)
	highs.pushMax(values, 2)
	if front := highs.front(); front != 1 {
		t.Fatalf("max deque front = %d, want index 1", front)
	}
	highs.popExpired(2)
	if front := highs.front(); front != 2 {
		t.Fatalf("max deque after expiry = %d, want index 2", front)
	}

	var lows indexDeque
	lows.reset(2)
	for index := range values {
		lows.pushMin(values, index)
	}
	if front := lows.front(); front != 3 {
		t.Fatalf("min deque front = %d, want index 3", front)
	}
	lows.reset(0)
	if front := lows.front(); front != 0 {
		t.Fatalf("empty deque front = %d, want 0", front)
	}
}

func TestRollingEMAStateMatchesRecomputedWindowWhenOldBarsAreTrimmed(t *testing.T) {
	state := newRollingEMATailState(3, 4, 3)
	for _, closeValue := range []float64{10, 13, 16, 19} {
		state.push(closeValue, false, 0, 0, false, false)
	}

	state.push(22, true, 10, 13, true, true)
	current, previous, currentOK, previousOK := state.snapshotValues()
	assertRollingStateApprox(t, current, 19.375)
	assertRollingStateApprox(t, previous, 16.75)
	if !currentOK || !previousOK {
		t.Fatalf("trimmed EMA availability = current:%v previous:%v, want both true", currentOK, previousOK)
	}
}

func TestRollingEMAStateResetsWhenTrimmedHistoryCannotBeAdjusted(t *testing.T) {
	state := newRollingEMATailState(3, 4, 2)
	for _, closeValue := range []float64{10, 13, 16} {
		state.push(closeValue, false, 0, 0, false, false)
	}

	state.push(20, true, 10, 0, true, false)
	current, previous, currentOK, previousOK := state.snapshotValues()
	if current != 20 || currentOK {
		t.Fatalf("reset EMA current = (%v, %v), want (20, false until rewarmed)", current, currentOK)
	}
	if previous != 0 || previousOK {
		t.Fatalf("reset EMA previous = (%v, %v), want (0, false)", previous, previousOK)
	}
}

func TestRollingEMAStateUsesMathematicalPowerBeyondPrecomputedLimit(t *testing.T) {
	state := newRollingEMATailState(3, 2, 1)
	assertRollingStateApprox(t, state.powerAt(5), math.Pow(0.5, 5))
	if value := (*rollingEMATailState)(nil).powerAt(1); value != 0 {
		t.Fatalf("nil EMA power = %v, want 0", value)
	}
}

func assertRollingStateApprox(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("value = %v, want %v", got, want)
	}
}
