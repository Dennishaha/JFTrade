package ir_test

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestPlanRequirementsKeepsBranchLocalIndicatorAliases(t *testing.T) {
	program := programWithStatements(&strategyir.IfStmt{
		Range:     strategyir.SourceRange{StartLine: 1},
		Condition: "close > open",
		Then: []strategyir.Statement{
			&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "signal", Expression: "macd(12, 26, 9)"},
			&strategyir.IfStmt{Range: strategyir.SourceRange{StartLine: 3}, Condition: "divergence_top(signal, 4)"},
		},
		Else: []strategyir.Statement{
			&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 4}, Name: "signal", Expression: "kdj(9, 3, 3)"},
			&strategyir.IfStmt{Range: strategyir.SourceRange{StartLine: 5}, Condition: "divergence_bottom(signal, 5)"},
		},
	})

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	keys := requirementKeySet(requirements)
	for _, key := range []string{
		"macd:12:26:9",
		"kdj:9:3:3",
		"divergence:macd:12:26:9:top:4",
		"divergence:kdj:9:3:3:bottom:5",
	} {
		if !keys[key] {
			t.Fatalf("missing branch-local indicator key %q in %#v", key, requirements.Indicators)
		}
	}
}

func TestPlanRequirementsCollectsLoopObjectAndExitExpressions(t *testing.T) {
	program := programWithStatements(&strategyir.LoopStmt{
		Range:           strategyir.SourceRange{StartLine: 10},
		StartExpression: "ma(SMA, 10)",
		EndExpression:   "highest(high, 20)",
		StepExpression:  "1",
		WhileCondition:  "equity > 0 and stdev(close, 10)",
		Body: []strategyir.Statement{
			&strategyir.ObjectStmt{
				Range: strategyir.SourceRange{StartLine: 11},
				Arguments: []string{
					"variance(close, 10)",
					"position_avg_price",
				},
			},
			&strategyir.ExitStmt{
				Range:              strategyir.SourceRange{StartLine: 12},
				QuantityMode:       "shares",
				QuantityExpression: "rsi(close, 7)",
				StopExpression:     "supertrend(3, 10)",
				LimitExpression:    "vwap(hlc3)",
				TrailPrice:         "sar(0.02, 0.02, 0.2)",
				TrailPoints:        "mfi(hlc3, 14)",
				TrailOffset:        "security_source(close, day)",
			},
		},
	})

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}
	if !requirements.RequiresPosition {
		t.Fatal("RequiresPosition = false, want true from object/exit expressions")
	}
	if !requirements.RequiresTotalAccountValue {
		t.Fatal("RequiresTotalAccountValue = false, want true from loop condition")
	}

	keys := requirementKeySet(requirements)
	for _, key := range []string{
		"ma:SMA:10",
		"highest:high:20",
		"stdev:10",
		"variance:close:10",
		"rsi:7",
		"supertrend:3:10",
		"vwap:hlc3",
		"sar:0.02:0.02:0.2",
		"mfi:hlc3:14",
		"security_source:day:close",
	} {
		if !keys[key] {
			t.Fatalf("missing collected expression key %q in %#v", key, requirements.Indicators)
		}
	}
}

func TestPlanRequirementsRejectsBusinessInvalidIndicatorParameters(t *testing.T) {
	tests := []struct {
		name       string
		statement  strategyir.Statement
		wantDetail string
	}{
		{
			name: "percentile above range",
			statement: &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: 20},
				Name:       "p",
				Expression: "percentile_nearest_rank(close, 20, 101)",
			},
			wantDetail: "percentage must be between 0 and 100",
		},
		{
			name: "kc boolean",
			statement: &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: 21},
				Name:       "channel",
				Expression: "kc(close, 20, 1.5, maybe)",
			},
			wantDetail: "useTrueRange must be boolean",
		},
		{
			name: "security source negative lookback",
			statement: &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: 22},
				Name:       "higher",
				Expression: "security_source(close, day, -1)",
			},
			wantDetail: "lookback must be a non-negative integer",
		},
		{
			name: "supertrend zero factor",
			statement: &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: 23},
				Name:       "trend",
				Expression: "supertrend(0, 10)",
			},
			wantDetail: "factor must be a positive number",
		},
		{
			name: "exit quantity mode",
			statement: &strategyir.ExitStmt{
				Range:        strategyir.SourceRange{StartLine: 24},
				QuantityMode: "cash_percent",
			},
			wantDetail: "unsupported exit quantity mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := strategyir.PlanRequirements(programWithStatements(tt.statement))
			if err == nil {
				t.Fatal("PlanRequirements() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tt.wantDetail) {
				t.Fatalf("PlanRequirements() error = %v, want detail %q", err, tt.wantDetail)
			}
		})
	}
}

func TestPlanRequirementsRejectsNilAndUnsupportedStatements(t *testing.T) {
	if _, err := strategyir.PlanRequirements(nil); err == nil {
		t.Fatal("PlanRequirements(nil) error = nil, want required program error")
	}

	_, err := strategyir.PlanRequirements(programWithStatements(unsupportedPlannerStatement{}))
	if err == nil {
		t.Fatal("PlanRequirements() error = nil, want unsupported statement error")
	}
	if !strings.Contains(err.Error(), "unsupported IR statement type") {
		t.Fatalf("unsupported statement error = %v", err)
	}
}

type unsupportedPlannerStatement struct{}

func (unsupportedPlannerStatement) Kind() strategyir.StatementKind {
	return "unsupported"
}

func (unsupportedPlannerStatement) SourceRange() strategyir.SourceRange {
	return strategyir.SourceRange{StartLine: 99}
}

func requirementKeySet(requirements strategyir.Requirements) map[string]bool {
	keys := map[string]bool{}
	for _, indicator := range requirements.Indicators {
		keys[indicator.Key] = true
	}
	return keys
}
