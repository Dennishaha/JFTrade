package runtimecontrol

import (
	"testing"
	"time"
)

func TestOptionalRuntimeControlValuesCoverTimeAndPositionEdges(t *testing.T) {
	if OptionalTime(time.Time{}) != nil || ObservationTime(time.Time{}) != nil {
		t.Fatal("zero observation time should be omitted")
	}
	local := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.FixedZone("UTC+8", 8*60*60))
	if got := OptionalTime(local); got == nil || got.Location() != time.UTC || !got.Equal(local.UTC()) {
		t.Fatalf("OptionalTime(local) = %v", got)
	}
	if got := MaxTime(local, local.Add(-time.Hour)); !got.Equal(local) {
		t.Fatalf("MaxTime(left newest) = %v", got)
	}
	if got := MaxTime(local, local.Add(time.Hour)); !got.Equal(local.Add(time.Hour)) {
		t.Fatalf("MaxTime(right newest) = %v", got)
	}
	if OptionalString("   ") != nil {
		t.Fatal("OptionalString(blank) should be nil")
	}
	if got := OptionalString("  reason "); got == nil || *got != "reason" {
		t.Fatalf("OptionalString(trimmed) = %v", got)
	}
	if PositionMatchesSymbol(Position{}, "US.AAPL") || PositionMatchesSymbol(Position{Symbol: "AAPL"}, " ") {
		t.Fatal("blank position or strategy symbol should not match")
	}
}

func TestEvaluateRiskOffModeIgnoresConfiguredLimits(t *testing.T) {
	limit := 1.0
	decision := EvaluateRisk(RiskSettings{Mode: ModeOff, MaxOrderQuantity: &limit}, OrderIntent{Symbol: "US.AAPL", Quantity: 10}, RiskContext{})
	if decision != (RiskDecision{}) {
		t.Fatalf("off-mode decision = %+v, want zero decision", decision)
	}
}
