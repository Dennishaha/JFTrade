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

func TestResolveProtectRequirementCachesDirectionFlags(t *testing.T) {
	runtime := &strategyRuntime{protectCache: map[*strategyir.ProtectStmt]cachedProtectRequirement{}}
	statement := &strategyir.ProtectStmt{
		Range:                strategyir.SourceRange{StartLine: 11},
		Direction:            "both",
		Mode:                 "stop_loss",
		TimeValueExpression:  "2",
		TimeUnit:             "day",
		PercentageExpression: "4%",
		WindowPolicy:         "continuous",
	}

	requirement, err := runtime.resolveProtectRequirement(statement)
	if err != nil {
		t.Fatalf("resolveProtectRequirement() error = %v", err)
	}
	if requirement.key != "sl:auto:2:day:4" {
		t.Fatalf("requirement.key = %q", requirement.key)
	}
	if !requirement.allowLongExit || !requirement.allowShortExit {
		t.Fatalf("requirement directions = %#v", requirement)
	}

	cached, err := runtime.resolveProtectRequirement(statement)
	if err != nil {
		t.Fatalf("resolveProtectRequirement() cached error = %v", err)
	}
	if cached != requirement {
		t.Fatalf("cached requirement = %#v, want %#v", cached, requirement)
	}
}

func TestExecuteProtectStatementBusinessSemantics(t *testing.T) {
	t.Run("long protect exits available long quantity at market", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		})
		account := session.GetAccount()
		account.SetBalance("AAPL", types.Balance{Available: fixedpoint.NewFromFloat(2.7)})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(3.4)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(101)

		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         executor,
			symbol:           "US.AAPL",
			strategy:         &Strategy{},
			protectCache:     map[*strategyir.ProtectStmt]cachedProtectRequirement{},
			entrySubmitCount: map[string]int{"LONG": 2},
		}
		statement := &strategyir.ProtectStmt{
			Range:                strategyir.SourceRange{StartLine: 21},
			Direction:            "long",
			Mode:                 "stop_loss",
			TimeValueExpression:  "2",
			TimeUnit:             "day",
			PercentageExpression: "4%",
			WindowPolicy:         "continuous",
			Comment:              "long protect",
		}
		key, err := buildProtectRequirementKey(statement)
		if err != nil {
			t.Fatalf("buildProtectRequirementKey() error = %v", err)
		}
		scope := &evaluationScope{
			runtime:          runtime,
			indicators:       map[string]any{key: map[string]any{"longTriggered": true}},
			currentKlineTime: time.Date(2026, time.June, 23, 14, 30, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(101),
			},
		}

		triggered, err := runtime.executeProtectStatement(statement, scope)
		if err != nil || !triggered {
			t.Fatalf("executeProtectStatement() = %v, %v", triggered, err)
		}
		if len(executor.orders) != 1 {
			t.Fatalf("submitted orders = %#v", executor.orders)
		}
		order := executor.orders[0]
		if order.Side != types.SideTypeSell || order.Type != types.OrderTypeMarket || order.Quantity.Float64() != 2 {
			t.Fatalf("protect order = %#v", order)
		}
		if _, exists := runtime.entrySubmitCount["LONG"]; exists {
			t.Fatalf("long protect should clear long count: %#v", runtime.entrySubmitCount)
		}
	})

	t.Run("short protect during warmup suppresses submit but reports triggered", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		})
		account := session.GetAccount()
		account.SetBalance("AAPL", types.Balance{Available: fixedpoint.NewFromFloat(1.9)})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(-2)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(99)

		executor := &pineTestExecutor{}
		runtime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         executor,
			symbol:           "US.AAPL",
			strategy:         &Strategy{WarmupUntil: time.Date(2026, time.June, 23, 14, 31, 0, 0, time.UTC)},
			protectCache:     map[*strategyir.ProtectStmt]cachedProtectRequirement{},
			entrySubmitCount: map[string]int{"SHORT": 3},
		}
		statement := &strategyir.ProtectStmt{
			Range:                strategyir.SourceRange{StartLine: 31},
			Direction:            "short",
			Mode:                 "stop_loss",
			TimeValueExpression:  "2",
			TimeUnit:             "day",
			PercentageExpression: "4%",
			WindowPolicy:         "continuous",
		}
		key, err := buildProtectRequirementKey(statement)
		if err != nil {
			t.Fatalf("buildProtectRequirementKey() error = %v", err)
		}
		scope := &evaluationScope{
			runtime:          runtime,
			indicators:       map[string]any{key: map[string]any{"shortTriggered": true}},
			currentKlineTime: time.Date(2026, time.June, 23, 14, 30, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(99),
			},
		}

		triggered, err := runtime.executeProtectStatement(statement, scope)
		if err != nil || !triggered {
			t.Fatalf("executeProtectStatement(warmup) = %v, %v", triggered, err)
		}
		if len(executor.orders) != 0 {
			t.Fatalf("warmup protect should not submit orders: %#v", executor.orders)
		}
		if runtime.entrySubmitCount["SHORT"] != 3 {
			t.Fatalf("warmup protect should preserve short count: %#v", runtime.entrySubmitCount)
		}
	})

	t.Run("unsupported quantity mode returns line-numbered error", func(t *testing.T) {
		session := newPineTestSession()
		session.SetMarkets(types.MarketMap{
			"US.AAPL": {Symbol: "US.AAPL", BaseCurrency: "AAPL", QuoteCurrency: "USD"},
		})
		account := session.GetAccount()
		account.SetBalance("AAPL", types.Balance{Available: fixedpoint.NewFromFloat(2)})
		pos, _ := session.Position("US.AAPL")
		pos.Base = fixedpoint.NewFromFloat(2)
		pos.AverageCost = fixedpoint.NewFromFloat(100)
		session.LastPrices()["US.AAPL"] = fixedpoint.NewFromFloat(101)

		runtime := &strategyRuntime{
			ctx:              context.Background(),
			session:          session,
			executor:         &pineTestExecutor{},
			symbol:           "US.AAPL",
			strategy:         &Strategy{},
			protectCache:     map[*strategyir.ProtectStmt]cachedProtectRequirement{},
			entrySubmitCount: map[string]int{"LONG": 1},
		}
		statement := &strategyir.ProtectStmt{
			Range:                strategyir.SourceRange{StartLine: 41},
			Direction:            "long",
			Mode:                 "stop_loss",
			QuantityMode:         "weird",
			TimeValueExpression:  "2",
			TimeUnit:             "day",
			PercentageExpression: "4%",
			WindowPolicy:         "continuous",
		}
		key, err := buildProtectRequirementKey(statement)
		if err != nil {
			t.Fatalf("buildProtectRequirementKey() error = %v", err)
		}
		scope := &evaluationScope{
			runtime:          runtime,
			indicators:       map[string]any{key: map[string]any{"longTriggered": true}},
			currentKlineTime: time.Date(2026, time.June, 23, 14, 32, 0, 0, time.UTC),
			currentKline: &types.KLine{
				Symbol: "US.AAPL",
				Close:  fixedpoint.NewFromFloat(101),
			},
		}

		triggered, err := runtime.executeProtectStatement(statement, scope)
		if err == nil || !strings.Contains(err.Error(), `pine line 41: unsupported exit quantity mode "weird"`) {
			t.Fatalf("executeProtectStatement() error = %v", err)
		}
		if triggered {
			t.Fatal("executeProtectStatement() triggered = true, want false")
		}
	})
}

func TestRuntimeEntrypointsInitializeAndFilterKLines(t *testing.T) {
	initStatement := &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 51},
		Name:       "seed",
		Expression: "1",
		Mode:       strategyir.AssignmentModeVar,
	}
	runtime := &strategyRuntime{
		ctx:              context.Background(),
		strategy:         &Strategy{Symbol: "US.AAPL", Interval: types.Interval1m},
		symbol:           "US.AAPL",
		interval:         types.Interval1m,
		hooks:            map[strategyir.HookKind]*strategyir.HookBlock{strategyir.HookInit: {Kind: strategyir.HookInit, Statements: []strategyir.Statement{initStatement}}},
		ifScopePlans:     map[*strategyir.IfStmt]ifScopePlan{},
		expressionCache:  map[string]exprast.Node{},
		bindingCache:     map[*strategyir.LetStmt]cachedIndicatorBinding{},
		protectCache:     map[*strategyir.ProtectStmt]cachedProtectRequirement{},
		persistentValues: map[string]any{},
		baseScope:        &evaluationScope{variables: map[string]any{}},
		barIndex:         -1,
	}

	if err := runtime.runInit(); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}
	if got, ok := coerceFloatValue(runtime.persistentValues["seed"]); !ok || got != 1 {
		t.Fatalf("persistent seed = %#v", runtime.persistentValues["seed"])
	}

	ignored := types.KLine{
		Symbol:    "HK.00700",
		Interval:  types.Interval1m,
		StartTime: types.Time(time.Date(2026, time.June, 23, 13, 30, 0, 0, time.UTC)),
		EndTime:   types.Time(time.Date(2026, time.June, 23, 13, 30, 59, int(999*time.Millisecond), time.UTC)),
		Open:      fixedpoint.NewFromFloat(10),
		High:      fixedpoint.NewFromFloat(11),
		Low:       fixedpoint.NewFromFloat(9),
		Close:     fixedpoint.NewFromFloat(10.5),
		Volume:    fixedpoint.NewFromFloat(100),
	}
	runtime.handleKLineClosed(ignored)
	if runtime.barIndex != -1 || runtime.hasPreviousClose {
		t.Fatalf("ignored kline should not mutate runtime: barIndex=%d hasPreviousClose=%v", runtime.barIndex, runtime.hasPreviousClose)
	}

	matching := types.KLine{
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
	runtime.handleKLineClosed(matching)
	if runtime.barIndex != 0 {
		t.Fatalf("barIndex = %d, want 0", runtime.barIndex)
	}
	if !runtime.hasPreviousClose || !runtime.hasPreviousBarTime {
		t.Fatalf("previous flags = %v/%v, want true/true", runtime.hasPreviousClose, runtime.hasPreviousBarTime)
	}
	if runtime.previousClose != 101 || runtime.previousOpen != 100 || runtime.previousHigh != 102 || runtime.previousLow != 99 || runtime.previousVolume != 1000 {
		t.Fatalf("previous OHLCV = %v/%v/%v/%v/%v", runtime.previousOpen, runtime.previousHigh, runtime.previousLow, runtime.previousClose, runtime.previousVolume)
	}
}
