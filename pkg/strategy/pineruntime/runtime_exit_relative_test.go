package pineruntime

import (
	"context"
	"testing"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestResolveExitTriggerPricesSupportsProfitAndLossTicks(t *testing.T) {
	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {
			Symbol:        "US.AAPL",
			BaseCurrency:  "AAPL",
			QuoteCurrency: "USD",
			TickSize:      fixedpoint.NewFromFloat(0.05),
		},
	})
	runtime := &strategyRuntime{
		ctx:      context.Background(),
		session:  session,
		symbol:   "US.AAPL",
		strategy: &Strategy{},
	}
	scope := runtime.newScope(&types.KLine{
		Symbol: "US.AAPL",
		Close:  fixedpoint.NewFromFloat(100),
	}, "")

	longStmt := &strategyir.ExitStmt{ProfitExpression: "10", LossExpression: "4"}
	longPosition := &positionSnapshot{Direction: "LONG", AveragePrice: 100}
	stopPrice, hasStop, limitPrice, hasLimit, err := runtime.resolveExitTriggerPrices(longStmt, scope, longPosition)
	if err != nil {
		t.Fatalf("resolveExitTriggerPrices(long) error = %v", err)
	}
	if !hasStop || !hasLimit || stopPrice != 99.8 || limitPrice != 100.5 {
		t.Fatalf("long exit prices = stop:%v/%v limit:%v/%v", stopPrice, hasStop, limitPrice, hasLimit)
	}

	shortStmt := &strategyir.ExitStmt{ProfitExpression: "8", LossExpression: "6"}
	shortPosition := &positionSnapshot{Direction: "SHORT", AveragePrice: 100}
	stopPrice, hasStop, limitPrice, hasLimit, err = runtime.resolveExitTriggerPrices(shortStmt, scope, shortPosition)
	if err != nil {
		t.Fatalf("resolveExitTriggerPrices(short) error = %v", err)
	}
	if !hasStop || !hasLimit || stopPrice != 100.3 || limitPrice != 99.6 {
		t.Fatalf("short exit prices = stop:%v/%v limit:%v/%v", stopPrice, hasStop, limitPrice, hasLimit)
	}
}
