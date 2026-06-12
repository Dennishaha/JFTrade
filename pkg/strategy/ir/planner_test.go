package ir_test

import (
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestPlanRequirementsCollectsPineIndicatorsAndRuntimeNeeds(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Mean Revert", overlay=true)
fast = request.security(syminfo.tickerid, "D", ta.ema(close, 5))
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
signal = ta.macd(close, 12, 26, 9)
if ta.crossover(fast, slow) and divergence_top(signal, 6)
    strategy.entry("Long", strategy.long, qty=(strategy.equity * 50 / 100) / close)
else
    strategy.exit("Long trail", "Long", trail_points=close * 4 / 100, trail_offset=close * 4 / 100)`)
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
		"ma:SMA:20:day",
		"macd:12:26:9",
		"risk:trailingStop:long:1:bar:4:continuous",
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
	if !requirements.RequiresTotalAccountValue {
		t.Fatal("RequiresTotalAccountValue = false, want true")
	}
}

func TestPlanRequirementsRejectsInvalidIndicatorBinding(t *testing.T) {
	program := programWithStatements(&strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 1},
		Name:       "fast",
		Expression: "ma(EMA, nope, day)",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid ma binding error")
	}
}

func TestPlanRequirementsRejectsInvalidMovingAverageType(t *testing.T) {
	program := programWithStatements(&strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 1},
		Name:       "fast",
		Expression: "ma(WILD, 5, day)",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid moving average type error")
	}
}

func TestPlanRequirementsRejectsUnsupportedOrderQuantityMode(t *testing.T) {
	program := programWithStatements(&strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 1},
		Action:             strategyir.OrderActionBuy,
		QuantityMode:       "bananas",
		QuantityExpression: "100",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want unsupported order quantity mode error")
	}
}

func TestPlanRequirementsRejectsInvalidProtectTimeUnit(t *testing.T) {
	program := programWithStatements(&strategyir.ProtectStmt{
		Range:                strategyir.SourceRange{StartLine: 1},
		Direction:            "auto",
		Mode:                 "trailing_stop",
		TimeValueExpression:  "2",
		TimeUnit:             "lunar",
		PercentageExpression: "4%",
		WindowPolicy:         "session",
	})
	if _, err := strategyir.PlanRequirements(program); err == nil {
		t.Fatal("PlanRequirements() error = nil, want invalid protect time unit error")
	}
}

func TestPlanRequirementsDetectsPositionVariablesInExpressions(t *testing.T) {
	program := programWithStatements(
		&strategyir.LetStmt{
			Range:      strategyir.SourceRange{StartLine: 1},
			Name:       "stopPrice",
			Expression: "position_avg_price * 0.95",
		},
		&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: 2},
			Condition: "position_size > 0",
		},
	)

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}
	if !requirements.RequiresPosition {
		t.Fatal("RequiresPosition = false, want true")
	}
}

func TestPlanRequirementsIndicatorKeysMatchRuntimeBindingParity(t *testing.T) {
	program := programWithStatements(
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "fast", Expression: "ma(EMA,14,minute)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "slow", Expression: "ma(SMA,20)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 3}, Name: "avg", Expression: "ma(ema,5,h)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 4}, Name: "r", Expression: "rsi(14)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 5}, Name: "m", Expression: "macd(12,26,9)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 6}, Name: "k", Expression: "kdj(9,3,3)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 7}, Name: "a", Expression: "atr(20)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 8}, Name: "c", Expression: "cci(14)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 9}, Name: "wr", Expression: "williams_r(14)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 10}, Name: "bInt", Expression: "bollinger(20,2)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 11}, Name: "bFloat", Expression: "bollinger(20,2.5)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 12}, Name: "sd", Expression: "stdev(20)"},
		&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: 13},
			Condition: "cross_over(fast, slow)",
			Then: []strategyir.Statement{&strategyir.ProtectStmt{
				Range:                strategyir.SourceRange{StartLine: 14},
				Direction:            "auto",
				Mode:                 "trailing_stop",
				TimeValueExpression:  "2",
				TimeUnit:             "day",
				PercentageExpression: "4%",
				WindowPolicy:         "session",
			}},
		},
	)

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	expectedKeys := map[string]bool{
		"ma:EMA:14:minute":                       true,
		"ma:SMA:20":                              true,
		"ma:EMA:5:hour":                          true,
		"rsi:14":                                 true,
		"macd:12:26:9":                           true,
		"kdj:9:3:3":                              true,
		"atr:20":                                 true,
		"cci:14":                                 true,
		"williamsr:14":                           true,
		"bollinger:20:2":                         true,
		"bollinger:20:2.5":                       true,
		"stdev:20":                               true,
		"risk:trailingStop:auto:2:day:4:session": true,
	}
	for _, ind := range requirements.Indicators {
		if !expectedKeys[ind.Key] {
			t.Fatalf("unexpected indicator key %q in plan", ind.Key)
		}
		delete(expectedKeys, ind.Key)
	}
	for key := range expectedKeys {
		t.Fatalf("missing indicator key %q in plan", key)
	}
}

func programWithStatements(statements ...strategyir.Statement) *strategyir.Program {
	return &strategyir.Program{
		SourceFormat: strategypine.SourceFormatPineV6,
		Hooks: []strategyir.HookBlock{{
			Kind:       strategyir.HookKLineClose,
			Statements: statements,
		}},
	}
}
