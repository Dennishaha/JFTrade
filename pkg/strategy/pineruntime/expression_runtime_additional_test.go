package pineruntime

import (
	"strings"
	"testing"

	exprast "github.com/expr-lang/expr/ast"
)

func TestEvaluateExpressionSupportsIndicatorLookupsAndConditionalHelpers(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	scope.indicators = map[string]any{
		"anchored_vwap:day:hlc3": 101.25,
		"bbw:hlc3:20:2:week":     1.5,
		"cog:hlc3:10:day":        99.5,
		"cum:volume":             4321.0,
		"stoch:hlc3:14:day":      77.0,
		"rising:close:3":         true,
	}

	if got, err := evaluateFloatExpression("anchored_vwap(hlc3, day)", scope); err != nil || got != 101.25 {
		t.Fatalf("anchored_vwap expression = %v, %v", got, err)
	}
	if got, err := evaluateFloatExpression("bbw(hlc3, 20, 2, week)", scope); err != nil || got != 1.5 {
		t.Fatalf("bbw expression = %v, %v", got, err)
	}
	if got, err := evaluateFloatExpression("cog(hlc3, 10, day)", scope); err != nil || got != 99.5 {
		t.Fatalf("cog expression = %v, %v", got, err)
	}
	if got, err := evaluateFloatExpression("cum(volume)", scope); err != nil || got != 4321 {
		t.Fatalf("cum expression = %v, %v", got, err)
	}
	if got, err := evaluateFloatExpression("stoch(hlc3, high, low, 14, day)", scope); err != nil || got != 77 {
		t.Fatalf("stoch expression = %v, %v", got, err)
	}
	if got, err := evaluateBoolExpression("rising(close, 3)", scope); err != nil || !got {
		t.Fatalf("rising expression = %v, %v", got, err)
	}
	if got, err := evaluateBoolExpression("ifelse(enabled, rising(close, 3), false)", scope); err != nil || !got {
		t.Fatalf("ifelse(bool) expression = %v, %v", got, err)
	}
	if got, err := evaluateFloatExpression("nz(na, bbw(hlc3, 20, 2, week))", scope); err != nil || got != 1.5 {
		t.Fatalf("nz expression = %v, %v", got, err)
	}
	value, err := evaluateExpression("tostring(ifelse(enabled, anchored_vwap(hlc3, day), bbw(hlc3, 20, 2, week)), '#.##')", scope)
	if err != nil {
		t.Fatalf("tostring expression error = %v", err)
	}
	if value != "101.25" {
		t.Fatalf("tostring expression = %#v, want %q", value, "101.25")
	}
}

func TestEvaluateDivergenceExpressionUsesParentBindingsAndRuntimeCache(t *testing.T) {
	runtime := &strategyRuntime{
		expressionCache: map[string]exprast.Node{},
		divergenceCache: map[divergenceRequirementCacheKey]string{},
	}
	parent := newBarExpressionScope(runtime)
	parent.bindings = map[string]indicatorBinding{
		"flow": {Alias: "flow", Kind: "macd", Key: "macd:12:26:9", Args: []string{"12", "26", "9"}},
	}
	scope := &evaluationScope{
		runtime:    runtime,
		parent:     parent,
		variables:  map[string]any{},
		indicators: map[string]any{"divergence:macd:12:26:9:top:6": true},
	}

	got, err := evaluateBoolExpression("divergence_top(flow, 6)", scope)
	if err != nil {
		t.Fatalf("divergence_top() error = %v", err)
	}
	if !got {
		t.Fatal("divergence_top() = false, want true")
	}
	if len(runtime.divergenceCache) != 1 {
		t.Fatalf("divergenceCache len = %d, want 1", len(runtime.divergenceCache))
	}

	got, err = evaluateBoolExpression("divergence_top(flow, 6)", scope)
	if err != nil || !got {
		t.Fatalf("second divergence_top() = %v, %v", got, err)
	}
	if len(runtime.divergenceCache) != 1 {
		t.Fatalf("divergenceCache len after cache hit = %d, want 1", len(runtime.divergenceCache))
	}

	got, err = evaluateBoolExpression("divergence_bottom(flow, 6)", scope)
	if err != nil {
		t.Fatalf("divergence_bottom() error = %v", err)
	}
	if got {
		t.Fatal("divergence_bottom() = true, want false when indicator snapshot is absent")
	}
}

func TestEvaluateExpressionValidatesConditionalAndIndicatorArguments(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	scope.indicators = map[string]any{
		"rising:close:3": 1.0,
	}

	cases := []struct {
		expression string
		want       string
	}{
		{expression: "ifelse(1, close, open)", want: "condition must be boolean"},
		{expression: "stoch(close, close, low, 14)", want: "literal high and low"},
		{expression: "anchored_vwap(hlc3, hour)", want: "day/week/month"},
		{expression: "divergence_top(flow, 0)", want: "positive integer"},
		{expression: "rising(close, 3)", want: "indicator value is not boolean"},
	}

	scope.bindings = map[string]indicatorBinding{
		"flow": {Alias: "flow", Kind: "macd", Key: "macd:12:26:9", Args: []string{"12", "26", "9"}},
	}

	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			_, err := evaluateExpression(tc.expression, scope)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("evaluateExpression(%q) error = %v, want substring %q", tc.expression, err, tc.want)
			}
		})
	}
}
