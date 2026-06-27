package pineruntime

import (
	"context"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestExecuteOrderStatementHonorsWhenCondition(t *testing.T) {
	runtime, executor, _ := newPineRiskTestRuntime()
	kline := &types.KLine{
		Symbol:    "US.AAPL",
		StartTime: types.Time(time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.June, 29, 13, 30, 59, 0, time.UTC)),
		Open:      fixedpoint.NewFromFloat(101),
		Close:     fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(102),
		Low:       fixedpoint.NewFromFloat(99),
	}
	scope := runtime.newScope(kline, market.SessionRegular)
	statement := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 3, EndLine: 3},
		Action:             strategyir.OrderActionBuy,
		Intent:             strategyir.OrderIntentEntry,
		WhenExpression:     "close > open",
		QuantityMode:       "shares",
		QuantityExpression: "1",
		EntryPolicy:        "allow",
		OrderType:          "MARKET",
	}
	if err := runtime.executeOrderStatement(statement, scope); err != nil {
		t.Fatalf("executeOrderStatement(false when) error = %v", err)
	}
	if len(executor.orders) != 0 {
		t.Fatalf("submitted orders with false when = %#v, want none", executor.orders)
	}

	kline.Open = fixedpoint.NewFromFloat(99)
	kline.Close = fixedpoint.NewFromFloat(100)
	kline.High = fixedpoint.NewFromFloat(101)
	kline.Low = fixedpoint.NewFromFloat(98)
	scope = runtime.newScope(kline, market.SessionRegular)
	if err := runtime.executeOrderStatement(statement, scope); err != nil {
		t.Fatalf("executeOrderStatement(true when) error = %v", err)
	}
	if len(executor.orders) != 1 {
		t.Fatalf("submitted orders with true when = %#v, want one", executor.orders)
	}
}

func TestExecuteExitStatementHonorsWhenCondition(t *testing.T) {
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
		strategy:         &Strategy{},
		entrySubmitCount: map[string]int{"LONG": 1},
		trailingExits:    map[string]trailingExitState{},
	}
	kline := &types.KLine{
		Symbol:    "US.AAPL",
		StartTime: types.Time(time.Date(2026, time.June, 29, 13, 31, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.June, 29, 13, 31, 59, 0, time.UTC)),
		Open:      fixedpoint.NewFromFloat(105),
		High:      fixedpoint.NewFromFloat(110),
		Low:       fixedpoint.NewFromFloat(95),
		Close:     fixedpoint.NewFromFloat(100),
	}
	scope := runtime.newScope(kline, market.SessionRegular)
	runtime.storeCachedPosition("US.AAPL", scope.currentKlineTime, &positionSnapshot{
		Symbol:            "US.AAPL",
		Direction:         "LONG",
		Quantity:          5,
		AvailableQuantity: 5,
		AveragePrice:      100,
		MarketValue:       500,
	})
	statement := &strategyir.ExitStmt{
		Range:              strategyir.SourceRange{StartLine: 4, EndLine: 4},
		ID:                 "Exit",
		Direction:          "long",
		WhenExpression:     "close > open",
		QuantityMode:       "shares",
		QuantityExpression: "2",
		StopExpression:     "96",
	}
	triggered, err := runtime.executeExitStatement(statement, scope)
	if err != nil {
		t.Fatalf("executeExitStatement(false when) error = %v", err)
	}
	if triggered {
		t.Fatal("executeExitStatement(false when) triggered = true, want false")
	}
	if len(executor.orders) != 0 {
		t.Fatalf("submitted exit orders with false when = %#v, want none", executor.orders)
	}

	statement.WhenExpression = "close < open"
	triggered, err = runtime.executeExitStatement(statement, scope)
	if err != nil {
		t.Fatalf("executeExitStatement(true when) error = %v", err)
	}
	if !triggered {
		t.Fatal("executeExitStatement(true when) triggered = false, want true")
	}
	if len(executor.orders) != 1 {
		t.Fatalf("submitted exit orders with true when = %#v, want one", executor.orders)
	}
}
