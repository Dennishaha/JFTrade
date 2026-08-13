package backtest

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestBacktestSourceExchangeExposesPinnedRulesWithoutLiveOperations(t *testing.T) {
	spec := resolveInstrumentSpec("HK.00700", InstrumentSpec{})
	if spec.Symbol != "HK.00700" || spec.QuoteCurrency != "HKD" || spec.TickSize <= 0 ||
		spec.LotSize != 1 || spec.QuantityStep != 1 || !spec.MissingCriticalRules || len(spec.Warnings) != 1 {
		t.Fatalf("resolved HK spec = %+v", spec)
	}
	exchange := newBacktestSourceExchange(spec)
	if exchange.Name() != types.ExchangeBacktest || exchange.PlatformFeeCurrency() != "HKD" || exchange.NewStream() == nil {
		t.Fatalf("source identity = %q/%q", exchange.Name(), exchange.PlatformFeeCurrency())
	}
	markets, err := exchange.QueryMarkets(t.Context())
	if err != nil || markets[spec.Symbol].MinQuantity.Float64() != 1 || markets[spec.Symbol].TickSize.Float64() <= 0 {
		t.Fatalf("source markets = %+v, %v", markets, err)
	}
	if ticker, err := exchange.QueryTicker(t.Context(), spec.Symbol); err == nil || ticker != nil {
		t.Fatalf("QueryTicker = %+v, %v", ticker, err)
	}
	if tickers, err := exchange.QueryTickers(t.Context(), spec.Symbol); err == nil || tickers != nil {
		t.Fatalf("QueryTickers = %+v, %v", tickers, err)
	}
	if klines, err := exchange.QueryKLines(t.Context(), spec.Symbol, types.Interval1m, types.KLineQueryOptions{}); err == nil || klines != nil {
		t.Fatalf("QueryKLines = %+v, %v", klines, err)
	}
	if account, err := exchange.QueryAccount(t.Context()); err != nil || account == nil {
		t.Fatalf("QueryAccount = %+v, %v", account, err)
	}
	if balances, err := exchange.QueryAccountBalances(t.Context()); err != nil || len(balances) != 0 {
		t.Fatalf("QueryAccountBalances = %+v, %v", balances, err)
	}
	if order, err := exchange.SubmitOrder(t.Context(), types.SubmitOrder{}); err == nil || order != nil {
		t.Fatalf("SubmitOrder = %+v, %v", order, err)
	}
	if orders, err := exchange.QueryOpenOrders(t.Context(), spec.Symbol); err != nil || orders != nil {
		t.Fatalf("QueryOpenOrders = %+v, %v", orders, err)
	}
	if err := exchange.CancelOrders(t.Context()); err != nil {
		t.Fatalf("CancelOrders = %v", err)
	}
}

func TestInstrumentSpecConservativeDefaultsRespectMarketProfiles(t *testing.T) {
	sh := resolveInstrumentSpec(" sh.600000 ", InstrumentSpec{})
	if sh.Symbol != "SH.600000" || sh.LotSize != 100 || sh.QuantityStep != 100 || sh.QuoteCurrency != "CNY" {
		t.Fatalf("A-share spec = %+v", sh)
	}
	custom := resolveInstrumentSpec("US.AAPL", InstrumentSpec{
		QuoteCurrency: "CHF", PricePrecision: 7, QuotePrecision: 8,
		TickSize: 0.125, LotSize: 5, QuantityStep: 2,
	})
	if custom.QuoteCurrency != "CHF" || custom.PricePrecision != 7 || custom.QuotePrecision != 8 ||
		custom.TickSize != 0.125 || custom.LotSize != 5 || custom.QuantityStep != 2 {
		t.Fatalf("custom spec overwritten = %+v", custom)
	}
	unknown := resolveInstrumentSpec("XX.TEST", InstrumentSpec{})
	if unknown.TickSize != 0.01 || unknown.LotSize != 1 || unknown.QuantityStep != 1 || unknown.QuoteCurrency == "" {
		t.Fatalf("unknown-market spec = %+v", unknown)
	}
	if table := KLineTableNameForProviderAndSessionScope("yfinance", "US.AAPL", types.Interval1m, "none", "regular"); table == "" {
		t.Fatal("provider-scoped K-line table name is empty")
	}
}
