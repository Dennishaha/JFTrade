package pineruntime

import (
	"strings"
	"testing"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	exprast "github.com/expr-lang/expr/ast"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestRuntimeRiskPositionSizingAndLocks(t *testing.T) {
	t.Run("max_position_size caps risk increasing entries", func(t *testing.T) {
		runtime, _, _ := newPineRiskTestRuntime()
		runtime.maxPositionSize = 5
		if got := runtime.capOrderQuantityToMaxPosition(strategyir.OrderActionBuy, strategyir.OrderIntentEntry, &positionSnapshot{Quantity: 4}, 3); got != 1 {
			t.Fatalf("capOrderQuantityToMaxPosition() = %v, want 1", got)
		}
		if got := runtime.capOrderQuantityToMaxPosition(strategyir.OrderActionShort, strategyir.OrderIntentEntry, &positionSnapshot{Quantity: 10}, 12); got != 12 {
			t.Fatalf("reversal quantity = %v, want unchanged 12", got)
		}
	})

	t.Run("drawdown and intraday loss lock new exposure", func(t *testing.T) {
		runtime, _, session := newPineRiskTestRuntime()
		runtime.maxDrawdownValue = 10
		runtime.maxDrawdownType = "percent_of_equity"
		runtime.maxIntradayLossValue = 5
		runtime.maxIntradayLossType = "cash"
		at := time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)
		runtime.syncRiskState(at)
		session.Account.TotalAccountValue = fixedpoint.NewFromFloat(850)
		runtime.syncRiskState(at.Add(time.Minute))
		if !runtime.riskState.drawdownTriggered {
			t.Fatal("drawdownTriggered = false, want true")
		}
		if !runtime.riskState.intradayLossTriggered {
			t.Fatal("intradayLossTriggered = false, want true")
		}
		blocked, reason := runtime.shouldBlockRiskIncreasingOrder(strategyir.OrderActionBuy, strategyir.OrderIntentEntry, nil, 1)
		if !blocked || !strings.Contains(reason, "max_drawdown") {
			t.Fatalf("shouldBlockRiskIncreasingOrder() = %v, %q", blocked, reason)
		}
	})

	t.Run("consecutive loss days lock future exposure", func(t *testing.T) {
		runtime, _, session := newPineRiskTestRuntime()
		runtime.maxConsLossDays = 2
		day1 := time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)
		runtime.syncRiskState(day1)
		session.Account.TotalAccountValue = fixedpoint.NewFromFloat(900)
		runtime.syncRiskState(day1.Add(2 * time.Hour))

		day2 := day1.Add(24 * time.Hour)
		runtime.syncRiskState(day2)
		if runtime.riskState.consecutiveLossDays != 1 {
			t.Fatalf("day1 consecutiveLossDays = %d, want 1", runtime.riskState.consecutiveLossDays)
		}

		session.Account.TotalAccountValue = fixedpoint.NewFromFloat(800)
		runtime.syncRiskState(day2.Add(2 * time.Hour))
		day3 := day2.Add(24 * time.Hour)
		runtime.syncRiskState(day3)
		if !runtime.riskState.consLossDaysTriggered {
			t.Fatal("consLossDaysTriggered = false, want true")
		}
		blocked, reason := runtime.shouldBlockRiskIncreasingOrder(strategyir.OrderActionBuy, strategyir.OrderIntentEntry, nil, 1)
		if !blocked || !strings.Contains(reason, "max_cons_loss_days") {
			t.Fatalf("shouldBlockRiskIncreasingOrder() = %v, %q", blocked, reason)
		}
	})
}

func TestExecuteOrderStatementHonorsIntradayFilledOrderLimit(t *testing.T) {
	runtime, executor, _ := newPineRiskTestRuntime()
	runtime.maxIntradayFilledOrders = 1
	now := time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)
	scope := &evaluationScope{
		runtime:          runtime,
		currentKlineTime: now,
		currentKline: &types.KLine{
			Symbol: "US.AAPL",
			Close:  fixedpoint.NewFromFloat(100),
		},
	}
	statement := &strategyir.OrderStmt{
		Range:              strategyir.SourceRange{StartLine: 3, EndLine: 3},
		Action:             strategyir.OrderActionBuy,
		Intent:             strategyir.OrderIntentEntry,
		QuantityMode:       "shares",
		QuantityExpression: "1",
		EntryPolicy:        "allow",
		OrderType:          "MARKET",
	}
	if err := runtime.executeOrderStatement(statement, scope); err != nil {
		t.Fatalf("first executeOrderStatement() error = %v", err)
	}
	if err := runtime.executeOrderStatement(statement, scope); err != nil {
		t.Fatalf("second executeOrderStatement() error = %v", err)
	}
	if len(executor.orders) != 1 {
		t.Fatalf("submitted orders = %#v, want one order after intraday risk limit", executor.orders)
	}
}

func newPineRiskTestRuntime() (*strategyRuntime, *pineTestExecutor, *bbgo2.ExchangeSession) {
	session := newPineTestSession()
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
	})
	session.Account = types.NewAccount()
	session.Account.TotalAccountValue = fixedpoint.NewFromFloat(1000)
	session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(100)
	executor := &pineTestExecutor{}
	runtime := &strategyRuntime{
		session:          session,
		executor:         executor,
		strategy:         &Strategy{Symbol: "US.AAPL"},
		symbol:           "US.AAPL",
		expressionCache:  map[string]exprast.Node{},
		entrySubmitCount: map[string]int{},
		pendingOrders:    map[string]pendingOrder{},
		trailingExits:    map[string]trailingExitState{},
	}
	return runtime, executor, session
}
