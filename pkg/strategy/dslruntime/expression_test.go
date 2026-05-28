package dslruntime

import (
	"testing"

	exprast "github.com/expr-lang/expr/ast"
)

func TestEvaluateExpressionUsesExprParserAndPreservesSeriesSemantics(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := &evaluationScope{
		runtime: runtime,
		variables: map[string]any{
			"close": seriesNumber{Current: 101, Previous: 99, HasCurrent: true, HasPrevious: true},
			"fast":  seriesNumber{Current: 11, Previous: 9, HasCurrent: true, HasPrevious: true},
			"slow":  seriesNumber{Current: 10, Previous: 10, HasCurrent: true, HasPrevious: true},
			"bands": map[string]any{
				"upper":         100.0,
				"previousUpper": 102.0,
			},
		},
		bindings:   map[string]indicatorBinding{},
		indicators: map[string]any{},
	}

	value, err := evaluateExpression("cross_over(fast, slow) and close > bands.upper and abs(-2) == 2", scope)
	if err != nil {
		t.Fatalf("evaluateExpression() error = %v", err)
	}
	if value != true {
		t.Fatalf("evaluateExpression() = %#v, want true", value)
	}
	if len(runtime.expressionCache) != 1 {
		t.Fatalf("expression cache size = %d, want 1", len(runtime.expressionCache))
	}

	value, err = evaluateExpression("cross_over(fast, slow) and close > bands.upper and abs(-2) == 2", scope)
	if err != nil {
		t.Fatalf("evaluateExpression() second run error = %v", err)
	}
	if value != true {
		t.Fatalf("evaluateExpression() second run = %#v, want true", value)
	}
	if len(runtime.expressionCache) != 1 {
		t.Fatalf("expression cache size after second run = %d, want 1", len(runtime.expressionCache))
	}
}
