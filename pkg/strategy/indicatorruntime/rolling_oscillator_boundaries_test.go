package indicatorruntime

import "testing"

func TestRollingCCIStateAndRuntimeFallbackBusinessBoundaries(t *testing.T) {
	if states := newRollingCCIStates(indicatorRequirements{}); states != nil {
		t.Fatalf("empty CCI requirements = %#v, want nil", states)
	}
	states := newRollingCCIStates(indicatorRequirements{cci: []int{0, 3}})
	if len(states) != 1 || states[3] == nil {
		t.Fatalf("CCI states = %#v, want only period 3", states)
	}

	var nilState *rollingCCIState
	nilState.push(10)
	if value, ok := nilState.currentValue(); ok || value != 0 {
		t.Fatalf("nil CCI current = %v/%v", value, ok)
	}

	state := &rollingCCIState{period: 3}
	for _, value := range []float64{10, 10, 10} {
		state.push(value)
	}
	if value, ok := state.currentValue(); !ok || value != 0 {
		t.Fatalf("flat CCI = %v/%v, want 0/true", value, ok)
	}
	state.push(13)
	if value := state.value(); value == nil {
		t.Fatal("CCI value() = nil after full rolling window")
	}

	runtime := &indicatorRuntime{cciStates: states}
	runtime.pushCCIStates(12, 9, 10)
	runtime.pushCCIStates(13, 10, 11)
	runtime.pushCCIStates(14, 11, 12)
	if value, ok := runtime.cciSnapshotValue(3); !ok || value == 0 {
		t.Fatalf("runtime CCI state snapshot = %v/%v", value, ok)
	}
	fallback := &indicatorRuntime{
		highs:  []float64{12, 13, 14},
		lows:   []float64{9, 10, 11},
		closes: []float64{10, 11, 12},
	}
	if value, ok := fallback.cciSnapshotValue(3); !ok || value == 0 {
		t.Fatalf("runtime CCI fallback snapshot = %v/%v", value, ok)
	}
	if value, ok := ((*indicatorRuntime)(nil)).cciSnapshotValue(3); ok || value != 0 {
		t.Fatalf("nil runtime CCI snapshot = %v/%v", value, ok)
	}
}

func TestRollingWilliamsRStateAndRuntimeFallbackBusinessBoundaries(t *testing.T) {
	if states := newRollingWilliamsRStates(indicatorRequirements{}); states != nil {
		t.Fatalf("empty WilliamsR requirements = %#v, want nil", states)
	}
	states := newRollingWilliamsRStates(indicatorRequirements{williamsR: []int{-1, 3}})
	if len(states) != 1 || states[3] == nil {
		t.Fatalf("WilliamsR states = %#v, want only period 3", states)
	}

	var nilState *rollingWilliamsRState
	nilState.push(10, 10, 10)
	if value, ok := nilState.currentValue(); ok || value != 0 {
		t.Fatalf("nil WilliamsR current = %v/%v", value, ok)
	}

	state := &rollingWilliamsRState{period: 3}
	for range 3 {
		state.push(10, 10, 10)
	}
	if value, ok := state.currentValue(); !ok || value != -50 {
		t.Fatalf("flat WilliamsR = %v/%v, want -50/true", value, ok)
	}
	state.push(15, 9, 12)
	if value := state.value(); value == nil {
		t.Fatal("WilliamsR value() = nil after full rolling window")
	}

	runtime := &indicatorRuntime{williamsRStates: states}
	runtime.pushWilliamsRStates(12, 9, 10)
	runtime.pushWilliamsRStates(13, 10, 11)
	runtime.pushWilliamsRStates(14, 11, 12)
	if value, ok := runtime.williamsRSnapshotValue(3); !ok || value >= 0 {
		t.Fatalf("runtime WilliamsR state snapshot = %v/%v", value, ok)
	}
	fallback := &indicatorRuntime{
		highs:  []float64{12, 13, 14},
		lows:   []float64{9, 10, 11},
		closes: []float64{10, 11, 12},
	}
	if value, ok := fallback.williamsRSnapshotValue(3); !ok || value >= 0 {
		t.Fatalf("runtime WilliamsR fallback snapshot = %v/%v", value, ok)
	}
	if value, ok := ((*indicatorRuntime)(nil)).williamsRSnapshotValue(3); ok || value != 0 {
		t.Fatalf("nil runtime WilliamsR snapshot = %v/%v", value, ok)
	}
}
