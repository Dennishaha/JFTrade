package pineruntime

import (
	"testing"
	"time"

	exprast "github.com/expr-lang/expr/ast"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func TestEvaluateTimeframeChangeExpressionBusinessSemantics(t *testing.T) {
	scope := &evaluationScope{
		runtime: &strategyRuntime{
			symbol: "US.AAPL",
		},
		currentKline: &types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1h,
			StartTime: types.Time(time.Date(2026, time.January, 2, 14, 30, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 2, 15, 29, 59, 0, time.UTC)),
			Close:     fixedpoint.NewFromFloat(101),
		},
		currentKlineSymbol: "US.AAPL",
	}

	value, err := evaluateTimeframeChangeExpression([]exprast.Node{&exprast.StringNode{Value: "1D"}}, scope)
	if err != nil || value != true {
		t.Fatalf("first timeframe.change() = %#v, %v", value, err)
	}

	scope.runtime.previousBarTime = time.Date(2026, time.January, 2, 13, 30, 0, 0, time.UTC)
	scope.runtime.hasPreviousBarTime = true
	value, err = evaluateTimeframeChangeExpression([]exprast.Node{&exprast.StringNode{Value: "1D"}}, scope)
	if err != nil || value != false {
		t.Fatalf("same-day timeframe.change() = %#v, %v", value, err)
	}

	scope.runtime.previousBarTime = time.Date(2026, time.January, 1, 15, 30, 0, 0, time.UTC)
	value, err = evaluateTimeframeChangeExpression([]exprast.Node{&exprast.StringNode{Value: "1D"}}, scope)
	if err != nil || value != true {
		t.Fatalf("new-day timeframe.change() = %#v, %v", value, err)
	}

	if got, err := evaluateTimeframeChangeExpression([]exprast.Node{&exprast.StringNode{Value: ""}}, scope); err == nil || got != nil {
		t.Fatalf("blank timeframe.change() = %#v, %v", got, err)
	}
	if got, err := evaluateTimeframeChangeExpression([]exprast.Node{}, scope); err == nil || got != nil {
		t.Fatalf("missing timeframe.change() = %#v, %v", got, err)
	}
	if got, err := evaluateTimeframeChangeExpression([]exprast.Node{&exprast.StringNode{Value: "1D"}}, nil); err != nil || got != false {
		t.Fatalf("nil-scope timeframe.change() = %#v, %v", got, err)
	}
}

func TestEvaluateTimeframeInSecondsResolvesRuntimeAndStaticFrames(t *testing.T) {
	scope := &evaluationScope{
		runtime: &strategyRuntime{interval: types.Interval5m},
		currentKline: &types.KLine{
			Interval: types.Interval1h,
		},
	}

	value, err := evaluateTimeframeInSecondsExpression(nil, scope)
	if err != nil || value != float64(300) {
		t.Fatalf("timeframe.in_seconds(runtime) = %#v, %v", value, err)
	}

	value, err = evaluateTimeframeInSecondsExpression([]exprast.Node{&exprast.StringNode{Value: "90"}}, scope)
	if err != nil || value != float64(5400) {
		t.Fatalf("timeframe.in_seconds(90) = %#v, %v", value, err)
	}
	value, err = evaluateTimeframeInSecondsExpression([]exprast.Node{&exprast.StringNode{Value: "1W"}}, scope)
	if err != nil || value != float64(7*24*60*60) {
		t.Fatalf("timeframe.in_seconds(1W) = %#v, %v", value, err)
	}
	if got, err := evaluateTimeframeInSecondsExpression([]exprast.Node{&exprast.StringNode{Value: "bad"}}, scope); err == nil || got != nil {
		t.Fatalf("timeframe.in_seconds(bad) = %#v, %v", got, err)
	}
	if got, err := evaluateTimeframeInSecondsExpression(nil, &evaluationScope{}); err == nil || got != nil {
		t.Fatalf("timeframe.in_seconds(no runtime) = %#v, %v", got, err)
	}
}

func TestPineStaticTimeframeParsersAndHelpers(t *testing.T) {
	if duration, ok := pineStaticTimeframeDuration("90"); !ok || duration != 90*time.Minute {
		t.Fatalf("pineStaticTimeframeDuration(90) = %v, %v", duration, ok)
	}
	if duration, ok := pineStaticTimeframeDuration("2H"); ok || duration != 0 {
		t.Fatalf("pineStaticTimeframeDuration(2H) = %v, %v, want unsupported", duration, ok)
	}
	if unit, duration, ok := pineStaticTimeframeBucket("90"); !ok || unit != "intraday" || duration != 90*time.Minute {
		t.Fatalf("pineStaticTimeframeBucket(90) = %q %v %v", unit, duration, ok)
	}
	if unit, duration, ok := pineStaticTimeframeBucket("1M"); !ok || unit != "month" || duration != 0 {
		t.Fatalf("pineStaticTimeframeBucket(1M) = %q %v %v", unit, duration, ok)
	}
	if unit, duration, ok := pineStaticTimeframeBucket("bad"); ok || unit != "" || duration != 0 {
		t.Fatalf("pineStaticTimeframeBucket(bad) = %q %v %v", unit, duration, ok)
	}

	if got := (&evaluationScope{currentKline: &types.KLine{Interval: types.Interval15m}}).runtimeInterval(); got != types.Interval15m {
		t.Fatalf("runtimeInterval(currentKline) = %q", got)
	}
}
