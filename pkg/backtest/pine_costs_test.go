package backtest

import (
	"context"
	"math"
	"strconv"
	"testing"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestPineCommissionRateConvertsPercentToRate(t *testing.T) {
	got := pineCommissionRate(strategyir.StrategyMetadata{
		CommissionType:  "percent",
		CommissionValue: 0.15,
	})
	if got.Compare(fixedpoint.NewFromFloat(0.0015)) != 0 {
		t.Fatalf("commission rate = %s, want 0.0015", got.String())
	}
	if got := pineCommissionRate(strategyir.StrategyMetadata{CommissionType: "cash_per_order", CommissionValue: 2}); !got.IsZero() {
		t.Fatalf("cash commission rate = %s, want zero exchange rate", got.String())
	}
}

func TestResolvePineInitialBalancePrecedence(t *testing.T) {
	metadata := strategyir.StrategyMetadata{InitialCapital: 250000}
	if got := resolvePineInitialBalance(50000, metadata); got != 50000 {
		t.Fatalf("explicit balance = %v, want 50000", got)
	}
	if got := resolvePineInitialBalance(0, metadata); got != 250000 {
		t.Fatalf("script balance = %v, want 250000", got)
	}
	if got := resolvePineInitialBalance(0, strategyir.StrategyMetadata{}); got != 100000 {
		t.Fatalf("default balance = %v, want 100000", got)
	}
}

func TestBacktestSlippagePriceUsesMarketTickSize(t *testing.T) {
	session := &bbgo2.ExchangeSession{}
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {
			Symbol:   "US.AAPL",
			TickSize: fixedpoint.NewFromFloat(0.01),
		},
	})
	executor := newBacktestSlippageExecutor(nil, session, 3)
	executor.onKLineClosed(types.KLine{
		Symbol: "US.AAPL",
		Close:  fixedpoint.NewFromFloat(100),
	})

	buy, _, ok := executor.slippagePrice(types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy})
	if !ok || buy.Compare(fixedpoint.NewFromFloat(100.03)) != 0 {
		t.Fatalf("buy slippage = %s, ok=%v, want 100.03", buy.String(), ok)
	}
	sell, _, ok := executor.slippagePrice(types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeSell})
	if !ok || sell.Compare(fixedpoint.NewFromFloat(99.97)) != 0 {
		t.Fatalf("sell slippage = %s, ok=%v, want 99.97", sell.String(), ok)
	}
}

func TestRunAppliesPineInitialCapitalCommissionAndSlippage(t *testing.T) {
	isolateBacktestHome(t)
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()

	run := func(script string) *RunResult {
		cfg := strategyBlockBenchmarkRunConfig(dbPath, startTime, endTime, script)
		cfg.InitialBalance = 0
		result := Run(context.Background(), cfg)
		if result == nil || result.Error != "" {
			t.Fatalf("Run() result = %#v", result)
		}
		if len(result.RuntimeErrors) != 0 {
			t.Fatalf("RuntimeErrors = %#v", result.RuntimeErrors)
		}
		if len(result.Trades) == 0 {
			t.Fatal("expected at least one trade")
		}
		return result
	}

	base := run(`//@version=6
strategy("No Costs", initial_capital=123456)
strategy.entry("Long", strategy.long, qty=1)`)
	withCosts := run(`//@version=6
strategy("Costs", initial_capital=123456, commission_type=strategy.commission.percent, commission_value=1, slippage=2)
strategy.entry("Long", strategy.long, qty=1)`)

	if math.Abs(base.PnL+123456-base.FinalBalance) > 1e-8 {
		t.Fatalf("base initial capital not applied: final=%v pnl=%v", base.FinalBalance, base.PnL)
	}
	basePrice, err := strconv.ParseFloat(base.Trades[0].Price, 64)
	if err != nil {
		t.Fatalf("parse base trade price: %v", err)
	}
	costPrice, err := strconv.ParseFloat(withCosts.Trades[0].Price, 64)
	if err != nil {
		t.Fatalf("parse cost trade price: %v", err)
	}
	if costPrice <= basePrice {
		t.Fatalf("slippage trade price = %s, base = %s", withCosts.Trades[0].Price, base.Trades[0].Price)
	}
	if withCosts.FinalBalance >= base.FinalBalance {
		t.Fatalf("cost final balance = %v, base = %v", withCosts.FinalBalance, base.FinalBalance)
	}
}
