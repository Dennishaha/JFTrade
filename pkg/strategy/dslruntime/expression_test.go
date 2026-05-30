package dslruntime

import (
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"
	"github.com/jftrade/jftrade-main/pkg/futu"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type testObjectFieldReader struct {
	fields map[string]any
}

type testSeriesFieldReader struct {
	current     float64
	previous    float64
	hasCurrent  bool
	hasPrevious bool
}

var benchmarkExpressionBoolSink bool

func (r testObjectFieldReader) FieldValue(name string) (any, bool) {
	value, ok := r.fields[name]
	return value, ok
}

func (r testSeriesFieldReader) FieldValue(name string) (any, bool) {
	switch name {
	case "value":
		if r.hasCurrent {
			return r.current, true
		}
		return nil, true
	case "previous":
		if r.hasPrevious {
			return r.previous, true
		}
		return nil, true
	default:
		return nil, false
	}
}

func (r testSeriesFieldReader) SeriesField(name string) (float64, float64, bool, bool, bool) {
	if name != "value" {
		return 0, 0, false, false, false
	}
	return r.current, r.previous, r.hasCurrent, r.hasPrevious, true
}

func (r testSeriesFieldReader) PreferredScalarValue() (float64, bool) {
	if !r.hasCurrent {
		return 0, false
	}
	return r.current, true
}

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

func TestEvaluateExpressionSupportsObjectFieldReaders(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := &evaluationScope{
		runtime: runtime,
		variables: map[string]any{
			"bands": testObjectFieldReader{fields: map[string]any{
				"upper":         101.0,
				"previousUpper": 99.0,
			}},
		},
		bindings:   map[string]indicatorBinding{},
		indicators: map[string]any{},
	}

	value, err := evaluateExpression("bands.upper > 100", scope)
	if err != nil {
		t.Fatalf("evaluateExpression() error = %v", err)
	}
	if value != true {
		t.Fatalf("evaluateExpression() = %#v, want true", value)
	}
}

func TestEvaluateExpressionShortCircuitsLogicalBinary(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := &evaluationScope{
		runtime:    runtime,
		variables:  map[string]any{},
		bindings:   map[string]indicatorBinding{},
		indicators: map[string]any{},
	}

	value, err := evaluateExpression("false and missing.value > 0", scope)
	if err != nil {
		t.Fatalf("false-and short circuit error = %v", err)
	}
	if value != false {
		t.Fatalf("false-and short circuit = %#v, want false", value)
	}

	value, err = evaluateExpression("true or missing.value > 0", scope)
	if err != nil {
		t.Fatalf("true-or short circuit error = %v", err)
	}
	if value != true {
		t.Fatalf("true-or short circuit = %#v, want true", value)
	}
}

func TestEvaluateExpressionSupportsReservedBarVariablesAndShadowing(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	scope.indicators = map[string]any{"ready": true}

	value, err := evaluateExpression("indicators.ready and close > open and high > low and volume > 1000 and kline.close > open", scope)
	if err != nil {
		t.Fatalf("reserved variable expression error = %v", err)
	}
	if value != true {
		t.Fatalf("reserved variable expression = %#v, want true", value)
	}

	child := scope.clone()
	child.setVariable("close", 10.0)
	value, err = evaluateExpression("close == 10", child)
	if err != nil {
		t.Fatalf("shadowed close expression error = %v", err)
	}
	if value != true {
		t.Fatalf("shadowed close expression = %#v, want true", value)
	}
}

func TestExecuteStatementsKeepsBranchLetScoped(t *testing.T) {
	ifStmt := &strategyir.IfStmt{
		Range:     strategyir.SourceRange{StartLine: 1, EndLine: 2},
		Condition: "true",
		Then: []strategyir.Statement{
			&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2, EndLine: 2}, Name: "temp", Expression: "42"},
		},
	}
	statements := []strategyir.Statement{ifStmt}
	runtime := &strategyRuntime{
		expressionCache: map[string]exprast.Node{},
		bindingCache:    map[*strategyir.LetStmt]cachedIndicatorBinding{},
		ifScopePlans:    buildIfScopePlans(&strategyir.Program{Hooks: []strategyir.HookBlock{{Kind: strategyir.HookKLineClose, Statements: statements}}}),
	}
	scope := newBarExpressionScope(runtime)
	_, err := runtime.executeStatements(statements, scope)
	if err != nil {
		t.Fatalf("executeStatements() error = %v", err)
	}
	if _, ok := scope.variable("temp"); ok {
		t.Fatal("expected branch let variable to stay scoped inside branch")
	}
}

func TestEvaluateExpressionCoercesObjectFieldReaderForNumericComparison(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := &evaluationScope{
		runtime: runtime,
		variables: map[string]any{
			"ma": testObjectFieldReader{fields: map[string]any{
				"value":    11.0,
				"previous": 10.0,
			}},
		},
		bindings:   map[string]indicatorBinding{},
		indicators: map[string]any{},
	}

	value, err := evaluateExpression("ma > 10 and ma == 11", scope)
	if err != nil {
		t.Fatalf("object field reader numeric comparison error = %v", err)
	}
	if value != true {
		t.Fatalf("object field reader numeric comparison = %#v, want true", value)
	}
}

func TestEvaluateExpressionSupportsSeriesFieldReaders(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := &evaluationScope{
		runtime: runtime,
		variables: map[string]any{
			"ma":   testSeriesFieldReader{current: 11, previous: 9, hasCurrent: true, hasPrevious: true},
			"slow": seriesNumber{Current: 10, Previous: 10, HasCurrent: true, HasPrevious: true},
		},
		bindings:   map[string]indicatorBinding{},
		indicators: map[string]any{},
	}

	value, err := evaluateExpression("cross_over(ma, slow) and cross_over(ma.value, slow) and ma > 10 and ma.value > 10", scope)
	if err != nil {
		t.Fatalf("series field reader expression error = %v", err)
	}
	if value != true {
		t.Fatalf("series field reader expression = %#v, want true", value)
	}
}

func BenchmarkEvaluateExpressionBinaryHeavy(b *testing.B) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	expression := "enabled and not halted and fast > slow and close > bands.lower and close / open > 1"
	if _, err := evaluateBoolExpression(expression, scope); err != nil {
		b.Fatalf("evaluateBoolExpression() warmup error = %v", err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result, err := evaluateBoolExpression(expression, scope)
		if err != nil {
			b.Fatalf("evaluateBoolExpression() error = %v", err)
		}
		benchmarkExpressionBoolSink = result
	}
}

func BenchmarkEvaluateExpressionDivergenceHeavy(b *testing.B) {
	runtime := &strategyRuntime{
		expressionCache: map[string]exprast.Node{},
		divergenceCache: map[divergenceRequirementCacheKey]string{},
	}
	scope := newBarExpressionScope(runtime)
	scope.bindings["flow"] = indicatorBinding{
		Alias: "flow",
		Kind:  "macd",
		Key:   "macd:12:26:9",
		Args:  []string{"12", "26", "9"},
	}
	scope.indicators["divergence:macd:12:26:9:top:6"] = true
	expression := "divergence_top(flow, 6) and divergence_top(flow, 6)"
	if _, err := evaluateBoolExpression(expression, scope); err != nil {
		b.Fatalf("evaluateBoolExpression() warmup error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result, err := evaluateBoolExpression(expression, scope)
		if err != nil {
			b.Fatalf("evaluateBoolExpression() error = %v", err)
		}
		benchmarkExpressionBoolSink = result
	}
}

func BenchmarkEvaluateExpressionSeriesReaderHeavy(b *testing.B) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	scope.variables["ma"] = testSeriesFieldReader{current: 11, previous: 9, hasCurrent: true, hasPrevious: true}
	expression := "cross_over(ma, slow) and cross_over(ma.value, slow) and ma > 10 and ma.value > 10"
	if _, err := evaluateBoolExpression(expression, scope); err != nil {
		b.Fatalf("evaluateBoolExpression() warmup error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result, err := evaluateBoolExpression(expression, scope)
		if err != nil {
			b.Fatalf("evaluateBoolExpression() error = %v", err)
		}
		benchmarkExpressionBoolSink = result
	}
}

func newBarExpressionScope(runtime *strategyRuntime) *evaluationScope {
	bar := types.KLine{
		Symbol:      "US.AAPL",
		Interval:    types.Interval1m,
		StartTime:   types.Time(time.Date(2026, time.May, 28, 9, 30, 0, 0, time.UTC)),
		EndTime:     types.Time(time.Date(2026, time.May, 28, 9, 30, 59, 0, time.UTC)),
		Open:        fixedpoint.NewFromFloat(100.0),
		High:        fixedpoint.NewFromFloat(102.0),
		Low:         fixedpoint.NewFromFloat(99.0),
		Close:       fixedpoint.NewFromFloat(101.5),
		Volume:      fixedpoint.NewFromFloat(1234.0),
		QuoteVolume: fixedpoint.NewFromFloat(125241.0),
	}
	return &evaluationScope{
		runtime: runtime,
		variables: map[string]any{
			"fast":    seriesNumber{Current: 11, Previous: 10, HasCurrent: true, HasPrevious: true},
			"slow":    seriesNumber{Current: 9, Previous: 9, HasCurrent: true, HasPrevious: true},
			"enabled": true,
			"halted":  false,
			"bands": testObjectFieldReader{fields: map[string]any{
				"upper": 103.0,
				"lower": 99.0,
			}},
		},
		bindings:           map[string]indicatorBinding{},
		indicators:         map[string]any{},
		currentKline:       &bar,
		currentKlineTime:   bar.EndTime.Time(),
		currentKlineSymbol: bar.Symbol,
		currentSession:     futu.MarketSessionRegular,
		klinePayload:       klinePayloadView{kline: &bar, session: futu.MarketSessionRegular},
		closeSeries:        seriesNumber{Current: bar.Close.Float64(), Previous: 100.0, HasCurrent: true, HasPrevious: true},
		openValue:          bar.Open.Float64(),
		highValue:          bar.High.Float64(),
		lowValue:           bar.Low.Float64(),
		volumeValue:        bar.Volume.Float64(),
		hasBarData:         true,
	}
}
