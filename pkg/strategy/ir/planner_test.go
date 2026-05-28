package ir_test

import (
	"testing"

	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestPlanRequirementsCollectsIndicatorsAndRuntimeNeeds(t *testing.T) {
	script := `strategy Mean Revert
version 0.1.0
symbol 00700
interval 1m

on kline_close:
  let fast = ma(EMA, 5, day)
  let slow = ma(MA, 20, day)
  let signal = macd(12, 26, 9)
  if cross_over(fast, slow) and divergence_top(signal, 6):
    buy cash_percent 50 policy same_direction
  else:
    protect auto trailing_stop 2 day 4% window session`

	program, err := strategydsl.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	expectedKeys := []string{
		"divergence:macd:12:26:9:top:6",
		"ma:EMA:5:day",
		"ma:MA:20:day",
		"macd:12:26:9",
		"risk:trailingStop:auto:2:day:4:session",
	}
	if len(requirements.Indicators) != len(expectedKeys) {
		t.Fatalf("len(requirements.Indicators) = %d, want %d", len(requirements.Indicators), len(expectedKeys))
	}
	for index, key := range expectedKeys {
		if requirements.Indicators[index].Key != key {
			t.Fatalf("requirements.Indicators[%d].Key = %q, want %q", index, requirements.Indicators[index].Key, key)
		}
	}
	if !requirements.RequiresPosition {
		t.Fatal("RequiresPosition = false, want true")
	}
	if !requirements.RequiresAvailableCash {
		t.Fatal("RequiresAvailableCash = false, want true")
	}
	if requirements.RequiresMarginBuyingPower {
		t.Fatal("RequiresMarginBuyingPower = true, want false")
	}
	if requirements.RequiresShortSellingPower {
		t.Fatal("RequiresShortSellingPower = true, want false")
	}
	if requirements.RequiresTotalAccountValue {
		t.Fatal("RequiresTotalAccountValue = true, want false")
	}
}

func TestPlanRequirementsRejectsInvalidIndicatorBinding(t *testing.T) {
	script := `on kline_close:
  let fast = ma(EMA, nope, day)
  log "x"`

	program, err := strategydsl.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid ma binding error")
	}
}

func TestPlanRequirementsRejectsInvalidMovingAverageType(t *testing.T) {
	script := `on kline_close:
  let fast = ma(WILD, 5, day)
  log "x"`

	program, err := strategydsl.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid moving average type error")
	}
}

func TestPlanRequirementsRejectsUnsupportedOrderQuantityMode(t *testing.T) {
	script := `on kline_close:
  buy bananas 100`

	program, err := strategydsl.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want unsupported order quantity mode error")
	}
}

func TestPlanRequirementsRejectsInvalidProtectTimeUnit(t *testing.T) {
	script := `on kline_close:
  protect auto trailing_stop 2 lunar 4% window session`

	program, err := strategydsl.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid protect time unit error")
	}
}
