package pineruntime

import (
	"testing"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"

	"github.com/jftrade/jftrade-main/pkg/market"
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

	value, err := evaluateExpression("indicators.ready and close > open and high > low and volume > 1000 and kline.close > open and equity == 0 and bar_index == 20", scope)
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

func TestEvaluateExpressionSupportsMathAndTimeVariables(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)

	value, err := evaluateExpression("abs(-2) + min(3, 7) + max(4, 9) + round(2.6) + floor(2.9) + ceil(2.1) + sqrt(9) + pow(2, 3) + log(1) + sign(-5)", scope)
	if err != nil {
		t.Fatalf("math expression error = %v", err)
	}
	if value != 32.0 {
		t.Fatalf("math expression = %#v, want 32", value)
	}

	timeValue := float64(time.Date(2026, time.May, 28, 13, 30, 0, 0, time.UTC).UnixMilli())
	value, err = evaluateExpression("time == 1779975000000 and hour == 9 and minute == 30 and dayofweek == 5 and dayofmonth == 28 and month == 5 and year == 2026", scope)
	if err != nil {
		t.Fatalf("time expression error = %v", err)
	}
	if value != true {
		t.Fatalf("time expression = %#v, want true for time %v", value, timeValue)
	}
}

func TestEvaluateExpressionSupportsHistoryFunction(t *testing.T) {
	runtime := &strategyRuntime{
		expressionCache: map[string]exprast.Node{},
		historyValues: map[string]*historyBuffer{
			"id:close":                        historyBufferForTest(98.0, 99.0, 100.0),
			"id:hlc3":                         historyBufferForTest(97.0, 98.0, 99.0),
			"member:id:bands.string:upper":    historyBufferForTest(101.0, 102.0, 103.0),
			"member:id:macd.string:histogram": historyBufferForTest(1.0, 2.0, 3.0),
			"id:time":                         historyBufferForTest(float64(time.Date(2026, time.May, 28, 9, 28, 0, 0, time.UTC).UnixMilli())),
			"id:bar_index":                    historyBufferForTest(18.0, 19.0),
		},
	}
	scope := newBarExpressionScope(runtime)
	scope.variables["bands"] = map[string]any{"upper": 104.0}
	scope.variables["macd"] = map[string]any{"histogram": 4.0}

	value, err := evaluateExpression("history(close, 1) == 100 and history(close, 3) == 98 and history(hlc3, 2) == 98 and history(bands.upper, 2) == 102 and history(macd.histogram, 1) == 3 and history(time, 1) == 1779960480000 and history(bar_index, 2) == 18", scope)
	if err != nil {
		t.Fatalf("history expression error = %v", err)
	}
	if value != true {
		t.Fatalf("history expression = %#v, want true", value)
	}

	value, err = evaluateExpression("history(close, 4)", scope)
	if err != nil {
		t.Fatalf("missing history expression error = %v", err)
	}
	if value != nil {
		t.Fatalf("missing history expression = %#v, want nil", value)
	}
}

func historyBufferForTest(values ...any) *historyBuffer {
	buffer := newHistoryBuffer(len(values))
	for _, value := range values {
		buffer.push(value)
	}
	return buffer
}

func TestEvaluateExpressionSupportsDerivedSourcesEnvironmentTimestampAndTR(t *testing.T) {
	runtime := &strategyRuntime{symbol: "US.AAPL", interval: types.Interval1m, expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)

	value, err := evaluateExpression("hl2 == 100.5 and hlc3 == 100.83333333333333 and ohlc4 == 100.625", scope)
	if err != nil {
		t.Fatalf("derived source expression error = %v", err)
	}
	if value != true {
		t.Fatalf("derived source expression = %#v, want true", value)
	}

	value, err = evaluateExpression("syminfo_tickerid == 'US.AAPL' and syminfo_prefix == 'US' and timeframe_period == '1m' and timeframe_multiplier == 1 and timeframe_isintraday and timeframe_isminutes and !timeframe_isseconds and !timeframe_isdaily", scope)
	if err != nil {
		t.Fatalf("environment expression error = %v", err)
	}
	if value != true {
		t.Fatalf("environment expression = %#v, want true", value)
	}

	value, err = evaluateExpression("timestamp(2026, 5, 28, 9, 30) == 1779975000000 and tr(true) == 3", scope)
	if err != nil {
		t.Fatalf("timestamp/TR expression error = %v", err)
	}
	if value != true {
		t.Fatalf("timestamp/TR expression = %#v, want true", value)
	}

	value, err = evaluateExpression("time_close == 1779975059000", scope)
	if err != nil {
		t.Fatalf("time_close expression error = %v", err)
	}
	if value != true {
		t.Fatalf("time_close expression = %#v, want true", value)
	}

	runtime.previousBarTime = time.Date(2026, time.May, 28, 13, 29, 0, 0, time.UTC)
	runtime.hasPreviousBarTime = true
	value, err = evaluateExpression("timeframe_change('15')", scope)
	if err != nil {
		t.Fatalf("timeframe_change expression error = %v", err)
	}
	if value != true {
		t.Fatalf("timeframe_change expression = %#v, want true", value)
	}
	runtime.previousBarTime = time.Date(2026, time.May, 28, 13, 30, 0, 0, time.UTC)
	value, err = evaluateExpression("!timeframe_change('15')", scope)
	if err != nil || value != true {
		t.Fatalf("same bucket timeframe_change expression = %#v, err %v, want true", value, err)
	}
	value, err = evaluateExpression("timeframe_in_seconds() == 60 and timeframe_in_seconds('15') == 900 and timeframe_in_seconds('2D') == 172800", scope)
	if err != nil {
		t.Fatalf("timeframe_in_seconds expression error = %v", err)
	}
	if value != true {
		t.Fatalf("timeframe_in_seconds expression = %#v, want true", value)
	}

	value, err = evaluateExpression("str_length('Alpha') == 5 and str_contains('Alpha', 'ph') and str_pos('Alpha', 'ha') == 3 and str_substring('Alpha', 1, 4) == 'lph' and str_replace('A-B', '-', '+') == 'A+B' and str_upper('ab') == 'AB' and str_lower('CD') == 'cd' and str_format('x={0}, y={1}', 3, true) == 'x=3, y=true'", scope)
	if err != nil {
		t.Fatalf("string helper expression error = %v", err)
	}
	if value != true {
		t.Fatalf("string helper expression = %#v, want true", value)
	}

	scope.barIndex = 0
	scope.currentSession = market.SessionRegular
	originalBacktesting := bbgo2.IsBackTesting
	t.Cleanup(func() { bbgo2.IsBackTesting = originalBacktesting })
	bbgo2.IsBackTesting = true
	value, err = evaluateExpression("barstate_isfirst and barstate_isnew and barstate_isconfirmed and barstate_ishistory and !barstate_isrealtime and barstate_islast and session_ismarket and !session_ispremarket and !session_ispostmarket", scope)
	if err != nil {
		t.Fatalf("barstate/session expression error = %v", err)
	}
	if value != true {
		t.Fatalf("barstate/session expression = %#v, want true", value)
	}
	bbgo2.IsBackTesting = false
	value, err = evaluateExpression("!barstate_ishistory and barstate_isrealtime", scope)
	if err != nil || value != true {
		t.Fatalf("realtime barstate expression = %#v, err %v, want true", value, err)
	}
	scope.currentSession = market.SessionPre
	value, err = evaluateExpression("session_ispremarket and !session_ismarket", scope)
	if err != nil || value != true {
		t.Fatalf("premarket session expression = %#v, err %v, want true", value, err)
	}
	scope.currentSession = market.SessionAfter
	value, err = evaluateExpression("session_ispostmarket and !session_ismarket", scope)
	if err != nil || value != true {
		t.Fatalf("postmarket session expression = %#v, err %v, want true", value, err)
	}
}

func TestEvaluateExpressionSupportsNewIndicatorLookups(t *testing.T) {
	runtime := &strategyRuntime{expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	scope.indicators = map[string]any{
		"rsi:hlc3:14":                 map[string]any{"value": 55.0},
		"stdev:volume:20":             map[string]any{"value": 12.0},
		"variance:volume:20":          map[string]any{"value": 144.0},
		"cci:close:20":                map[string]any{"value": 80.0},
		"vwap:hlc3":                   map[string]any{"value": 100.0},
		"mfi:hlc3:14":                 map[string]any{"value": 62.0},
		"dmi:14:14":                   map[string]any{"plus": 25.0, "minus": 12.0, "adx": 28.0},
		"supertrend:3:10":             map[string]any{"line": 98.5, "direction": 1.0},
		"sar:0.02:0.02:0.2":           map[string]any{"value": 97.5, "previous": 96.5},
		"highest:hlc3:3":              map[string]any{"value": 101.0},
		"range:close:5":               map[string]any{"value": 6.0},
		"mode:close:5":                map[string]any{"value": 103.0},
		"sum:volume:3":                map[string]any{"value": 3000.0},
		"ma:SMA:3:15m":                map[string]any{"value": 102.0},
		"security_source:15m:close":   map[string]any{"value": 105.0, "previous": 101.0},
		"security_source:15m:close:1": map[string]any{"value": 101.0, "previous": 99.0},
		"rsi:close:14:15m":            map[string]any{"value": 58.0},
		"macd:close:12:26:9:15m":      map[string]any{"diff": 2.0, "signal": 1.0, "histogram": 2.0},
		"atr:14:15m":                  map[string]any{"value": 3.5},
		"bollinger:close:20:2:15m":    map[string]any{"middle": 100.0, "upper": 106.0, "lower": 94.0},
		"supertrend:3:10:15m":         map[string]any{"line": 99.0, "direction": 1.0},
	}

	value, err := evaluateExpression("rsi(hlc3, 14) > 50 and stdev(volume, 20) == 12 and variance(volume, 20) == 144 and cci(close, 20) > 0 and vwap(hlc3) == 100 and mfi(hlc3, 14) > 60 and dmi(14, 14).plus > dmi(14, 14).minus and dmi(14, 14).adx > 20 and supertrend(3, 10).direction == 1 and sar(0.02, 0.02, 0.2) == 97.5 and previous(sar(0.02, 0.02, 0.2)) == 96.5 and highest(hlc3, 3) == 101 and range(close, 5) == 6 and mode(close, 5) == 103 and sum(volume, 3) == 3000 and security_source(close, '15m') > ma(SMA, 3, '15m') and previous(security_source(close, '15m')) == 101 and security_source(close, '15m', 1) == 101 and rsi(close, 14, '15m') > 50 and macd(12, 26, 9, '15m', close).histogram == 2 and atr(14, '15m') == 3.5 and bollinger(20, 2, '15m', close).upper == 106 and supertrend(3, 10, '15m').direction == 1", scope)
	if err != nil {
		t.Fatalf("indicator lookup expression error = %v", err)
	}
	if value != true {
		t.Fatalf("indicator lookup expression = %#v, want true", value)
	}

	value, err = evaluateExpression("security_source(close, '15m') > ma(SMA, 3, '15m') ? 1 : 0", scope)
	if err != nil || value != 1.0 {
		t.Fatalf("conditional MTF expression = %#v, err %v, want 1", value, err)
	}
}

func TestEvaluateExpressionSupportsBarsSinceAndValueWhenState(t *testing.T) {
	runtime := &strategyRuntime{
		expressionCache: map[string]exprast.Node{},
		barssinceStates: map[string]*barssinceState{},
		valuewhenStates: map[string]*valuewhenState{},
	}
	scope := newBarExpressionScope(runtime)

	scope.barIndex = 0
	scope.closeSeries = seriesNumber{Current: 99, Previous: 100, HasCurrent: true, HasPrevious: true}
	scope.openSeries = seriesNumber{Current: 100, Previous: 99, HasCurrent: true, HasPrevious: true}
	value, err := evaluateExpression("barssince(close > open)", scope)
	if err != nil {
		t.Fatalf("barssince first expression error = %v", err)
	}
	if value != nil {
		t.Fatalf("barssince first = %#v, want nil", value)
	}

	scope.barIndex = 1
	scope.closeSeries = seriesNumber{Current: 103, Previous: 99, HasCurrent: true, HasPrevious: true}
	scope.openSeries = seriesNumber{Current: 100, Previous: 100, HasCurrent: true, HasPrevious: true}
	value, err = evaluateExpression("barssince(close > open)", scope)
	if err != nil || value != 0.0 {
		t.Fatalf("barssince trigger = %#v, err %v, want 0", value, err)
	}
	value, err = evaluateExpression("barssince(close > open)", scope)
	if err != nil || value != 0.0 {
		t.Fatalf("barssince same bar = %#v, err %v, want 0", value, err)
	}

	scope.barIndex = 2
	scope.closeSeries = seriesNumber{Current: 98, Previous: 103, HasCurrent: true, HasPrevious: true}
	scope.openSeries = seriesNumber{Current: 100, Previous: 100, HasCurrent: true, HasPrevious: true}
	value, err = evaluateExpression("barssince(close > open)", scope)
	if err != nil || value != 1.0 {
		t.Fatalf("barssince next = %#v, err %v, want 1", value, err)
	}

	value, err = evaluateExpression("valuewhen(close > open, close, 0)", scope)
	if err != nil || value != nil {
		t.Fatalf("valuewhen before trigger = %#v, err %v, want nil", value, err)
	}
	scope.barIndex = 3
	scope.closeSeries = seriesNumber{Current: 105, Previous: 98, HasCurrent: true, HasPrevious: true}
	scope.openSeries = seriesNumber{Current: 100, Previous: 100, HasCurrent: true, HasPrevious: true}
	value, err = evaluateExpression("valuewhen(close > open, close, 0)", scope)
	series, ok := value.(seriesNumber)
	if err != nil || !ok || series.Current != 105 {
		t.Fatalf("valuewhen trigger = %#v, err %v, want current close series", value, err)
	}
	scope.barIndex = 4
	scope.closeSeries = seriesNumber{Current: 110, Previous: 105, HasCurrent: true, HasPrevious: true}
	scope.openSeries = seriesNumber{Current: 100, Previous: 100, HasCurrent: true, HasPrevious: true}
	value, err = evaluateExpression("valuewhen(close > open, close, 1)", scope)
	if err != nil {
		t.Fatalf("valuewhen occurrence expression error = %v", err)
	}
	series, ok = value.(seriesNumber)
	if !ok || series.Current != 105 {
		t.Fatalf("valuewhen occurrence = %#v, want previous trigger close series", value)
	}
}

func TestEvaluateExpressionSupportsPositionVariables(t *testing.T) {
	runtime := &strategyRuntime{symbol: "US.AAPL", expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	runtime.storeCachedPosition("US.AAPL", scope.currentKlineTime, &positionSnapshot{
		Symbol:       "US.AAPL",
		Quantity:     3,
		AveragePrice: 101.25,
		Direction:    "LONG",
	})

	value, err := evaluateExpression("position_size == 3 and position_avg_price == 101.25", scope)
	if err != nil {
		t.Fatalf("position variable expression error = %v", err)
	}
	if value != true {
		t.Fatalf("position variable expression = %#v, want true", value)
	}

	runtime.storeCachedPosition("US.AAPL", scope.currentKlineTime, nil)
	value, err = evaluateExpression("position_size == 0 and position_avg_price == na", scope)
	if err != nil {
		t.Fatalf("flat position expression error = %v", err)
	}
	if value != true {
		t.Fatalf("flat position expression = %#v, want true", value)
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

func TestEvaluateExpressionComparesNAWithoutBoolCoercion(t *testing.T) {
	scope := &evaluationScope{variables: map[string]any{}}
	cases := []struct {
		expression string
		want       bool
	}{
		{expression: "0 == na", want: false},
		{expression: "0 != na", want: true},
		{expression: "na == na", want: true},
		{expression: "na != na", want: false},
	}
	for _, tc := range cases {
		value, err := evaluateExpression(tc.expression, scope)
		if err != nil {
			t.Fatalf("evaluateExpression(%q) error = %v", tc.expression, err)
		}
		if value != tc.want {
			t.Fatalf("evaluateExpression(%q) = %#v, want %v", tc.expression, value, tc.want)
		}
	}
}

func TestExecuteLetSupportsPersistentVarReassignAndPrevious(t *testing.T) {
	runtime := &strategyRuntime{
		expressionCache:  map[string]exprast.Node{},
		bindingCache:     map[*strategyir.LetStmt]cachedIndicatorBinding{},
		persistentValues: map[string]any{},
	}
	statements := []strategyir.Statement{
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "count", Expression: "0", Mode: strategyir.AssignmentModeVar},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "count", Expression: "count + 1", Mode: strategyir.AssignmentModeReassign},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 3}, Name: "prev", Expression: "nz(previous(count), -1)"},
	}

	firstScope := &evaluationScope{runtime: runtime, variables: map[string]any{}}
	if _, err := runtime.executeStatements(statements, firstScope); err != nil {
		t.Fatalf("first executeStatements() error = %v", err)
	}
	if got, _ := coerceFloatValue(firstScope.variables["prev"]); got != 0 {
		t.Fatalf("first prev = %v, want 0", firstScope.variables["prev"])
	}

	secondScope := &evaluationScope{runtime: runtime, variables: map[string]any{}}
	if _, err := runtime.executeStatements(statements, secondScope); err != nil {
		t.Fatalf("second executeStatements() error = %v", err)
	}
	if got, _ := coerceFloatValue(secondScope.variables["prev"]); got != 1 {
		t.Fatalf("second prev = %v, want 1", secondScope.variables["prev"])
	}
	if got, _ := coerceFloatValue(secondScope.variables["count"]); got != 2 {
		t.Fatalf("second count = %v, want 2", secondScope.variables["count"])
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
		StartTime:   types.Time(time.Date(2026, time.May, 28, 13, 30, 0, 0, time.UTC)),
		EndTime:     types.Time(time.Date(2026, time.May, 28, 13, 30, 59, 0, time.UTC)),
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
		currentSession:     market.SessionRegular,
		klinePayload:       klinePayloadView{kline: &bar, session: market.SessionRegular},
		barIndex:           20,
		closeSeries:        seriesNumber{Current: bar.Close.Float64(), Previous: 100.0, HasCurrent: true, HasPrevious: true},
		openSeries:         seriesNumber{Current: bar.Open.Float64(), Previous: 99.0, HasCurrent: true, HasPrevious: true},
		highSeries:         seriesNumber{Current: bar.High.Float64(), Previous: 101.0, HasCurrent: true, HasPrevious: true},
		lowSeries:          seriesNumber{Current: bar.Low.Float64(), Previous: 98.0, HasCurrent: true, HasPrevious: true},
		volumeSeries:       seriesNumber{Current: bar.Volume.Float64(), Previous: 900.0, HasCurrent: true, HasPrevious: true},
		hl2Series:          seriesNumber{Current: 100.5, Previous: 99.5, HasCurrent: true, HasPrevious: true},
		hlc3Series:         seriesNumber{Current: (102.0 + 99.0 + 101.5) / 3, Previous: (101.0 + 98.0 + 100.0) / 3, HasCurrent: true, HasPrevious: true},
		ohlc4Series:        seriesNumber{Current: (100.0 + 102.0 + 99.0 + 101.5) / 4, Previous: (99.0 + 101.0 + 98.0 + 100.0) / 4, HasCurrent: true, HasPrevious: true},
		hasBarData:         true,
	}
}

func TestPineCalendarFieldsUseExchangeTimezone(t *testing.T) {
	runtime := &strategyRuntime{symbol: "US.AAPL", expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)
	scope.currentKline.StartTime = types.Time(time.Date(2026, time.January, 1, 0, 30, 0, 0, time.UTC))
	scope.currentKline.EndTime = types.Time(time.Date(2026, time.January, 1, 0, 30, 59, 0, time.UTC))

	value, err := evaluateExpression("hour == 19 and minute == 30 and dayofweek == 4 and dayofmonth == 31 and month == 12 and year == 2025", scope)
	if err != nil {
		t.Fatalf("evaluateExpression: %v", err)
	}
	if value != true {
		t.Fatalf("exchange calendar expression = %#v, want true", value)
	}
}

func TestPineTimestampAndTimeframeChangeUseExchangeTimezone(t *testing.T) {
	runtime := &strategyRuntime{symbol: "US.AAPL", expressionCache: map[string]exprast.Node{}}
	scope := newBarExpressionScope(runtime)

	value, err := evaluateExpression("timestamp(2026, 3, 8) == 1772946000000", scope)
	if err != nil || value != true {
		t.Fatalf("DST timestamp expression = %#v, err=%v", value, err)
	}

	location := pineExchangeLocation(scope)
	if pineTimeframeBucketChanged(
		time.Date(2025, time.December, 31, 23, 30, 0, 0, time.UTC),
		time.Date(2026, time.January, 1, 0, 30, 0, 0, time.UTC),
		"1D",
		location,
	) {
		t.Fatal("UTC midnight must not change the New York trading calendar day")
	}
	if !pineTimeframeBucketChanged(
		time.Date(2026, time.January, 1, 4, 30, 0, 0, time.UTC),
		time.Date(2026, time.January, 1, 5, 30, 0, 0, time.UTC),
		"1D",
		location,
	) {
		t.Fatal("New York midnight must change the daily timeframe bucket")
	}
}
