package indicatorruntime

import "testing"

func TestCoverage98IndicatorConfigParsingRejectsMalformedRiskAndTimeframeInputs(t *testing.T) {
	if _, ok := parseMovingAverageConfig([]string{"ma", "EMA", "0", "hour", "close"}); ok {
		t.Fatal("non-positive moving-average period was accepted")
	}
	if _, ok := parseMovingAverageConfig([]string{"ma", "EMA", "20", "hour", "not_a_source"}); ok {
		t.Fatal("unknown moving-average source was accepted")
	}
	if _, ok := parseMovingAverageConfig([]string{"ma", "EMA", "20", "not_a_timeframe", "close"}); ok {
		t.Fatal("unknown moving-average timeframe was accepted")
	}
	if config, ok := parseMovingAverageConfig([]string{"ma", "EMA", "20", "5m", "hlc3"}); !ok || config.timeUnit != "5m" || config.source != "hlc3" {
		t.Fatalf("numeric-minute moving-average config = %+v, %v", config, ok)
	}

	for _, parts := range [][]string{
		{"sl", "long"},
		{"sl", "long", "nope", "hour", "1"},
		{"sl", "long", "1", "unknown", "1"},
		{"risk", "unknown", "long", "1", "hour", "1", "continuous"},
		{"risk", "stopLoss", "long", "1", "hour", "0", "continuous"},
		{"risk", "stopLoss", "long", "1", "hour", "1", "unsupported"},
	} {
		if _, ok := parseStopLossConfig(parts); ok {
			t.Fatalf("malformed stop-loss config was accepted: %v", parts)
		}
	}
	if config, ok := parseStopLossConfig([]string{"risk", "takeProfit", "short", "2", "day", "1.5", "session"}); !ok || config.mode != "takeProfit" || config.direction != "short" || config.windowPolicy != "session" {
		t.Fatalf("valid risk window config = %+v, %v", config, ok)
	}
	if _, ok := parseStopLossTimeUnit("unexpected"); ok {
		t.Fatal("unexpected stop-loss time unit was accepted")
	}
	if _, ok := parseIndicatorTimeUnit("unexpected"); ok {
		t.Fatal("unexpected indicator time unit was accepted")
	}
	if got := resolveBarCount(5, "unexpected", 15); got != 5 {
		t.Fatalf("unknown time unit must retain period bars, got %d", got)
	}
}

func TestCoverage98RollingIndicatorStatesDistinguishUnwarmedAndInvalidSeries(t *testing.T) {
	if states := newRollingMovingAverageStates(indicatorRequirements{ma: []movingAverageConfig{
		{averageType: "EMA", period: 5}, {averageType: "SMA", period: 0}, {averageType: "SMA", period: 5, timeUnit: "5m"},
	}}, 1); states != nil {
		t.Fatalf("only non-streamable MA requirements must not allocate rolling states: %#v", states)
	}

	vwma := &rollingMovingAverageSnapshotState{kind: "VWMA", period: 2}
	vwma.push(10, 0)
	vwma.push(20, 0)
	if vwma.hasCurrent {
		t.Fatal("zero-volume VWMA must remain unavailable")
	}
	if value, ok := vwma.FieldValue("value"); !ok || value != nil {
		t.Fatalf("unwarmed VWMA value field = %#v, %v", value, ok)
	}
	if _, ok := vwma.FieldValue("unknown"); ok {
		t.Fatal("unknown MA field was accepted")
	}

	if state := newRollingMACDState(macdConfig{fastPeriod: 0, slowPeriod: 3, signalPeriod: 2}, 4, nil); state != nil {
		t.Fatal("invalid MACD periods allocated a rolling state")
	}
	macd := newRollingMACDState(macdConfig{fastPeriod: 1, slowPeriod: 1, signalPeriod: 1}, 1, nil)
	if macd == nil {
		t.Fatal("valid minimal MACD state was not allocated")
		return
	}
	macd.push(10, false, 0, 0, false, false)
	if _, ok := macd.previousSignal(); ok {
		t.Fatal("one-sample MACD must not report a previous signal")
	}
	if _, _, _, _, currentOK, _ := macd.snapshotValues(); currentOK {
		t.Fatal("one-sample MACD must remain unwarmed")
	}
	macd.push(11, false, 0, 0, false, false)
	if _, _, _, _, currentOK, _ := macd.snapshotValues(); currentOK {
		t.Fatal("MACD must remain unwarmed when retained history cannot reach its minimum")
	}
	readyMACD := newRollingMACDState(macdConfig{fastPeriod: 1, slowPeriod: 1, signalPeriod: 1}, 2, nil)
	readyMACD.push(10, false, 0, 0, false, false)
	readyMACD.push(11, false, 0, 0, false, false)
	if _, _, _, _, currentOK, _ := readyMACD.snapshotValues(); !currentOK {
		t.Fatal("MACD must become current once retained history reaches its minimum")
	}
	if macd.detectDivergence([]float64{10}, "unsupported", 1) {
		t.Fatal("unsupported MACD divergence direction was accepted")
	}

	if states := newRollingStochStates(indicatorRequirements{stoch: []sourcePeriodConfig{{source: "volume", period: 3}, {source: "close", period: 0}, {source: "close", period: 3, timeUnit: "hour"}}}); states != nil {
		t.Fatalf("invalid or aggregate stoch requirements allocated states: %#v", states)
	}
	if _, ok := calculateStochAt([]float64{1, 2}, []float64{2}, []float64{0, 1}, 2, 1); ok {
		t.Fatal("mismatched OHLC arrays produced a stochastic value")
	}
	if value, ok := calculateStochAt([]float64{1, 1}, []float64{1, 1}, []float64{1, 1}, 2, 1); !ok || value != 50 {
		t.Fatalf("flat stochastic range = %v, %v", value, ok)
	}
}
