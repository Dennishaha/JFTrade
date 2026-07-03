package backtest

import (
	"context"
	"testing"

	bbgo2 "github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

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

func TestBacktestSlippageExecutorSubmitsAdjustedMarketOrdersAndPassesCancels(t *testing.T) {
	session := &bbgo2.ExchangeSession{}
	session.SetMarkets(types.MarketMap{
		"US.AAPL": {
			Symbol:   "US.AAPL",
			TickSize: fixedpoint.NewFromFloat(0.01),
		},
	})
	delegate := &fakeWorkerOrderExecutor{}
	executor := newBacktestSlippageExecutor(delegate, session, 2)
	executor.onKLineClosed(types.KLine{
		Symbol: "US.AAPL",
		Close:  fixedpoint.NewFromFloat(100),
	})

	created, err := executor.SubmitOrders(
		context.Background(),
		types.SubmitOrder{
			ClientOrderID: "market-buy",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeBuy,
			Type:          types.OrderTypeMarket,
			Quantity:      fixedpoint.NewFromFloat(1),
		},
		types.SubmitOrder{
			ClientOrderID: "limit-sell",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeSell,
			Type:          types.OrderTypeLimit,
			Price:         fixedpoint.NewFromFloat(101),
			Quantity:      fixedpoint.NewFromFloat(1),
		},
	)
	if err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}
	if len(created) != 2 || len(delegate.submitted) != 2 {
		t.Fatalf("created/submitted = %#v / %#v", created, delegate.submitted)
	}
	marketBuy := delegate.submitted[0]
	if marketBuy.Type != types.OrderTypeLimit || marketBuy.Price.Compare(fixedpoint.NewFromFloat(100.02)) != 0 {
		t.Fatalf("market buy after slippage = %#v, want limit 100.02", marketBuy)
	}
	if marketBuy.Market.TickSize.Compare(fixedpoint.NewFromFloat(0.01)) != 0 {
		t.Fatalf("market buy tick size = %s, want 0.01", marketBuy.Market.TickSize)
	}
	limitSell := delegate.submitted[1]
	if limitSell.Type != types.OrderTypeLimit || limitSell.Price.Compare(fixedpoint.NewFromFloat(101)) != 0 {
		t.Fatalf("limit sell should pass through unchanged, got %#v", limitSell)
	}

	cancelOrder := types.Order{SubmitOrder: types.SubmitOrder{ClientOrderID: "limit-sell", Symbol: "US.AAPL"}}
	if err := executor.CancelOrders(context.Background(), cancelOrder); err != nil {
		t.Fatalf("CancelOrders error = %v", err)
	}
	if len(delegate.cancelled) != 1 || delegate.cancelled[0].ClientOrderID != "limit-sell" {
		t.Fatalf("cancelled orders = %#v", delegate.cancelled)
	}
}
