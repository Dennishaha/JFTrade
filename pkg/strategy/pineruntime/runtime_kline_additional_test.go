package pineruntime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestHandleKLineClosedTriggersPendingOrders(t *testing.T) {
	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
	})
	executor := &pineTestExecutor{}
	runtime := &strategyRuntime{
		ctx:              context.Background(),
		session:          session,
		executor:         executor,
		symbol:           "US.AAPL",
		interval:         types.Interval1m,
		strategy:         &Strategy{Symbol: "US.AAPL", Interval: types.Interval1m},
		pendingOrders:    map[string]pendingOrder{},
		entrySubmitCount: map[string]int{},
		baseScope:        &evaluationScope{variables: map[string]any{}},
		reusableScope:    &evaluationScope{variables: map[string]any{}},
	}
	runtime.pendingOrders["stop-buy"] = pendingOrder{
		id:                 "stop-buy",
		sequence:           1,
		action:             strategyir.OrderActionBuy,
		intent:             strategyir.OrderIntentEntry,
		quantityMode:       "shares",
		quantityExpression: "2",
		rangeInfo:          strategyir.SourceRange{StartLine: 61},
		stopPrice:          101,
		hasStop:            true,
	}

	kline := types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval1m,
		StartTime: types.Time(time.Date(2026, time.June, 23, 13, 30, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.June, 23, 13, 30, 59, int(999*time.Millisecond), time.UTC)),
		Open:      fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(102),
		Low:       fixedpoint.NewFromFloat(99),
		Close:     fixedpoint.NewFromFloat(101),
		Volume:    fixedpoint.NewFromFloat(1000),
	}

	runtime.handleKLineClosed(kline)

	if len(executor.orders) != 1 {
		t.Fatalf("submitted orders = %#v", executor.orders)
	}
	order := executor.orders[0]
	if order.Side != types.SideTypeBuy || order.Type != types.OrderTypeMarket || order.Quantity.Float64() != 2 {
		t.Fatalf("submitted order = %#v", order)
	}
	if _, exists := runtime.pendingOrders["stop-buy"]; exists {
		t.Fatalf("pending order should be removed after submit: %#v", runtime.pendingOrders)
	}
	if runtime.barIndex != 1 || runtime.previousClose != 101 {
		t.Fatalf("runtime state after handleKLineClosed = barIndex:%d previousClose:%v", runtime.barIndex, runtime.previousClose)
	}
}

func TestHandleKLineClosedReportsPendingOrderErrorsViaOnError(t *testing.T) {
	var errorsSeen []string
	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
	})
	runtime := &strategyRuntime{
		ctx:              context.Background(),
		session:          session,
		executor:         &pineTestExecutor{},
		symbol:           "US.AAPL",
		interval:         types.Interval1m,
		strategy:         &Strategy{Symbol: "US.AAPL", Interval: types.Interval1m, OnError: func(message string) { errorsSeen = append(errorsSeen, message) }},
		pendingOrders:    map[string]pendingOrder{},
		entrySubmitCount: map[string]int{},
		baseScope:        &evaluationScope{variables: map[string]any{}},
		reusableScope:    &evaluationScope{variables: map[string]any{}},
	}
	runtime.pendingOrders["bad-stop"] = pendingOrder{
		id:                 "bad-stop",
		sequence:           1,
		action:             strategyir.OrderActionBuy,
		intent:             strategyir.OrderIntentEntry,
		quantityMode:       "weird",
		quantityExpression: "2",
		rangeInfo:          strategyir.SourceRange{StartLine: 71},
		stopPrice:          101,
		hasStop:            true,
	}

	runtime.handleKLineClosed(types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval1m,
		StartTime: types.Time(time.Date(2026, time.June, 23, 13, 31, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.June, 23, 13, 31, 59, int(999*time.Millisecond), time.UTC)),
		Open:      fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(102),
		Low:       fixedpoint.NewFromFloat(99),
		Close:     fixedpoint.NewFromFloat(101),
		Volume:    fixedpoint.NewFromFloat(1000),
	})

	if len(errorsSeen) != 1 || !strings.Contains(errorsSeen[0], `unsupported order quantity mode "weird"`) {
		t.Fatalf("OnError pending-order errors = %#v", errorsSeen)
	}
	if runtime.barIndex != 1 || runtime.previousClose != 101 {
		t.Fatalf("runtime state after pending-order error = barIndex:%d previousClose:%v", runtime.barIndex, runtime.previousClose)
	}
}

func TestHandleKLineClosedReportsHookErrorsViaOnError(t *testing.T) {
	var errorsSeen []string
	badLet := &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 81},
		Name:       "broken",
		Expression: "close >",
	}
	runtime := &strategyRuntime{
		ctx:              context.Background(),
		symbol:           "US.AAPL",
		interval:         types.Interval1m,
		strategy:         &Strategy{Symbol: "US.AAPL", Interval: types.Interval1m, OnError: func(message string) { errorsSeen = append(errorsSeen, message) }},
		hooks:            map[strategyir.HookKind]*strategyir.HookBlock{strategyir.HookKLineClose: {Kind: strategyir.HookKLineClose, Statements: []strategyir.Statement{badLet}}},
		ifScopePlans:     map[*strategyir.IfStmt]ifScopePlan{},
		expressionCache:  map[string]exprast.Node{},
		bindingCache:     map[*strategyir.LetStmt]cachedIndicatorBinding{},
		persistentValues: map[string]any{},
		baseScope:        &evaluationScope{variables: map[string]any{}},
		reusableScope: &evaluationScope{
			variables: map[string]any{},
		},
		barIndex: -1,
	}
	runtime.baseScope.runtime = runtime
	runtime.reusableScope.runtime = runtime
	runtime.reusableScope.parent = runtime.baseScope

	runtime.handleKLineClosed(types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval1m,
		StartTime: types.Time(time.Date(2026, time.June, 23, 13, 32, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.June, 23, 13, 32, 59, int(999*time.Millisecond), time.UTC)),
		Open:      fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(102),
		Low:       fixedpoint.NewFromFloat(99),
		Close:     fixedpoint.NewFromFloat(101),
		Volume:    fixedpoint.NewFromFloat(1000),
	})

	if len(errorsSeen) != 1 || !strings.Contains(errorsSeen[0], "pine line 81") {
		t.Fatalf("OnError hook errors = %#v", errorsSeen)
	}
	if runtime.barIndex != 0 || !runtime.hasPreviousClose || !runtime.hasPreviousBarTime {
		t.Fatalf("runtime state after hook error = barIndex:%d hasPreviousClose:%v hasPreviousBarTime:%v", runtime.barIndex, runtime.hasPreviousClose, runtime.hasPreviousBarTime)
	}
}
