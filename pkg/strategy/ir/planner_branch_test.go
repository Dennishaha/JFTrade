package ir_test

import (
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestPlanRequirementsCollectsLoopExitDivergenceAndLegacyProtectKeys(t *testing.T) {
	program := programWithStatements(
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "r", Expression: "rsi(14)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "k", Expression: "kdj(9,3,3)"},
		&strategyir.LoopStmt{
			Range:           strategyir.SourceRange{StartLine: 3},
			Variable:        "i",
			StartExpression: "security_source(close, day, 1)",
			EndExpression:   "atr(14, week)",
			StepExpression:  "equity",
			WhileCondition:  "position_size > 0 and macd(12,26,9,day,hlc3) > 0",
			Body: []strategyir.Statement{
				&strategyir.ObjectStmt{
					Range:     strategyir.SourceRange{StartLine: 4},
					Arguments: []string{"bollinger(20,2,week,ohlc4)", "stoch(hlc3, high, low, 14, day)"},
				},
				&strategyir.IfStmt{
					Range:     strategyir.SourceRange{StartLine: 5},
					Condition: "divergence_top(r, 5) or divergence_bottom(k, 3)",
					Then: []strategyir.Statement{
						&strategyir.ExitStmt{
							Range:              strategyir.SourceRange{StartLine: 6},
							QuantityMode:       "account_position_percent",
							QuantityExpression: "equity * 10 / 100",
							StopExpression:     "security_source(close, week, 2)",
							LimitExpression:    "anchored_vwap(hlc3, month)",
							TrailPrice:         "cum(volume)",
							TrailPoints:        "vwap(close)",
							TrailOffset:        "supertrend(3, 10, day)",
						},
						&strategyir.BreakStmt{Range: strategyir.SourceRange{StartLine: 7}},
						&strategyir.ContinueStmt{Range: strategyir.SourceRange{StartLine: 8}},
					},
					Else: []strategyir.Statement{
						&strategyir.CancelStmt{Range: strategyir.SourceRange{StartLine: 9}, All: true},
						&strategyir.LogStmt{Range: strategyir.SourceRange{StartLine: 10}, Message: "skip branch"},
						&strategyir.NotifyStmt{Range: strategyir.SourceRange{StartLine: 11}, Message: "notify branch"},
					},
				},
			},
		},
		&strategyir.ProtectStmt{
			Range:                strategyir.SourceRange{StartLine: 12},
			Direction:            "long",
			Mode:                 "stop_loss",
			TimeValueExpression:  "3",
			TimeUnit:             "",
			PercentageExpression: "5%",
			WindowPolicy:         "continuous",
		},
	)

	requirements, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	if !requirements.RequiresPosition {
		t.Fatal("RequiresPosition = false, want true")
	}
	if !requirements.RequiresTotalAccountValue {
		t.Fatal("RequiresTotalAccountValue = false, want true")
	}

	keys := map[string]bool{}
	for _, indicator := range requirements.Indicators {
		keys[indicator.Key] = true
	}
	for _, key := range []string{
		"rsi:14",
		"kdj:9:3:3",
		"security_source:day:close:1",
		"atr:14:week",
		"macd:hlc3:12:26:9:day",
		"bollinger:ohlc4:20:2:week",
		"stoch:hlc3:14:day",
		"divergence:rsi:14:top:5",
		"divergence:kdj:9:3:3:bottom:3",
		"security_source:week:close:2",
		"anchored_vwap:month:hlc3",
		"cum:volume",
		"vwap:close",
		"supertrend:3:10:day",
		"sl:long:3:bar:5",
	} {
		if !keys[key] {
			t.Fatalf("missing planned requirement key %q in %#v", key, requirements.Indicators)
		}
	}
}

func TestIRStatementKindsAndSourceRangesStayStableForDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		stmt strategyir.Statement
		kind strategyir.StatementKind
		rng  strategyir.SourceRange
	}{
		{name: "let", stmt: &strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}}, kind: strategyir.StatementKindLet, rng: strategyir.SourceRange{StartLine: 1}},
		{name: "collection", stmt: &strategyir.CollectionStmt{Range: strategyir.SourceRange{StartLine: 2}}, kind: strategyir.StatementKindCollection, rng: strategyir.SourceRange{StartLine: 2}},
		{name: "tuple", stmt: &strategyir.TupleStmt{Range: strategyir.SourceRange{StartLine: 3}}, kind: strategyir.StatementKindTuple, rng: strategyir.SourceRange{StartLine: 3}},
		{name: "loop", stmt: &strategyir.LoopStmt{Range: strategyir.SourceRange{StartLine: 4}}, kind: strategyir.StatementKindLoop, rng: strategyir.SourceRange{StartLine: 4}},
		{name: "break", stmt: &strategyir.BreakStmt{Range: strategyir.SourceRange{StartLine: 5}}, kind: strategyir.StatementKindBreak, rng: strategyir.SourceRange{StartLine: 5}},
		{name: "continue", stmt: &strategyir.ContinueStmt{Range: strategyir.SourceRange{StartLine: 6}}, kind: strategyir.StatementKindContinue, rng: strategyir.SourceRange{StartLine: 6}},
		{name: "object", stmt: &strategyir.ObjectStmt{Range: strategyir.SourceRange{StartLine: 7}}, kind: strategyir.StatementKindObject, rng: strategyir.SourceRange{StartLine: 7}},
		{name: "if", stmt: &strategyir.IfStmt{Range: strategyir.SourceRange{StartLine: 8}}, kind: strategyir.StatementKindIf, rng: strategyir.SourceRange{StartLine: 8}},
		{name: "log", stmt: &strategyir.LogStmt{Range: strategyir.SourceRange{StartLine: 9}}, kind: strategyir.StatementKindLog, rng: strategyir.SourceRange{StartLine: 9}},
		{name: "notify", stmt: &strategyir.NotifyStmt{Range: strategyir.SourceRange{StartLine: 10}}, kind: strategyir.StatementKindNotify, rng: strategyir.SourceRange{StartLine: 10}},
		{name: "order", stmt: &strategyir.OrderStmt{Range: strategyir.SourceRange{StartLine: 11}}, kind: strategyir.StatementKindOrder, rng: strategyir.SourceRange{StartLine: 11}},
		{name: "exit", stmt: &strategyir.ExitStmt{Range: strategyir.SourceRange{StartLine: 12}}, kind: strategyir.StatementKindExit, rng: strategyir.SourceRange{StartLine: 12}},
		{name: "cancel", stmt: &strategyir.CancelStmt{Range: strategyir.SourceRange{StartLine: 13}}, kind: strategyir.StatementKindCancel, rng: strategyir.SourceRange{StartLine: 13}},
		{name: "protect", stmt: &strategyir.ProtectStmt{Range: strategyir.SourceRange{StartLine: 14}}, kind: strategyir.StatementKindProtect, rng: strategyir.SourceRange{StartLine: 14}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.stmt.Kind(); got != tc.kind {
				t.Fatalf("Kind() = %q, want %q", got, tc.kind)
			}
			if got := tc.stmt.SourceRange(); got != tc.rng {
				t.Fatalf("SourceRange() = %#v, want %#v", got, tc.rng)
			}
		})
	}
}
