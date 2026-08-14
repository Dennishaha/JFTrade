package backtest

import (
	"math"
	"testing"
)

func TestPinetsShadowEMAUsesSMAInitialization(t *testing.T) {
	got := pinetsShadowEMA([]float64{1, 2, 3, 4, 5}, 3)
	if !math.IsNaN(got[0]) || !math.IsNaN(got[1]) {
		t.Fatalf("EMA warmup = %#v, want leading NaN values", got[:2])
	}
	want := []float64{2, 3, 4}
	for index, expected := range want {
		if got[index+2] != expected {
			t.Fatalf("EMA[%d] = %v, want %v", index+2, got[index+2], expected)
		}
	}
}

func TestPinetsShadowMACDSkipsNaNValuesForSignalInitialization(t *testing.T) {
	macd, signal, hist := pinetsShadowMACD([]float64{1, 2, 3, 4, 5, 6, 7}, 3, 5, 2)

	for index := 0; index < 4; index++ {
		if !math.IsNaN(macd[index]) {
			t.Fatalf("MACD[%d] = %v, want NaN before slow EMA is initialized", index, macd[index])
		}
		if !math.IsNaN(signal[index]) || !math.IsNaN(hist[index]) {
			t.Fatalf("MACD derived values at %d = signal:%v hist:%v, want NaN", index, signal[index], hist[index])
		}
	}
	if macd[4] != 1 || !math.IsNaN(signal[4]) || !math.IsNaN(hist[4]) {
		t.Fatalf("first valid MACD values = %v/%v/%v, want 1/NaN/NaN", macd[4], signal[4], hist[4])
	}
	for index := 5; index < 7; index++ {
		if macd[index] != 1 || signal[index] != 1 || hist[index] != 0 {
			t.Fatalf("MACD[%d] = %v/%v/%v, want 1/1/0", index, macd[index], signal[index], hist[index])
		}
	}
}
