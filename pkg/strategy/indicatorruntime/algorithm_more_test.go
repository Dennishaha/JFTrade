package indicatorruntime

import (
	"math"
	"reflect"
	"testing"
)

func TestSelectedSeriesAveragesRespectSelectionOrderAndVolume(t *testing.T) {
	values := []float64{10, 20, 30, 40}
	volumes := []float64{1, 2, 3, 4}
	selected := []int{3, 1, 0}

	if got, ok := simpleMovingAverageFromSelected(values, selected); !ok || got != (40+20+10)/3.0 {
		t.Fatalf("simpleMovingAverageFromSelected() = (%v, %v)", got, ok)
	}
	if got, ok := simpleMovingAverageFromSelected(values, nil); ok || got != 0 {
		t.Fatalf("simpleMovingAverageFromSelected(nil) = (%v, %v), want (0, false)", got, ok)
	}

	weighted, ok := linearWeightedMovingAverageFromSelected(values, selected, len(selected))
	if !ok {
		t.Fatal("linearWeightedMovingAverageFromSelected() ok = false")
	}
	wantWeighted := (10*1 + 20*2 + 40*3) / 6.0
	if math.Abs(weighted-wantWeighted) > 1e-9 {
		t.Fatalf("linearWeightedMovingAverageFromSelected() = %v, want %v", weighted, wantWeighted)
	}
	if got, ok := linearWeightedMovingAverageFromSelected(values, selected[:2], 3); ok || got != 0 {
		t.Fatalf("linearWeightedMovingAverageFromSelected(short) = (%v, %v), want (0, false)", got, ok)
	}

	volumeWeighted, ok := volumeWeightedMovingAverageFromSelected(values, volumes, selected)
	if !ok {
		t.Fatal("volumeWeightedMovingAverageFromSelected() ok = false")
	}
	wantVWMA := (10*1 + 20*2 + 40*4) / float64(1+2+4)
	if math.Abs(volumeWeighted-wantVWMA) > 1e-9 {
		t.Fatalf("volumeWeightedMovingAverageFromSelected() = %v, want %v", volumeWeighted, wantVWMA)
	}
	if got, ok := volumeWeightedMovingAverageFromSelected(values, []float64{1}, selected); ok || got != 0 {
		t.Fatalf("volumeWeightedMovingAverageFromSelected(short volumes) = (%v, %v), want (0, false)", got, ok)
	}
	if got, ok := volumeWeightedMovingAverageFromSelected(values, []float64{0, 0, 0, 0}, selected); ok || got != 0 {
		t.Fatalf("volumeWeightedMovingAverageFromSelected(zero volume) = (%v, %v), want (0, false)", got, ok)
	}
}

func TestCCIHelpersComputeSeriesAndGracefulEdgeCases(t *testing.T) {
	highs := []float64{11, 12, 13, 14, 15}
	lows := []float64{9, 10, 11, 12, 13}
	closes := []float64{10, 11, 12, 13, 14}

	series := calculateCCISeries(highs, lows, closes, 3)
	if len(series) != 3 {
		t.Fatalf("calculateCCISeries() len = %d, want 3", len(series))
	}
	last := calculateCCI(highs, lows, closes, 3)
	if last == nil || math.Abs(last.(float64)-series[len(series)-1]) > 1e-9 {
		t.Fatalf("calculateCCI() = %#v, want %v", last, series[len(series)-1])
	}

	fromValues, ok := calculateCCIFromValues([]float64{10, 12, 14}, 3)
	if !ok {
		t.Fatal("calculateCCIFromValues() ok = false")
	}
	if math.Abs(fromValues-100) > 1e-9 {
		t.Fatalf("calculateCCIFromValues() = %v, want 100", fromValues)
	}

	flatCCI, ok := calculateCCIFromValues([]float64{5, 5, 5}, 3)
	if !ok || flatCCI != 0 {
		t.Fatalf("calculateCCIFromValues(flat) = (%v, %v), want (0, true)", flatCCI, ok)
	}
	if got := calculateCCISeries(highs[:2], lows[:2], closes[:2], 3); got != nil {
		t.Fatalf("calculateCCISeries(short) = %#v, want nil", got)
	}
}

func TestSupertrendHelpersCaptureTrendDirectionAndInvalidInput(t *testing.T) {
	upHighs := []float64{10, 11, 12, 13, 14, 15}
	upLows := []float64{9, 10, 11, 12, 13, 14}
	upCloses := []float64{9.5, 10.5, 11.5, 12.5, 13.5, 14.5}
	config := supertrendConfig{factor: 2, atrPeriod: 2}

	line, direction, ok := calculateSupertrendValues(upHighs, upLows, upCloses, config)
	if !ok {
		t.Fatal("calculateSupertrendValues(uptrend) ok = false")
	}
	if direction != 1 {
		t.Fatalf("calculateSupertrendValues(uptrend) direction = %v, want 1", direction)
	}
	if line >= upCloses[len(upCloses)-1] {
		t.Fatalf("calculateSupertrendValues(uptrend) line = %v, want below latest close %v", line, upCloses[len(upCloses)-1])
	}
	snapshot := calculateSupertrendSnapshot(upHighs, upLows, upCloses, config)
	if !reflect.DeepEqual(snapshot, map[string]any{"line": line, "direction": direction}) {
		t.Fatalf("calculateSupertrendSnapshot(uptrend) = %#v", snapshot)
	}

	downHighs := []float64{10, 11, 12, 13, 10, 9}
	downLows := []float64{9, 10, 11, 12, 7, 6}
	downCloses := []float64{9.5, 10.5, 11.5, 12.5, 7.5, 6.5}
	line, direction, ok = calculateSupertrendValues(downHighs, downLows, downCloses, config)
	if !ok {
		t.Fatal("calculateSupertrendValues(downtrend) ok = false")
	}
	if direction != -1 {
		t.Fatalf("calculateSupertrendValues(downtrend) direction = %v, want -1", direction)
	}
	if line <= downCloses[len(downCloses)-1] {
		t.Fatalf("calculateSupertrendValues(downtrend) line = %v, want above latest close %v", line, downCloses[len(downCloses)-1])
	}

	if snapshot := calculateSupertrendSnapshot(upHighs[:2], upLows[:2], upCloses[:2], supertrendConfig{}); snapshot != nil {
		t.Fatalf("calculateSupertrendSnapshot(invalid) = %#v, want nil", snapshot)
	}
}
