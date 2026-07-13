package pineworker

import (
	"encoding/json"
	"math"
	"testing"
)

func TestJSONSizeMatchesMarshalAcrossPayloadShapes(t *testing.T) {
	tests := []any{nil, map[string]string{}, map[string]string{"quoted": "a\"b", "unicode": "交易"}, []int{1, 2, 3}}
	for _, value := range tests {
		want, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("Marshal(%#v): %v", value, err)
		}
		got, err := jsonSize(value)
		if err != nil {
			t.Fatalf("jsonSize(%#v): %v", value, err)
		}
		if got != len(want) {
			t.Fatalf("jsonSize(%#v) = %d, want %d", value, got, len(want))
		}
	}
}

func TestEstimateRunScriptRequestJSONSizeHandlesNilAndEmptyCollections(t *testing.T) {
	base := RunScriptRequest{JobID: "job", Source: "source", Symbol: "US.AAPL", Timeframe: "1m", Mode: ModeAnalyze}
	empty := base
	empty.Candles = []Candle{}
	empty.Params = map[string]string{}
	for _, request := range []RunScriptRequest{base, empty} {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		got, err := estimateRunScriptRequestJSONSize(request)
		if err != nil {
			t.Fatalf("estimateRunScriptRequestJSONSize: %v", err)
		}
		if got != len(encoded) {
			t.Fatalf("estimated size = %d, want %d", got, len(encoded))
		}
	}
	if got := estimateKnownJSONObjectSize(nil); got != 2 {
		t.Fatalf("empty object size = %d", got)
	}
}

func TestEstimateCandleJSONSizeRejectsNonFiniteFields(t *testing.T) {
	valid := Candle{OpenTime: 1, CloseTime: 2, Open: 1, High: 2, Low: 0, Close: 1, Volume: 1}
	fields := []func(*Candle){
		func(candle *Candle) { candle.Open = math.NaN() },
		func(candle *Candle) { candle.High = math.Inf(1) },
		func(candle *Candle) { candle.Low = math.Inf(-1) },
		func(candle *Candle) { candle.Close = math.NaN() },
		func(candle *Candle) { candle.Volume = math.Inf(1) },
	}
	for _, mutate := range fields {
		candle := valid
		mutate(&candle)
		if _, err := estimateCandleJSONSize(candle); err == nil {
			t.Fatalf("estimateCandleJSONSize(%#v) succeeded", candle)
		}
		if _, err := estimateCandlesJSONSize([]Candle{candle}); err == nil {
			t.Fatalf("estimateCandlesJSONSize(%#v) succeeded", candle)
		}
	}
}
